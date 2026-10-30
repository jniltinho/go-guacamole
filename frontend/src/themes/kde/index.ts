import { defineTheme, promptIcon } from '../define'
import './theme.css'

export default defineTheme({
  id: 'kde',
  label: 'KDE Plasma',
  scheme: 'dark',
  order: 9,
  animated: false,
  favicon: promptIcon('#232629', '#3daee9', '#eff0f1'),
})
