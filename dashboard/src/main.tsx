import React from 'react'
import ReactDOM from 'react-dom/client'
import './styles/theme-colors.css'
import './styles/base.css'
import App from './App'
import './components/FilesView/FilesView.css'
import { ToastProvider } from './context/ToastContext'
import { installChunkReloadRecovery } from './chunkReloadRecovery'
import './styles/delightful.css'

installChunkReloadRecovery()

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ToastProvider>
      <App />
    </ToastProvider>
  </React.StrictMode>,
)
