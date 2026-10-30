# Spec Delta

## Purpose

Controls which sessions GoGuacamole records — screen video, SSH text transcript and keystrokes — globally and per
connection, who may change that policy, what happens when recording cannot be guaranteed, how users are informed,
and how long recordings are kept, including legal hold and automatic purge.

## ADDED Requirements

### Requirement: Global recording defaults
Administrators (`ADMINISTER`) SHALL be able to edit, in the web UI, the default policy for inheriting connections:
video on/off, text on/off, keystrokes on/off, video retention and text retention (1 to 3650 days, or keep forever).
The defaults SHALL be stored in the database and apply to launches after saving, without restart.

#### Scenario: Change without restart
- **WHEN** an administrator turns text recording on in the recording settings
- **THEN** the next SSH session launched on an inheriting connection produces a transcript

#### Scenario: Invalid retention
- **WHEN** the settings are saved with video retention of 0 days without choosing keep forever
- **THEN** the API answers 422 for that field and nothing is changed

#### Scenario: Auditor cannot change defaults
- **WHEN** a user with `AUDIT` but not `ADMINISTER` saves the recording defaults
- **THEN** the API answers 403

### Requirement: Defaults from configuration
Until an administrator saves the recording defaults, they SHALL be derived from the `RECORDING_*` configuration.
Background jobs (purge, usage) SHALL NOT create or overwrite the saved defaults.

#### Scenario: Upgrade keeps defaults
- **WHEN** a deployment with `RECORDING_ENABLED=true`, `RECORDING_SCREEN=true`, `RECORDING_TYPESCRIPT=false`
  upgrades and nobody saved the recording settings
- **THEN** new sessions on connections without overrides record video and not text

#### Scenario: Jobs do not freeze defaults
- **WHEN** the purge and usage jobs run before any administrator saved the recording settings
- **THEN** the defaults still follow `RECORDING_*`

### Requirement: Upgrade inventory of existing recording parameters
On upgrade, connections whose own parameters enable recording (`recording-path`, `typescript-path`,
`recording-include-keys`) SHALL receive the equivalent explicit overrides, recording parameters without an
equivalent SHALL be removed, and an inventory SHALL list every connection changed this way. Imports from Apache
Guacamole SHALL apply the same mapping and report it.

#### Scenario: Host recorded through its own parameters
- **WHEN** a connection has `recording-path` set in its parameters and recording is globally disabled
- **THEN** after the upgrade it has video `on`, is listed in the inventory and keeps being recorded

#### Scenario: Import report
- **WHEN** an Apache Guacamole connection with `typescript-path` and `recording-include-keys=true` but no
  `recording-path` is imported
- **THEN** it gets text `on`, keys stay off (no video) and the import report states both facts

### Requirement: Per-connection recording policy
Each connection SHALL have video, text and keystroke settings (`inherit`, `on` or `off`) and optional video and text
retention overrides. Text recording SHALL apply only to SSH connections. Keystroke capture exists only inside a video
recording: an inherited keystroke default applies only when the session records video, and an explicit keystroke
`on` whose effective video is off SHALL be rejected with 422. Notices and history SHALL show effective values.

#### Scenario: Video only on one host
- **WHEN** a connection has video `on` and text `off` while the defaults are both on
- **THEN** a session on it produces a screen recording and no transcript, and its history entry says so

#### Scenario: Keys without video
- **WHEN** a policy with video `off` and keys `on` is submitted
- **THEN** the API answers 422 and the stored policy is unchanged

### Requirement: Policy snapshot per launch
The effective policy SHALL be resolved when the launch ticket is issued and that single snapshot SHALL be used for the
user notice, the guacd parameters, the active session list and the history entry. Policy changes SHALL NOT affect
tickets already issued or sessions already running.

#### Scenario: Change between ticket and connect
- **WHEN** a ticket is issued with video on and an auditor switches video off before the tunnel opens
- **THEN** that session is still recorded and its notice, parameters and history all say video on

#### Scenario: Ticket never used
- **WHEN** a ticket expires or its tunnel handshake fails
- **THEN** its history entry is finished as not started and no recording is expected for it

#### Scenario: Disable on a host at any time
- **WHEN** an auditor sets video `off` on a connection while a session on it is running
- **THEN** the running session keeps recording until it ends and the next launch is not recorded

### Requirement: Policy overrides connection parameters
Recording-related guacd parameters typed in a connection's own parameters SHALL be ignored: the snapshot alone
decides what guacd records and where.

#### Scenario: Hand-typed parameters ignored
- **WHEN** a connection's parameters contain `recording-path=/tmp/x` and its effective video is `off`
- **THEN** guacd receives no recording parameters for that session

### Requirement: Recording permissions
Per-connection policy and legal holds SHALL be changed only by users with `AUDIT` or `ADMINISTER`, through an
operation dedicated to recording that grants no other connection edit. The general connection update SHALL never
change recording policy. Changing a connection's guacd endpoint (host, port, encryption) or its protocol SHALL require
`ADMINISTER`. Hold reasons SHALL be visible only to `AUDIT` and `ADMINISTER`.

#### Scenario: Editor cannot change recording
- **WHEN** a connection editor without `AUDIT` calls the recording operation for that connection
- **THEN** the API answers 403 and the policy is unchanged

#### Scenario: Concurrent edits keep the policy
- **WHEN** an editor saves a stale copy of a connection after an auditor changed its recording policy
- **THEN** the recording policy set by the auditor is kept

#### Scenario: Auditor without edit rights
- **WHEN** a user with `AUDIT` and no edit right on a connection changes its recording policy
- **THEN** the change succeeds and the user still cannot change the connection's name or parameters

#### Scenario: guacd endpoint
- **WHEN** a connection editor without `ADMINISTER` sets a connection's guacd host
- **THEN** the API answers 403

#### Scenario: Protocol change
- **WHEN** a connection editor without `ADMINISTER` changes an SSH connection to RDP
- **THEN** the API answers 403 and the connection keeps recording text

### Requirement: Atomic audit of policy changes
Every change to recording defaults, per-connection policy, holds and failure exceptions SHALL be stored together
with its audit event (actor, target, old and new values) so that either both exist or neither does.

#### Scenario: Audit trail
- **WHEN** an auditor changes text recording of a connection from `inherit` to `off`
- **THEN** the audit log shows `connection.recording.update` with `text: inherit → off`

#### Scenario: Audit write fails
- **WHEN** the audit event cannot be stored
- **THEN** the policy change is rejected and the previous policy remains

### Requirement: Fail closed when recording cannot be guaranteed
A launch whose snapshot requires any recording SHALL be refused when the recordings directory is not configured or
not writable; the user SHALL see that recording is unavailable and an audit event SHALL be written. Every launch
SHALL be refused when its history entry cannot be stored.

#### Scenario: Volume read-only
- **WHEN** the recordings directory is read-only and a user launches a connection with video on
- **THEN** the launch is refused with a recording-unavailable error and an audit event is written

### Requirement: Recording exceptions
Administrators SHALL be able to grant a time-limited exception (global or per connection, with reason, audited)
during which sessions that require recording start without capture; their snapshot SHALL record "required but not
captured (exception)" and the notice SHALL say so. An exception SHALL NOT waive the history entry.

#### Scenario: Exception
- **WHEN** an administrator grants a one-hour exception with a reason and the volume is still read-only
- **THEN** sessions start, their notice and history say "not recorded (exception)" and the exception is audited

### Requirement: Recording verification
Shortly after a recorded session connects, the expected recording files SHALL be checked; if a required file is
missing, the session SHALL be closed with a recording-failed error, the history entry marked as capture failed and an
audit event written. After the session ends, the files SHALL be checked again and a missing or empty file SHALL mark
the entry as capture failed.

#### Scenario: guacd could not write
- **WHEN** guacd connects a recorded session but cannot create its screen recording file
- **THEN** the session is closed within seconds with a recording-failed error, the history entry shows capture
  failed and the audit log has `recording.failed`

### Requirement: Recording transparency
Before a recorded session connects and while it runs, the remote client SHALL tell the user which recordings are
active (video, text, keystrokes). Sessions and history SHALL show, per entry, whether each recording is available,
expired, held or failed.

#### Scenario: User is told
- **WHEN** a user opens a connection whose snapshot records video and keystrokes
- **THEN** the client shows a recording notice naming video and keystrokes for the whole session

#### Scenario: Unrecorded session
- **WHEN** the snapshot records nothing
- **THEN** no recording notice is shown

### Requirement: Recording file sets
Each kind of a session SHALL be handled as its complete file set: the video set (the screen recording and numbered
variants guacd creates) and the text set (the typescript, its `.timing` companion and their numbered variants).
Retention, usage and deletion SHALL apply per set; a hold SHALL protect both sets of the entry; partial deletion
SHALL be reported as an error and retried, not as purged.

#### Scenario: Timing file follows transcript
- **WHEN** a transcript expires
- **THEN** both `NAME.typescript` and `NAME.typescript.timing` are deleted and the text set is marked purged

### Requirement: Retention by current policy
Each file set of a finished session SHALL be deleted, whatever its capture result, once the session ended longer ago
than its effective retention (the connection's current override or global default; keep forever never expires).
Shortening a retention applies to existing recordings; lengthening extends them. Sets of held or still-running
sessions SHALL NOT be deleted.

#### Scenario: Shorter retention applies to old recordings
- **WHEN** video retention of a connection changes from 90 to 30 days and the purge runs
- **THEN** its video recordings of sessions that ended more than 30 days ago are deleted and marked purged

### Requirement: Retention after connection deletion
When a connection is deleted, the retention then in effect SHALL be frozen into its history entries and used from
then on, so deleting or recreating a connection never shortens the retention of its recordings.

#### Scenario: Deleting the connection does not shorten retention
- **WHEN** a connection's text retention is raised from 30 days to keep forever, then the connection is deleted, and
  the global text retention is 30 days
- **THEN** its transcripts are not deleted by the purge

### Requirement: Retention change preview
Before a retention is shortened (global or per connection), the server SHALL report how many recordings and bytes
the next purge would delete, and the change SHALL only be applied when the request confirms that preview.

#### Scenario: Confirm deletion count
- **WHEN** an auditor shortens a connection's video retention and the preview reports 120 recordings
- **THEN** the change is applied only with the confirmation of that preview, and rejected without it

### Requirement: Purge job
A purge SHALL run at least daily and on demand by an administrator, whether or not recording is enabled. It SHALL
keep history entries and record per entry and per kind whether the set was deleted, already missing or failed, and
write one summary audit event per run.

#### Scenario: Purge while recording is off
- **WHEN** recording is disabled and recordings older than their retention exist
- **THEN** the purge still deletes them

#### Scenario: File already missing
- **WHEN** an expired recording's files no longer exist
- **THEN** the entry is marked as missing, not as purged by the job

### Requirement: Orphan recordings
Recording sets matching the generated name pattern whose absence from history is confirmed by the database SHALL be
deleted when older than the longest finite global retention plus seven days; if any global retention is keep
forever, orphans SHALL NOT be deleted. Files not matching the pattern SHALL never be touched, and a database error
SHALL stop orphan deletion for that run.

#### Scenario: Orphan file
- **WHEN** a file named like a recording has no history entry and is older than the longest retention plus seven days
- **THEN** the purge deletes it

#### Scenario: Keep forever disables orphan purge
- **WHEN** the global video retention is keep forever
- **THEN** no orphan is deleted

### Requirement: Legal hold
Users with `AUDIT` or `ADMINISTER` SHALL be able to place a hold on a history entry with a mandatory reason and
release it. A confirmed hold SHALL prevent deletion even when a purge runs at the same time; a hold requested for a
set already being deleted SHALL fail explicitly. Audit events of a held entry (its session, the hold and policy
changes of its connection) SHALL be kept by the audit retention job while the hold lasts.

#### Scenario: Held recording survives
- **WHEN** an auditor holds a session with reason "incident 2026-114" and its retention expires
- **THEN** the purge keeps its files and the history shows the hold

#### Scenario: Hold races the purge
- **WHEN** a hold is confirmed while a purge is running over the same entry
- **THEN** the files still exist after the purge finishes

#### Scenario: Hold without reason
- **WHEN** a hold is requested with an empty reason
- **THEN** the API answers 422

### Requirement: Storage visibility
The recording settings page SHALL show space used in total, by kind and for the connections using the most space,
the time and result of the last purge, and the inventory from the upgrade.

#### Scenario: Usage shown
- **WHEN** an administrator opens the recording settings
- **THEN** the page lists total, video and text usage, the top connections and the last purge result
