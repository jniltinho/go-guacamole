## ADDED Requirements

### Requirement: Read-only import from Guacamole
`goguacamole import guacamole --source-driver mysql|postgres --dsn <dsn> [--dry-run] [--conflict skip|update] [--accept-expansion]` SHALL read an official Apache Guacamole 1.x JDBC database (MySQL/MariaDB or PostgreSQL) inside a read-only transaction using only `SELECT` statements, and copy into the configured GoGuacamole database in a single transaction: users (existing hash/salt, `disabled`, `expired` → `must_change_password`, `valid_from`, `valid_until`, `access_window_start`, `access_window_end`, `timezone`, full name, e-mail), user groups (with `disabled`) and memberships, connection groups (tree), connections with all parameters (secrets encrypted per the registry), guacd host/port/encryption overrides, `max_connections`/`max_connections_per_user`, system permissions (`ADMINISTER`, `CREATE_CONNECTION`, `CREATE_USER`, `AUDIT`) and connection `READ` grants.

#### Scenario: Dry run
- **WHEN** the importer runs with `--dry-run`
- **THEN** it prints counts of objects that would be created, updated or skipped plus all warnings and writes nothing

#### Scenario: Imported user logs in
- **WHEN** user `guacadmin` imported with its Guacamole hash logs in with its existing password
- **THEN** login succeeds

#### Scenario: Restrictions preserved
- **WHEN** a source user has `expired = 1` and an access window 08:00–18:00 in `America/Sao_Paulo`
- **THEN** after import the user must change the password at first login and cannot log in outside that window

#### Scenario: Source untouched
- **WHEN** the import finishes
- **THEN** no row in the source database was inserted, updated or deleted

### Requirement: No silent access expansion
The importer SHALL preserve each user's effective set of launchable connections. Guacamole connection-group `READ` grants (which do not grant access to child connections upstream) SHALL NOT be imported as inheritable group grants; they are skipped and reported. Nested user groups SHALL be flattened such that a membership path through a disabled group grants nothing. `READ` on a `BALANCING` group (which upstream allows launching the group without READ on its children) SHALL be imported as READ on each child connection only with `--accept-expansion`; otherwise those grants are skipped and reported. The importer SHALL print, per user, any connection whose accessibility differs from the source.

#### Scenario: Folder READ not expanded
- **WHEN** the source grants `ops` READ on group `Servers` and READ on only 1 of its 3 connections
- **THEN** after import `ops` members can launch exactly that 1 connection

#### Scenario: Balancing without acceptance
- **WHEN** the source has a balancing group with READ for `bob` and the importer runs without `--accept-expansion`
- **THEN** the group is imported as organizational, bob gets no grant on its children and a warning lists the skipped grant

### Requirement: Lossy mappings are reported
Objects without equivalent (sharing profiles, unmapped attributes, object permissions other than `READ`, user/user-group permissions) SHALL be skipped with a per-category warning and counts; the importer exits 0 unless the target transaction fails.

#### Scenario: Sharing profiles
- **WHEN** the source has 2 sharing profiles
- **THEN** the importer prints `skipped 2 sharing profiles (not supported)`

### Requirement: Idempotent re-runs
Re-running the importer SHALL match objects by natural key (username, group name, connection path) and either skip (default) or update (`--conflict update`) them, never duplicating.

#### Scenario: Re-run
- **WHEN** the importer is executed twice with default options
- **THEN** the second run reports all objects as skipped
