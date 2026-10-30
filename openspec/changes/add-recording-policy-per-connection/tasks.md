# Tasks

## 1. Data model, settings and upgrade

- [x] 1.1 Migration (next free number; see design D1) with connection policy/retention columns and history snapshot, capture result, file state, file sets, purge and hold columns; down migration refuses while holds exist. Verify: migration up/down tests on SQLite and `make test-db` (MariaDB, PostgreSQL), including the refusal with a hold.
- [x] 1.2 Store: `recording.policy`, `recording.exceptions` and `recording.state` settings rows, one writer each; defaults derived from `RECORDING_*` when the policy row is absent. Verify: store tests prove purge/usage never create the policy row.
- [ ] 1.3 Store-level audit insert usable inside transactions; recording writes (policy, holds, exceptions) commit with their audit event. Verify: test that a failing audit insert rolls back the policy change.
- [x] 1.4 Upgrade inventory (design D8) run once after the SQL migration: overrides from `recording-path`/`typescript-path`/`recording-include-keys`, removal of recording parameters without equivalent, file sets of old history discovered on the volume (`present`/`missing`). Verify: tests with connections carrying those parameters with and without video, and history rows whose files exist or not.

## 2. Launch snapshot and fail-closed

- [ ] 2.1 Resolve the snapshot (required/captured per kind) at ticket issue, insert the history row before publishing the in-memory ticket, finish it as `not_started` on expiry, revocation or handshake failure, return the snapshot in the launch response; refuse the launch if the row cannot be stored. Verify: server tests (fake guacd) for video-only, text-only, none, keys with video, keys rejected without video, text skipped on RDP, expired ticket.
- [ ] 2.2 Tunnel builds guacd parameters from the ticket snapshot, stripping all hand-typed `recording-*`/`typescript-*` and adding generated ones only for captured kinds. Verify: test changes the policy between ticket and connect and asserts parameters and history follow the ticket.
- [x] 2.3 Availability check and fail-closed with time-limited admin exceptions (`RECORDING_UNAVAILABLE` 503, audit events, `captured=false` under an exception). Verify: tests with a read-only temp dir, with and without an exception, and after the exception expires.
- [ ] 2.4 In-session check (10 s after `ready`) that captured kinds' files exist, closing the tunnel with `RECORDING_FAILED` otherwise; post-session discovery of file sets per kind (`.timing`, numbered variants) and capture result. Verify: tests where the fake guacd writes nothing, writes only the typescript, and writes everything.
- [ ] 2.5 Lab check against real guacd 1.6: volume not writable by uid 1000 and volume full produce a refused launch or a closed session marked failed, never a silent unrecorded session. Verify: `deploy/e2e.sh` scenario.

## 3. Permissions and API

- [ ] 3.1 `PUT /connections/{id}/recording` (AUDIT or ADMINISTER, policy version → 409 on mismatch, 422 for explicit keys without video); the general update ignores recording fields. Verify: authorization tests for pure auditor, reader, owner-editor, admin; stale-copy test keeps the auditor's policy.
- [x] 3.2 guacd host/port/encryption and protocol changes require `ADMINISTER`; connection deletion freezes the effective retention into its history rows. Verify: tests 403 for an editor, 200 for an admin; retention raised then connection deleted keeps the raised value.
- [x] 3.3 Recording defaults, exceptions, purge-now and usage endpoints (`ADMINISTER`), retention 1–3650 or keep forever (422 otherwise). Verify: server tests including 403 for an auditor on defaults.
- [x] 3.4 Retention preview endpoint and confirmation token required to shorten a retention (global or per connection). Verify: test without token (rejected), with stale token (rejected), with valid token (applied).
- [x] 3.5 Hold place/release with reason (1–256 chars), conditional on file state (409 while purging/purged), reasons hidden from non-auditors in history responses. Verify: server tests for 422, 409, 403 and reason visibility.
- [ ] 3.6 Importer applies the D8 mapping on first import and `--conflict update`, listed in the report. Verify: importer tests on the MySQL and PostgreSQL fixtures.

## 4. Retention job

- [x] 4.1 History-driven purge (design D4, D6, D7): per-kind file sets of finished sessions whatever their capture result, current policy or frozen retention for deleted connections, keep forever as infinity, holds, running sessions, per-entry results (`purged`, `missing`, `error` retried), crash-safe `purging` state, purge mutex, runs with recording disabled, one summary audit event. Verify: server tests with a temp recordings dir covering every retention scenario of the spec.
- [ ] 4.2 Orphan purge only after a successful DB check, threshold longest finite global retention + 7 days, disabled when any global retention is keep forever. Verify: tests for orphan deleted, keep-forever, DB error.
- [ ] 4.3 Hold vs purge race. Verify: test running a purge and a hold concurrently on the same entry (`-race`); files remain when the hold was confirmed.
- [x] 4.4 Storage usage per run and on demand, cached in the state row with the last purge result. Verify: test with known file sizes.
- [x] 4.5 Audit retention keeps events of held entries while the hold lasts. Verify: test with an expired held entry.

## 5. User interface

- [x] 5.1 Administration → Recording page: defaults, retentions, availability, exceptions, usage, last purge, upgrade inventory, purge now; shortening shows the server preview and sends its token. Verify: typecheck, unit test for the form logic, manual check.
- [x] 5.2 Connection Recording panel using the dedicated endpoint; read-only for users without `AUDIT`/`ADMINISTER`; keys disabled when video is off; keystroke warning text. Verify: manual check as editor, auditor and admin.
- [x] 5.3 Client notice in the connecting overlay and persistent badge from the launch snapshot (including "not recorded (exception)"); recording-unavailable and recording-failed error screens. Verify: manual check on the lab SSH target for recorded, unrecorded, exception and refused launches.
- [x] 5.4 Sessions and history: per-kind states (available, expired, held, failed, exception), hold/release with reason. Verify: manual check; history panel in connection and user edit shows the same states.
- [x] 5.5 en/pt-BR strings. Verify: locale parity test passes.

## 6. Documentation and spec consolidation

- [x] 6.1 Docs: configuration (env vars as defaults, exceptions), operations (retention, preview, holds, rollback procedure, storage sizing, single-instance limit), security/LGPD (what the controller must define, keystroke risk, hold reason access, history metadata, backups). Verify: docs reviewed.
- [x] 6.2 Spec consolidation: update `add-goguacamole-panel`'s `session-recording` requirements "Recording policy" and "Shared storage and retention" to defer to `recording-policy` (keep playback, transcripts and path validation). Verify: `openspec validate --strict` passes for both changes.

## Workflow follow-up

- Archive `add-goguacamole-panel` first, then this change with `/opsx:archive`, so `recording-policy` becomes a
  baseline spec next to the consolidated `session-recording`.

## Verification gaps (open)

- 1.3: no test forces an audit insert failure to prove the rollback (code path: policy write and audit share one transaction).
- 2.1/2.2: expired-ticket and "policy changed between ticket and connect" are not covered by tests (behaviour verified by reading: the tunnel uses the ticket snapshot).
- 2.4: only the "guacd wrote nothing" case is tested; "only the typescript" and "everything" are not.
- 2.5: lab checked a read-only volume (launch refused with 503); a full volume was not tested.
- 3.1: stale-copy race between editor and auditor is not tested (the general update never writes the policy columns).
- 3.6: importer mapping runs after `goguacamole import` but has no fixture test.
- 4.2: keep-forever and DB-error orphan cases are not tested.
- 4.3: no explicit concurrent hold-vs-purge test; the conditional updates were tested sequentially and the suite passes with `-race`.
