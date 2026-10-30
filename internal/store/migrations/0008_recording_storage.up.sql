-- Recording storage (openspec add-async-jobs-object-storage): object storage targets (sealed config, versioned),
-- per-set storage state on history rows (video_*, text_*: local|owed|uploading|remote|failed|delete_pending|deleted,
-- target, manifest of keys/versions/digests, claim with fencing token), the instance owning an open row, and the
-- job queue's database side (completed periods, leases, instance heartbeats).
CREATE TABLE storage_targets (
    id {{ID}},
    config {{TEXT}} NOT NULL,
    created_at {{TS}} NOT NULL,
    retired_at {{TS}}
){{ENGINE}};
CREATE TABLE job_runs (
    job {{STR}}(64) NOT NULL,
    period {{STR}}(32) NOT NULL,
    completed_at {{TS}} NOT NULL,
    PRIMARY KEY (job, period)
){{ENGINE}};
CREATE TABLE job_leases (
    job {{STR}}(64) NOT NULL PRIMARY KEY,
    owner {{STR}}(64) NOT NULL,
    run_token {{STR}}(64) NOT NULL,
    expires_at {{TS}} NOT NULL
){{ENGINE}};
CREATE TABLE instance_heartbeats (
    instance {{STR}}(64) NOT NULL PRIMARY KEY,
    seen_at {{TS}} NOT NULL
){{ENGINE}};
ALTER TABLE connection_history ADD COLUMN owner_instance {{STR}}(64);
ALTER TABLE connection_history ADD COLUMN video_storage {{STR}}(16);
ALTER TABLE connection_history ADD COLUMN video_target_id INTEGER;
ALTER TABLE connection_history ADD COLUMN video_remote {{TEXT}};
ALTER TABLE connection_history ADD COLUMN video_attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE connection_history ADD COLUMN video_storage_error {{STR}}(512);
ALTER TABLE connection_history ADD COLUMN video_uploaded_at {{TS}};
ALTER TABLE connection_history ADD COLUMN video_local_removed {{BOOL}} NOT NULL DEFAULT {{FALSE}};
ALTER TABLE connection_history ADD COLUMN video_claim_owner {{STR}}(64);
ALTER TABLE connection_history ADD COLUMN video_claim_token {{STR}}(64);
ALTER TABLE connection_history ADD COLUMN video_claim_until {{TS}};
ALTER TABLE connection_history ADD COLUMN text_storage {{STR}}(16);
ALTER TABLE connection_history ADD COLUMN text_target_id INTEGER;
ALTER TABLE connection_history ADD COLUMN text_remote {{TEXT}};
ALTER TABLE connection_history ADD COLUMN text_attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE connection_history ADD COLUMN text_storage_error {{STR}}(512);
ALTER TABLE connection_history ADD COLUMN text_uploaded_at {{TS}};
ALTER TABLE connection_history ADD COLUMN text_local_removed {{BOOL}} NOT NULL DEFAULT {{FALSE}};
ALTER TABLE connection_history ADD COLUMN text_claim_owner {{STR}}(64);
ALTER TABLE connection_history ADD COLUMN text_claim_token {{STR}}(64);
ALTER TABLE connection_history ADD COLUMN text_claim_until {{TS}};
