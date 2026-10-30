# Design

## Context

- `App.launch` issues a single-use ticket; the tunnel handler later re-reads the connection and calls
  `recordingParams` (`internal/server/tunnel.go`), which, when recording is enabled, overwrites `recording-*` /
  `typescript-*` with the configured path and the tunnel id as name, and otherwise returns the connection's own
  parameters untouched (so hand-typed `recording-path` records today). `StartHistory` failures are only logged.
- `purgeRecordings` (`recordings.go`) runs from the janitor, returns early when recording is disabled, deletes by
  mtime, and `recordingName` does not accept the `.timing` companion or guacd's numbered variants.
- `connectionInput.apply` updates guacd host/port/encryption for any connection editor; `CanEditConnection` is
  admin or creator with `CREATE_CONNECTION`. Connection deletion sets `connection_history.connection_id` to NULL.
- `auditBy` swallows errors. Settings use one sealed JSON row per name (`PutSetting` replaces the whole value).
- guacd 1.6 may log a recording error and keep the session running; `recording-include-keys` only has effect with a
  graphical recording; a typescript is two files (`NAME`, `NAME.timing`).
- The connection API accepts `ssh`, `rdp`, `vnc` (Telnet is not editable today).

## Goals / Non-Goals

**Goals:**
- One immutable policy snapshot per launch, used everywhere.
- No path for a non-auditor to reduce, bypass or delete auditing.
- Retention computed from data with holds, safe against concurrent holds and restarts.

**Non-Goals:**
- Changing recording of a running session; storage quotas; external/immutable storage; signed recordings; per-user
  policy; Telnet support (text recording is SSH-only here).
- Multiple GoGuacamole replicas or several recordings volumes (documented as unsupported for this feature).
- A consent step before recorded sessions (the notice is informational; the legal basis is the controller's).

## Decisions

### D1. Data model (migration `0005_recording_policy`)
- `connections`: `rec_video`, `rec_text`, `rec_keys` (`inherit|on|off`, NULL = inherit), `retention_video_days`,
  `retention_text_days` (NULL = inherit, `-1` = keep forever).
- `connection_history`: snapshot `rec_video`, `rec_text`, `rec_keys` (NULL = unknown for pre-upgrade rows),
  `retention_video_days`, `retention_text_days` (frozen when the connection is deleted), per kind a capture result
  (`none|ok|failed|exception|not_started`) and a file state (`present|purging|purged|missing|error`), `*_purged_at`,
  `hold_reason`, `hold_by`, `hold_at`, and the discovered file sets per kind (JSON).
- Settings rows: `recording.policy` (defaults; absent ⇒ derived from env), `recording.exceptions` (admin only) and
  `recording.state` (last purge, usage cache, inventory; jobs only) — one writer each.
- Policy columns are separate from connection parameters, which stay user-editable guacd input.
Numbering: the next free migration number at implementation time; coordinated with `refine-themes-premium`, which
also adds a migration.

### D2. Snapshot on the ticket
`launch` resolves per kind `required = override ?? default` (keys only with video; text only for ssh), decides
`captured` (= required, or false under an exception, D5), inserts the history row with the snapshot (`not_started`)
**before** publishing the ticket in the in-memory registry (tickets are not in the database), and returns the
snapshot. If the insert fails the launch is refused; if publishing fails the row is finished as `not_started`.
Ticket expiry, revocation or a failed handshake finish the row as `not_started`. The tunnel handler builds guacd
parameters from the ticket's snapshot: all `recording-*`/`typescript-*` keys of the connection are removed and
generated ones added only for captured kinds.

### D3. Dedicated recording operation and permission matrix
| Operation | Permission |
|---|---|
| Global defaults, exceptions, purge now, usage | `ADMINISTER` |
| Per-connection policy (`PUT /connections/{id}/recording`) | `AUDIT` or `ADMINISTER` (no edit right needed) |
| Hold place/release, see hold reasons | `AUDIT` or `ADMINISTER` |
| guacd host/port/encryption in connection update | `ADMINISTER` |
| Read policy and recording states | anyone who can read the connection/history entry |
The general update ignores recording fields entirely (fixes stale-copy overwrites). The recording operation
carries the policy version it was based on; a mismatch answers 409. Each write and its audit event share one
transaction (a store-level audit insert), so failures roll back both.

### D4. Retention semantics
Effective retention per kind = current connection override ?? current global default when the connection exists;
on connection deletion the then-effective retention is copied into its history rows, which are used afterwards (so
deleting or recreating a connection never shortens it). `-1` is
infinity in every comparison. Shortening goes through `POST .../retention-preview` returning `{recordings, bytes,
token}`; the update must include the token, which is bound to the preview's inputs and expires after 10 minutes.

### D5. Availability and fail-closed
Availability = directory configured, exists and writable by the app (checked at startup and every minute). If a
required kind is unavailable, launch answers 503 `RECORDING_UNAVAILABLE` with an audit event, unless an active
exception exists (global or per connection, max 24 h, reason, `ADMINISTER`, stored in its own settings row
`recording.exceptions` written only by admins), in which case `captured=false` and the notice says "not recorded
(exception)". guacd 1.6 may keep a session running when it cannot create the file, so after `ready` the tunnel
checks within 10 s that each captured kind's file exists (and grows for video); otherwise it closes the tunnel with
`RECORDING_FAILED`, sets capture `failed` and audits `recording.failed`. After the tunnel closes the sets are checked
again (missing or empty ⇒ capture `failed`).

### D6. File sets and names
`recordingName` accepts the generated base name and guacd suffixes (`.N`, `.timing`). The history row stores the
actual file set discovered after the session (covering numbered variants). Usage, holds and deletion operate on the
set.

### D7. Purge and hold concurrency
Single process: a purge mutex serialises janitor and manual runs. Per entry and kind, for finished sessions whatever their capture
result: in a transaction re-check hold and file state, set `purging` (conditional update `WHERE hold_at IS NULL AND
file_state IN ('present','error')`); then delete the set; then set `purged`, `missing` or `error` (retried next run)
with timestamps. The hold API sets the hold with a conditional update that
fails when state is `purging` or `purged` (409). A crash leaves `purging` rows, retried on the next run. Orphans: a
set is orphan only when the DB query succeeded and returned no row; age measured from the newest file mtime.

### D8. Upgrade inventory and import
The migration step (Go, after SQL) scans connections: `recording-path` ⇒ `rec_video=on`, `typescript-path` ⇒
`rec_text=on` (SSH), `recording-include-keys=true` ⇒ `rec_keys=on` only when video is on; recording parameters with no equivalent (`recording-exclude-*`, `recording-name`, `typescript-name`…)
are removed from the connection and listed. The inventory is stored in `recording.state` and logged. Pre-upgrade
history rows get snapshot NULL (unknown) and retention NULL (current policy applies); their file sets are discovered
on the volume: found ⇒ `present`, absent ⇒ `missing`. The importer applies the same mapping on first import and on `--conflict update`, and
reports it.

### D9. Transparency UI
Connecting overlay line and a persistent compact badge ("● REC video · keys") in the client chrome; sessions and
history show per-kind state icons; hold reason shown only to auditors/admins.

## Risks / Trade-offs

- [Fail-closed blocks access during storage incidents] → time-limited admin exception, audited; clear error to
  users; availability shown on the settings page.
- [guacd may start a session it cannot record] → the in-session check closes it within ~10 s, flags it `failed` and
  audits it; up to those seconds of activity can be unrecorded (documented); operations doc on alerting.
- [Retroactive shortening deletes evidence] → server preview + confirmation token, holds, per-entry purge results,
  audit.
- [Rollback to a build without holds would purge by mtime] → down migration refuses while holds exist; documented
  rollback settings (Migration Plan step 4).
- [History metadata (user, IP, times) has no retention of its own] → out of scope; documented for the controller.
- [LGPD] → keystroke capture flagged in the UI as able to capture typed secrets; docs list what the controller must
  define (purpose, legal basis, notice, retention review, backup/restore handling); hold reasons restricted.
- [Replicas] → unsupported: `CloseDanglingHistory` and the purge assume one instance; documented.

## Migration Plan

1. SQL migration adds columns (NULL ⇒ inherit / unknown) and settings rows are not created.
2. Go step runs the inventory (D8) once and records it.
3. Behaviour: connections without overrides follow `RECORDING_*` defaults as before; connections that recorded via
   their own parameters keep recording through explicit overrides.
4. Rollback: only when no hold exists (the down migration checks and refuses otherwise). Older builds purge by file
   age unless recording is disabled, so start them with `RECORDING_RETENTION_DAYS=3650` (files younger than ten
   years are kept) or `RECORDING_ENABLED=false` (no purge, no recording); documented in operations.
