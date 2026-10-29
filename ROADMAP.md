# Roadmap

What is still missing or worth improving after **v0.1.0**. Items are grouped by priority; each one says *why*
it matters. Open an issue (or an OpenSpec change under `openspec/changes/`) before starting the larger ones.

## Before replacing the Java client in production (P0)

- [x] **MFA / SSO.** TOTP, Duo, LDAP, OIDC, SAML, header and encrypted JSON (Administration → Authentication).
  Still missing: TOTP recovery codes, enforcement per group, and second factors after SSO sign-in.
- [ ] **Migration rehearsal with real data.** Run `goguacamole import guacamole --dry-run` against a *copy* of the
  production `guacamole_db`, review every `WARNING` / `ACCESS DIFF`, then pilot with a few users side by side with
  the Java client (different port) before switching the reverse proxy.
- [ ] **Secret key custody.** Document where `APP_SECRET_KEY` lives (vault / secret store) and test a restore of
  database + key on a clean host — without the key, stored connection passwords are unrecoverable.
- [ ] **Recording storage sizing.** Measure MB/hour for typical RDP and SSH sessions, size the `/record` volume and
  `RECORDING_RETENTION_DAYS`, and decide whether recordings must go to separate/immutable storage.
- [ ] **Archive the OpenSpec change** `add-goguacamole-panel` (`/opsx:archive`) once the pilot is accepted, so the
  specs move to `openspec/specs/` as the baseline for future changes.

## Features (P1)

- [x] **File transfer UI** (SFTP / RDP drive): upload by drag-and-drop and a file browser in the session menu —
  guacd already supports it via `enable-sftp` / `enable-drive`, only the client UI is missing.
- [ ] **On-screen keyboard** and touch gestures panel for tablets (`Guacamole.OnScreenKeyboard`).
- [ ] **Session sharing** (read-only / collaborative join of an active tunnel by an admin or a second user).
- [x] **Per-connection and per-user history tabs** in the edit screens (the data and API filters already exist).
- [ ] **Recording search**: search across all transcripts (e.g. "who ran `rm -rf` this week?"), with an index.
- [ ] **Notifications**: alert auditors on configurable events (login denied bursts, sessions to sensitive
  servers, transcripts matching patterns) via webhook / e-mail.
- [ ] **Bulk import** of connections from CSV and MobaXterm (`.mobaconf`), replacing `import_moba.py`.
- [ ] **Balancing groups / failover** (pick the least-loaded child connection) — today they are imported as plain
  folders.
- [ ] **Nested user groups** (currently flat; the importer flattens Guacamole's nested groups).
- [ ] **Telnet and Kubernetes** protocols in the forms (guacd supports them; they need forms and e2e coverage).
- [ ] **More languages** (es, fr…): add `src/locales/<lang>.json`; the parity test already guards missing keys.
- [ ] **Light/dark auto-switch per theme** (several legacy themes had both variants: macOS, Ubuntu, KDE, Google).
- [ ] **Health of targets**: optional per-connection probe port/interval and a history of reachability.

## Operations and scale (P1)

- [ ] **Metrics**: Prometheus endpoint (`/metrics`) with logins, active tunnels, guacd errors, tunnel bytes and
  latency; Grafana dashboard example.
- [ ] **Horizontal scaling**: the tunnel registry, limits, kills and rate limiters are in-memory (single replica).
  Move them to Redis/PostgreSQL (with LISTEN/NOTIFY for revocation) to run several replicas.
- [ ] **Backup command**: `goguacamole backup` (consistent SQLite snapshot / DB dump + secret key check).
- [ ] **Packages**: `.deb` / `.rpm` with systemd unit (nfpm), like go-glpi-agent.
- [ ] **Helm chart / Kubernetes manifests**.
- [ ] **Audit export** (CSV / JSON, and syslog/SIEM forwarding).

## Security hardening (P1)

- [ ] **External security review / pentest** of the tunnel, importer and permission paths.
- [ ] **CSP without `style-src 'unsafe-inline'`** (Vue style bindings and guacamole-common-js set inline styles; needs
  nonces or a review of each usage).
- [ ] **Password policy options** (min length, complexity, history, max age — the Guacamole JDBC settings).
- [ ] **Login lockout** per account (beyond the current per-IP / per-username rate limit) with admin unlock.
- [ ] **Configurable cookie `SameSite=Strict`** and session binding to client IP / user agent (optional).
- [ ] **Signed / hashed recordings** (tamper evidence for audit trails).
- [ ] **Dependency and image scanning** in CI (govulncheck, npm audit, Trivy) and Dependabot/Renovate.

## Quality (P2)

- [ ] **Playwright suite in CI is slow** (pulls VNC/RDP desktop images): cache images or use lighter targets.
- [ ] **E2E for guacd TLS** (`GUACD_TLS=true` with a CA) and for native HTTPS (`make up-tls`).
- [ ] **Visual regression screenshots** for the 16 themes (like go-dash `make screenshots`).
- [ ] **Load test**: concurrent tunnels per CPU/RAM, backpressure, long-running RDP sessions.
- [ ] **Upstream**: report the `guacamole-common-js` 1.5.0 `SessionRecording(Blob)` bug (internal `recordingBlob`
  never assigned); the player uses `StaticHTTPTunnel` as a workaround.
- [ ] **Accessibility pass** (axe-core in Playwright, keyboard-only navigation of every screen).
- [ ] **Mobile layout** review of admin screens below 768 px.
