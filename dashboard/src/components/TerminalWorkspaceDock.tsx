import { memo, useCallback, useEffect, useRef, useState } from 'react'
import type { Dispatch, SetStateAction } from 'react'
import { FolderTree, SquareTerminal } from 'lucide-react'
import type { WorkspaceId } from '../types'
import { useSession } from '../context/SessionContext'
import { useMediaQuery } from '../hooks/useMediaQuery'
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
  onOpenSessionBankSettings: () => void
  onOpenInFiles: (path: string) => void
}

const ESCAPE_BLOCKERS = '.floating-modal, .file-peek, .session-context-menu, .send-session-modal, [role="dialog"]'

function isVisibleEscapeBlocker(element: Element): boolean {
  if (!(element instanceof HTMLElement)) return false

  for (let current: HTMLElement | null = element; current; current = current.parentElement) {
    const style = window.getComputedStyle(current)
    if (
      current.hidden
      || current.hasAttribute('inert')
      || current.getAttribute('aria-hidden') === 'true'
      || style.display === 'none'
      || style.visibility === 'hidden'
      || style.visibility === 'collapse'
      || style.opacity === '0'
    ) return false
  }

  return element.getClientRects().length > 0
}

function hasVisibleEscapeBlocker(): boolean {
  return Array.from(document.querySelectorAll(ESCAPE_BLOCKERS)).some(isVisibleEscapeBlocker)
}

function TerminalWorkspaceDock({
  workspaceId,
  active,
  sessionsDockState,
  onSessionsDockStateChange,
  onFilesOpenChange,
  onOpenSessionBankSettings,
  onOpenInFiles,
}: TerminalWorkspaceDockProps) {
  const { sessions } = useSession()
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

  const toggleFiles = useCallback(() => {
    setFilesDockState(previous => ({ ...previous, open: !previous.open }))
  }, [])

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

  const closeAllSidecars = useCallback(() => {
    closeSessions()
    closeFiles()
  }, [closeFiles, closeSessions])

  const toggleSessionsPin = useCallback(() => {
    if (isNarrow) return
    onSessionsDockStateChange(previous => ({ ...previous, pinned: !previous.pinned }))
  }, [isNarrow, onSessionsDockStateChange])

  const toggleFilesPin = useCallback(() => {
    if (isNarrow) return
    setFilesDockState(previous => ({ ...previous, pinned: !previous.pinned }))
  }, [isNarrow])

  useEffect(() => {
    if (!active || openSidecarCount === 0 || anyPinned) return
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key !== 'Escape' || event.defaultPrevented) return
      if (hasVisibleEscapeBlocker()) return
      closeAllSidecars()
    }
    window.addEventListener('keydown', closeOnEscape)
    return () => window.removeEventListener('keydown', closeOnEscape)
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
          onOpenSessionBankSettings={onOpenSessionBankSettings}
          collapsed={false}
          width={sessionsDockState.width}
          pinned={sessionsPinned}
          canPin={false}
          panelId={sessionsPanelId}
          onTogglePin={toggleSessionsPin}
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
        />
      )}
      <TerminalArea
        workspaceId={workspaceId}
        sidecarControls={sidecarControls}
        onOpenFilesAtPath={openFilesAtPath}
      />
    </div>
  )
}

// Memoized so unrelated App state changes do not reconcile the terminal subtree.
// Shared Sessions presentation changes deliberately reach every mounted dock.
export default memo(TerminalWorkspaceDock)
