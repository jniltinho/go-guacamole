# Changelog

All notable changes to this project are documented here. The version is the git tag.

## [Unreleased]

## [0.4.1] — 2026-10-30

### 🔧 Improvements
- style: dashboard — the *Server access* tabs and the period select stay on one line with the monospace themes
  (xterm, matrix), as in the other themes

## [0.4.0] — 2026-10-30

**Your own application name, AD Manager-style tabs on every admin page, and a complete new-user form in a modal with a
strong password generator.**

### ✨ New Features
- feat: **Settings → Application name** — the name shown in the sidebar, the browser tab and the sign-in page
- feat: new user in a **modal** with the Java client's fields — account (username, password, disabled), profile (full
  name, e-mail, **organization** and **role**, new columns in migration 0009, also read by `import`) and restrictions
  (must change password, access window with time zone, valid from/until); **Create** stays disabled until the basics
  are valid, and the server applies the same rules (username characters, password 8 characters to 72 bytes, e-mail
  format, both window ends or neither, date order, valid time zone)
- feat: **password and confirmation** with show/hide, a strong password generator (CSPRNG, unbiased) and copy — in the
  new-user modal, the user's password reset and Settings → Security when changing your own password
- feat: new and edited **connection folders** in a modal; a folder can't be moved under itself or its subfolders
- feat: theme **dialogs and toasts** replace the browser's confirm/alert/prompt (AD Manager style)

### 🔧 Improvements
- style: AD Manager-style tabs on Settings, Authentication, Recording, Audit, Permissions and the user and group pages;
  the selected tab opens into the card, and Save sits right below the tab content
- style: one page width for every admin page; Settings shows the themes in full rows (4 or 8 columns)
- style: dialogs open 5% below the top of the window
- docs: concise README with a cover and diagrams; [docs/README.md](docs/README.md) with the details
- test(e2e): the storage test waits for guacd to finish the recording before ageing it (flaky in CI)

## [0.3.0] — 2026-10-30

**Background jobs on Redis with the asynqmon dashboard, session recordings in object storage (S3, OCI, GCS), an
all-in-one image, and a smoother XP-default UI.**

### ✨ New Features
- feat: background jobs on [asynq](https://github.com/hibiken/asynq) (Redis): retention purges, storage usage,
  expired web sessions and storage reconciliation run as periodic jobs with a database lease per period (one instance
  runs each, even with replicas or an emptied Redis); without `REDIS_ADDR` they run in-process as before; `/ready`
  reports the queue
- feat: **asynqmon** dashboard at `/asynqmon` (Jobs in the sidebar) for administrators only — same-origin checks on
  changes, audited, XP theme
- feat: recordings in **object storage** — AWS S3, S3-compatible (MinIO, Ceph, R2, Wasabi…), Oracle Cloud (S3 API or
  native) and Google Cloud Storage through rclone: **Test access** (required before saving a new target; reports
  versioning and object lock), verified uploads (SHA-256 read-back), optional removal of the local copy, **Upload
  existing recordings**, playback and transcripts straight from the bucket, retention deleting every object version,
  orphan sweep limited to the installation's namespace; history shows each recording's storage state and
  administrators can retry a failed upload
- feat: object storage connects through an in-process proxy that refuses private, loopback, link-local and metadata
  addresses unless allowed (`OBJSTORE_ALLOWED_ENDPOINTS`, `OBJSTORE_ALLOW_HTTP`); credentials are sealed, write-only
  and scrubbed from errors and logs
- feat: replica-safe sessions — instances send heartbeats and end the sessions of an instance that stopped
- feat: the Compose stack ships a `redis` service; a lab `s3` service (profile `objstore`)
- feat: all-in-one Docker stack in `docker/` — guacd, goguacamole and Redis in one container (supervisord on the
  official guacd image, `--target allinone`), everything local to the folder (SQLite and recordings in `./data`,
  MariaDB/PostgreSQL ready to uncomment with data in `./mariadb` / `./postgres`) and an nginx reverse-proxy config
  (TLS, WebSocket tunnel, request id); with `APP_TRUSTED_PROXIES` the access log, audit and history keep the client's
  IP, never nginx's, and forged `X-Forwarded-For` from elsewhere is ignored
- feat: **Administration → Recording** can hide the recording notice (connecting screen and session menu); on by
  default, sessions are still recorded when it is off, and the change is audited like every recording setting

### 🔧 Improvements
- style: tabs show a hover hint; larger initials in the topbar avatar; readable subtitle in the transcript and player
  headers; long pages keep a gap above the bottom edge
- test: `./deploy/e2e.sh storage` (and CI) — PostgreSQL + Redis + S3 with versioning, from upload to purge
- docs: [all-in-one guide](docs/all-in-one.md) (build with `make docker-allinone` or Buildx, options, replicas);
  README, API, architecture, security and operations cover the job queue and object storage; new screenshots
- style: Windows XP is the default theme, and the language follows the browser again (pt* → Português, else
  English) until the user picks one
- style: remote session — the REC mark moved out of the terminal into the session menu, discreetly under the
  connection name (the connecting screen still announces what is recorded); theme sidebar styles no longer repaint
  the session menu
- style: XP — light Bliss desktop (pale sky over the green hill, black page text) so the dark-blue bars and frames stand
  out; the current menu item uses the hover dark blue with white bold text and a green bar; stat cards keep a closed
  frame
- style: every panel title has the same height, with or without a button
- style: smoother page change, as in AD Manager — the page fades out, the next one fades in rising 6px (fade only
  with reduced effects, instant with effects off); every view animates through a keyed wrapper
- style: sidebar items run edge to edge (no inset), aligned with the logo; the footer shows only the release number,
  right-aligned (the app name is in the header), and the retro taskbar clock was removed
- style: collapsing the sidebar is a smooth drawer — the width eases, labels stay clipped and fade, the arrow turns;
  icons are centered in the collapsed bar

## [0.2.1] — 2026-10-30

### 🔧 Improvements
- style: sign-in page without theme and language buttons, release number shown discreetly in the bottom-right
  corner (`/auth/methods` now returns the version); NOC is the default theme and English the default language until
  the user picks one in Settings (the browser language is no longer used)

## [0.2.0] — 2026-10-30

**Sign-in methods and MFA, file transfer, per-connection recording policy with retention, premium AAA themes with
SSH terminal color schemes, and a theme QA suite in CI.**

### ✨ New Features
- feat: sign-in methods configured in **Administration → Authentication** (stored sealed in the database, secrets
  write-only, a *Test* button per method): TOTP second factor with QR enrollment and replay protection, LDAP /
  Active Directory, OpenID Connect (code + PKCE), SAML 2.0, HTTP header from trusted proxies, encrypted JSON
  (guacamole-auth-json compatible, single use) and Duo Universal Prompt; external users are provisioned on first
  sign-in and directory groups map to local user groups by name
- feat: file transfer in the session menu — SFTP / RDP drive browser, downloads, uploads to the current folder and
  drag-and-drop onto the screen
- feat: History panel in the connection and user edit screens (auditors)
- feat: every SSH host option of the Java client (color scheme, font, scrollback, command, locale, time zone,
  keepalive, backspace, terminal type) plus SFTP/RDP upload and download switches

- feat: recording policy per connection — video (SSH/RDP/VNC), SSH text and keystrokes as inherit/on/off with
  retention overrides (AUDIT or ADMINISTER), global defaults in **Administration → Recording**, fail-closed when
  recording is unavailable (time-limited audited exceptions), session closed when guacd does not record, user notice
  and REC badge, history-driven retention with server-side preview, legal hold, orphan purge and storage usage

- feat: premium themes (openspec `refine-themes-premium`) — shared design tokens with elevation and motion, hover
  lift / press / named transitions, skeletons, toasts, live-session pulse, an **Effects** preference (system, full,
  reduced, off) and WCAG 2.2 AAA contrast in all 16 themes, checked by token tests and an axe + keyboard Playwright
  suite (login pixel references and a live-session check in every effects mode, run by the CI e2e job in the
  pinned Playwright image); legacy XP/Win98 desktops darkened to carry white text
- feat: SSH terminal color schemes (Dracula, Solarized, Gruvbox, Nord, One Dark, Monokai, Tango, xterm, Campbell,
  Ubuntu, Breeze and palettes matching the UI themes) as a user preference with "follow UI theme" and live preview;
  a connection's own color scheme overrides it
- style: subtle rounded corners (token, ≤ 8px) in modern themes, retro themes stay square; Bionexo rebuilt on the brand
  (navy bars, teal highlights, light surfaces); "/" focuses the search (can be turned off); tabular digits in tables
- style: themes with structure and volume (after AD Manager): page tinted apart from the cards, visible panel frames
  and row dividers, panel title bands with an accent bar, table header band, striped rows, beveled buttons and icon
  tiles, layered card shadows; xterm, High contrast and XP redesigned; bold, high-contrast sidebar hover in every
  theme; all checked by token tests (structure + AAA on the new surfaces)
- style: Mac OS (retro) rebuilt as Platinum (Mac OS 8/9) on the Crystal structure of AD Manager
- style: one table rhythm everywhere (same padding and row height, slightly larger text); connection host shown as a
  tooltip; pencil icon for "Edit" in the dashboard; sidebar version on one line
- style: slimmer layout — thinner top bar and sidebar header, compact tables, panel titles and controls, narrower
  search

### 🔧 Improvements
- security: changing a connection's protocol or guacd endpoint requires ADMINISTER; recording parameters can no longer
  be typed in connection parameters (legacy ones are converted to overrides)
- build: Go 1.27.3; `make docker-binary` / `make docker-test` build and test without local Go or Node.js
- docs: README screenshots refreshed; `make screenshots` retakes them on a clean lab stack with demo data

## [0.1.0] — 2026-10-29

**First release: a Go replacement for the Apache Guacamole Java client — Echo v5 + Cobra backend talking to guacd
directly, Vue 3 / Vite+ / Tailwind v4 UI embedded in a single static binary.**

### ✨ New Features
- feat: native Guacamole protocol and guacd handshake (version negotiation, timezone, TLS to guacd) and a WebSocket
  tunnel compatible with guacamole-common-js — SSH, RDP and VNC
- feat: single-use launch tickets, re-authorization before activation, connection limits, connection history
- feat: SQLite (default), MariaDB/MySQL and PostgreSQL storage with embedded migrations
- feat: users, user groups, connection folders, connections and permissions (direct, group, inherited folder),
  active sessions, history and audit log; connection secrets encrypted at rest (AES-256-GCM)
- feat: login with Guacamole-compatible hashes (upgraded to bcrypt), forced password change, account validity and
  access windows, rate limiting, CSRF/Origin protection, revocation that closes live tunnels (any process)
- feat: read-only importer from an Apache Guacamole database (MySQL/PostgreSQL) that never widens access
- feat: session recording like the Java client — screen recordings played back in the browser and SSH text
  transcripts of every command and its output (AUDIT), with retention
- feat: server access report for auditors — who accessed which server and for how long, time per server and per
  user, on the dashboard and in Audit
- feat: dashboard, connection table/tree, favorites, reachability, remote client (clipboard, Ctrl+Alt+Del,
  fullscreen, resize), administration screens, refined settings
- feat: 16 plug-in themes (Light, Dark + 14 legacy Guacamole themes), English and Português (Brasil),
  full-height layout without page scrollbars, Full HD / QHD / 4K scaling
- feat: TOML + environment configuration, native HTTPS (TLS 1.2+, SIGHUP reload, HTTP redirect)
- feat: Cobra CLI — serve, migrate, user, import, config print, healthcheck, version

### 🔧 Improvements
- build: multi-stage Dockerfile (Alpine, non-root, healthcheck), docker-compose with MariaDB / PostgreSQL / lab
  profiles and a shared recordings volume, Makefile
- ci: tests on three databases, API + browser (Chromium/Firefox) e2e gate, linux/amd64 image on
  ghcr.io/jniltinho/go-guacamole, CHANGELOG-driven releases
- fix: findings of the Codex review (password-version session binding, importer privilege diff, strict
  restrictions, full Origin comparison, relay close frames)

### 📚 Documentation
- docs: README quick start, architecture, configuration, API, security and operations guides, ROADMAP,
  OpenSpec proposal
