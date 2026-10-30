# Proposal

## Why

The 16 themes are palettes over a flat layout: each `theme.css` sets ~25 color tokens and a few decorations, there is
no shared notion of depth, motion or interaction feedback, and contrast is only checked by hand against WCAG AA. For
a product that replaces the Java client in front of operators all day (NOC screens, auditors), the interface should
feel premium and consistent in every theme, and be measurably accessible (WCAG 2.2 AAA text contrast) instead of
"looks fine on my screen".

## What Changes

- A shared **design-token layer** for every theme: elevation scale (layered shadows + inner highlight, no rounded
  corners), surface layers (background → surface → raised → overlay), borders, focus ring and **motion tokens**
  (durations, easings) consumed by all components.
- **Volume**: cards, panels, sidebar, topbar, menus, modals and tables get depth from the elevation scale; hover and
  active states lift/press; overlays sit on a dimmed, optionally blurred backdrop.
- **Effects and micro-interactions**: route and panel transitions, popover/modal enter/exit, button press, focus ring
  animation, skeleton loaders for lists, toast notifications, live-session pulse — all CSS (transform/opacity only).
- **Per-theme signature effects** refined, keeping each theme's current identity (`macmodern` translucent bars,
  `win11` mica-like surfaces, `xp`/`win98` bevels and title bars, `macos` classic black-and-white System 7 look,
  `xterm` twm title bars on grey, `dracula`/`noc` accent glow, `matrix` rain), all scoped and switchable.
- **Accessibility as a contract (WCAG 2.2 AAA, 1.4.6)**: every normal-size text — body, secondary, labels, buttons,
  badges, status messages — ≥ 7:1 in every theme; large text ≥ 4.5:1; UI boundaries and focus indicators ≥ 3:1.
  Legacy themes whose backgrounds cannot reach 7:1 (Win98 teal, XP blue) get opaque surfaces under text and adjusted
  tones, keeping bevels and title bars as identity. Enforced by tests on tokens and on rendered screens;
  `prefers-reduced-motion` and a user **Effects** preference (full / reduced / off) honoured everywhere.
- **Visual regression**: screenshots of the key screens in all themes, compared in CI.
- Square corners stay mandatory; the remote session (`.client-view`) stays undecorated.

## Capabilities

### New Capabilities
- `ui-theming`: theme catalogue, shared design tokens (color, elevation, motion), interaction effects, per-theme
  signature effects, accessibility (contrast, reduced motion, effects preference) and visual regression guarantees.

### Modified Capabilities
<!-- none: openspec/specs has no baseline yet (add-goguacamole-panel is not archived) -->

## Impact

- Frontend: `frontend/src/themes/*` (all 16 themes, `base.css`, `define.ts`), `frontend/src/styles/main.css`,
  shared components (`Panel`, `PageHeader`, `ConfirmDialog`, pickers, tables, sidebar, topbar, `ErrorBox`, new
  `Toast`/`Skeleton`), `SettingsView` (Effects preference), locales.
- Backend/database: new `effects` column in `preferences` (migration, read/write, validation) on the three drivers.
- Tests: token and contrast tests, a component test environment (DOM), axe-core and keyboard checks, Playwright
  screenshot suite in a pinned container (ROADMAP "Visual regression").
- No new runtime dependency; dev dependencies for component and accessibility tests; CSS budget (see design).
- Spec consolidation: `add-goguacamole-panel`'s `web-ui` "Themes" requirement and `administration` preferences
  must defer to `ui-theming` before archiving.
