## ADDED Requirements

### Requirement: Password hashing
New and changed passwords SHALL be stored with bcrypt (cost ≥ 12) and have at least 8 characters and at most 72 bytes in UTF-8 (bcrypt's input limit); longer passwords are rejected with 422 `PASSWORD_TOO_LONG`. Imported Guacamole hashes (`SHA-256(password || UPPER_HEX(salt))`, or unsalted `SHA-256(password)`) SHALL be verified in constant time and re-hashed with bcrypt after a successful login; if the legacy password exceeds 72 bytes the legacy hash is kept and the user is flagged `must_change_password` instead. Every failed login (unknown user, wrong bcrypt password, wrong legacy password, disabled user) SHALL perform one bcrypt comparison so that response timing does not reveal account existence or hash scheme.

#### Scenario: Legacy hash upgrade
- **WHEN** an imported user logs in with the correct password
- **THEN** login succeeds and the stored hash becomes a bcrypt hash

#### Scenario: Long legacy password
- **WHEN** an imported user whose password is 80 bytes logs in correctly
- **THEN** the API returns 403 `PASSWORD_CHANGE_REQUIRED`, and the legacy hash remains valid until a new password is set

#### Scenario: Uniform failure
- **WHEN** a wrong password or unknown user is submitted
- **THEN** the API returns 401 `INVALID_CREDENTIALS` and a bcrypt comparison was performed in both cases

### Requirement: Login contract
`POST /api/v1/auth/login` SHALL accept `{username, password, newPassword?}`. Because no session exists yet, it SHALL be protected by requiring header `X-Requested-With: goguacamole` and a same-origin `Origin` (or `Sec-Fetch-Site: same-origin`). When a user is flagged `must_change_password`, a request without `newPassword` returns 403 `PASSWORD_CHANGE_REQUIRED` without a session; a retry with the same credentials plus a valid `newPassword` (different from the current one) updates the password, clears the flag and creates the session.

#### Scenario: Cross-site login
- **WHEN** a login POST arrives with `Origin: https://evil.example`
- **THEN** the API returns 403 `CSRF_INVALID`

#### Scenario: Forced password change
- **WHEN** a flagged user retries login with `newPassword`
- **THEN** the password changes, the flag clears and a session cookie is set

### Requirement: Bootstrap administrator
On startup with no users, the system SHALL create an administrator from `APP_ADMIN_USER`/`APP_ADMIN_PASSWORD`, or, if unset, user `admin` with a random password printed once to the log, flagged `must_change_password`.

#### Scenario: First start
- **WHEN** the app starts on an empty database without admin env vars
- **THEN** user `admin` exists and its one-time password is logged once

### Requirement: Account restrictions
Login and every authenticated request SHALL be refused for disabled users (403 `ACCOUNT_DISABLED`), for users outside `valid_from`/`valid_until` (403 `ACCOUNT_EXPIRED`) and for users outside their `access_window_start`/`access_window_end` evaluated in the user's `timezone` (default server timezone; windows may cross midnight) (403 `ACCESS_WINDOW`). Open sessions and tunnels SHALL be terminated by a scheduler when the access window ends or `valid_until` passes, and re-evaluated immediately whenever these restrictions are edited.

#### Scenario: Validity ends during session
- **WHEN** a user's `valid_until` is today and midnight passes in their timezone while a tunnel is open
- **THEN** the tunnel closes with status 771 and the session is revoked

#### Scenario: Window ends during session
- **WHEN** a user's access window ends at 18:00 while connected
- **THEN** at 18:00 the tunnel closes with status `SESSION_TIMEOUT` (522) and the session is revoked

### Requirement: Server-side sessions
A successful login SHALL create an `app_sessions` row keyed by the SHA-256 of a random 256-bit token, sent only as cookie `gg_session` (`HttpOnly`, `SameSite=Lax`, `Path=/`, `Secure` when served over HTTPS or `APP_COOKIE_SECURE=true`). Sessions SHALL expire after `APP_SESSION_IDLE_TIMEOUT` (default 30m) without activity or after the absolute `APP_SESSION_TTL` (default 8h). API calls and user input in an open tunnel count as activity; protocol keep-alives (`nop`, internal `ping`, `sync` acknowledgements) do not. No token SHALL be stored in `localStorage`, `sessionStorage`, HTML or logs.

#### Scenario: Absolute TTL closes tunnels
- **WHEN** a session reaches `APP_SESSION_TTL` with an open tunnel
- **THEN** the tunnel is closed with status `SESSION_TIMEOUT` (522) and further API calls return 401

#### Scenario: Logout
- **WHEN** `POST /api/v1/auth/logout` is called
- **THEN** the session row is deleted, the cookie is cleared and the session's tunnels are closed

### Requirement: Revocation
Disabling or deleting a user, or an administrator resetting their password, SHALL revoke all of that user's sessions, pending launch tickets and tunnels immediately. Loss of effective READ on a connection (see access-control events) SHALL close that user's open tunnels to it and invalidate pending tickets for it before the triggering API call returns.

#### Scenario: Revoke during session
- **WHEN** an admin removes the only permission giving `bob` access to `srv-db-01` while `bob` is connected
- **THEN** bob's tunnel to `srv-db-01` is closed with status `CLIENT_FORBIDDEN` (771) and new launches return 404

### Requirement: Request protection
Authenticated mutating requests SHALL require header `X-CSRF-Token` equal to the per-session token returned by `GET /api/v1/me`. WebSocket upgrades SHALL validate `Origin` against `APP_BASE_URL` and `APP_ALLOWED_ORIGINS`. Login SHALL be rate-limited per client IP and per username (10 failures / 5 min → 429 `TOO_MANY_ATTEMPTS`) and launches per user (30 / min). The client IP SHALL honor `X-Forwarded-For` only from `APP_TRUSTED_PROXIES`.

#### Scenario: Missing CSRF token
- **WHEN** a `POST /api/v1/connections` is sent without `X-CSRF-Token`
- **THEN** the API returns 403 `CSRF_INVALID`

#### Scenario: Foreign origin WebSocket
- **WHEN** a WebSocket upgrade arrives with `Origin: https://evil.example`
- **THEN** it is rejected with 403 before any guacd connection

### Requirement: Native HTTPS
With `TLS_ENABLED=true` the server SHALL serve HTTPS (and WSS) using `TLS_CERT_FILE`/`TLS_KEY_FILE` (PEM, full chain), minimum version `TLS_MIN_VERSION` (default `1.2`, `1.3` allowed) with Go's secure cipher defaults and HTTP/2; reload the certificate on SIGHUP without dropping connections; optionally listen on `TLS_REDIRECT_ADDR` (e.g. `:80`) redirecting HTTP to HTTPS; and, for labs only, generate an in-memory self-signed certificate when `TLS_SELF_SIGNED=true` (refused when `APP_ENV=production`). When TLS is terminated by a reverse proxy, `APP_COOKIE_SECURE=true` and `APP_TRUSTED_PROXIES` SHALL be used instead.

#### Scenario: HTTPS with certificate files
- **WHEN** `TLS_ENABLED=true` with valid cert and key files
- **THEN** `https://host:8443/health` answers 200, cookies carry `Secure` and the WebSocket works over `wss://`

#### Scenario: Old TLS rejected
- **WHEN** a client offers only TLS 1.1
- **THEN** the handshake fails

#### Scenario: Certificate rotation
- **WHEN** the certificate files are replaced and SIGHUP is sent
- **THEN** new connections present the new certificate and open tunnels stay connected

### Requirement: Security headers
Responses SHALL include a Content-Security-Policy (`default-src 'self'`, `connect-src 'self'` plus ws/wss of the same host, `img-src 'self' data: blob:`, `frame-ancestors 'none'`), `X-Content-Type-Options: nosniff`, `Referrer-Policy: same-origin`, and HSTS when served over HTTPS.

#### Scenario: CSP present
- **WHEN** `GET /` is requested
- **THEN** the response carries the CSP header
