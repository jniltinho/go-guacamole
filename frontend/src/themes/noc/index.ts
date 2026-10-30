import { defineTheme, svgIcon } from '../define'
import './theme.css'

export default defineTheme({
  id: 'noc',
  label: 'Monitoring (NOC)',
  scheme: 'dark',
  order: 2,
  animated: false,
  favicon: svgIcon(
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" fill="none"><rect width="32" height="32" fill="#0a0d12"/><path d="M4 19h5l2.5-7 4 12 3-8 2 4H28" stroke="#22c3e6" stroke-width="2.4" stroke-linejoin="round" stroke-linecap="round"/><circle cx="26" cy="7" r="3" fill="#3fb950"/></svg>`,
  ),
})
