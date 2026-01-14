import { createContext, useContext, useState, useEffect, useCallback, useMemo, ReactNode } from 'react'
import type { DashboardContextType, TmuxSession, TerminalWindow, SessionsResponse } from '../types'

const STORAGE_KEY = 'arena-dashboard-state'
const POLL_INTERVAL = 5000 // 5 seconds

interface StoredState {
  windows: TerminalWindow[]
  windowCount: number
  sidebarCollapsed: boolean
}

function loadStoredState(): StoredState | null {
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    if (stored) {
      return JSON.parse(stored)
    }
  } catch (e) {
    console.warn('Failed to load stored state:', e)
  }
  return null
}

function saveState(state: StoredState): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(state))
  } catch (e) {
    console.warn('Failed to save state:', e)
  }
}

function createDefaultWindows(count: number): TerminalWindow[] {
  return Array.from({ length: count }, (_, i) => ({
    id: `window-${i}`,
    boundSessions: [],
    activeSession: null,
    colorIndex: i,
  }))
}

const SessionContext = createContext<DashboardContextType | null>(null)

export function SessionProvider({ children }: { children: ReactNode }) {
  // Load initial state from localStorage or use defaults
  const stored = useMemo(() => loadStoredState(), [])

  const [sessions, setSessions] = useState<TmuxSession[]>([])
  const [groupedSessions, setGroupedSessions] = useState<Record<string, TmuxSession[]>>({})
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [windowCount, setWindowCountState] = useState(stored?.windowCount ?? 2)
  const [windows, setWindows] = useState<TerminalWindow[]>(
    stored?.windows ?? createDefaultWindows(2)
  )
  const [sidebarCollapsed, setSidebarCollapsed] = useState(stored?.sidebarCollapsed ?? false)
  const [floatingSession, setFloatingSession] = useState<string | null>(null)
  const [isDragging, setIsDragging] = useState(false)

  // Computed: which sessions are assigned to any window
  const assignedSessions = useMemo(() => {
    const assigned = new Set<string>()
    windows.forEach(w => w.boundSessions.forEach(s => assigned.add(s)))
    return assigned
  }, [windows])

  // Persist state to localStorage
  useEffect(() => {
    saveState({ windows, windowCount, sidebarCollapsed })
  }, [windows, windowCount, sidebarCollapsed])

  // Fetch sessions from API
  const refreshSessions = useCallback(async () => {
    try {
      const response = await fetch('/api/tmux/sessions')
      const data: SessionsResponse = await response.json()

      if (data.error) {
        setError(data.error)
        setSessions([])
        setGroupedSessions({})
      } else {
        setError(null)
        setSessions(data.sessions)
        setGroupedSessions(data.grouped)
      }
    } catch (e) {
      setError('Failed to fetch sessions')
      console.error('Failed to fetch sessions:', e)
    } finally {
      setLoading(false)
    }
  }, [])

  // Poll for sessions
  useEffect(() => {
    refreshSessions()
    const interval = setInterval(refreshSessions, POLL_INTERVAL)
    return () => clearInterval(interval)
  }, [refreshSessions])

  // Actions
  const setWindowCount = useCallback((count: number) => {
    const newCount = Math.max(1, Math.min(4, count))
    setWindowCountState(newCount)

    setWindows(prev => {
      if (newCount > prev.length) {
        // Add windows
        const newWindows = [...prev]
        for (let i = prev.length; i < newCount; i++) {
          newWindows.push({
            id: `window-${i}`,
            boundSessions: [],
            activeSession: null,
            colorIndex: i,
          })
        }
        return newWindows
      } else if (newCount < prev.length) {
        // Remove windows (keep bound sessions - they'll be unassigned)
        return prev.slice(0, newCount)
      }
      return prev
    })
  }, [])

  const addSessionToWindow = useCallback((windowId: string, sessionName: string) => {
    setWindows(prev => prev.map(w => {
      if (w.id !== windowId) return w
      if (w.boundSessions.includes(sessionName)) return w

      const newBound = [...w.boundSessions, sessionName]
      return {
        ...w,
        boundSessions: newBound,
        activeSession: w.activeSession ?? sessionName, // Set as active if first
      }
    }))

    // Also remove from any other window
    setWindows(prev => prev.map(w => {
      if (w.id === windowId) return w
      if (!w.boundSessions.includes(sessionName)) return w

      const newBound = w.boundSessions.filter(s => s !== sessionName)
      return {
        ...w,
        boundSessions: newBound,
        activeSession: w.activeSession === sessionName
          ? (newBound[0] ?? null)
          : w.activeSession,
      }
    }))
  }, [])

  const removeSessionFromWindow = useCallback((windowId: string, sessionName: string) => {
    setWindows(prev => prev.map(w => {
      if (w.id !== windowId) return w
      if (!w.boundSessions.includes(sessionName)) return w

      const newBound = w.boundSessions.filter(s => s !== sessionName)
      return {
        ...w,
        boundSessions: newBound,
        activeSession: w.activeSession === sessionName
          ? (newBound[0] ?? null)
          : w.activeSession,
      }
    }))
  }, [])

  const setActiveSession = useCallback((windowId: string, sessionName: string) => {
    setWindows(prev => prev.map(w => {
      if (w.id !== windowId) return w
      if (!w.boundSessions.includes(sessionName)) return w
      return { ...w, activeSession: sessionName }
    }))
  }, [])

  const cycleSession = useCallback((windowId: string, direction: 'prev' | 'next') => {
    setWindows(prev => prev.map(w => {
      if (w.id !== windowId) return w
      if (w.boundSessions.length <= 1) return w

      const currentIndex = w.activeSession
        ? w.boundSessions.indexOf(w.activeSession)
        : 0

      let newIndex: number
      if (direction === 'next') {
        newIndex = (currentIndex + 1) % w.boundSessions.length
      } else {
        newIndex = (currentIndex - 1 + w.boundSessions.length) % w.boundSessions.length
      }

      return { ...w, activeSession: w.boundSessions[newIndex] }
    }))
  }, [])

  const toggleSidebar = useCallback(() => {
    setSidebarCollapsed(prev => !prev)
  }, [])

  const openFloatingModal = useCallback((sessionName: string) => {
    setFloatingSession(sessionName)
  }, [])

  const closeFloatingModal = useCallback(() => {
    setFloatingSession(null)
  }, [])

  const handleSessionClick = useCallback((sessionName: string) => {
    // Find if session is assigned to any window
    const assignedWindow = windows.find(w => w.boundSessions.includes(sessionName))

    if (assignedWindow) {
      // If assigned, make it active in that window
      setActiveSession(assignedWindow.id, sessionName)
    } else {
      // If not assigned, open floating modal
      openFloatingModal(sessionName)
    }
  }, [windows, setActiveSession, openFloatingModal])

  const contextValue: DashboardContextType = {
    // State
    sessions,
    groupedSessions,
    loading,
    error,
    windows,
    windowCount,
    sidebarCollapsed,
    floatingSession,
    assignedSessions,
    isDragging,

    // Actions
    setWindowCount,
    addSessionToWindow,
    removeSessionFromWindow,
    setActiveSession,
    cycleSession,
    toggleSidebar,
    openFloatingModal,
    closeFloatingModal,
    handleSessionClick,
    refreshSessions,
    setIsDragging,
  }

  return (
    <SessionContext.Provider value={contextValue}>
      {children}
    </SessionContext.Provider>
  )
}

export function useSession(): DashboardContextType {
  const context = useContext(SessionContext)
  if (!context) {
    throw new Error('useSession must be used within a SessionProvider')
  }
  return context
}
