import { defineTheme, promptIcon } from '../define'
import './theme.css'

export default defineTheme({
  id: 'dracula',
  label: 'Dracula',
  scheme: 'dark',
  order: 3,
  animated: false,
  favicon: promptIcon('#282a36', '#bd93f9', '#ff79c6'),
})
