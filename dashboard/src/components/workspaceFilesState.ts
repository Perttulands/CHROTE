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

export type WorkspaceSidecar = 'sessions' | 'files' | null

export interface WorkspaceDockState {
  activeSidecar: WorkspaceSidecar
  sidecarPinned: boolean
  sessionsWidth: number
  filesWidth: number
}

const DOCK_STORAGE_KEY = 'chrote.workspaceDock.v2'
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

export const DEFAULT_WORKSPACE_DOCK_STATE: WorkspaceDockState = {
  activeSidecar: null,
  sidecarPinned: false,
  sessionsWidth: 260,
  filesWidth: 320,
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

function readLegacySidebarCollapsed(workspaceId: WorkspaceId): boolean | null {
  if (typeof window === 'undefined' || workspaceId !== 'terminal1') return null
  try {
    const parsed: unknown = JSON.parse(window.localStorage.getItem('chrote-dashboard-state') || '{}')
    return isRecord(parsed) && typeof parsed.sidebarCollapsed === 'boolean' ? parsed.sidebarCollapsed : null
  } catch {
    return null
  }
}

export function readWorkspaceDockState(workspaceId: WorkspaceId): WorkspaceDockState {
  const raw = readStorageMap(DOCK_STORAGE_KEY, 2)[workspaceId]
  if (isRecord(raw)) {
    const activeSidecar = raw.activeSidecar === 'sessions' || raw.activeSidecar === 'files'
      ? raw.activeSidecar
      : null
    return {
      activeSidecar,
      // The pin preference survives a closed sidecar so reopening restores
      // the same presentation (pinned beside vs overlay above the terminal).
      sidecarPinned: raw.sidecarPinned === true,
      sessionsWidth: finiteNumber(raw.sessionsWidth, DEFAULT_WORKSPACE_DOCK_STATE.sessionsWidth, 220, 480),
      filesWidth: finiteNumber(raw.filesWidth, DEFAULT_WORKSPACE_DOCK_STATE.filesWidth, 240, 560),
    }
  }

  const legacy = readStorageMap(LEGACY_DOCK_STORAGE_KEY)[workspaceId]
  if (isRecord(legacy)) {
    const activeSidecar: WorkspaceSidecar = legacy.sessionsCollapsed !== true
      ? 'sessions'
      : legacy.filesCollapsed === false
        ? 'files'
        : null
    return {
      activeSidecar,
      sidecarPinned: activeSidecar !== null,
      sessionsWidth: finiteNumber(legacy.sessionsWidth, DEFAULT_WORKSPACE_DOCK_STATE.sessionsWidth, 220, 480),
      filesWidth: finiteNumber(legacy.filesWidth, DEFAULT_WORKSPACE_DOCK_STATE.filesWidth, 240, 560),
    }
  }

  const legacySidebarCollapsed = readLegacySidebarCollapsed(workspaceId)
  if (legacySidebarCollapsed !== null) {
    return {
      ...DEFAULT_WORKSPACE_DOCK_STATE,
      activeSidecar: legacySidebarCollapsed ? null : 'sessions',
      sidecarPinned: !legacySidebarCollapsed,
    }
  }

  return { ...DEFAULT_WORKSPACE_DOCK_STATE }
}

export function writeWorkspaceDockState(workspaceId: WorkspaceId, state: WorkspaceDockState): void {
  const activeSidecar = state.activeSidecar === 'sessions' || state.activeSidecar === 'files'
    ? state.activeSidecar
    : null
  writeStorageMap(DOCK_STORAGE_KEY, workspaceId, {
    activeSidecar,
    sidecarPinned: state.sidecarPinned === true,
    sessionsWidth: finiteNumber(state.sessionsWidth, DEFAULT_WORKSPACE_DOCK_STATE.sessionsWidth, 220, 480),
    filesWidth: finiteNumber(state.filesWidth, DEFAULT_WORKSPACE_DOCK_STATE.filesWidth, 240, 560),
  }, 2)
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
