## ADDED Requirements

### Requirement: Dashboard
`GET /api/v1/dashboard` and the `/dashboard` screen SHALL show, computed from real data only (no fabricated trends): count of connections visible to the user, active sessions (own, or all with `AUDIT`), and for `ADMINISTER`/`AUDIT` users the counts of users and user groups; the user's available connections table (first 10); recent activity (own events, or all with `AUDIT`); active sessions; and for administrators a permission overview (users and groups with permission count and inherited-from groups).

#### Scenario: Regular user dashboard
- **WHEN** a non-admin opens the dashboard
- **THEN** user/group counts and the permission overview are not returned nor rendered

### Requirement: Connection catalog
`GET /api/v1/connections` SHALL return only connections the user can READ, with id, name, protocol, hostname (no other parameters), group path, favorite flag and reachability status, supporting search (`q` on name/hostname), filters (`protocol`, `groupId`, `favorite`), sorting and pagination. `GET /api/v1/connection-groups/tree` SHALL return the visible tree (groups containing at least one visible connection).

#### Scenario: Search
- **WHEN** a user searches `q=srv-db`
- **THEN** only visible connections whose name or hostname contain `srv-db` (case-insensitive) are returned

#### Scenario: No secrets
- **WHEN** any user lists connections
- **THEN** no secret parameter values appear in the response

### Requirement: Favorites
Users SHALL add/remove favorites via `POST /api/v1/favorites` / `DELETE /api/v1/favorites/{connectionId}`; favorites of connections the user can no longer READ SHALL be hidden.

#### Scenario: Favorite persists
- **WHEN** a user stars `SRV-LINUX-01` and logs in from another browser
- **THEN** it is still marked as favorite

### Requirement: Reachability status
The backend SHALL periodically (default every 60 s, configurable, can be disabled) test TCP reachability of each connection's `hostname:port` with a 2 s timeout and report `online`, `offline` or `unknown`; the probe runs server-side only and is never triggered directly by user input.

#### Scenario: Offline target
- **WHEN** a VNC connection's host does not accept TCP on its port
- **THEN** it is listed with status `offline`
