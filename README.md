# GoGuacamole

A modern web gateway for [Apache Guacamole](https://guacamole.apache.org/) **guacd** — RDP, SSH and VNC in the
browser — written in **Go** (Echo v5 + Cobra) with an embedded **Vue 3** UI (Vite+ · Tailwind CSS v4).
It replaces the Java `guacamole-client`: one static binary + `guacd`, nothing else.

```
browser (Vue 3 + guacamole-common-js) ──HTTPS/WSS──▶ goguacamole (Go) ──Guacamole protocol──▶ guacd ──▶ RDP / SSH / VNC
```

- **No Java / Tomcat** — the Guacamole protocol, guacd handshake and WebSocket tunnel are implemented in Go.
- **SQLite** (default), **MariaDB/MySQL** or **PostgreSQL**; read-only **importer** from an existing Guacamole database.
- Users, user groups, connection folders, connections, permissions (direct and inherited), active sessions,
  history and audit log — with secrets encrypted at rest (AES-256-GCM).
- Secure by default: server-side sessions, CSRF + Origin checks, single-use launch tickets, revocation that closes
  live tunnels, rate limiting, CSP, native HTTPS (TLS 1.2+), TLS to guacd.
- **16 themes** (Light, Dark + the 14 legacy Guacamole themes: NOC, Dracula, Bionexo, Matrix, xterm, macOS, Ubuntu,
  KDE, Google, High contrast, Windows 11/XP/98, Mac OS retro) — adding one is a folder, see
  [frontend/src/themes](frontend/src/themes/README.md).
- UI in **English** and **Português (Brasil)**.
- Container image on **GHCR**: `ghcr.io/jniltinho/go-guacamole`.

## Quick start (Docker)

```bash
git clone https://github.com/jniltinho/go-guacamole.git && cd go-guacamole
cp .env.example .env            # set APP_ADMIN_PASSWORD (8+ chars) and APP_SECRET_KEY (openssl rand -hex 32)
make up                         # app + guacd, SQLite volume
open http://localhost:8080      # sign in as admin; you will be asked to change the password
```

Without `APP_ADMIN_PASSWORD` a random password is printed once: `docker compose logs app | grep bootstrap`.

Other stacks:

| Command | What it starts |
|---|---|
| `make up-mariadb` | app + guacd + MariaDB 11.4 |
| `make up-postgres` | app + guacd + PostgreSQL 17 |
| `make up-lab` | app + guacd + SSH / VNC / RDP test targets (`ssh-target:2222`, `vnc-target:5901`, `rdp-target:3389`) |
| `make certs up-tls` | native HTTPS on `https://localhost:8443` with a local CA |
| `make down` | stop everything |

Pre-built image only:

```bash
docker network create guac
docker run -d --name guacd --network guac guacamole/guacd:1.6.0
docker run -d --name goguacamole --network guac -p 8080:8080 -v gg-data:/data \
  -e APP_ADMIN_PASSWORD='change-me-now' ghcr.io/jniltinho/go-guacamole:latest
```

## Quick start (binary)

```bash
make build                                   # frontend (vp build → web/dist) + Go binary (bin/goguacamole)
docker run -d -p 127.0.0.1:4822:4822 guacamole/guacd:1.6.0
APP_ADMIN_PASSWORD='change-me-now' GUACD_ADDR=127.0.0.1:4822 ./bin/goguacamole serve
```

## Migrating from Apache Guacamole

```bash
# read-only: the source database is never modified
goguacamole import guacamole --source-driver mysql \
  --dsn 'guacamole_user:pass@tcp(old-db:3306)/guacamole_db' --dry-run
goguacamole import guacamole --source-driver mysql --dsn '...'
```

Users keep their current passwords (Guacamole SHA-256 hashes are verified and upgraded to bcrypt on first login).
The importer never widens access: it prints, per user, any connection whose access would differ. See
[docs/operations.md](docs/operations.md#importing-from-apache-guacamole).

## CLI

```
goguacamole serve                       start the HTTP(S) server
goguacamole migrate up|down|status      database migrations
goguacamole user create|passwd|list|disable
goguacamole import guacamole            import from an Apache Guacamole database
goguacamole config print                effective configuration (secrets redacted)
goguacamole healthcheck                 used by the Docker HEALTHCHECK
goguacamole version
```

Configuration: `config.toml` (see [config.toml.example](config.toml.example)) and/or environment variables
(`APP_*`, `TLS_*`, `DB_*`, `GUACD_*`, `AUDIT_*`) — details in [docs/configuration.md](docs/configuration.md).

## Development

```bash
make deps
make dev-backend      # terminal 1: Go API on :8080 (needs guacd, e.g. make up-lab)
make dev-frontend     # terminal 2: Vite+ on :9000, proxies /api (and the WebSocket) to :8080
make test             # Go + frontend unit tests
make test-db          # repository tests on MariaDB and PostgreSQL containers
make lint             # go vet, gofmt, vp check
make e2e              # API/tunnel e2e on 3 databases + Playwright browser suite (Docker)
```

Layout: `main.go` (embeds `web/dist`), `cmd/` (Cobra commands), `internal/` (config, guac protocol, tunnel, store,
auth, access, importer, server), `frontend/` (Vue app), `e2e/`, `deploy/`, `docs/`, `openspec/`.

## Documentation

- [Architecture](docs/architecture.md)
- [Configuration](docs/configuration.md)
- [REST API](docs/api.md)
- [Security](docs/security.md)
- [Operations](docs/operations.md) — backups, upgrades, import, TLS, troubleshooting
- [Themes](frontend/src/themes/README.md)

## License

[MIT](LICENSE). See [NOTICE](NOTICE) for third-party attributions.
