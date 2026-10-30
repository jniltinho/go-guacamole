# Tasks

## 1. Database foundation and session ownership

- [x] 1.1 Make the Makefile run recipes with bash and `-eo pipefail` so `make test-db` fails when `go test` fails;
  verify by forcing a failing test and seeing a non-zero exit.
- [x] 1.2 Migration `0008_recording_storage` (storage targets, installation id, per-set storage and claim columns
  with fencing token, `owner_instance`, `job_runs`, `job_leases`, `instance_heartbeats`; down migration refusing
  before any DDL while any remote copy, upload in progress or pending deletion exists); verify upgrade tests from a populated 0007 database on SQLite, MariaDB and PostgreSQL via
  `make test-db`.
- [x] 1.3 Instance id and heartbeats; `CloseDanglingHistory` and `finishAbandoned` close only rows of this instance or
  of expired instances; verify a two-instance test where B restarts while A serves a session and A's row stays open.
- [x] 1.4 Quiet-period check over the whole file set (size and mtime unchanged for 2 minutes) used before upload, and
  the local-removal safety window (default 1 hour after the session ended); verify tests where a file keeps growing
  after the session ended (upload waits) and where removal is attempted inside the window (refused).

## 2. Container with embedded Redis

- [x] 2.1 All-in-one image (`Dockerfile` target `allinone`: guacd + goguacamole + Redis under supervisord as the guacd
  user, Redis on 127.0.0.1 without persistence, entrypoint honoring `REDIS_EMBEDDED` and defaulting `REDIS_ADDR`) and
  `docker/` (Compose with local data, MariaDB/PostgreSQL options, nginx config, README, `make docker-allinone`);
  verified: stack up with the three programs, a real SSH session recorded into `docker/data/recordings`, client IP
  logged through nginx.
- [x] 2.2 Regular `docker-compose.yml`: `redis` service (no persistence, internal network) and `REDIS_ADDR` for the app;
  docs (configuration, operations: a dedicated Redis per installation, replicas need a shared one); verify
  `docker compose up` passes `/ready` with the hardening unchanged and the app reports the queue as up.

## 3. Job queue

- [x] 3.1 Config `[redis]` (addr, password, db, tls) and `[jobs]` (worker, concurrency) with env overrides and
  `config print` redaction; verify config tests.
- [x] 3.2 `internal/jobs`: asynq client/server on `goguacamole:`-prefixed queues, installation mark,
  `Register`/`Enqueue`, dedicated-database check through the Inspector (unprefixed queue or foreign mark), fallback without Redis (`ErrQueueUnavailable`, handlers callable from the janitor), queue
  health in readiness details and Administration; verify tests with a Redis container (skipped without
  `GG_TEST_REDIS`), a foreign-queue test and fallback tests.
- [x] 3.3 Periodic enqueuer per UTC period with `TaskID`, `job_runs` checked under the lease (a period already
  completed ends without effect) and used for missed periods, `job_leases` with a per-run token, renewal, cancellation
  on lost renewal and token-conditioned completion; purge, audit purge, orphan sweep, usage, reconciliation and expired web sessions (every minute) moved to
  handlers; "purge now" uses the lease; `RecoverPurging` only resets expired claims; verify tests with three
  instances on one Redis (one run per period), a re-enqueue after the TaskID expired (no second effect), a manual
  purge during a scheduled one (refused), a missed period after a simulated outage (runs once), and an old run that
  resumes after its lease expired (cannot complete or release).
- [x] 3.4 Graceful shutdown (stop fetching, bounded grace, release leases); verify a test that cancels a long handler
  and sees the work completed later exactly once.
- [x] 3.5 `/asynqmon` (jniltinho/asynqmon pinned) with the ADMINISTER middleware, same-origin check on non-GET,
  `jobs.<method>` audit, scoped CSP, Administration link only when the queue is healthy; verify API tests for
  anonymous, auditor, administrator and cross-origin POST, and the theme suite.

## 4. Object storage core

- [x] 4.1 rclone v1.75.2 with only s3, oracleobjectstorage, googlecloudstorage (+ memory in tests); in-memory config
  storage; rclone logs redirected through the sanitizer before any backend exists; a test shows no `rclone.conf` is
  created. Binary size (stripped, linux/amd64): 31.0 MB before, 51.3 MB with object storage (+20.3 MB, within the
  40 MB budget), 31.1 MB with `-tags noobjstore`.
- [x] 4.2 Safe dialer and HTTP client (resolve-and-check at dial time, operator allowlist, HTTPS by default, no
  redirects, no environment proxies); verify tests for 169.254.169.254, fd00:ec2::254, RFC 1918, an allowlisted host,
  a DNS answer that changes between checks, and `HTTPS_PROXY` set in the environment (ignored).
- [x] 4.3 Sanitizer for secrets, base64 forms, JSON key material, Authorization and signature parameters; verify table
  tests and a test that captures rclone's own log output during a failing request.
- [x] 4.4 `objstore` operations: put set (streamed copy), read-back SHA-256 verify (provider checksums never trusted),
  remove other versions of the keys after a verified upload, open, delete every version of a set's keys with
  confirmation that none remains, list the installation namespace; verified on rclone's memory backend (success, a
  tampered object caught) and on an in-memory S3 server with versioning (gofakes3: retries leave one version, deletion
  removes every version and leaves no delete marker) and with deletion refused like an object lock (an error, never
  counted as deleted); and against RustFS 1.0.4 (real S3 server: a versioned bucket, and a bucket with object
  lock COMPLIANCE whose deletion is refused), with `GG_TEST_S3="<endpoint> <key> <secret>"`.
- [x] 4.5 `noobjstore` build tag reporting storage unavailable; verify `go build -tags noobjstore` and its test.

## 5. Storage settings

- [x] 5.1 Versioned targets and `GET/PUT /api/v1/settings/recording/storage` (ADMINISTER, write-only sealed secrets,
  new version on target changes, old versions kept while referenced, refused without a healthy queue, audit
  before/after without secrets); verify API tests for each rule, including changing the bucket with remote sets.
- [x] 5.2 `POST .../storage/test` with form values (probe put/read-back/delete of every version, versioning and
  deletion-refusal report, endpoint safety with a clear message before any request, sanitized error); verified against
  an in-memory S3 that checks signatures (right credentials: ok and versioned; wrong secret: provider error without
  the secret) and a metadata endpoint (refused).
- [x] 5.3 OCI options: S3-compat endpoint from namespace + region with Customer Secret Keys; native with instance or
  workload principal only; verify a config-building test for both and documentation of the OCI setup.
- [x] 5.4 Storage panel in Administration → Recording (provider, bucket, prefix, region, endpoint, namespace,
  path-style, SSE/KMS, credentials set/not set, keep-local policy, enable uploads, Test, Upload existing recordings) in
  en and pt-BR; verify the theme suite (axe AAA) and a Playwright test of save + test against MinIO.

## 6. Upload pipeline

- [ ] 6.1 Record upload owed per present set in the transaction that ends the history row (also `FinishNotStarted`
  and owner-expiry closes); verify store tests for each path.
- [ ] 6.2 Upload handler with fenced claims (D6: token on every update, context cancelled on lost claim), the
  quiet-period gate, full file inventory, object keys, verified
  put, state transitions, sanitized errors, abort-and-clean when the set was purged; verify handler tests on the memory
  backend for success, mismatch, provider down, deleted row (stop), purge during upload, an expired claim whose old
  worker tries to write (rejected), and a crash after upload before bookkeeping (one version left after retry).
- [ ] 6.3 Reconciliation job: expired `uploading` back to owed, present-but-not-remote sets marked owed while uploads
  are enabled, stray versions of remote sets removed, owed/retryable sets with no valid claim re-enqueued; verify tests
  that empty Redis, kill a worker mid-upload, crash between `EndHistory` and `finishCapture`, and leave a stray
  version, and see every set stored once with one version.
- [ ] 6.4 Local copy policy (remove after verified upload after re-hashing, never while held or active; changed files
  are uploaded again), hold refused during any claim, and the backfill action (ADMINISTER, audited); verify tests
  for removal, a held recording kept, a file changed after upload, a hold during any claim (409), and backfill.

## 7. Reading, purge and visibility

- [ ] 7.1 `recordingReader` per set (local, else the set's target version) for playback and transcript text/raw;
  verify tests that remove local files and still stream, with the same audit event, after the bucket was changed.
- [ ] 7.2 Purge per set with the current retention rule, holds blocking everything, exact-version remote deletion,
  `delete_pending` on refusal, expiry of never-uploaded sets, orphan sweep only over the installation namespace and
  key layout with the grace period; verify a test per spec scenario, including different video/text retentions and
  foreign objects under the same prefix (untouched).
- [ ] 7.3 Storage state per set in the history API and the Sessions/history views, retry for administrators; verify
  API tests and the theme suite on the views.

## 8. Integration

- [ ] 8.1 Lab e2e (`deploy/e2e.sh`): the image with embedded Redis plus a MinIO (versioning on) as an allowlisted
  endpoint; record an SSH session, wait for the upload, remove the local file through the policy, play from the
  bucket, then purge a set whose end time was moved back beyond a 1-day retention and check the object version is
  gone; verify the CI e2e job.
- [ ] 8.2 Codex review of the implementation (secrets, asynqmon exposure, claims and purge, endpoint safety); verify
  every finding fixed or answered in the PR.

## Workflow follow-up

- Archive `add-recording-policy-per-connection` first so `recording-policy` exists in `openspec/specs`, then archive
  this change after the PR is merged.
