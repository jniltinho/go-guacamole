## ADDED Requirements

### Requirement: Database drivers
The application SHALL support `DB_DRIVER=sqlite` (default, pure Go, `DB_DSN` is a file path), `DB_DRIVER=mysql` (MariaDB ≥ 10.6 / MySQL ≥ 8.0, go-sql-driver DSN) and `DB_DRIVER=postgres` (PostgreSQL ≥ 13, pgx URL or keyword DSN) behind one repository layer. Server-side database TLS SHALL be configurable through the DSN (`tls=` for MySQL, `sslmode`/`sslrootcert` for PostgreSQL). Any other value SHALL fail startup listing supported drivers.

#### Scenario: SQLite default
- **WHEN** `serve` runs without DB settings
- **THEN** it creates `./data/goguacamole.db` with foreign keys on, WAL journal and busy timeout set

#### Scenario: MariaDB
- **WHEN** `DB_DRIVER=mysql` and a reachable DSN are set
- **THEN** migrations run and the app serves requests

#### Scenario: PostgreSQL with TLS
- **WHEN** `DB_DRIVER=postgres` and `DB_DSN=postgres://gg:pw@db:5432/gg?sslmode=verify-full&sslrootcert=/certs/ca.pem`
- **THEN** the app connects over verified TLS and serves requests

#### Scenario: Unsupported driver
- **WHEN** `DB_DRIVER=oracle`
- **THEN** startup fails with `unsupported DB_DRIVER "oracle" (supported: sqlite, mysql, postgres)`

### Requirement: Application schema
The schema SHALL contain at least: `users` (incl. disabled, must_change_password, valid_from, valid_until, access_window_start, access_window_end, timezone), `user_groups` (incl. disabled), `user_group_members`, `connection_groups` (tree via `parent_id`), `connections`, `connection_parameters`, `system_permissions`, `connection_permissions`, `connection_group_permissions`, `favorites`, `app_sessions`, `preferences`, `connection_history`, `audit_events`, `schema_migrations`. Names SHALL be unique per parent for connections and connection groups. Deleting a user, connection or group SHALL cascade to grants, members, favorites, sessions and parameters, but SHALL NOT delete `connection_history` or `audit_events`: those keep immutable snapshots (username, connection name, protocol, target name) and their references become NULL.

#### Scenario: Duplicate connection name
- **WHEN** two connections with the same name are created in the same group
- **THEN** the second insert fails and the API returns 409 `NAME_CONFLICT`

#### Scenario: History survives deletion
- **WHEN** a connection with history rows is deleted
- **THEN** its history rows remain with `connection_id = NULL` and the original connection name

### Requirement: Versioned migrations for every driver
Each schema change SHALL ship as an up/down migration pair rendered for each driver (a single SQL template with per-dialect type tokens), embedded in the binary, applied in a transaction where the driver supports DDL transactions, and recorded in `schema_migrations`.

#### Scenario: Same tests on all drivers
- **WHEN** the repository test suite runs with SQLite, MariaDB (`TEST_MYSQL_DSN`) and PostgreSQL (`TEST_POSTGRES_DSN`)
- **THEN** all tests pass on all three, and CI runs all three

### Requirement: Secret parameter registry
A single versioned registry in code SHALL declare, per protocol, which parameters are secret; it SHALL include at least `password`, `passphrase`, `private-key`, `gateway-password`, `sftp-password`, `sftp-passphrase`, `sftp-private-key`, `client-key`, `client-cert` and `ca-cert` where the protocol defines them. The importer, encryption at rest, API masking and UI forms SHALL all use this registry, and a test SHALL enumerate every guacd 1.6 credential parameter of the supported protocols against it.

#### Scenario: Gateway password masked
- **WHEN** an admin reads an RDP connection with `gateway-password` set
- **THEN** the API returns `{"set": true}` for it, never the value

### Requirement: Secrets at rest
Secret parameters SHALL be encrypted with AES-256-GCM using a key derived (HKDF-SHA256) from `APP_SECRET_KEY` (or the content of `APP_SECRET_KEY_FILE`, generated on first start when neither is set); startup SHALL fail if encrypted values exist and the key is missing or wrong.

#### Scenario: Ciphertext in DB
- **WHEN** a connection is saved with `password=Pa55`
- **THEN** the stored value is not `Pa55` and the tunnel handshake still sends `Pa55` to guacd

### Requirement: Audit retention
`audit_events` older than `AUDIT_RETENTION_DAYS` (default 90) SHALL be purged by a daily background job.

#### Scenario: Purge
- **WHEN** the job runs and an event is 91 days old with retention 90
- **THEN** the event is deleted
