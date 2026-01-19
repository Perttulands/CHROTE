import { useState, useEffect, useCallback, useMemo } from 'react'
import {
  FileItem,
  FileBrowserState,
  OperationState,
  ViewTab,
  ContextMenuState,
  toDisplayPath,
} from './types'
import {
  fetchDirectory,
  createFolder,
  renameItem,
  deleteItem,
  getDownloadUrl,
  getErrorMessage,
} from './fileService'
import {
  Breadcrumbs,
  ColumnHeader,
  FileRow,
  FileGridItem,
  ContextMenu,
  NewFolderDialog,
  DeleteDialog,
  UploadZone,
  InboxPanel,
  InfoPanel,
  ErrorToast,
} from './components'

function FilesView() {
  const [activeTab, setActiveTab] = useState<ViewTab>('browser')
  const [state, setState] = useState<FileBrowserState>({
    items: [],
    loading: true,
    error: null,
    currentPath: '/',
    selectedItems: new Set(),
    sortBy: 'name',
    sortDir: 'asc',
    viewMode: 'list',
    searchQuery: '',
  })
  const [operation, setOperation] = useState<OperationState>({
    type: null,
    loading: false,
    error: null,
  })
  const [toast, setToast] = useState<string | null>(null)
  const [contextMenu, setContextMenu] = useState<ContextMenuState | null>(null)
  const [renamingItem, setRenamingItem] = useState<string | null>(null)
  const [renameValue, setRenameValue] = useState('')
  const [showNewFolderDialog, setShowNewFolderDialog] = useState(false)
  const [deleteItemState, setDeleteItemState] = useState<FileItem | null>(null)
  const [navigationHistory, setNavigationHistory] = useState<string[]>(['/'])
  const [historyIndex, setHistoryIndex] = useState(0)

  // Show error toast
  const showError = useCallback((message: string) => {
    setToast(message)
  }, [])

  // Clear operation state
  const clearOperation = useCallback(() => {
    setOperation({ type: null, loading: false, error: null })
  }, [])

  // Load directory contents
  const loadDirectory = useCallback(async (path: string) => {
    setState(prev => ({ ...prev, loading: true, error: null }))

    try {
      const items = await fetchDirectory(path)
      setState(prev => ({
        ...prev,
        items,
        loading: false,
        currentPath: path,
        selectedItems: new Set(),
        error: null,
      }))
    } catch (error) {
      const message = getErrorMessage(error, 'fetch')
      setState(prev => ({
        ...prev,
        loading: false,
        error: message,
        items: [],
      }))
    }
  }, [])

  // Initial load
  useEffect(() => {
    loadDirectory('/')
  }, [loadDirectory])

  // Navigate to path
  const navigateTo = useCallback((path: string, addToHistory = true) => {
    if (addToHistory) {
      const newHistory = [...navigationHistory.slice(0, historyIndex + 1), path]
      setNavigationHistory(newHistory)
      setHistoryIndex(newHistory.length - 1)
    }
    loadDirectory(path)
  }, [loadDirectory, navigationHistory, historyIndex])

  // Go back
  const goBack = useCallback(() => {
    if (historyIndex > 0) {
      const newIndex = historyIndex - 1
      setHistoryIndex(newIndex)
      loadDirectory(navigationHistory[newIndex])
    }
  }, [historyIndex, navigationHistory, loadDirectory])

  // Go forward
  const goForward = useCallback(() => {
    if (historyIndex < navigationHistory.length - 1) {
      const newIndex = historyIndex + 1
      setHistoryIndex(newIndex)
      loadDirectory(navigationHistory[newIndex])
    }
  }, [historyIndex, navigationHistory, loadDirectory])

  // Go to parent directory
  const goUp = useCallback(() => {
    const parts = state.currentPath.split('/').filter(Boolean)
    if (parts.length > 0) {
      parts.pop()
      navigateTo('/' + parts.join('/'))
    }
  }, [state.currentPath, navigateTo])

  // Sort items
  const sortedItems = useMemo(() => {
    const items = [...state.items]

    const filtered = state.searchQuery
      ? items.filter(item => item.name.toLowerCase().includes(state.searchQuery.toLowerCase()))
      : items

    filtered.sort((a, b) => {
      if (a.isDir !== b.isDir) return a.isDir ? -1 : 1

      let comparison = 0
      switch (state.sortBy) {
        case 'name':
          comparison = a.name.localeCompare(b.name)
          break
        case 'size':
          comparison = a.size - b.size
          break
        case 'modified':
          comparison = new Date(a.modified).getTime() - new Date(b.modified).getTime()
          break
      }

      return state.sortDir === 'asc' ? comparison : -comparison
    })

    return filtered
  }, [state.items, state.sortBy, state.sortDir, state.searchQuery])

  // Handle sort
  const handleSort = (key: 'name' | 'size' | 'modified') => {
    setState(prev => ({
      ...prev,
      sortBy: key,
      sortDir: prev.sortBy === key && prev.sortDir === 'asc' ? 'desc' : 'asc',
    }))
  }

  // Handle selection
  const handleSelect = (item: FileItem, e: React.MouseEvent) => {
    e.preventDefault()

    if (e.ctrlKey || e.metaKey) {
      setState(prev => {
        const newSelected = new Set(prev.selectedItems)
        if (newSelected.has(item.name)) {
          newSelected.delete(item.name)
        } else {
          newSelected.add(item.name)
        }
        return { ...prev, selectedItems: newSelected }
      })
    } else if (e.shiftKey && state.selectedItems.size > 0) {
      const lastSelected = Array.from(state.selectedItems).pop()
      const lastIndex = sortedItems.findIndex(i => i.name === lastSelected)
      const currentIndex = sortedItems.findIndex(i => i.name === item.name)
      const start = Math.min(lastIndex, currentIndex)
      const end = Math.max(lastIndex, currentIndex)

      const rangeSelection = new Set(
        sortedItems.slice(start, end + 1).map(i => i.name)
      )
      setState(prev => ({ ...prev, selectedItems: rangeSelection }))
    } else {
      setState(prev => ({ ...prev, selectedItems: new Set([item.name]) }))
    }
  }

  // Handle context menu
  const handleContextMenu = (item: FileItem | null, e: React.MouseEvent) => {
    e.preventDefault()
    setContextMenu({ x: e.clientX, y: e.clientY, item })
    if (item && !state.selectedItems.has(item.name)) {
      setState(prev => ({ ...prev, selectedItems: new Set([item.name]) }))
    }
  }

  // Handle rename
  const startRename = (item: FileItem) => {
    setRenamingItem(item.name)
    setRenameValue(item.name)
    setContextMenu(null)
  }

  const submitRename = async () => {
    if (!renamingItem || !renameValue.trim() || renameValue === renamingItem) {
      cancelRename()
      return
    }

    const item = state.items.find(i => i.name === renamingItem)
    if (!item) return

    const newPath = state.currentPath === '/'
      ? `/${renameValue}`
      : `${state.currentPath}/${renameValue}`

    setOperation({ type: 'rename', loading: true, error: null, target: item.name })

    try {
      await renameItem(item.path, newPath)
      await loadDirectory(state.currentPath)
      clearOperation()
    } catch (error) {
      const message = getErrorMessage(error, 'rename')
      setOperation({ type: 'rename', loading: false, error: message, target: item.name })
      showError(message)
    } finally {
      cancelRename()
    }
  }

  const cancelRename = () => {
    setRenamingItem(null)
    setRenameValue('')
  }

  // Handle delete
  const handleDelete = async () => {
    if (!deleteItemState) return

    setOperation({ type: 'delete', loading: true, error: null, target: deleteItemState.name })

    try {
      await deleteItem(deleteItemState.path)
      await loadDirectory(state.currentPath)
      clearOperation()
      setDeleteItemState(null)
    } catch (error) {
      const message = getErrorMessage(error, 'delete')
      setOperation({ type: 'delete', loading: false, error: message, target: deleteItemState.name })
      showError(message)
    }
  }

  // Handle new folder
  const handleNewFolder = async (name: string) => {
    setOperation({ type: 'create', loading: true, error: null, target: name })

    try {
      await createFolder(state.currentPath, name)
      await loadDirectory(state.currentPath)
      clearOperation()
      setShowNewFolderDialog(false)
    } catch (error) {
      const message = getErrorMessage(error, 'create')
      setOperation({ type: 'create', loading: false, error: message, target: name })
    }
  }

  // Handle download
  const handleDownload = () => {
    if (contextMenu?.item && !contextMenu.item.isDir) {
      window.open(getDownloadUrl(contextMenu.item.path), '_blank')
    }
    setContextMenu(null)
  }

  // Handle copy path
  const handleCopyPath = () => {
    if (contextMenu?.item) {
      const displayPath = toDisplayPath(contextMenu.item.path)
      navigator.clipboard.writeText(displayPath)
    }
    setContextMenu(null)
  }

  // Keyboard shortcuts
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (activeTab !== 'browser') return
      if ((e.target as HTMLElement).tagName === 'INPUT' || (e.target as HTMLElement).tagName === 'TEXTAREA') return

      if (e.key === 'F5') {
        e.preventDefault()
        loadDirectory(state.currentPath)
      } else if (e.key === 'Backspace') {
        e.preventDefault()
        goUp()
      } else if (e.key === 'Delete') {
        const selectedItem = state.items.find(i => state.selectedItems.has(i.name))
        if (selectedItem) {
          setDeleteItemState(selectedItem)
        }
      } else if (e.key === 'F2') {
        const selectedItem = state.items.find(i => state.selectedItems.has(i.name))
        if (selectedItem) {
          startRename(selectedItem)
        }
      } else if (e.ctrlKey && e.key === 'a') {
        e.preventDefault()
        setState(prev => ({
          ...prev,
          selectedItems: new Set(prev.items.map(i => i.name)),
        }))
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [activeTab, state.currentPath, state.items, state.selectedItems, loadDirectory, goUp])

  // Check if at root
  const isAtRoot = state.currentPath === '/' || state.currentPath === ''

  // Check if operation is in progress
  const isOperationInProgress = operation.loading

  return (
    <div className="fb-container files-view">
      {/* Error Toast */}
      {toast && (
        <ErrorToast
          message={toast}
          onDismiss={() => setToast(null)}
        />
      )}

      {/* Header */}
      <div className="fb-header">
        <div className="fb-header-left">
          <h2 className="fb-title">Files</h2>
          <div className="fb-tabs">
            <button
              className={`fb-tab ${activeTab === 'browser' ? 'active' : ''}`}
              onClick={() => setActiveTab('browser')}
            >
              Browser
            </button>
            <button
              className={`fb-tab ${activeTab === 'info' ? 'active' : ''}`}
              onClick={() => setActiveTab('info')}
            >
              Info
            </button>
          </div>
        </div>
        <div className="fb-header-right">
          {activeTab === 'browser' && (
            <>
              <UploadZone
                currentPath={state.currentPath}
                onUploadComplete={() => loadDirectory(state.currentPath)}
                onError={showError}
              />
              <button
                className="fb-btn"
                onClick={() => setShowNewFolderDialog(true)}
                title="New Folder"
                disabled={isOperationInProgress}
              >
                +📁
              </button>
            </>
          )}
          <button
            className="fb-btn"
            onClick={() => loadDirectory(state.currentPath)}
            title="Refresh"
            disabled={state.loading}
          >
            ↻
          </button>
          <button
            className="fb-btn"
            onClick={() => window.open('/files/', '_blank')}
            title="Open in new tab"
          >
            ↗
          </button>
        </div>
      </div>

      {/* Content */}
      <div className="fb-content">
        {activeTab === 'browser' ? (
          <>
            {/* Inbox Panel */}
            <InboxPanel onError={showError} />

            {/* Toolbar */}
            <div className="fb-toolbar">
              <div className="fb-toolbar-nav">
                <button
                  className="fb-nav-btn"
                  onClick={goBack}
                  disabled={historyIndex === 0}
                  title="Back"
                >
                  ←
                </button>
                <button
                  className="fb-nav-btn"
                  onClick={goForward}
                  disabled={historyIndex >= navigationHistory.length - 1}
                  title="Forward"
                >
                  →
                </button>
                <button
                  className="fb-nav-btn"
                  onClick={goUp}
                  disabled={state.currentPath === '/'}
                  title="Up"
                >
                  ↑
                </button>
              </div>
              <Breadcrumbs path={state.currentPath} onNavigate={navigateTo} />
              <div className="fb-toolbar-actions">
                <input
                  type="text"
                  className="fb-search"
                  placeholder="Filter..."
                  value={state.searchQuery}
                  onChange={(e) => setState(prev => ({ ...prev, searchQuery: e.target.value }))}
                />
                <button
                  className={`fb-view-btn ${state.viewMode === 'list' ? 'active' : ''}`}
                  onClick={() => setState(prev => ({ ...prev, viewMode: 'list' }))}
                  title="List view"
                >
                  ≡
                </button>
                <button
                  className={`fb-view-btn ${state.viewMode === 'grid' ? 'active' : ''}`}
                  onClick={() => setState(prev => ({ ...prev, viewMode: 'grid' }))}
                  title="Grid view"
                >
                  ⊞
                </button>
              </div>
            </div>

            {/* File List */}
            <div
              className="fb-list-container"
              onContextMenu={(e) => {
                if ((e.target as HTMLElement).closest('.fb-row, .fb-grid-item')) return
                handleContextMenu(null, e)
              }}
            >
              {state.loading ? (
                <div className="fb-loading">
                  <span className="fb-spinner" />
                  Loading...
                </div>
              ) : state.error ? (
                <div className="fb-error">
                  <span className="fb-error-icon">⚠</span>
                  <span className="fb-error-message">{state.error}</span>
                  <button className="fb-retry-btn" onClick={() => loadDirectory(state.currentPath)}>
                    Retry
                  </button>
                </div>
              ) : sortedItems.length === 0 ? (
                <div className="fb-empty">
                  <span className="fb-empty-icon">📂</span>
                  {state.searchQuery ? 'No matches found' : 'This folder is empty'}
                </div>
              ) : state.viewMode === 'list' ? (
                <div className="fb-list" role="grid">
                  {/* Header */}
                  <div className="fb-list-header" role="row">
                    <ColumnHeader
                      label="Name"
                      sortKey="name"
                      currentSort={state.sortBy}
                      currentDir={state.sortDir}
                      onSort={handleSort}
                      className="fb-cell-name"
                    />
                    <ColumnHeader
                      label="Size"
                      sortKey="size"
                      currentSort={state.sortBy}
                      currentDir={state.sortDir}
                      onSort={handleSort}
                      className="fb-cell-size"
                    />
                    <ColumnHeader
                      label="Modified"
                      sortKey="modified"
                      currentSort={state.sortBy}
                      currentDir={state.sortDir}
                      onSort={handleSort}
                      className="fb-cell-modified"
                    />
                  </div>
                  {/* Rows */}
                  <div className="fb-list-body">
                    {sortedItems.map(item => (
                      <FileRow
                        key={item.name}
                        item={item}
                        isSelected={state.selectedItems.has(item.name)}
                        onSelect={(e) => handleSelect(item, e)}
                        onNavigate={() => navigateTo(item.path)}
                        onContextMenu={(e) => handleContextMenu(item, e)}
                        onRename={() => startRename(item)}
                        isRenaming={renamingItem === item.name}
                        renameValue={renameValue}
                        setRenameValue={setRenameValue}
                        onRenameSubmit={submitRename}
                        onRenameCancel={cancelRename}
                        isAtRoot={isAtRoot}
                        disabled={isOperationInProgress}
                      />
                    ))}
                  </div>
                </div>
              ) : (
                <div className="fb-grid" role="grid">
                  {sortedItems.map(item => (
                    <FileGridItem
                      key={item.name}
                      item={item}
                      isSelected={state.selectedItems.has(item.name)}
                      onSelect={(e) => handleSelect(item, e)}
                      onNavigate={() => navigateTo(item.path)}
                      onContextMenu={(e) => handleContextMenu(item, e)}
                      isAtRoot={isAtRoot}
                      disabled={isOperationInProgress}
                    />
                  ))}
                </div>
              )}
            </div>

            {/* Status Bar */}
            <div className="fb-statusbar">
              <span>{sortedItems.length} items</span>
              {state.selectedItems.size > 0 && (
                <span>{state.selectedItems.size} selected</span>
              )}
              {isOperationInProgress && (
                <span className="fb-statusbar-operation">
                  {operation.type === 'rename' && 'Renaming...'}
                  {operation.type === 'delete' && 'Deleting...'}
                  {operation.type === 'create' && 'Creating...'}
                  {operation.type === 'upload' && 'Uploading...'}
                </span>
              )}
            </div>
          </>
        ) : (
          <InfoPanel />
        )}
      </div>

      {/* Context Menu */}
      {contextMenu && (
        <ContextMenu
          x={contextMenu.x}
          y={contextMenu.y}
          item={contextMenu.item}
          onClose={() => setContextMenu(null)}
          onDownload={handleDownload}
          onRename={() => contextMenu.item && startRename(contextMenu.item)}
          onDelete={() => contextMenu.item && setDeleteItemState(contextMenu.item)}
          onCopyPath={handleCopyPath}
          onNewFolder={() => {
            setContextMenu(null)
            setShowNewFolderDialog(true)
          }}
        />
      )}

      {/* Dialogs */}
      {showNewFolderDialog && (
        <NewFolderDialog
          onClose={() => {
            setShowNewFolderDialog(false)
            clearOperation()
          }}
          onCreate={handleNewFolder}
          loading={operation.type === 'create' && operation.loading}
          error={operation.type === 'create' ? operation.error : null}
        />
      )}

      {deleteItemState && (
        <DeleteDialog
          item={deleteItemState}
          onClose={() => {
            setDeleteItemState(null)
            clearOperation()
          }}
          onConfirm={handleDelete}
          loading={operation.type === 'delete' && operation.loading}
          error={operation.type === 'delete' ? operation.error : null}
        />
      )}
    </div>
  )
}

export default FilesView
