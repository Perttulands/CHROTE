import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { CSSProperties, ChangeEvent, DragEvent, MouseEvent as ReactMouseEvent, PointerEvent as ReactPointerEvent } from 'react'
import { toDisplayPath } from './types'
import type {
  ContextMenuState,
  CreateIntent,
  CreateKind,
  FileItem,
  FilesViewProps,
  SortDir,
  SortKey,
  TabContextMenuState,
  ViewMode,
} from './types'
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
import { CONFIRM_WINDOW_MS } from '../confirmInPlace'
import { copyTextToClipboard } from '../../utils/clipboard'
import {
  MAX_TEXT_PREVIEW_BYTES,
  getFileBaseName as getBaseName,
  getPreviewKind,
  makeFileItemFromPath,
  normalizeFilePath as normalizePath,
} from '../FileViewer'
import type { FileViewState } from '../workspaceFilesState'
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
import {
  RECENT_STORAGE_KEY,
  readSavedGroupsCollapsed,
  readSavedPaths,
  writeSavedGroupsCollapsed,
  writeSavedPaths,
  type SavedGroupsCollapsed,
  type SavedPathGroup,
} from './savedPaths'

export function useFilesView({ navigateRequest = null, onSendPath, sendTargetLabel = null }: FilesViewProps) {
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

  // Which dirty buffer was asked to close, and when, so a repeat discards it.
  const discardArmed = useRef<{ path: string | null; at: number }>({ path: null, at: 0 })

  const currentPathPinned = pinnedPaths.some(item => item.path === currentPath)
  const workbenchStyle = { '--fb-explorer-width': `${explorerWidth}px` } as CSSProperties
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

  // Closing one tab has an unambiguous target, so a dirty buffer confirms in
  // place: the same close, pressed again within three seconds, discards. Bulk
  // closes cover many buffers at once and keep blocking instead, so the
  // operator resolves them one by one.
  const closeOpenFile = useCallback((path: string) => {
    const target = findBuffer(openFilesStateRef.current, path)
    const armed = discardArmed.current.path === path && Date.now() - discardArmed.current.at < CONFIRM_WINDOW_MS
    if (target?.dirty && !armed) {
      discardArmed.current = { path, at: Date.now() }
      showError(`${target.name} has unsaved changes. Close it again to discard them.`)
      return
    }
    discardArmed.current = { path: null, at: 0 }

    setOpenFilesState(previous => {
      const next = closeBuffer(previous, path)
      if (next.files.length === 0) setContentMode('folder')
      return next
    })
  }, [showError])

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

  return {
    uploadInputRef, pathInputRef, items, treeRefreshToken, setTreeRefreshToken,
    expandedPaths, setExpandedPaths, treeScrollTop, setTreeScrollTop, currentPath, history, historyIndex,
    loading, error, selectedPaths, sortBy, sortDir, viewMode, setViewMode,
    contentMode, setContentMode, searchQuery, setSearchQuery, contextMenu, setContextMenu,
    tabContextMenu, setTabContextMenu, renamingPath, renameValue, setRenameValue,
    createIntent, setCreateIntent, deleteTargets, setDeleteTargets, toast, setToast,
    openFiles, activeFilePath, setActiveFilePath, fileViewStates, setFileViewStates,
    pinnedPaths, recentPaths, savedGroupsCollapsed, editingPath, setEditingPath,
    pathDraft, setPathDraft, setDraggingPaths, dropTargetPath, setDropTargetPath,
    operationLabel, currentPathPinned, workbenchStyle, setExplorerWidth,
    loadDirectory, navigateTo, refreshCurrentPath, startExplorerResize, updateOpenFile,
    openFile, openSavedPath, closeOpenFile, closeAllOpenFiles, closeOtherOpenFiles,
    activeFile, selectedItems, visibleItems, toggleSort, goBack, goForward, goUp,
    handlePathSubmit, handleItemClick, handleContextMenu, beginRename, cancelRename,
    submitRename, startCreate, cancelCreate, confirmCreate, requestDelete, confirmDelete,
    handleUploadInput, handleDropUpload, downloadItems, copyPath, copySelectedPaths,
    copyRelativePath, openParentFolder, togglePin, toggleSavedGroup, saveActiveFile,
    handleInternalDragStart, handleFolderDragOver, handleFolderDrop, onSendPath,
    sendTargetLabel,
  }
}
