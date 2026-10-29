-- stable identity at the provider (OIDC issuer|subject, SAML NameID): a reused user name cannot inherit the account
ALTER TABLE users ADD COLUMN external_id {{STR}}(512);

-- single-use sign-in tokens (encrypted JSON signatures) until they expire
CREATE TABLE used_tokens (
    token {{STR}}(128) NOT NULL PRIMARY KEY,
    expires_at {{TS}} NOT NULL
){{ENGINE}};
