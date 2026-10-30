# Spec Delta

## Purpose

Runs GoGuacamole's background work — scheduled upkeep and per-recording tasks such as uploads — reliably across one
or many instances: queued in Redis (embedded in the container by default), retried on failure, never overlapping,
recoverable from the database when Redis loses its data, visible to administrators, and still working in-process when
no Redis is configured.

## ADDED Requirements

### Requirement: Redis in the Docker deployments
The all-in-one image SHALL run Redis next to guacd and goguacamole under a process supervisor, listening only on
`127.0.0.1`, with `REDIS_ADDR` pointing to it by default; `REDIS_EMBEDDED=false` SHALL keep it from starting so an
external Redis can be used. The regular image SHALL keep running goguacamole alone as a non-root user that works with
all Linux capabilities dropped and a read-only root filesystem, and its Compose file SHALL provide a dedicated Redis
service.

#### Scenario: All-in-one container
- **WHEN** the all-in-one image starts with no Redis variables
- **THEN** the embedded Redis runs on 127.0.0.1, the queue is enabled and `/asynqmon` is available to administrators

#### Scenario: External Redis for the all-in-one
- **WHEN** the all-in-one image starts with `REDIS_EMBEDDED=false` and `REDIS_ADDR=redis.internal:6379`
- **THEN** no Redis process runs in the container and goguacamole uses the external Redis

#### Scenario: Regular Compose stack
- **WHEN** the repository's Compose stack starts with its hardening (`cap_drop: [ALL]`, read-only root)
- **THEN** goguacamole runs alone in its container and uses the Compose Redis service

### Requirement: Optional queue outside the container
Without a Redis address, goguacamole SHALL run sessions, recording and the scheduled upkeep as before, in-process;
features that need the queue (object storage uploads, the job dashboard) SHALL be reported as unavailable with the
reason instead of failing silently.

#### Scenario: No Redis configured
- **WHEN** the binary starts without `REDIS_ADDR`
- **THEN** sessions and recordings work, the retention purge still runs daily in-process, and the Storage settings
  show that object storage needs the job queue

#### Scenario: Redis unreachable
- **WHEN** `REDIS_ADDR` is set but Redis does not answer
- **THEN** goguacamole starts, reports the queue as down in the readiness details and in Administration, keeps
  retrying, and sessions are not affected

### Requirement: Dedicated Redis database
Goguacamole SHALL use its own Redis database (number configurable), name its queues with a `goguacamole:` prefix and
mark the database with its installation id. It SHALL refuse to start its workers and the dashboard when that database
holds any queue without its prefix or the mark of another installation, and SHALL say so in the log and in
Administration, so that tasks of other applications are never processed or managed.

#### Scenario: Redis shared with another application
- **WHEN** the configured Redis database contains asynq queues of another application
- **THEN** goguacamole reports the conflict, runs upkeep in-process, and does not serve `/asynqmon`

### Requirement: Database is the source of truth
Every piece of work whose loss would matter (an upload owed, a deletion pending) SHALL be recorded in the database in
the same transaction as the event that creates it, so that a restarted, emptied or replaced Redis delays work but
never loses it.

#### Scenario: Redis restarted without persistence
- **WHEN** the embedded Redis restarts empty while uploads were queued
- **THEN** within the next reconciliation period the pending uploads are queued again and complete once

### Requirement: Reconciliation
A reconciliation job SHALL periodically enqueue recorded work that has no valid claim, including work left in
progress by a crashed worker whose claim expired, and recording sets found on disk but not yet stored remotely while
uploads are enabled.

#### Scenario: Crash right after a session ends
- **WHEN** an instance crashes after ending a session but before recording its files
- **THEN** the next reconciliation finds the files on disk and the recording is uploaded

### Requirement: Payloads carry no secrets
Queued tasks SHALL carry only identifiers and non-sensitive parameters (history ids, set kinds, period labels).
Credentials, tokens, connection parameters and recording contents SHALL NOT be placed in task payloads, results or
error messages stored in the queue.

#### Scenario: Inspecting an upload task
- **WHEN** an administrator opens an upload task in the job dashboard
- **THEN** its payload shows the history id and set kind only, never a bucket credential

### Requirement: Periodic upkeep per UTC period
With the queue enabled, the daily retention purge (recordings and audit), the orphan sweep, the storage usage refresh,
the reconciliation and the expired web-session cleanup (every minute, as today) SHALL run as periodic jobs generated
for each UTC period. Periods missed while every instance was down SHALL run once at the next start.

#### Scenario: All instances down at midnight
- **WHEN** no instance runs during the daily purge period and one starts the next morning
- **THEN** that period's purge runs once after startup

### Requirement: Periodic upkeep without overlap
A run of a periodic job SHALL hold a database lease with an expiry and a token of its own, and SHALL first check,
under that lease, that its period is not already recorded as completed. A second run of the same job (another period,
another replica, a retry after the queue forgot the task, or a manual "purge now") SHALL wait, be refused or end
without effect, and an abandoned lease SHALL be recovered only after it expires.

#### Scenario: Three replicas
- **WHEN** three instances share one Redis and the daily purge period starts
- **THEN** one purge runs for that period and the audit log records one `recording.purge` event

#### Scenario: Manual purge during the scheduled one
- **WHEN** an administrator starts "purge now" while the scheduled purge holds the lease
- **THEN** the manual request is refused with a message that a purge is running, and nothing is deleted twice

### Requirement: Lost leases stop work
A run that can no longer renew its lease SHALL stop, and completing the period or releasing the lease SHALL succeed
only with the run's own token while the lease is valid.

#### Scenario: Old run resumes after its lease expired
- **WHEN** a paused purge run resumes after another run took over the expired lease
- **THEN** the old run cannot record the period as completed or release the new lease

### Requirement: Retries and failure records
Every job SHALL have bounded retries with exponential backoff and a timeout. When retries are exhausted, the failure
SHALL be recorded in the database on the item it concerns (for example the recording set's storage state and error),
independently of the queue's own archive, which is best-effort and limited in size. Jobs that cannot succeed by
retrying (for example a deleted history row) SHALL stop immediately and record why.

#### Scenario: Retries exhausted
- **WHEN** an upload fails on every attempt because the bucket denies writes
- **THEN** the recording set shows the failed state with the last error, and an administrator can retry it after
  fixing the permissions even if the queue archive was trimmed

### Requirement: Graceful shutdown
On shutdown, an instance SHALL stop taking new jobs, let running jobs finish within a bounded grace period, release
its leases, and leave unfinished work recorded so another instance (or the same one after restart) completes it.

#### Scenario: Restart during an upload
- **WHEN** an instance is stopped while uploading a recording set
- **THEN** the upload is completed later and the set ends in the stored state exactly once

### Requirement: Job dashboard for administrators
When the queue is enabled, GoGuacamole SHALL serve a job dashboard at `/asynqmon`. Only signed-in users with
`ADMINISTER` SHALL reach it; others SHALL be redirected to sign in or receive 403. Its state-changing actions SHALL be
accepted only from the same origin, and every such action SHALL be written to the audit log.

#### Scenario: Auditor opens the dashboard
- **WHEN** a user with `AUDIT` but not `ADMINISTER` opens `/asynqmon`
- **THEN** the request is refused with 403

#### Scenario: Cross-site action
- **WHEN** a page on another origin posts a delete-task request to `/asynqmon` with the administrator's cookie
- **THEN** the request is rejected and no task changes

#### Scenario: Not signed in
- **WHEN** someone without a session opens `/asynqmon`
- **THEN** they are redirected to the sign-in page and come back to the dashboard after signing in
