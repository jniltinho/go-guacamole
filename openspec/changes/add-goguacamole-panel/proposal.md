## Why

Remote access today runs the official Apache Guacamole web application (Java/Tomcat + AngularJS 1.x) in front of `guacd`, with our 14 visual themes injected as a CSS extension that overrides upstream markup. The Java client is heavy, hard to customize and couples us to its data model and UI. `guacd` alone is the part that actually speaks RDP/SSH/VNC; everything the Java client adds (login, connection catalog, permissions, WebSocket tunnel) can be owned by a small Go service with a modern UI. GoGuacamole replaces the Java client entirely: **browser → GoGuacamole (Go) → guacd → targets**.

## What Changes

- New Go service **GoGuacamole** (module `github.com/jniltinho/go-guacamole`, binary `goguacamole`, standard Cobra layout: `main.go` at the root embedding `web/dist`, commands in `cmd/`) built with **Echo v5** (REST `/api/v1` + WebSocket) and **Cobra** (CLI).
- Configuration via a **TOML** file and environment variables (env overrides file), plus flags.
- Native **HTTPS/WSS** (certificate files, TLS ≥ 1.2, SIGHUP reload, HTTP→HTTPS redirect, self-signed for labs), TLS to guacd and TLS to the database.
- Native Go implementation of the Guacamole protocol and guacd handshake (SSH, RDP, VNC); a WebSocket tunnel compatible with `guacamole-common-js` that is authorized per user and per connection. **No Java client, no Tomcat.**
- Own application schema (users, user groups, connection groups, connections + parameters, permissions, favorites, app sessions, audit events, preferences) with versioned migrations for **SQLite** (default), **MariaDB/MySQL** and **PostgreSQL**.
- One-shot, read-only **importer** from an existing Guacamole JDBC database (MySQL/MariaDB or PostgreSQL): users with their current password hashes and restrictions, user groups and memberships, connection groups, connections and parameters, READ/system permissions — without silently expanding anyone's access.
- Local authentication with secure server-side sessions, bootstrap administrator, rate limiting, CSRF/Origin protection, revocation that closes live tunnels, and audit of security-relevant actions.
- **Vue 3 + TypeScript** SPA built with **Vite+ (`vp`)** and **Tailwind CSS v4**, layout following the GoGuacamole mockup (sidebar, topbar search, dashboard cards, connections table, recent activity, active sessions, permission overview), no rounded corners, light/dark plus the **14 existing themes**, embedded in the Go binary.
- UI in **English** (source) with full **pt-BR** translation; code, docs and specs in English.
- Docker: multi-stage image published to **GitHub Container Registry** (`ghcr.io/jniltinho/go-guacamole`) by GitHub Actions after e2e passes; `docker-compose.yml` (app + guacd, `mariadb`, `postgres`, `tls` and `lab` profiles with SSH/RDP/VNC targets); `Makefile`; e2e tests.
- The local `base/` folder (reference Guacamole stack, its database, backups, spec drafts) is reference-only and **never committed**.

## Capabilities

### New Capabilities
- `cli`: Cobra commands (`serve`, `migrate`, `user`, `import guacamole`, `config print`, `healthcheck`, `version`) and configuration via flags / env / TOML.
- `app-storage`: database drivers (SQLite, MariaDB/MySQL, PostgreSQL), application schema, secret registry and migrations.
- `auth-session`: login/logout, password hashing, sessions, bootstrap admin, revocation, rate limiting, CSRF/Origin, native HTTPS.
- `access-control`: permission model (system permissions, object READ via users/groups) and authorization matrix.
- `guacamole-import`: read-only import from an official Guacamole database.
- `remote-tunnel`: Guacamole protocol codec, guacd handshake, launch tickets, WebSocket relay, limits, history.
- `connections-dashboard`: dashboard, connection catalog/tree, search, favorites, reachability, recent activity.
- `administration`: CRUD of connections, connection groups, users, user groups, permissions; active sessions; audit log.
- `web-ui`: SPA shell, login, remote client view, themes, i18n.
- `deployment`: Docker image on GHCR, compose stacks, Makefile, CI, e2e.

### Modified Capabilities
<!-- none: greenfield repository -->

## Impact

- New repository `github.com/jniltinho/go-guacamole` with `main.go`, `cmd/`, `internal/` (Go), `frontend/` (Vue → `web/dist`), `e2e/`, `deploy/`, `docs/`, `openspec/`. License MIT.
- Runtime: `guacamole/guacd:1.6.0` on a private network; SQLite volume, MariaDB or PostgreSQL.
- The Java `guacamole/guacamole` container is no longer needed; its database is only read once by the importer.
- Go deps: `labstack/echo/v5`, `spf13/cobra`, `spf13/viper` (TOML), `coder/websocket`, `go-sql-driver/mysql`, `jackc/pgx/v5`, `modernc.org/sqlite`, `golang.org/x/crypto` (bcrypt).
- JS deps: `vue`, `vue-router`, `pinia`, `vue-i18n`, `guacamole-common-js`, `tailwindcss` v4, `vite-plus`.
- Out of scope for this change (documented as P2): SSO/LDAP/OIDC/MFA, session sharing/recording playback, file transfer UI, multiple guacd environments, Telnet/Kubernetes protocols.
