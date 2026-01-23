import { createContext, useContext, useState, useEffect, useCallback, useMemo, ReactNode } from 'react'
import type { DashboardContextType, TmuxSession, TerminalWindow, SessionsResponse, UserSettings, TmuxAppearance, WorkspaceId, TerminalWorkspace, LayoutPreset } from '../types'
import { DEFAULT_SETTINGS, DEFAULT_TMUX_APPEARANCE, MAX_PRESETS } from '../types'
import { useToast } from './ToastContext'

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

const STORAGE_KEY = 'chrote-dashboard-state'
const PRESETS_STORAGE_KEY = 'chrote-dashboard-presets'

const WORKSPACE_IDS: WorkspaceId[] = ['terminal1', 'terminal2']

// Load presets from localStorage
function loadStoredPresets(): LayoutPreset[] {
  try {
    const stored = localStorage.getItem(PRESETS_STORAGE_KEY)
    if (stored) {
      const parsed = JSON.parse(stored)
      if (Array.isArray(parsed)) {
        return parsed
      }
    }
  } catch (e) {
    console.warn('Failed to load stored presets:', e)
  }
  return []
}

// Save presets to localStorage
function savePresets(presets: LayoutPreset[]): void {
  try {
    localStorage.setItem(PRESETS_STORAGE_KEY, JSON.stringify(presets))
  } catch (e) {
    console.warn('Failed to save presets:', e)
  }
}

// Generate a unique ID for presets
function generatePresetId(): string {
  return `preset-${Date.now()}-${Math.random().toString(36).substring(2, 9)}`
}

// Deep clone workspaces to avoid reference issues
function cloneWorkspaces(workspaces: Record<WorkspaceId, TerminalWorkspace>): Record<WorkspaceId, TerminalWorkspace> {
  return JSON.parse(JSON.stringify(workspaces))
}

interface StoredStateV2 {
  workspaces: Record<WorkspaceId, TerminalWorkspace>
  sidebarCollapsed: boolean
  settings: UserSettings
}

function loadStoredState(): StoredStateV2 | null {
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    if (stored) {
      const parsed: unknown = JSON.parse(stored)
      return migrateStoredState(parsed)
    }
  } catch (e) {
    console.warn('Failed to load stored state:', e)
  }
  return null
}

function saveState(state: StoredStateV2): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(state))
  } catch (e) {
    console.warn('Failed to save state:', e)
  }
}

function clampWindowCount(count: number): number {
  return Math.max(1, Math.min(4, count))
}

function createDefaultWindows(workspaceId: WorkspaceId, count: number): TerminalWindow[] {
  const safeCount = clampWindowCount(count)
  return Array.from({ length: safeCount }, (_, i) => ({
    id: `${workspaceId}-window-${i}`,
    boundSessions: [],
    activeSession: null,
    colorIndex: i,
  }))
}

function createDefaultWorkspace(workspaceId: WorkspaceId, count: number): TerminalWorkspace {
  return {
    windows: createDefaultWindows(workspaceId, count),
    windowCount: clampWindowCount(count),
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function migrateStoredState(raw: unknown): StoredStateV2 {
  if (isRecord(raw)) {
    // V2: workspaces already present
    if (isRecord(raw.workspaces) && isRecord(raw.workspaces.terminal1) && isRecord(raw.workspaces.terminal2)) {
      const sidebarCollapsed = typeof raw.sidebarCollapsed === 'boolean' ? raw.sidebarCollapsed : false
      const settings = (raw.settings as UserSettings) ?? DEFAULT_SETTINGS

      const toWorkspace = (workspaceId: WorkspaceId, wsRaw: unknown): TerminalWorkspace => {
        const ws = isRecord(wsRaw) ? wsRaw : {}
        const windowCount = clampWindowCount(typeof ws.windowCount === 'number' ? ws.windowCount : 2)
        const windowsRaw = Array.isArray(ws.windows) ? (ws.windows as TerminalWindow[]) : []

        const windows: TerminalWindow[] = Array.from({ length: windowCount }, (_, i) => {
          const existing = windowsRaw[i]
          return {
            id: `${workspaceId}-window-${i}`,
            boundSessions: existing?.boundSessions ?? [],
            activeSession: existing?.activeSession ?? null,
            colorIndex: typeof existing?.colorIndex === 'number' ? existing.colorIndex : i,
          }
        })

        return {
          windows,
          windowCount,
        }
      }

      return {
        workspaces: {
          terminal1: toWorkspace('terminal1', raw.workspaces.terminal1),
          terminal2: toWorkspace('terminal2', raw.workspaces.terminal2),
        },
        sidebarCollapsed,
        settings,
      }
    }

    // V1: migrate windows -> terminal1
    if (Array.isArray(raw.windows) && typeof raw.windowCount === 'number') {
      const sidebarCollapsed = typeof raw.sidebarCollapsed === 'boolean' ? raw.sidebarCollapsed : false
      const settings = (raw.settings as UserSettings) ?? DEFAULT_SETTINGS
      const windowCount = clampWindowCount(raw.windowCount)
      const windowsRaw = raw.windows as TerminalWindow[]

      const terminal1Windows: TerminalWindow[] = Array.from({ length: windowCount }, (_, i) => {
        const existing = windowsRaw[i]
        return {
          id: `terminal1-window-${i}`,
          boundSessions: existing?.boundSessions ?? [],
          activeSession: existing?.activeSession ?? null,
          colorIndex: typeof existing?.colorIndex === 'number' ? existing.colorIndex : i,
        }
      })

      return {
        workspaces: {
          terminal1: {
            windows: terminal1Windows,
            windowCount,
          },
          terminal2: createDefaultWorkspace('terminal2', 2),
        },
        sidebarCollapsed,
        settings,
      }
    }
  }

  // Default
  return {
    workspaces: {
      terminal1: createDefaultWorkspace('terminal1', 2),
      terminal2: createDefaultWorkspace('terminal2', 2),
    },
    sidebarCollapsed: false,
    settings: DEFAULT_SETTINGS,
  }
}

const SessionContext = createContext<DashboardContextType | null>(null)

export function SessionProvider({ children }: { children: ReactNode }) {
  // Load initial state from localStorage or use defaults
  const stored = useMemo(() => loadStoredState(), [])

  // Toast notifications
  const { addToast } = useToast()

  const [sessions, setSessions] = useState<TmuxSession[]>([])
  const [groupedSessions, setGroupedSessions] = useState<Record<string, TmuxSession[]>>({})
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [workspaces, setWorkspaces] = useState<Record<WorkspaceId, TerminalWorkspace>>(
    stored?.workspaces ?? {
      terminal1: createDefaultWorkspace('terminal1', 2),
      terminal2: createDefaultWorkspace('terminal2', 2),
    }
  )
  const [sidebarCollapsed, setSidebarCollapsed] = useState(stored?.sidebarCollapsed ?? false)
  const [floatingSession, setFloatingSession] = useState<string | null>(null)
  const [isDragging, setIsDragging] = useState(false)
  const [settings, setSettings] = useState<UserSettings>(stored?.settings ?? DEFAULT_SETTINGS)
  // Track which window has focus for keyboard navigation (workspaceId-windowId)
  const [focusedWindowKey, setFocusedWindowKey] = useState<string | null>(null)
  // Layout presets
  const [layoutPresets, setLayoutPresets] = useState<LayoutPreset[]>(() => loadStoredPresets())

  // Computed: which sessions are assigned to any window
  const assignedSessions = useMemo(() => {
    const assigned = new Map<string, { workspaceId: WorkspaceId; windowId: string; colorIndex: number; windowIndex: number }>()
    WORKSPACE_IDS.forEach(workspaceId => {
      const ws = workspaces[workspaceId]
      ws.windows.forEach((w, idx) => {
        w.boundSessions.forEach(s => {
          assigned.set(s, {
            workspaceId,
            windowId: w.id,
            colorIndex: w.colorIndex,
            windowIndex: idx + 1, // 1-based index for UI badge
          })
        })
      })
    })
    return assigned
  }, [workspaces])

  // Clean up any stuck INIT-PENDING windows on mount (from page refresh during creation)
  useEffect(() => {
    setWorkspaces(prev => {
      let changed = false
      const next: Record<WorkspaceId, TerminalWorkspace> = { ...prev }
      WORKSPACE_IDS.forEach(workspaceId => {
        const ws = prev[workspaceId]
        const hasStuck = ws.windows.some(w => w.activeSession === 'INIT-PENDING')
        if (!hasStuck) return
        changed = true
        next[workspaceId] = {
          ...ws,
          windows: ws.windows.map(w =>
            w.activeSession === 'INIT-PENDING'
              ? { ...w, activeSession: null }
              : w
          ),
        }
      })
      return changed ? next : prev
    })
  }, [])

  // Persist state to localStorage (filter out INIT-PENDING to avoid stuck state)
  useEffect(() => {
    const cleanWorkspaces: Record<WorkspaceId, TerminalWorkspace> = { ...workspaces }
    WORKSPACE_IDS.forEach(workspaceId => {
      const ws = workspaces[workspaceId]
      cleanWorkspaces[workspaceId] = {
        ...ws,
        windows: ws.windows.map(w =>
          w.activeSession === 'INIT-PENDING'
            ? { ...w, activeSession: null }
            : w
        ),
      }
    })

    saveState({ workspaces: cleanWorkspaces, sidebarCollapsed, settings })
  }, [workspaces, sidebarCollapsed, settings])

  // Persist presets to localStorage
  useEffect(() => {
    savePresets(layoutPresets)
  }, [layoutPresets])

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

        // NOTE: We intentionally do NOT clean up "orphaned" sessions here.
        // If a session is in the layout but not in the API list (e.g. server restart, network blip),
        // we want to PERSIST it in the UI rather than wiping the user's layout.
        // The terminal window will just show a disconnected state or error until it comes back.
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
  const setWindowCount = useCallback((workspaceId: WorkspaceId, count: number) => {
    const newCount = clampWindowCount(count)

    setWorkspaces(prev => {
      const ws = prev[workspaceId]
      if (!ws) return prev

      const nextWindows = (() => {
        if (newCount > ws.windows.length) {
          const newWindows = [...ws.windows]
          for (let i = ws.windows.length; i < newCount; i++) {
            newWindows.push({
              id: `${workspaceId}-window-${i}`,
              boundSessions: [],
              activeSession: null,
              colorIndex: i,
            })
          }
          return newWindows
        }

        if (newCount < ws.windows.length) {
          return ws.windows.slice(0, newCount)
        }

        return ws.windows
      })()

      return {
        ...prev,
        [workspaceId]: {
          ...ws,
          windowCount: newCount,
          windows: nextWindows,
        },
      }
    })
  }, [])

  const addSessionToWindow = useCallback((workspaceId: WorkspaceId, windowId: string, sessionName: string) => {
    setWorkspaces(prev => {
      // 1) Remove session from ALL windows across ALL workspaces
      const cleaned: Record<WorkspaceId, TerminalWorkspace> = { ...prev }
      let targetWasEmpty = false
      let targetFound = false

      WORKSPACE_IDS.forEach(wsId => {
        const ws = prev[wsId]
        const updatedWindows = ws.windows.map(w => {
          const isTarget = wsId === workspaceId && w.id === windowId
          const hasSession = w.boundSessions.includes(sessionName)

          if (!isTarget && !hasSession) return w

          if (!isTarget && hasSession) {
            const newBound = w.boundSessions.filter(s => s !== sessionName)
            return {
              ...w,
              boundSessions: newBound,
              activeSession: w.activeSession === sessionName ? (newBound[0] ?? null) : w.activeSession,
            }
          }

          // target window
          targetFound = true
          targetWasEmpty = w.boundSessions.length === 0 || (w.boundSessions.length === 1 && hasSession)
          return w
        })

        cleaned[wsId] = { ...ws, windows: updatedWindows }
      })

      if (!targetFound) return prev

      // 2) Add to target window (within the selected workspace)
      const ws = cleaned[workspaceId]
      const updatedWindows = ws.windows.map(w => {
        if (w.id !== windowId) return w
        if (w.boundSessions.includes(sessionName)) return w
        return {
          ...w,
          boundSessions: [...w.boundSessions, sessionName],
          activeSession: targetWasEmpty ? sessionName : w.activeSession,
        }
      })

      return {
        ...cleaned,
        [workspaceId]: { ...ws, windows: updatedWindows },
      }
    })
  }, [])

  const removeSessionFromWindow = useCallback((workspaceId: WorkspaceId, windowId: string, sessionName: string) => {
    setWorkspaces(prev => {
      const ws = prev[workspaceId]
      if (!ws) return prev

      const updatedWindows = ws.windows.map(w => {
        if (w.id !== windowId) return w
        if (!w.boundSessions.includes(sessionName)) return w

        const newBound = w.boundSessions.filter(s => s !== sessionName)
        return {
          ...w,
          boundSessions: newBound,
          activeSession: w.activeSession === sessionName ? (newBound[0] ?? null) : w.activeSession,
        }
      })

      return {
        ...prev,
        [workspaceId]: { ...ws, windows: updatedWindows },
      }
    })
  }, [])

  const setActiveSession = useCallback((workspaceId: WorkspaceId, windowId: string, sessionName: string) => {
    setWorkspaces(prev => {
      const ws = prev[workspaceId]
      if (!ws) return prev

      const updatedWindows = ws.windows.map(w => {
        if (w.id !== windowId) return w
        if (!w.boundSessions.includes(sessionName)) return w
        return { ...w, activeSession: sessionName }
      })

      return {
        ...prev,
        [workspaceId]: { ...ws, windows: updatedWindows },
      }
    })
  }, [])

  const cycleSession = useCallback((workspaceId: WorkspaceId, windowId: string, direction: 'prev' | 'next') => {
    setWorkspaces(prev => {
      const ws = prev[workspaceId]
      if (!ws) return prev

      const updatedWindows = ws.windows.map(w => {
        if (w.id !== windowId) return w
        if (w.boundSessions.length <= 1) return w

        const currentIndex = w.activeSession ? w.boundSessions.indexOf(w.activeSession) : 0
        const newIndex = direction === 'next'
          ? (currentIndex + 1) % w.boundSessions.length
          : (currentIndex - 1 + w.boundSessions.length) % w.boundSessions.length

        return { ...w, activeSession: w.boundSessions[newIndex] }
      })

      return {
        ...prev,
        [workspaceId]: { ...ws, windows: updatedWindows },
      }
    })
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
    // Check if session is already assigned to any window
    const assignment = assignedSessions.get(sessionName)
    if (assignment) {
      // Focus the session in its assigned window instead of opening modal
      setActiveSession(assignment.workspaceId, assignment.windowId, sessionName)
    } else {
      // Open floating modal for "peek" functionality
      openFloatingModal(sessionName)
    }
  }, [assignedSessions, setActiveSession, openFloatingModal])

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
        addToast(`Session '${sessionName}' deleted`, 'info')
        refreshSessions()
      } else {
        const errorText = await response.text()
        console.error('Failed to delete session:', errorText)
        addToast(`Failed to delete session`, 'error')
      }
    } catch (e) {
      console.error('Failed to delete session:', e)
      addToast(`Failed to delete session`, 'error')
    }
  }, [refreshSessions, addToast])

  const renameSession = useCallback(async (oldName: string, newName: string): Promise<boolean> => {
    try {
      const response = await fetch(`/api/tmux/sessions/${encodeURIComponent(oldName)}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ newName }),
      })
      if (response.ok) {
        // Update window bindings to use the new name
        setWorkspaces(prev => {
          const next: Record<WorkspaceId, TerminalWorkspace> = { ...prev }
          WORKSPACE_IDS.forEach(workspaceId => {
            const ws = prev[workspaceId]
            next[workspaceId] = {
              ...ws,
              windows: ws.windows.map(w => ({
                ...w,
                boundSessions: w.boundSessions.map(s => s === oldName ? newName : s),
                activeSession: w.activeSession === oldName ? newName : w.activeSession,
              })),
            }
          })
          return next
        })
        addToast(`Session renamed to '${newName}'`, 'success')
        refreshSessions()
        return true
      } else {
        const errorText = await response.text()
        console.error('Failed to rename session:', errorText)
        addToast(`Failed to rename session`, 'error')
        return false
      }
    } catch (e) {
      console.error('Failed to rename session:', e)
      addToast(`Failed to rename session`, 'error')
      return false
    }
  }, [refreshSessions, addToast])

  // Layout Preset Actions
  const saveCurrentLayout = useCallback((name: string): boolean => {
    if (layoutPresets.length >= MAX_PRESETS) {
      addToast(`Maximum ${MAX_PRESETS} presets reached`, 'warning')
      return false
    }

    const newPreset: LayoutPreset = {
      id: generatePresetId(),
      name,
      createdAt: Date.now(),
      workspaces: cloneWorkspaces(workspaces),
    }

    setLayoutPresets(prev => [...prev, newPreset])
    addToast(`Layout '${name}' saved`, 'success')
    return true
  }, [layoutPresets.length, workspaces, addToast])

  const loadPreset = useCallback((presetId: string) => {
    const preset = layoutPresets.find(p => p.id === presetId)
    if (!preset) {
      addToast('Preset not found', 'error')
      return
    }

    setWorkspaces(cloneWorkspaces(preset.workspaces))
    addToast(`Layout '${preset.name}' loaded`, 'info')
  }, [layoutPresets, addToast])

  const deletePreset = useCallback((presetId: string) => {
    const preset = layoutPresets.find(p => p.id === presetId)
    if (preset) {
      setLayoutPresets(prev => prev.filter(p => p.id !== presetId))
      addToast(`Preset '${preset.name}' deleted`, 'info')
    }
  }, [layoutPresets, addToast])

  const renamePreset = useCallback((presetId: string, newName: string) => {
    setLayoutPresets(prev => prev.map(p =>
      p.id === presetId ? { ...p, name: newName } : p
    ))
  }, [])

  // Memoize context value to prevent unnecessary rerenders of consuming components
  const contextValue: DashboardContextType = useMemo(() => ({
    // State
    sessions,
    groupedSessions,
    loading,
    error,
    workspaces,
    sidebarCollapsed,
    floatingSession,
    assignedSessions,
    isDragging,
    settings,
    focusedWindowKey,
    layoutPresets,

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
    deleteSession,
    renameSession,
    setIsDragging,
    updateSettings,
    setFocusedWindowKey,
    saveCurrentLayout,
    loadPreset,
    deletePreset,
    renamePreset,
  }), [
    sessions,
    groupedSessions,
    loading,
    error,
    workspaces,
    sidebarCollapsed,
    floatingSession,
    assignedSessions,
    isDragging,
    settings,
    focusedWindowKey,
    layoutPresets,
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
    deleteSession,
    renameSession,
    setIsDragging,
    updateSettings,
    setFocusedWindowKey,
    saveCurrentLayout,
    loadPreset,
    deletePreset,
    renamePreset,
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
