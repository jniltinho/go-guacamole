-- Session recordings: guacd file names (screen recording and SSH typescript) per history row.
ALTER TABLE connection_history ADD COLUMN recording {{STR}}(64);
ALTER TABLE connection_history ADD COLUMN typescript {{STR}}(64);
