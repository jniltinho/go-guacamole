import { defineTheme, promptIcon } from '../define'
import './theme.css'

export default defineTheme({
  id: 'win98',
  label: 'Windows 98 (retro)',
  scheme: 'light',
  order: 14,
  animated: false,
  favicon: promptIcon('#000080', '#ffffff'),
})
