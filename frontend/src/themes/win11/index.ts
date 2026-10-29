import { defineTheme, promptIcon } from '../define'
import './theme.css'

export default defineTheme({
  id: 'win11',
  label: 'Windows 11',
  scheme: 'light',
  order: 12,
  clock: true,
  animated: false,
  favicon: promptIcon('#005fb8', '#ffffff'),
})
