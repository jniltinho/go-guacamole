## ADDED Requirements

### Requirement: SPA embedded in the binary
The frontend SHALL be a Vue 3 + TypeScript SPA in `frontend/` using Vue Router, Pinia, vue-i18n and `guacamole-common-js`, built with Vite+ (`vp build`) and Tailwind CSS v4. The build output SHALL be embedded in the Go binary and served with history fallback (non-`/api` paths) and `Cache-Control: public, max-age=31536000, immutable` for hashed assets, `no-cache` for `index.html`.

#### Scenario: Deep link
- **WHEN** the browser loads `/connections` directly
- **THEN** `index.html` is returned and the router shows the connections screen

### Requirement: Application shell
The UI SHALL follow the GoGuacamole mockup: collapsible left sidebar (Dashboard, Connections, Sessions, Users, Groups, Connection groups, Permissions, Audit logs, Settings — items hidden when not permitted), topbar with global search, minimal icon-only theme and language pickers (popover menus) and user menu, breadcrumbs, dense tables, loading/empty/error/disabled states, responsive down to 360 px width, keyboard-accessible with visible focus, and **no rounded corners** on cards, buttons, inputs, modals and tables in every theme (themes change palette, fonts, borders and decorations, never `border-radius`).

#### Scenario: Hidden admin menu
- **WHEN** a user without `ADMINISTER` logs in
- **THEN** Users, Groups, Permissions items are not shown and navigating to `/users` shows the 403 page

### Requirement: Routes
The SPA SHALL provide `/login`, `/dashboard`, `/connections`, `/sessions`, `/sessions/:connectionId/client`, `/users`, `/users/:id`, `/groups`, `/groups/:id`, `/connection-groups`, `/permissions`, `/audit`, `/settings`, `/403`, `/404`.

#### Scenario: Unknown route
- **WHEN** the user opens `/does-not-exist`
- **THEN** the 404 page is shown

### Requirement: Login screen
The login screen SHALL handle invalid credentials, disabled/expired accounts, rate limiting and the forced password change flow with translated messages.

#### Scenario: Forced change
- **WHEN** login returns `PASSWORD_CHANGE_REQUIRED`
- **THEN** the form shows new password + confirmation and resubmits

### Requirement: Remote client view
The client view SHALL request a launch ticket, connect `Guacamole.Client` through `Guacamole.WebSocketTunnel`, scale the display to the container, send size on resize, capture keyboard only while focused, forward mouse and touch, support fullscreen, Ctrl+Alt+Del, clipboard sync in both directions, show connecting/connected/error/disconnected states with translated Guacamole status messages and a Reconnect action, and remove all listeners and disconnect on unmount.

#### Scenario: Error status
- **WHEN** the tunnel fails with `UPSTREAM_NOT_FOUND`
- **THEN** a translated message and a Reconnect button are shown

#### Scenario: Leaving the page
- **WHEN** the user navigates away from the client view
- **THEN** the client disconnects and no keyboard listener remains attached

### Requirement: Themes
The UI SHALL provide `light` and `dark` (default follows `prefers-color-scheme`) plus the 14 themes of the current Guacamole extension — `noc`, `dracula`, `bionexo`, `matrix`, `xterm`, `macmodern`, `ubuntu`, `kde`, `google`, `contrast`, `win11`, `xp`, `win98`, `macos` — implemented as `data-theme` on `<html>` driving CSS custom properties mapped to Tailwind v4 tokens, with signature extras (Matrix character rain, Windows 98/XP bevels and title bars, Win98 taskbar clock, monospace for xterm) and per-theme SVG favicons. The choice SHALL be persisted in the user's server-side preferences and cached in `localStorage` (`gg-theme`) for first paint. Decorations SHALL be disabled inside the remote client view.

#### Scenario: Theme persisted
- **WHEN** a user selects `dracula` and logs in from another browser
- **THEN** the UI renders with Dracula after login

#### Scenario: Rain stops in session
- **WHEN** the Matrix theme is active and a remote session opens
- **THEN** the rain animation stops and its canvas is removed

### Requirement: Internationalization
All UI strings SHALL come from catalogs `en` (source and fallback) and `pt-BR`, whose key sets are enforced equal by a unit test. Initial locale: saved preference, else browser language (`pt*` → `pt-BR`), else `en`. Dates, durations and numbers SHALL be formatted with the active locale. API error codes and Guacamole status codes SHALL map to translation keys.

#### Scenario: Portuguese browser
- **WHEN** a first-time visitor's browser language is `pt-BR`
- **THEN** the login screen is in Portuguese

#### Scenario: Catalog parity
- **WHEN** `vp test` runs
- **THEN** it fails if a key exists in only one catalog
