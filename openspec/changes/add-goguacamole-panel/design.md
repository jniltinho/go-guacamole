## Context

The reference environment (local `base/`, never versioned) runs MariaDB 11.4 + `guacamole/guacd:1.6.0` + `guacamole/guacamole:1.6.0` (Java), with a theme extension (`bionexo-themes.jar`, 14 Tailwind-3 CSS themes overriding AngularJS markup, `theme-switcher.js` with localStorage key `guac-theme`, default `noc`, Matrix rain, Win98 clock, per-theme SVG favicons). Connections were imported from MobaXterm through the Java REST API. `base/GO_GUACAMOLE_ESPECIFICACAO.md` and the mockup `Painel GoGuacamole – Visão Geral de Administração.png` describe the desired panel (sidebar, dashboard cards, connections table, recent activity, active sessions, permission overview; no rounded corners; SQLite first; Echo v5; Makefile; Docker).

Decision taken with the product owner: **drop the Java client**. GoGuacamole talks to `guacd` directly and owns auth, catalog, permissions and audit, with a one-shot importer from the old `guacamole_db`.

## Goals / Non-Goals

**Goals:**
- One static Go binary (API + WebSocket tunnel + embedded SPA) and `guacd`; nothing else required (SQLite default; MariaDB/MySQL and PostgreSQL optional).
- Configuration by TOML file + environment variables; native HTTPS.
- Secure-by-default: server-side sessions, CSRF, Origin checks, per-request authorization, launch tickets, revocation closing tunnels, encrypted secrets at rest, audit.
- SSH, RDP and VNC sessions in the browser with clipboard, resize, fullscreen.
- Admin of connections, connection groups, users, user groups, permissions; active sessions; audit.
- UI per mockup, 16 themes (light, dark + 14 ported), en + pt-BR.
- Docker image on GHCR via GitHub Actions; Makefile; e2e on all three DB drivers plus browser tests.

**Non-Goals (P2, documented):**
- SSO / LDAP / OIDC / SAML / MFA (TOTP). Production cut-over from the Java client is conditional on an explicit decision on MFA if current policy requires it.
- Session sharing, recording playback, file-transfer UI, printing (guacd params for recording may still be configured).
- Balancing groups / failover; nested user groups (flattened by importer).
- Multiple guacd environments (mockup "Production" selector), notifications bell.
- Horizontal scaling: tunnel registry, limits and kills are process-local; single replica is the supported topology.
- Telnet and Kubernetes protocols (guacd supports them; deferred until they get equivalent e2e coverage).

## Decisions

### Repository layout
Standard Cobra layout used by the author's other Go projects (go-dash, go-postfixadmin, ad-manager):
```
main.go                      embeds web/dist (go:embed all:web/dist) and calls cmd.Execute(assets)
cmd/                         root, serve (HTTP/HTTPS), migrate, user, import, config, healthcheck, version
internal/config/             typed config, TOML + env + flags (Viper), validation
internal/guac/               protocol codec, streaming reader, guacd dial (TCP/TLS) + handshake, status codes
internal/tunnel/             launch tickets, registry (reservations, kill, revocation), ws relay
internal/store/              repositories (database/sql) + dialects + embedded migrations (single template, per-driver tokens)
internal/secrets/            secret-parameter registry + AES-256-GCM box
internal/auth/               hashing (bcrypt + legacy guac sha256), restrictions, rate limiter
internal/access/             effective permission resolver + authorization helpers
internal/importer/           guacamole_db → app schema (read-only source)
internal/server/             echo app, middleware, handlers (/api/v1), enforcer, prober, SPA serving
frontend/                    Vue 3 + TS (vp) → builds into ../web/dist; themes in src/themes/<id>/ (plug-in folders)
web/dist/.gitkeep            keeps the embed directive compiling without a frontend build
e2e/                         Go API/tunnel e2e (build tag e2e); frontend/e2e Playwright suite
deploy/                      e2e.sh runner, certs/
docs/                        architecture, configuration, api, security, operations
.github/workflows/           ci.yml, docker-image-publish.yml
Dockerfile, docker-compose.yml, Makefile, config.toml.example, .env.example
```
Module: `github.com/jniltinho/go-guacamole`; build metadata via `-X github.com/jniltinho/go-guacamole/cmd.Version=…`.

### HTTP: Echo v5
Echo v5 routing and middleware: recover, request id, `slog` access logs (JSON in production), body limit, secure headers/CSP, gzip (skipped for WebSocket), CSRF (custom: header vs session token), auth (cookie → session → user + effective permissions cached per request). Uniform error body `{code, message, requestId}`; the UI translates `code`.

### CLI/config: Cobra + Viper (TOML + ENV)
Flags bound to Viper keys; env names from the spec contract (`APP_*`, `TLS_*`, `DB_*`, `GUACD_*`, `AUDIT_*`) via explicit `BindEnv` so `[section] key` ↔ `SECTION_KEY`; config file type `toml` (`--config`, `APP_CONFIG`, `./goguacamole.toml`, `/etc/goguacamole/goguacamole.toml`). The merged map is decoded into a typed struct with `mapstructure` `ErrorUnused` to reject unknown keys. `config print` emits redacted TOML. Validation produces actionable errors; `APP_ENV=production` enforces TLS or secure cookies behind a proxy, and `APP_SECRET_KEY`.

### HTTPS
`http.Server` with `tls.Config{MinVersion, GetCertificate}`; `GetCertificate` returns an atomically swapped certificate loaded from files, reloaded on SIGHUP (open tunnels unaffected). Optional redirect listener (`TLS_REDIRECT_ADDR`) and HSTS. `TLS_SELF_SIGNED` creates an ECDSA P-256 cert in memory for labs. Secure cookie flag is set automatically when serving TLS. `make certs` creates a local CA + server cert with `openssl` in a container for the `tls` compose profile and e2e.

### Guacamole protocol — own package, informed by wwt/guac
`github.com/wwt/guac` (Apache-2.0) was evaluated. It provides a streaming reader and handshake, but: Origin check is `return true // TODO`, no `timezone`, `VERSION_*` answered with empty string (guacd falls back to protocol 1.0), internal pings dropped instead of echoed, no tunnel-UUID instruction, unbounded client messages, no guacd TLS, and it is built on gorilla/websocket + logrus with its own `http.Handler`, which fits poorly with Echo v5 and `slog`. We implement `internal/guac` (~500 LOC) reusing its ideas (credited in `NOTICE`):
- `Instruction{Opcode, Args}`; encoder uses `utf8.RuneCountInString`.
- `Reader`: buffered, scans length prefixes counting runes to the `;` terminator, returns raw bytes of whole instructions, `Buffered()` lets the relay batch several instructions per WS message.
- `Dial(ctx, addr, tlsCfg)` and `Handshake(ctx, conn, cfg)`: `select` → `args` → `size`/`audio`/`video`/`image`/`timezone` → `connect` (args order; version = min(offered, 1.5.0); missing = "") → `ready`; guacd `error` surfaces as `*guac.Error{Status, Message}`.
- Status code table matches guacamole-common-js `Guacamole.Status.Code` (e.g. 512 SERVER_ERROR, 514 UPSTREAM_TIMEOUT, 515 UPSTREAM_ERROR, 519 UPSTREAM_NOT_FOUND, 520 UPSTREAM_UNAVAILABLE, 522 SESSION_TIMEOUT, 523 SESSION_CLOSED, 768 CLIENT_BAD_REQUEST, 769 CLIENT_UNAUTHORIZED, 771 CLIENT_FORBIDDEN, 797 CLIENT_TOO_MANY).

### Tunnel
- **Launch ticket**: `POST /api/v1/sessions` authorizes and atomically reserves capacity in the registry, returns `{ticket, expiresAt}`; ticket = random 128-bit, stored in memory, single use, 30 s, bound to session id + user + connection. Unused reservations expire with the ticket. Authorization is re-checked at redemption and again after the guacd handshake, right before relay activation; tickets and in-flight handshakes are registered in the same registry so revocation events cancel them (race tests use injected hooks between steps). This gives the browser readable JSON errors (403/404/409/429) before the WebSocket, which `Guacamole.WebSocketTunnel` cannot read after a failed upgrade.
- **WebSocket**: `github.com/coder/websocket` via Echo handler, subprotocol `guacamole`, Origin check, read limit, ping/pong handled at Guacamole level. On upgrade: consume ticket, dial + handshake guacd, send `0.,36.<uuid>;`, then two goroutines under `errgroup` with context cancellation: guacd→WS batches whole instructions until reader buffer empty or 8 KiB; WS→guacd validates each message parses as complete instructions within limits, echoes internal `ping`, forwards the rest. Write deadlines 10 s; guacd read idle 30 s (guacd sends `sync` frames regularly). Post-upgrade failures send `error` then a close frame: code 1008 (CLIENT_*), 1011 (SERVER_*/UPSTREAM_*), 1000 (normal), reason = decimal Guacamole status — matching how `Guacamole.WebSocketTunnel` derives `Guacamole.Status` from `event.reason`.
- **Registry**: map tunnelID → {user, session, connection, start, ip, cancel}; indexes by user/session/connection for limits, revocation and kills. The access service publishes revocation events (user disabled/deleted/password reset, permission removed, group disabled, membership removed) that trigger re-evaluation of affected open tunnels (immediate) — the 5 s bound is met without polling.
- Cross-process changes: `users.security_stamp` and `system_state.access_epoch` are polled every 5 s; a change triggers re-evaluation of the affected (or all) open sessions, tickets and tunnels. A restriction scheduler arms timers for the earliest access-window end / `valid_until` among active sessions.
- Activity: messages from the browser other than internal/`nop`/`sync` acks touch the session's `last_seen` at most every 30 s.

### Storage
`database/sql` with queries written using `?` placeholders and a dialect layer that rebinds to `$n` for PostgreSQL (`jackc/pgx/v5/stdlib`), returns inserted ids (`LastInsertId` on SQLite/MySQL, `RETURNING id` on PostgreSQL) and provides upsert syntax (`ON CONFLICT` for SQLite/PostgreSQL, `ON DUPLICATE KEY` for MySQL). Timestamps generated in Go (UTC). SQLite via `modernc.org/sqlite` (no CGO) with `_pragma=foreign_keys(1)`, WAL, `busy_timeout(5000)`, `MaxOpenConns=1` for writes. Migrations: own minimal runner over embedded files (`NNNN_name.up.sql`/`.down.sql`) written once with type tokens (`{{ID}}`, `{{BOOL}}`, `{{STR}}`…) rendered per dialect; avoids extra deps and keeps the three schemas identical. IDs are integer autoincrement; API exposes them as strings. Secrets: AES-256-GCM with key = HKDF-SHA256(`APP_SECRET_KEY`), value stored as `v1:<nonce>:<ciphertext>` base64; a key check row detects wrong keys.
Effective permissions: one query unions direct grants and enabled-group grants; connection-group READ is expanded over the tree in Go (tree is small, cached per request). History and audit rows carry name snapshots with nullable FKs (`ON DELETE SET NULL`).
Alternative: GORM — rejected; explicit SQL is clearer and portable here.

### Auth
bcrypt cost 12 for new hashes; legacy Guacamole `sha256(password + upperhex(salt))` verified with `subtle.ConstantTimeCompare` then upgraded. Every failure path (unknown user, legacy mismatch, disabled) runs one bcrypt compare against a dummy hash to equalize timing. Login (pre-session) is protected by `X-Requested-With` + same-origin `Origin`/`Sec-Fetch-Site`; post-login mutations use the per-session CSRF token. Access windows are evaluated with `time.LoadLocation(user.timezone)`; a scheduler revokes sessions at window end. Sessions persisted in `app_sessions` (token hash, user, csrf token, created, last_seen, expires, ip, user agent) so restarts keep users logged in; a janitor deletes expired rows. Rate limiter: in-memory sliding window keyed by IP and by username.

### Importer
Separate `*sql.DB` to the source (MySQL/MariaDB or PostgreSQL Guacamole schema) inside a `READ ONLY` transaction, `SELECT` only. Mapping: `guacamole_entity`+`guacamole_user` → users (hash/salt kept with scheme `guac_sha256`; `expired` → `must_change_password`; validity, access window, timezone preserved), `guacamole_user_group` → user_groups (disabled kept), memberships flattened through nested groups skipping any path through a disabled group, `guacamole_connection_group` → connection_groups (balancing → organizational), connections + parameters (secrets via registry), connection READ and system permissions. Connection-group READ is *not* imported (upstream it does not grant children; ours is inheritable). Balancing-group READ becomes per-child READ only with `--accept-expansion`. After mapping, the importer computes each user's launchable set in source vs target and prints differences. Single target transaction; natural-key matching for idempotency.

### Frontend
Vite+ toolchain (package.json pinned like go-dash; `vite` aliased to `@voidzero-dev/vite-plus-core`: `vp dev`, `vp build`, `vp test` = Vitest, `vp lint` = Oxlint, `vp fmt`). Vue 3 `<script setup>` TS, Vue Router (guards from `/api/v1/me` permissions), Pinia stores (auth, prefs, connections), vue-i18n (legacy: false), `guacamole-common-js` 1.5 with a local `.d.ts`, Tailwind v4 via `@tailwindcss/vite`. Icons: `@lucide/vue` (tree-shaken SVG, matches mockup). API client: `fetch` wrapper adding `X-CSRF-Token`, JSON errors → `ApiError(code)`, 401 → redirect to login. Dev server proxies `/api` (with `ws: true`) to `:8080`.

### Themes
Each theme is a plug-in folder `frontend/src/themes/<id>/` with `theme.css` (tokens + decorations) and `index.ts` (`defineTheme({id,label,scheme,order,clock,animated,favicon})`); `themes/index.ts` discovers them with `import.meta.glob`, so adding a theme needs no other change (the server accepts any `[a-z0-9-]` theme id). Theme and language pickers are minimal icon buttons with popovers. UI scale: zoom 1.333 at ≥2400 CSS px (QHD) and 2 at ≥3400 (4K), never inside the remote client.
Tokens on `html[data-theme=<id>]`: `--bg`, `--surface`, `--surface-2`, `--sidebar`, `--sidebar-fg`, `--fg`, `--muted`, `--border`, `--accent`, `--accent-fg`, `--success`, `--danger`, `--warning`, `--font`, `--font-mono`, `--shadow`, `--sb-thumb`, `--sb-track`. Tailwind v4 `@theme inline` maps them to utilities (`bg-surface`, `text-muted`, `border-line`, …). `light` reproduces the mockup (white content, navy sidebar `#0f172a`, accent `#1d63ed`); `dark` its inverse. The 14 legacy themes take palettes from `base/guacamole/theme/tailwind.config.js` and the CSS files; signature extras in `frontend/src/themes/extras/*.css` (bevels for win98/xp, title-bar gradients, Matrix rain canvas component, xterm mono, contrast focus rings, Win98 clock in the sidebar footer). Global rule `border-radius: 0 !important` in every theme (mockup constraint); themes differ by palette, fonts, borders, shadows and decorations only. Favicons reused from `theme-switcher.js`.

### Docker / CI
Dockerfile: `node:24-bookworm-slim` (npm ci, `vp check`, typecheck, `vp test`, `vp build`) → `golang:1.27-alpine` (vet, test, `go build -trimpath -ldflags "-s -w -X ..."`) → `alpine:3.22` non-root (tzdata, `HEALTHCHECK goguacamole healthcheck`). Compose: `app`, `guacd` (internal only), named volume `app-data`; profiles `mariadb`, `postgres`, `lab` (+ `make up-tls` for native HTTPS) (`linuxserver/openssh-server`, a VNC server and an xrdp desktop image). E2E: Go test (build tag `e2e`) for the API suite across the three drivers; Playwright (Chromium, Firefox) for the browser suite. GitHub Actions: `ci.yml` (go vet/test incl. MariaDB/PostgreSQL service containers, golangci-lint, vp lint/test/build) on every push/PR; on `main`/tags an `e2e` job, then a `publish` job (needs e2e) using `docker/build-push-action` with `GITHUB_TOKEN` (`packages: write`) to `ghcr.io/jniltinho/go-guacamole`, buildx multi-arch, metadata-action tags.

## Risks / Trade-offs

- [We now own the security model formerly provided by Guacamole] → explicit authorization matrix, tests per row, security review checklist (`docs/security.md`), e2e permission/revocation tests.
- [`guacamole-common-js` tunnel expectations] → UUID-first, ping echo and batching verified with real guacd in e2e and a browser smoke test.
- [Process-local registry] → single-replica topology documented; interface allows Redis later.
- [Importer lossy cases] → dry-run with warnings; source never modified; old stack kept for rollback.
- [Theme fidelity with a new DOM] → palettes/fonts/decorations kept; screenshots per theme reviewed.
- [Vite+ maturity] → config is plain `vite.config.ts`; can fall back to `vite`/`vitest` CLIs.
- [SQLite write contention] → WAL + single writer; MariaDB/PostgreSQL available.
- [Three SQL dialects] → one query set with rebind + dialect hooks; the full repository suite runs on all three in CI.
- [No MFA] → cut-over gated on decision; P2.

## Migration Plan

1. Deploy GoGuacamole + guacd next to the existing stack (different port), SQLite or MariaDB.
2. `goguacamole import guacamole --source-driver mysql --dsn <old> --dry-run`, review warnings and access differences, then run for real.
3. Pilot users validate SSH/RDP/VNC, permissions and admin.
4. Switch reverse proxy; keep the Java container stopped for rollback (old DB untouched).

## Open Questions

- MFA requirement before production cut-over?
- Should reachability probing be opt-in for large catalogs (default on, 60 s)?
