import { defineTheme, promptIcon } from '../define'
import './theme.css'

export default defineTheme({
  id: 'google',
  label: 'Google',
  scheme: 'light',
  order: 10,
  clock: false,
  animated: false,
  favicon: promptIcon('#0b57d0', '#ffffff'),
})
