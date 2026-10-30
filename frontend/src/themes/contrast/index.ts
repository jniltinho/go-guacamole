import { defineTheme, promptIcon } from '../define'
import './theme.css'

export default defineTheme({
  id: 'contrast',
  label: 'High contrast',
  scheme: 'dark',
  order: 11,
  animated: false,
  favicon: promptIcon('#000000', '#ffff00', '#ffffff'),
})
