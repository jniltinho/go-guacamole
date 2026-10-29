import { defineTheme, promptIcon } from '../define'
import './theme.css'

export default defineTheme({
  id: 'ubuntu',
  label: 'Ubuntu',
  scheme: 'light',
  order: 8,
  clock: true,
  animated: false,
  favicon: promptIcon('#e95420', '#ffffff'),
})
