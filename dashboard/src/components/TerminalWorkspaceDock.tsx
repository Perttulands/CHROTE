import { memo, useCallback, useEffect, useRef, useState } from 'react'
import type { Dispatch, SetStateAction } from 'react'
import { FolderTree, SquareTerminal } from 'lucide-react'
import type { WorkspaceId } from '../types'
import { useSession } from '../context/SessionContext'
import { useMediaQuery } from '../hooks/useMediaQuery'
import { registerSurface } from '../keys/dismiss'
import SessionPanel from './SessionPanel'
import TerminalArea from './TerminalArea'
import TerminalFilesPanel from './TerminalFilesPanel'
import {
  readWorkspaceFilesDockState,
  writeWorkspaceFilesDockState,
  type SessionsDockState,
  type WorkspaceFilesDockState,
} from './workspaceFilesState'

interface TerminalWorkspaceDockProps {
  workspaceId: WorkspaceId
  active: boolean
  sessionsDockState: SessionsDockState
  onSessionsDockStateChange: Dispatch<SetStateAction<SessionsDockState>>
  sessionsForcedPinned: boolean
  onFilesOpenChange: (workspaceId: WorkspaceId, open: boolean) => void
  onOpenInFiles: (path: string) => void
  /** A path asked for from a terminal link, routed here while this dock is the active tab. */
  openFilesRequest?: { path: string; nonce: number } | null
}

// An overlay sidecar is a glance over the terminals: what belongs to it is
// the panels themselves and the switcher that opens and closes them, so a
// press on the other panel's trigger still reaches it.
const SIDECAR_SURFACE = '.session-panel, .terminal-files-panel, .terminal-sidecar-switcher'

function TerminalWorkspaceDock({
  workspaceId,
  active,
  sessionsDockState,
  onSessionsDockStateChange,
  onFilesOpenChange,
  onOpenInFiles,
  openFilesRequest = null,
}: TerminalWorkspaceDockProps) {
  const { sessions } = useSession()
  const dockRef = useRef<HTMLDivElement>(null)
  const isNarrow = useMediaQuery('(max-width: 768px)')
  const [filesDockState, setFilesDockState] = useState<WorkspaceFilesDockState>(() => readWorkspaceFilesDockState(workspaceId))
  const [filesNavigateRequest, setFilesNavigateRequest] = useState<{ path: string; requestId: number } | null>(null)
  const nextFilesNavigateRequestId = useRef(0)
  const sessionsOpen = sessionsDockState.open
  const filesOpen = filesDockState.open
  const openSidecarCount = Number(sessionsOpen) + Number(filesOpen)
  // Desktop sidecars stay in the flex rail so they cannot cover terminals.
  // Narrow viewports retain the dismissible overlay presentation.
  const sessionsPinned = sessionsOpen && !isNarrow
  const filesPinned = filesOpen && !isNarrow
  const anyPinned = sessionsPinned || filesPinned
  const sessionsPanelId = `${workspaceId}-sessions-sidecar`
  const filesPanelId = `${workspaceId}-files-sidecar`

  useEffect(() => {
    writeWorkspaceFilesDockState(workspaceId, filesDockState)
  }, [filesDockState, workspaceId])

  useEffect(() => {
    onFilesOpenChange(workspaceId, filesDockState.open)
  }, [filesDockState.open, onFilesOpenChange, workspaceId])

  useEffect(() => () => {
    onFilesOpenChange(workspaceId, false)
  }, [onFilesOpenChange, workspaceId])

  const toggleSessions = useCallback(() => {
    onSessionsDockStateChange(previous => ({ ...previous, open: !previous.open }))
  }, [onSessionsDockStateChange])

  // Alt+O and the Files button run through here, and opening the panel is what
  // puts the cursor in its find field: mounting is not the same event, because
  // the panel also mounts when the operator merely returns to this terminal tab
  // and expects to keep typing where he was.
  const toggleFiles = useCallback(() => {
    const opening = !filesDockState.open
    setFilesDockState(previous => ({ ...previous, open: !previous.open }))
    if (!opening) return
    requestAnimationFrame(() => {
      const field = document.getElementById(filesPanelId)?.querySelector('input[aria-label="Find files"]')
      if (field instanceof HTMLInputElement) field.focus()
    })
  }, [filesDockState.open, filesPanelId])

  const closeSessions = useCallback(() => {
    onSessionsDockStateChange(previous => ({ ...previous, open: false }))
  }, [onSessionsDockStateChange])

  const closeFiles = useCallback(() => {
    setFilesDockState(previous => ({ ...previous, open: false }))
  }, [])

  const openFilesAtPath = useCallback((path: string) => {
    nextFilesNavigateRequestId.current += 1
    setFilesNavigateRequest({ path, requestId: nextFilesNavigateRequestId.current })
    setFilesDockState(previous => ({ ...previous, open: true }))
  }, [])

  const handleFilesNavigateRequest = useCallback((requestId: number) => {
    setFilesNavigateRequest(previous => previous?.requestId === requestId ? null : previous)
  }, [])

  // A path from a terminal link takes the same way in as the tag's own menu.
  useEffect(() => {
    if (openFilesRequest) openFilesAtPath(openFilesRequest.path)
  }, [openFilesRequest, openFilesAtPath])

  const closeAllSidecars = useCallback(() => {
    closeSessions()
    closeFiles()
  }, [closeFiles, closeSessions])

  const toggleFilesPin = useCallback(() => {
    if (isNarrow) return
    setFilesDockState(previous => ({ ...previous, pinned: !previous.pinned }))
  }, [isNarrow])

  // Unpinned sidecars overlay the terminals, so they are a glance: Escape and a
  // press outside close them, through the owner, and anything opened on top of
  // them is reached first.
  useEffect(() => {
    if (!active || openSidecarCount === 0 || anyPinned) return
    return registerSurface({
      kind: 'glance',
      close: closeAllSidecars,
      contains: target => Array.from(dockRef.current?.querySelectorAll(SIDECAR_SURFACE) ?? [])
        .some(element => element.contains(target)),
    })
  }, [active, anyPinned, closeAllSidecars, openSidecarCount])

  const sidecarControls = (
    <div className="terminal-sidecar-switcher" role="group" aria-label="Workspace sidecar">
      <button
        type="button"
        className={`terminal-sidecar-button ${sessionsOpen ? 'active' : ''}`}
        aria-label="Sessions sidecar"
        aria-controls={sessionsPanelId}
        aria-expanded={sessionsOpen}
        aria-pressed={sessionsOpen}
        title="Sessions"
        onClick={toggleSessions}
      >
        <SquareTerminal size={16} aria-hidden="true" />
        <span className="terminal-sidecar-label">Sessions</span>
        <span className="terminal-sidecar-count" aria-label={`${sessions.length} live sessions`}>{sessions.length}</span>
      </button>
      <button
        type="button"
        className={`terminal-sidecar-button ${filesOpen ? 'active' : ''}`}
        aria-label="Files sidecar"
        aria-controls={filesPanelId}
        aria-expanded={filesOpen}
        aria-pressed={filesOpen}
        title="Files"
        onClick={toggleFiles}
      >
        <FolderTree size={16} aria-hidden="true" />
        <span className="terminal-sidecar-label">Files</span>
      </button>
    </div>
  )

  return (
    <div
      ref={dockRef}
      className="terminal-workspace-dock"
      data-workspace={workspaceId}
      data-active={active}
      data-sidecar={[sessionsOpen && 'sessions', filesOpen && 'files'].filter(Boolean).join(',') || 'closed'}
      data-sidecar-count={openSidecarCount}
      data-sidecar-pinned={anyPinned}
      style={{ display: active ? 'flex' : 'none' }}
    >
      {active && openSidecarCount > 0 && !anyPinned && (
        <button
          type="button"
          className="terminal-sidecar-dismiss"
          aria-label="Close open sidecars"
          tabIndex={-1}
          onClick={closeAllSidecars}
        />
      )}
      {active && sessionsOpen && (
        <SessionPanel
          activeWorkspaceId={workspaceId}
          collapsed={false}
          width={sessionsDockState.width}
          pinned={sessionsPinned}
          panelId={sessionsPanelId}
          onClose={closeSessions}
          onWidthChange={width => onSessionsDockStateChange(previous => ({ ...previous, width }))}
          searchTerm={sessionsDockState.searchTerm}
          collapsedGroups={sessionsDockState.collapsedGroups}
          onSearchTermChange={searchTerm => onSessionsDockStateChange(previous => ({ ...previous, searchTerm }))}
          onCollapsedGroupsChange={collapsedGroups => onSessionsDockStateChange(previous => ({
            ...previous,
            collapsedGroups,
          }))}
        />
      )}
      {active && filesOpen && (
        <TerminalFilesPanel
          workspaceId={workspaceId}
          collapsed={false}
          width={filesDockState.width}
          pinned={filesPinned}
          canPin={false}
          panelId={filesPanelId}
          onTogglePin={toggleFilesPin}
          onClose={closeFiles}
          onWidthChange={width => setFilesDockState(previous => ({ ...previous, width }))}
          onOpenInFiles={onOpenInFiles}
          navigateRequest={filesNavigateRequest}
          onNavigateRequestHandled={handleFilesNavigateRequest}
        />
      )}
      <TerminalArea
        workspaceId={workspaceId}
        sidecarControls={sidecarControls}
        onOpenFilesAtPath={openFilesAtPath}
        workspaceActive={active}
      />
    </div>
  )
}

// Memoized so unrelated App state changes do not reconcile the terminal subtree.
// Shared Sessions presentation changes deliberately reach every mounted dock.
export default memo(TerminalWorkspaceDock)
import './TerminalWorkspaceDock.css'
