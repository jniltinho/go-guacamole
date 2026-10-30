import { defineTheme, promptIcon } from '../define'
import './theme.css'

export default defineTheme({
  id: 'bionexo',
  label: 'Bionexo',
  scheme: 'light',
  order: 4,
  animated: false,
  favicon: promptIcon('#1d3c55', '#34cdd7', '#ffffff'),
})
