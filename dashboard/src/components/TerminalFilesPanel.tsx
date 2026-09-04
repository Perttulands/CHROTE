import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { ChangeEvent, CSSProperties, FormEvent, MouseEvent as ReactMouseEvent } from 'react'
import { ArrowUp, Pin, PinOff, X } from 'lucide-react'
import { useSession } from '../context/SessionContext'
import { useStatus } from '../context/StatusContext'
import { useResizableWidth } from '../hooks/useResizableWidth'
import { getSessionKey, type WorkspaceId } from '../types'
import { copyAndAnnounce } from '../utils/clipboard'
import FilePanelViewer from './FilePanelViewer'
import FileTree from './FileTree'
import { FileContextMenu } from './FileContextMenu'
import { normalizeFilePath } from './FileViewer'
import PanelPath from './PanelPath'
import {
  createFile,
  createFolder,
  deleteItem,
  fetchDirectory,
  findFiles,
  getDownloadUrl,
  getErrorMessage,
  renameItem,
  sanitizeFilename,
  uploadFiles,
  type FileMatch,
} from './FilesView/fileService'
import { getParentPath, joinFilePath, pathRelativeTo } from './FilesView/pathActions'
import { usePinnedPaths } from './FilesView/pinnedPaths'
import type { FileItem } from './FilesView/types'
import {
  readWorkspaceFilesState,
  writeWorkspaceFilesState,
  type WorkspaceFilesState,
} from './workspaceFilesState'

/** How long the panel waits after a keystroke before it asks the server. */
const FIND_DEBOUNCE_MS = 140

interface TerminalFilesPanelProps {
  workspaceId: WorkspaceId
  collapsed: boolean
  width: number
  pinned: boolean
  canPin: boolean
  panelId: string
  onTogglePin: () => void
  onClose: () => void
  onWidthChange: (width: number) => void
  onOpenInFiles: (path: string) => void
  navigateRequest?: { path: string; requestId: number } | null
  onNavigateRequestHandled?: (requestId: number) => void
}

interface ContextMenuState {
  x: number
  y: number
  item: FileItem | null
}

interface NameDialogState {
  kind: 'file' | 'folder' | 'rename'
  item: FileItem | null
  value: string
}

/**
 * Files, beside the terminal.
 *
 * The panel is search-first: the header is a field, typing it finds a file by
 * name anywhere in the configured roots, and Enter opens the first hit. That is
 * how the operator actually reaches a file — he knows its name, not the six
 * directories above it. An empty field leaves the tree in place for the times
 * he is looking rather than fetching.
 *
 * Opening a file replaces the tree with the viewer instead of floating a window
 * over the terminals: the panel exists to be read next to the work, and a
 * window that covers the work is worse than no panel at all.
 */
function TerminalFilesPanel({
  workspaceId,
  collapsed,
  width,
  pinned,
  canPin,
  panelId,
  onTogglePin,
  onClose,
  onWidthChange,
  onOpenInFiles,
  navigateRequest,
  onNavigateRequestHandled,
}: TerminalFilesPanelProps) {
  const { workspaces, focusedWindowKey, sessions, openSendToSession } = useSession()
  const { announce } = useStatus()
  const uploadInputRef = useRef<HTMLInputElement | null>(null)
  const panelRef = useRef<HTMLElement>(null)
  const [filesState, setFilesState] = useState<WorkspaceFilesState>(() => readWorkspaceFilesState(workspaceId))
  const [query, setQuery] = useState('')
  const [matches, setMatches] = useState<FileMatch[]>([])
  const [truncated, setTruncated] = useState(false)
  const [finding, setFinding] = useState(false)
  const [refreshToken, setRefreshToken] = useState(0)
  const [contextMenu, setContextMenu] = useState<ContextMenuState | null>(null)
  const [nameDialog, setNameDialog] = useState<NameDialogState | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<FileItem | null>(null)
  const [pinnedPaths, togglePinnedPath] = usePinnedPaths()
  const workspace = workspaces[workspaceId]

  const updateFilesState = useCallback((update: (previous: WorkspaceFilesState) => WorkspaceFilesState) => {
    setFilesState(previous => {
      const next = update(previous)
      writeWorkspaceFilesState(workspaceId, next)
      return next
    })
  }, [workspaceId])

  const focusedWindow = useMemo(() => {
    const focused = workspace.windows.find(window => focusedWindowKey === `${workspaceId}-${window.id}`)
    return focused || workspace.windows.slice(0, workspace.windowCount).find(window => window.activeSession)
  }, [focusedWindowKey, workspace, workspaceId])
  const sendTarget = focusedWindow?.activeSession ?? null
  const sessionCwd = useMemo(() => {
    if (!sendTarget) return null
    return sessions.find(entry => getSessionKey(entry.name, entry.unixUser) === sendTarget)?.cwd || null
  }, [sendTarget, sessions])

  const navigateTo = useCallback((path: string) => {
    const normalized = normalizeFilePath(path)
    updateFilesState(previous => ({
      ...previous,
      currentPath: normalized,
      openPath: null,
      expandedPaths: Array.from(new Set([...previous.expandedPaths, normalized])),
      selectedPath: null,
      treeScrollTop: 0,
    }))
  }, [updateFilesState])

  const openPath = useCallback((path: string) => {
    const normalized = normalizeFilePath(path)
    updateFilesState(previous => ({ ...previous, openPath: normalized, selectedPath: normalized }))
  }, [updateFilesState])

  // A requested path is a folder to stand in or a file to read, and only its
  // parent's listing says which. A file, or a path the listing does not carry,
  // opens in the viewer, which is where a missing or unreadable file is
  // reported plainly; a parent that cannot be listed leaves the tree to try.
  // The request is acknowledged at once, so the answer is kept only until a
  // newer request replaces it, not until the acknowledgement re-renders.
  const navigateSequence = useRef(0)
  useEffect(() => {
    if (!navigateRequest) return
    const { path, requestId } = navigateRequest
    onNavigateRequestHandled?.(requestId)
    navigateSequence.current += 1
    const sequence = navigateSequence.current
    const stillWanted = () => navigateSequence.current === sequence
    const target = normalizeFilePath(path)
    const parent = getParentPath(target)
    if (parent === target) {
      navigateTo(target)
      return
    }
    void fetchDirectory(parent)
      .then(siblings => {
        if (!stillWanted()) return
        const item = siblings.find(candidate => normalizeFilePath(candidate.path) === target)
        if (item?.isDir) {
          navigateTo(target)
          return
        }
        navigateTo(parent)
        openPath(target)
      })
      .catch(() => {
        if (stillWanted()) navigateTo(target)
      })
  }, [navigateRequest, navigateTo, onNavigateRequestHandled, openPath])

  const trimmedQuery = query.trim()

  useEffect(() => {
    if (!trimmedQuery) {
      setMatches([])
      setTruncated(false)
      setFinding(false)
      return
    }
    const controller = new AbortController()
    setFinding(true)
    const timer = setTimeout(() => {
      void findFiles(trimmedQuery, controller.signal)
        .then(result => {
          setMatches(result.matches)
          setTruncated(result.truncated)
          setFinding(false)
        })
        .catch(error => {
          if (controller.signal.aborted) return
          setMatches([])
          setTruncated(false)
          setFinding(false)
          announce(`Find failed: ${getErrorMessage(error, 'read')}`, 'error')
        })
    }, FIND_DEBOUNCE_MS)
    return () => {
      clearTimeout(timer)
      controller.abort()
    }
  }, [announce, trimmedQuery])

  const refreshFiles = useCallback(() => {
    setRefreshToken(previous => previous + 1)
  }, [])

  const openContextMenu = (event: ReactMouseEvent<HTMLElement>, item: FileItem | null) => {
    event.preventDefault()
    if (item) {
      updateFilesState(previous => ({ ...previous, selectedPath: item.path }))
    }
    setContextMenu({ x: event.clientX, y: event.clientY, item })
  }

  const startCreate = (kind: 'file' | 'folder') => {
    setContextMenu(null)
    setNameDialog({
      kind,
      item: null,
      value: kind === 'folder' ? 'new-folder' : 'new-file.txt',
    })
  }

  const startRename = (item: FileItem) => {
    setContextMenu(null)
    setNameDialog({ kind: 'rename', item, value: item.name })
  }

  const submitNameDialog = async (event: FormEvent) => {
    event.preventDefault()
    if (!nameDialog) return

    try {
      if (nameDialog.kind === 'file') {
        await createFile(filesState.currentPath, nameDialog.value)
      } else if (nameDialog.kind === 'folder') {
        await createFolder(filesState.currentPath, nameDialog.value)
      } else if (nameDialog.item) {
        const sourcePath = nameDialog.item.path
        const destination = joinFilePath(getParentPath(sourcePath), sanitizeFilename(nameDialog.value))
        if (destination !== sourcePath) {
          await renameItem(sourcePath, destination)
          updateFilesState(previous => {
            const remapPath = (path: string | null) => {
              if (!path || (path !== sourcePath && !path.startsWith(`${sourcePath}/`))) return path
              return `${destination}${path.slice(sourcePath.length)}`
            }
            return {
              ...previous,
              selectedPath: remapPath(previous.selectedPath),
              openPath: remapPath(previous.openPath),
              expandedPaths: previous.expandedPaths.map(path => remapPath(path) || path),
            }
          })
        }
      }
      setNameDialog(null)
      refreshFiles()
    } catch (error) {
      announce(getErrorMessage(error, nameDialog.kind === 'rename' ? 'rename' : 'create'), 'error')
    }
  }

  const requestDelete = (item: FileItem) => {
    setContextMenu(null)
    setDeleteTarget(item)
  }

  const confirmDelete = async () => {
    if (!deleteTarget) return
    try {
      await deleteItem(deleteTarget.path)
      const deletedPath = deleteTarget.path
      const covers = (path: string | null) => Boolean(path) && (path === deletedPath || path!.startsWith(`${deletedPath}/`))
      updateFilesState(previous => ({
        ...previous,
        selectedPath: covers(previous.selectedPath) ? null : previous.selectedPath,
        openPath: covers(previous.openPath) ? null : previous.openPath,
        expandedPaths: previous.expandedPaths.filter(path => path !== deletedPath && !path.startsWith(`${deletedPath}/`)),
      }))
      setDeleteTarget(null)
      refreshFiles()
      announce(`Deleted ${deleteTarget.name}`, 'success')
    } catch (error) {
      announce(getErrorMessage(error, 'delete'), 'error')
    }
  }

  const handleUploadInput = async (event: ChangeEvent<HTMLInputElement>) => {
    const files = event.target.files ? Array.from(event.target.files) : []
    if (files.length === 0) return
    try {
      await uploadFiles(filesState.currentPath, files)
      refreshFiles()
      announce(`Uploaded ${files.length} item${files.length === 1 ? '' : 's'}`, 'success')
    } catch (error) {
      announce(getErrorMessage(error, 'upload'), 'error')
    } finally {
      if (uploadInputRef.current) uploadInputRef.current.value = ''
    }
  }

  const copyPath = (path: string) => {
    const shown = normalizeFilePath(path)
    void copyAndAnnounce(shown, shown, announce)
    setContextMenu(null)
  }

  const togglePin = (item: FileItem) => {
    togglePinnedPath(item.path, item.isDir ? 'directory' : 'file')
    setContextMenu(null)
  }

  const widest = useCallback(() => 560, [])
  const resize = useResizableWidth({
    elementRef: panelRef,
    width,
    minWidth: 240,
    maxWidth: widest,
    edge: 'right',
    onCommit: onWidthChange,
  })

  const sendPath = sendTarget
    ? (path: string) => openSendToSession({ targetSessionKey: sendTarget, reference: `path ${path}` })
    : null

  const panelStyle = collapsed ? undefined : ({ '--terminal-files-width': `${resize.width}px` } as CSSProperties)

  return (
    <aside
      ref={panelRef}
      id={panelId}
      className={`terminal-files-panel ${collapsed ? 'collapsed' : ''} ${pinned ? 'sidecar-pinned' : 'sidecar-overlay'}`}
      style={panelStyle}
      data-workspace-files={workspaceId}
      aria-label="Files sidecar"
    >
      <header className="terminal-files-header">
        <strong className="terminal-sidecar-title">Files</strong>
        {!collapsed && (
          <>
            {canPin && (
              <button
                type="button"
                className="sidecar-pin-btn"
                aria-label={pinned ? 'Unpin Files sidecar' : 'Pin Files sidecar'}
                title={pinned ? 'Unpin sidecar' : 'Pin sidecar'}
                aria-pressed={pinned}
                onClick={onTogglePin}
              >
                {pinned ? <PinOff size={15} aria-hidden="true" /> : <Pin size={15} aria-hidden="true" />}
              </button>
            )}
            <button
              type="button"
              className="sidecar-close-btn"
              aria-label="Close Files sidecar"
              title="Close sidecar"
              onClick={onClose}
            >
              <X size={16} aria-hidden="true" />
            </button>
          </>
        )}
      </header>
      {!collapsed && filesState.openPath && (
        <FilePanelViewer
          path={filesState.openPath}
          onBack={() => updateFilesState(previous => ({ ...previous, openPath: null }))}
          onOpenPath={openPath}
          onSend={sendPath}
        />
      )}
      {!collapsed && !filesState.openPath && (
        <>
          <input
            ref={uploadInputRef}
            className="fb-hidden-input terminal-files-upload"
            type="file"
            multiple
            onChange={event => void handleUploadInput(event)}
          />
          <div className="terminal-files-search">
            <input
              aria-label="Find files"
              placeholder="Find files…"
              autoComplete="off"
              spellCheck={false}
              value={query}
              onChange={event => setQuery(event.target.value)}
              onKeyDown={event => {
                if (event.key === 'Enter' && matches.length > 0) {
                  event.preventDefault()
                  openPath(matches[0].path)
                  return
                }
                if (event.key === 'Escape' && query) {
                  event.preventDefault()
                  event.stopPropagation()
                  setQuery('')
                }
              }}
            />
          </div>
          {trimmedQuery ? (
            <>
              <div className="terminal-files-matches" role="list" aria-label="Find results">
                {matches.map((match, index) => (
                  <button
                    type="button"
                    role="listitem"
                    key={match.path}
                    className={`terminal-files-match ${index === 0 ? 'is-first' : ''}`}
                    onClick={() => openPath(match.path)}
                  >
                    <PanelPath path={match.path} />
                  </button>
                ))}
              </div>
              <p className="terminal-files-count">
                {finding && matches.length === 0
                  ? 'Finding…'
                  : matches.length === 0
                    ? 'No path matches'
                    : `${matches.length} path${matches.length === 1 ? '' : 's'}${truncated ? ' · more matched' : ''} · Enter opens the first`}
              </p>
            </>
          ) : (
            <>
              <div className="terminal-files-nav">
                <button
                  type="button"
                  className="terminal-files-up"
                  aria-label="Go to parent folder"
                  title="Go to parent folder"
                  disabled={filesState.currentPath === '/'}
                  onClick={() => navigateTo(getParentPath(filesState.currentPath))}
                >
                  <ArrowUp size={15} aria-hidden="true" />
                </button>
                <PanelPath path={filesState.currentPath} className="terminal-files-here" />
                {sessionCwd && <button type="button" className="terminal-files-cwd" onClick={() => navigateTo(sessionCwd)}>CWD</button>}
                <button
                  type="button"
                  className="terminal-files-refresh"
                  aria-label="Refresh Files"
                  title="Refresh Files"
                  onClick={refreshFiles}
                >
                  ↻
                </button>
              </div>
              <FileTree
                rootPath={filesState.currentPath}
                currentPath={filesState.currentPath}
                selectedPath={filesState.selectedPath}
                expandedPaths={filesState.expandedPaths}
                scrollTop={filesState.treeScrollTop}
                refreshToken={refreshToken}
                onOpenDirectory={navigateTo}
                onOpenFile={item => openPath(item.path)}
                onExpandedPathsChange={expandedPaths => updateFilesState(previous => ({ ...previous, expandedPaths }))}
                onScrollTopChange={treeScrollTop => updateFilesState(previous => ({ ...previous, treeScrollTop }))}
                onItemContextMenu={(event, item) => openContextMenu(event, item)}
                onBackgroundContextMenu={event => openContextMenu(event, null)}
              />
            </>
          )}
        </>
      )}
      {!collapsed && (
        <div
          {...resize.handleProps}
          className={`dock-resizer${resize.resizing ? ' dragging' : ''}`}
          role="separator"
          aria-label="Resize Files panel"
          aria-orientation="vertical"
          aria-valuenow={Math.round(resize.width)}
          aria-valuemin={240}
          aria-valuemax={560}
          tabIndex={0}
        />
      )}
      {contextMenu && (
        <FileContextMenu
          x={contextMenu.x}
          y={contextMenu.y}
          item={contextMenu.item}
          itemPinned={Boolean(contextMenu.item && pinnedPaths.some(item => item.path === contextMenu.item!.path))}
          currentPathPinned={pinnedPaths.some(item => item.path === filesState.currentPath)}
          onClose={() => setContextMenu(null)}
          onOpen={item => {
            setContextMenu(null)
            if (item.isDir) navigateTo(item.path)
            else openPath(item.path)
          }}
          onDownload={item => {
            window.open(getDownloadUrl(item.path), '_blank')
            setContextMenu(null)
          }}
          onRename={startRename}
          onTogglePin={togglePin}
          onCopyPath={copyPath}
          onCopyRelativePath={path => {
            const relative = pathRelativeTo(filesState.currentPath, path)
            void copyAndAnnounce(relative, relative, announce)
            setContextMenu(null)
          }}
          onOpenParent={path => {
            setContextMenu(null)
            onOpenInFiles(getParentPath(path))
          }}
          onDelete={requestDelete}
          onNewFile={() => startCreate('file')}
          onNewFolder={() => startCreate('folder')}
          onUpload={() => {
            setContextMenu(null)
            uploadInputRef.current?.click()
          }}
          onRefresh={() => {
            setContextMenu(null)
            refreshFiles()
          }}
          onCopyCurrentPath={() => copyPath(filesState.currentPath)}
          onToggleCurrentPathPin={() => {
            togglePinnedPath(filesState.currentPath, 'directory')
            setContextMenu(null)
          }}
        />
      )}
      {nameDialog && (
        <div className="fb-dialog-overlay">
          <div
            className="fb-dialog"
            role="dialog"
            aria-modal="true"
            aria-label={nameDialog.kind === 'rename' ? `Rename ${nameDialog.item?.name || 'item'}` : nameDialog.kind === 'folder' ? 'New Folder' : 'New File'}
          >
            <div className="fb-dialog-header">
              <h3>{nameDialog.kind === 'rename' ? 'Rename' : nameDialog.kind === 'folder' ? 'New Folder' : 'New File'}</h3>
              <button className="fb-dialog-close" type="button" onClick={() => setNameDialog(null)}>×</button>
            </div>
            <form onSubmit={event => void submitNameDialog(event)}>
              <div className="fb-dialog-body">
                <label className="fb-dialog-label">
                  {nameDialog.kind === 'rename' ? 'New name' : nameDialog.kind === 'folder' ? 'Folder name' : 'File name'}
                  <input
                    className="fb-dialog-input"
                    aria-label={nameDialog.kind === 'rename' ? 'New name' : nameDialog.kind === 'folder' ? 'Folder name' : 'File name'}
                    value={nameDialog.value}
                    onChange={event => setNameDialog(previous => previous ? { ...previous, value: event.target.value } : previous)}
                    autoFocus
                  />
                </label>
              </div>
              <div className="fb-dialog-footer">
                <button className="fb-dialog-btn fb-dialog-btn-cancel" type="button" onClick={() => setNameDialog(null)}>Cancel</button>
                <button className="fb-dialog-btn fb-dialog-btn-primary" type="submit" disabled={!nameDialog.value.trim()}>
                  {nameDialog.kind === 'rename' ? 'Rename' : 'Create'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
      {deleteTarget && (
        <div className="fb-dialog-overlay">
          <div className="fb-dialog fb-dialog-danger" role="dialog" aria-modal="true" aria-label={`Delete ${deleteTarget.name}`}>
            <div className="fb-dialog-header">
              <h3>Delete {deleteTarget.name}</h3>
              <button className="fb-dialog-close" type="button" onClick={() => setDeleteTarget(null)}>×</button>
            </div>
            <div className="fb-dialog-body">
              <p className="fb-dialog-message">This permanently removes the selected item from disk.</p>
            </div>
            <div className="fb-dialog-footer">
              <button className="fb-dialog-btn fb-dialog-btn-cancel" type="button" onClick={() => setDeleteTarget(null)}>Cancel</button>
              <button className="fb-dialog-btn fb-dialog-btn-danger" type="button" onClick={() => void confirmDelete()}>Delete</button>
            </div>
          </div>
        </div>
      )}
    </aside>
  )
}

export default TerminalFilesPanel
import './TerminalFilesPanel.css'
