## ADDED Requirements

### Requirement: Permission model
Permissions SHALL be granted to users or to user groups. System permissions are `ADMINISTER`, `CREATE_CONNECTION`, `CREATE_USER`, `AUDIT`. Object permission `READ` can be granted on a connection or on a connection group. `READ` on a connection allows seeing and launching it. `READ` on a connection group is **inheritable**: it grants `READ` on every connection and subgroup beneath it, including ones added later; the UI SHALL label such grants "includes all current and future connections". User groups are flat (no nested groups) and can be disabled; a disabled group grants nothing. Effective permissions are the union of direct grants and those of the user's enabled groups. `ADMINISTER` implies every permission.

#### Scenario: Group grant
- **WHEN** `alice` is a member of enabled group `ops` which has `READ` on connection group `Servers`
- **THEN** alice can see and launch every connection under `Servers`, with source shown as `inherited from ops via Servers`

#### Scenario: Disabled group
- **WHEN** group `ops` is disabled
- **THEN** its members lose the permissions granted only through `ops`

### Requirement: Authorization matrix
Every REST handler and tunnel launch SHALL check effective permissions server-side according to:

| Operation | Required |
|---|---|
| View/launch a connection | effective `READ` or `ADMINISTER` |
| Create connection / connection group | `CREATE_CONNECTION` or `ADMINISTER`; target parent must be visible |
| Edit / non-recursive delete of a connection or empty group | `ADMINISTER`, or `CREATE_CONNECTION` and creator of the object |
| Move connection or empty group | edit right on the object and `CREATE_CONNECTION` on the target parent |
| Move non-empty group | `ADMINISTER` only |
| Recursive delete of a non-empty group | `ADMINISTER` only |
| Read connection parameters (secrets masked) | same as edit |
| Create users | `CREATE_USER` or `ADMINISTER` |
| Edit/delete users, manage user groups and grants | `ADMINISTER` |
| View all active sessions, history and audit log | `AUDIT` or `ADMINISTER` |
| Kill another user's session | `ADMINISTER` |

Resources the caller cannot see SHALL return 404 `NOT_FOUND`; visible resources the caller cannot modify SHALL return 403 `FORBIDDEN`.

#### Scenario: Manual URL to forbidden connection
- **WHEN** a user without access calls `GET /api/v1/connections/17`
- **THEN** the API returns 404 `NOT_FOUND`

#### Scenario: Creator cannot recursively delete
- **WHEN** a `CREATE_CONNECTION` user who created group `Lab` tries `DELETE /api/v1/connection-groups/{Lab}?recursive=true` after an admin added connections to it
- **THEN** the API returns 403 `FORBIDDEN`

#### Scenario: Auditor
- **WHEN** a user with only `AUDIT` lists `/api/v1/audit`
- **THEN** the list is returned, but `DELETE /api/v1/sessions/{id}` for another user returns 403

### Requirement: Access-changing events
Any change that can reduce a user's effective access SHALL emit a revocation event processed synchronously: grant removal, membership removal, group disable/delete, connection or group move or delete, user disable/delete/password reset, and session revocation.

#### Scenario: Move out of granted folder
- **WHEN** an admin moves connection `srv-db-01` out of a group that was bob's only source of READ, while bob is connected
- **THEN** bob's tunnel to `srv-db-01` is closed with status `CLIENT_FORBIDDEN` (771)

### Requirement: Cross-process revocation
Changes made outside the running server process (CLI `user` commands, importer with `--conflict update`, another replica-less maintenance process) SHALL take effect on open sessions and tunnels within 10 seconds. Each user row SHALL carry a `security_stamp` changed on disable, delete, password change, restriction edits and membership changes, and a global `access_epoch` SHALL be incremented by any out-of-process change to grants, groups or the connection tree. The server SHALL poll both every 5 seconds and re-evaluate affected sessions, tickets and tunnels.

#### Scenario: CLI disable during tunnel
- **WHEN** `goguacamole user disable bob` runs while bob has an open tunnel
- **THEN** within 10 seconds bob's tunnel closes with status 771 and his session is revoked

### Requirement: Self-lockout prevention
An administrator SHALL NOT be able to delete, disable or remove `ADMINISTER` from their own account, and the system SHALL refuse any change that would leave zero enabled administrators.

#### Scenario: Last admin
- **WHEN** the only administrator tries to remove their `ADMINISTER` permission
- **THEN** the API returns 409 `LAST_ADMIN`
