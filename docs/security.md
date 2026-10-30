# Security

## Model

GoGuacamole owns authentication, authorization and auditing; `guacd` must only be reachable from GoGuacamole
(private network, never published). Run one replica behind HTTPS.

## Controls

| Area | Control |
|---|---|
| Passwords | bcrypt cost 12, 8–72 bytes; imported Guacamole SHA-256 hashes verified in constant time and upgraded; uniform bcrypt work on every failed login |
| Sessions | random 256-bit token in an `HttpOnly`, `SameSite=Lax` (and `Secure`) cookie; only its SHA-256 is stored; idle 30 min / absolute 8 h; nothing in `localStorage` |
| CSRF | per-session token header on mutations; login requires `X-Requested-With` + same origin |
| WebSocket | Origin check before upgrade, single-use 30 s ticket bound to session/user/connection, re-authorization at redemption and before activation |
| Authorization | effective permissions on every request; 404 for invisible objects (no IDOR enumeration); self-lockout and last-admin protection |
| Revocation | access loss closes live tunnels synchronously (in-process) or within 5 s (CLI/importer, through security stamps) |
| Rate limits | 10 failed logins / 5 min per IP and per username; 30 launches / min per user |
| Secrets | secret parameters (registry in `internal/secrets`) encrypted with AES-256-GCM (HKDF from `APP_SECRET_KEY`); write-only in the API; never logged or audited |
| Tunnel input | bounded parser (64 KiB per element, 1 MiB per message), write deadlines, idle timeout; browser cannot set connection parameters |
| Transport | native TLS ≥ 1.2 (HTTP/2, SIGHUP reload, optional HTTP→HTTPS redirect), HSTS; TLS to guacd with CA/SNI verification and no plaintext fallback; DB TLS through the DSN |
| Headers | CSP `default-src 'self'` (no inline scripts), `frame-ancestors 'none'`, `nosniff`, `Referrer-Policy: same-origin` |
| Container | non-root user, read-only root filesystem, `cap_drop: ALL`, `no-new-privileges`, resource limits |
| Audit | logins (success/failure/denied), logout, tunnels (open/close/kill), every admin change; retention configurable |

## Session recording and data protection (LGPD)

GoGuacamole provides the controls; the data controller defines and documents purpose, legal basis, the notice given to
users, retention per kind (and periodic review of keep-forever), who holds `AUDIT`, and how backups and restores treat
recordings and history. Points to consider:

- Users are told which recordings are active before and during each session (informational notice, not consent).
- Keystroke capture can record typed secrets that never appear on screen; enable it only where justified.
- Transcripts include whatever the user typed in the terminal, passwords included.
- Hold reasons may reveal investigations: only `AUDIT`/`ADMINISTER` see them.
- History metadata (user, IP, times) has no retention of its own yet; recordings and audit events do.

## Production checklist

- `APP_ENV=production`, `APP_SECRET_KEY` from a secret store (back it up: without it, stored connection secrets are lost).
- HTTPS end to end (`TLS_ENABLED=true`, or a proxy + `APP_COOKIE_SECURE=true` + `APP_TRUSTED_PROXIES`).
- `APP_BASE_URL` set to the public URL.
- guacd on an isolated network; consider `GUACD_TLS=true`.
- Back up the database and the secret key together; test restores.
- MFA/SSO is not built in yet — put the panel behind your SSO proxy if policy requires MFA.

## Reporting

Please open a private security advisory on GitHub.
