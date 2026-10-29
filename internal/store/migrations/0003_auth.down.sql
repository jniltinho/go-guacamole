DROP TABLE settings;
ALTER TABLE users DROP COLUMN auth_source;
ALTER TABLE users DROP COLUMN totp_last_step;
ALTER TABLE users DROP COLUMN totp_confirmed;
ALTER TABLE users DROP COLUMN totp_secret;
