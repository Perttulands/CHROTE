import type { PreviewKind } from '../FileViewer'
import { DEFAULT_FILE_VIEW_STATE, type FileViewState } from '../workspaceFilesState'

export type FilesViewMode = 'list' | 'grid'
export type FilesContentMode = 'folder' | 'file'

export interface StoredOpenFile {
  path: string
  name: string
  size: number
  type: string
  kind: PreviewKind
}

export interface FilesWorkbenchState {
  version: 1
  currentPath: string
  history: string[]
  historyIndex: number
  viewMode: FilesViewMode
  contentMode: FilesContentMode
  expandedPaths: string[]
  treeScrollTop: number
  explorerWidth: number
  openFiles: StoredOpenFile[]
  activeFilePath: string | null
  fileViewStates: Record<string, FileViewState>
}

const STORAGE_KEY = 'chrote.files.workbench.v1'
const PREVIEW_KINDS = new Set<PreviewKind>(['text', 'image', 'audio', 'video', 'pdf', 'download'])

export const DEFAULT_FILES_WORKBENCH_STATE: FilesWorkbenchState = {
  version: 1,
  currentPath: '/',
  history: ['/'],
  historyIndex: 0,
  viewMode: 'list',
  contentMode: 'folder',
  expandedPaths: ['/'],
  treeScrollTop: 0,
  explorerWidth: 260,
  openFiles: [],
  activeFilePath: null,
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

function sanitizeOpenFiles(value: unknown): StoredOpenFile[] {
  if (!Array.isArray(value)) return []
  const seen = new Set<string>()
  return value.flatMap(item => {
    if (!isRecord(item) || typeof item.path !== 'string' || !item.path.startsWith('/')) return []
    if (typeof item.kind !== 'string' || !PREVIEW_KINDS.has(item.kind as PreviewKind)) return []
    if (seen.has(item.path)) return []
    seen.add(item.path)
    return [{
      path: item.path,
      name: typeof item.name === 'string' && item.name ? item.name : item.path.split('/').pop() || item.path,
      size: finiteNumber(item.size, 0, 0, Number.MAX_SAFE_INTEGER),
      type: typeof item.type === 'string' ? item.type : '',
      kind: item.kind as PreviewKind,
    }]
  })
}

export function readFilesWorkbenchState(): FilesWorkbenchState {
  if (typeof window === 'undefined') return { ...DEFAULT_FILES_WORKBENCH_STATE }
  try {
    const parsed: unknown = JSON.parse(window.localStorage.getItem(STORAGE_KEY) || '{}')
    if (!isRecord(parsed)) return { ...DEFAULT_FILES_WORKBENCH_STATE }
    if (parsed.version !== undefined && parsed.version !== 1) return { ...DEFAULT_FILES_WORKBENCH_STATE }

    const currentPath = typeof parsed.currentPath === 'string' && parsed.currentPath.startsWith('/') ? parsed.currentPath : '/'
    const history = Array.isArray(parsed.history)
      ? parsed.history.filter((path): path is string => typeof path === 'string' && path.startsWith('/'))
      : ['/']
    if (history.length === 0) history.push(currentPath)
    const historyIndex = Math.round(finiteNumber(parsed.historyIndex, history.length - 1, 0, history.length - 1))
    const expandedPaths = Array.isArray(parsed.expandedPaths)
      ? Array.from(new Set(parsed.expandedPaths.filter((path): path is string => typeof path === 'string' && path.startsWith('/'))))
      : ['/']
    if (!expandedPaths.includes('/')) expandedPaths.unshift('/')
    const openFiles = sanitizeOpenFiles(parsed.openFiles)
    const activeCandidate = typeof parsed.activeFilePath === 'string' ? parsed.activeFilePath : null
    const activeFilePath = activeCandidate && openFiles.some(file => file.path === activeCandidate) ? activeCandidate : null
    const fileViewStates = isRecord(parsed.fileViewStates)
      ? Object.entries(parsed.fileViewStates).reduce<Record<string, FileViewState>>((next, [path, value]) => {
          if (path.startsWith('/')) next[path] = sanitizeFileViewState(value)
          return next
        }, {})
      : {}

    return {
      version: 1,
      currentPath,
      history,
      historyIndex,
      viewMode: parsed.viewMode === 'grid' ? 'grid' : 'list',
      contentMode: parsed.contentMode === 'file' && activeFilePath ? 'file' : 'folder',
      expandedPaths,
      treeScrollTop: finiteNumber(parsed.treeScrollTop, 0, 0, 10_000_000),
      explorerWidth: finiteNumber(parsed.explorerWidth, DEFAULT_FILES_WORKBENCH_STATE.explorerWidth, 180, 560),
      openFiles,
      activeFilePath,
      fileViewStates,
    }
  } catch {
    return { ...DEFAULT_FILES_WORKBENCH_STATE }
  }
}

export function writeFilesWorkbenchState(state: FilesWorkbenchState): void {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify({
      ...state,
      version: 1,
      history: state.history.filter(path => path.startsWith('/')),
      expandedPaths: Array.from(new Set(state.expandedPaths.filter(path => path.startsWith('/')))),
      treeScrollTop: finiteNumber(state.treeScrollTop, 0, 0, 10_000_000),
      explorerWidth: finiteNumber(state.explorerWidth, DEFAULT_FILES_WORKBENCH_STATE.explorerWidth, 180, 560),
      openFiles: sanitizeOpenFiles(state.openFiles),
      fileViewStates: Object.entries(state.fileViewStates).reduce<Record<string, FileViewState>>((next, [path, value]) => {
        if (path.startsWith('/')) next[path] = sanitizeFileViewState(value)
        return next
      }, {}),
    }))
  } catch {
    // Quota/private-mode failures must not break file operations.
  }
}
