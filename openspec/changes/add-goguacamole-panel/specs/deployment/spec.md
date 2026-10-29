## ADDED Requirements

### Requirement: Container image on GHCR
A multi-stage `Dockerfile` SHALL build the frontend with Vite+, build a static Go binary (`CGO_ENABLED=0`, version ldflags) and produce a minimal non-root image (Alpine) exposing 8080 (HTTP) and 8443 (HTTPS) with `HEALTHCHECK` using `goguacamole healthcheck`. A GitHub Actions workflow SHALL run lint and unit tests on every push/PR; on pushes to `main` and tags `v*` it SHALL run the e2e suite and only then build `linux/amd64` images and push them to `ghcr.io/jniltinho/go-guacamole` with tags `main`, `sha-<short>`, the semver tags and `latest` (on release tags), with OCI labels linking the repository.

#### Scenario: Tag release
- **WHEN** tag `v0.1.0` is pushed and e2e passes
- **THEN** `ghcr.io/jniltinho/go-guacamole:0.1.0` and `:latest` are published

#### Scenario: E2E gate
- **WHEN** the e2e job fails on `main`
- **THEN** no image is pushed

#### Scenario: Image size
- **WHEN** `make docker-build` runs
- **THEN** the image is smaller than 60 MB

### Requirement: Health and readiness
`GET /health` SHALL return 200 when the process is alive; `GET /ready` SHALL return 200 only when the database is reachable and guacd accepts a connection, else 503 with the failing dependency.

#### Scenario: guacd down
- **WHEN** guacd is stopped
- **THEN** `/ready` returns 503 `{"guacd":"down"}` and `/health` still returns 200

### Requirement: Compose stacks
`docker-compose.yml` SHALL define `app` and `guacd` (`guacamole/guacd:1.6.0`, not published to the host) on a private network with SQLite on a named volume by default; profile `mariadb` adds `mariadb:11.4`, profile `postgres` adds `postgres:17` (the app switches driver via `.env`); profile `lab` adds SSH (`linuxserver/openssh-server`), VNC and RDP (xrdp) test targets; `make up-tls` runs the app with `TLS_ENABLED=true` using certificates mounted from `./deploy/certs` (a `make certs` target generates a local CA and server certificate). The app container SHALL run non-root with read-only root filesystem, restart policy and memory/CPU limits; configuration comes from a git-ignored `.env` (`.env.example` committed) and optionally a mounted `config.toml`.

#### Scenario: Default stack
- **WHEN** `make up` runs on a clean machine
- **THEN** app and guacd become healthy and `http://localhost:8080` shows the login screen

#### Scenario: TLS stack
- **WHEN** `make certs up-tls` runs
- **THEN** `https://localhost:8443` serves the UI with the generated certificate

### Requirement: Makefile
The Makefile SHALL provide `help` (default), `deps`, `dev`, `dev-backend`, `dev-frontend`, `build`, `test`, `test-backend`, `test-frontend`, `lint`, `fmt`, `certs`, `docker-build`, `docker-push`, `up`, `up-mariadb`, `up-postgres`, `up-lab`, `up-tls`, `down`, `logs`, `migrate-up`, `migrate-down`, `e2e` and `clean`.

#### Scenario: Help
- **WHEN** `make` runs without arguments
- **THEN** every target is listed with a description

### Requirement: End-to-end tests
`make e2e` SHALL start the lab stack and run, for each of SQLite, MariaDB and PostgreSQL:
- an API suite: login with CSRF/Origin, forced password change, create SSH/RDP/VNC connections to the lab targets, grant to a regular user via a group, issue a ticket, open the WebSocket tunnel and assert the UUID instruction and display instructions, assert ticket reuse and foreign Origin are refused, assert a user without permission gets 404, revoke during an open tunnel and during a handshake and assert closure with status 771, absolute-TTL expiry, history and audit rows (no secrets);
- a browser suite (Playwright, Chromium and Firefox) on the SQLite run: login in en and pt-BR, dashboard, open SSH, RDP and VNC sessions and assert a rendered display canvas, resize, clipboard round-trip on SSH, disconnect and reconnect, theme switch persisted.

#### Scenario: E2E green
- **WHEN** `make e2e` runs on a machine with Docker
- **THEN** it exits 0 after all driver runs and the browser suite pass

### Requirement: Repository hygiene
`base/`, `.env`, `data/`, `deploy/certs/*` (except a README), build outputs and `node_modules` SHALL be git-ignored and never committed.

#### Scenario: base ignored
- **WHEN** `git ls-files base` runs
- **THEN** it returns nothing
