import React from 'react'
import { createRoot } from 'react-dom/client'
import '../../styles/theme-colors.css'
import '../../styles/base.css'
import SendDrawerPrototype from './SendDrawerPrototype'

if (import.meta.env.DEV) {
  document.title = 'Send to session · CHROTE prototype'
  createRoot(document.getElementById('root')!).render(<React.StrictMode><SendDrawerPrototype /></React.StrictMode>)
}
