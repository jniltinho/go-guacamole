## ADDED Requirements

### Requirement: Guacamole protocol codec
The system SHALL encode and decode instructions of the form `LENGTH.VALUE(,LENGTH.VALUE)*;` where `LENGTH` counts Unicode code points, reading from guacd and from the browser with a bounded parser (max 64 KiB per element from the browser, 8 MiB per instruction from guacd) and never forwarding a partial instruction.

#### Scenario: Unicode length
- **WHEN** an element `ação` is encoded
- **THEN** it is written as `4.ação`

#### Scenario: Split TCP reads
- **WHEN** one guacd instruction arrives across three reads
- **THEN** it is relayed once, complete

#### Scenario: Oversized client instruction
- **WHEN** the browser sends an element above the limit
- **THEN** the tunnel is closed with status `CLIENT_BAD_REQUEST` (768)

### Requirement: guacd handshake
For each tunnel the backend SHALL dial guacd at the connection's guacd host/port override or `GUACD_ADDR`, using TLS when the connection's encryption is `SSL` or (when unset) when `GUACD_TLS=true`, verifying the certificate with `GUACD_TLS_CA` / system roots and `GUACD_TLS_SERVER_NAME` unless `GUACD_TLS_INSECURE=true`; it SHALL never silently fall back to plaintext. It SHALL send `select <protocol>`, read `args`, and reply in this order: `size` (width, height, dpi), `audio`, `video`, `image` (browser-declared mimetypes), `timezone`, then `connect` whose values follow the `args` order, where a `VERSION_x_y_z` arg is answered with the lower of the offered version and the highest version the backend supports (`VERSION_1_5_0`), stored parameters fill named args, and absent parameters are sent as empty strings. A guacd `error` during the handshake SHALL be relayed to the client; the handshake SHALL time out after 15 s.

#### Scenario: SSH ready
- **WHEN** an authorized user launches a reachable SSH connection
- **THEN** guacd answers `ready` and the relay starts

#### Scenario: Older guacd
- **WHEN** guacd offers `VERSION_1_3_0`
- **THEN** the backend answers `VERSION_1_3_0`

#### Scenario: TLS mismatch
- **WHEN** a connection requires `SSL` and guacd presents an untrusted certificate
- **THEN** the launch fails with status `UPSTREAM_ERROR` (515) and no plaintext attempt is made

#### Scenario: guacd down
- **WHEN** guacd is unreachable
- **THEN** the client receives status `UPSTREAM_UNAVAILABLE` (520)

### Requirement: Launch ticket and WebSocket endpoint
Launching SHALL be two-step. `POST /api/v1/sessions {connectionId}` authorizes the user (JSON errors: 404, 403, 409 `CONNECTION_LIMIT`, 429) and returns a single-use ticket valid 30 s bound to the session, user and connection. Session validity and effective READ SHALL be re-checked when the ticket is redeemed and again immediately before the relay is activated (after the guacd handshake); a revocation event arriving while a ticket is pending or a handshake is in progress SHALL invalidate it. `GET /api/v1/tunnel?ticket=...&width=&height=&dpi=&audio=&video=&image=&timezone=` upgrades to WebSocket with subprotocol `guacamole` (compatible with `Guacamole.WebSocketTunnel` from `guacamole-common-js` 1.5/1.6). Only display parameters are taken from the query string; connection parameters always come from the database. The server SHALL first send the internal instruction `0.,36.<tunnel-uuid>;`, echo internal `ping` instructions back without forwarding them to guacd, relay everything else, and when an error happens after upgrade send an `error` instruction with the Guacamole status code and then close the WebSocket with the close code mapped from that status (1008 for CLIENT_* statuses, 1011 for SERVER/UPSTREAM statuses, 1000 for normal closure) and the decimal Guacamole status as close reason, which `Guacamole.WebSocketTunnel` parses into `Guacamole.Status`.

#### Scenario: Close reason
- **WHEN** guacd is unreachable after a successful upgrade
- **THEN** the browser receives `error` with status 520 (asserted via `Guacamole.Client.onerror` in one test) and a close frame with code 1011 and reason `520` (asserted via `Guacamole.WebSocketTunnel.onerror` / `onstatechange` in a separate tunnel-only test)

#### Scenario: Revoked during handshake
- **WHEN** the user's READ is revoked while the guacd handshake is in progress
- **THEN** the relay is never activated and the browser receives status 771

#### Scenario: Reused ticket
- **WHEN** a ticket is used a second time
- **THEN** the upgrade is refused with 403 and no guacd dial happens

#### Scenario: Injected parameters ignored
- **WHEN** the query string contains `hostname=evil` or `password=x`
- **THEN** those values are ignored

#### Scenario: Browser closes tab
- **WHEN** the WebSocket closes
- **THEN** the guacd socket is closed, the registry entry removed and history finalized

### Requirement: Backpressure and timeouts
The relay SHALL use bounded buffers, write deadlines (10 s) on both sides and close the tunnel when the browser cannot keep up or no data is received from guacd for 30 s.

#### Scenario: Stalled browser
- **WHEN** the browser stops reading for longer than the write deadline
- **THEN** the tunnel is closed and resources released

### Requirement: Concurrency limits
Per-connection `max_connections` and `max_connections_per_user` (NULL or 0 = unlimited) SHALL be enforced by atomic reservation in the process-local tunnel registry at launch time; refused launches return 409 `CONNECTION_LIMIT` (status `CLIENT_TOO_MANY`, 797 on the wire). Limits are scoped to the single GoGuacamole instance.

#### Scenario: Per-user limit
- **WHEN** `max_connections_per_user=1` and two launches race for the same user
- **THEN** exactly one succeeds

### Requirement: Connection history
Each established tunnel SHALL write a `connection_history` row (user, connection id and name, protocol, client IP, start) and set the end time and close reason when it ends.

#### Scenario: History row
- **WHEN** an SSH session is opened and closed
- **THEN** a history row has both start and end time and reason `client_closed`
