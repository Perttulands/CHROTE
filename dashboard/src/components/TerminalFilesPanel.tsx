import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { ChangeEvent, CSSProperties, FormEvent, MouseEvent as ReactMouseEvent, PointerEvent as ReactPointerEvent } from 'react'
import { ArrowUp, Pin, PinOff, X } from 'lucide-react'
import { useSession } from '../context/SessionContext'
import { getSessionKey, getSessionNameFromKey, type WorkspaceId } from '../types'
import { copyTextToClipboard } from '../utils/clipboard'
import FileTree from './FileTree'
import { FileContextMenu } from './FileContextMenu'
import FileViewer, { normalizeFilePath } from './FileViewer'
import {
  createFile,
  createFolder,
  deleteItem,
  getDownloadUrl,
  getErrorMessage,
  renameItem,
  sanitizeFilename,
  uploadFiles,
} from './FilesView/fileService'
import { getParentPath, joinFilePath, pathRelativeTo } from './FilesView/pathActions'
import { pruneViewStates, remapViewStates } from './FilesView/openFilesModel'
import { usePinnedPaths } from './FilesView/pinnedPaths'
import type { FileItem } from './FilesView/types'
import {
  DEFAULT_FILE_VIEW_STATE,
  readWorkspaceFilesState,
  writeWorkspaceFilesState,
  type FileViewState,
  type WorkspaceFilePeekState,
  type WorkspaceFilesState,
} from './workspaceFilesState'

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

interface FilePeekProps {
  peek: WorkspaceFilePeekState
  viewState: FileViewState
  sendTarget: string | null
  sendTargetLabel: string | null
  onChangePeek: (peek: WorkspaceFilePeekState) => void
  onChangeViewState: (state: FileViewState) => void
  onClose: () => void
  onOpenInFiles: (path: string) => void
  onSend: (path: string) => void
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

function clampPeek(peek: WorkspaceFilePeekState): WorkspaceFilePeekState {
  const viewportWidth = typeof window === 'undefined' ? 1280 : window.innerWidth
  const viewportHeight = typeof window === 'undefined' ? 800 : window.innerHeight
  const width = Math.min(Math.max(320, peek.width), Math.max(320, viewportWidth - 24))
  const height = Math.min(Math.max(260, peek.height), Math.max(260, viewportHeight - 72))
  return {
    ...peek,
    width,
    height,
    x: Math.min(Math.max(0, peek.x), Math.max(0, viewportWidth - width)),
    y: Math.min(Math.max(48, peek.y), Math.max(48, viewportHeight - height)),
  }
}

function FilePeek({
  peek,
  viewState,
  sendTarget,
  sendTargetLabel,
  onChangePeek,
  onChangeViewState,
  onClose,
  onOpenInFiles,
  onSend,
}: FilePeekProps) {
  const dragRef = useRef<{ pointerId: number; startX: number; startY: number; x: number; y: number } | null>(null)
  const resizeRef = useRef<{ pointerId: number; startX: number; startY: number; width: number; height: number } | null>(null)
  const item: FileItem = useMemo(() => ({
    path: peek.path,
    name: peek.name,
    size: peek.size,
    type: peek.type,
    isDir: false,
    modified: '',
  }), [peek.name, peek.path, peek.size, peek.type])

  useEffect(() => {
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && !(event.target as Element | null)?.closest('.send-session-modal')) onClose()
    }
    window.addEventListener('keydown', closeOnEscape)
    return () => window.removeEventListener('keydown', closeOnEscape)
  }, [onClose])

  useEffect(() => {
    const clampOnResize = () => onChangePeek(clampPeek(peek))
    window.addEventListener('resize', clampOnResize)
    return () => window.removeEventListener('resize', clampOnResize)
  }, [onChangePeek, peek])

  const beginDrag = (event: ReactPointerEvent<HTMLElement>) => {
    if ((event.target as HTMLElement).closest('button')) return
    event.currentTarget.setPointerCapture(event.pointerId)
    dragRef.current = {
      pointerId: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
      x: peek.x,
      y: peek.y,
    }
  }

  const moveDrag = (event: ReactPointerEvent<HTMLElement>) => {
    const drag = dragRef.current
    if (!drag || drag.pointerId !== event.pointerId) return
    onChangePeek(clampPeek({
      ...peek,
      x: drag.x + event.clientX - drag.startX,
      y: drag.y + event.clientY - drag.startY,
    }))
  }

  const endDrag = (event: ReactPointerEvent<HTMLElement>) => {
    if (dragRef.current?.pointerId === event.pointerId) dragRef.current = null
  }

  const beginResize = (event: ReactPointerEvent<HTMLDivElement>) => {
    event.preventDefault()
    event.currentTarget.setPointerCapture(event.pointerId)
    resizeRef.current = {
      pointerId: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
      width: peek.width,
      height: peek.height,
    }
  }

  const moveResize = (event: ReactPointerEvent<HTMLDivElement>) => {
    const resize = resizeRef.current
    if (!resize || resize.pointerId !== event.pointerId) return
    onChangePeek(clampPeek({
      ...peek,
      width: resize.width + event.clientX - resize.startX,
      height: resize.height + event.clientY - resize.startY,
    }))
  }

  const endResize = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (resizeRef.current?.pointerId === event.pointerId) resizeRef.current = null
  }

  const style = {
    left: `${peek.x}px`,
    top: `${peek.y}px`,
    width: `${peek.width}px`,
    height: `${peek.height}px`,
  } as CSSProperties

  return (
    <section
      className="file-peek"
      style={style}
      role="dialog"
      aria-modal="false"
      aria-label={`File Peek: ${peek.name}`}
    >
      <header
        className="file-peek-header"
        onPointerDown={beginDrag}
        onPointerMove={moveDrag}
        onPointerUp={endDrag}
        onPointerCancel={endDrag}
      >
        <div className="file-peek-title">
          <strong>{peek.name}</strong>
          <span title={peek.path}>{peek.path}</span>
        </div>
        <div className="file-peek-actions">
          <button type="button" onClick={() => void copyTextToClipboard(peek.path)}>Copy Path</button>
          <button
            type="button"
            aria-label={`Send ${peek.name} to session`}
            disabled={!sendTarget}
            title={sendTarget ? `Send path to ${sendTargetLabel}` : 'Focus a terminal session first'}
            onClick={() => onSend(peek.path)}
          >
            Send…
          </button>
          <button type="button" aria-label={`Open ${peek.name} in Files tab`} onClick={() => onOpenInFiles(peek.path)}>Full</button>
          <button type="button" aria-label="Close File Peek" onClick={onClose}>×</button>
        </div>
      </header>
      <div className="file-peek-target">Send target: {sendTargetLabel || 'focus a live terminal first'}</div>
      <FileViewer item={item} viewState={viewState} onViewStateChange={onChangeViewState} />
      <div
        className="file-peek-resizer"
        role="separator"
        aria-label="Resize File Peek"
        aria-orientation="vertical"
        tabIndex={0}
        onPointerDown={beginResize}
        onPointerMove={moveResize}
        onPointerUp={endResize}
        onPointerCancel={endResize}
        onKeyDown={event => {
          const delta = event.shiftKey ? 40 : 16
          if (!['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown'].includes(event.key)) return
          event.preventDefault()
          onChangePeek(clampPeek({
            ...peek,
            width: peek.width + (event.key === 'ArrowRight' ? delta : event.key === 'ArrowLeft' ? -delta : 0),
            height: peek.height + (event.key === 'ArrowDown' ? delta : event.key === 'ArrowUp' ? -delta : 0),
          }))
        }}
      />
    </section>
  )
}

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
  const uploadInputRef = useRef<HTMLInputElement | null>(null)
  const [filesState, setFilesState] = useState<WorkspaceFilesState>(() => readWorkspaceFilesState(workspaceId))
  const [pathDraft, setPathDraft] = useState(filesState.currentPath)
  const [refreshToken, setRefreshToken] = useState(0)
  const [contextMenu, setContextMenu] = useState<ContextMenuState | null>(null)
  const [nameDialog, setNameDialog] = useState<NameDialogState | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<FileItem | null>(null)
  const [toast, setToast] = useState<string | null>(null)
  const [operationLabel, setOperationLabel] = useState<string | null>(null)
  const [pinnedPaths, togglePinnedPath] = usePinnedPaths()
  const workspace = workspaces[workspaceId]

  const updateFilesState = useCallback((update: (previous: WorkspaceFilesState) => WorkspaceFilesState) => {
    setFilesState(previous => {
      const next = update(previous)
      writeWorkspaceFilesState(workspaceId, next)
      return next
    })
  }, [workspaceId])

  useEffect(() => {
    if (!filesState.peek) return
    const clamped = clampPeek(filesState.peek)
    if (
      clamped.x !== filesState.peek.x || clamped.y !== filesState.peek.y ||
      clamped.width !== filesState.peek.width || clamped.height !== filesState.peek.height
    ) {
      updateFilesState(previous => ({ ...previous, peek: clamped }))
    }
  }, [filesState.peek, updateFilesState])

  const focusedWindow = useMemo(() => {
    const focused = workspace.windows.find(window => focusedWindowKey === `${workspaceId}-${window.id}`)
    return focused || workspace.windows.slice(0, workspace.windowCount).find(window => window.activeSession)
  }, [focusedWindowKey, workspace, workspaceId])
  const sendTarget = focusedWindow?.activeSession && focusedWindow.activeSession !== 'INIT-PENDING'
    ? focusedWindow.activeSession
    : null
  const sendTargetLabel = sendTarget ? getSessionNameFromKey(sendTarget) : null
  const sessionCwd = useMemo(() => {
    if (!sendTarget) return null
    return sessions.find(entry => getSessionKey(entry.name, entry.unixUser) === sendTarget)?.cwd || null
  }, [sendTarget, sessions])

  const navigateTo = useCallback((path: string) => {
    const normalized = normalizeFilePath(path)
    setPathDraft(normalized)
    updateFilesState(previous => ({
      ...previous,
      currentPath: normalized,
      expandedPaths: Array.from(new Set([...previous.expandedPaths, normalized])),
      selectedPath: null,
      treeScrollTop: 0,
    }))
  }, [updateFilesState])

  useEffect(() => {
    if (!navigateRequest) return
    navigateTo(navigateRequest.path)
    onNavigateRequestHandled?.(navigateRequest.requestId)
  }, [navigateRequest, navigateTo, onNavigateRequestHandled])

  const openPeek = useCallback((item: FileItem) => {
    const width = Math.min(760, Math.max(360, window.innerWidth - 96))
    const height = Math.min(680, Math.max(320, window.innerHeight - 136))
    updateFilesState(previous => ({
      ...previous,
      selectedPath: item.path,
      peek: clampPeek({
        path: item.path,
        name: item.name,
        size: item.size,
        type: item.type,
        x: Math.max(48, Math.round((window.innerWidth - width) / 2)),
        y: 84,
        width,
        height,
      }),
    }))
  }, [updateFilesState])

  const updatePeek = useCallback((peek: WorkspaceFilePeekState) => {
    updateFilesState(previous => ({ ...previous, peek }))
  }, [updateFilesState])

  const updatePeekView = useCallback((viewState: FileViewState) => {
    if (!filesState.peek) return
    const path = filesState.peek.path
    updateFilesState(previous => ({
      ...previous,
      fileViewStates: { ...previous.fileViewStates, [path]: viewState },
    }))
  }, [filesState.peek, updateFilesState])

  const submitPath = (event: FormEvent) => {
    event.preventDefault()
    navigateTo(pathDraft)
  }

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

    const operation = nameDialog.kind === 'rename'
      ? 'Renaming'
      : nameDialog.kind === 'folder' ? 'Creating folder' : 'Creating file'
    setOperationLabel(operation)
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
            const peekPath = previous.peek ? remapPath(previous.peek.path) : null
            return {
              ...previous,
              selectedPath: remapPath(previous.selectedPath),
              expandedPaths: previous.expandedPaths.map(path => remapPath(path) || path),
              fileViewStates: remapViewStates(previous.fileViewStates, sourcePath, destination),
              peek: previous.peek && peekPath ? {
                ...previous.peek,
                path: peekPath,
                name: previous.peek.path === sourcePath ? sanitizeFilename(nameDialog.value) : previous.peek.name,
              } : previous.peek,
            }
          })
        }
      }
      setNameDialog(null)
      refreshFiles()
    } catch (error) {
      setToast(getErrorMessage(error, nameDialog.kind === 'rename' ? 'rename' : 'create'))
    } finally {
      setOperationLabel(null)
    }
  }

  const requestDelete = (item: FileItem) => {
    setContextMenu(null)
    setDeleteTarget(item)
  }

  const confirmDelete = async () => {
    if (!deleteTarget) return
    setOperationLabel('Deleting')
    try {
      await deleteItem(deleteTarget.path)
      const deletedPath = deleteTarget.path
      updateFilesState(previous => ({
        ...previous,
        selectedPath: previous.selectedPath === deletedPath || previous.selectedPath?.startsWith(`${deletedPath}/`)
          ? null
          : previous.selectedPath,
        expandedPaths: previous.expandedPaths.filter(path => path !== deletedPath && !path.startsWith(`${deletedPath}/`)),
        fileViewStates: pruneViewStates(previous.fileViewStates, [deletedPath]),
        peek: previous.peek && (previous.peek.path === deletedPath || previous.peek.path.startsWith(`${deletedPath}/`))
          ? null
          : previous.peek,
      }))
      setDeleteTarget(null)
      refreshFiles()
    } catch (error) {
      setToast(getErrorMessage(error, 'delete'))
    } finally {
      setOperationLabel(null)
    }
  }

  const handleUploadInput = async (event: ChangeEvent<HTMLInputElement>) => {
    const files = event.target.files ? Array.from(event.target.files) : []
    if (files.length === 0) return
    setOperationLabel(`Uploading ${files.length} item${files.length === 1 ? '' : 's'}`)
    try {
      await uploadFiles(filesState.currentPath, files)
      refreshFiles()
    } catch (error) {
      setToast(getErrorMessage(error, 'upload'))
    } finally {
      setOperationLabel(null)
      if (uploadInputRef.current) uploadInputRef.current.value = ''
    }
  }

  const copyPath = (path: string) => {
    void copyTextToClipboard(normalizeFilePath(path))
    setContextMenu(null)
  }

  const togglePin = (item: FileItem) => {
    togglePinnedPath(item.path, item.isDir ? 'directory' : 'file')
    setContextMenu(null)
  }

  const startPanelResize = (event: ReactPointerEvent<HTMLDivElement>) => {
    event.currentTarget.setPointerCapture(event.pointerId)
    const startX = event.clientX
    const startWidth = width
    const pointerId = event.pointerId
    const move = (moveEvent: PointerEvent) => {
      if (moveEvent.pointerId !== pointerId) return
      onWidthChange(Math.min(560, Math.max(240, startWidth + moveEvent.clientX - startX)))
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
  }

  const panelStyle = collapsed ? undefined : ({ '--terminal-files-width': `${width}px` } as CSSProperties)
  const viewState = filesState.peek
    ? filesState.fileViewStates[filesState.peek.path] || DEFAULT_FILE_VIEW_STATE
    : DEFAULT_FILE_VIEW_STATE

  return (
    <aside
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
      {!collapsed && (
        <>
          <input
            ref={uploadInputRef}
            className="fb-hidden-input terminal-files-upload"
            type="file"
            multiple
            onChange={event => void handleUploadInput(event)}
          />
          <form className="terminal-files-path" aria-label="Files panel path form" onSubmit={submitPath}>
            <input aria-label="Files panel path" value={pathDraft} onChange={event => setPathDraft(event.target.value)} />
          </form>
          <FileTree
            rootPath={filesState.currentPath}
            currentPath={filesState.currentPath}
            selectedPath={filesState.selectedPath}
            expandedPaths={filesState.expandedPaths}
            scrollTop={filesState.treeScrollTop}
            refreshToken={refreshToken}
            onOpenDirectory={navigateTo}
            onOpenFile={openPeek}
            onExpandedPathsChange={expandedPaths => updateFilesState(previous => ({ ...previous, expandedPaths }))}
            onScrollTopChange={treeScrollTop => updateFilesState(previous => ({ ...previous, treeScrollTop }))}
            onItemContextMenu={(event, item) => openContextMenu(event, item)}
            onBackgroundContextMenu={event => openContextMenu(event, null)}
          />
          <footer className="terminal-files-footer" title={sendTarget || undefined}>
            {operationLabel ? `${operationLabel}…` : `Target · ${sendTargetLabel || 'focus a session'}`}
          </footer>
          <div
            className="dock-resizer"
            role="separator"
            aria-label="Resize Files panel"
            aria-orientation="vertical"
            tabIndex={0}
            onPointerDown={startPanelResize}
            onKeyDown={event => {
              if (event.key === 'ArrowLeft') onWidthChange(Math.max(240, width - 16))
              if (event.key === 'ArrowRight') onWidthChange(Math.min(560, width + 16))
            }}
          />
        </>
      )}
      {filesState.peek && (
        <FilePeek
          peek={filesState.peek}
          viewState={viewState}
          sendTarget={sendTarget}
          sendTargetLabel={sendTargetLabel}
          onChangePeek={updatePeek}
          onChangeViewState={updatePeekView}
          onClose={() => updateFilesState(previous => ({ ...previous, peek: null }))}
          onOpenInFiles={onOpenInFiles}
          onSend={path => {
            if (sendTarget) openSendToSession(sendTarget, `Please inspect:\n\n${path}\n\nNote: `)
          }}
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
            else openPeek(item)
          }}
          onDownload={item => {
            window.open(getDownloadUrl(item.path), '_blank')
            setContextMenu(null)
          }}
          onRename={startRename}
          onTogglePin={togglePin}
          onCopyPath={copyPath}
          onCopyRelativePath={path => {
            void copyTextToClipboard(pathRelativeTo(filesState.currentPath, path))
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
      {toast && (
        <div className="fb-error-toast" role="alert">
          <span className="fb-error-toast-icon">!</span>
          <span className="fb-error-toast-message">{toast}</span>
          <button className="fb-error-toast-dismiss" type="button" onClick={() => setToast(null)}>x</button>
        </div>
      )}
    </aside>
  )
}

export default TerminalFilesPanel
