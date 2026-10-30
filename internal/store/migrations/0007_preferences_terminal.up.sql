-- Terminal color scheme for SSH sessions (palette name, "follow" or NULL = match the UI theme).
ALTER TABLE preferences ADD COLUMN terminal_scheme {{STR}}(32);
