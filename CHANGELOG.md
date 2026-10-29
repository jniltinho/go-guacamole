# Changelog

All notable changes to this project are documented here. The version is the git tag.

## [Unreleased]

—

## [0.1.0] — 2026-10-29

**First release: a Go replacement for the Apache Guacamole Java client — Echo v5 + Cobra backend talking to
guacd directly, Vue 3 / Vite+ / Tailwind v4 UI embedded in a single static binary.**

### ✨ New Features
- feat: native Guacamole protocol + guacd handshake (version negotiation, timezone, TLS) and WebSocket tunnel
  compatible with guacamole-common-js (SSH, RDP, VNC)
- feat: single-use launch tickets, re-authorization before activation, connection limits, history
- feat: SQLite (default), MariaDB/MySQL and PostgreSQL storage with embedded migrations
- feat: users, user groups, connection folders, connections, permissions (direct, group, inherited folder),
  active sessions, history and audit log; secrets encrypted at rest (AES-256-GCM)
- feat: login with Guacamole-compatible hashes (upgraded to bcrypt), forced password change, account validity and
  access windows, rate limiting, CSRF/Origin protection, revocation that closes live tunnels
- feat: read-only importer from an Apache Guacamole database (`goguacamole import guacamole`) that never widens access
- feat: dashboard, connection table/tree, favorites, reachability, remote client (clipboard, Ctrl+Alt+Del,
  fullscreen, resize), administration screens
- feat: session recording — screen playback in the browser and SSH text transcripts (AUDIT), retention
- feat: 16 plug-in themes (Light, Dark + 14 legacy Guacamole themes), English and Português (Brasil),
  Full HD / QHD / 4K scaling
- feat: TOML + environment configuration, native HTTPS (TLS 1.2+, SIGHUP reload, HTTP redirect)
- feat: Cobra CLI: serve, migrate, user, import, config print, healthcheck, version

### 🔧 Improvements
- build: multi-stage Dockerfile (Alpine, non-root, healthcheck), docker-compose with MariaDB / PostgreSQL / lab
  profiles, Makefile
- ci: CI on three databases, e2e gate and linux/amd64 image on ghcr.io/jniltinho/go-guacamole, CHANGELOG-driven releases

### 📚 Documentation
- docs: README quick start, architecture, configuration, API, security and operations guides; OpenSpec proposal
