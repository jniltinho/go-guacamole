import { defineTheme, promptIcon } from '../define'
import './theme.css'

export default defineTheme({
  id: 'macos',
  label: 'Mac OS (retro)',
  scheme: 'light',
  order: 15,
  clock: false,
  animated: false,
  favicon: promptIcon('#ffffff', '#000000'),
})
