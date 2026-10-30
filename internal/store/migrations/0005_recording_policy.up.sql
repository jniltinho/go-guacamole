-- Recording policy per connection (NULL = inherit the global default; retention -1 = keep forever)
-- and, per history entry, the policy snapshot, capture result, file state per kind, frozen retention and legal hold.
ALTER TABLE connections ADD COLUMN rec_video {{STR}}(8);
ALTER TABLE connections ADD COLUMN rec_text {{STR}}(8);
ALTER TABLE connections ADD COLUMN rec_keys {{STR}}(8);
ALTER TABLE connections ADD COLUMN retention_video_days INTEGER;
ALTER TABLE connections ADD COLUMN retention_text_days INTEGER;
ALTER TABLE connections ADD COLUMN rec_version INTEGER NOT NULL DEFAULT 0;
ALTER TABLE connection_history ADD COLUMN rec_video {{BOOL}};
ALTER TABLE connection_history ADD COLUMN rec_text {{BOOL}};
ALTER TABLE connection_history ADD COLUMN rec_keys {{BOOL}};
ALTER TABLE connection_history ADD COLUMN rec_exception {{BOOL}} NOT NULL DEFAULT {{FALSE}};
ALTER TABLE connection_history ADD COLUMN video_capture {{STR}}(16);
ALTER TABLE connection_history ADD COLUMN text_capture {{STR}}(16);
ALTER TABLE connection_history ADD COLUMN video_files {{STR}}(16);
ALTER TABLE connection_history ADD COLUMN text_files {{STR}}(16);
ALTER TABLE connection_history ADD COLUMN video_purged_at {{TS}};
ALTER TABLE connection_history ADD COLUMN text_purged_at {{TS}};
ALTER TABLE connection_history ADD COLUMN retention_video_days INTEGER;
ALTER TABLE connection_history ADD COLUMN retention_text_days INTEGER;
ALTER TABLE connection_history ADD COLUMN hold_reason {{STR}}(256);
ALTER TABLE connection_history ADD COLUMN hold_by {{STR}}(128);
ALTER TABLE connection_history ADD COLUMN hold_at {{TS}};
