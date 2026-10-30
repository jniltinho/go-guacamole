# Tasks

## 1. Test infrastructure and token contract

- [ ] 1.1 Add dev dependencies (`happy-dom`, `@vue/test-utils`, `@axe-core/playwright`) and a DOM Vitest project for component/token tests. Verify: a sample component test runs in CI.
- [x] 1.2 Add shared defaults (elevation, motion, fonts, `--motion-distance`) and new color tokens in `themes/base.css` for `:root` and `.theme-swatch`; `--surface-2` aliases `--surface-raised`; map them in `styles/main.css`. Verify: `npx vp build` passes and light/dark render unchanged.
- [x] 1.3 Resolved-token completeness test: render each theme and swatch, read computed custom properties, fail naming theme and token. Verify: passes; removing a default makes it fail.
- [x] 1.4 Contrast inventory (`themes/contrast-pairs.ts`) of every used text/background combination (tokens, states, gradients, badges, messages, sidebar, selection, focus) and the contrast test (normal 7, large 4.5, non-text 3; translucent composited per mode). Verify: test runs and lists current failures (expected until group 5).
- [x] 1.5 Update `themes/README.md` with tokens, inventory, identities and test commands. Verify: README reviewed.

## 2. Effects preference

- [x] 2.1 Migration (next free number, coordinated with the recording change) adding `preferences.effects`; store read/write; API enum validation (422). Verify: `go test` and `make test-db` cover valid, missing and invalid values on the three drivers.
- [x] 2.2 Frontend type, Settings selector (en/pt-BR), `html[data-effects]` resolution with `prefers-reduced-motion` fallback and first-paint cache. Verify: unit test for the resolution; switching in Settings changes the attribute immediately.
- [ ] 2.3 Migrate existing transitions (`main.css`, `IconMenu.vue`, `SettingsView.vue`) and decorative animations (Matrix rain) to the motion tokens and modes. Verify: Playwright asserts no computed `transition-duration` > 0 with `off`, no transform movement with `reduced`, and that switching to `off` while a menu opens ends in the final state at once.

## 3. Remote session isolation

- [x] 3.1 Scope every decoration, text effect, transition and canvas with `html:not(.in-client)`; ensure effects never set transforms inside `.client-view`. Verify: Playwright enters/leaves a session in Matrix and macmodern, checks computed styles of the display and ancestors, resizes, and checks pointer mapping in full/reduced/off.

## 4. Volume and motion in shared components

- [x] 4.1 Elevation on `.card`/`Panel`, sticky headers, sidebar, topbar, menus/popovers, `ConfirmDialog` with dimmed backdrop; hover lift and press. Verify: Playwright computed `box-shadow`/`transform` on hover and press in light and dark.
- [x] 4.2 Named transitions (`fade`, `rise`, `pop`) for route, panels, menus, modals; opaque visible focus for inputs and buttons. Verify: keyboard test shows a focus indicator ≥ 3:1 on every focusable element of the login and connections screens.
- [ ] 4.3 `Skeleton` with transform-based shimmer in connections, users, sessions, audit and history lists. Verify: component test; static in reduced/off.
- [ ] 4.4 `Toast` (live region, dismissible, errors as alert) replacing inline notices in Settings, Users, Connections, Authentication. Verify: component test of roles; manual NVDA/Orca check recorded in the PR.
- [x] 4.5 Live-session pulse on active session indicators. Verify: visible with full, static with reduced/off.

## 5. Refine every theme (AAA)

- [x] 5.1 `light`, `dark`, `contrast`, `google`, `ubuntu`: palettes and elevation to pass the contrast inventory. Verify: contrast test green for these themes.
- [x] 5.2 `macmodern`, `win11`, `kde`: translucent/mica-like surfaces with opaque fallback, elevation. Verify: contrast test green in all modes; Effects off shows opaque bars.
- [x] 5.3 `xp`, `win98`: opaque surfaces under text over the teal/blue backgrounds, adjusted gradients, bevels on the elevation model; visual approval recorded. Verify: contrast test green; screenshots approved.
- [x] 5.4 `macos` (System 7), `xterm` (twm): keep black-and-white identity, add depth via bevels/shadows that do not lower contrast. Verify: contrast test green.
- [x] 5.5 `dracula`, `noc`, `matrix`, `bionexo`: glow effects scoped and preference-aware; fix `.badge`/accent text (e.g. white on Matrix green). Verify: the whole contrast suite passes.

## 6. Visual regression, accessibility and budgets

- [x] 6.1 Screenshot suite (5 screens × 16 themes) in the pinned Playwright container (fonts, locale, DPR, fixed clock, seed, effects off, `maxDiffPixelRatio` 0.002), references committed, diffs uploaded. Verify: passes twice in a row in CI.
- [x] 6.2 axe-core (`color-contrast-enhanced`, focus and live-region rules) on key screens for every theme and mode; `incomplete` results fail unless in a reviewed allowlist. Verify: zero violations.
- [ ] 6.3 CSS size step (≤ 60 KB gzip) and the reference performance trace (4× CPU throttle, 200-row hover, no Layout from hover, p95 frame ≤ 16.7 ms). Verify: CI job output.
- [x] 6.4 Wire the jobs into CI and document `make` targets in the README. Verify: CI on the PR is green.
- [x] 6.5 Spec consolidation: update `add-goguacamole-panel`'s `web-ui` "Themes" requirement and the preferences in `administration` to defer to `ui-theming`. Verify: `openspec validate --strict` passes for both changes.

## Verification gaps (open)

- 1.1: only `@axe-core/playwright` was added; the token tests read the CSS files with `node:fs` instead of a DOM
  environment, so `happy-dom`/`@vue/test-utils` and component tests are not in place (swatch cascade not checked).
- 2.3: transitions were migrated to the motion tokens, but no Playwright test asserts the computed durations per mode.
- 4.3: skeletons cover users and connections only (sessions, audit, history still show the plain empty state).
- 4.4: toasts replace inline notices; the NVDA/Orca check was not recorded.
- 6.1: pixel references cover the login page of the 16 themes (recorded twice, stable, in
  `mcr.microsoft.com/playwright:v1.64.1-noble`); the 8 data screens per theme are gated by axe AAA, keyboard focus and
  sidebar focus contrast instead of pixels, since their content depends on the database.
- 6.3: theme CSS measured at 11.7 KB gzip (budget 60 KB) by hand; no CI size step nor performance trace yet.
- 6.4: wired through `deploy/e2e.sh browser` (both Playwright suites now run in the pinned image, the CI job no longer
  installs browsers); not yet observed green in GitHub Actions, only locally on a clean lab stack (14 + 18 passed).

Done beyond the plan (round 2): SSH terminal color schemes with preference, connection override and "follow UI theme"
(theme sent at launch); subtle radius token (≤ 8px) for modern themes; Bionexo rebuilt on the brand; role-aware search
placeholder, "/" shortcut (disabled in dialogs, can be turned off), tabular digits; sidebar focus visible on active
and inactive links; QA as a plain user of a group (`noc-l1`): only granted connections, no parameters, own history
only, 403/404 on administration, recording and hold endpoints. Reviewed with Codex in three passes (6 findings fixed,
then confirmed resolved).

Done beyond the plan: legacy decorations (gradients of XP/Win98) are covered by a contrast test, the layout is
slimmer (topbar h-10, compact tables, panels and controls), and the axe AAA + keyboard focus suite passes in all 16
themes on login, dashboard, connections, settings and recording.

## Workflow follow-up

- Archive `add-goguacamole-panel` first, then this change with `/opsx:archive`, so `ui-theming` becomes a baseline
  spec.
