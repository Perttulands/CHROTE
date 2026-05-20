import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { ChangeEvent, DragEvent, KeyboardEvent as ReactKeyboardEvent, MouseEvent as ReactMouseEvent } from 'react'
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

type SortKey = 'name' | 'size' | 'modified'
type SortDir = 'asc' | 'desc'
type ViewMode = 'list' | 'grid'
type PreviewKind = 'text' | 'image' | 'audio' | 'video' | 'pdf' | 'download'
type SavedKind = 'file' | 'directory'
type CreateKind = 'file' | 'folder'

interface SavedPath {
  path: string
  kind: SavedKind
}

interface OpenFile {
  path: string
  name: string
  size: number
  type: string
  kind: PreviewKind
  content: string
  dirty: boolean
  loading: boolean
  error: string | null
}

interface ContextMenuState {
  x: number
  y: number
  item: FileItem | null
}

interface CreateIntent {
  kind: CreateKind
  parentPath: string
  name: string
}

const PINNED_STORAGE_KEY = 'chrote.files.pinnedPaths'
const RECENT_STORAGE_KEY = 'chrote.files.recentPaths'
const MAX_TEXT_PREVIEW_BYTES = 1024 * 1024

const TEXT_EXTENSIONS = new Set([
  'bash',
  'c',
  'conf',
  'cpp',
  'css',
  'csv',
  'dockerfile',
  'env',
  'go',
  'h',
  'html',
  'ini',
  'java',
  'js',
  'json',
  'jsx',
  'log',
  'md',
  'py',
  'rb',
  'rs',
  'sh',
  'sql',
  'toml',
  'ts',
  'tsx',
  'txt',
  'xml',
  'yaml',
  'yml',
])

const IMAGE_EXTENSIONS = new Set(['bmp', 'gif', 'jpeg', 'jpg', 'png', 'svg', 'webp'])
const AUDIO_EXTENSIONS = new Set(['flac', 'm4a', 'mp3', 'ogg', 'wav'])
const VIDEO_EXTENSIONS = new Set(['avi', 'mkv', 'mov', 'mp4', 'webm'])

function normalizePath(path: string): string {
  const trimmed = path.trim()
  if (!trimmed || trimmed === '.') return '/'
  const withRoot = trimmed.startsWith('/') ? trimmed : `/${trimmed}`
  const compact = withRoot.replace(/\/+/g, '/')
  return compact.length > 1 ? compact.replace(/\/$/, '') : compact
}

function joinPath(parent: string, name: string): string {
  const cleanParent = normalizePath(parent)
  return cleanParent === '/' ? `/${name}` : `${cleanParent}/${name}`
}

function getParentPath(path: string): string {
  const normalized = normalizePath(path)
  if (normalized === '/') return '/'
  const parts = normalized.split('/').filter(Boolean)
  parts.pop()
  return parts.length === 0 ? '/' : `/${parts.join('/')}`
}

function getBaseName(path: string): string {
  const parts = normalizePath(path).split('/').filter(Boolean)
  return parts[parts.length - 1] || '/'
}

function getExtension(name: string): string {
  const lower = name.toLowerCase()
  if (lower === 'dockerfile' || lower.startsWith('dockerfile.')) return 'dockerfile'
  if (lower.startsWith('.env')) return 'env'
  const parts = lower.split('.')
  return parts.length > 1 ? parts.pop() || '' : ''
}

function getPreviewKind(item: FileItem): PreviewKind {
  const ext = getExtension(item.name)
  const lower = item.name.toLowerCase()

  if (TEXT_EXTENSIONS.has(ext) || lower === 'readme' || lower === 'license' || lower === 'makefile') {
    return 'text'
  }
  if (IMAGE_EXTENSIONS.has(ext)) return 'image'
  if (AUDIO_EXTENSIONS.has(ext)) return 'audio'
  if (VIDEO_EXTENSIONS.has(ext)) return 'video'
  if (ext === 'pdf') return 'pdf'
  return 'download'
}

function getFileBadge(item: FileItem): string {
  if (item.isDir) return 'DIR'
  const ext = getExtension(item.name)
  if (!ext) return 'TXT'
  return ext.slice(0, 4).toUpperCase()
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

function makeFileItemFromPath(path: string): FileItem {
  const name = getBaseName(path)
  return {
    name,
    path,
    isDir: false,
    size: 0,
    modified: new Date().toISOString(),
    type: getExtension(name),
  }
}

function FilesView() {
  const uploadInputRef = useRef<HTMLInputElement | null>(null)
  const pathInputRef = useRef<HTMLInputElement | null>(null)
  const [items, setItems] = useState<FileItem[]>([])
  const [treeItems, setTreeItems] = useState<Record<string, FileItem[]>>({})
  const [treeLoading, setTreeLoading] = useState<Set<string>>(new Set())
  const [expandedPaths, setExpandedPaths] = useState<Set<string>>(new Set(['/']))
  const [currentPath, setCurrentPath] = useState('/')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [selectedPaths, setSelectedPaths] = useState<Set<string>>(new Set())
  const [sortBy, setSortBy] = useState<SortKey>('name')
  const [sortDir, setSortDir] = useState<SortDir>('asc')
  const [viewMode, setViewMode] = useState<ViewMode>('list')
  const [searchQuery, setSearchQuery] = useState('')
  const [history, setHistory] = useState<string[]>(['/'])
  const [historyIndex, setHistoryIndex] = useState(0)
  const [contextMenu, setContextMenu] = useState<ContextMenuState | null>(null)
  const [renamingPath, setRenamingPath] = useState<string | null>(null)
  const [renameValue, setRenameValue] = useState('')
  const [createIntent, setCreateIntent] = useState<CreateIntent | null>(null)
  const [deleteTargets, setDeleteTargets] = useState<FileItem[] | null>(null)
  const [toast, setToast] = useState<string | null>(null)
  const [openFiles, setOpenFiles] = useState<OpenFile[]>([])
  const [activeFilePath, setActiveFilePath] = useState<string | null>(null)
  const [pinnedPaths, setPinnedPaths] = useState<SavedPath[]>(() => readSavedPaths(PINNED_STORAGE_KEY))
  const [recentPaths, setRecentPaths] = useState<SavedPath[]>(() => readSavedPaths(RECENT_STORAGE_KEY))
  const [editingPath, setEditingPath] = useState(false)
  const [pathDraft, setPathDraft] = useState('/')
  const [draggingPaths, setDraggingPaths] = useState<string[]>([])
  const [dropTargetPath, setDropTargetPath] = useState<string | null>(null)
  const [operationLabel, setOperationLabel] = useState<string | null>(null)

  const isRootListing = currentPath === '/'

  const showError = useCallback((message: string) => {
    setToast(message)
  }, [])

  const loadTree = useCallback(async (path: string) => {
    const normalized = normalizePath(path)
    setTreeLoading(prev => new Set(prev).add(normalized))
    try {
      const nextItems = await fetchDirectory(normalized)
      const directories = nextItems
        .filter(item => item.isDir)
        .sort((a, b) => a.name.localeCompare(b.name))
      setTreeItems(prev => ({ ...prev, [normalized]: directories }))
    } catch {
      setTreeItems(prev => (normalized in prev ? prev : { ...prev, [normalized]: [] }))
    } finally {
      setTreeLoading(prev => {
        const next = new Set(prev)
        next.delete(normalized)
        return next
      })
    }
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
      setExpandedPaths(prev => new Set(prev).add(normalized).add('/'))
      void loadTree(normalized)
    } catch (loadError) {
      setItems([])
      setError(getErrorMessage(loadError, 'fetch'))
    } finally {
      setLoading(false)
    }
  }, [loadTree])

  const navigateTo = useCallback((path: string, addToHistory = true) => {
    const normalized = normalizePath(path)
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
    void loadTree(currentPath)
    void loadTree('/')
  }, [currentPath, loadDirectory, loadTree])

  useEffect(() => {
    void loadDirectory('/')
  }, [loadDirectory])

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

  useEffect(() => {
    if (!contextMenu) return

    const handlePointerDown = (event: MouseEvent) => {
      const target = event.target as Element
      if (!target.closest('.fb-context-menu')) {
        setContextMenu(null)
      }
    }
    const handleKeyDown = (event: globalThis.KeyboardEvent) => {
      if (event.key === 'Escape') setContextMenu(null)
    }

    document.addEventListener('mousedown', handlePointerDown)
    document.addEventListener('keydown', handleKeyDown)
    return () => {
      document.removeEventListener('mousedown', handlePointerDown)
      document.removeEventListener('keydown', handleKeyDown)
    }
  }, [contextMenu])

  const updateOpenFile = useCallback((path: string, patch: Partial<OpenFile>) => {
    setOpenFiles(prev => prev.map(file => (file.path === path ? { ...file, ...patch } : file)))
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
    setActiveFilePath(item.path)
    rememberRecent(item.path)

    setOpenFiles(prev => {
      if (prev.some(file => file.path === item.path)) return prev
      return [
        ...prev,
        {
          path: item.path,
          name: item.name,
          size: item.size,
          type: item.type,
          kind,
          content: '',
          dirty: false,
          loading: kind === 'text' && item.size <= MAX_TEXT_PREVIEW_BYTES,
          error: kind === 'text' && item.size > MAX_TEXT_PREVIEW_BYTES ? 'File is too large for inline editing' : null,
        },
      ]
    })

    if (kind === 'text' && item.size <= MAX_TEXT_PREVIEW_BYTES) {
      try {
        const content = await readTextFile(item.path)
        updateOpenFile(item.path, { content, loading: false, error: null })
      } catch (readError) {
        updateOpenFile(item.path, {
          loading: false,
          error: getErrorMessage(readError, 'read'),
        })
      }
    }
  }, [navigateTo, rememberRecent, updateOpenFile])

  const openSavedPath = useCallback((saved: SavedPath) => {
    if (saved.kind === 'directory') {
      navigateTo(saved.path)
      return
    }
    void openFile(makeFileItemFromPath(saved.path))
  }, [navigateTo, openFile])

  const closeOpenFile = useCallback((path: string) => {
    setOpenFiles(prev => {
      const index = prev.findIndex(file => file.path === path)
      const next = prev.filter(file => file.path !== path)
      if (activeFilePath === path) {
        const fallback = next[index] || next[index - 1] || next[0] || null
        setActiveFilePath(fallback?.path || null)
      }
      return next
    })
  }, [activeFilePath])

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
    if (isRootListing && item.isDir) return
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

    setOperationLabel('Renaming')
    try {
      const safeName = sanitizeFilename(renameValue)
      const destination = joinPath(getParentPath(item.path), safeName)
      await renameItem(item.path, destination)
      setOpenFiles(prev => prev.map(file => {
        if (file.path !== item.path) return file
        return { ...file, path: destination, name: safeName }
      }))
      if (activeFilePath === item.path) setActiveFilePath(destination)
      await loadDirectory(currentPath)
      await loadTree(currentPath)
    } catch (renameError) {
      showError(getErrorMessage(renameError, 'rename'))
    } finally {
      setOperationLabel(null)
      cancelRename()
    }
  }

  const startCreate = (kind: CreateKind, parentPath = currentPath) => {
    if (parentPath === '/') {
      showError('Choose a workspace folder before creating files')
      return
    }
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
      await loadTree(createIntent.parentPath)
      setCreateIntent(null)
    } catch (createError) {
      showError(getErrorMessage(createError, 'create'))
    } finally {
      setOperationLabel(null)
    }
  }

  const requestDelete = (targets: FileItem[]) => {
    const mutableTargets = targets.filter(target => !(isRootListing && target.isDir))
    if (mutableTargets.length === 0) return
    setDeleteTargets(mutableTargets)
    setContextMenu(null)
  }

  const confirmDelete = async () => {
    if (!deleteTargets) return
    setOperationLabel('Deleting')
    try {
      for (const target of deleteTargets) {
        await deleteItem(target.path)
      }
      const deletedPaths = new Set(deleteTargets.map(item => item.path))
      setOpenFiles(prev => prev.filter(file => !Array.from(deletedPaths).some(path => file.path === path || file.path.startsWith(`${path}/`))))
      if (activeFilePath && Array.from(deletedPaths).some(path => activeFilePath === path || activeFilePath.startsWith(`${path}/`))) {
        setActiveFilePath(null)
      }
      await loadDirectory(currentPath)
      await loadTree(currentPath)
      setDeleteTargets(null)
    } catch (deleteError) {
      showError(getErrorMessage(deleteError, 'delete'))
    } finally {
      setOperationLabel(null)
    }
  }

  const handleUpload = async (fileList: FileList | File[]) => {
    if (currentPath === '/') {
      showError('Choose a workspace folder before uploading')
      return
    }
    const files = Array.isArray(fileList) ? fileList : Array.from(fileList)
    if (files.length === 0) return

    setOperationLabel(`Uploading ${files.length} item${files.length === 1 ? '' : 's'}`)
    try {
      await uploadFiles(currentPath, files)
      await loadDirectory(currentPath)
      await loadTree(currentPath)
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
    void navigator.clipboard?.writeText(toDisplayPath(path))
    setContextMenu(null)
  }

  const togglePin = (path: string, kind: SavedKind) => {
    setPinnedPaths(prev => {
      const exists = prev.some(item => item.path === path)
      const next = exists
        ? prev.filter(item => item.path !== path)
        : [{ path, kind }, ...prev].slice(0, 20)
      writeSavedPaths(PINNED_STORAGE_KEY, next)
      return next
    })
    setContextMenu(null)
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

    setOperationLabel('Moving')
    try {
      for (const sourcePath of draggingPaths) {
        const destination = joinPath(target.path, getBaseName(sourcePath))
        await renameItem(sourcePath, destination)
      }
      await loadDirectory(currentPath)
      await loadTree(currentPath)
      await loadTree(target.path)
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

  const renderTreeNode = (item: FileItem, level: number) => {
    const expanded = expandedPaths.has(item.path)
    const children = treeItems[item.path] || []
    const active = currentPath === item.path
    const loadingChildren = treeLoading.has(item.path)

    return (
      <div className="fb-tree-node" key={item.path}>
        <div
          className={`fb-tree-row ${active ? 'active' : ''}`}
          style={{ paddingLeft: `${8 + level * 14}px` }}
          onClick={() => navigateTo(item.path)}
          onContextMenu={(event) => handleContextMenu(event, item)}
        >
          <button
            className="fb-tree-toggle"
            type="button"
            title={expanded ? 'Collapse' : 'Expand'}
            onClick={(event) => {
              event.stopPropagation()
              setExpandedPaths(prev => {
                const next = new Set(prev)
                if (next.has(item.path)) next.delete(item.path)
                else next.add(item.path)
                return next
              })
              if (!expanded && !(item.path in treeItems)) {
                void loadTree(item.path)
              }
            }}
          >
            {expanded ? 'v' : '>'}
          </button>
          <span className="fb-tree-glyph">DIR</span>
          <span className="fb-tree-name" title={item.path}>{item.name}</span>
        </div>
        {expanded && (
          <div className="fb-tree-children">
            {loadingChildren ? (
              <div className="fb-tree-loading" style={{ paddingLeft: `${28 + level * 14}px` }}>Loading...</div>
            ) : (
              children.map(child => renderTreeNode(child, level + 1))
            )}
          </div>
        )}
      </div>
    )
  }

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
        draggable={!isRootListing}
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
        draggable={!isRootListing}
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
              className="fb-tab"
              type="button"
              title="Pin current folder"
              onClick={() => togglePin(currentPath, 'directory')}
            >
              Pin
            </button>
          </div>
        </div>
        <div className="fb-header-right">
          <button className="fb-btn" type="button" title="New File" disabled={isRootListing} onClick={() => startCreate('file')}>+ File</button>
          <button className="fb-btn" type="button" title="New Folder" disabled={isRootListing} onClick={() => startCreate('folder')}>+ Folder</button>
          <button className="fb-btn" type="button" title="Upload" disabled={isRootListing} onClick={() => uploadInputRef.current?.click()}>Upload</button>
          <button className="fb-btn" type="button" title="Refresh" disabled={loading} onClick={refreshCurrentPath}>Refresh</button>
        </div>
      </div>

      <div className="fb-workbench">
        <aside className="fb-sidebar" aria-label="Explorer">
          <div className="fb-sidebar-header">
            <span>Explorer</span>
            <button className="fb-sidebar-action" type="button" title="Refresh tree" onClick={() => loadTree('/')}>Refresh</button>
          </div>

          {pinnedPaths.length > 0 && (
            <section className="fb-sidebar-section">
              <div className="fb-section-title">Pinned</div>
              <div className="fb-saved-list">
                {pinnedPaths.map(item => renderSavedPath(item, 'fb-saved-item'))}
              </div>
            </section>
          )}

          {recentPaths.length > 0 && (
            <section className="fb-sidebar-section">
              <div className="fb-section-title">Recent</div>
              <div className="fb-saved-list">
                {recentPaths.map(item => renderSavedPath(item, 'fb-saved-item'))}
              </div>
            </section>
          )}

          <section className="fb-sidebar-section fb-tree-section">
            <div className="fb-section-title">Workspace</div>
            <div className="fb-tree">
              {treeLoading.has('/') && !treeItems['/'] ? (
                <div className="fb-tree-loading">Loading...</div>
              ) : (
                (treeItems['/'] || []).map(item => renderTreeNode(item, 0))
              )}
            </div>
          </section>
        </aside>

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
              <input
                className="fb-search"
                type="search"
                placeholder="Filter"
                value={searchQuery}
                onChange={(event) => setSearchQuery(event.target.value)}
              />
              <button className={`fb-view-btn ${viewMode === 'list' ? 'active' : ''}`} type="button" title="List view" onClick={() => setViewMode('list')}>List</button>
              <button className={`fb-view-btn ${viewMode === 'grid' ? 'active' : ''}`} type="button" title="Grid view" onClick={() => setViewMode('grid')}>Grid</button>
            </div>
          </div>

          <div
            className="fb-content"
            onDragOver={(event) => event.preventDefault()}
            onDrop={handleDropUpload}
            onContextMenu={(event) => {
              if (!(event.target as HTMLElement).closest('.fb-row, .fb-grid-item')) {
                handleContextMenu(event, null)
              }
            }}
          >
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
                {isRootListing && <span>Open a workspace folder to create or upload files</span>}
                {operationLabel && <span className="fb-statusbar-operation">{operationLabel}...</span>}
              </div>
            </section>

            <aside className="fb-editor-pane">
              <div className="fb-editor-tabs">
                {openFiles.length === 0 ? (
                  <span className="fb-editor-placeholder-tab">Preview</span>
                ) : (
                  openFiles.map(file => (
                    <button
                      key={file.path}
                      className={`fb-editor-tab ${file.path === activeFilePath ? 'active' : ''}`}
                      type="button"
                      title={file.path}
                      onClick={() => setActiveFilePath(file.path)}
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
                  ))
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
                    </div>
                  </div>

                  {activeFile.loading ? (
                    <div className="fb-editor-empty">Loading file...</div>
                  ) : activeFile.error ? (
                    <div className="fb-editor-empty">{activeFile.error}</div>
                  ) : activeFile.kind === 'text' ? (
                    <textarea
                      className="fb-editor-textarea"
                      value={activeFile.content}
                      spellCheck={false}
                      onChange={(event) => updateOpenFile(activeFile.path, { content: event.target.value, dirty: true })}
                    />
                  ) : activeFile.kind === 'image' ? (
                    <div className="fb-media-preview"><img src={getDownloadUrl(activeFile.path)} alt={activeFile.name} /></div>
                  ) : activeFile.kind === 'audio' ? (
                    <div className="fb-media-preview"><audio src={getDownloadUrl(activeFile.path)} controls /></div>
                  ) : activeFile.kind === 'video' ? (
                    <div className="fb-media-preview"><video src={getDownloadUrl(activeFile.path)} controls /></div>
                  ) : activeFile.kind === 'pdf' ? (
                    <iframe className="fb-pdf-preview" src={getDownloadUrl(activeFile.path)} title={activeFile.name} />
                  ) : (
                    <div className="fb-editor-empty">
                      <p>No inline preview is available for this file type.</p>
                      <button className="fb-btn" type="button" onClick={() => downloadItems([makeFileItemFromPath(activeFile.path)])}>Download</button>
                    </div>
                  )}
                </div>
              ) : (
                <div className="fb-editor-empty">
                  <strong>No file selected</strong>
                  <span>Select a file to preview or edit it here.</span>
                </div>
              )}
            </aside>
          </div>
        </main>
      </div>

      {contextMenu && (
        <div
          className="fb-context-menu"
          style={{ left: contextMenu.x, top: contextMenu.y }}
        >
          {contextMenu.item?.isDir && (
            <button className="fb-context-item" type="button" onClick={() => navigateTo(contextMenu.item!.path)}>Open Folder</button>
          )}
          {contextMenu.item && !contextMenu.item.isDir && (
            <>
              <button className="fb-context-item" type="button" onClick={() => void openFile(contextMenu.item!)}>Open</button>
              <button className="fb-context-item" type="button" onClick={() => downloadItems([contextMenu.item!])}>Download</button>
            </>
          )}
          {!contextMenu.item && (
            <>
              <button className="fb-context-item" type="button" disabled={isRootListing} onClick={() => startCreate('file')}>New File</button>
              <button className="fb-context-item" type="button" disabled={isRootListing} onClick={() => startCreate('folder')}>New Folder</button>
              <button className="fb-context-item" type="button" disabled={isRootListing} onClick={() => uploadInputRef.current?.click()}>Upload</button>
            </>
          )}
          {contextMenu.item && !(isRootListing && contextMenu.item.isDir) && (
            <>
              <div className="fb-context-divider" />
              <button className="fb-context-item" type="button" onClick={() => beginRename(contextMenu.item!)}>Rename</button>
              <button
                className="fb-context-item"
                type="button"
                onClick={() => togglePin(contextMenu.item!.path, contextMenu.item!.isDir ? 'directory' : 'file')}
              >
                {pinnedPaths.some(item => item.path === contextMenu.item!.path) ? 'Unpin' : 'Pin'}
              </button>
              <button className="fb-context-item" type="button" onClick={() => copyPath(contextMenu.item!.path)}>Copy Path</button>
              <div className="fb-context-divider" />
              <button className="fb-context-item fb-context-danger" type="button" onClick={() => requestDelete(contextTargets)}>Delete</button>
            </>
          )}
        </div>
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
