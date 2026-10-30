-- A disabled connection stays configured but cannot be launched; open sessions to it are closed.
ALTER TABLE connections ADD COLUMN disabled {{BOOL}} NOT NULL DEFAULT {{FALSE}};
