import { defineTheme, svgIcon } from '../define'
import './theme.css'

export default defineTheme({
  id: 'xterm',
  label: 'xterm',
  scheme: 'light',
  order: 6,
  animated: false,
  favicon: svgIcon(
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" shape-rendering="crispEdges"><rect x="1" y="1" width="30" height="30" fill="#fff" stroke="#000" stroke-width="2"/><path d="M2 8h28" stroke="#000" stroke-width="2"/><path d="M6 13h2v2h2v2H8v2H6z"/><rect x="12" y="19" width="8" height="2"/></svg>`,
  ),
})
