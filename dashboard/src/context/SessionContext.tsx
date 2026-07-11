import { createContext, useContext, useState, useEffect, useCallback, useMemo, useRef, ReactNode } from 'react'
import type { DashboardContextType, TmuxSession, SessionBankEntry, TerminalWindow, SessionsResponse, UserSettings, TmuxAppearance, WorkspaceId, TerminalWorkspace, LayoutPreset, LaunchUser, CreateSessionOptions, PersistentAgentPayload, SendToSessionPayload } from '../types'
import { DEFAULT_SETTINGS, DEFAULT_TMUX_APPEARANCE, MAX_PRESETS, TERMINAL_WORKSPACE_IDS, getSessionKey, getSessionPrefixForUser, normalizeTerminalUsers, resolveLaunchUser } from '../types'
import { useToast } from './ToastContext'

// Apply tmux appearance settings via API (hot-reload)
async function applyTmuxAppearance(appearance: TmuxAppearance): Promise<void> {
  try {
    await fetch('/api/tmux/appearance', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(appearance),
      signal: AbortSignal.timeout(10000),
    })
  } catch (e) {
    console.warn('Failed to apply tmux appearance:', e)
  }
}

// Apply tmux mouse mode via API (hot-reload). Mouse mode lets the scroll wheel
// scroll tmux history; it is a global tmux option affecting all sessions.
async function applyTmuxMouse(enabled: boolean): Promise<void> {
  try {
    await fetch('/api/tmux/mouse', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ enabled }),
      signal: AbortSignal.timeout(10000),
    })
  } catch (e) {
    console.warn('Failed to apply tmux mouse mode:', e)
  }
}

const STORAGE_KEY = 'chrote-dashboard-state'
const PRESETS_STORAGE_KEY = 'chrote-dashboard-presets'
const SETTINGS_SCHEMA_VERSION = 2

const WORKSPACE_IDS: WorkspaceId[] = [...TERMINAL_WORKSPACE_IDS]
const VIEWPORT_BUCKETS = ['mobile', 'tablet', 'desktop'] as const
type ViewportBucket = typeof VIEWPORT_BUCKETS[number]

// Load presets from localStorage
function loadStoredPresets(): LayoutPreset[] {
  try {
    const stored = localStorage.getItem(PRESETS_STORAGE_KEY)
    if (stored) {
      const parsed = JSON.parse(stored)
      if (Array.isArray(parsed)) {
        return parsed.filter(isRecord).map((preset) => ({
          id: typeof preset.id === 'string' ? preset.id : generatePresetId(),
          name: typeof preset.name === 'string' ? preset.name : 'Untitled',
          createdAt: typeof preset.createdAt === 'number' ? preset.createdAt : Date.now(),
          workspaces: sanitizeWorkspaces(preset.workspaces),
        }))
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

interface StoredLayout {
  workspaces: Record<WorkspaceId, TerminalWorkspace>
}

interface StoredStateV3 {
  version: 3
  settingsSchemaVersion?: number
  layoutsByViewport: Partial<Record<ViewportBucket, StoredLayout>>
  sidebarCollapsed: boolean
  settings: UserSettings
}

interface LoadedStoredState extends StoredStateV2 {
  layoutsByViewport: Partial<Record<ViewportBucket, StoredLayout>>
}

function getCurrentViewportBucket(): ViewportBucket {
  if (typeof window === 'undefined') return 'desktop'
  const width = window.innerWidth || document.documentElement.clientWidth || 1024
  if (width <= 768) return 'mobile'
  if (width <= 1180) return 'tablet'
  return 'desktop'
}

function isViewportBucket(value: string): value is ViewportBucket {
  return VIEWPORT_BUCKETS.includes(value as ViewportBucket)
}

function mergeTerminalLaunchUsers(raw: unknown): Record<WorkspaceId, LaunchUser> {
  const rawUsers = isRecord(raw) ? raw : {}
  return WORKSPACE_IDS.reduce((acc, workspaceId) => {
    const value = rawUsers[workspaceId]
    acc[workspaceId] = typeof value === 'string' ? value : ''
    return acc
  }, {} as Record<WorkspaceId, LaunchUser>)
}

function mergeStringMap(raw: unknown): Record<LaunchUser, string> {
  const source = isRecord(raw) ? raw : {}
  return Object.entries(source).reduce((acc, [key, value]) => {
    const user = key.trim()
    if (user && typeof value === 'string') acc[user] = value
    return acc
  }, {} as Record<LaunchUser, string>)
}

function mergeTerminalSessionPrefixes(raw: unknown, legacyPrefix: unknown, rawLaunchUsers: unknown): Record<LaunchUser, string> {
  const prefixes = mergeStringMap(raw)
  const legacy = typeof legacyPrefix === 'string' && legacyPrefix.trim() !== '' ? legacyPrefix : null
  if (legacy && Object.keys(prefixes).length === 0) {
    const users = normalizeTerminalUsers(Object.values(mergeTerminalLaunchUsers(rawLaunchUsers)))
    if (users[0]) prefixes[users[0]] = legacy
  }
  return prefixes
}

function mergeSettings(rawSettings: unknown): UserSettings {
  if (!isRecord(rawSettings)) return DEFAULT_SETTINGS

  const tmuxAppearance = isRecord(rawSettings.tmuxAppearance)
    ? { ...DEFAULT_TMUX_APPEARANCE, ...rawSettings.tmuxAppearance }
    : DEFAULT_TMUX_APPEARANCE
  const terminalLaunchUsers = mergeTerminalLaunchUsers(rawSettings.terminalLaunchUsers)
  const terminalSessionPrefixes = mergeTerminalSessionPrefixes(
    rawSettings.terminalSessionPrefixes,
    rawSettings.defaultSessionPrefix,
    rawSettings.terminalLaunchUsers,
  )
  const terminalLabels = mergeStringMap(rawSettings.terminalLabels)
  const terminalUserColors = mergeStringMap(rawSettings.terminalUserColors)

  return {
    ...DEFAULT_SETTINGS,
    ...rawSettings,
    terminalLaunchUsers,
    terminalSessionPrefixes,
    terminalLabels,
    terminalUserColors,
    tmuxAppearance,
  } as UserSettings
}

function migrateSettings(rawSettings: unknown, schemaVersion: unknown): UserSettings {
  const settings = mergeSettings(rawSettings)
  if (schemaVersion !== SETTINGS_SCHEMA_VERSION && settings.theme === 'matrix') {
    return {
      ...settings,
      theme: DEFAULT_SETTINGS.theme,
      tmuxAppearance: DEFAULT_SETTINGS.tmuxAppearance,
    }
  }
  return settings
}

function defaultStoredState(): LoadedStoredState {
  return {
    workspaces: {
      terminal1: createDefaultWorkspace('terminal1', 2),
      terminal2: createDefaultWorkspace('terminal2', 2),
      terminal3: createDefaultWorkspace('terminal3', 2),
    },
    layoutsByViewport: {},
    sidebarCollapsed: false,
    settings: DEFAULT_SETTINGS,
  }
}

function loadStoredState(viewportBucket: ViewportBucket = getCurrentViewportBucket()): LoadedStoredState | null {
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    if (stored) {
      const parsed: unknown = JSON.parse(stored)
      return migrateStoredState(parsed, viewportBucket)
    }
  } catch (e) {
    console.warn('Failed to load stored state:', e)
  }
  return null
}

function saveState(state: StoredStateV2, viewportBucket: ViewportBucket): void {
  try {
    const existing = loadStoredState(viewportBucket) ?? defaultStoredState()
    const next: StoredStateV3 = {
      version: 3,
      settingsSchemaVersion: SETTINGS_SCHEMA_VERSION,
      layoutsByViewport: {
        ...existing.layoutsByViewport,
        [viewportBucket]: { workspaces: cloneWorkspaces(state.workspaces) },
      },
      sidebarCollapsed: state.sidebarCollapsed,
      settings: state.settings,
    }
    localStorage.setItem(STORAGE_KEY, JSON.stringify(next))
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

function apiErrorMessage(raw: string, fallback: string): string {
  const text = raw.trim()
  if (!text) return fallback
  try {
    const parsed = JSON.parse(text) as unknown
    if (isRecord(parsed)) {
      const error = parsed.error
      if (isRecord(error) && typeof error.message === 'string' && error.message.trim()) {
        return error.message.trim()
      }
      if (typeof parsed.message === 'string' && parsed.message.trim()) {
        return parsed.message.trim()
      }
    }
  } catch {
    // Non-JSON responses are still useful if they are small enough to show directly.
  }
  return text.length <= 240 ? text : fallback
}

function nextSessionNameForPrefix(sessions: TmuxSession[], prefix: string): string {
  const escapedPrefix = prefix.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const regex = new RegExp(`^${escapedPrefix}(\\d+)$`)
  const existingNumbers = sessions
    .map(s => s.name.match(regex))
    .filter(Boolean)
    .map(m => parseInt(m![1], 10))

  const nextNum = existingNumbers.length > 0
    ? Math.max(...existingNumbers) + 1
    : 1

  return `${prefix}${nextNum}`
}

function liveSessionKeys(sessions: TmuxSession[]): Set<string> {
  const live = new Set<string>()
  sessions.forEach(s => {
    live.add(getSessionKey(s.name, s.unixUser))
    live.add(s.name) // backward compatibility for layouts saved before user-qualified keys
  })
  return live
}

function pruneWindowToLiveSessions(window: TerminalWindow, live: Set<string>, pruneCandidates?: Set<string>): TerminalWindow {
  const boundSessions = window.boundSessions.filter(s => (
    s === 'INIT-PENDING' || live.has(s) || (pruneCandidates ? !pruneCandidates.has(s) : false)
  ))
  const activeSession = window.activeSession && boundSessions.includes(window.activeSession)
    ? window.activeSession
    : (pruneCandidates
        ? (boundSessions.find(sessionName => sessionName === 'INIT-PENDING' || live.has(sessionName)) ?? null)
        : (boundSessions[0] ?? null))

  if (boundSessions.length === window.boundSessions.length && activeSession === window.activeSession) {
    return window
  }

  return { ...window, boundSessions, activeSession }
}

function pruneWorkspacesToLiveSessions(
  workspaces: Record<WorkspaceId, TerminalWorkspace>,
  sessions: TmuxSession[],
  pruneCandidates?: Set<string>,
): Record<WorkspaceId, TerminalWorkspace> {
  const live = liveSessionKeys(sessions)
  let changed = false
  const next: Record<WorkspaceId, TerminalWorkspace> = { ...workspaces }

  WORKSPACE_IDS.forEach(workspaceId => {
    const ws = workspaces[workspaceId]
    if (!ws) return
    const windows = ws.windows.map(w => {
      const pruned = pruneWindowToLiveSessions(w, live, pruneCandidates)
      if (pruned !== w) changed = true
      return pruned
    })
    next[workspaceId] = windows === ws.windows ? ws : { ...ws, windows }
  })

  return changed ? next : workspaces
}

function staleSessionKeysInWorkspaces(workspaces: Record<WorkspaceId, TerminalWorkspace>, live: Set<string>): Set<string> {
  const stale = new Set<string>()
  WORKSPACE_IDS.forEach(workspaceId => {
    const ws = workspaces[workspaceId]
    if (!ws) return
    ws.windows.forEach(w => {
      w.boundSessions.forEach(sessionName => {
        if (sessionName !== 'INIT-PENDING' && !live.has(sessionName)) stale.add(sessionName)
      })
    })
  })
  return stale
}

function sanitizeWorkspace(workspaceId: WorkspaceId, wsRaw: unknown): TerminalWorkspace {
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

function sanitizeWorkspaces(rawWorkspaces: unknown): Record<WorkspaceId, TerminalWorkspace> {
  const workspaces = isRecord(rawWorkspaces) ? rawWorkspaces : {}
  return WORKSPACE_IDS.reduce((acc, workspaceId) => {
    acc[workspaceId] = sanitizeWorkspace(workspaceId, workspaces[workspaceId])
    return acc
  }, {} as Record<WorkspaceId, TerminalWorkspace>)
}

function migrateStoredState(raw: unknown, viewportBucket: ViewportBucket): LoadedStoredState {
  if (isRecord(raw)) {
    if (raw.version === 3 && isRecord(raw.layoutsByViewport)) {
      const layoutsByViewport: Partial<Record<ViewportBucket, StoredLayout>> = {}

      Object.entries(raw.layoutsByViewport).forEach(([key, value]) => {
        if (!isViewportBucket(key) || !isRecord(value)) return
        layoutsByViewport[key] = {
          workspaces: sanitizeWorkspaces(value.workspaces),
        }
      })

      return {
        workspaces: layoutsByViewport[viewportBucket]?.workspaces ?? defaultStoredState().workspaces,
        layoutsByViewport,
        sidebarCollapsed: typeof raw.sidebarCollapsed === 'boolean' ? raw.sidebarCollapsed : false,
        settings: migrateSettings(raw.settings, raw.settingsSchemaVersion),
      }
    }

    // V2: workspaces already present
    if (isRecord(raw.workspaces) && isRecord(raw.workspaces.terminal1) && isRecord(raw.workspaces.terminal2)) {
      const sidebarCollapsed = typeof raw.sidebarCollapsed === 'boolean' ? raw.sidebarCollapsed : false
      const settings = migrateSettings(raw.settings, raw.settingsSchemaVersion)
      const workspaces = sanitizeWorkspaces(raw.workspaces)

      return {
        workspaces,
        layoutsByViewport: {
          [viewportBucket]: { workspaces },
        },
        sidebarCollapsed,
        settings,
      }
    }

    // V1: migrate windows -> terminal1
    if (Array.isArray(raw.windows) && typeof raw.windowCount === 'number') {
      const sidebarCollapsed = typeof raw.sidebarCollapsed === 'boolean' ? raw.sidebarCollapsed : false
      const settings = migrateSettings(raw.settings, raw.settingsSchemaVersion)
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

      const workspaces: Record<WorkspaceId, TerminalWorkspace> = {
        terminal1: {
          windows: terminal1Windows,
          windowCount,
        },
        terminal2: createDefaultWorkspace('terminal2', 2),
        terminal3: createDefaultWorkspace('terminal3', 2),
      }

      return {
        workspaces,
        layoutsByViewport: {
          [viewportBucket]: { workspaces },
        },
        sidebarCollapsed,
        settings,
      }
    }
  }

  // Default
  return defaultStoredState()
}

const SessionContext = createContext<DashboardContextType | null>(null)

export function SessionProvider({ children }: { children: ReactNode }) {
  const viewportBucket = useMemo(() => getCurrentViewportBucket(), [])
  // Load initial state from localStorage or use defaults
  const stored = useMemo(() => loadStoredState(viewportBucket), [viewportBucket])

  // Toast notifications
  const { addToast } = useToast()

  const [sessions, setSessions] = useState<TmuxSession[]>([])
  const [groupedSessions, setGroupedSessions] = useState<Record<string, TmuxSession[]>>({})
  const [sessionBank, setSessionBank] = useState<SessionBankEntry[]>([])
  const [terminalUsers, setTerminalUsers] = useState<LaunchUser[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [workspaces, setWorkspaces] = useState<Record<WorkspaceId, TerminalWorkspace>>(
    stored?.workspaces ?? WORKSPACE_IDS.reduce((acc, workspaceId) => {
      acc[workspaceId] = createDefaultWorkspace(workspaceId, 2)
      return acc
    }, {} as Record<WorkspaceId, TerminalWorkspace>)
  )
  const [sidebarCollapsed, setSidebarCollapsed] = useState(stored?.sidebarCollapsed ?? false)
  const [floatingSession, setFloatingSession] = useState<string | null>(null)
  const [sendToSessionTarget, setSendToSessionTarget] = useState<string | null>(null)
  const [isDragging, setIsDragging] = useState(false)
  const [settings, setSettings] = useState<UserSettings>(stored?.settings ?? DEFAULT_SETTINGS)
  // Track which window has focus for keyboard navigation (workspaceId-windowId)
  const [focusedWindowKey, setFocusedWindowKey] = useState<string | null>(null)
  // Layout presets
  const [layoutPresets, setLayoutPresets] = useState<LayoutPreset[]>(() => loadStoredPresets())
  const staleSessionCandidatesRef = useRef<Set<string>>(new Set())
  const staleSessionProtectionRef = useRef<Map<string, number>>(new Map())
  const protectStaleSessionAliases = useCallback((aliases: string[]) => {
    aliases.forEach(alias => {
      if (alias) staleSessionProtectionRef.current.set(alias, 2)
    })
  }, [])

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

    saveState({ workspaces: cleanWorkspaces, sidebarCollapsed, settings }, viewportBucket)
  }, [workspaces, sidebarCollapsed, settings, viewportBucket])

  // Persist presets to localStorage
  useEffect(() => {
    savePresets(layoutPresets)
  }, [layoutPresets])

  // Fetch sessions from API
  const refreshSessions = useCallback(async () => {
    try {
      const response = await fetch('/api/tmux/sessions', { signal: AbortSignal.timeout(10000) })
      const data = await response.json().catch(() => ({})) as Partial<SessionsResponse>

      if (Array.isArray(data.terminalUsers)) {
        setTerminalUsers(normalizeTerminalUsers(data.terminalUsers))
      }

      if (!response.ok) {
        setError(typeof data.error === 'string' ? data.error : 'Failed to fetch sessions')
        return
      }

      if (data.error) {
        setError(data.error)
      } else {
        setError(null)
      }
      const nextSessions = Array.isArray(data.sessions) ? data.sessions : []
      setSessions(nextSessions)
      setGroupedSessions(isRecord(data.grouped) ? data.grouped as Record<string, TmuxSession[]> : {})
      setSessionBank(Array.isArray(data.banked) ? data.banked : [])

      if (!data.error && Array.isArray(data.sessions)) {
        const live = liveSessionKeys(nextSessions)
        const protectedKeys = new Set(staleSessionProtectionRef.current.keys())
        const pruneCandidates = new Set([...staleSessionCandidatesRef.current].filter(key => !protectedKeys.has(key)))
        setWorkspaces(prev => {
          const pruned = pruneWorkspacesToLiveSessions(prev, nextSessions, pruneCandidates)
          const currentStale = staleSessionKeysInWorkspaces(pruned, live)
          protectedKeys.forEach(key => currentStale.delete(key))
          staleSessionCandidatesRef.current = currentStale
          return pruned
        })
        setFloatingSession(prev => prev && (live.has(prev) || protectedKeys.has(prev) || !pruneCandidates.has(prev)) ? prev : null)
        setSendToSessionTarget(prev => prev && (live.has(prev) || protectedKeys.has(prev) || !pruneCandidates.has(prev)) ? prev : null)
        staleSessionProtectionRef.current.forEach((remaining, key) => {
          if (remaining <= 1) {
            staleSessionProtectionRef.current.delete(key)
          } else {
            staleSessionProtectionRef.current.set(key, remaining - 1)
          }
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

  // Apply tmux appearance and mouse mode on initial load (ensures container matches saved settings)
  useEffect(() => {
    // Merge saved settings with defaults to handle missing fields from older localStorage
    const appearance = { ...DEFAULT_TMUX_APPEARANCE, ...settings.tmuxAppearance }
    applyTmuxAppearance(appearance)
    applyTmuxMouse(settings.mouseScroll)
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

  const clearWorkspaceAssignments = useCallback((workspaceId: WorkspaceId) => {
    setWorkspaces(prev => {
      const ws = prev[workspaceId]
      if (!ws) return prev
      return {
        ...prev,
        [workspaceId]: {
          ...ws,
          windows: ws.windows.map(w => ({ ...w, boundSessions: [], activeSession: null })),
        },
      }
    })
  }, [])

  const clearStaleSessionsFromWindow = useCallback((workspaceId: WorkspaceId, windowId: string) => {
    const liveSessions = liveSessionKeys(sessions)
    setWorkspaces(prev => {
      const ws = prev[workspaceId]
      if (!ws) return prev
      return {
        ...prev,
        [workspaceId]: {
          ...ws,
          windows: ws.windows.map(w => w.id === windowId ? pruneWindowToLiveSessions(w, liveSessions) : w),
        },
      }
    })
  }, [sessions])

  const addSessionToWindow = useCallback((workspaceId: WorkspaceId, windowId: string, sessionName: string, unixUser?: LaunchUser) => {
    const sessionKey = getSessionKey(sessionName, unixUser)
    const aliases = unixUser ? [sessionKey, sessionName] : [sessionKey]
    aliases.forEach(alias => staleSessionCandidatesRef.current.delete(alias))
    protectStaleSessionAliases(aliases)
    setWorkspaces(prev => {
      // 1) Remove session from ALL windows across ALL workspaces
      const cleaned: Record<WorkspaceId, TerminalWorkspace> = { ...prev }
      let targetWasEmpty = false
      let targetFound = false

      WORKSPACE_IDS.forEach(wsId => {
        const ws = prev[wsId]
        const updatedWindows = ws.windows.map(w => {
          const isTarget = wsId === workspaceId && w.id === windowId
          const hasSession = aliases.some(alias => w.boundSessions.includes(alias))

          if (!isTarget && !hasSession) return w

          if (!isTarget && hasSession) {
            const newBound = w.boundSessions.filter(s => !aliases.includes(s))
            return {
              ...w,
              boundSessions: newBound,
              activeSession: w.activeSession && aliases.includes(w.activeSession) ? (newBound[0] ?? null) : w.activeSession,
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
        const boundSessions = w.boundSessions.filter(s => !aliases.includes(s))
        return {
          ...w,
          boundSessions: [...boundSessions, sessionKey],
          activeSession: targetWasEmpty || (w.activeSession && aliases.includes(w.activeSession)) ? sessionKey : w.activeSession,
        }
      })

      return {
        ...cleaned,
        [workspaceId]: { ...ws, windows: updatedWindows },
      }
    })
  }, [protectStaleSessionAliases])

  const createSession = useCallback(async (options: CreateSessionOptions = {}): Promise<string | null> => {
    const workspaceId = options.workspaceId ?? options.attachTo?.workspaceId ?? 'terminal1'
    const unixUser = options.unixUser ?? resolveLaunchUser(settings, workspaceId, terminalUsers)
    const explicitName = options.name?.trim()
    const prefix = getSessionPrefixForUser(settings, unixUser, terminalUsers)
    const sessionName = explicitName || nextSessionNameForPrefix(sessions, prefix)

    try {
      const response = await fetch('/api/tmux/sessions', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: sessionName, unixUser, mouseScroll: options.mouseScroll ?? settings.mouseScroll }),
        signal: AbortSignal.timeout(10000),
      })

      if (!response.ok) {
        addToast('Failed to create session', 'error')
        return null
      }

      addToast(`Session '${sessionName}' created`, 'success')
      if (options.attachTo) {
        addSessionToWindow(options.attachTo.workspaceId, options.attachTo.windowId, sessionName, unixUser)
      }
      void refreshSessions()
      return sessionName
    } catch (e) {
      console.error('Failed to create session:', e)
      addToast('Failed to create session', 'error')
      return null
    }
  }, [addSessionToWindow, addToast, refreshSessions, sessions, settings, terminalUsers])

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

  const openSendToSession = useCallback((sessionName: string) => {
    setSendToSessionTarget(sessionName)
  }, [])

  const closeSendToSession = useCallback(() => {
    setSendToSessionTarget(null)
  }, [])

  const sendToSession = useCallback(async (sessionName: string, payload: SendToSessionPayload, unixUser?: LaunchUser): Promise<boolean> => {
    try {
      const form = new FormData()
      form.set('text', payload.text)
      form.set('submit', payload.submit ? 'true' : 'false')
      payload.files.forEach(file => form.append('files', file, file.name))
      const query = unixUser ? `?unixUser=${encodeURIComponent(unixUser)}` : ''
      const response = await fetch(`/api/tmux/sessions/${encodeURIComponent(sessionName)}/send${query}`, {
        method: 'POST',
        body: form,
        signal: AbortSignal.timeout(30000),
      })
      if (!response.ok) {
        const errorText = await response.text()
        const message = apiErrorMessage(errorText, 'Failed to send to session')
        console.error('Failed to send to session:', errorText)
        addToast(message, 'error')
        return false
      }
      addToast(`Sent to '${sessionName}'`, 'success')
      return true
    } catch (e) {
      console.error('Failed to send to session:', e)
      addToast('Failed to send to session', 'error')
      return false
    }
  }, [addToast])

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
      // Hot-reload tmux mouse mode if it changed
      if (newSettings.mouseScroll !== undefined) {
        applyTmuxMouse(updated.mouseScroll)
      }
      return updated
    })
  }, [])

  const deleteSession = useCallback(async (sessionName: string, unixUser?: LaunchUser) => {
    try {
      const query = unixUser ? `?unixUser=${encodeURIComponent(unixUser)}` : ''
      const response = await fetch(`/api/tmux/sessions/${encodeURIComponent(sessionName)}${query}`, {
        method: 'DELETE',
        signal: AbortSignal.timeout(10000),
      })
      if (response.ok) {
        const deletedKey = getSessionKey(sessionName, unixUser)
        setWorkspaces(prev => {
          const next: Record<WorkspaceId, TerminalWorkspace> = { ...prev }
          WORKSPACE_IDS.forEach(workspaceId => {
            const ws = prev[workspaceId]
            next[workspaceId] = {
              ...ws,
              windows: ws.windows.map(w => {
                const boundSessions = w.boundSessions.filter(s => s !== deletedKey && s !== sessionName)
                return {
                  ...w,
                  boundSessions,
                  activeSession: w.activeSession && boundSessions.includes(w.activeSession) ? w.activeSession : (boundSessions[0] ?? null),
                }
              }),
            }
          })
          return next
        })
        addToast(`Session '${sessionName}' deleted`, 'info')
        refreshSessions()
        return true
      } else {
        const errorText = await response.text()
        console.error('Failed to delete session:', errorText)
        addToast(`Failed to delete session`, 'error')
        return false
      }
    } catch (e) {
      console.error('Failed to delete session:', e)
      addToast(`Failed to delete session`, 'error')
      return false
    }
  }, [refreshSessions, addToast])

  const renameSession = useCallback(async (oldName: string, newName: string, unixUser?: LaunchUser): Promise<boolean> => {
    try {
      const query = unixUser ? `?unixUser=${encodeURIComponent(unixUser)}` : ''
      const response = await fetch(`/api/tmux/sessions/${encodeURIComponent(oldName)}${query}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ newName }),
        signal: AbortSignal.timeout(10000),
      })
      if (response.ok) {
        const oldKey = getSessionKey(oldName, unixUser)
        const newKey = getSessionKey(newName, unixUser)
        const renameAliases = [oldKey, oldName, newKey, newName]
        renameAliases.forEach(alias => staleSessionCandidatesRef.current.delete(alias))
        protectStaleSessionAliases(renameAliases)
        // Update window bindings to use the new name/key
        setWorkspaces(prev => {
          const next: Record<WorkspaceId, TerminalWorkspace> = { ...prev }
          WORKSPACE_IDS.forEach(workspaceId => {
            const ws = prev[workspaceId]
            next[workspaceId] = {
              ...ws,
              windows: ws.windows.map(w => ({
                ...w,
                boundSessions: w.boundSessions.map(s => s === oldKey || s === oldName ? newKey : s),
                activeSession: w.activeSession === oldKey || w.activeSession === oldName ? newKey : w.activeSession,
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
  }, [refreshSessions, addToast, protectStaleSessionAliases])


  const makeSessionPersistent = useCallback(async (sessionName: string, payload: PersistentAgentPayload, unixUser?: LaunchUser): Promise<boolean> => {
    try {
      const query = unixUser ? `?unixUser=${encodeURIComponent(unixUser)}` : ''
      const body: Record<string, string> = {}
      const agentKind = payload.agentKind?.trim()
      if (agentKind) body.agentKind = agentKind
      const agentSessionId = payload.agentSessionId?.trim()
      if (agentSessionId) body.agentSessionId = agentSessionId
      const identity = payload.identity?.trim()
      if (identity) body.identity = identity
      const newName = payload.newName?.trim()
      if (newName) body.newName = newName
      const cwd = payload.cwd?.trim()
      if (cwd) body.cwd = cwd
      const transcriptPath = payload.transcriptPath?.trim()
      if (transcriptPath) body.transcriptPath = transcriptPath

      const response = await fetch(`/api/tmux/sessions/${encodeURIComponent(sessionName)}/persistence${query}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
        signal: AbortSignal.timeout(10000),
      })
      if (!response.ok) {
        const errorText = await response.text()
        const message = apiErrorMessage(errorText, 'Failed to make session persistent')
        console.error('Failed to make session persistent:', errorText)
        addToast(message, 'error')
        return false
      }
      addToast(`Session '${sessionName}' is persistent`, 'success')
      refreshSessions()
      return true
    } catch (e) {
      console.error('Failed to make session persistent:', e)
      addToast('Failed to make session persistent', 'error')
      return false
    }
  }, [addToast, refreshSessions])

  const makeSessionMortal = useCallback(async (sessionName: string, unixUser?: LaunchUser): Promise<boolean> => {
    try {
      const query = unixUser ? `?unixUser=${encodeURIComponent(unixUser)}` : ''
      const response = await fetch(`/api/tmux/sessions/${encodeURIComponent(sessionName)}/persistence${query}`, {
        method: 'DELETE',
        signal: AbortSignal.timeout(10000),
      })
      if (!response.ok) {
        const errorText = await response.text()
        console.error('Failed to make session mortal:', errorText)
        addToast('Failed to make session mortal', 'error')
        return false
      }
      addToast(`Session '${sessionName}' is mortal`, 'info')
      refreshSessions()
      return true
    } catch (e) {
      console.error('Failed to make session mortal:', e)
      addToast('Failed to make session mortal', 'error')
      return false
    }
  }, [addToast, refreshSessions])


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

    setWorkspaces(sanitizeWorkspaces(cloneWorkspaces(preset.workspaces)))
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
    sessionBank,
    terminalUsers,
    loading,
    error,
    workspaces,
    sidebarCollapsed,
    floatingSession,
    sendToSessionTarget,
    assignedSessions,
    isDragging,
    settings,
    focusedWindowKey,
    layoutPresets,

    // Actions
    setWindowCount,
    clearWorkspaceAssignments,
    clearStaleSessionsFromWindow,
    addSessionToWindow,
    removeSessionFromWindow,
    setActiveSession,
    cycleSession,
    toggleSidebar,
    openFloatingModal,
    closeFloatingModal,
    openSendToSession,
    closeSendToSession,
    sendToSession,
    handleSessionClick,
    refreshSessions,
    createSession,
    deleteSession,
    renameSession,
    makeSessionPersistent,
    makeSessionMortal,
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
    sessionBank,
    terminalUsers,
    loading,
    error,
    workspaces,
    sidebarCollapsed,
    floatingSession,
    sendToSessionTarget,
    assignedSessions,
    isDragging,
    settings,
    focusedWindowKey,
    layoutPresets,
    setWindowCount,
    clearWorkspaceAssignments,
    clearStaleSessionsFromWindow,
    addSessionToWindow,
    removeSessionFromWindow,
    setActiveSession,
    cycleSession,
    toggleSidebar,
    openFloatingModal,
    closeFloatingModal,
    openSendToSession,
    closeSendToSession,
    sendToSession,
    handleSessionClick,
    refreshSessions,
    createSession,
    deleteSession,
    renameSession,
    makeSessionPersistent,
    makeSessionMortal,
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
