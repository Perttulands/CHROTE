import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { CSSProperties, ChangeEvent, DragEvent, KeyboardEvent as ReactKeyboardEvent, MouseEvent as ReactMouseEvent, PointerEvent as ReactPointerEvent } from 'react'
import { FileItem, toDisplayPath } from './types'
import {
  createFile,
  createFolder,
  deleteItem,
  fetchDirectory,
  getDownloadUrl,
  getErrorMessage,
  readTextFile,
  renameItem,
  sanitizeFilename,
  uploadFiles,
  writeTextFile,
} from './fileService'
import { formatDate, formatSize } from './utils'
import { useViewportMenuPosition } from '../../hooks/useViewportMenuPosition'
import { copyTextToClipboard } from '../../utils/clipboard'
import FileTree from '../FileTree'
import DismissiblePanel from '../DismissiblePanel'
import FileViewer, {
  MAX_TEXT_PREVIEW_BYTES,
  getFileBadge,
  getFileBaseName as getBaseName,
  getPreviewKind,
  makeFileItemFromPath,
  normalizeFilePath as normalizePath,
} from '../FileViewer'
import { DEFAULT_FILE_VIEW_STATE, type FileViewState } from '../workspaceFilesState'
import { FileContextMenu } from '../FileContextMenu'
import { getParentPath, joinFilePath, pathRelativeTo } from './pathActions'
import { usePinnedPaths, type SavedKind, type SavedPath } from './pinnedPaths'
import {
  readFilesWorkbenchState,
  writeFilesWorkbenchState,
  type FilesContentMode,
} from './filesWorkbenchState'
import {
  applyRead,
  closeAllBuffers,
  closeBuffer,
  closeOtherBuffers,
  describeBuffers,
  dirtyBuffersUnder,
  findBuffer,
  openBuffer,
  patchBuffer,
  pruneViewStates,
  remapBuffers,
  remapConflicts,
  remapViewStates,
  removeBuffersUnder,
  type OpenFile,
  type OpenFilesState,
} from './openFilesModel'

type SortKey = 'name' | 'size' | 'modified'
type SortDir = 'asc' | 'desc'
type ViewMode = 'list' | 'grid'
type CreateKind = 'file' | 'folder'
type SavedPathGroup = 'pinned' | 'recent'
type SavedGroupsCollapsed = Record<SavedPathGroup, boolean>

interface FilesViewProps {
  navigateRequest?: {
    path: string
    nonce: number
  } | null
  onSendPath?: (path: string) => void
  sendTargetLabel?: string | null
}

interface ContextMenuState {
  x: number
  y: number
  item: FileItem | null
}

interface TabContextMenuState {
  x: number
  y: number
  path: string
}

interface CreateIntent {
  kind: CreateKind
  parentPath: string
  name: string
}

const RECENT_STORAGE_KEY = 'chrote.files.recentPaths'
const SAVED_GROUPS_COLLAPSED_STORAGE_KEY = 'chrote.files.savedGroupsCollapsed'
const DEFAULT_SAVED_GROUPS_COLLAPSED: SavedGroupsCollapsed = {
  pinned: false,
  recent: false,
}

function savedPathLabel(path: string): string {
  const base = getBaseName(path)
  return base === '/' ? path : base
}

function readSavedPaths(key: string): SavedPath[] {
  if (typeof window === 'undefined') return []
  try {
    const raw = window.localStorage.getItem(key)
    if (!raw) return []
    const parsed = JSON.parse(raw) as unknown
    if (!Array.isArray(parsed)) return []
    return parsed
      .map((item): SavedPath | null => {
        if (typeof item === 'string') return { path: item, kind: 'file' }
        if (
          item &&
          typeof item === 'object' &&
          'path' in item &&
          typeof item.path === 'string'
        ) {
          const kind = 'kind' in item && item.kind === 'directory' ? 'directory' : 'file'
          return { path: item.path, kind }
        }
        return null
      })
      .filter((item): item is SavedPath => item !== null)
  } catch {
    return []
  }
}

function writeSavedPaths(key: string, paths: SavedPath[]): void {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(key, JSON.stringify(paths))
  } catch {
    // localStorage quota/private mode failures should not break the file UI.
  }
}

function readSavedGroupsCollapsed(): SavedGroupsCollapsed {
  if (typeof window === 'undefined') return { ...DEFAULT_SAVED_GROUPS_COLLAPSED }
  try {
    const raw = window.localStorage.getItem(SAVED_GROUPS_COLLAPSED_STORAGE_KEY)
    if (!raw) return { ...DEFAULT_SAVED_GROUPS_COLLAPSED }
    const parsed = JSON.parse(raw) as unknown
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return { ...DEFAULT_SAVED_GROUPS_COLLAPSED }
    const record = parsed as Partial<Record<SavedPathGroup, unknown>>
    return {
      pinned: record.pinned === true,
      recent: record.recent === true,
    }
  } catch {
    return { ...DEFAULT_SAVED_GROUPS_COLLAPSED }
  }
}

function writeSavedGroupsCollapsed(collapsed: SavedGroupsCollapsed): void {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(SAVED_GROUPS_COLLAPSED_STORAGE_KEY, JSON.stringify(collapsed))
  } catch {
    // localStorage quota/private mode failures should not break the file UI.
  }
}

function FilesView({ navigateRequest = null, onSendPath, sendTargetLabel = null }: FilesViewProps) {
  const [initialWorkbench] = useState(readFilesWorkbenchState)
  const uploadInputRef = useRef<HTMLInputElement | null>(null)
  const pathInputRef = useRef<HTMLInputElement | null>(null)
  const [items, setItems] = useState<FileItem[]>([])
  const [treeRefreshToken, setTreeRefreshToken] = useState(0)
  const [expandedPaths, setExpandedPaths] = useState<string[]>(initialWorkbench.expandedPaths)
  const [treeScrollTop, setTreeScrollTop] = useState(initialWorkbench.treeScrollTop)
  const [currentPath, setCurrentPath] = useState(initialWorkbench.currentPath)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [selectedPaths, setSelectedPaths] = useState<Set<string>>(new Set())
  const [sortBy, setSortBy] = useState<SortKey>('name')
  const [sortDir, setSortDir] = useState<SortDir>('asc')
  const [viewMode, setViewMode] = useState<ViewMode>(initialWorkbench.viewMode)
  const [contentMode, setContentMode] = useState<FilesContentMode>(initialWorkbench.contentMode)
  const [searchQuery, setSearchQuery] = useState('')
  const [history, setHistory] = useState<string[]>(initialWorkbench.history)
  const [historyIndex, setHistoryIndex] = useState(initialWorkbench.historyIndex)
  const [contextMenu, setContextMenu] = useState<ContextMenuState | null>(null)
  const [tabContextMenu, setTabContextMenu] = useState<TabContextMenuState | null>(null)
  const [renamingPath, setRenamingPath] = useState<string | null>(null)
  const [renameValue, setRenameValue] = useState('')
  const [createIntent, setCreateIntent] = useState<CreateIntent | null>(null)
  const [deleteTargets, setDeleteTargets] = useState<FileItem[] | null>(null)
  const [toast, setToast] = useState<string | null>(null)
  // openFilesModel owns every buffer transition; the view holds the state but
  // never rewrites the buffer set inline.
  const [openFilesState, setOpenFilesState] = useState<OpenFilesState>(() => ({
    files: initialWorkbench.openFiles.map((file, index) => ({
      ...file,
      content: '',
      dirty: false,
      loading: file.kind === 'text',
      error: null,
      readToken: index + 1,
    })),
    activePath: initialWorkbench.activeFilePath,
  }))
  const openFiles = openFilesState.files
  const activeFilePath = openFilesState.activePath
  const openFilesStateRef = useRef(openFilesState)
  openFilesStateRef.current = openFilesState
  // Monotonic for the life of the workbench, so a token is never reused by a
  // reopened tab while an older read for the same path is still in flight.
  const readTokenRef = useRef(initialWorkbench.openFiles.length)
  const nextReadToken = useCallback(() => {
    readTokenRef.current += 1
    return readTokenRef.current
  }, [])
  const setActiveFilePath = useCallback((path: string | null) => {
    setOpenFilesState(previous => ({ ...previous, activePath: path }))
  }, [])
  const [fileViewStates, setFileViewStates] = useState<Record<string, FileViewState>>(initialWorkbench.fileViewStates)
  const [pinnedPaths, togglePinnedPath] = usePinnedPaths()
  const [recentPaths, setRecentPaths] = useState<SavedPath[]>(() => readSavedPaths(RECENT_STORAGE_KEY))
  const [savedGroupsCollapsed, setSavedGroupsCollapsed] = useState<SavedGroupsCollapsed>(() => readSavedGroupsCollapsed())
  const [editingPath, setEditingPath] = useState(false)
  const [pathDraft, setPathDraft] = useState(initialWorkbench.currentPath)
  const [draggingPaths, setDraggingPaths] = useState<string[]>([])
  const [dropTargetPath, setDropTargetPath] = useState<string | null>(null)
  const [operationLabel, setOperationLabel] = useState<string | null>(null)
  const [explorerWidth, setExplorerWidth] = useState(initialWorkbench.explorerWidth)

  const currentPathPinned = pinnedPaths.some(item => item.path === currentPath)
  const workbenchStyle = { '--fb-explorer-width': `${explorerWidth}px` } as CSSProperties
  const tabContextMenuPosition = useViewportMenuPosition<HTMLDivElement>(
    tabContextMenu ? { x: tabContextMenu.x, y: tabContextMenu.y } : null,
    { estimatedSize: { width: 180, height: 84 } },
  )

  const showError = useCallback((message: string) => {
    setToast(message)
  }, [])

  const loadDirectory = useCallback(async (path: string) => {
    const normalized = normalizePath(path)
    setLoading(true)
    setError(null)
    try {
      const nextItems = await fetchDirectory(normalized)
      setItems(nextItems)
      setCurrentPath(normalized)
      setPathDraft(normalized)
      setSelectedPaths(new Set())
      setError(null)
      setExpandedPaths(previous => Array.from(new Set([...previous, '/', normalized])))
    } catch (loadError) {
      setItems([])
      setError(getErrorMessage(loadError, 'fetch'))
    } finally {
      setLoading(false)
    }
  }, [])

  const navigateTo = useCallback((path: string, addToHistory = true) => {
    const normalized = normalizePath(path)
    setContentMode('folder')
    if (addToHistory) {
      setHistory(prev => {
        const base = prev.slice(0, historyIndex + 1)
        if (base[base.length - 1] === normalized) return prev
        const next = [...base, normalized]
        setHistoryIndex(next.length - 1)
        return next
      })
    }
    void loadDirectory(normalized)
  }, [historyIndex, loadDirectory])

  const refreshCurrentPath = useCallback(() => {
    void loadDirectory(currentPath)
    setTreeRefreshToken(previous => previous + 1)
  }, [currentPath, loadDirectory])

  const startExplorerResize = useCallback((event: ReactPointerEvent<HTMLDivElement>) => {
    event.preventDefault()
    event.currentTarget.setPointerCapture(event.pointerId)
    const startX = event.clientX
    const startWidth = explorerWidth
    const pointerId = event.pointerId
    const move = (moveEvent: PointerEvent) => {
      if (moveEvent.pointerId === pointerId) setExplorerWidth(Math.min(560, Math.max(180, startWidth + moveEvent.clientX - startX)))
    }
    const finish = (upEvent: PointerEvent) => {
      if (upEvent.pointerId !== pointerId) return
      window.removeEventListener('pointermove', move)
      window.removeEventListener('pointerup', finish)
      window.removeEventListener('pointercancel', finish)
    }
    window.addEventListener('pointermove', move)
    window.addEventListener('pointerup', finish)
    window.addEventListener('pointercancel', finish)
  }, [explorerWidth])

  useEffect(() => {
    void loadDirectory(initialWorkbench.currentPath)
  }, [initialWorkbench.currentPath, loadDirectory])

  useEffect(() => {
    writeFilesWorkbenchState({
      version: 1,
      currentPath,
      history,
      historyIndex,
      viewMode,
      contentMode,
      expandedPaths,
      treeScrollTop,
      explorerWidth,
      openFiles: openFiles.map(({ path, name, size, type, kind }) => ({ path, name, size, type, kind })),
      activeFilePath,
      fileViewStates,
    })
  }, [activeFilePath, contentMode, currentPath, expandedPaths, explorerWidth, fileViewStates, history, historyIndex, openFiles, treeScrollTop, viewMode])

  useEffect(() => {
    if (!editingPath) {
      setPathDraft(currentPath)
    }
  }, [currentPath, editingPath])

  useEffect(() => {
    if (editingPath) {
      setTimeout(() => pathInputRef.current?.select(), 0)
    }
  }, [editingPath])


  const updateOpenFile = useCallback((path: string, patch: Partial<OpenFile>) => {
    setOpenFilesState(previous => patchBuffer(previous, path, patch))
  }, [])

  // Read results are applied through the model, which drops them unless the
  // buffer still holds the token the read was issued under. That is what makes
  // out-of-order, superseded, and post-move reads harmless.
  const readIntoBuffer = useCallback(async (path: string, readToken: number) => {
    try {
      const content = await readTextFile(path, MAX_TEXT_PREVIEW_BYTES)
      setOpenFilesState(previous => applyRead(previous, path, readToken, { content }))
    } catch (readError) {
      setOpenFilesState(previous => applyRead(previous, path, readToken, { error: getErrorMessage(readError, 'read') }))
    }
  }, [])

  const rememberRecent = useCallback((path: string) => {
    setRecentPaths(prev => {
      const next = [{ path, kind: 'file' as const }, ...prev.filter(item => item.path !== path)].slice(0, 10)
      writeSavedPaths(RECENT_STORAGE_KEY, next)
      return next
    })
  }, [])

  const openFile = useCallback(async (item: FileItem) => {
    if (item.isDir) {
      navigateTo(item.path)
      return
    }

    const kind = getPreviewKind(item)
    setContentMode('file')
    rememberRecent(item.path)

    // Reopening an already-open file focuses its tab. It is deliberately not
    // re-read: that is what used to overwrite unsaved edits with disk content.
    const result = openBuffer(openFilesStateRef.current, {
      path: item.path,
      name: item.name,
      size: item.size,
      type: item.type,
      kind,
      loading: kind === 'text' && item.size <= MAX_TEXT_PREVIEW_BYTES,
      error: kind === 'text' && item.size > MAX_TEXT_PREVIEW_BYTES ? 'File is too large for inline editing' : null,
    }, nextReadToken())
    openFilesStateRef.current = result.state
    setOpenFilesState(result.state)

    if (result.readToken !== null) {
      await readIntoBuffer(item.path, result.readToken)
    }
  }, [navigateTo, nextReadToken, readIntoBuffer, rememberRecent])

  useEffect(() => {
    // Restored tabs read once, under the token they were restored with.
    openFilesStateRef.current.files.forEach(file => {
      if (file.kind !== 'text' || !file.loading) return
      if (file.size > MAX_TEXT_PREVIEW_BYTES) {
        updateOpenFile(file.path, { loading: false, error: 'File is too large for inline editing' })
        return
      }
      void readIntoBuffer(file.path, file.readToken)
    })
    // Restore runs once for the state the workbench mounted with.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    if (!navigateRequest) return
    const requestedPath = normalizePath(navigateRequest.path)
    const openRequestedPath = async () => {
      const parentPath = getParentPath(requestedPath)
      try {
        const siblings = await fetchDirectory(parentPath)
        const item = siblings.find(candidate => candidate.path === requestedPath)
        if (item?.isDir) {
          navigateTo(item.path)
        } else if (item) {
          await loadDirectory(parentPath)
          await openFile(item)
        } else {
          navigateTo(requestedPath)
        }
      } catch {
        navigateTo(requestedPath)
      }
    }
    void openRequestedPath()
  }, [navigateRequest?.nonce])

  const openSavedPath = useCallback((saved: SavedPath) => {
    if (saved.kind === 'directory') {
      navigateTo(saved.path)
      return
    }
    void openFile(makeFileItemFromPath(saved.path))
  }, [navigateTo, openFile])

  // Closing one tab has an unambiguous target, so a dirty buffer asks for an
  // explicit discard. Bulk closes cover many buffers at once and keep blocking
  // instead, so the operator resolves them one by one.
  const closeOpenFile = useCallback((path: string) => {
    const target = findBuffer(openFilesStateRef.current, path)
    if (target?.dirty && !window.confirm(`${target.name} has unsaved changes. Discard them?`)) return

    setOpenFilesState(previous => {
      const next = closeBuffer(previous, path)
      if (next.files.length === 0) setContentMode('folder')
      return next
    })
  }, [])

  const closeAllOpenFiles = useCallback(() => {
    setTabContextMenu(null)
    if (openFiles.some(file => file.dirty)) {
      showError('Save or close unsaved files before using Close All.')
      return
    }

    setOpenFilesState(closeAllBuffers())
    setContentMode('folder')
  }, [openFiles, showError])

  const closeOtherOpenFiles = useCallback((path: string) => {
    setTabContextMenu(null)
    const fileToKeep = openFiles.find(file => file.path === path)
    if (!fileToKeep) return

    const filesToClose = openFiles.filter(file => file.path !== path)
    if (filesToClose.some(file => file.dirty)) {
      showError('Save or close unsaved files before closing other tabs.')
      return
    }

    setOpenFilesState(previous => closeOtherBuffers(previous, path))
  }, [openFiles, showError])

  const activeFile = useMemo(
    () => openFiles.find(file => file.path === activeFilePath) || null,
    [activeFilePath, openFiles]
  )

  const selectedItems = useMemo(
    () => items.filter(item => selectedPaths.has(item.path)),
    [items, selectedPaths]
  )

  const visibleItems = useMemo(() => {
    const query = searchQuery.trim().toLowerCase()
    const filtered = query ? items.filter(item => item.name.toLowerCase().includes(query)) : items
    const sorted = [...filtered].sort((a, b) => {
      if (a.isDir !== b.isDir) return a.isDir ? -1 : 1

      let result = 0
      if (sortBy === 'name') result = a.name.localeCompare(b.name)
      if (sortBy === 'size') result = a.size - b.size
      if (sortBy === 'modified') result = new Date(a.modified).getTime() - new Date(b.modified).getTime()
      return sortDir === 'asc' ? result : -result
    })
    return sorted
  }, [items, searchQuery, sortBy, sortDir])

  const toggleSort = (key: SortKey) => {
    setSortBy(prev => {
      if (prev === key) {
        setSortDir(dir => (dir === 'asc' ? 'desc' : 'asc'))
        return prev
      }
      setSortDir('asc')
      return key
    })
  }

  const goBack = () => {
    if (historyIndex <= 0) return
    const nextIndex = historyIndex - 1
    setHistoryIndex(nextIndex)
    void loadDirectory(history[nextIndex])
  }

  const goForward = () => {
    if (historyIndex >= history.length - 1) return
    const nextIndex = historyIndex + 1
    setHistoryIndex(nextIndex)
    void loadDirectory(history[nextIndex])
  }

  const goUp = () => {
    if (currentPath === '/') return
    navigateTo(getParentPath(currentPath))
  }

  const handlePathSubmit = () => {
    setEditingPath(false)
    navigateTo(pathDraft)
  }

  const handleSelection = (item: FileItem, event: ReactMouseEvent) => {
    if (event.metaKey || event.ctrlKey) {
      setSelectedPaths(prev => {
        const next = new Set(prev)
        if (next.has(item.path)) next.delete(item.path)
        else next.add(item.path)
        return next
      })
      return
    }

    if (event.shiftKey && selectedPaths.size > 0) {
      const lastPath = Array.from(selectedPaths).pop()
      const lastIndex = visibleItems.findIndex(candidate => candidate.path === lastPath)
      const currentIndex = visibleItems.findIndex(candidate => candidate.path === item.path)
      if (lastIndex >= 0 && currentIndex >= 0) {
        const [start, end] = [Math.min(lastIndex, currentIndex), Math.max(lastIndex, currentIndex)]
        setSelectedPaths(new Set(visibleItems.slice(start, end + 1).map(candidate => candidate.path)))
        return
      }
    }

    setSelectedPaths(new Set([item.path]))
  }

  const handleItemClick = (item: FileItem, event: ReactMouseEvent) => {
    handleSelection(item, event)
    if (!item.isDir) {
      void openFile(item)
    }
  }

  const handleContextMenu = (event: ReactMouseEvent, item: FileItem | null) => {
    event.preventDefault()
    setContextMenu({ x: event.clientX, y: event.clientY, item })
    if (item && !selectedPaths.has(item.path)) {
      setSelectedPaths(new Set([item.path]))
    }
  }

  const beginRename = (item: FileItem) => {
    setRenamingPath(item.path)
    setRenameValue(item.name)
    setContextMenu(null)
  }

  const cancelRename = () => {
    setRenamingPath(null)
    setRenameValue('')
  }

  const submitRename = async (item: FileItem) => {
    if (!renameValue.trim() || renameValue === item.name) {
      cancelRename()
      return
    }

    const safeName = sanitizeFilename(renameValue)
    const destination = joinFilePath(getParentPath(item.path), safeName)
    const conflicts = remapConflicts(openFilesStateRef.current, item.path, destination)
    if (conflicts.length > 0) {
      showError(`Close the open tab at ${conflicts[0]} before renaming onto it.`)
      cancelRename()
      return
    }

    setOperationLabel('Renaming')
    try {
      await renameItem(item.path, destination)
      // Remap buffers and view state together so open tabs — including
      // descendants of a renamed folder — follow the file to its new path.
      setOpenFilesState(previous => remapBuffers(previous, item.path, destination))
      setFileViewStates(previous => remapViewStates(previous, item.path, destination))
      await loadDirectory(currentPath)
      setTreeRefreshToken(previous => previous + 1)
    } catch (renameError) {
      showError(getErrorMessage(renameError, 'rename'))
    } finally {
      setOperationLabel(null)
      cancelRename()
    }
  }

  const startCreate = (kind: CreateKind, parentPath = currentPath) => {
    setCreateIntent({
      kind,
      parentPath,
      name: kind === 'folder' ? 'new-folder' : 'new-file.txt',
    })
    setContextMenu(null)
  }

  const cancelCreate = () => {
    setCreateIntent(null)
  }

  const confirmCreate = async () => {
    if (!createIntent) return
    setOperationLabel(createIntent.kind === 'folder' ? 'Creating folder' : 'Creating file')
    try {
      if (createIntent.kind === 'folder') {
        await createFolder(createIntent.parentPath, createIntent.name)
      } else {
        const path = await createFile(createIntent.parentPath, createIntent.name)
        await openFile(makeFileItemFromPath(path))
      }
      await loadDirectory(currentPath)
      setTreeRefreshToken(previous => previous + 1)
      setCreateIntent(null)
    } catch (createError) {
      showError(getErrorMessage(createError, 'create'))
    } finally {
      setOperationLabel(null)
    }
  }

  const requestDelete = (targets: FileItem[]) => {
    if (targets.length === 0) return
    setDeleteTargets(targets)
    setContextMenu(null)
  }

  const confirmDelete = async () => {
    if (!deleteTargets) return
    const deletedPaths = deleteTargets.map(item => item.path)

    // Deleting the file under an unsaved buffer destroys the edit and the disk
    // copy at once, so it blocks the same way a bulk close does.
    const dirty = dirtyBuffersUnder(openFilesStateRef.current, deletedPaths)
    if (dirty.length > 0) {
      showError(`Save or close unsaved files before deleting: ${describeBuffers(dirty)}.`)
      setDeleteTargets(null)
      return
    }

    setOperationLabel('Deleting')
    try {
      for (const target of deleteTargets) {
        await deleteItem(target.path)
      }
      setOpenFilesState(previous => removeBuffersUnder(previous, deletedPaths))
      setFileViewStates(previous => pruneViewStates(previous, deletedPaths))
      await loadDirectory(currentPath)
      setTreeRefreshToken(previous => previous + 1)
      setDeleteTargets(null)
    } catch (deleteError) {
      showError(getErrorMessage(deleteError, 'delete'))
    } finally {
      setOperationLabel(null)
    }
  }

  const handleUpload = async (fileList: FileList | File[]) => {
    const files = Array.isArray(fileList) ? fileList : Array.from(fileList)
    if (files.length === 0) return

    setOperationLabel(`Uploading ${files.length} item${files.length === 1 ? '' : 's'}`)
    try {
      await uploadFiles(currentPath, files)
      await loadDirectory(currentPath)
      setTreeRefreshToken(previous => previous + 1)
    } catch (uploadError) {
      showError(getErrorMessage(uploadError, 'upload'))
    } finally {
      setOperationLabel(null)
      if (uploadInputRef.current) uploadInputRef.current.value = ''
    }
  }

  const handleUploadInput = (event: ChangeEvent<HTMLInputElement>) => {
    if (event.target.files) {
      void handleUpload(event.target.files)
    }
  }

  const handleDropUpload = (event: DragEvent<HTMLDivElement>) => {
    event.preventDefault()
    if (event.dataTransfer.files.length > 0) {
      void handleUpload(event.dataTransfer.files)
    }
  }

  const downloadItems = (targets: FileItem[]) => {
    targets.filter(item => !item.isDir).slice(0, 6).forEach(item => {
      window.open(getDownloadUrl(item.path), '_blank')
    })
    setContextMenu(null)
  }

  const copyPath = (path: string) => {
    void copyTextToClipboard(toDisplayPath(path))
    setContextMenu(null)
  }

  const pathRelativeToCurrent = (path: string) => {
    return pathRelativeTo(currentPath, path)
  }

  const copySelectedPaths = (targets: FileItem[]) => {
    if (targets.length === 0) return
    void copyTextToClipboard(targets.map(target => toDisplayPath(target.path)).join('\n'))
    setContextMenu(null)
  }

  const copyRelativePath = (path: string) => {
    void copyTextToClipboard(pathRelativeToCurrent(path))
    setContextMenu(null)
  }

  const openParentFolder = (path: string) => {
    navigateTo(getParentPath(path))
    setContextMenu(null)
  }

  const togglePin = (path: string, kind: SavedKind) => {
    togglePinnedPath(path, kind)
    setContextMenu(null)
  }

  const toggleSavedGroup = (group: SavedPathGroup) => {
    setSavedGroupsCollapsed(prev => {
      const next = {
        ...prev,
        [group]: !prev[group],
      }
      writeSavedGroupsCollapsed(next)
      return next
    })
  }

  const saveActiveFile = async () => {
    if (!activeFile || activeFile.kind !== 'text') return
    setOperationLabel('Saving')
    try {
      await writeTextFile(activeFile.path, activeFile.content)
      updateOpenFile(activeFile.path, { dirty: false })
      await loadDirectory(currentPath)
    } catch (saveError) {
      showError(getErrorMessage(saveError, 'save'))
    } finally {
      setOperationLabel(null)
    }
  }

  const handleInternalDragStart = (event: DragEvent<HTMLElement>, item: FileItem) => {
    const paths = selectedPaths.has(item.path) ? Array.from(selectedPaths) : [item.path]
    setDraggingPaths(paths)
    event.dataTransfer.effectAllowed = 'move'
    event.dataTransfer.setData('application/x-chrote-paths', JSON.stringify(paths))
  }

  const handleFolderDragOver = (event: DragEvent<HTMLElement>, item: FileItem) => {
    if (!item.isDir || draggingPaths.length === 0) return
    if (draggingPaths.some(path => path === item.path || item.path.startsWith(`${path}/`))) return
    event.preventDefault()
    event.dataTransfer.dropEffect = 'move'
    setDropTargetPath(item.path)
  }

  const handleFolderDrop = async (event: DragEvent<HTMLElement>, target: FileItem) => {
    event.preventDefault()
    setDropTargetPath(null)
    if (!target.isDir || draggingPaths.length === 0) return

    const moves = draggingPaths.map(sourcePath => ({
      sourcePath,
      destination: joinFilePath(target.path, getBaseName(sourcePath)),
    }))
    const conflict = moves
      .flatMap(move => remapConflicts(openFilesStateRef.current, move.sourcePath, move.destination))
      .find(Boolean)
    if (conflict) {
      showError(`Close the open tab at ${conflict} before moving onto it.`)
      setDraggingPaths([])
      return
    }

    setOperationLabel('Moving')
    try {
      for (const { sourcePath, destination } of moves) {
        await renameItem(sourcePath, destination)
        // Remap after each successful move so a mid-batch failure still leaves
        // the moved buffers pointing at their real paths.
        setOpenFilesState(previous => remapBuffers(previous, sourcePath, destination))
        setFileViewStates(previous => remapViewStates(previous, sourcePath, destination))
      }
      await loadDirectory(currentPath)
      setTreeRefreshToken(previous => previous + 1)
      setTreeRefreshToken(previous => previous + 1)
    } catch (moveError) {
      showError(getErrorMessage(moveError, 'move'))
    } finally {
      setOperationLabel(null)
      setDraggingPaths([])
    }
  }

  useEffect(() => {
    const handleKeyDown = (event: globalThis.KeyboardEvent) => {
      const target = event.target as HTMLElement
      if (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA') {
        if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 's') {
          event.preventDefault()
          void saveActiveFile()
        }
        return
      }

      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 's') {
        event.preventDefault()
        void saveActiveFile()
      } else if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'f') {
        event.preventDefault()
        document.querySelector<HTMLInputElement>('.fb-search')?.focus()
      } else if (event.key === 'F5') {
        event.preventDefault()
        refreshCurrentPath()
      } else if (event.key === 'Backspace') {
        event.preventDefault()
        goUp()
      } else if (event.key === 'F2' && selectedItems.length === 1) {
        beginRename(selectedItems[0])
      } else if (event.key === 'Delete') {
        requestDelete(selectedItems)
      } else if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'a') {
        event.preventDefault()
        setSelectedPaths(new Set(visibleItems.map(item => item.path)))
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [refreshCurrentPath, saveActiveFile, selectedItems, visibleItems])

  const renderSavedPath = (item: SavedPath, className: string) => (
    <button
      key={`${item.kind}:${item.path}`}
      className={className}
      type="button"
      title={item.path}
      onClick={() => openSavedPath(item)}
      onContextMenu={(event) => {
        event.preventDefault()
        setContextMenu({
          x: event.clientX,
          y: event.clientY,
          item: item.kind === 'file' ? makeFileItemFromPath(item.path) : {
            name: savedPathLabel(item.path),
            path: item.path,
            isDir: true,
            size: 0,
            modified: new Date().toISOString(),
            type: '',
          },
        })
      }}
    >
      <span className="fb-mini-glyph">{item.kind === 'directory' ? 'DIR' : getFileBadge(makeFileItemFromPath(item.path))}</span>
      <span className="fb-saved-name">{savedPathLabel(item.path)}</span>
    </button>
  )

  const renderSavedPathGroup = (group: SavedPathGroup, title: string, paths: SavedPath[]) => {
    if (paths.length === 0) return null

    const collapsed = savedGroupsCollapsed[group]
    const listId = `fb-${group}-saved-list`

    return (
      <section className={`fb-sidebar-section fb-saved-section ${collapsed ? 'is-collapsed' : ''}`}>
        <button
          className="fb-section-title fb-section-toggle"
          type="button"
          aria-expanded={!collapsed}
          aria-controls={listId}
          onClick={() => toggleSavedGroup(group)}
        >
          <span className="fb-section-caret" aria-hidden="true">{collapsed ? '>' : 'v'}</span>
          <span>{title}</span>
          <span className="fb-section-count">{paths.length}</span>
        </button>
        {!collapsed && (
          <div id={listId} className="fb-saved-list">
            {paths.map(item => renderSavedPath(item, 'fb-saved-item'))}
          </div>
        )}
      </section>
    )
  }

  const renderCreateRow = () => {
    if (!createIntent || createIntent.parentPath !== currentPath) return null
    return (
      <div className="fb-row fb-row-editing">
        <span className="fb-file-glyph folder">{createIntent.kind === 'folder' ? 'DIR' : 'NEW'}</span>
        <input
          className="fb-rename-input"
          value={createIntent.name}
          onChange={(event) => setCreateIntent(prev => prev ? { ...prev, name: event.target.value } : prev)}
          onKeyDown={(event: ReactKeyboardEvent<HTMLInputElement>) => {
            if (event.key === 'Enter') void confirmCreate()
            if (event.key === 'Escape') cancelCreate()
          }}
          onBlur={() => void confirmCreate()}
          autoFocus
        />
      </div>
    )
  }

  const renderItemName = (item: FileItem) => {
    if (renamingPath === item.path) {
      return (
        <input
          className="fb-rename-input"
          value={renameValue}
          onChange={(event) => setRenameValue(event.target.value)}
          onClick={(event) => event.stopPropagation()}
          onKeyDown={(event: ReactKeyboardEvent<HTMLInputElement>) => {
            if (event.key === 'Enter') void submitRename(item)
            if (event.key === 'Escape') cancelRename()
          }}
          onBlur={() => void submitRename(item)}
          autoFocus
        />
      )
    }

    return <span className="fb-filename" title={item.name}>{item.name}</span>
  }

  const renderListItem = (item: FileItem) => {
    const selected = selectedPaths.has(item.path)
    const dropTarget = dropTargetPath === item.path

    return (
      <div
        className={`fb-row ${selected ? 'selected' : ''} ${dropTarget ? 'drop-target' : ''}`}
        key={item.path}
        role="row"
        tabIndex={0}
        draggable
        onClick={(event) => handleItemClick(item, event)}
        onDoubleClick={() => item.isDir ? navigateTo(item.path) : void openFile(item)}
        onContextMenu={(event) => handleContextMenu(event, item)}
        onDragStart={(event) => handleInternalDragStart(event, item)}
        onDragEnd={() => {
          setDraggingPaths([])
          setDropTargetPath(null)
        }}
        onDragOver={(event) => handleFolderDragOver(event, item)}
        onDragLeave={() => setDropTargetPath(null)}
        onDrop={(event) => void handleFolderDrop(event, item)}
      >
        <div className="fb-cell fb-cell-name">
          <span className={`fb-file-glyph ${item.isDir ? 'folder' : 'file'}`}>{getFileBadge(item)}</span>
          {renderItemName(item)}
        </div>
        <div className="fb-cell fb-cell-size">{item.isDir ? '-' : formatSize(item.size)}</div>
        <div className="fb-cell fb-cell-modified">{formatDate(item.modified)}</div>
      </div>
    )
  }

  const renderGridItem = (item: FileItem) => {
    const selected = selectedPaths.has(item.path)
    const dropTarget = dropTargetPath === item.path

    return (
      <div
        className={`fb-grid-item ${selected ? 'selected' : ''} ${dropTarget ? 'drop-target' : ''}`}
        key={item.path}
        tabIndex={0}
        draggable
        onClick={(event) => handleItemClick(item, event)}
        onDoubleClick={() => item.isDir ? navigateTo(item.path) : void openFile(item)}
        onContextMenu={(event) => handleContextMenu(event, item)}
        onDragStart={(event) => handleInternalDragStart(event, item)}
        onDragEnd={() => {
          setDraggingPaths([])
          setDropTargetPath(null)
        }}
        onDragOver={(event) => handleFolderDragOver(event, item)}
        onDragLeave={() => setDropTargetPath(null)}
        onDrop={(event) => void handleFolderDrop(event, item)}
      >
        <span className={`fb-grid-icon ${item.isDir ? 'folder' : 'file'}`}>{getFileBadge(item)}</span>
        <span className="fb-grid-name" title={item.name}>{item.name}</span>
        <span className="fb-grid-meta">{item.isDir ? 'Folder' : formatSize(item.size)}</span>
      </div>
    )
  }

  const activeFileItem: FileItem | null = activeFile ? {
    path: activeFile.path,
    name: activeFile.name,
    size: activeFile.size,
    type: activeFile.type,
    isDir: false,
    modified: '',
  } : null
  const activeViewState = activeFilePath ? fileViewStates[activeFilePath] || DEFAULT_FILE_VIEW_STATE : DEFAULT_FILE_VIEW_STATE
  const updateActiveViewState = (next: FileViewState) => {
    if (!activeFilePath) return
    setFileViewStates(previous => ({ ...previous, [activeFilePath]: next }))
  }
  const imageItems = items.filter(item => !item.isDir && getPreviewKind(item) === 'image')
  const activeImageIndex = activeFile ? imageItems.findIndex(item => item.path === activeFile.path) : -1
  const previousImage = activeImageIndex > 0 ? imageItems[activeImageIndex - 1] : null
  const nextImage = activeImageIndex >= 0 && activeImageIndex < imageItems.length - 1 ? imageItems[activeImageIndex + 1] : null

  const contextTargets = contextMenu?.item
    ? [contextMenu.item]
    : selectedItems

  return (
    <div className="fb-container files-view">
      {toast && (
        <div className="fb-error-toast" role="alert">
          <span className="fb-error-toast-icon">!</span>
          <span className="fb-error-toast-message">{toast}</span>
          <button className="fb-error-toast-dismiss" type="button" onClick={() => setToast(null)}>x</button>
        </div>
      )}

      <input
        ref={uploadInputRef}
        className="fb-hidden-input"
        type="file"
        multiple
        onChange={handleUploadInput}
      />

      <div className="fb-header">
        <div className="fb-header-left">
          <h2 className="fb-title">Files</h2>
          <div className="fb-tabs" aria-label="File workbench views">
            <button className="fb-tab active" type="button">Explorer</button>
            <button
              className={`fb-tab ${currentPathPinned ? 'active' : ''}`}
              type="button"
              title={currentPathPinned ? 'Unpin current folder' : 'Pin current folder'}
              aria-pressed={currentPathPinned}
              onClick={() => togglePin(currentPath, 'directory')}
            >
              {currentPathPinned ? 'Unpin' : 'Pin'}
            </button>
          </div>
        </div>
        <div className="fb-header-right">
          <button className="fb-btn" type="button" title="New File" onClick={() => startCreate('file')}>+ File</button>
          <button className="fb-btn" type="button" title="New Folder" onClick={() => startCreate('folder')}>+ Folder</button>
          <button className="fb-btn" type="button" title="Upload" onClick={() => uploadInputRef.current?.click()}>Upload</button>
          <button className="fb-btn" type="button" title="Refresh" disabled={loading} onClick={refreshCurrentPath}>Refresh</button>
        </div>
      </div>

      <div className="fb-workbench" style={workbenchStyle}>
        <aside className="fb-sidebar" aria-label="Explorer">
          <div className="fb-sidebar-header">
            <span>Explorer</span>
            <button className="fb-sidebar-action" type="button" title="Refresh tree" onClick={() => setTreeRefreshToken(previous => previous + 1)}>Refresh</button>
          </div>

          {renderSavedPathGroup('pinned', 'Pinned', pinnedPaths)}
          {renderSavedPathGroup('recent', 'Recent', recentPaths)}

          <section className="fb-sidebar-section fb-tree-section">
            <div className="fb-section-title">Workspace</div>
            <FileTree
              currentPath={currentPath}
              selectedPath={activeFilePath || currentPath}
              expandedPaths={expandedPaths}
              scrollTop={treeScrollTop}
              refreshToken={treeRefreshToken}
              onOpenDirectory={navigateTo}
              onOpenFile={item => void openFile(item)}
              onExpandedPathsChange={setExpandedPaths}
              onScrollTopChange={setTreeScrollTop}
              onItemContextMenu={(event, item) => handleContextMenu(event, item)}
            />
          </section>
        </aside>
        <div
          className="fb-explorer-resizer"
          role="separator"
          aria-label="Resize Files explorer"
          aria-orientation="vertical"
          tabIndex={0}
          onPointerDown={startExplorerResize}
          onKeyDown={event => {
            if (event.key === 'ArrowLeft') setExplorerWidth(previous => Math.max(180, previous - 16))
            if (event.key === 'ArrowRight') setExplorerWidth(previous => Math.min(560, previous + 16))
          }}
        />

        <main className="fb-main">
          <div className="fb-toolbar">
            <div className="fb-toolbar-nav">
              <button className="fb-nav-btn" type="button" title="Back" disabled={historyIndex === 0} onClick={goBack}>Back</button>
              <button className="fb-nav-btn" type="button" title="Forward" disabled={historyIndex >= history.length - 1} onClick={goForward}>Forward</button>
              <button className="fb-nav-btn" type="button" title="Up" disabled={currentPath === '/'} onClick={goUp}>Up</button>
            </div>

            <div className="fb-pathbar">
              {editingPath ? (
                <input
                  ref={pathInputRef}
                  className="fb-path-input"
                  value={pathDraft}
                  onChange={(event) => setPathDraft(event.target.value)}
                  onBlur={handlePathSubmit}
                  onKeyDown={(event) => {
                    if (event.key === 'Enter') handlePathSubmit()
                    if (event.key === 'Escape') setEditingPath(false)
                  }}
                />
              ) : (
                <button className="fb-path-display" type="button" onClick={() => setEditingPath(true)} title="Edit path">
                  <span
                    className="fb-breadcrumb-root"
                    onClick={(event) => {
                      event.stopPropagation()
                      navigateTo('/')
                    }}
                  >
                    /
                  </span>
                  {currentPath.split('/').filter(Boolean).map((part, index, parts) => {
                    const path = `/${parts.slice(0, index + 1).join('/')}`
                    return (
                      <span className="fb-breadcrumb-segment" key={path}>
                        <span className="fb-breadcrumb-sep">/</span>
                        <span className="fb-breadcrumb-item">{part}</span>
                      </span>
                    )
                  })}
                </button>
              )}
            </div>

            <div className="fb-toolbar-actions">
              <button
                className={`fb-view-btn ${contentMode === 'folder' ? 'active' : ''}`}
                type="button"
                aria-pressed={contentMode === 'folder'}
                onClick={() => setContentMode('folder')}
              >
                Folder
              </button>
              <button
                className={`fb-view-btn ${contentMode === 'file' ? 'active' : ''}`}
                type="button"
                aria-pressed={contentMode === 'file'}
                disabled={!activeFile}
                onClick={() => setContentMode('file')}
              >
                File
              </button>
              {contentMode === 'folder' && (
                <>
                  <input
                    className="fb-search"
                    type="search"
                    placeholder="Filter"
                    value={searchQuery}
                    onChange={(event) => setSearchQuery(event.target.value)}
                  />
                  <button className={`fb-view-btn ${viewMode === 'list' ? 'active' : ''}`} type="button" title="List view" onClick={() => setViewMode('list')}>List</button>
                  <button className={`fb-view-btn ${viewMode === 'grid' ? 'active' : ''}`} type="button" title="Grid view" onClick={() => setViewMode('grid')}>Grid</button>
                </>
              )}
            </div>
          </div>

          <div
            className={`fb-content mode-${contentMode}`}
            onDragOver={(event) => event.preventDefault()}
            onDrop={handleDropUpload}
            onContextMenu={(event) => {
              if (!(event.target as HTMLElement).closest('.fb-row, .fb-grid-item')) {
                handleContextMenu(event, null)
              }
            }}
          >
            {contentMode === 'folder' && (
              <section className="fb-browser-pane">
              <div className="fb-list-container">
                {loading ? (
                  <div className="fb-loading">
                    <span className="fb-spinner" />
                    Loading...
                  </div>
                ) : error ? (
                  <div className="fb-error">
                    <span className="fb-error-icon">!</span>
                    <span className="fb-error-message">{error}</span>
                    <button className="fb-retry-btn" type="button" onClick={() => loadDirectory(currentPath)}>Retry</button>
                  </div>
                ) : viewMode === 'list' ? (
                  <div className="fb-list" role="grid">
                    <div className="fb-list-header" role="row">
                      <button className={`fb-column-header fb-cell-name ${sortBy === 'name' ? 'active' : ''}`} type="button" onClick={() => toggleSort('name')}>
                        Name {sortBy === 'name' ? (sortDir === 'asc' ? '^' : 'v') : ''}
                      </button>
                      <button className={`fb-column-header fb-cell-size ${sortBy === 'size' ? 'active' : ''}`} type="button" onClick={() => toggleSort('size')}>
                        Size {sortBy === 'size' ? (sortDir === 'asc' ? '^' : 'v') : ''}
                      </button>
                      <button className={`fb-column-header fb-cell-modified ${sortBy === 'modified' ? 'active' : ''}`} type="button" onClick={() => toggleSort('modified')}>
                        Modified {sortBy === 'modified' ? (sortDir === 'asc' ? '^' : 'v') : ''}
                      </button>
                    </div>
                    <div className="fb-list-body">
                      {renderCreateRow()}
                      {visibleItems.length === 0 ? (
                        <div className="fb-empty">
                          <span className="fb-empty-icon">EMPTY</span>
                          {searchQuery ? 'No matching files' : 'This folder is empty'}
                        </div>
                      ) : (
                        visibleItems.map(renderListItem)
                      )}
                    </div>
                  </div>
                ) : (
                  <div className="fb-grid" role="grid">
                    {renderCreateRow()}
                    {visibleItems.length === 0 ? (
                      <div className="fb-empty">
                        <span className="fb-empty-icon">EMPTY</span>
                        {searchQuery ? 'No matching files' : 'This folder is empty'}
                      </div>
                    ) : (
                      visibleItems.map(renderGridItem)
                    )}
                  </div>
                )}
              </div>

              <div className="fb-statusbar">
                <span>{visibleItems.length} items</span>
                {selectedPaths.size > 0 && <span>{selectedPaths.size} selected</span>}
                {operationLabel && <span className="fb-statusbar-operation">{operationLabel}...</span>}
              </div>
            </section>
            )}

            {contentMode === 'file' && (
              <aside className="fb-editor-pane">
              <div className="fb-editor-tabs">
                {openFiles.length === 0 ? (
                  <span className="fb-editor-placeholder-tab">Preview</span>
                ) : (
                  <>
                    {openFiles.map(file => (
                      <button
                        key={file.path}
                        className={`fb-editor-tab ${file.path === activeFilePath ? 'active' : ''}`}
                        type="button"
                        title={file.path}
                        onClick={() => {
                          setActiveFilePath(file.path)
                          setContentMode('file')
                        }}
                        onContextMenu={(event) => {
                          event.preventDefault()
                          event.stopPropagation()
                          setTabContextMenu({ x: event.clientX, y: event.clientY, path: file.path })
                        }}
                      >
                        <span>{file.dirty ? '* ' : ''}{file.name}</span>
                        <span
                          className="fb-editor-tab-close"
                          role="button"
                          tabIndex={0}
                          onClick={(event) => {
                            event.stopPropagation()
                            closeOpenFile(file.path)
                          }}
                          onKeyDown={(event) => {
                            if (event.key === 'Enter' || event.key === ' ') {
                              event.stopPropagation()
                              closeOpenFile(file.path)
                            }
                          }}
                        >
                          x
                        </span>
                      </button>
                    ))}
                    {openFiles.length > 1 && (
                      <button
                        className="fb-editor-tabs-close-all"
                        type="button"
                        title="Close all open files"
                        onClick={closeAllOpenFiles}
                      >
                        Close All
                      </button>
                    )}
                  </>
                )}
              </div>

              {activeFile ? (
                <div className="fb-editor">
                  <div className="fb-editor-header">
                    <div className="fb-editor-title">
                      <span className="fb-file-glyph file">{getFileBadge(makeFileItemFromPath(activeFile.path))}</span>
                      <div>
                        <strong>{activeFile.name}</strong>
                        <span>{activeFile.path}</span>
                      </div>
                    </div>
                    <div className="fb-editor-actions">
                      {activeFile.kind === 'text' && (
                        <button className="fb-btn" type="button" disabled={!activeFile.dirty || activeFile.loading} onClick={() => void saveActiveFile()}>Save</button>
                      )}
                      <button className="fb-btn" type="button" onClick={() => downloadItems([makeFileItemFromPath(activeFile.path)])}>Download</button>
                      <button className="fb-btn" type="button" onClick={() => copyPath(activeFile.path)}>Copy Path</button>
                      <button
                        className="fb-btn"
                        type="button"
                        disabled={!onSendPath}
                        title={onSendPath ? `Send path to ${sendTargetLabel || 'focused session'}` : 'Focus a terminal session first'}
                        onClick={() => onSendPath?.(activeFile.path)}
                      >
                        Send Path
                      </button>
                    </div>
                  </div>

                  {activeFile.loading ? (
                    <div className="fb-editor-empty">Loading file...</div>
                  ) : activeFile.error ? (
                    <div className="fb-editor-empty">{activeFile.error}</div>
                  ) : activeFileItem ? (
                    <FileViewer
                      item={activeFileItem}
                      content={activeFile.kind === 'text' ? activeFile.content : undefined}
                      editable={activeFile.kind === 'text'}
                      onContentChange={content => updateOpenFile(activeFile.path, { content, dirty: true })}
                      viewState={activeViewState}
                      onViewStateChange={updateActiveViewState}
                      onPrevious={previousImage ? () => void openFile(previousImage) : undefined}
                      onNext={nextImage ? () => void openFile(nextImage) : undefined}
                    />
                  ) : null}
                </div>
              ) : (
                <div className="fb-editor-empty">
                  <strong>No file selected</strong>
                  <span>Select a file to preview or edit it here.</span>
                </div>
              )}
              </aside>
            )}
          </div>
        </main>
      </div>

      {tabContextMenu && openFiles.length > 1 && (
        <DismissiblePanel onDismiss={() => setTabContextMenu(null)} panelZIndex={2200} panelPosition="fixed">
          <div
            ref={tabContextMenuPosition.ref}
            className="fb-context-menu fb-tab-context-menu"
            style={tabContextMenuPosition.style}
          >
            <button className="fb-context-item" type="button" onClick={() => closeOtherOpenFiles(tabContextMenu.path)}>Close Others</button>
            <button className="fb-context-item" type="button" onClick={closeAllOpenFiles}>Close All</button>
          </div>
        </DismissiblePanel>
      )}

      {contextMenu && (
        <FileContextMenu
          x={contextMenu.x}
          y={contextMenu.y}
          item={contextMenu.item}
          itemPinned={Boolean(contextMenu.item && pinnedPaths.some(item => item.path === contextMenu.item!.path))}
          currentPathPinned={currentPathPinned}
          onClose={() => setContextMenu(null)}
          onOpen={item => item.isDir ? navigateTo(item.path) : void openFile(item)}
          onDownload={item => downloadItems([item])}
          onRename={beginRename}
          onTogglePin={item => togglePin(item.path, item.isDir ? 'directory' : 'file')}
          onCopyPath={copyPath}
          onCopySelectedPaths={contextMenu.item || selectedItems.length > 0 ? () => copySelectedPaths(contextTargets) : undefined}
          onCopyRelativePath={copyRelativePath}
          onOpenParent={openParentFolder}
          onDelete={() => requestDelete(contextTargets)}
          onNewFile={() => startCreate('file')}
          onNewFolder={() => startCreate('folder')}
          onUpload={() => uploadInputRef.current?.click()}
          onRefresh={refreshCurrentPath}
          onCopyCurrentPath={() => copyPath(currentPath)}
          onToggleCurrentPathPin={() => togglePin(currentPath, 'directory')}
        />
      )}

      {deleteTargets && (
        <div className="fb-dialog-overlay">
          <div className="fb-dialog fb-dialog-danger">
            <div className="fb-dialog-header">
              <h3>Delete {deleteTargets.length === 1 ? deleteTargets[0].name : `${deleteTargets.length} items`}</h3>
              <button className="fb-dialog-close" type="button" onClick={() => setDeleteTargets(null)}>x</button>
            </div>
            <div className="fb-dialog-body">
              <p className="fb-dialog-message">
                This permanently removes the selected {deleteTargets.length === 1 ? 'item' : 'items'} from disk.
              </p>
            </div>
            <div className="fb-dialog-footer">
              <button className="fb-dialog-btn fb-dialog-btn-cancel" type="button" onClick={() => setDeleteTargets(null)}>Cancel</button>
              <button className="fb-dialog-btn fb-dialog-btn-danger" type="button" onClick={() => void confirmDelete()}>Delete</button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

export default FilesView
