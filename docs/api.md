# REST API (`/api/v1`)

JSON over HTTPS. Authentication is the `gg_session` cookie (HttpOnly). Every mutating request needs
`X-CSRF-Token: <csrfToken from GET /me>`; the login request needs `X-Requested-With: goguacamole` and a same-origin
`Origin`. Errors: `{"code": "...", "message": "...", "field": "..."}`; `code` is stable and translated by the UI.
Lists: `{"items": [...], "total": n}` with `limit` / `offset` query parameters. IDs are strings.

## Public

| Method | Path | |
|---|---|---|
| GET | `/health` | process alive |
| GET | `/ready` | `{"db":"up","guacd":"up"}` or 503 |
| POST | `/api/v1/auth/login` | `{username, password, newPassword?}` → me payload; 401 `INVALID_CREDENTIALS`, 403 `PASSWORD_CHANGE_REQUIRED` / `ACCOUNT_DISABLED` / `ACCOUNT_EXPIRED` / `ACCESS_WINDOW`, 429 `TOO_MANY_ATTEMPTS` |

## Session

| Method | Path | |
|---|---|---|
| POST | `/auth/logout` | ends the session and its tunnels |
| GET | `/me` | user, permissions, `isAdmin`, `csrfToken`, preferences, groups |
| PUT | `/me/password` | `{currentPassword, newPassword}`; ends the other sessions |
| GET/PUT | `/me/preferences` | `{theme, locale, sidebarCollapsed}` |

## Catalog

| Method | Path | Permission |
|---|---|---|
| GET | `/dashboard` | any (admin/audit see more) |
| GET | `/connections?q&protocol&groupId&favorite&sort&limit&offset` | READ (filtered) |
| GET | `/connections/:id` | READ; parameters (secrets masked as `{"set":true}`) only with edit rights |
| POST | `/connections` | `CREATE_CONNECTION` |
| PUT | `/connections/:id` | creator with `CREATE_CONNECTION`, or `ADMINISTER`; omitted secret = keep, `null` = clear |
| DELETE | `/connections/:id` | same as PUT |
| GET | `/connection-groups` · `/connection-groups/tree` | visible folders / visible tree |
| POST · PUT · DELETE | `/connection-groups[/:id]` | `DELETE ?recursive=true` for non-empty folders (`ADMINISTER`) |
| GET · POST · DELETE | `/favorites[/:connectionId]` | own |

## Sessions

| Method | Path | |
|---|---|---|
| POST | `/sessions` | `{connectionId}` → `{ticket, tunnelId, expiresAt}`; 404, 409 `CONNECTION_LIMIT`, 429 |
| GET | `/tunnel?ticket=…&width&height&dpi&audio&video&image&timezone` | WebSocket, subprotocol `guacamole` |
| GET | `/sessions` | active tunnels (own, or all with `AUDIT`) |
| DELETE | `/sessions/:id` | own, or any with `ADMINISTER` |
| GET | `/history?q&userId&connectionId&protocol&from&to` | own, or all with `AUDIT`; rows carry `hasRecording` / `hasTypescript` |
| GET | `/history/:id/recording` | screen recording (Guacamole protocol dump) for `Guacamole.SessionRecording`; `AUDIT` |
| GET | `/history/:id/typescript[?format=raw]` | SSH transcript as clean text, or the raw typescript; `AUDIT` |

## Administration (`ADMINISTER` unless noted)

| Method | Path | |
|---|---|---|
| GET · POST | `/users` | POST also with `CREATE_USER` |
| GET · PUT · DELETE | `/users/:id` | cannot disable/delete yourself; `LAST_ADMIN` protection |
| PUT | `/users/:id/password` | `{password, mustChangePassword}`; ends the user's sessions |
| GET · POST | `/user-groups` | |
| GET · PUT · DELETE | `/user-groups/:id` | |
| PUT | `/user-groups/:id/members` | `{members: [userId…]}` |
| GET | `/permissions?subject=user:<id>\|group:<id>` | direct grants + (users) effective access with sources; `AUDIT` can read |
| PUT | `/permissions` | `{subject, system[], connections[], connectionGroups[]}` (atomic replace) |
| GET | `/audit?actor&action&targetType&targetId&from&to` | `AUDIT` |

## Guacamole status codes on the tunnel

Errors after the WebSocket upgrade are sent as an `error` instruction and a close frame whose reason is the decimal
status (1008 for client errors, 1011 for server/upstream): 512 server error, 514 upstream timeout, 515 upstream
error, 519 upstream not found, 520 guacd unavailable, 522 session timeout, 523 session closed, 768 bad request,
771 forbidden (access revoked), 797 too many connections.
