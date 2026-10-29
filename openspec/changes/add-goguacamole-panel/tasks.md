## 1. Repository setup

- [x] 1.1 Layout `main.go`, `cmd/`, `internal/`, `frontend/`, `deploy/`, `docs/`; `.gitignore` (base/, .env, data/, dist, node_modules, bin), `LICENSE` (MIT), `NOTICE` (credit wwt/guac)
- [x] 1.2 `.env.example` and `goguacamole.example.toml` with the configuration contract, `README.md`

## 2. Backend foundation

- [x] 2.1 Go module, Cobra root + Viper (TOML + ENV) binding + typed config with unknown-key rejection, validation and tests (precedence, invalid values, production rules); `config print` with redaction
- [x] 2.2 `version`, `healthcheck` commands; slog logger setup
- [x] 2.3 Echo v5 server: middleware (recover, request id, logs, body limit, security headers/CSP), error model, `/health`, `/ready`, graceful shutdown
- [x] 2.4 Native HTTPS: cert files, min version, SIGHUP reload, redirect listener, HSTS, self-signed lab mode, tests

## 3. Storage

- [x] 3.1 Models and repository interfaces
- [x] 3.2 Dialect layer (rebind, insert id, upsert) + migration runner + `0001_init` up/down for sqlite, mysql and postgres; `migrate up|down|status`
- [x] 3.3 Secret parameter registry + AES-GCM encryption with tests enumerating guacd credential params
- [x] 3.4 sqlstore: users, user groups, members, connection groups, connections, parameters, permissions, favorites, sessions, history/audit snapshots, preferences
- [x] 3.5 Repository tests on SQLite, MariaDB (`TEST_MYSQL_DSN`) and PostgreSQL (`TEST_POSTGRES_DSN`)

## 4. Auth and access control

- [x] 4.1 bcrypt + legacy Guacamole SHA-256 verification/upgrade with uniform failure cost; tests (salted, unsalted, unicode)
- [x] 4.2 Bootstrap admin, login contract (pre-session CSRF/Origin, forced change), sessions (cookie, idle/absolute expiry, janitor), CSRF token, logout/me/password/preferences endpoints
- [x] 4.3a Account restrictions: disabled, validity dates, access windows with timezone and end-of-window revocation
- [x] 4.3b Rate limiter (IP + username), trusted proxy client IP
- [x] 4.4 Effective permission resolver and authorization helpers per matrix; tests per matrix row
- [x] 4.6 Cross-process revocation (`security_stamp`, `access_epoch` polling) and restriction expiry scheduler; tests with CLI disable during an open tunnel
- [x] 4.5 Revocation events (grants, membership, group disable, moves, deletes, user changes) closing sessions, tickets, handshakes and tunnels
- [x] 4.7 `user create|passwd|list|disable` commands

## 5. Guacamole protocol and tunnel

- [x] 5.1 `internal/guac` codec + streaming reader (rune lengths, limits) with tests incl. split reads and malformed data
- [x] 5.2 guacd dial (TCP/TLS) + handshake (version negotiation, timezone, error relay, timeout) with fake-guacd tests
- [x] 5.3 Launch tickets + registry with atomic reservations, limits and re-authorization at redemption and before activation; race tests
- [x] 5.4 WebSocket relay (Origin, subprotocol, UUID first, ping echo, batching, deadlines, error instruction + close code/reason contract, close propagation), history and audit hooks
- [x] 5.5 Tunnel tests with fake guacd + websocket client

## 6. API

- [x] 6.1 Connections catalog (search/filter/sort/paginate), tree, favorites, dashboard
- [x] 6.2 Connection and connection group CRUD with protocol parameter validation, secret masking, cycle check
- [x] 6.3 Users, user groups, members, permissions (direct + effective with source), self-lockout/last-admin guards
- [x] 6.4 Sessions list/kill, history (scoped, filtered), audit list + retention job
- [x] 6.5 TCP reachability prober
- [x] 6.6 Embedded SPA serving (fallback, cache headers)
- [x] 6.7 Handler tests (httptest + SQLite)

## 7. Importer

- [x] 7.1 `import guacamole` (mysql/postgres source, read-only tx, dry-run, conflict modes, no-expansion rules, `--accept-expansion`, access diff report, restrictions, single transaction)
- [x] 7.2 Importer tests against the official Guacamole 1.6 schema in MariaDB and PostgreSQL (fixtures incl. folder READ, balancing, nested/disabled groups, expired users)

## 8. Frontend

- [x] 8.1 Scaffold with `vp create vue`; Tailwind v4, router, pinia, vue-i18n, guacamole-common-js, lucide; dev proxy
- [x] 8.2 API client, auth store, router guards, permission-aware menu
- [x] 8.3 i18n `en` + `pt-BR`, locale detection, parity test, status/error code translations
- [x] 8.4 Theme system: tokens for 16 themes, extras, favicons, Matrix rain, clock, switcher, preference sync
- [x] 8.5 Shell (sidebar, topbar search, breadcrumbs), login (forced password change), 403/404
- [x] 8.6 Dashboard and connections (table + tree, search, filters, favorites, status, connect)
- [x] 8.7 Remote client view (ticket, tunnel, scale, resize, keyboard/mouse/touch, clipboard, fullscreen, Ctrl+Alt+Del, states, cleanup)
- [x] 8.8 Admin screens: connections + protocol forms, connection groups, users, groups, permissions, sessions, audit, settings
- [x] 8.9 `vp lint`, `vp test`, `vp build` clean; build output copied into `backend/internal/webui/dist`

## 9. Docker, Makefile, CI, E2E

- [x] 9.1 Multi-stage Dockerfile (distroless nonroot, healthcheck)
- [x] 9.2 `docker-compose.yml` with default, `mariadb`, `postgres`, `tls` and `lab` (SSH, VNC, xrdp) profiles; `make certs`
- [x] 9.3 Makefile with all targets from the deployment spec
- [x] 9.4 API e2e suite per deployment spec for SQLite, MariaDB and PostgreSQL
- [ ] 9.5 Playwright browser suite (Chromium, Firefox): login en/pt-BR, dashboard, SSH/RDP/VNC sessions, resize, clipboard, reconnect, theme persistence
- [x] 9.6 GitHub Actions: CI (lint/test on 3 DBs/build) on push/PR; e2e on main/tags; GHCR multi-arch publish gated on e2e
- [x] 9.7 `docs/` (architecture, api, security, operations)

## 10. Delivery

- [x] 10.1 `openspec validate --strict` and Codex review; address findings
- [ ] 10.2 Verify `base/` is ignored; create GitHub repo `jniltinho/go-guacamole`, push, confirm CI and GHCR image
- [ ] 10.3 Archive the OpenSpec change
