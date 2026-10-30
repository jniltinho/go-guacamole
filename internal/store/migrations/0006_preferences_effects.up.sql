-- Visual effects preference (full, reduced, off); NULL = not chosen, the OS reduced-motion setting decides.
ALTER TABLE preferences ADD COLUMN effects {{STR}}(8);
