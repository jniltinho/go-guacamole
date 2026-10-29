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

Rules: never use `border-radius` (the layout forbids rounded corners), keep text contrast ≥ 4.5:1,
and do not style inside `.client-view` (the remote session must not be decorated).
