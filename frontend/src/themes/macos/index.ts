import { defineTheme, promptIcon } from '../define'
import './theme.css'

export default defineTheme({
  id: 'macos',
  label: 'Mac OS (retro)',
  scheme: 'light',
  order: 15,
  animated: false,
  favicon: promptIcon('#dddddd', '#1f3a9a'),
})
