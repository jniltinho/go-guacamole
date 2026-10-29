## ADDED Requirements

### Requirement: Connection administration
Authorized users SHALL create, read, update and delete connections via `/api/v1/connections` with protocol `ssh`, `rdp` or `vnc`, parent connection group, optional guacd host/port/encryption override, concurrency limits and protocol parameters validated against a per-protocol schema (required hostname, numeric port, enum values). Secret parameters SHALL be write-only: reads return `{"set": true}` and an omitted secret on update keeps the current value; an explicit `null` clears it.

#### Scenario: Create RDP connection
- **WHEN** an admin creates `SRV-WINDOWS-02` with protocol `rdp`, hostname `192.168.10.20`, `security=nla`, `ignore-cert=true`
- **THEN** it is stored and appears for users with READ

#### Scenario: Invalid port
- **WHEN** port `abc` is submitted
- **THEN** the API returns 422 `VALIDATION_FAILED` with the field name

### Requirement: Connection group administration
Authorized users SHALL create, rename, move and delete connection groups via `/api/v1/connection-groups`; moving a group under itself or a descendant SHALL return 400 `GROUP_CYCLE`; deleting a non-empty group SHALL require `?recursive=true` and `ADMINISTER`.

#### Scenario: Cycle
- **WHEN** group `A` is moved under its child `B`
- **THEN** the API returns 400 `GROUP_CYCLE`

### Requirement: User and user group administration
Administrators SHALL list (search, paginate), create, update (full name, e-mail, disabled, validity dates, must-change-password), reset password and delete users via `/api/v1/users`, and create, rename, enable/disable, delete user groups and manage members via `/api/v1/user-groups`.

#### Scenario: Reset password revokes sessions
- **WHEN** an admin resets bob's password
- **THEN** bob's sessions and tunnels are terminated and bob must log in again

### Requirement: Permission management
`GET /api/v1/permissions?subject=user:{id}|group:{id}` SHALL return direct grants and, for users, effective permissions with their source; `PUT /api/v1/permissions` SHALL atomically set system permissions and connection / connection-group READ grants for a subject.

#### Scenario: Inherited source
- **WHEN** an admin views alice's permissions and alice is in `Developers`
- **THEN** connections granted to `Developers` show `inherited: Developers`

### Requirement: Preferences and history APIs
`GET|PUT /api/v1/me/preferences` SHALL read/write the caller's `theme`, `locale` and `sidebarCollapsed`; `PUT /api/v1/me/password` SHALL change the caller's password given the current one (403 `INVALID_CREDENTIALS` if wrong) and revoke the caller's other sessions. `GET /api/v1/history` SHALL list connection history (own rows for regular users, all for `AUDIT`/`ADMINISTER`) with filters (user, connection, protocol, date range) and pagination.

#### Scenario: Preference round trip
- **WHEN** a user saves `{theme: "win98", locale: "pt-BR"}`
- **THEN** a subsequent `GET /api/v1/me/preferences` returns the same values

#### Scenario: History scope
- **WHEN** a regular user lists `/api/v1/history`
- **THEN** only their own sessions are returned

### Requirement: Active sessions
`GET /api/v1/sessions` SHALL list active tunnels (user, connection, protocol, client IP, start, duration) — own for regular users, all for `AUDIT`/`ADMINISTER` — and `DELETE /api/v1/sessions/{id}` SHALL terminate a tunnel (own, or any for `ADMINISTER`).

#### Scenario: Admin kill
- **WHEN** an admin terminates charlie's tunnel
- **THEN** charlie's client shows "session terminated by an administrator" and an audit event is written

### Requirement: Audit log
Security-relevant actions SHALL be recorded in `audit_events` (time, actor, client IP, action, target type/id/name, outcome, non-sensitive metadata): login success/failure/denied, logout, session expiry, tunnel open/close/kill, and every create/update/delete of users, groups, connections, connection groups and permissions. Passwords, tokens, tickets, clipboard and parameter values SHALL never be recorded. `GET /api/v1/audit` SHALL support filters (actor, action, target, date range) and pagination for `AUDIT`/`ADMINISTER`.

#### Scenario: No secrets in audit
- **WHEN** an admin changes a connection's password parameter
- **THEN** the audit event lists `parameters.password` as changed without its value

### Requirement: Destructive confirmations
The UI SHALL require explicit confirmation (typing the object name for recursive deletes) before deleting users, groups, connections or connection groups, or terminating sessions.

#### Scenario: Recursive delete confirmation
- **WHEN** an admin deletes a connection group containing connections
- **THEN** the dialog requires typing the group name before enabling the delete button

### Requirement: Server access report
Users with `AUDIT` SHALL see, on the dashboard and in Audit → Server access, who accessed which server and for how long: a recent-access log (user, server, protocol, start, end, time spent — open sessions counted until now —, client IP, recordings) with search and period filters, and aggregated time per server and per user (`GET /api/v1/history/stats?days=N`: sessions, total time, distinct users/servers, last access, currently connected).

#### Scenario: Time per server
- **WHEN** alice spent 10 min and bob 5 min on `web` in the last 30 days
- **THEN** the report lists `web` with 15 min total, 2 sessions and 2 users

#### Scenario: Not an auditor
- **WHEN** a user without `AUDIT` calls `/api/v1/history/stats`
- **THEN** the API returns 403 and the dashboard does not show the panel
