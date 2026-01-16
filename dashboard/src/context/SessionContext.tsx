import { createContext, useContext, useState, useEffect, useCallback, useMemo, ReactNode } from 'react'
import type { DashboardContextType, TmuxSession, TerminalWindow, SessionsResponse, UserSettings, TmuxAppearance } from '../types'
import { DEFAULT_SETTINGS, DEFAULT_TMUX_APPEARANCE } from '../types'

// Apply tmux appearance settings via API (hot-reload)
async function applyTmuxAppearance(appearance: TmuxAppearance): Promise<void> {
  try {
    await fetch('/api/tmux/appearance', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(appearance),
    })
  } catch (e) {
    console.warn('Failed to apply tmux appearance:', e)
  }
}

const STORAGE_KEY = 'arena-dashboard-state'

interface StoredState {
  windows: TerminalWindow[]
  windowCount: number
  sidebarCollapsed: boolean
  settings: UserSettings
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

  const [windowCount, setWindowCountState] = useState(stored?.windowCount ?? 1)
  const [windows, setWindows] = useState<TerminalWindow[]>(
    stored?.windows ?? createDefaultWindows(1)
  )
  const [sidebarCollapsed, setSidebarCollapsed] = useState(stored?.sidebarCollapsed ?? false)
  const [floatingSession, setFloatingSession] = useState<string | null>(null)
  const [isDragging, setIsDragging] = useState(false)
  const [settings, setSettings] = useState<UserSettings>(stored?.settings ?? DEFAULT_SETTINGS)
  const [focusedWindowIndex, setFocusedWindowIndex] = useState(0)

  // Computed: which sessions are assigned to any window
  const assignedSessions = useMemo(() => {
    const assigned = new Map<string, { windowId: string; colorIndex: number; windowIndex: number }>()
    windows.forEach((w, idx) => {
      w.boundSessions.forEach(s => {
        assigned.set(s, {
          windowId: w.id,
          colorIndex: w.colorIndex,
          windowIndex: idx + 1 // 1-based index for UI badge
        })
      })
    })
    return assigned
  }, [windows])

  // Clean up any stuck INIT-PENDING windows on mount (from page refresh during creation)
  useEffect(() => {
    setWindows(prev => {
      const hasStuck = prev.some(w => w.activeSession === 'INIT-PENDING')
      if (!hasStuck) return prev
      return prev.map(w =>
        w.activeSession === 'INIT-PENDING'
          ? { ...w, activeSession: null }
          : w
      )
    })
  }, [])

  // Persist state to localStorage (filter out INIT-PENDING to avoid stuck state)
  useEffect(() => {
    const cleanWindows = windows.map(w =>
      w.activeSession === 'INIT-PENDING'
        ? { ...w, activeSession: null }
        : w
    )
    saveState({ windows: cleanWindows, windowCount, sidebarCollapsed, settings })
  }, [windows, windowCount, sidebarCollapsed, settings])

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

        // Clean up orphaned session bindings (sessions that no longer exist in tmux)
        const existingSessionNames = new Set(data.sessions.map(s => s.name))

        setWindows(prev => {
          let changed = false
          const updated = prev.map(w => {
            const validBound = w.boundSessions.filter(s => existingSessionNames.has(s))
            if (validBound.length !== w.boundSessions.length) {
              changed = true
              const orphaned = w.boundSessions.filter(s => !existingSessionNames.has(s))
              console.error(`Orphaned sessions removed from ${w.id}:`, orphaned)

              // If activeSession was orphaned, set to first valid or null
              const newActive = validBound.includes(w.activeSession ?? '')
                ? w.activeSession
                : (validBound[0] ?? null)

              return { ...w, boundSessions: validBound, activeSession: newActive }
            }
            return w
          })
          return changed ? updated : prev
        })
      }
    } catch (e) {
      setError('Failed to fetch sessions')
      console.error('Failed to fetch sessions:', e)
    } finally {
      setLoading(false)
    }
  }, [])

  // Poll for sessions using settings interval
  useEffect(() => {
    refreshSessions()
    const interval = setInterval(refreshSessions, settings.autoRefreshInterval)
    return () => clearInterval(interval)
  }, [refreshSessions, settings.autoRefreshInterval])

  // Apply tmux appearance on initial load (ensures container matches saved settings)
  useEffect(() => {
    // Merge saved settings with defaults to handle missing fields from older localStorage
    const appearance = { ...DEFAULT_TMUX_APPEARANCE, ...settings.tmuxAppearance }
    applyTmuxAppearance(appearance)
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  // Actions
  const setWindowCount = useCallback((count: number) => {
    const newCount = Math.max(1, Math.min(4, count))
    setWindowCountState(newCount)

    // Ensure focusedWindowIndex stays valid
    setFocusedWindowIndex(prev => Math.min(prev, newCount - 1))

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
    setWindows(prev => {
      // 1. Remove from any other window first
      const cleaned = prev.map(w => {
        if (w.id === windowId) return w
        if (!w.boundSessions.includes(sessionName)) return w

        const newBound = w.boundSessions.filter(s => s !== sessionName)
        return {
          ...w,
          boundSessions: newBound,
          activeSession: w.activeSession === sessionName ? (newBound[0] ?? null) : w.activeSession
        }
      })

      // 2. Add to target window
      // If window is empty, auto-set as active (no interruption since nothing is playing)
      // If window has sessions, do NOT change activeSession - user must explicitly select
      return cleaned.map(w => {
        if (w.id !== windowId) return w
        if (w.boundSessions.includes(sessionName)) {
          return w  // Already bound, no change needed
        }
        const isEmpty = w.boundSessions.length === 0
        return {
          ...w,
          boundSessions: [...w.boundSessions, sessionName],
          activeSession: isEmpty ? sessionName : w.activeSession,
        }
      })
    })
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

  const cycleWindow = useCallback((direction: 'prev' | 'next') => {
    setFocusedWindowIndex(prev => {
      if (direction === 'next') {
        return (prev + 1) % windowCount
      } else {
        return (prev - 1 + windowCount) % windowCount
      }
    })
  }, [windowCount])

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
    // Always open floating modal for "peek" functionality
    openFloatingModal(sessionName)
  }, [openFloatingModal])

  const updateSettings = useCallback((newSettings: Partial<UserSettings>) => {
    setSettings(prev => {
      const updated = { ...prev, ...newSettings }
      // Hot-reload tmux appearance if it changed
      if (newSettings.tmuxAppearance) {
        applyTmuxAppearance(updated.tmuxAppearance)
      }
      return updated
    })
  }, [])

  const deleteSession = useCallback(async (sessionName: string) => {
    try {
      const response = await fetch(`/api/tmux/sessions/${encodeURIComponent(sessionName)}`, {
        method: 'DELETE',
      })
      if (response.ok) {
        refreshSessions()
      } else {
        console.error('Failed to delete session:', await response.text())
      }
    } catch (e) {
      console.error('Failed to delete session:', e)
    }
  }, [refreshSessions])

  const renameSession = useCallback(async (oldName: string, newName: string): Promise<boolean> => {
    try {
      const response = await fetch(`/api/tmux/sessions/${encodeURIComponent(oldName)}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ newName }),
      })
      if (response.ok) {
        // Update window bindings to use the new name
        setWindows(prev => prev.map(w => ({
          ...w,
          boundSessions: w.boundSessions.map(s => s === oldName ? newName : s),
          activeSession: w.activeSession === oldName ? newName : w.activeSession,
        })))
        refreshSessions()
        return true
      } else {
        console.error('Failed to rename session:', await response.text())
        return false
      }
    } catch (e) {
      console.error('Failed to rename session:', e)
      return false
    }
  }, [refreshSessions])

  // Memoize context value to prevent unnecessary rerenders of consuming components
  const contextValue: DashboardContextType = useMemo(() => ({
    // State
    sessions,
    groupedSessions,
    loading,
    error,
    windows,
    windowCount,
    focusedWindowIndex,
    sidebarCollapsed,
    floatingSession,
    assignedSessions,
    isDragging,
    settings,

    // Actions
    setWindowCount,
    addSessionToWindow,
    removeSessionFromWindow,
    setActiveSession,
    cycleSession,
    cycleWindow,
    setFocusedWindowIndex,
    toggleSidebar,
    openFloatingModal,
    closeFloatingModal,
    handleSessionClick,
    refreshSessions,
    deleteSession,
    renameSession,
    setIsDragging,
    updateSettings,
  }), [
    sessions,
    groupedSessions,
    loading,
    error,
    windows,
    windowCount,
    focusedWindowIndex,
    sidebarCollapsed,
    floatingSession,
    assignedSessions,
    isDragging,
    settings,
    setWindowCount,
    addSessionToWindow,
    removeSessionFromWindow,
    setActiveSession,
    cycleSession,
    cycleWindow,
    setFocusedWindowIndex,
    toggleSidebar,
    openFloatingModal,
    closeFloatingModal,
    handleSessionClick,
    refreshSessions,
    deleteSession,
    renameSession,
    setIsDragging,
    updateSettings,
  ])

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
