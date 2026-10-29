import { defineTheme, promptIcon } from '../define'
import './theme.css'

export default defineTheme({
  id: 'bionexo',
  label: 'Bionexo',
  scheme: 'dark',
  order: 4,
  clock: false,
  animated: false,
  favicon: promptIcon('#1f6bd6', '#ffffff'),
})
