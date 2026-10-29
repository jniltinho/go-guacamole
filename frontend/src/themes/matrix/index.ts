import { defineTheme, promptIcon } from '../define'
import './theme.css'

export default defineTheme({
  id: 'matrix',
  label: 'Matrix',
  scheme: 'dark',
  order: 5,
  clock: false,
  animated: true,
  favicon: promptIcon('#000000', '#00ff41'),
})
