import { createContext, useContext, useState, useEffect, useCallback, useMemo, useRef, ReactNode } from 'react'
import type { DashboardContextType, TmuxSession, SessionBankEntry, ManagedRecoveryStatusEntry, TerminalWindow, SessionsResponse, UserSettings, TmuxAppearance, WorkspaceId, TerminalWorkspace, LayoutPreset, LaunchUser, CreateSessionOptions, PersistentAgentPayload, SendSessionPane, SendToSessionOutcome, SendToSessionPayload, SendToSessionResult, WindowRevealRequest } from '../types'
import { DEFAULT_SETTINGS, DEFAULT_TMUX_APPEARANCE, MAX_PRESETS, getSessionKey, getSessionNameFromKey, getSessionPrefixForUser, getSessionUserFromKey, normalizeTerminalTabCount, normalizeTerminalUsers, resolveLaunchUser, sortTerminalWorkspaceIds, terminalWorkspaceIds } from '../types'
import { useToast } from './ToastContext'
import { apiErrorMessage } from '../apiErrors'

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

// Shape check for workspace ids as they appear in persisted data. Broader than
// the visible list on purpose: hidden workspaces (count shrunk) must keep
// round-tripping through storage untouched.
const STORED_WORKSPACE_ID_PATTERN = /^terminal[1-9]\d*$/

function isStoredWorkspaceId(value: string): value is WorkspaceId {
  return STORED_WORKSPACE_ID_PATTERN.test(value)
}

function visibleWorkspaceIds(settings: UserSettings): WorkspaceId[] {
  return terminalWorkspaceIds(normalizeTerminalTabCount(settings.terminalTabCount))
}

function idsInWorkspaces(workspaces: Record<WorkspaceId, TerminalWorkspace>): WorkspaceId[] {
  return sortTerminalWorkspaceIds(Object.keys(workspaces) as WorkspaceId[])
}

const CANONICAL_WINDOW_COUNT = 4
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
          workspaces: sanitizeWorkspaces(preset.workspaces, []),
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
  // Sparse: an absent entry and '' mean the same thing to resolveLaunchUser,
  // so only meaningful assignments are kept. Entries for hidden workspaces
  // (beyond the current tab count) are retained, not filtered.
  return Object.entries(rawUsers).reduce((acc, [key, value]) => {
    if (isStoredWorkspaceId(key) && typeof value === 'string' && value !== '') acc[key] = value
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
    terminalTabCount: normalizeTerminalTabCount(rawSettings.terminalTabCount),
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

function defaultWorkspacesFor(ids: readonly WorkspaceId[]): Record<WorkspaceId, TerminalWorkspace> {
  return ids.reduce((acc, workspaceId) => {
    acc[workspaceId] = createDefaultWorkspace(workspaceId, 2)
    return acc
  }, {} as Record<WorkspaceId, TerminalWorkspace>)
}

function defaultStoredState(): LoadedStoredState {
  return {
    workspaces: defaultWorkspacesFor(visibleWorkspaceIds(DEFAULT_SETTINGS)),
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
        [viewportBucket]: { workspaces: sanitizeWorkspaces(state.workspaces, visibleWorkspaceIds(state.settings)) },
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

function createDefaultWindows(workspaceId: WorkspaceId): TerminalWindow[] {
  return Array.from({ length: CANONICAL_WINDOW_COUNT }, (_, i) => ({
    id: `${workspaceId}-window-${i}`,
    boundSessions: [],
    activeSession: null,
    colorIndex: i,
  }))
}

function createDefaultWorkspace(workspaceId: WorkspaceId, count: number): TerminalWorkspace {
  return {
    windows: createDefaultWindows(workspaceId),
    windowCount: clampWindowCount(count),
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

const definitiveSendErrorCodes = new Map<number, ReadonlySet<string>>([
  [400, new Set(['BAD_REQUEST'])],
  [404, new Set(['SESSION_NOT_FOUND'])],
  [408, new Set(['REQUEST_CANCELLED'])],
  [409, new Set(['PANE_REQUIRED', 'PANE_NOT_IN_SESSION', 'TARGET_CHANGED'])],
])

function definitiveSendErrorMessage(status: number, raw: string): string | null {
  const allowedCodes = definitiveSendErrorCodes.get(status)
  if (!allowedCodes) return null
  try {
    const parsed = JSON.parse(raw) as unknown
    if (!isRecord(parsed) || parsed.success !== false || !isRecord(parsed.error) ||
        typeof parsed.timestamp !== 'string' || Number.isNaN(Date.parse(parsed.timestamp))) {
      return null
    }
    const code = parsed.error.code
    const message = parsed.error.message
    if (typeof code !== 'string' || !allowedCodes.has(code) || typeof message !== 'string' || !message.trim()) {
      return null
    }
    return message.trim()
  } catch {
    return null
  }
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
    if (s.persistentSessionMissing) return
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

  idsInWorkspaces(workspaces).forEach(workspaceId => {
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
  idsInWorkspaces(workspaces).forEach(workspaceId => {
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

function sanitizeWorkspaceSlots(workspaceId: WorkspaceId, wsRaw: unknown): TerminalWorkspace {
  const ws = isRecord(wsRaw) ? wsRaw : {}
  const windowCount = clampWindowCount(typeof ws.windowCount === 'number' ? ws.windowCount : 2)
  const windowsRaw = Array.isArray(ws.windows) ? ws.windows : []

  const windows: TerminalWindow[] = Array.from({ length: CANONICAL_WINDOW_COUNT }, (_, i) => {
    const existing = isRecord(windowsRaw[i]) ? windowsRaw[i] : {}
    const boundSessions = Array.isArray(existing.boundSessions)
      ? existing.boundSessions.filter((session): session is string => typeof session === 'string' && session.length > 0)
      : []
    const activeSession = existing.activeSession === 'INIT-PENDING'
      ? existing.activeSession
      : (typeof existing.activeSession === 'string'
          ? (boundSessions.includes(existing.activeSession) ? existing.activeSession : (boundSessions[0] ?? null))
          : null)
    return {
      id: `${workspaceId}-window-${i}`,
      boundSessions,
      activeSession,
      colorIndex: typeof existing.colorIndex === 'number' ? existing.colorIndex : i,
    }
  })

  return { windows, windowCount }
}

function qualifiedUsersBySessionName(workspaces: Record<WorkspaceId, TerminalWorkspace>): Map<string, Set<LaunchUser>> {
  const qualifiedUsers = new Map<string, Set<LaunchUser>>()
  idsInWorkspaces(workspaces).forEach(workspaceId => {
    workspaces[workspaceId].windows.forEach(window => {
      window.boundSessions.forEach(sessionKey => {
        const unixUser = getSessionUserFromKey(sessionKey)
        if (!unixUser) return
        const sessionName = getSessionNameFromKey(sessionKey)
        const users = qualifiedUsers.get(sessionName) ?? new Set<LaunchUser>()
        users.add(unixUser)
        qualifiedUsers.set(sessionName, users)
      })
    })
  })
  return qualifiedUsers
}

function sessionBindingIdentity(sessionKey: string, qualifiedUsers: Map<string, Set<LaunchUser>>): string {
  const sessionName = getSessionNameFromKey(sessionKey)
  const unixUser = getSessionUserFromKey(sessionKey)
  if (unixUser) return `qualified:${getSessionKey(sessionName, unixUser)}`

  const users = qualifiedUsers.get(sessionName)
  if (users?.size === 1) {
    return `qualified:${getSessionKey(sessionName, users.values().next().value ?? '')}`
  }
  return `bare:${sessionKey}`
}

function safeSessionAliases(
  sessionName: string,
  unixUser: LaunchUser | undefined,
  qualifiedUsers: Map<string, Set<LaunchUser>>,
): string[] {
  const sessionKey = getSessionKey(sessionName, unixUser)
  if (!unixUser) return [sessionKey]

  const users = new Set(qualifiedUsers.get(sessionName) ?? [])
  users.add(unixUser)
  return users.size === 1 ? [sessionKey, sessionName] : [sessionKey]
}

function deduplicateWorkspaceBindings(
  workspaces: Record<WorkspaceId, TerminalWorkspace>,
  targetSessionKey?: string,
): Record<WorkspaceId, TerminalWorkspace> {
  const qualifiedUsers = qualifiedUsersBySessionName(workspaces)
  const targetIdentity = targetSessionKey
    ? sessionBindingIdentity(targetSessionKey, qualifiedUsers)
    : null
  const seen = new Set<string>()

  return idsInWorkspaces(workspaces).reduce((next, workspaceId) => {
    const workspace = workspaces[workspaceId]
    next[workspaceId] = {
      ...workspace,
      windows: workspace.windows.map(window => {
        const originalBound = window.boundSessions
        const boundSessions = originalBound.filter(sessionKey => {
          const identity = sessionBindingIdentity(sessionKey, qualifiedUsers)
          if (targetIdentity !== null && identity !== targetIdentity) return true
          if (seen.has(identity)) return false
          seen.add(identity)
          return true
        })
        const activeWasRemoved = window.activeSession !== null
          && window.activeSession !== 'INIT-PENDING'
          && !boundSessions.includes(window.activeSession)
        return {
          ...window,
          boundSessions,
          activeSession: activeWasRemoved ? (boundSessions[0] ?? null) : window.activeSession,
        }
      }),
    }
    return next
  }, {} as Record<WorkspaceId, TerminalWorkspace>)
}

// Guarantees every id in `ids` exists (defaulting missing ones) and keeps any
// extra stored terminal ids intact — hidden workspaces survive sanitization.
function sanitizeWorkspaces(rawWorkspaces: unknown, ids: readonly WorkspaceId[]): Record<WorkspaceId, TerminalWorkspace> {
  const raw = isRecord(rawWorkspaces) ? rawWorkspaces : {}
  const canonicalIds = new Set<WorkspaceId>(ids)
  Object.keys(raw).forEach(key => {
    if (isStoredWorkspaceId(key)) canonicalIds.add(key)
  })
  const canonical = sortTerminalWorkspaceIds([...canonicalIds]).reduce((workspaces, workspaceId) => {
    workspaces[workspaceId] = sanitizeWorkspaceSlots(workspaceId, raw[workspaceId])
    return workspaces
  }, {} as Record<WorkspaceId, TerminalWorkspace>)
  return deduplicateWorkspaceBindings(canonical)
}

function migrateStoredState(raw: unknown, viewportBucket: ViewportBucket): LoadedStoredState {
  if (isRecord(raw)) {
    if (raw.version === 3 && isRecord(raw.layoutsByViewport)) {
      const settings = migrateSettings(raw.settings, raw.settingsSchemaVersion)
      const ids = visibleWorkspaceIds(settings)
      const layoutsByViewport: Partial<Record<ViewportBucket, StoredLayout>> = {}

      Object.entries(raw.layoutsByViewport).forEach(([key, value]) => {
        if (!isViewportBucket(key) || !isRecord(value)) return
        layoutsByViewport[key] = {
          workspaces: sanitizeWorkspaces(value.workspaces, ids),
        }
      })

      return {
        workspaces: layoutsByViewport[viewportBucket]?.workspaces ?? defaultWorkspacesFor(ids),
        layoutsByViewport,
        sidebarCollapsed: typeof raw.sidebarCollapsed === 'boolean' ? raw.sidebarCollapsed : false,
        settings,
      }
    }

    // V2: workspaces already present
    if (isRecord(raw.workspaces) && isRecord(raw.workspaces.terminal1) && isRecord(raw.workspaces.terminal2)) {
      const sidebarCollapsed = typeof raw.sidebarCollapsed === 'boolean' ? raw.sidebarCollapsed : false
      const settings = migrateSettings(raw.settings, raw.settingsSchemaVersion)
      const workspaces = sanitizeWorkspaces(raw.workspaces, visibleWorkspaceIds(settings))

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
      const workspaces = sanitizeWorkspaces({
        terminal1: {
          windows: raw.windows,
          windowCount: raw.windowCount,
        },
      }, visibleWorkspaceIds(settings))

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
  // Live-switch policy (ctx-00t): the bucket follows the viewport across
  // breakpoint crossings and rotations instead of freezing at mount, so
  // edits are never persisted under a stale viewport key.
  const [viewportBucket, setViewportBucket] = useState<ViewportBucket>(() => getCurrentViewportBucket())
  // Load initial state from localStorage or use defaults. Mount-time only:
  // later bucket switches load their layout in the viewport-change handler.
  const stored = useMemo(() => loadStoredState(viewportBucket), [])

  // Toast notifications
  const { addToast } = useToast()

  const [sessions, setSessions] = useState<TmuxSession[]>([])
  const [groupedSessions, setGroupedSessions] = useState<Record<string, TmuxSession[]>>({})
  const [sessionBank, setSessionBank] = useState<SessionBankEntry[]>([])
  const [managedSessions, setManagedSessions] = useState<ManagedRecoveryStatusEntry[]>([])
  const [terminalUsers, setTerminalUsers] = useState<LaunchUser[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [workspaces, setWorkspaces] = useState<Record<WorkspaceId, TerminalWorkspace>>(
    stored?.workspaces ?? defaultWorkspacesFor(visibleWorkspaceIds(stored?.settings ?? DEFAULT_SETTINGS))
  )
  const viewportBucketRef = useRef(viewportBucket)
  viewportBucketRef.current = viewportBucket
  const workspacesRef = useRef(workspaces)
  workspacesRef.current = workspaces

  // Follow live breakpoint crossings and rotations. Entering a bucket with a
  // stored layout loads it; a bucket never used before carries the current
  // layout over (and the save effect then persists it there). Both setters
  // batch, so the persistence effect observes only the consistent new
  // (workspaces, bucket) pair — nothing is ever written under a stale key.
  useEffect(() => {
    const handleViewportChange = () => {
      const next = getCurrentViewportBucket()
      if (next === viewportBucketRef.current) return
      const storedNext = loadStoredState(next)
      const nextLayout = storedNext?.layoutsByViewport[next]
      setViewportBucket(next)
      if (nextLayout) setWorkspaces(nextLayout.workspaces)
    }
    window.addEventListener('resize', handleViewportChange)
    window.addEventListener('orientationchange', handleViewportChange)
    return () => {
      window.removeEventListener('resize', handleViewportChange)
      window.removeEventListener('orientationchange', handleViewportChange)
    }
  }, [])

  const [sidebarCollapsed, setSidebarCollapsed] = useState(stored?.sidebarCollapsed ?? false)
  const [floatingSession, setFloatingSession] = useState<string | null>(null)
  const [sendToSessionTarget, setSendToSessionTarget] = useState<string | null>(null)
  const [sendToSessionPrefill, setSendToSessionPrefill] = useState('')
  const [sendToSessionRequestId, setSendToSessionRequestId] = useState(0)
  const [settings, setSettings] = useState<UserSettings>(stored?.settings ?? DEFAULT_SETTINGS)
  // Track which window has focus for keyboard navigation (workspaceId-windowId)
  const [focusedWindowKey, setFocusedWindowKey] = useState<string | null>(null)
  const [windowRevealRequest, setWindowRevealRequest] = useState<WindowRevealRequest | null>(null)
  const windowRevealRequestIdRef = useRef(0)
  // Layout presets
  const [layoutPresets, setLayoutPresets] = useState<LayoutPreset[]>(() => loadStoredPresets())
  const staleSessionCandidatesRef = useRef<Set<string>>(new Set())
  const staleSessionProtectionRef = useRef<Map<string, number>>(new Map())
  const refreshMountedRef = useRef(false)
  const refreshGenerationRef = useRef(0)
  const trailingRefreshRef = useRef(false)
  const activeRefreshRef = useRef<{
    timeout: ReturnType<typeof setTimeout>
    promise: Promise<void>
    cancel: (reason: 'timeout' | 'lifecycle') => void
  } | null>(null)
  const protectStaleSessionAliases = useCallback((aliases: string[]) => {
    aliases.forEach(alias => {
      if (alias) staleSessionProtectionRef.current.set(alias, 2)
    })
  }, [])

  // Visible terminal tabs; hidden workspaces stay in the record untouched.
  const workspaceIds = useMemo(() => visibleWorkspaceIds(settings), [settings])

  // Growing the count reveals workspaces: any visible id missing from the
  // record gets a default workspace. Shrinking removes nothing.
  useEffect(() => {
    setWorkspaces(prev => {
      const missing = workspaceIds.filter(workspaceId => !prev[workspaceId])
      if (missing.length === 0) return prev
      return { ...prev, ...defaultWorkspacesFor(missing) }
    })
  }, [workspaceIds])

  // Computed: which sessions are assigned to any window
  const assignedSessions = useMemo(() => {
    const assigned = new Map<string, { workspaceId: WorkspaceId; windowId: string; colorIndex: number; windowIndex: number }>()
    idsInWorkspaces(workspaces).forEach(workspaceId => {
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
      idsInWorkspaces(prev).forEach(workspaceId => {
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
    idsInWorkspaces(workspaces).forEach(workspaceId => {
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

  // Fetch sessions from API. Triggers while a request is active share that request
  // and request one coalesced trailing refresh rather than starting concurrent work.
  const refreshSessions: () => Promise<void> = useCallback(() => {
    if (!refreshMountedRef.current) return Promise.resolve()

    const current = activeRefreshRef.current
    if (current) {
      trailingRefreshRef.current = true
      return current.promise.then(() => activeRefreshRef.current?.promise ?? Promise.resolve())
    }

    type CancellationReason = 'timeout' | 'lifecycle'
    const controller = new AbortController()
    const generation = refreshGenerationRef.current
    let cancellationReason: CancellationReason | null = null
    let resolveCancellation!: (reason: CancellationReason) => void
    const cancellation = new Promise<CancellationReason>(resolve => {
      resolveCancellation = resolve
    })
    const cancel = (reason: CancellationReason) => {
      if (cancellationReason) return
      cancellationReason = reason
      // Settle our independent race before aborting. Some fetch implementations reject
      // synchronously on abort, but timeout still needs timeout (not lifecycle/error) semantics.
      resolveCancellation(reason)
      controller.abort()
    }
    const active = {
      cancel,
      timeout: setTimeout(() => cancel('timeout'), 10000),
      promise: Promise.resolve(),
    }
    activeRefreshRef.current = active
    const hasCurrentCleanupAuthority = () => (
      refreshMountedRef.current &&
      refreshGenerationRef.current === generation &&
      activeRefreshRef.current === active
    )
    const isAuthoritative = () => (
      hasCurrentCleanupAuthority() &&
      cancellationReason === null
    )
    const raceWithCancellation = <T,>(operation: Promise<T>) => Promise.race([
      operation.then(
        value => ({ kind: 'value' as const, value }),
        failure => ({ kind: 'failure' as const, failure }),
      ),
      cancellation.then(reason => ({ kind: 'cancelled' as const, reason })),
    ])
    const reportCancellation = (reason: CancellationReason) => {
      if (reason === 'timeout' && hasCurrentCleanupAuthority()) {
        setError('Failed to fetch sessions (request timed out)')
      }
    }

    active.promise = (async () => {
      try {
        const responseOutcome = await raceWithCancellation(
          fetch('/api/tmux/sessions', { signal: controller.signal }),
        )
        if (responseOutcome.kind === 'cancelled') {
          reportCancellation(responseOutcome.reason)
          return
        }
        if (responseOutcome.kind === 'failure') {
          if (cancellationReason) {
            reportCancellation(cancellationReason)
          } else if (isAuthoritative()) {
            setError('Failed to fetch sessions')
            console.error('Failed to fetch sessions:', responseOutcome.failure)
          }
          return
        }
        if (!isAuthoritative()) return

        const response = responseOutcome.value
        const dataOutcome = await raceWithCancellation(
          response.json() as Promise<Partial<SessionsResponse>>,
        )
        if (dataOutcome.kind === 'cancelled') {
          reportCancellation(dataOutcome.reason)
          return
        }
        if (dataOutcome.kind === 'failure') {
          if (cancellationReason) {
            reportCancellation(cancellationReason)
          } else if (isAuthoritative()) {
            setError('Failed to fetch sessions')
          }
          return
        }
        if (!isAuthoritative()) return

        const data = dataOutcome.value
        // Total failures are not authoritative, on a 200 as much as on a non-ok,
        // so they preserve the last known good state. A configured multi-user
        // response can explicitly mark healthy users' results as authoritative
        // partial data while retaining its user-prefixed error.
        const isPartial = response.ok && data.partial === true
        if (!response.ok || (data.error && !isPartial)) {
          setError(typeof data.error === 'string' ? data.error : 'Failed to fetch sessions')
          return
        }

        const nextSessions = Array.isArray(data.sessions) ? data.sessions : []
        setError(typeof data.error === 'string' ? data.error : null)
        setSessions(nextSessions)
        setGroupedSessions(isRecord(data.grouped) ? data.grouped as Record<string, TmuxSession[]> : {})
        setSessionBank(Array.isArray(data.banked) ? data.banked : [])
        setManagedSessions(Array.isArray(data.managed) ? data.managed : [])
        if (Array.isArray(data.terminalUsers)) {
          setTerminalUsers(normalizeTerminalUsers(data.terminalUsers))
        }

        if (Array.isArray(data.sessions)) {
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
        if (isAuthoritative()) {
          setError('Failed to fetch sessions')
          console.error('Failed to fetch sessions:', e)
        }
      } finally {
        const hasCleanupAuthority = hasCurrentCleanupAuthority()
        clearTimeout(active.timeout)
        if (activeRefreshRef.current === active) {
          activeRefreshRef.current = null
          if (hasCleanupAuthority) setLoading(false)
          if (!refreshMountedRef.current || refreshGenerationRef.current !== generation) {
            trailingRefreshRef.current = false
          } else if (trailingRefreshRef.current) {
            trailingRefreshRef.current = false
            void refreshSessions()
          }
        }
      }
    })()
    return active.promise
  }, [])

  useEffect(() => {
    refreshMountedRef.current = true
    return () => {
      refreshMountedRef.current = false
      refreshGenerationRef.current += 1
      trailingRefreshRef.current = false
      const active = activeRefreshRef.current
      activeRefreshRef.current = null
      if (active) {
        clearTimeout(active.timeout)
        active.cancel('lifecycle')
      }
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
      if (!ws || ws.windowCount === newCount) return prev
      return {
        ...prev,
        [workspaceId]: { ...ws, windowCount: newCount },
      }
    })
  }, [])

  const revealWindow = useCallback((workspaceId: WorkspaceId, windowId: string) => {
    if (!workspacesRef.current[workspaceId]) return
    const windowIndex = Array.from(
      { length: CANONICAL_WINDOW_COUNT },
      (_, index) => `${workspaceId}-window-${index}`,
    ).indexOf(windowId)
    if (windowIndex < 0) return

    setWorkspaces(prev => {
      const workspace = prev[workspaceId]
      if (!workspace) return prev
      const visibleCount = Math.max(workspace.windowCount, windowIndex + 1)
      if (visibleCount === workspace.windowCount) return prev
      return {
        ...prev,
        [workspaceId]: { ...workspace, windowCount: visibleCount },
      }
    })
    windowRevealRequestIdRef.current += 1
    setWindowRevealRequest({
      workspaceId,
      windowId,
      requestId: windowRevealRequestIdRef.current,
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
      const targetWorkspace = prev[workspaceId]
      if (!targetWorkspace?.windows.some(window => window.id === windowId)) return prev

      const qualifiedUsers = qualifiedUsersBySessionName(prev)
      if (unixUser) {
        const users = qualifiedUsers.get(sessionName) ?? new Set<LaunchUser>()
        users.add(unixUser)
        qualifiedUsers.set(sessionName, users)
      }
      const targetIdentity = sessionBindingIdentity(sessionKey, qualifiedUsers)

      return idsInWorkspaces(prev).reduce((next, wsId) => {
        const workspace = prev[wsId]
        next[wsId] = {
          ...workspace,
          windows: workspace.windows.map(window => {
            const isTarget = wsId === workspaceId && window.id === windowId
            const boundSessions = window.boundSessions.filter(bound => (
              sessionBindingIdentity(bound, qualifiedUsers) !== targetIdentity
            ))
            if (isTarget) {
              return {
                ...window,
                boundSessions: [...boundSessions, sessionKey],
                activeSession: sessionKey,
              }
            }
            const activeWasMoved = window.activeSession !== null
              && sessionBindingIdentity(window.activeSession, qualifiedUsers) === targetIdentity
            return {
              ...window,
              boundSessions,
              activeSession: activeWasMoved ? (boundSessions[0] ?? null) : window.activeSession,
            }
          }),
        }
        return next
      }, {} as Record<WorkspaceId, TerminalWorkspace>)
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

  const openSendToSession = useCallback((sessionName: string, prefill = '') => {
    setSendToSessionPrefill(prefill)
    setSendToSessionRequestId(prev => prev + 1)
    setSendToSessionTarget(sessionName)
  }, [])

  const closeSendToSession = useCallback(() => {
    setSendToSessionTarget(null)
    setSendToSessionPrefill('')
  }, [])

  const listSessionPanes = useCallback(async (sessionName: string, unixUser?: LaunchUser): Promise<SendSessionPane[] | null> => {
    const expectedUnixUser = unixUser ?? ''
    try {
      const query = unixUser ? `?unixUser=${encodeURIComponent(unixUser)}` : ''
      const response = await fetch(`/api/tmux/sessions/${encodeURIComponent(sessionName)}/panes${query}`, {
        signal: AbortSignal.timeout(10000),
      })
      if (!response.ok) {
        const errorText = await response.text()
        addToast(apiErrorMessage(errorText, 'Failed to resolve session panes'), 'error')
        return null
      }
      const result = await response.json().catch(() => null) as { success?: unknown; session?: unknown; unixUser?: unknown; panes?: unknown } | null
      if (!result || result.success !== true || result.session !== sessionName || result.unixUser !== expectedUnixUser || !Array.isArray(result.panes)) {
        addToast('Unexpected pane discovery response', 'error')
        return null
      }
      const panes = result.panes.filter((pane): pane is SendSessionPane => {
        if (!pane || typeof pane !== 'object') return false
        const candidate = pane as Partial<SendSessionPane>
        return typeof candidate.sessionId === 'string' && /^\$\d+$/.test(candidate.sessionId) &&
          typeof candidate.pane === 'string' && /^%\d+$/.test(candidate.pane) &&
          typeof candidate.panePid === 'string' && /^[1-9]\d*$/.test(candidate.panePid) &&
          typeof candidate.serverPid === 'string' && /^[1-9]\d*$/.test(candidate.serverPid) &&
          typeof candidate.active === 'boolean' &&
          (candidate.windowId === undefined || (typeof candidate.windowId === 'string' && /^@\d+$/.test(candidate.windowId))) &&
          (candidate.windowName === undefined || typeof candidate.windowName === 'string') &&
          (candidate.currentPath === undefined || typeof candidate.currentPath === 'string') &&
          (candidate.currentCommand === undefined || typeof candidate.currentCommand === 'string')
      })
      if (panes.length !== result.panes.length || panes.length === 0 ||
          new Set(panes.map(pane => pane.pane)).size !== panes.length ||
          new Set(panes.map(pane => pane.sessionId)).size !== 1 ||
          new Set(panes.map(pane => pane.serverPid)).size !== 1) {
        addToast('Unexpected pane discovery response', 'error')
        return null
      }
      return panes
    } catch (e) {
      console.error('Failed to resolve session panes:', e)
      addToast('Failed to resolve session panes', 'error')
      return null
    }
  }, [addToast])

  const sendToSession = useCallback(async (sessionName: string, payload: SendToSessionPayload, unixUser?: LaunchUser): Promise<SendToSessionOutcome> => {
    const expectedUnixUser = unixUser ?? ''
    try {
      const form = new FormData()
      form.set('text', payload.text)
      form.set('submit', payload.submit ? 'true' : 'false')
      if (payload.pane) {
        form.set('pane', payload.pane)
        if (payload.sessionId) form.set('sessionId', payload.sessionId)
        if (payload.panePid) form.set('panePid', payload.panePid)
        if (payload.serverPid) form.set('serverPid', payload.serverPid)
      }
      payload.files.forEach(file => form.append('files', file, file.name))
      const query = unixUser ? `?unixUser=${encodeURIComponent(unixUser)}` : ''
      const response = await fetch(`/api/tmux/sessions/${encodeURIComponent(sessionName)}/send${query}`, {
        method: 'POST',
        body: form,
        signal: AbortSignal.timeout(30000),
      })
      if (!response.ok) {
        const errorText = await response.text()
        const definitiveMessage = definitiveSendErrorMessage(response.status, errorText)
        console.error('Failed to send to session:', errorText)
        if (definitiveMessage) {
          addToast(definitiveMessage, 'error')
          return 'failed'
        } else {
          addToast(`Delivery outcome is unknown for '${sessionName}'; inspect the exact pane before retrying`, 'error')
          return 'unknown'
        }
      }
      const result = await response.json().catch(() => null) as SendToSessionResult | null
      const commonResultValid = !!result &&
        result.session === sessionName &&
        typeof result.sessionId === 'string' && /^\$\d+$/.test(result.sessionId) &&
        typeof result.pane === 'string' && /^%\d+$/.test(result.pane) &&
        typeof result.panePid === 'string' && /^[1-9]\d*$/.test(result.panePid) &&
        typeof result.serverPid === 'string' && /^[1-9]\d*$/.test(result.serverPid) &&
        result.unixUser === expectedUnixUser &&
        (!payload.pane || result.pane === payload.pane) &&
        (!payload.sessionId || result.sessionId === payload.sessionId) &&
        (!payload.panePid || result.panePid === payload.panePid) &&
        (!payload.serverPid || result.serverPid === payload.serverPid) &&
        typeof result.submissionRequested === 'boolean' &&
        result.submissionRequested === payload.submit &&
        typeof result.submitKeyDispatched === 'boolean' &&
        typeof result.bufferCleaned === 'boolean' &&
        typeof result.targetVerified === 'boolean' &&
        typeof result.warning === 'string'
      if (commonResultValid && result &&
          result.success === false &&
          result.transport === 'unknown' &&
          result.retryable === false &&
          result.deliveryConfirmed === false &&
          result.submitKeyDispatched === false &&
          result.targetVerified === false &&
          result.warning.trim() !== '') {
        addToast(`Delivery outcome is unknown for '${sessionName}' (${result.pane}); ${result.warning.trim()}`, 'error')
        return 'unknown'
      }
      if (commonResultValid && result && result.success === true && result.transport === 'pasted' &&
          payload.submit && result.submitKeyDispatched === false) {
        const warning = result.warning.trim()
        addToast(`Pasted to '${sessionName}' (${result.pane}), but the submit key was not dispatched${warning ? `; ${warning}` : ''}`, 'error')
        return 'unknown'
      }
      if (!commonResultValid || !result || result.success !== true || result.transport !== 'pasted' ||
          result.submitKeyDispatched !== payload.submit || result.bufferCleaned !== true ||
          (result.targetVerified !== true && result.warning.trim() === '')) {
        addToast('Unexpected send response; inspect the target pane before retrying', 'error')
        return 'unknown'
      }
      const paneLabel = ` (${result.pane})`
      const submitLabel = result.submitKeyDispatched ? '; submit key dispatched (application acceptance unconfirmed)' : ''
      const warning = result.warning?.trim() ?? ''
      addToast(`Pasted to '${sessionName}'${paneLabel}${submitLabel}${warning ? `; ${warning}` : ''}`, warning ? 'info' : 'success')
      return 'sent'
    } catch (e) {
      console.error('Send-to-session delivery outcome is unknown:', e)
      addToast(`Delivery outcome is unknown for '${sessionName}'; inspect the exact pane before retrying`, 'error')
      return 'unknown'
    }
  }, [addToast])

  const handleSessionClick = useCallback((sessionName: string) => {
    openFloatingModal(sessionName)
  }, [openFloatingModal])

  const focusSessionAssignment = useCallback((sessionName: string) => {
    const assignment = assignedSessions.get(sessionName)
    if (!assignment) return
    revealWindow(assignment.workspaceId, assignment.windowId)
    setActiveSession(assignment.workspaceId, assignment.windowId, sessionName)
    setFocusedWindowKey(`${assignment.workspaceId}-${assignment.windowId}`)
  }, [assignedSessions, revealWindow, setActiveSession])

  const updateSettings = useCallback((newSettings: Partial<UserSettings>) => {
    setSettings(prev => {
      const updated = { ...prev, ...newSettings }
      if (newSettings.terminalTabCount !== undefined) {
        updated.terminalTabCount = normalizeTerminalTabCount(newSettings.terminalTabCount)
      }
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
          idsInWorkspaces(prev).forEach(workspaceId => {
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
    const qualifiedUsers = qualifiedUsersBySessionName(workspaces)
    const newKey = getSessionKey(newName, unixUser)
    const sourceAliases = safeSessionAliases(oldName, unixUser, qualifiedUsers)
    const targetAliases = safeSessionAliases(newName, unixUser, qualifiedUsers)
    const sourceAliasSet = new Set(sourceAliases)

    try {
      const query = unixUser ? `?unixUser=${encodeURIComponent(unixUser)}` : ''
      const response = await fetch(`/api/tmux/sessions/${encodeURIComponent(oldName)}${query}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ newName }),
        signal: AbortSignal.timeout(10000),
      })
      if (response.ok) {
        sourceAliases.concat(targetAliases).forEach(alias => staleSessionCandidatesRef.current.delete(alias))
        protectStaleSessionAliases(targetAliases)
        // Update window bindings to use the new name/key
        setWorkspaces(prev => {
          const next: Record<WorkspaceId, TerminalWorkspace> = { ...prev }
          idsInWorkspaces(prev).forEach(workspaceId => {
            const ws = prev[workspaceId]
            next[workspaceId] = {
              ...ws,
              windows: ws.windows.map(w => ({
                ...w,
                boundSessions: w.boundSessions.map(s => sourceAliasSet.has(s) ? newKey : s),
                activeSession: w.activeSession && sourceAliasSet.has(w.activeSession) ? newKey : w.activeSession,
              })),
            }
          })
          return deduplicateWorkspaceBindings(next, newKey)
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
  }, [refreshSessions, addToast, protectStaleSessionAliases, workspaces])


  const makeSessionPersistent = useCallback(async (sessionName: string, payload: PersistentAgentPayload, unixUser?: LaunchUser): Promise<boolean> => {
    try {
      const query = unixUser ? `?unixUser=${encodeURIComponent(unixUser)}` : ''
      const body: Record<string, unknown> = {}
      const agentKind = payload.agentKind?.trim()
      if (agentKind) body.agentKind = agentKind
      const agentSessionId = payload.agentSessionId?.trim()
      if (agentSessionId) body.agentSessionId = agentSessionId
      const identity = payload.identity?.trim()
      if (identity) body.identity = identity
      const newName = payload.newName?.trim()
      if (newName) body.newName = newName
      if (payload.recoveryDescriptor) body.recoveryDescriptor = payload.recoveryDescriptor

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
      const responseText = await response.text()
      if (!response.ok) {
        console.error('Failed to make session mortal:', responseText)
        addToast(apiErrorMessage(responseText, 'Failed to make session mortal'), 'error')
        return false
      }
      let body: { success?: boolean; unitWarning?: string } = {}
      try {
        body = JSON.parse(responseText) as typeof body
      } catch {
        // A successful unlock must be explicit; an empty or malformed 2xx body
        // cannot prove that supervision stopped.
      }
      if (body.success !== true || body.unitWarning) {
        const message = body.unitWarning || 'Server did not confirm that supervision stopped'
        console.error('Failed to make session mortal:', responseText)
        addToast(message, 'error')
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
      workspaces: sanitizeWorkspaces(workspaces, []),
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

    // Merge, not replace: preset ids apply, current ids absent from the preset
    // are preserved, extra preset ids are stored for later growth. Loading a
    // preset never changes the tab count.
    setWorkspaces(prev => deduplicateWorkspaceBindings({
      ...prev,
      ...sanitizeWorkspaces(cloneWorkspaces(preset.workspaces), []),
    }))
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
    managedSessions,
    terminalUsers,
    loading,
    error,
    workspaces,
    workspaceIds,
    sidebarCollapsed,
    floatingSession,
    sendToSessionTarget,
    sendToSessionPrefill,
    sendToSessionRequestId,
    assignedSessions,
    settings,
    focusedWindowKey,
    windowRevealRequest,
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
    listSessionPanes,
    sendToSession,
    handleSessionClick,
    focusSessionAssignment,
    refreshSessions,
    createSession,
    deleteSession,
    renameSession,
    makeSessionPersistent,
    makeSessionMortal,
    updateSettings,
    setFocusedWindowKey,
    revealWindow,
    saveCurrentLayout,
    loadPreset,
    deletePreset,
    renamePreset,
  }), [
    sessions,
    groupedSessions,
    sessionBank,
    managedSessions,
    terminalUsers,
    loading,
    error,
    workspaces,
    workspaceIds,
    sidebarCollapsed,
    floatingSession,
    sendToSessionTarget,
    sendToSessionPrefill,
    sendToSessionRequestId,
    assignedSessions,
    settings,
    focusedWindowKey,
    windowRevealRequest,
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
    listSessionPanes,
    sendToSession,
    handleSessionClick,
    focusSessionAssignment,
    refreshSessions,
    createSession,
    deleteSession,
    renameSession,
    makeSessionPersistent,
    makeSessionMortal,
    updateSettings,
    setFocusedWindowKey,
    revealWindow,
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

/** Like useSession, but returns null outside a SessionProvider (e.g. isolated
    component tests) so consumers can fall back to defaults. */
export function useSessionOptional(): DashboardContextType | null {
  return useContext(SessionContext)
}
