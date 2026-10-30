# Proposal

## Why

Session auditing is all-or-nothing today: `RECORDING_ENABLED`, `RECORDING_SCREEN` and `RECORDING_TYPESCRIPT` apply the
same policy to every connection, change only with a restart, and retention is a single `RECORDING_RETENTION_DAYS`
applied by file age — the purge does not even run while recording is disabled, so old recordings stay forever, and
a failure to write a recording goes unnoticed while the session runs unaudited. Administrators and auditors need to
choose per host what is recorded (video, text, keystrokes), change it at any time, trust that "recorded" really
means recorded, and keep recordings exactly as long as policy and data-protection law (LGPD) require.

## What Changes

- **Global recording defaults in the admin UI** (database, sealed like the authentication settings): video, text
  (SSH typescript), keystrokes (only together with video), retention for video and for text (days or keep forever).
  `RECORDING_*` environment values become first-boot defaults; paths stay in configuration. Operational state
  (last purge, storage usage) is kept apart from the policy.
- **Per-connection policy** (`inherit | on | off` for video, text, keys; retention overrides), changed through a
  dedicated recording endpoint — never through the general connection update — by users with `AUDIT` or
  `ADMINISTER`; every change audited atomically with old and new values. Combinations guacd cannot honour (keys
  without video) are rejected.
- **No evasion paths**: recording parameters typed in connection parameters are ignored; changing the guacd
  endpoint of a connection (host, port, encryption) requires `ADMINISTER`; deleting a connection never shortens the
  retention of its recordings.
- **Fail closed**: when a session must be recorded and recording is unavailable, or its history entry cannot be
  written, the session does not start; an administrator can grant a time-limited, audited exception. After each
  session the expected files are verified and a missing recording is flagged and audited.
- **One snapshot per launch**: the effective policy is resolved when the launch ticket is issued and used for the
  user notice, guacd parameters, active sessions and history.
- **Transparency**: the remote client tells the user which recordings are active before and during the session;
  sessions and history show available / expired / held / failed recordings.
- **Retention**: history-driven purge (daily and on demand) of complete file sets (`.timing` and numbered files
  included), evaluated with the current policy, never touching held entries or running sessions, recording a
  per-entry result, previewed server-side before a retention is shortened, and running even when recording is off.
- **Legal hold** with a mandatory reason, visible only to auditors/administrators, safe against a concurrent purge.
- **Upgrade inventory**: connections that record today through their own parameters are migrated to explicit
  overrides and listed in a report, so no host silently stops being recorded.

## Capabilities

### New Capabilities
- `recording-policy`: global and per-connection control of video, text and keystroke recording, who may change it,
  failure handling, user notice, retention with legal hold and purge, storage visibility.

### Modified Capabilities
<!-- none in openspec/specs yet. The pending change add-goguacamole-panel (not archived) defines
     session-recording requirements "Recording policy" and "Shared storage and retention" that this change
     supersedes; see tasks "Spec consolidation". Playback, transcripts and path validation stay as defined there. -->

## Impact

- Database: connection policy/retention columns; history snapshot, file-set state, failure, hold and purge result;
  separate settings rows for recording policy and recording state.
- Backend: launch/tunnel (snapshot on the ticket, fail-closed, verification), connection API (dedicated recording
  endpoint, guacd endpoint restricted to admins), retention job, hold API, importer and migration inventory, audit
  writes in the same transaction as the change.
- Frontend: Administration → Recording page, connection Recording panel, client notice, sessions/history states,
  hold actions, retention preview dialog.
- Scope: one GoGuacamole instance and one shared recordings volume (as today); multi-replica coordination is out of
  scope and documented.
- Docs: configuration, operations (retention, hold, rollback), security and LGPD guidance for the data controller.
