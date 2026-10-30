import { defineTheme, promptIcon } from '../define'
import './theme.css'

export default defineTheme({
  id: 'xp',
  label: 'Windows XP (retro)',
  scheme: 'light',
  order: 13,
  animated: false,
  favicon: promptIcon('#0058ee', '#33ff33'),
})
