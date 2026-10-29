import { defineTheme, promptIcon } from '../define'
import './theme.css'

export default defineTheme({
  id: 'light',
  label: 'Light',
  scheme: 'light',
  order: 0,
  clock: false,
  animated: false,
  favicon: promptIcon('#1d63ed', '#ffffff'),
})
