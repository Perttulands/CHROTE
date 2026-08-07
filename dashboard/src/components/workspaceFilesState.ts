import type { WorkspaceId } from '../types'

export type MarkdownMode = 'preview' | 'source' | 'split'

export interface FileViewState {
  scrollTop: number
  markdownMode: MarkdownMode
  fontSize: number
  markdownSplitPercent: number
  imageZoom: number
  imageFit: boolean
}

export interface WorkspaceFilePeekState {
  path: string
  name: string
  size: number
  type: string
  x: number
  y: number
  width: number
  height: number
}

export interface WorkspaceFilesState {
  currentPath: string
  expandedPaths: string[]
  selectedPath: string | null
  treeScrollTop: number
  peek: WorkspaceFilePeekState | null
  fileViewStates: Record<string, FileViewState>
}

export interface SessionsDockState {
  open: boolean
  pinned: boolean
  width: number
  searchTerm: string
  collapsedGroups: string[]
}

export interface WorkspaceFilesDockState {
  open: boolean
  pinned: boolean
  width: number
}

const SESSIONS_DOCK_STORAGE_KEY = 'chrote.sessionsDock.v1'
const WORKSPACE_FILES_DOCK_STORAGE_KEY = 'chrote.workspaceFilesDock.v1'
const LEGACY_DOCK_V2_STORAGE_KEY = 'chrote.workspaceDock.v2'
const LEGACY_DOCK_STORAGE_KEY = 'chrote.workspaceDock.v1'
const FILES_STORAGE_KEY = 'chrote.workspaceFiles.v1'

export const DEFAULT_FILE_VIEW_STATE: FileViewState = {
  scrollTop: 0,
  markdownMode: 'preview',
  fontSize: 15,
  markdownSplitPercent: 50,
  imageZoom: 1,
  imageFit: true,
}

export const DEFAULT_SESSIONS_DOCK_STATE: SessionsDockState = {
  open: false,
  pinned: false,
  width: 260,
  searchTerm: '',
  collapsedGroups: [],
}

export const DEFAULT_WORKSPACE_FILES_DOCK_STATE: WorkspaceFilesDockState = {
  open: false,
  pinned: false,
  width: 320,
}

export const DEFAULT_WORKSPACE_FILES_STATE: WorkspaceFilesState = {
  currentPath: '/',
  expandedPaths: ['/'],
  selectedPath: null,
  treeScrollTop: 0,
  peek: null,
  fileViewStates: {},
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}

function finiteNumber(value: unknown, fallback: number, min: number, max: number): number {
  return typeof value === 'number' && Number.isFinite(value)
    ? Math.min(max, Math.max(min, value))
    : fallback
}

function readStorageMap(key: string, expectedVersion = 1): Record<string, unknown> {
  if (typeof window === 'undefined') return {}
  try {
    const parsed: unknown = JSON.parse(window.localStorage.getItem(key) || '{}')
    if (!isRecord(parsed)) return {}
    if (parsed.version === expectedVersion && isRecord(parsed.workspaces)) return parsed.workspaces
    if (parsed.version !== undefined) return {}
    return parsed // Migrate the original unwrapped v1 draft on the next write.
  } catch {
    return {}
  }
}

function writeStorageMap(key: string, workspaceId: WorkspaceId, value: unknown, version = 1): void {
  if (typeof window === 'undefined') return
  try {
    const stored = readStorageMap(key, version)
    window.localStorage.setItem(key, JSON.stringify({ version, workspaces: { ...stored, [workspaceId]: value } }))
  } catch {
    // Private mode/quota failures must not make the terminal workspace unusable.
  }
}

function readStorageRecord(key: string, expectedVersion = 1): Record<string, unknown> | null {
  if (typeof window === 'undefined') return null
  try {
    const parsed: unknown = JSON.parse(window.localStorage.getItem(key) || 'null')
    if (!isRecord(parsed) || parsed.version !== expectedVersion || !isRecord(parsed.state)) return null
    return parsed.state
  } catch {
    return null
  }
}

function writeStorageRecord(key: string, value: unknown, version = 1): void {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(key, JSON.stringify({ version, state: value }))
  } catch {
    // Private mode/quota failures must not make the terminal workspace unusable.
  }
}

function sanitizeFileViewState(value: unknown): FileViewState {
  if (!isRecord(value)) return { ...DEFAULT_FILE_VIEW_STATE }
  const markdownMode = value.markdownMode === 'source' || value.markdownMode === 'split'
    ? value.markdownMode
    : 'preview'
  return {
    scrollTop: finiteNumber(value.scrollTop, 0, 0, 10_000_000),
    markdownMode,
    fontSize: finiteNumber(value.fontSize, DEFAULT_FILE_VIEW_STATE.fontSize, 11, 28),
    markdownSplitPercent: finiteNumber(value.markdownSplitPercent, DEFAULT_FILE_VIEW_STATE.markdownSplitPercent, 20, 80),
    imageZoom: finiteNumber(value.imageZoom, 1, 0.1, 8),
    imageFit: value.imageFit !== false,
  }
}

function sanitizePeek(value: unknown): WorkspaceFilePeekState | null {
  if (!isRecord(value) || typeof value.path !== 'string' || !value.path.startsWith('/')) return null
  const name = typeof value.name === 'string' && value.name ? value.name : value.path.split('/').pop() || value.path
  return {
    path: value.path,
    name,
    size: finiteNumber(value.size, 0, 0, Number.MAX_SAFE_INTEGER),
    type: typeof value.type === 'string' ? value.type : '',
    x: finiteNumber(value.x, 360, 0, 100_000),
    y: finiteNumber(value.y, 96, 0, 100_000),
    width: finiteNumber(value.width, 720, 320, 1600),
    height: finiteNumber(value.height, 620, 260, 1200),
  }
}

function readLegacySidebarCollapsed(): boolean | null {
  if (typeof window === 'undefined') return null
  try {
    const parsed: unknown = JSON.parse(window.localStorage.getItem('chrote-dashboard-state') || '{}')
    return isRecord(parsed) && typeof parsed.sidebarCollapsed === 'boolean' ? parsed.sidebarCollapsed : null
  } catch {
    return null
  }
}

export function readSessionsDockState(): SessionsDockState {
  const raw = readStorageRecord(SESSIONS_DOCK_STORAGE_KEY)
  if (raw) {
    return {
      open: raw.open === true,
      pinned: raw.pinned === true,
      width: finiteNumber(raw.width, DEFAULT_SESSIONS_DOCK_STATE.width, 220, 480),
      searchTerm: typeof raw.searchTerm === 'string' ? raw.searchTerm : '',
      collapsedGroups: Array.isArray(raw.collapsedGroups)
        ? Array.from(new Set(raw.collapsedGroups.filter((group): group is string => typeof group === 'string')))
        : [],
    }
  }

  const legacyWorkspaceDockState = readStorageMap(LEGACY_DOCK_V2_STORAGE_KEY, 2)
  if (Object.keys(legacyWorkspaceDockState).length > 0) {
    // The immediate predecessor had one Sessions preference per workspace, so
    // it has no unambiguous global owner. Its presence still outranks the older
    // global sidebar key: start from the global default and let App persist the
    // new format rather than resurrecting stale presentation.
    return { ...DEFAULT_SESSIONS_DOCK_STATE }
  }

  const legacySidebarCollapsed = readLegacySidebarCollapsed()
  if (legacySidebarCollapsed !== null) {
    return {
      ...DEFAULT_SESSIONS_DOCK_STATE,
      open: !legacySidebarCollapsed,
      pinned: !legacySidebarCollapsed,
    }
  }

  return { ...DEFAULT_SESSIONS_DOCK_STATE }
}

export function writeSessionsDockState(state: SessionsDockState): void {
  writeStorageRecord(SESSIONS_DOCK_STORAGE_KEY, {
    open: state.open === true,
    pinned: state.pinned === true,
    width: finiteNumber(state.width, DEFAULT_SESSIONS_DOCK_STATE.width, 220, 480),
    searchTerm: typeof state.searchTerm === 'string' ? state.searchTerm : '',
    collapsedGroups: Array.from(new Set(state.collapsedGroups.filter(group => typeof group === 'string'))),
  })
}

export function readWorkspaceFilesDockState(workspaceId: WorkspaceId): WorkspaceFilesDockState {
  const raw = readStorageMap(WORKSPACE_FILES_DOCK_STORAGE_KEY)[workspaceId]
  if (isRecord(raw)) {
    return {
      open: raw.open === true,
      pinned: raw.pinned === true,
      width: finiteNumber(raw.width, DEFAULT_WORKSPACE_FILES_DOCK_STATE.width, 240, 560),
    }
  }

  const legacyV2 = readStorageMap(LEGACY_DOCK_V2_STORAGE_KEY, 2)[workspaceId]
  if (isRecord(legacyV2)) {
    const openSidecars = Array.isArray(legacyV2.openSidecars)
      ? legacyV2.openSidecars
      : legacyV2.activeSidecar ? [legacyV2.activeSidecar] : []
    return {
      open: openSidecars.includes('files'),
      pinned: legacyV2.sidecarPinned === true,
      width: finiteNumber(legacyV2.filesWidth, DEFAULT_WORKSPACE_FILES_DOCK_STATE.width, 240, 560),
    }
  }

  const legacy = readStorageMap(LEGACY_DOCK_STORAGE_KEY)[workspaceId]
  if (isRecord(legacy)) {
    return {
      open: legacy.filesCollapsed === false,
      pinned: legacy.filesCollapsed === false,
      width: finiteNumber(legacy.filesWidth, DEFAULT_WORKSPACE_FILES_DOCK_STATE.width, 240, 560),
    }
  }

  return { ...DEFAULT_WORKSPACE_FILES_DOCK_STATE }
}

export function writeWorkspaceFilesDockState(workspaceId: WorkspaceId, state: WorkspaceFilesDockState): void {
  writeStorageMap(WORKSPACE_FILES_DOCK_STORAGE_KEY, workspaceId, {
    open: state.open === true,
    pinned: state.pinned === true,
    width: finiteNumber(state.width, DEFAULT_WORKSPACE_FILES_DOCK_STATE.width, 240, 560),
  })
}

export function readWorkspaceFilesState(workspaceId: WorkspaceId): WorkspaceFilesState {
  const raw = readStorageMap(FILES_STORAGE_KEY)[workspaceId]
  if (!isRecord(raw)) return {
    ...DEFAULT_WORKSPACE_FILES_STATE,
    expandedPaths: [...DEFAULT_WORKSPACE_FILES_STATE.expandedPaths],
    fileViewStates: {},
  }

  const expandedPaths = Array.isArray(raw.expandedPaths)
    ? Array.from(new Set(raw.expandedPaths.filter((path): path is string => typeof path === 'string' && path.startsWith('/'))))
    : ['/']
  if (!expandedPaths.includes('/')) expandedPaths.unshift('/')

  const fileViewStates = isRecord(raw.fileViewStates)
    ? Object.entries(raw.fileViewStates).reduce<Record<string, FileViewState>>((next, [path, value]) => {
        if (path.startsWith('/')) next[path] = sanitizeFileViewState(value)
        return next
      }, {})
    : {}

  return {
    currentPath: typeof raw.currentPath === 'string' && raw.currentPath.startsWith('/') ? raw.currentPath : '/',
    expandedPaths,
    selectedPath: typeof raw.selectedPath === 'string' && raw.selectedPath.startsWith('/') ? raw.selectedPath : null,
    treeScrollTop: finiteNumber(raw.treeScrollTop, 0, 0, 10_000_000),
    peek: sanitizePeek(raw.peek),
    fileViewStates,
  }
}

export function writeWorkspaceFilesState(workspaceId: WorkspaceId, state: WorkspaceFilesState): void {
  writeStorageMap(FILES_STORAGE_KEY, workspaceId, {
    ...state,
    expandedPaths: Array.from(new Set(state.expandedPaths.filter(path => path.startsWith('/')))),
    treeScrollTop: finiteNumber(state.treeScrollTop, 0, 0, 10_000_000),
    peek: sanitizePeek(state.peek),
    fileViewStates: Object.entries(state.fileViewStates).reduce<Record<string, FileViewState>>((next, [path, value]) => {
      if (path.startsWith('/')) next[path] = sanitizeFileViewState(value)
      return next
    }, {}),
  })
}
