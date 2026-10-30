# Design

## Context

See proposal.md for motivation. Today `App.Start` closes every open history row (`CloseDanglingHistory`), then runs
the prober, the enforcer and a `janitor` goroutine that every minute finishes abandoned launches (`finishAbandoned`,
which only knows this process's tunnels) and removes expired web sessions, and once a day purges audit events and
recordings. Recordings have per-set state on the history row (`video_files`, `text_files`: present, purging, purged,
error) and `purgeRecordings` claims a set with `BeginPurge`, recovers half-done purges with `RecoverPurging`, and
uses the connection's **current** retention (the row's frozen retention only when the connection was deleted). guacd
writes the files on the shared recordings volume; `fileSet(tunnelID, video)` lists every file of a set. UI-editable
settings are sealed JSON rows in `settings`. The runtime image is Alpine with one non-root process.

AD Manager uses asynq v0.26 (`Client`, `Server`, `Scheduler`, `Inspector`), the `jniltinho/asynqmon` UI behind its
session check, rclone embedded as a library, and an all-in-one image whose supervisord runs Redis on 127.0.0.1 with
no persistence next to the panel.

## Goals / Non-Goals

**Goals:**
- Background work that survives restarts and Redis loss, runs on 1..N replicas without overlap, and is visible.
- Recording sets copied to object storage without touching the session path, verified before any local removal.
- One storage layer (rclone) for S3, S3-compatible, OCI and GCS.

**Non-Goals:**
- guacd writing to the bucket; client-side encryption (rclone `crypt`); several simultaneous targets or
  per-connection targets; moving the audit log or the database.
- Exactly-once execution from the queue: the queue is at-least-once; correctness comes from database claims and
  idempotent effects.

## Decisions

### D1. Container: supervisord with embedded Redis
The runtime image adds `supervisor` and `redis` (Alpine packages) and a `supervisord.conf` modeled on
`ad-manager/docker/allinone`: `[program:redis]` runs `redis-server --bind 127.0.0.1 --port 6379 --save "" --appendonly
no --dir /tmp`, `[program:goguacamole]` runs `goguacamole serve`; both log to stdout. Unlike AD Manager, supervisord
itself runs as the `goguacamole` user (`USER goguacamole` in the image) and no program sets `user=`, so no setuid or
setgid is needed: the image keeps working under the Compose file's `cap_drop: [ALL]`, `no-new-privileges` and a
read-only root, with the pid file and control socket on a tmpfs (`/run`, `/tmp`). The entrypoint sets
`autostart=false` for Redis when `REDIS_EMBEDDED=false` and defaults `REDIS_ADDR=127.0.0.1:6379` otherwise. The healthcheck stays `goguacamole healthcheck`. No persistence is needed because the database is the
source of truth (D4). *Alternative:* a separate Redis service in Compose only — rejected as the default because a
single `docker run` must work, but Compose keeps a `redis` profile for external Redis.

### D2. asynq workers inside goguacamole, dedicated database
Every instance runs an `asynq.Server` on queues `goguacamole:critical`, `goguacamole:default`, `goguacamole:low`
(concurrency `[jobs] concurrency`, default 2); `[jobs] worker = false` makes an instance enqueue only. The database is
marked with a `goguacamole:installation` key holding the installation id (set if absent). At startup the
`asynq.Inspector` lists queues: any queue without the prefix, or a mark with another id, means a shared database
(spec "Dedicated Redis database") and disables workers and the dashboard.

### D3. Periodic jobs: per-fire enqueue plus a database lease
`asynq.Scheduler` reuses the task registered at start, so it cannot stamp a new period id. Instead each instance runs
a small ticker (every 30 s) that computes, in UTC, the current period of each periodic job and enqueues it with
`TaskID("<job>:<period>")`; a conflict means another replica already did and is ignored. A `job_runs` table records
the last completed period per job, so a period missed while all instances were down is enqueued at the next start.
The handler acquires a row in `job_leases` (`job`, `owner` instance id, `run_token` new for every run,
`expires_at`) with an atomic insert-or-update-if-expired, cancels its context when a renewal fails, then reads `job_runs` inside that lease and ends without effect when the period is
already completed (asynq forgets a finished `TaskID` after its retention, so the id alone is not enough); it renews
the lease while running and records the completed period and releases the lease in one transaction conditioned on
its `run_token` and an unexpired lease. "Purge now" uses
the same lease.
`RecoverPurging` only resets sets whose purge claim (D6) expired. Audit events of periodic jobs carry the period, so
a retried run does not log twice.

### D4. Database as source of truth, reconciliation
Upload owed, uploading, deletion pending are columns of the recording set (D7) written in the same transaction as the
event (history end, purge). A periodic `jobs:reconcile` (every 5 minutes) moves sets left `uploading` with an expired claim
back to `owed`, marks as owed every set present on disk and not stored remotely while uploads are enabled (this also
covers a crash between `EndHistory` and `finishCapture`, which records the files only afterwards), removes versions of
remote sets' keys other than the verified one, then selects sets that are owed/failed-retryable with no valid claim and enqueues `recording:upload`
with `TaskID("upload:<history>:<set>:<attempt>")`. Queue loss only
delays work.

### D5. When a recording is closed
The end of the relay does not prove guacd closed its files (guacd may still flush). The owner writes
`ended_at` and the upload owed in the transaction that ends the row; the upload handler then requires every file of
the set (from `fileSet`) to keep the same size and modification time over a quiet period (2 minutes) before copying.
guacd gives no close signal, so two more nets bound the risk: local removal waits a safety window (1 hour by default)
after the session ended, and re-hashes the files against the uploaded SHA-256 (D14): a file that changed after upload
is uploaded again instead of being removed. For rows whose owner disappeared, liveness decides: history rows get `owner_instance`
and instances renew `instance_heartbeats` every 30 s; `CloseDanglingHistory` and `finishAbandoned` are changed to
close only rows of this instance or of instances whose heartbeat expired (fixing today's cross-replica close). A row
closed by expiry follows the same quiet-period rule. `fileSet` provides every file of the set (numbered parts,
`.timing`).

### D6. Atomic claims per set
Each set has `claim_owner`, `claim_token` (random, new on every claim) and `claim_until`. Upload, local removal and
purge take the claim with a conditional update (`... WHERE claim is free or expired AND state IN (...)`); every later
update is fenced with `WHERE claim_token = ? AND claim_until > now`, so a worker whose claim expired cannot write,
even before a successor claims the set. Remote effects are idempotent (same keys); a version written by a stale worker
is removed by the reconciliation's version sweep. Workers renew the claim
during transfers and cancel their context when a renewal fails. An upload that finds the set purged deletes the
objects it wrote. The purge skips (until next run) sets whose claim is held. `SetHold` refuses with 409 while any claim
(upload, local removal, purge) is valid on a set of the recording; local removal checks `hold_at IS NULL` when taking
its claim and keeps the claim until the files are unlinked, so a hold cannot slip in between.

### D7. Data model
Migration `0008_recording_storage`:
- `storage_targets` (`id`, `version`, sealed config, `created_at`, `retired_at`); settings point to the current one.
- `history` per set (`video_*`, `text_*`): `storage_state` (`local|owed|uploading|remote|failed|delete_pending|
  deleted`), `target_id`, `remote_keys` (JSON of key → version id), `sha256` (JSON), `attempts`, `storage_error`
  (sanitized, ≤ 500 chars), `uploaded_at`, `local_removed`, `claim_owner`, `claim_token`, `claim_until`; plus `owner_instance` on the row.
- `job_runs`, `job_leases`, `instance_heartbeats`; the installation id in `settings`.
The down migration refuses (before any DDL) while any set has a remote copy, an upload in progress or a deletion
pending, since the targets and manifests would be lost. Upgrade tests start from a populated 0007
database on SQLite, MariaDB and PostgreSQL, and the Makefile sets `SHELL := /bin/bash` and `.SHELLFLAGS := -eo pipefail -c` (dash has no
`pipefail`), so a failing `go test` cannot pass through a pipe.

### D8. rclone as a library
`internal/objstore` imports only `backend/s3`, `backend/oracleobjectstorage`, `backend/googlecloudstorage`,
`backend/local` and, for tests, `backend/memory`. Remotes are built with `configmap.Simple` and `fs.NewFs`; the config
storage is replaced at init with an in-memory one so no `rclone.conf` is read or written. rclone's log output is
redirected, before any backend is created, to a handler that runs the sanitizer (D11) at level ERROR. Copy uses
`operations.Copy` (streaming, multipart). The module is pinned to a release; AD Manager's patched fork is not needed.

OCI: the S3 compatibility endpoint `https://<namespace>.compat.objectstorage.<region>.oraclecloud.com` with Customer
Secret Keys is the default OCI path. The native backend is offered only with `instance_principal_auth` or
`workload_identity_auth` (no secrets to store); `user_principal_auth` needs an OCI config file on disk, so it is out
of scope. *Alternatives:* three provider SDKs (three auth models and retry policies), minio-go (S3 only).

### D9. Verification and exact versions
After the copy, the handler opens each object and hashes it (SHA-256) while streaming, and compares with the local
digest. A provider checksum is accepted instead only when the provider computed it over the stored bytes: GCS MD5 for
non-composite objects, S3 ETag for single-part uploads without SSE-KMS. MD5 kept by rclone in S3 metadata for
multipart uploads is client-supplied and never accepted. Keys are unique per installation and history row and only GoGuacamole writes them, so deletion lists and removes
every version of the set's keys (S3 versions, GCS generations) and then checks that no version remains; after a
verified upload, versions left by earlier attempts are removed the same way. The verified version id is recorded for
diagnostics. *Test* reports bucket versioning and object lock so administrators see the deletion behavior in advance.

### D10. Endpoint safety
S3-compatible endpoints go through a dialer that resolves the host and rejects loopback, link-local (incl.
169.254.169.254 and fd00:ec2::254), RFC 1918, CGNAT and ULA addresses unless the host or CIDR is in
`[objstore] allowed_endpoints` (server configuration); `allow_http = false` by default; the HTTP client disables
redirects and environment proxies (`Proxy: nil`, rclone's proxy options unset). Checks happen at dial time, so DNS
rebinding cannot bypass them.

### D11. Secrets hygiene
A sanitizer removes configured secret values, their base64 forms, JSON key material, `Authorization` headers and
signature query parameters (`X-Amz-Signature`, `X-Goog-Signature`, `Signature`) from any text before it reaches logs,
stored errors, *Test* results or the queue. Payloads hold ids only.

### D12. asynqmon protection
`/asynqmon` and `/asynqmon/*` require a session with `ADMINISTER` (redirect to `/login?next=/asynqmon` without a
session, 403 without permission). Non-GET requests must pass the Origin / `Sec-Fetch-Site: same-origin` check used by
the API, and are audited as `jobs.<method>` with the path. CSP is relaxed only on these paths, as in AD Manager.

### D13. Orphans and installation namespace
Keys live under `<prefix>/<installation-id>/…`; the installation id is generated once and stored in the database. The
orphan sweep lists only that namespace and only keys matching the layout (`yyyy/mm/dd/<digits>/<known file name>`).

### D15. Reading recordings
`recordingReader(h, set)` opens the local files when present, otherwise the remote objects of the set's
`target_id`/versions; playback, transcript text and raw downloads use it with the existing permission checks, audit
events and headers.

### D14. Retention unchanged, removal re-checks
The purge keeps today's rule (current connection retention; frozen only after connection deletion) per set; for sets
with remote copies it deletes the exact versions first, then the local files; failures set `delete_pending`. Expiry
removes local files of sets never uploaded. Holds block every removal path, including the local copy policy, whose
removal re-hashes the files first and uploads again if they changed.

## Risks / Trade-offs

- [Binary size grows with rclone] → only three backends; measured in the tasks; `noobjstore` build tag for minimal
  builds.
- [Image now runs two processes] → supervisord as in AD Manager; goguacamole stays non-root; Redis bound to loopback.
- [Read-back verification costs egress] → only once per set; accepted provider checksums avoid it where trustworthy.
- [guacd has no explicit close signal] → quiet period before upload, safety window and re-hash before local removal;
  the local copy is the last thing ever removed.
- [Heartbeat-based ownership needs a shared clock] → expiries are generous (minutes) and use database time.
- [asynq archive is size-limited] → failures are recorded on the set in the database; the archive is diagnostic only.
- [Object lock can keep data longer than retention] → reported as deletion pending, retried, never bypassed.

## Migration Plan

1. Deploy the new image: migrations run; embedded Redis starts; upkeep moves to jobs; uploads stay off.
2. Configure Storage, run *Test*, enable uploads; optionally *Upload existing recordings*.
3. Optionally switch the local policy to *remove after upload*.
4. Replicas: set `REDIS_EMBEDDED=false` and a shared, dedicated `REDIS_ADDR` on every instance.

Rollback: disable uploads (remote sets stay readable through their target versions); `REDIS_EMBEDDED=false` without
`REDIS_ADDR` returns upkeep to the janitor; the down migration refuses while sets are remote-only.

## Open Questions

- Which `jniltinho/asynqmon` tag to pin (AD Manager uses v0.8.1; newer tags take the same options).
