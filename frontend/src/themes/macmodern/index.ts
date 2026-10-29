import { defineTheme, promptIcon } from '../define'
import './theme.css'

export default defineTheme({
  id: 'macmodern',
  label: 'macOS',
  scheme: 'light',
  order: 7,
  clock: false,
  animated: false,
  favicon: promptIcon('#0a84ff', '#ffffff'),
})
