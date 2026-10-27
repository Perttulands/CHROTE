import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { ChangeEvent, CSSProperties, FormEvent, MouseEvent as ReactMouseEvent } from 'react'
import { useSession } from '../context/SessionContext'
import { useStatus } from '../context/StatusContext'
import { useResizableWidth } from '../hooks/useResizableWidth'
import { getSessionKey, type WorkspaceId } from '../types'
import { copyAndAnnounce } from '../utils/clipboard'
import FilePopout from './FilePopout'
import FileTree from './FileTree'
import { FileContextMenu } from './FileContextMenu'
import { normalizeFilePath } from './fileKinds'
import PanelPath from './PanelPath'
import PathTrail from './PathTrail'
import SearchField from './kit/SearchField'
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
import FilePanelViewer from './FilePanelViewer'
import FilesProjects, { projectGroups, projectOf } from './FilesView/FilesProjects'
import ProjectLanding from './FilesView/ProjectLanding'
import { fetchWorkspaces, workspaceName, type Workspace } from '../workspaces/workspacesApi'
import {
  readWorkspaceFilesState,
  writeWorkspaceFilesState,
  type FilesHome,
  type WorkspaceFilesState,
} from './workspaceFilesState'

/** How long the panel waits after a keystroke before it asks the server. */
const FIND_DEBOUNCE_MS = 140

interface TerminalFilesPanelProps {
  workspaceId: FilesHome
  /**
   * Beside a terminal, the panel is a sidecar and an open file hangs off its
   * edge; as the Files tab it is the tab's column and the file fills the rest.
   */
  place?: 'sidecar' | 'tab'
  /** The session a path is sent to where the panel has no terminal of its own. */
  sendTarget?: string | null
  collapsed: boolean
  width: number
  pinned: boolean
  panelId: string
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
 * Opening a file leaves the tree exactly where it is and hangs the file off
 * the panel's right edge in the pop-out, so walking a directory is one click
 * each and the row that is selected is the file being read. The pop-out is
 * owned by `openPath`: it is open while there is one, the next click retargets
 * it, and Close and Escape put it away.
 */
function TerminalFilesPanel({
  workspaceId,
  place = 'sidecar',
  sendTarget: givenSendTarget = null,
  collapsed,
  width,
  pinned,
  panelId,
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
  const workspace = place === 'tab' ? undefined : workspaces[workspaceId as WorkspaceId]

  const updateFilesState = useCallback((update: (previous: WorkspaceFilesState) => WorkspaceFilesState) => {
    setFilesState(previous => {
      const next = update(previous)
      writeWorkspaceFilesState(workspaceId, next)
      return next
    })
  }, [workspaceId])

  const focusedWindow = useMemo(() => {
    if (!workspace) return undefined
    const focused = workspace.windows.find(window => focusedWindowKey === `${workspaceId}-${window.id}`)
    return focused || workspace.windows.slice(0, workspace.windowCount).find(window => window.activeSession)
  }, [focusedWindowKey, workspace, workspaceId])
  const sendTarget = focusedWindow?.activeSession ?? givenSendTarget
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

  const closeOpenPath = useCallback(() => {
    updateFilesState(previous => ({ ...previous, openPath: null }))
  }, [updateFilesState])

  // Overlaid, the file and the sidecar are one glance: a press on a terminal
  // takes both away, as it did before there was a pop-out to take.
  const dismissOpenPath = useCallback(() => {
    closeOpenPath()
    onClose()
  }, [closeOpenPath, onClose])

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

  // As the Files tab, the panel's home is the host's projects rather than /,
  // and inside a project its find looks only there.
  const [projects, setProjects] = useState<Workspace[] | null>(null)
  const [projectsError, setProjectsError] = useState<string | null>(null)
  const [roots, setRoots] = useState<FileItem[] | null>(null)
  const atHome = place === 'tab' && filesState.currentPath === '/'
  const project = place === 'tab' && projects ? projectOf(projects, filesState.currentPath) : null
  const projectPath = project?.path ?? null
  useEffect(() => {
    if (place !== 'tab' || (!atHome && projects)) return
    let current = true
    fetchWorkspaces({ beads: true })
      .then(found => { if (current) { setProjects(found); setProjectsError(null) } })
      .catch((cause: unknown) => { if (current) setProjectsError(cause instanceof Error ? cause.message : 'Could not list the projects') })
    // The roots themselves, for the folders that are nobody's project.
    fetchDirectory('/')
      .then(items => { if (current) setRoots(items.filter(item => item.isDir)) })
      .catch(() => { if (current) setRoots([]) })
    return () => { current = false }
    // Asked again each time the operator comes home, never while working in a project.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [atHome, place])

  const openProject = useCallback((path: string) => {
    setQuery('')
    navigateTo(path)
  }, [navigateTo])

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
      void (projectPath ? findFiles(trimmedQuery, controller.signal, projectPath) : findFiles(trimmedQuery, controller.signal))
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
  }, [announce, atHome, projectPath, trimmedQuery])

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

  const findResults = (
    <>
    <div className="terminal-files-matches" role="list" aria-label="Find results">
      {matches.map(match => (
        <button
          type="button"
          role="listitem"
          key={match.path}
          className={`terminal-files-match ${filesState.openPath === match.path ? 'is-first' : ''}`}
          onClick={() => openPath(match.path)}
          onKeyDown={event => {
            if (event.altKey || event.ctrlKey || event.metaKey || event.shiftKey) return
            if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') return
            event.preventDefault()
            const rows = Array.from(panelRef.current?.querySelectorAll<HTMLButtonElement>('.terminal-files-match') ?? [])
            const next = rows[rows.indexOf(event.currentTarget) + (event.key === 'ArrowDown' ? 1 : -1)]
            if (next) { next.focus({ preventScroll: true }); next.scrollIntoView?.({ block: 'nearest' }); next.click() }
          }}
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
  )

  const pins = pinnedPaths.length > 0 && (
    <div className="terminal-files-pins" role="list" aria-label="Pinned">
      {pinnedPaths.map(pin => (
        <button
          type="button"
          role="listitem"
          key={pin.path}
          className={`terminal-files-match${pin.path === filesState.currentPath || pin.path === filesState.openPath ? ' is-first' : ''}`}
          title={pin.path}
          onClick={() => (pin.kind === 'directory' ? navigateTo(pin.path) : openPath(pin.path))}
        >
          <PanelPath path={pin.path} />
        </button>
      ))}
    </div>
  )

  const panel = (
      <aside
        ref={panelRef}
        id={panelId}
        className={`terminal-files-panel ${collapsed ? 'collapsed' : ''} ${pinned ? 'sidecar-pinned' : 'sidecar-overlay'}${place === 'tab' ? ' in-tab' : ''}`}
        style={panelStyle}
        data-workspace-files={workspaceId}
        aria-label={place === 'tab' ? 'Files' : 'Files sidecar'}
      >
        {place === 'sidecar' && <header className="terminal-files-header">
          <strong className="terminal-sidecar-title">Files</strong>
          {!collapsed && place === 'sidecar' && (
            <button type="button" className="kit-button" aria-label="Close Files sidecar" onClick={onClose}>
              Close
            </button>
          )}
        </header>}
        {!collapsed && (
          <>
            <input
              ref={uploadInputRef}
              className="fb-hidden-input terminal-files-upload"
              type="file"
              multiple
              onChange={event => void handleUploadInput(event)}
            />
            <div className="terminal-files-search">
              <SearchField
                label={atHome ? 'Search projects' : project ? `Search files in ${workspaceName(project.path)}` : 'Search files'}
                title="Down enters the list; Up/Down browses and opens files"
                value={query}
                onChange={setQuery}
                onKeyDown={event => {
                  if (atHome && event.key === 'Enter') {
                    const first = projectGroups(projects ?? [], query)[0]?.projects[0]
                    if (first) { event.preventDefault(); openProject(first.path) }
                    else if (matches.length > 0) { event.preventDefault(); openPath(matches[0].path) }
                    return
                  }
                  if (atHome && event.key === 'ArrowDown') {
                    const first = panelRef.current?.querySelector<HTMLElement>('.files-project')
                    if (first) { event.preventDefault(); first.focus() }
                    return
                  }
                  if (event.key === 'ArrowDown' && !event.altKey && !event.ctrlKey && !event.metaKey) {
                    const first = panelRef.current?.querySelector<HTMLElement>(trimmedQuery ? '.terminal-files-match' : '[role="treeitem"]')
                    if (first) { event.preventDefault(); first.focus(); if (!first.hasAttribute('aria-expanded')) first.click() }
                    return
                  }
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
            {atHome ? (
              <div className="files-home">
                {pins}
                <FilesProjects
                  workspaces={projects}
                  roots={roots}
                  error={projectsError}
                  query={query}
                  onOpen={openProject}
                />
                {trimmedQuery && (
                  <section className="files-home-find" aria-label="Files everywhere">
                    <h3 className="files-project-parent">Files everywhere</h3>
                    {findResults}
                  </section>
                )}
              </div>
            ) : trimmedQuery ? (
              <>
                {findResults}
              </>
            ) : (
              <>
                <div className="terminal-files-nav">
                  <PathTrail
                    path={filesState.currentPath}
                    onNavigate={navigateTo}
                    base={project ? { path: project.path, label: workspaceName(project.path) } : null}
                    home={place === 'tab' ? { label: 'Projects', onSelect: () => openProject('/') } : null}
                  />
                  {sessionCwd && <button type="button" className="kit-button" title={sessionCwd} onClick={() => navigateTo(sessionCwd)}>CWD</button>}
                </div>
                {pins}
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
        {!collapsed && place === 'sidecar' && filesState.openPath && (
          <FilePopout
            path={filesState.openPath}
            pinned={pinned}
            panelRef={panelRef}
            onClose={closeOpenPath}
            onDismiss={dismissOpenPath}
            onOpenPath={openPath}
            onSend={sendPath}
          />
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
                <button className="kit-button" type="button" onClick={() => setNameDialog(null)}>Close</button>
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
                  <button className="kit-button fb-dialog-btn fb-dialog-btn-cancel" type="button" onClick={() => setNameDialog(null)}>Cancel</button>
                  <button className="kit-button fb-dialog-btn fb-dialog-btn-primary" type="submit" disabled={!nameDialog.value.trim()}>
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
                <button className="kit-button" type="button" onClick={() => setDeleteTarget(null)}>Close</button>
              </div>
              <div className="fb-dialog-body">
                <p className="fb-dialog-message">This permanently removes the selected item from disk.</p>
              </div>
              <div className="fb-dialog-footer">
                <button className="kit-button fb-dialog-btn fb-dialog-btn-cancel" type="button" onClick={() => setDeleteTarget(null)}>Cancel</button>
                <button className="kit-button danger fb-dialog-btn" type="button" onClick={() => void confirmDelete()}>Delete</button>
              </div>
            </div>
          </div>
        )}
      </aside>
  )

  if (place === 'sidecar') return panel

  // The Files tab: the same column, and the file it opens filling the rest.
  return (
    <div className="files-tab" data-ui="files.tab">
      {panel}
      <section className="files-tab-viewer" aria-label="File">
        {filesState.openPath ? (
          <FilePanelViewer path={filesState.openPath} onClose={closeOpenPath} onOpenPath={openPath} onSend={sendPath} />
        ) : project ? (
          <ProjectLanding project={project} onOpenPath={openPath} />
        ) : (
          <p className="files-panel-note">{atHome ? 'Open a project, or find one or any file by name.' : 'Open a file from the tree, a pin, or by name.'}</p>
        )}
      </section>
    </div>
  )
}

export default TerminalFilesPanel
import './TerminalFilesPanel.css'
