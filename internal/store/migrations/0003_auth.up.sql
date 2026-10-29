-- Authentication: TOTP second factor (secret sealed with APP_SECRET_KEY) and the
-- source of externally provisioned users (local, ldap, oidc, saml, header, json).
ALTER TABLE users ADD COLUMN totp_secret {{STR}}(255);
ALTER TABLE users ADD COLUMN totp_confirmed {{BOOL}} NOT NULL DEFAULT {{FALSE}};
ALTER TABLE users ADD COLUMN totp_last_step {{TS}} NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN auth_source {{STR}}(16) NOT NULL DEFAULT 'local';
-- Application settings edited in the admin UI (JSON, sealed with APP_SECRET_KEY).
CREATE TABLE settings (
    name {{STR}}(64) NOT NULL PRIMARY KEY,
    value {{TEXT}} NOT NULL,
    updated_at {{TS}} NOT NULL
){{ENGINE}};
