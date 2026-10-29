# Configuration

Sources, highest precedence first: **flags** → **environment variables** → **TOML file** → defaults.

The TOML file is `--config <path>`, else `$APP_CONFIG`, else `./config.toml`, else `/etc/goguacamole/config.toml`.
Every key maps to an environment variable `SECTION_KEY`. Unknown keys and invalid values abort startup.
`goguacamole config print` shows the merged configuration with secrets redacted.

| TOML | Env | Default | Notes |
|---|---|---|---|
| `app.env` | `APP_ENV` | `development` | `production` enforces TLS/secure cookies, a secret key and no insecure TLS options |
| `app.addr` | `APP_ADDR` | `:8080` | flag `--addr` |
| `app.base_url` | `APP_BASE_URL` | | public URL; its host is accepted as WebSocket Origin |
| `app.log_level` | `APP_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `app.log_format` | `APP_LOG_FORMAT` | `text` | `text`, `json` |
| `app.secret_key` | `APP_SECRET_KEY` | | ≥ 32 chars; derives the AES-256-GCM key for connection secrets |
| `app.secret_key_file` | `APP_SECRET_KEY_FILE` | `./data/secret.key` | used when `secret_key` is empty; generated on first start |
| `app.cookie_secure` | `APP_COOKIE_SECURE` | `false` | set when TLS terminates at a proxy |
| `app.session_idle_timeout` | `APP_SESSION_IDLE_TIMEOUT` | `30m` | user input in a tunnel counts as activity |
| `app.session_ttl` | `APP_SESSION_TTL` | `8h` | absolute lifetime; closes tunnels when reached |
| `app.allowed_origins` | `APP_ALLOWED_ORIGINS` | | comma-separated in env |
| `app.trusted_proxies` | `APP_TRUSTED_PROXIES` | | IPs/CIDRs allowed to set `X-Forwarded-For` |
| `app.admin_user` / `app.admin_password` | `APP_ADMIN_USER` / `APP_ADMIN_PASSWORD` | `admin` / random | bootstrap admin on an empty database (must change password at first login) |
| `app.probe_interval` | `APP_PROBE_INTERVAL` | `60s` | TCP reachability of targets; `0s` disables |
| `app.shutdown_timeout` | `APP_SHUTDOWN_TIMEOUT` | `15s` | |
| `tls.enabled` | `TLS_ENABLED` | `false` | native HTTPS/WSS (HTTP/2) |
| `tls.cert_file` / `tls.key_file` | `TLS_CERT_FILE` / `TLS_KEY_FILE` | | PEM full chain; reloaded on `SIGHUP` |
| `tls.min_version` | `TLS_MIN_VERSION` | `1.2` | `1.2` or `1.3` |
| `tls.self_signed` | `TLS_SELF_SIGNED` | `false` | in-memory certificate for labs |
| `tls.redirect_addr` | `TLS_REDIRECT_ADDR` | | e.g. `:80` → redirects to HTTPS |
| `db.driver` | `DB_DRIVER` | `sqlite` | `sqlite`, `mysql`, `postgres` (flag `--db-driver`) |
| `db.dsn` | `DB_DSN` | `./data/goguacamole.db` | flag `--db-dsn` |
| `db.max_open_conns` | `DB_MAX_OPEN_CONNS` | `10` | SQLite always uses 1 writer |
| `guacd.addr` | `GUACD_ADDR` | `127.0.0.1:4822` | flag `--guacd`; per-connection override possible |
| `guacd.tls` | `GUACD_TLS` | `false` | default for connections without an encryption override |
| `guacd.tls_ca` | `GUACD_TLS_CA` | | CA bundle for the guacd certificate |
| `guacd.tls_server_name` | `GUACD_TLS_SERVER_NAME` | | SNI / verification name |
| `guacd.tls_insecure` | `GUACD_TLS_INSECURE` | `false` | labs only |
| `audit.retention_days` | `AUDIT_RETENTION_DAYS` | `90` | daily purge |
| `recording.enabled` | `RECORDING_ENABLED` | `false` (`true` in compose) | record sessions with guacd |
| `recording.path` | `RECORDING_PATH` | `/record` | directory inside the guacd container |
| `recording.local_path` | `RECORDING_LOCAL_PATH` | = `path` | same directory as seen by goguacamole |
| `recording.screen` | `RECORDING_SCREEN` | `true` | graphical recording of every protocol (browser playback) |
| `recording.typescript` | `RECORDING_TYPESCRIPT` | `true` | SSH/Telnet text transcript |
| `recording.include_keys` | `RECORDING_INCLUDE_KEYS` | `false` | include key events in screen recordings |
| `recording.retention_days` | `RECORDING_RETENTION_DAYS` | `90` | daily purge of old recordings |

## Database DSNs

| Driver | Example |
|---|---|
| SQLite | `/data/goguacamole.db` (WAL, foreign keys and busy timeout are set automatically) |
| MariaDB / MySQL | `user:pass@tcp(db:3306)/goguacamole` — TLS: append `?tls=true` (or a registered TLS config) |
| PostgreSQL | `postgres://user:pass@db:5432/goguacamole?sslmode=verify-full&sslrootcert=/certs/ca.pem` |

Create the database and user beforehand for MariaDB/PostgreSQL; tables are created by `migrate up` (run automatically by
`serve` unless `--no-migrate`).
