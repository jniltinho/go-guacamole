## ADDED Requirements

### Requirement: Recording policy
When `RECORDING_ENABLED=true`, every tunnel SHALL be recorded by guacd according to a global policy that overrides per-connection settings: a graphical screen recording (`recording-path`, `recording-name`, `create-recording-path`, optional `recording-include-keys`) for every protocol when `RECORDING_SCREEN=true`, and a text typescript (`typescript-path`, `typescript-name`, `create-typescript-path`) for SSH/Telnet when `RECORDING_TYPESCRIPT=true`. File names SHALL be generated server-side from the tunnel UUID and stored in the history row; stored connection parameters SHALL NOT be modified.

#### Scenario: SSH session recorded
- **WHEN** a user opens an SSH session with recording enabled, runs `echo GGREC-OK` and disconnects
- **THEN** the history row reports `hasRecording` and `hasTypescript`, and both files exist in the recordings directory

#### Scenario: Recording disabled
- **WHEN** `RECORDING_ENABLED=false`
- **THEN** no recording parameters are sent to guacd and history rows report no recordings

### Requirement: Transcript and playback
Users with `AUDIT` (or `ADMINISTER`) SHALL be able to read the SSH transcript through `GET /api/v1/history/{id}/typescript` (terminal control sequences removed; `?format=raw` downloads the original typescript, replayable with `scriptreplay`) and to play the screen recording in the browser through `GET /api/v1/history/{id}/recording` with `Guacamole.SessionRecording` (play, pause, seek, elapsed/total time). Every access SHALL be audited (`recording.view`). File names SHALL be validated against the generated pattern so no path outside the recordings directory can be served.

#### Scenario: Read the commands typed
- **WHEN** an auditor opens the transcript of that session
- **THEN** it contains the line `echo GGREC-OK` and its output

#### Scenario: Not an auditor
- **WHEN** a user without `AUDIT` requests a recording
- **THEN** the API returns 403

#### Scenario: Path traversal
- **WHEN** a history row references a name like `../../etc/passwd`
- **THEN** the API returns 404 and no file outside the directory is read

### Requirement: Shared storage and retention
guacd SHALL write recordings to `RECORDING_PATH`; GoGuacamole SHALL read them from the same directory (`RECORDING_LOCAL_PATH`, defaulting to the same path) and delete files older than `RECORDING_RETENTION_DAYS` daily. The compose stack SHALL share one volume between guacd (uid 1000, owner) and the app (member of gid 1000).

#### Scenario: Retention
- **WHEN** the daily job runs with retention 90 days
- **THEN** recordings whose modification time is older than 90 days are deleted
