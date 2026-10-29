# Operations

## First start

On an empty database `serve` creates the administrator from `APP_ADMIN_USER` / `APP_ADMIN_PASSWORD` (or `admin`
with a random password printed once in the log). The administrator must change the password at first sign-in.

## Backups

| Driver | Backup |
|---|---|
| SQLite | `sqlite3 /data/goguacamole.db ".backup /backup/gg.db"` (safe while running, WAL mode) or stop the app and copy the volume |
| MariaDB | `mariadb-dump --single-transaction goguacamole` |
| PostgreSQL | `pg_dump -Fc goguacamole` |

Always back up the **secret key** (`APP_SECRET_KEY` or `/data/secret.key`) with the database.

## Upgrades

Pull the new image (or binary) and restart; pending migrations run automatically. `goguacamole migrate status`
lists them; `migrate down` rolls back one step.

## Importing from Apache Guacamole

```bash
goguacamole import guacamole --source-driver mysql|postgres --dsn '<guacamole db dsn>' [--dry-run] \
  [--conflict skip|update] [--accept-expansion]
```

- Source opened in a READ ONLY transaction, `SELECT` only.
- Imported: users (hash/salt, disabled, expired → must change password, validity dates, access window, time zone,
  name, e-mail), user groups and memberships (nested groups flattened; paths through disabled groups grant nothing),
  connection folders, connections + parameters (secrets encrypted), guacd overrides, limits, system permissions and
  connection READ grants.
- Not imported (reported): connection-group READ grants (upstream they do not grant child connections), sharing
  profiles, non-READ object permissions. Balancing-group READ becomes READ on each child only with `--accept-expansion`.
- After writing, the importer compares each user's launchable connections in source and target and prints
  `ACCESS DIFF` lines. Re-runs are idempotent (`skip` by default).

## Users from the CLI

```bash
goguacamole user create alice --admin          # password from stdin
goguacamole user passwd alice                  # ends alice's sessions
goguacamole user disable alice                 # open tunnels close within 5 s
goguacamole user list
```

## TLS

```bash
make certs                                   # local CA + server cert in deploy/certs (labs)
TLS_ENABLED=true TLS_CERT_FILE=... TLS_KEY_FILE=... goguacamole serve --addr :8443
kill -HUP <pid>                              # reload rotated certificates
```

## Health and logs

- `/health` (liveness), `/ready` (DB + guacd). The image `HEALTHCHECK` runs `goguacamole healthcheck`.
- Logs are structured (`APP_LOG_FORMAT=json` in containers) with `request_id`; tunnel open/close lines include user,
  connection and Guacamole status.

## Troubleshooting

| Symptom | Check |
|---|---|
| "remote desktop gateway (guacd) is unavailable" (520) | `GUACD_ADDR`, `docker compose ps guacd`, `/ready` |
| "remote server could not be found" (519) / 515 | target hostname/port from the guacd container, credentials, RDP `security` / `ignore-cert` |
| WebSocket closes immediately (403) | `APP_BASE_URL` / `APP_ALLOWED_ORIGINS` vs the browser origin; proxy must forward `Upgrade` |
| Logged out behind a proxy | `APP_COOKIE_SECURE`, `APP_TRUSTED_PROXIES`, `X-Forwarded-Proto` |
| `encrypted connection parameters exist but APP_SECRET_KEY is missing or wrong` | restore the original key |
