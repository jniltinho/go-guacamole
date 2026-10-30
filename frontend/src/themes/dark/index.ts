import { defineTheme, promptIcon } from '../define'
import './theme.css'

export default defineTheme({
  id: 'dark',
  label: 'Dark',
  scheme: 'dark',
  order: 1,
  animated: false,
  favicon: promptIcon('#0b1120', '#3b82f6', '#e2e8f0'),
})
