# Design

## Context

Themes live in `frontend/src/themes/<id>/` (`theme.css` with `html[data-theme='<id>'], .theme-swatch[data-theme='<id>']`
tokens + scoped decorations, `index.ts` with `defineTheme` metadata), discovered by `import.meta.glob`.
`styles/main.css` maps tokens to Tailwind v4 utilities and defines `.card`, `.btn`, `.input`, `.table`, `.badge`; it
already has color/border/shadow transitions, and `.input:focus` replaces the outline with a translucent ring.
Some colors live outside tokens (XP/98 gradients, XP sidebar, button states, `.badge` with fixed white text).
Preferences are SQL columns (`preferences` table: theme, locale, sidebar_collapsed) read/written by
`GetPreferences/PutPreferences`. The remote client toggles `html.in-client`; `ClientView.rescale` scales the
Guacamole display with a CSS transform. Matrix applies `text-shadow` on `body`. Vitest runs in the `node`
environment with no DOM or component test utilities.

Current identities: `macos` is classic Mac OS (System 7, black and white); `macmodern` is modern macOS
(translucency); `xterm` is X11/twm (grey, black-on-white title bars). Measured today: Win98 desktop `#008080` gives
4.40:1 with black and 4.77:1 with white; XP blue `#3a6ea5` gives 3.95:1 / 5.31:1 — no text color reaches 7:1 on them.

## Goals / Non-Goals

**Goals:**
- One token contract consumed by components; themes tune values.
- Measurable quality: resolved-token completeness, AAA contrast on used combinations, screenshots, frame budget.
- Effects that degrade gracefully (full → reduced → off) and never touch the remote session.

**Non-Goals:**
- New themes, theme editor, per-user palettes, rounded corners, navigation redesign.
- Animating anything while a remote session is shown.

## Decisions

### D1. Tokens with shared defaults; contract checked after cascade
New tokens: `--surface-raised` (replaces `--surface-2`, kept as an alias during migration), `--overlay`, `--info`,
`--focus`, `--selection`, `--elev-0..--elev-4`, `--dur-1/2/3`, `--ease-out`, `--ease-in-out`, `--motion-distance`.
Defaults for elevation/motion/fonts are declared in `themes/base.css` for both `:root` and `.theme-swatch`; a theme
overrides what defines its character. The completeness test renders each theme (and its swatch) in a DOM test
environment and reads computed custom properties, so inherited defaults count. Components never reference a theme
id.

### D2. Elevation = layered shadow + 1px inner highlight, no radius
Each level is `inset 0 1px 0 <highlight>, 0 <y> <blur> <spread> <color>…`; light themes use soft dark shadows, dark
themes deeper shadows plus a faint top highlight. Hover lift = next level + `translateY(calc(-1 * var(--motion-distance)))`;
press = previous level + `translateY(var(--motion-distance))`. Legacy themes map levels to bevels.

### D3. Effects preference persisted as a column
Migration (next free number; coordinated with `add-recording-policy-per-connection`) adds
`preferences.effects VARCHAR(8) NULL`; store, API (enum validation, 422) and frontend types updated; NULL = not
chosen. `html[data-effects]` is set from the preference, else from `prefers-reduced-motion`, cached in
`localStorage` for first paint. `reduced`: `--motion-distance: 0`, no blur, no decorative animation, fades kept;
`off`: durations 0, no blur, decorative layers hidden. Existing transitions in `main.css`, `IconMenu.vue`,
`SettingsView.vue` are migrated to the motion tokens so the modes cover them.
*Alternative*: only `@media (prefers-reduced-motion)` — rejected: shared NOC screens need an explicit switch.

### D4. Transitions on opacity/transform only
Named transitions (`fade`, `rise`, `pop`) in `main.css` with Vue `<Transition>` for route, panels, menus, modals and
toasts. The skeleton shimmer is a pseudo-element translated with `transform` (no `background-position`).

### D5. Remote session isolation via `html.in-client`
All decorations, transitions and text effects are scoped with `html:not(.in-client)` (including the Matrix
`text-shadow` on `body`, the rain canvas and route transitions). Effects rules never set `transform` on elements
inside `.client-view`, so `ClientView.rescale` keeps working. Verified by a Playwright test that enters and leaves a
session, resizes, and checks computed styles of the display and its ancestors and the pointer mapping in each mode.

### D6. Toasts and skeletons
`Toast` (stack in `role="status"`/`aria-live="polite"`, errors `role="alert"`) replaces inline notices; `Skeleton`
renders placeholders sized like the real rows. No dependency.

### D7. Contrast verified on used combinations
1. An inventory (`themes/contrast-pairs.ts`) lists every text/background combination the UI uses: tokens, states
   (hover, active, selected, disabled), gradients sampled at every stop and midpoint, `surface-raised`,
   badges, messages, sidebar, selection, focus ring on each surface.
2. A token test checks the inventory for every theme: normal text ≥ 7, large text ≥ 4.5, non-text ≥ 3;
   translucent colors are composited over their declared opaque base for `full`, and the opaque fallback for
   `reduced`/`off` and for browsers without `backdrop-filter` (`@supports not`), tested explicitly.
3. Playwright + axe-core with `color-contrast-enhanced`, `focus-order-semantics` and ARIA live-region rules on the
   key screens for every theme and mode; `incomplete` results fail the job unless listed in a reviewed allowlist.
4. A keyboard test tabs through each key screen asserting a visible focus indicator ≥ 3:1 (`.input:focus` gets an
   opaque ring/outline). Screen reader announcement of toasts is checked manually with NVDA/Orca and recorded.
Legacy backgrounds that cannot reach 7:1 get opaque surfaces under text (desktop labels, title bars) and adjusted
tones, approved visually per theme.

### D8. Test infrastructure
Dev dependencies: `happy-dom` (Vitest environment for component/token tests), `@vue/test-utils`,
`@axe-core/playwright`. Screenshots run in the pinned Playwright container image (fonts installed, `en` locale,
DPR 1, 1440×900, fixed clock, seeded DB, effects off) with `maxDiffPixelRatio: 0.002`; diffs uploaded as CI
artifacts. Effects are verified separately (computed-style assertions), not in screenshots.

### D9. Budgets
Compiled theme CSS (all themes + base) ≤ 60 KB gzip (CI size step). Reference performance profile: Chromium in the
pinned CI container on the standard GitHub-hosted Linux runner (4 vCPU; CPU model recorded in the job output, budget
re-baselined only by an explicit change) with 4× CPU throttling, 200-row connections table, pointer sweep over all rows; pass if the trace has
no Layout events from hover and p95 frame time ≤ 16.7 ms. `backdrop-filter` only on fixed bars and overlays.

## Risks / Trade-offs

- [AAA forces visible palette changes in legacy themes] → opaque text surfaces and tone tweaks, identity kept in
  bevels/title bars; per-theme visual approval before references are recorded.
- [Blur is expensive on low-end GPUs/VMs] → only on fixed bars/overlays; disabled by `reduced`/`off`.
- [Screenshot flakiness] → pinned container, effects off, numeric tolerance, diffs published.
- [Heavier theme authoring] → defaults in `base.css`; README documents tokens, inventory and test commands.

## Migration Plan

Migration adds the nullable `effects` column (NULL = not chosen; OS setting decides). `--surface-2` stays as an alias
of `--surface-raised` until all components are migrated. Rollback: revert frontend and run the down migration
dropping the column; older builds ignore it if left in place.
