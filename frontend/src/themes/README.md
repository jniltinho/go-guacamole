# Themes

Each theme is a folder discovered automatically by `index.ts`:

```
themes/
  base.css          shared rules (Matrix canvas, retro clock)
  define.ts         ThemeMeta type and helpers
  index.ts          registry (import.meta.glob('./*/index.ts'))
  <id>/
    theme.css       html[data-theme='<id>'] { --bg: ...; --accent: ...; }  + optional decorations
    index.ts        export default defineTheme({ id, label, scheme, order, clock, animated, favicon })
```

## Adding a theme

1. Copy an existing folder (e.g. `light/`) to `themes/<new-id>/`.
2. In `theme.css` change the token selector to `html[data-theme='<new-id>'], .theme-swatch[data-theme='<new-id>']`
   (the second one feeds the color swatch in the theme picker) and set every token:
   `--scheme --bg --surface --surface-2 --sidebar --sidebar-fg --sidebar-active --sidebar-active-fg --fg --muted
   --border --accent --accent-fg --success --danger --warning --proto-ssh --proto-rdp --proto-vnc --font
   --font-mono --shadow --sb-thumb --sb-track` (optional: `--input-bg --focus --selection`).
3. Add decorations below the tokens, always scoped with `html[data-theme='<new-id>']`.
   Useful hooks: `.card`, `.card-title`, `.btn`, `.btn-primary`, `.input`, `.table`, `aside` (sidebar),
   `header.topbar`, `.conn-name`, `.host`, `.section-title`.
4. In `index.ts` set the metadata. Add `themes.<new-id>` to `src/locales/en.json` and `pt-BR.json`.

Rules: never write `border-radius` in a theme — set `--radius` (max 8px) to opt into subtle curves (retro themes stay
square) — and never style the remote session
(`.client-view`; global rules such as `body` must be written `html[data-theme='<id>']:not(.in-client) body`).

## Tokens, depth and motion (openspec `ui-theming`)

Shared defaults live in `base.css` (`:root, .theme-swatch`); a theme overrides only what defines its character.

| Group      | Tokens                                                                                                                                                |
| ---------- | ----------------------------------------------------------------------------------------------------------------------------------------------------- |
| surfaces   | `--bg --surface --surface-2 (= --surface-raised) --overlay --sidebar --sidebar-active`                                                                |
| text       | `--fg --muted --on-bg --on-bg-muted --accent --accent-fg --sidebar-fg --sidebar-active-fg --success --danger --danger-fg --warning --info --badge-fg` |
| boundaries | `--border --input-border --focus --focus-halo --selection`                                                                                            |
| depth      | `--elev-0..--elev-4` (layered shadow + 1px `--highlight`, from `--shadow-color`; `--elev-1` defaults to `--shadow`)                                   |
| motion     | `--dur-1/2/3 --ease-out --ease-in-out --motion-distance`                                                                                              |

Accessibility is a contract (WCAG 2.2 AAA): every normal text pair ≥ 7:1, input boundaries and focus ≥ 3:1.
`themes.test.ts` checks the resolved tokens of every theme against the inventory in `contrast.ts`
(`npx vp test run src/themes`); add a pair there when the UI draws a new text/background combination.
Text drawn directly on the page background uses `--on-bg`/`--on-bg-muted` (legacy desktops such as Win98 teal or XP
blue use white there). When no single focus color stands out from both the page and the cards, set `--focus-halo`
(a second ring).

Effects follow `html[data-effects]` (`full | reduced | off`, user preference with the OS reduced-motion setting as
fallback): animate only `opacity`/`transform`, use the motion tokens, and keep blur (`backdrop-filter`) on fixed bars
and overlays — `reduced` removes movement and blur, `off` removes every transition, animation and decorative layer.
Named transitions: `fade`, `rise`, `pop`; helpers `.lift`, `.skeleton`, `.live-pulse`.
