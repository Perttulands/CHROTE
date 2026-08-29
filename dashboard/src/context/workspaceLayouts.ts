import type { LaunchUser, LayoutPreset, TerminalWindow, TerminalWorkspace, UserSettings, WorkspaceId } from '../types'
import {
  DEFAULT_SETTINGS,
  DEFAULT_TMUX_APPEARANCE,
  getSessionKey,
  getSessionNameFromKey,
  getSessionUserFromKey,
  normalizeTerminalTabCount,
  normalizeTerminalUsers,
  sortTerminalWorkspaceIds,
  terminalWorkspaceIds,
} from '../types'

const STORAGE_KEY = 'chrote-dashboard-state'
const PRESETS_STORAGE_KEY = 'chrote-dashboard-presets'
const SETTINGS_SCHEMA_VERSION = 2
const STORED_WORKSPACE_ID_PATTERN = /^terminal[1-9]\d*$/
export const CANONICAL_WINDOW_COUNT = 4
const VIEWPORT_BUCKETS = ['mobile', 'tablet', 'desktop'] as const
export type ViewportBucket = typeof VIEWPORT_BUCKETS[number]

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

export interface LoadedStoredState extends StoredStateV2 {
  layoutsByViewport: Partial<Record<ViewportBucket, StoredLayout>>
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function isStoredWorkspaceId(value: string): value is WorkspaceId {
  return STORED_WORKSPACE_ID_PATTERN.test(value)
}

export function visibleWorkspaceIds(settings: UserSettings): WorkspaceId[] {
  return terminalWorkspaceIds(normalizeTerminalTabCount(settings.terminalTabCount))
}

export function idsInWorkspaces(workspaces: Record<WorkspaceId, TerminalWorkspace>): WorkspaceId[] {
  return sortTerminalWorkspaceIds(Object.keys(workspaces) as WorkspaceId[])
}

export function getCurrentViewportBucket(): ViewportBucket {
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
  return {
    ...DEFAULT_SETTINGS,
    ...rawSettings,
    terminalTabCount: normalizeTerminalTabCount(rawSettings.terminalTabCount),
    terminalLaunchUsers: mergeTerminalLaunchUsers(rawSettings.terminalLaunchUsers),
    terminalSessionPrefixes: mergeTerminalSessionPrefixes(
      rawSettings.terminalSessionPrefixes,
      rawSettings.defaultSessionPrefix,
      rawSettings.terminalLaunchUsers,
    ),
    terminalLabels: mergeStringMap(rawSettings.terminalLabels),
    terminalUserColors: mergeStringMap(rawSettings.terminalUserColors),
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

export function defaultWorkspacesFor(ids: readonly WorkspaceId[]): Record<WorkspaceId, TerminalWorkspace> {
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

export function loadStoredState(viewportBucket: ViewportBucket = getCurrentViewportBucket()): LoadedStoredState | null {
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    if (stored) return migrateStoredState(JSON.parse(stored), viewportBucket)
  } catch (e) {
    console.warn('Failed to load stored state:', e)
  }
  return null
}

export function saveState(state: StoredStateV2, viewportBucket: ViewportBucket): void {
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

export function loadStoredPresets(): LayoutPreset[] {
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

export function savePresets(presets: LayoutPreset[]): void {
  try {
    localStorage.setItem(PRESETS_STORAGE_KEY, JSON.stringify(presets))
  } catch (e) {
    console.warn('Failed to save presets:', e)
  }
}

export function generatePresetId(): string {
  return `preset-${Date.now()}-${Math.random().toString(36).substring(2, 9)}`
}

export function cloneWorkspaces(workspaces: Record<WorkspaceId, TerminalWorkspace>): Record<WorkspaceId, TerminalWorkspace> {
  return JSON.parse(JSON.stringify(workspaces))
}

export function clampWindowCount(count: number): number {
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
  return { windows: createDefaultWindows(workspaceId), windowCount: clampWindowCount(count) }
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

export function qualifiedUsersBySessionName(workspaces: Record<WorkspaceId, TerminalWorkspace>): Map<string, Set<LaunchUser>> {
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

export function sessionBindingIdentity(sessionKey: string, qualifiedUsers: Map<string, Set<LaunchUser>>): string {
  const sessionName = getSessionNameFromKey(sessionKey)
  const unixUser = getSessionUserFromKey(sessionKey)
  if (unixUser) return `qualified:${getSessionKey(sessionName, unixUser)}`
  const users = qualifiedUsers.get(sessionName)
  if (users?.size === 1) return `qualified:${getSessionKey(sessionName, users.values().next().value ?? '')}`
  return `bare:${sessionKey}`
}

export function safeSessionAliases(
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

export function deduplicateWorkspaceBindings(
  workspaces: Record<WorkspaceId, TerminalWorkspace>,
  targetSessionKey?: string,
): Record<WorkspaceId, TerminalWorkspace> {
  const qualifiedUsers = qualifiedUsersBySessionName(workspaces)
  const targetIdentity = targetSessionKey ? sessionBindingIdentity(targetSessionKey, qualifiedUsers) : null
  const seen = new Set<string>()
  return idsInWorkspaces(workspaces).reduce((next, workspaceId) => {
    const workspace = workspaces[workspaceId]
    next[workspaceId] = {
      ...workspace,
      windows: workspace.windows.map(window => {
        const boundSessions = window.boundSessions.filter(sessionKey => {
          const identity = sessionBindingIdentity(sessionKey, qualifiedUsers)
          if (targetIdentity !== null && identity !== targetIdentity) return true
          if (seen.has(identity)) return false
          seen.add(identity)
          return true
        })
        const activeWasRemoved = window.activeSession !== null &&
          window.activeSession !== 'INIT-PENDING' && !boundSessions.includes(window.activeSession)
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

export function sanitizeWorkspaces(rawWorkspaces: unknown, ids: readonly WorkspaceId[]): Record<WorkspaceId, TerminalWorkspace> {
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
        if (isViewportBucket(key) && isRecord(value)) {
          layoutsByViewport[key] = { workspaces: sanitizeWorkspaces(value.workspaces, ids) }
        }
      })
      return {
        workspaces: layoutsByViewport[viewportBucket]?.workspaces ?? defaultWorkspacesFor(ids),
        layoutsByViewport,
        sidebarCollapsed: typeof raw.sidebarCollapsed === 'boolean' ? raw.sidebarCollapsed : false,
        settings,
      }
    }

    if (isRecord(raw.workspaces) && isRecord(raw.workspaces.terminal1) && isRecord(raw.workspaces.terminal2)) {
      const sidebarCollapsed = typeof raw.sidebarCollapsed === 'boolean' ? raw.sidebarCollapsed : false
      const settings = migrateSettings(raw.settings, raw.settingsSchemaVersion)
      const workspaces = sanitizeWorkspaces(raw.workspaces, visibleWorkspaceIds(settings))
      return {
        workspaces,
        layoutsByViewport: { [viewportBucket]: { workspaces } },
        sidebarCollapsed,
        settings,
      }
    }

    if (Array.isArray(raw.windows) && typeof raw.windowCount === 'number') {
      const sidebarCollapsed = typeof raw.sidebarCollapsed === 'boolean' ? raw.sidebarCollapsed : false
      const settings = migrateSettings(raw.settings, raw.settingsSchemaVersion)
      const workspaces = sanitizeWorkspaces({
        terminal1: { windows: raw.windows, windowCount: raw.windowCount },
      }, visibleWorkspaceIds(settings))
      return {
        workspaces,
        layoutsByViewport: { [viewportBucket]: { workspaces } },
        sidebarCollapsed,
        settings,
      }
    }
  }
  return defaultStoredState()
}
