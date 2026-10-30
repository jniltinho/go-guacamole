# Spec Delta

## Purpose

Defines where session recordings are kept — the local recordings volume and, optionally, an object store (AWS S3,
S3-compatible services, Oracle Cloud Object Storage, Google Cloud Storage) — how each recording set (video;
transcript with timing) gets there asynchronously and verified, how it is played back from either place, and how
retention and legal hold reach the remote copies.

## ADDED Requirements

### Requirement: Storage targets
Administrators (`ADMINISTER`) SHALL be able to choose, in Administration → Recording → Storage, either *local only*
(default) or *local + object storage* with one provider: AWS S3, S3-compatible (custom endpoint, optional path-style
addressing; includes OCI's S3 compatibility API), Oracle Cloud Object Storage native API with instance or workload
principal authentication, or Google Cloud Storage. The settings SHALL be stored in the database and apply without
restart.

#### Scenario: Configure an S3-compatible bucket
- **WHEN** an administrator selects S3-compatible, sets endpoint, bucket, prefix, access key and secret, and saves
- **THEN** recording sets of sessions that end afterwards are uploaded to that bucket under the prefix

#### Scenario: Object storage without the job queue
- **WHEN** the job queue is not available
- **THEN** the object storage options are shown as unavailable with the reason, and enabling uploads is refused

### Requirement: Storage settings fields
The settings SHALL include bucket, key prefix, region, endpoint (S3-compatible), namespace (OCI), the provider's
credentials, optional server-side encryption (provider-managed or a KMS key id), whether uploads are enabled, and the
local copy policy.

#### Scenario: Server-side encryption with a KMS key
- **WHEN** an administrator sets a KMS key id for an AWS S3 target and saves
- **THEN** uploaded objects are encrypted with that key by the provider

#### Scenario: Auditor cannot change storage
- **WHEN** a user with `AUDIT` but not `ADMINISTER` saves the storage settings
- **THEN** the API answers 403 and nothing changes

### Requirement: Targets are versioned
Saving changes to provider, endpoint, region, namespace, bucket or prefix SHALL create a new target version used by
new uploads. Every stored recording set SHALL reference the target version that holds it, and a target version SHALL
stay readable (its configuration and credentials kept) while any set references it or a deletion on it is pending.

#### Scenario: Moving to a new bucket
- **WHEN** an administrator changes the bucket while older recordings are stored remotely
- **THEN** new uploads go to the new bucket and the older recordings still play from the previous one

### Requirement: Credentials are write-only and sealed
Storage credentials SHALL be encrypted at rest with the application secret, SHALL never be returned by any API (only
whether they are set), and SHALL never appear in logs (including the storage library's own logs), audit metadata,
job payloads, stored errors or test results. Changing the settings SHALL be audited with the non-secret fields before
and after.

#### Scenario: Reading the settings back
- **WHEN** an administrator opens the Storage settings after saving a secret key
- **THEN** the form shows the key as set, without its value, and the API response contains no secret

#### Scenario: Provider echoes the request
- **WHEN** an endpoint answers an error that contains the signed request
- **THEN** the stored error and the log show it with the signature and the credentials removed

### Requirement: Endpoint safety
Object storage connections SHALL use HTTPS. Endpoints whose addresses resolve, at connection time, to loopback,
link-local (including cloud metadata addresses), private or unique-local networks SHALL be refused unless the operator
allows them in the server configuration (not in the web UI); plain HTTP SHALL likewise need operator configuration.
Redirects SHALL NOT be followed and proxy settings from the environment SHALL NOT be used for object storage. The same
rules SHALL apply to *Test*.

#### Scenario: Metadata address
- **WHEN** an administrator sets an endpoint that resolves to 169.254.169.254 and runs *Test*
- **THEN** the connection is refused with an endpoint-not-allowed error and no request is sent

#### Scenario: Private MinIO allowed by the operator
- **WHEN** the operator lists `minio.lab.internal` as an allowed endpoint and an administrator uses it
- **THEN** uploads and *Test* reach that MinIO over HTTPS

### Requirement: Connection test
The Storage settings SHALL offer a *Test* that, with the values in the form, writes a probe object under the prefix,
reads it back, compares its SHA-256, deletes it, and reports success or the sanitized error, plus whether the bucket
has versioning or object lock. The test SHALL NOT change the saved settings.

#### Scenario: Wrong secret
- **WHEN** an administrator tests with an invalid secret key
- **THEN** the result shows the provider's authentication error and the saved settings are unchanged

#### Scenario: Versioned bucket
- **WHEN** the bucket has versioning enabled
- **THEN** the test result says so, and the probe's exact version is the one deleted

### Requirement: Upload owed when the recording is closed
When uploads are enabled, ending a recorded session SHALL record, in the same transaction, an upload owed for each
present recording set (video; transcript with timing). An upload SHALL start only after the session ended (by its
owning instance, or by liveness expiry when the owner disappeared) and every file of the set — numbered parts and the
timing file included — has stopped changing for a quiet period.

#### Scenario: Normal end of session
- **WHEN** an SSH session with video and text recording ends
- **THEN** two uploads are owed (video, transcript), and both start after guacd closed the files

#### Scenario: Another replica restarts during the session
- **WHEN** replica B restarts while replica A still serves a recorded session
- **THEN** B does not end A's session and no upload starts until A ends it

### Requirement: Verified upload
An upload job SHALL copy every file of the set to the target version in use, read each object back and compare its
SHA-256 with the local file, record the object keys, versions (when the bucket is versioned) and digests, and only
then mark the set as stored remotely. A provider-reported checksum SHALL replace the read-back only when the provider
itself computed it over the stored bytes.

#### Scenario: Corrupted transfer
- **WHEN** an object read back does not match the local SHA-256
- **THEN** the job fails, the local files are kept, and the upload is retried

#### Scenario: Bucket down for a day
- **WHEN** the provider is unreachable for longer than the retry window
- **THEN** the set stays local in the failed state, sessions keep working, and a retry after recovery uploads it

### Requirement: Uploads never affect sessions
Uploading SHALL never block, delay or end a session. Launch decisions under the recording policy SHALL stay as before:
fail-closed checks the local recordings volume, not the object store.

#### Scenario: Slow bucket
- **WHEN** uploads are slow or failing
- **THEN** new sessions launch and are recorded locally as usual

### Requirement: Upload and purge never race
A recording set SHALL be uploaded, locally removed or purged only by the holder of a valid claim on that set. An
upload SHALL NOT start for a set being purged or already purged, and an upload that finishes after its set was purged
SHALL delete what it wrote.

#### Scenario: Retention ends during an upload
- **WHEN** the purge reaches a set while its upload is running
- **THEN** the purge waits for the claim, and the set ends deleted both locally and remotely

### Requirement: Stale workers cannot write
Every state change on a set SHALL be accepted only from the current holder of a claim that has not expired, and a
worker that loses its claim SHALL stop. Remote effects SHALL be idempotent (same keys), and versions of a set's keys
other than the verified one SHALL be removed by the next reconciliation.

#### Scenario: Worker paused past its claim
- **WHEN** an upload worker resumes after its claim expired
- **THEN** its state update is rejected, and any version it wrote is removed by the next reconciliation

### Requirement: Legal hold waits for claims
Placing a legal hold SHALL be refused while any claim (upload, local removal, purge) is valid on the recording, and
SHALL be accepted once it is released.

#### Scenario: Hold during local removal
- **WHEN** an auditor places a hold while the local copy policy is removing the recording's files
- **THEN** the hold is refused with a retry message, and succeeds once the removal ends

### Requirement: Object keys
Objects SHALL be written under `<prefix>/<installation-id>/<yyyy>/<mm>/<dd>/<history-id>/` with the recording and
transcript file names, using the session start date in UTC; the installation id is generated once per database.
Keys SHALL contain no user-controlled text other than ids. Re-uploading the same set SHALL write the same keys, and
after a verified upload any other version of those keys left by earlier attempts SHALL be deleted.

#### Scenario: Key layout
- **WHEN** history row 4812 started on 2026-10-30 is uploaded with prefix `goguacamole/recordings` by installation
  `a1b2c3`
- **THEN** its objects are under `goguacamole/recordings/a1b2c3/2026/10/30/4812/`

#### Scenario: Crash between upload and bookkeeping
- **WHEN** a worker uploaded a set but crashed before recording it, and a retry uploads it again
- **THEN** after the retry is verified only one version of each key remains

### Requirement: Local copy policy
Administrators SHALL choose whether, after a verified upload, local files are kept (until retention) or removed.
Local files SHALL never be removed by this policy before a verified upload, while the session is active, while the
recording is under legal hold, before a safety window (1 hour by default) has passed since the session ended, or when
their current SHA-256 differs from the uploaded one (they are uploaded again instead).

#### Scenario: Remove after upload
- **WHEN** the policy is *remove local after upload* and a set is verified in the bucket
- **THEN** its local files are deleted and the recording still plays from the bucket

#### Scenario: Held recording
- **WHEN** a recording under legal hold is uploaded with *remove local after upload*
- **THEN** its local files are kept

### Requirement: Per-set storage state
Each recording set SHALL expose its storage state — local, upload owed, uploading, stored remotely (with or without a
local copy), failed (with the sanitized error), deletion pending, deleted — to users with `AUDIT`, in the history and
Sessions views, independently for video and transcript. Administrators SHALL be able to retry a failed upload.

#### Scenario: Different retentions
- **WHEN** a recording's transcript retention ended but its video retention did not
- **THEN** the transcript shows deleted and the video still shows stored remotely and plays

### Requirement: Playback from either place
Watching a screen recording and reading or downloading an SSH transcript SHALL work whether the set is local or only
in the bucket, with the same permission checks, audit events and `no-store` caching. Remote files SHALL be streamed
through GoGuacamole; no public or pre-signed URL SHALL reach the browser.

#### Scenario: Playback after local removal
- **WHEN** an auditor plays a recording whose local file was removed after upload
- **THEN** the player streams it from the bucket and the audit log records `recording.view`

### Requirement: Retention over remote copies
The retention purge SHALL delete a set's remote objects together with its local files when that set's retention ends,
following the recording policy's rule (the connection's current retention; the retention frozen on the row when its
connection was deleted). Expiry deletes the local files even when the set was never uploaded. A recording under legal
hold SHALL NOT be deleted locally or remotely.

#### Scenario: Retention ends
- **WHEN** a set's retention ends and the recording is not on hold
- **THEN** both its local files and its remote objects are deleted and the purge audit event counts the remote
  deletions

#### Scenario: Legal hold
- **WHEN** a recording under legal hold reaches the end of its retention
- **THEN** its remote objects and its local files remain

### Requirement: Exact deletion and immutability
Remote deletion SHALL remove every version of the set's keys, and SHALL count as done only after the provider
confirms no version of those keys remains. A refused deletion (object lock, retention policy, permissions)
SHALL leave the set in deletion pending, be retried by later purges and stay reported; GoGuacamole SHALL NOT try to
bypass a lock.

#### Scenario: Versioned bucket
- **WHEN** the purge deletes a set stored in a versioned bucket
- **THEN** the uploaded versions themselves are removed, not hidden behind delete markers

#### Scenario: Object lock longer than retention
- **WHEN** the bucket's object lock still protects a set whose retention ended
- **THEN** the set stays deletion pending and is deleted by a purge after the lock expires

### Requirement: Orphan objects
The orphan sweep SHALL consider only keys inside this installation's namespace that match the key layout. Such
objects that no history row references SHALL be deleted only after the orphan grace period used for local files, and
SHALL be counted in the purge summary. Anything else under the prefix SHALL be left untouched.

#### Scenario: Orphan object
- **WHEN** a key under this installation's namespace belongs to no history row and is older than the grace period
- **THEN** the orphan sweep deletes it and counts it

#### Scenario: Another application's files
- **WHEN** the prefix also holds objects outside this installation's namespace or layout
- **THEN** the orphan sweep never deletes them
