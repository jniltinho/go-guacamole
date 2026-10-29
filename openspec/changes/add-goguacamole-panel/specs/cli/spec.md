## ADDED Requirements

### Requirement: Command tree
The `goguacamole` binary SHALL provide Cobra commands `serve`, `migrate up|down|status`, `user create|passwd|list|disable`, `import guacamole`, `config print`, `healthcheck` and `version`, each with English `--help` text.

#### Scenario: Help
- **WHEN** the user runs `goguacamole --help`
- **THEN** all top-level commands are listed with a one-line description

#### Scenario: Version
- **WHEN** the user runs `goguacamole version`
- **THEN** it prints version, git commit and build date injected via `-ldflags`

### Requirement: TOML file and environment configuration
Configuration SHALL be resolved from flags, then environment variables, then a TOML file, then defaults. The TOML file is given with `--config` or `APP_CONFIG`, else `./config.toml` or `/etc/goguacamole/config.toml` if present. A commented `config.toml.example` SHALL be shipped. TOML sections and env names SHALL map 1:1 (`[app] addr` ↔ `APP_ADDR`, `[db] dsn` ↔ `DB_DSN`, `[guacd] tls_ca` ↔ `GUACD_TLS_CA`, `[tls] cert_file` ↔ `TLS_CERT_FILE`). The contract is:
`APP_ENV`, `APP_ADDR`, `APP_BASE_URL`, `APP_LOG_LEVEL`, `APP_LOG_FORMAT`, `APP_SECRET_KEY`, `APP_SECRET_KEY_FILE`, `APP_COOKIE_SECURE`, `APP_SESSION_IDLE_TIMEOUT`, `APP_SESSION_TTL`, `APP_ALLOWED_ORIGINS`, `APP_TRUSTED_PROXIES`, `APP_ADMIN_USER`, `APP_ADMIN_PASSWORD`, `APP_PROBE_INTERVAL`;
`TLS_ENABLED`, `TLS_CERT_FILE`, `TLS_KEY_FILE`, `TLS_MIN_VERSION`, `TLS_SELF_SIGNED`, `TLS_REDIRECT_ADDR`;
`DB_DRIVER`, `DB_DSN`, `DB_MAX_OPEN_CONNS`;
`GUACD_ADDR`, `GUACD_TLS`, `GUACD_TLS_CA`, `GUACD_TLS_SERVER_NAME`, `GUACD_TLS_INSECURE`;
`AUDIT_RETENTION_DAYS`.
Unknown TOML keys and invalid values SHALL abort startup with an error naming the key.

#### Scenario: Env beats TOML
- **WHEN** `config.toml` has `[app] addr = ":9000"` and `APP_ADDR=:9090` is set
- **THEN** the server listens on `:9090`

#### Scenario: Flag beats env
- **WHEN** `APP_ADDR=:9090` is set and `serve --addr :7070` runs
- **THEN** the server listens on `:7070`

#### Scenario: Unknown key
- **WHEN** the TOML contains `[app] adress = ":80"`
- **THEN** startup fails with `unknown config key app.adress`

#### Scenario: Effective config
- **WHEN** `goguacamole config print` runs
- **THEN** the merged configuration is printed as TOML with secrets (`secret_key`, passwords in DSN, admin password) redacted

#### Scenario: Insecure production
- **WHEN** `APP_ENV=production` and neither TLS is enabled nor `APP_COOKIE_SECURE=true` (TLS terminated by a proxy), or neither `APP_SECRET_KEY` nor `APP_SECRET_KEY_FILE` is set
- **THEN** `serve` refuses to start

### Requirement: Serve and graceful shutdown
`serve` SHALL run pending migrations (unless `--no-migrate`), start the HTTP(S) server, reload TLS certificate files on SIGHUP, and on SIGINT/SIGTERM stop accepting requests, close active tunnels (finalizing history) and exit 0 within a configurable timeout.

#### Scenario: SIGTERM with open tunnel
- **WHEN** SIGTERM arrives while a tunnel is open
- **THEN** the tunnel is closed, its history row gets an end time and the process exits 0

### Requirement: Migrations
`migrate up|down|status` SHALL apply, roll back one step, or list versioned embedded migrations for the configured driver.

#### Scenario: Status on fresh DB
- **WHEN** `migrate status` runs on an empty database
- **THEN** every migration is listed as pending

### Requirement: User commands
`user create <name> [--password P] [--admin]`, `user passwd <name>`, `user list` and `user disable <name>` SHALL operate on the configured database; when `--password` is omitted the password SHALL be read from stdin without echo.

#### Scenario: Create admin
- **WHEN** `goguacamole user create alice --password s3cret123 --admin` runs
- **THEN** `alice` can log in and holds the `ADMINISTER` system permission

### Requirement: Healthcheck command
`goguacamole healthcheck [--url URL] [--insecure]` SHALL exit 0 when the endpoint returns 200 and non-zero otherwise; the default URL follows the configured address and scheme so distroless images can use it as Docker `HEALTHCHECK`.

#### Scenario: Server down
- **WHEN** nothing listens on the URL
- **THEN** the command exits 1
