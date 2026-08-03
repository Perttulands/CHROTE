import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App'
import { ToastProvider } from './context/ToastContext'
import { installChunkReloadRecovery } from './chunkReloadRecovery'
import './styles/index.css'

installChunkReloadRecovery()

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ToastProvider>
      <App />
    </ToastProvider>
  </React.StrictMode>,
)
