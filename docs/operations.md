# Operations

## First start

On an empty database `serve` creates the administrator from `APP_ADMIN_USER` / `APP_ADMIN_PASSWORD` (or `admin`
with a random password printed once in the log). The administrator must change the password at first sign-in.

## Backups

| Driver | Backup |
|---|---|
| SQLite | `sqlite3 /data/goguacamole.db ".backup /backup/gg.db"` (safe while running, WAL mode) or stop the app and copy the volume |
| MariaDB | `mariadb-dump --single-transaction goguacamole` |
| PostgreSQL | `pg_dump -Fc goguacamole` |

Always back up the **secret key** (`APP_SECRET_KEY` or `/data/secret.key`) with the database.

## Upgrades

Pull the new image (or binary) and restart; pending migrations run automatically. `goguacamole migrate status`
lists them; `migrate down` rolls back one step.

## Importing from Apache Guacamole

```bash
goguacamole import guacamole --source-driver mysql|postgres --dsn '<guacamole db dsn>' [--dry-run] \
  [--conflict skip|update] [--accept-expansion]
```

- Source opened in a READ ONLY transaction, `SELECT` only.
- Imported: users (hash/salt, disabled, expired → must change password, validity dates, access window, time zone,
  name, e-mail), user groups and memberships (nested groups flattened; paths through disabled groups grant nothing),
  connection folders, connections + parameters (secrets encrypted), guacd overrides, limits, system permissions and
  connection READ grants.
- Not imported (reported): connection-group READ grants (upstream they do not grant child connections), sharing
  profiles, non-READ object permissions. Balancing-group READ becomes READ on each child only with `--accept-expansion`.
- After writing, the importer compares each user's launchable connections in source and target and prints
  `ACCESS DIFF` lines. Re-runs are idempotent (`skip` by default).

## Users from the CLI

```bash
goguacamole user create alice --admin          # password from stdin
goguacamole user passwd alice                  # ends alice's sessions
goguacamole user disable alice                 # open tunnels close within 5 s
goguacamole user list
```

## TLS

```bash
make certs                                   # local CA + server cert in deploy/certs (labs)
TLS_ENABLED=true TLS_CERT_FILE=... TLS_KEY_FILE=... goguacamole serve --addr :8443
kill -HUP <pid>                              # reload rotated certificates
```

## Session recording

What is recorded is a policy (openspec `recording-policy`):

- **Administration → Recording** (`ADMINISTER`): defaults for video (screen of SSH, RDP and VNC), text (SSH transcript),
  keystrokes (inside the video only) and retention per kind (1–3650 days or keep forever). Until saved, the defaults
  come from `RECORDING_*`.
- **Connection → Recording** (`AUDIT` or `ADMINISTER`, dedicated endpoint): `inherit | on | off` per kind and
  retention overrides. Applies to sessions launched afterwards; the policy is snapshotted per launch (notice to the
  user, guacd parameters and history all use that snapshot). Recording parameters typed in connection parameters are
  rejected; legacy ones are converted to overrides at startup and by `goguacamole import` (listed under *Converted
  from connection parameters*). Changing a connection's protocol or guacd endpoint requires `ADMINISTER`.
- **Fail closed**: a launch that must be recorded is refused (`RECORDING_UNAVAILABLE`) when the recordings directory
  is not writable; an administrator can grant a time-limited exception (≤ 24 h, reason, audited) during an incident.
  ~10 s after connecting, a session whose files guacd did not create is closed (`RECORDING_FAILED`) and audited.
- **Retention**: a daily purge (and *Purge now*) deletes expired video/text file sets (`.timing` and numbered files
  included) by session end time and the **current** retention — shortening asks for confirmation with the number of
  recordings that will go. Deleting a connection freezes its retention in its history entries. Held entries and
  running sessions are never touched; orphan files are removed after the longest finite retention + 7 days (never when
  a default is keep forever). History rows stay, marked expired/missing; one `recording.purge` audit event per run.
- **Legal hold** (`AUDIT`): lock icon in the history, reason required; held recordings and the audit events of their
  connection survive retention. Reasons are visible only to auditors.
- **Single instance**: these guarantees assume one GoGuacamole process and one recordings volume.
- **Rollback**: `migrate down` refuses while holds exist. Older builds purge by file age, so start them with
  `RECORDING_RETENTION_DAYS=3650` or `RECORDING_ENABLED=false`.

guacd writes `/record/<tunnel-uuid>` (screen) and, for SSH, `/record/<tunnel-uuid>.typescript` (+ `.timing`).
Auditors open **Sessions → History → Watch / Read**: the screen recording plays in the browser (play, pause, seek)
and the transcript shows every command typed and its output (searchable; raw download replays with
`scriptreplay <file>.typescript.timing <file>.typescript`). Every view is audited (`recording.view`).

- Share the directory: guacd (uid 1000) writes; goguacamole reads/deletes (compose: `record-init` + `group_add: ["1000"]`).
  Outside compose, mount the same storage and set `RECORDING_LOCAL_PATH` to where goguacamole sees it.
- Size: a busy RDP session can produce tens of MB per hour; watch **Retention and storage** (usage by kind and top
  connections) and size the volume and retentions accordingly.
- Passwords typed in a terminal are part of the transcript (as in the Java client); restrict `AUDIT`.

## Job queue (Redis)

Background upkeep (retention purge, orphan sweep, storage usage, expired web sessions) and, with object storage,
recording uploads run as jobs on [asynq](https://github.com/hibiken/asynq) when `REDIS_ADDR` is set. The Compose stack
ships a `redis` service and the all-in-one image runs Redis inside the container (`docker/README.md`). Without
`REDIS_ADDR` the upkeep runs in-process, as before, and object storage is unavailable.

- **Dedicated database.** Queues are named `goguacamole:critical|default|low` and the database is marked with the
  installation id (`goguacamole:installation`). If the database holds another application's queues or another
  installation's mark, the queue stays off (state `conflict`) and upkeep runs in-process: give each installation its
  own Redis or `REDIS_DB`.
- **No persistence needed.** Work that matters (uploads owed, deletions pending) is recorded in the database and
  re-enqueued by a reconciliation job, so an empty or restarted Redis only delays it.
- **Replicas** must share one Redis (`REDIS_EMBEDDED=false` and the same `REDIS_ADDR` in the all-in-one image).
  Sessions belong to the instance serving them; an instance that stops sending heartbeats for 3 minutes has its open
  sessions ended by the others.
- **State:** `/ready` reports `queue` as `up`, `down` (retrying), `conflict` or `disabled`; the queue being down never
  makes an instance unready.

## Recording storage (object storage)

**Administration → Recording → Storage** copies every recording set (screen video; SSH transcript with its timing
file) to an object store after the session ends, verifies it by reading it back (SHA-256) and can then remove the local
copy. It needs the job queue. Keys are `<prefix>/<installation id>/<yyyy>/<mm>/<dd>/<history id>/<file>`.

| Provider | Settings | Notes |
|---|---|---|
| AWS S3 | region, bucket, access key + secret (or an instance role when the keys are empty) | SSE `AES256` or `aws:kms` + KMS key id |
| S3-compatible | endpoint (HTTPS), bucket, path-style, access key + secret | MinIO, Ceph, Wasabi, Cloudflare R2, Backblaze B2 |
| Oracle Cloud (S3 API) | namespace, region, bucket, Customer Secret Key (access key + secret) | endpoint `https://<namespace>.compat.objectstorage.<region>.oraclecloud.com` |
| Oracle Cloud (native) | namespace, region, compartment OCID, bucket, instance or workload principal | no secret stored; user principal (OCI config file) is not supported |
| Google Cloud Storage | bucket, service account JSON | uniform bucket-level access |

- **Permissions** on the prefix: put, get, list, delete — and, for versioned S3 buckets, list and delete versions.
- **Versioning and object lock.** GoGuacamole deletes every version of a recording when its retention ends; *Test*
  reports whether the bucket keeps versions and whether deletion is refused. An object lock longer than the retention
  keeps the recording as *deletion pending* until the lock expires. GCS and OCI native list current objects only: keep
  versioning off on those buckets (or expire noncurrent versions with a lifecycle rule).
- **Private endpoints.** Loopback, private, link-local and metadata addresses are refused unless listed in
  `objstore.allowed_endpoints`; plain HTTP needs `objstore.allow_http`. Object storage connects directly: the
  environment's HTTP proxies (`HTTPS_PROXY`) are not used for it.
- **Playback** and transcripts read the local copy when there is one, else the copy in the bucket the set was uploaded
  to (changing the bucket later keeps older recordings readable). *Upload existing recordings* queues every recording
  not in storage yet, failed ones included.
- Credentials are sealed with the application secret, never shown again and never logged.

## Health and logs

- `/health` (liveness), `/ready` (DB + guacd). The image `HEALTHCHECK` runs `goguacamole healthcheck`.
- Logs are structured (`APP_LOG_FORMAT=json` in containers) with `request_id`; tunnel open/close lines include user,
  connection and Guacamole status.

## Troubleshooting

| Symptom | Check |
|---|---|
| "remote desktop gateway (guacd) is unavailable" (520) | `GUACD_ADDR`, `docker compose ps guacd`, `/ready` |
| "remote server could not be found" (519) / 515 | target hostname/port from the guacd container, credentials, RDP `security` / `ignore-cert` |
| WebSocket closes immediately (403) | `APP_BASE_URL` / `APP_ALLOWED_ORIGINS` vs the browser origin; proxy must forward `Upgrade` |
| Logged out behind a proxy | `APP_COOKIE_SECURE`, `APP_TRUSTED_PROXIES`, `X-Forwarded-Proto` |
| `encrypted connection parameters exist but APP_SECRET_KEY is missing or wrong` | restore the original key |
