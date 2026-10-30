# Changelog

All notable changes to this project are documented here. The version is the git tag.

## [Unreleased]

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

### 🔧 Improvements
- security: changing a connection's protocol or guacd endpoint requires ADMINISTER; recording parameters can no longer
  be typed in connection parameters (legacy ones are converted to overrides)
- build: Go 1.27.3; `make docker-binary` / `make docker-test` build and test without local Go or Node.js

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
