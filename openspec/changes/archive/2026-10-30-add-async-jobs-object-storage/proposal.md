# Proposal

## Why

Recordings (screen `.guac`, SSH typescripts) live only on the local recordings volume. A disk failure, a lost
container volume or a full disk takes the audit trail with it, and auditors ask for the evidence to sit in an object
store with its own retention and immutability. Upkeep (retention purge, orphan sweep, storage usage) also runs in an
in-process ticker of every instance, with no retries, no visibility and duplicate work when several replicas run. AD
Manager already solved both with asynq (Redis), the asynqmon dashboard, rclone's engine and a supervisord container
that embeds Redis, so we reuse that proven pattern.

## What Changes

- **Job queue:** add [asynq](https://github.com/hibiken/asynq) on Redis for background work, with workers inside the
  goguacamole binary. The database stays the source of truth for every recording's storage state, so losing Redis
  (or running an ephemeral one) delays work but never loses it; a reconciliation job re-enqueues what is pending.
  Without Redis the current in-process upkeep keeps working.
- **Container with embedded Redis:** the image runs supervisord with Redis bound to `127.0.0.1` and goguacamole, like
  AD Manager's all-in-one image; `REDIS_ADDR` points to it by default. Deployments with several replicas set an
  external, dedicated Redis and turn the embedded one off.
- **Scheduled upkeep as jobs:** retention purge, orphan sweep, storage usage and expired-session cleanup run as
  periodic jobs, generated per UTC period, with retries, and with a database lease so two runs never overlap.
- **Recording upload to object storage:** once a session's recording files are closed, a job uploads each recording
  set (video; transcript + timing) to an object store, verifies it by reading it back (SHA-256), records the target,
  keys and object versions, and (per policy) removes the local copy.
- **Targets through rclone (library):** AWS S3, S3-compatible services (MinIO, Ceph, Wasabi, Cloudflare R2, Backblaze
  B2, OCI's S3 compatibility API) and Google Cloud Storage; OCI Object Storage natively with instance or workload
  principal authentication. Configured in memory from sealed settings, never through `rclone.conf`.
- **Administration → Recording → Storage:** target settings with write-only credentials, endpoint safety rules, a
  *Test* that also reports bucket versioning and object lock, keep-local policy, upload existing recordings. Target
  changes create a new target version; older versions stay readable while recordings use them.
- **Playback reads from wherever the file is:** local first, else streamed from the target that holds it; the history
  shows each set's storage state.
- **Retention and legal hold reach remote copies:** the purge deletes the exact object versions it uploaded, a held
  recording is never deleted anywhere, and refused or pending deletions stay visible and retried.
- **asynqmon** at `/asynqmon` for administrators only, on a dedicated Redis database.

## Capabilities

### New Capabilities
- `job-queue`: background work on asynq/Redis — the embedded Redis of the container, enqueueing and workers, periodic
  jobs per UTC period with a database lease, retries and failures recorded in the database, reconciliation after
  Redis loss, the in-process fallback without Redis, and the administrator-only asynqmon dashboard.
- `recording-storage`: where recordings are kept — local volume and versioned object storage targets (S3,
  S3-compatible, OCI, GCS) through rclone, endpoint safety, asynchronous verified upload per recording set, storage
  state, playback from either place, and retention/legal hold over remote copies.

### Modified Capabilities
<!-- none in openspec/specs yet: the recording policy (retention, legal hold, fail-closed) is still the unarchived
     change `add-recording-policy-per-connection`; `recording-storage` keeps its retention rule (the connection's
     current policy, frozen only when the connection is deleted) and states how the purge reaches remote copies -->

## Impact

- **Code:** new `internal/jobs` (asynq client/server, per-period enqueuer, leases, reconciliation, fallback runner),
  new `internal/objstore` (rclone backends, in-memory config, safe dialer, sanitized logging, put/verify/open/delete
  by version), changes in `internal/server` (janitor → jobs, history end → pending upload, readers, Recording
  settings API, `/asynqmon`), `internal/store` (per-set storage columns, targets, leases, instance ownership of open
  history rows), `internal/config` (`[redis]`, `[jobs]`, `[objstore]`), frontend Recording settings (Storage panel)
  and history/Sessions views.
- **Database:** migrations for storage targets, per-set storage state on history rows, job leases and history owner
  instance; tested as upgrades from a populated 0007 on SQLite, MariaDB and PostgreSQL.
- **APIs:** `GET/PUT /api/v1/settings/recording/storage`, `POST .../storage/test`, `POST .../storage/backfill`,
  storage state in history responses, retry of a failed upload; `/asynqmon` (administrators).
- **Container:** runtime image gains `supervisor` and `redis`; entrypoint decides whether the embedded Redis starts.
- **Dependencies:** `github.com/hibiken/asynq`, `github.com/jniltinho/asynqmon`, `github.com/rclone/rclone` (selected
  backends only); binary size grows (measured in the tasks).
- **Operations:** a dedicated Redis database (embedded by default); bucket permissions (put/get/delete/list and
  version operations on a prefix); private endpoints only through the operator's allowlist; the recordings volume
  stays shared across replicas.
