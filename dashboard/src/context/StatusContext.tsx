/**
 * What CHROTE last said, and when it said it.
 *
 * Every announcement in the dashboard lands here and is drawn twice: as a
 * toast in the bottom-centre slot for as long as it takes to read, and on the
 * status line at the foot of the window as the record. There is no queue and
 * nothing has to be dismissed: the newest event replaces the one before it.
 *
 * `announce` carries the same two arguments the old toast did, so a call site
 * says what it always said and says it in one place.
 */

import { createContext, useCallback, useContext, useMemo, useState } from 'react'
import type { ReactNode } from 'react'

export type StatusSeverity = 'success' | 'info' | 'warning' | 'error'

export interface StatusEvent {
  id: number
  message: string
  severity: StatusSeverity
  at: number
}

interface StatusContextType {
  /** The last thing CHROTE said, or null before it has said anything. */
  status: StatusEvent | null
  announce: (message: string, severity: StatusSeverity) => void
}

const StatusContext = createContext<StatusContextType | null>(null)

export function StatusProvider({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<StatusEvent | null>(null)

  const announce = useCallback((message: string, severity: StatusSeverity) => {
    setStatus(previous => ({
      id: (previous?.id ?? 0) + 1,
      message,
      severity,
      at: Date.now(),
    }))
  }, [])

  const value = useMemo(() => ({ status, announce }), [status, announce])

  return <StatusContext.Provider value={value}>{children}</StatusContext.Provider>
}

export function useStatus(): StatusContextType {
  const context = useContext(StatusContext)
  if (!context) throw new Error('useStatus must be used within a StatusProvider')
  return context
}
