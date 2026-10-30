# Tasks

## 1. Test infrastructure and token contract

- [ ] 1.1 Add dev dependencies (`happy-dom`, `@vue/test-utils`, `@axe-core/playwright`) and a DOM Vitest project for component/token tests. Verify: a sample component test runs in CI.
- [ ] 1.2 Add shared defaults (elevation, motion, fonts, `--motion-distance`) and new color tokens in `themes/base.css` for `:root` and `.theme-swatch`; `--surface-2` aliases `--surface-raised`; map them in `styles/main.css`. Verify: `npx vp build` passes and light/dark render unchanged.
- [ ] 1.3 Resolved-token completeness test: render each theme and swatch, read computed custom properties, fail naming theme and token. Verify: passes; removing a default makes it fail.
- [ ] 1.4 Contrast inventory (`themes/contrast-pairs.ts`) of every used text/background combination (tokens, states, gradients, badges, messages, sidebar, selection, focus) and the contrast test (normal 7, large 4.5, non-text 3; translucent composited per mode). Verify: test runs and lists current failures (expected until group 5).
- [ ] 1.5 Update `themes/README.md` with tokens, inventory, identities and test commands. Verify: README reviewed.

## 2. Effects preference

- [ ] 2.1 Migration (next free number, coordinated with the recording change) adding `preferences.effects`; store read/write; API enum validation (422). Verify: `go test` and `make test-db` cover valid, missing and invalid values on the three drivers.
- [ ] 2.2 Frontend type, Settings selector (en/pt-BR), `html[data-effects]` resolution with `prefers-reduced-motion` fallback and first-paint cache. Verify: unit test for the resolution; switching in Settings changes the attribute immediately.
- [ ] 2.3 Migrate existing transitions (`main.css`, `IconMenu.vue`, `SettingsView.vue`) and decorative animations (Matrix rain) to the motion tokens and modes. Verify: Playwright asserts no computed `transition-duration` > 0 with `off`, no transform movement with `reduced`, and that switching to `off` while a menu opens ends in the final state at once.

## 3. Remote session isolation

- [ ] 3.1 Scope every decoration, text effect, transition and canvas with `html:not(.in-client)`; ensure effects never set transforms inside `.client-view`. Verify: Playwright enters/leaves a session in Matrix and macmodern, checks computed styles of the display and ancestors, resizes, and checks pointer mapping in full/reduced/off.

## 4. Volume and motion in shared components

- [ ] 4.1 Elevation on `.card`/`Panel`, sticky headers, sidebar, topbar, menus/popovers, `ConfirmDialog` with dimmed backdrop; hover lift and press. Verify: Playwright computed `box-shadow`/`transform` on hover and press in light and dark.
- [ ] 4.2 Named transitions (`fade`, `rise`, `pop`) for route, panels, menus, modals; opaque visible focus for inputs and buttons. Verify: keyboard test shows a focus indicator ≥ 3:1 on every focusable element of the login and connections screens.
- [ ] 4.3 `Skeleton` with transform-based shimmer in connections, users, sessions, audit and history lists. Verify: component test; static in reduced/off.
- [ ] 4.4 `Toast` (live region, dismissible, errors as alert) replacing inline notices in Settings, Users, Connections, Authentication. Verify: component test of roles; manual NVDA/Orca check recorded in the PR.
- [ ] 4.5 Live-session pulse on active session indicators. Verify: visible with full, static with reduced/off.

## 5. Refine every theme (AAA)

- [ ] 5.1 `light`, `dark`, `contrast`, `google`, `ubuntu`: palettes and elevation to pass the contrast inventory. Verify: contrast test green for these themes.
- [ ] 5.2 `macmodern`, `win11`, `kde`: translucent/mica-like surfaces with opaque fallback, elevation. Verify: contrast test green in all modes; Effects off shows opaque bars.
- [ ] 5.3 `xp`, `win98`: opaque surfaces under text over the teal/blue backgrounds, adjusted gradients, bevels on the elevation model; visual approval recorded. Verify: contrast test green; screenshots approved.
- [ ] 5.4 `macos` (System 7), `xterm` (twm): keep black-and-white identity, add depth via bevels/shadows that do not lower contrast. Verify: contrast test green.
- [ ] 5.5 `dracula`, `noc`, `matrix`, `bionexo`: glow effects scoped and preference-aware; fix `.badge`/accent text (e.g. white on Matrix green). Verify: the whole contrast suite passes.

## 6. Visual regression, accessibility and budgets

- [ ] 6.1 Screenshot suite (5 screens × 16 themes) in the pinned Playwright container (fonts, locale, DPR, fixed clock, seed, effects off, `maxDiffPixelRatio` 0.002), references committed, diffs uploaded. Verify: passes twice in a row in CI.
- [ ] 6.2 axe-core (`color-contrast-enhanced`, focus and live-region rules) on key screens for every theme and mode; `incomplete` results fail unless in a reviewed allowlist. Verify: zero violations.
- [ ] 6.3 CSS size step (≤ 60 KB gzip) and the reference performance trace (4× CPU throttle, 200-row hover, no Layout from hover, p95 frame ≤ 16.7 ms). Verify: CI job output.
- [ ] 6.4 Wire the jobs into CI and document `make` targets in the README. Verify: CI on the PR is green.
- [ ] 6.5 Spec consolidation: update `add-goguacamole-panel`'s `web-ui` "Themes" requirement and the preferences in `administration` to defer to `ui-theming`. Verify: `openspec validate --strict` passes for both changes.

## Workflow follow-up

- Archive `add-goguacamole-panel` first, then this change with `/opsx:archive`, so `ui-theming` becomes a baseline
  spec.
