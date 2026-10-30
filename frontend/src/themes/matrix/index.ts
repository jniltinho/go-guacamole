import { defineTheme, promptIcon } from '../define'
import './theme.css'

export default defineTheme({
  id: 'matrix',
  label: 'Matrix',
  scheme: 'dark',
  order: 5,
  animated: true,
  favicon: promptIcon('#000000', '#00ff41'),
})
