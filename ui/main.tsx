import { LayerProvider, Theme } from '@astryxdesign/core'
import { neutralTheme } from '@astryxdesign/theme-neutral/built'
import ReactDOM from 'react-dom/client'

import App from './App'

import './styles.css'

ReactDOM.createRoot(document.getElementById('root')!).render(
  <Theme theme={neutralTheme}>
    <LayerProvider>
      <App />
    </LayerProvider>
  </Theme>,
)
