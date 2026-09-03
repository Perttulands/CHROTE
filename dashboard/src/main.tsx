import React from 'react'
import ReactDOM from 'react-dom/client'
import './styles/theme-colors.css'
import './styles/base.css'
import App from './App'
import './components/FilesView/FilesView.css'
import { installChunkReloadRecovery } from './chunkReloadRecovery'

installChunkReloadRecovery()

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
)
