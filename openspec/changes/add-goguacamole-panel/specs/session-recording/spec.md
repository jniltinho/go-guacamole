## ADDED Requirements

### Requirement: Recording policy
Which sessions are recorded (screen video for every protocol, SSH text transcript, keystrokes) SHALL be decided by
the recording policy defined in the `recording-policy` capability (global defaults seeded by `RECORDING_*`,
per-connection overrides, one snapshot per launch). File names SHALL be generated server-side from the tunnel UUID
and stored in the history row; recording parameters stored on connections SHALL NOT reach guacd.

#### Scenario: SSH session recorded
- **WHEN** a user opens an SSH session whose policy records video and text, runs `echo GGREC-OK` and disconnects
- **THEN** the history row reports `hasRecording` and `hasTypescript`, and both files exist in the recordings directory

#### Scenario: Recording disabled
- **WHEN** the effective policy records nothing
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
guacd SHALL write recordings to `RECORDING_PATH`; GoGuacamole SHALL read them from the same directory
(`RECORDING_LOCAL_PATH`, defaulting to the same path). Retention, purge and legal hold SHALL follow the
`recording-policy` capability. The compose stack SHALL share one volume between guacd (uid 1000, owner) and the app
(member of gid 1000).

#### Scenario: Shared volume
- **WHEN** the compose stack starts
- **THEN** guacd can write and GoGuacamole can read and delete files in the shared recordings volume
