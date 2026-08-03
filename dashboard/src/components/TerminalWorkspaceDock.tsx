import { memo, useCallback, useEffect, useState } from 'react'
import { FolderTree, SquareTerminal } from 'lucide-react'
import type { WorkspaceId } from '../types'
import { useSession } from '../context/SessionContext'
import { useMediaQuery } from '../hooks/useMediaQuery'
import SessionPanel from './SessionPanel'
import TerminalArea from './TerminalArea'
import TerminalFilesPanel from './TerminalFilesPanel'
import {
  readWorkspaceDockState,
  writeWorkspaceDockState,
  type WorkspaceDockState,
  type WorkspaceSidecar,
} from './workspaceFilesState'

interface TerminalWorkspaceDockProps {
  workspaceId: WorkspaceId
  active: boolean
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
  onOpenSessionBankSettings,
  onOpenInFiles,
}: TerminalWorkspaceDockProps) {
  const { sessions } = useSession()
  const isNarrow = useMediaQuery('(max-width: 768px)')
  const [dockState, setDockState] = useState<WorkspaceDockState>(() => readWorkspaceDockState(workspaceId))
  const sessionsOpen = dockState.openSidecars.includes('sessions')
  const filesOpen = dockState.openSidecars.includes('files')
  // Two panels need a rail so they remain usable instead of occupying the
  // same overlay position. On narrow screens the existing overlay behavior is
  // retained.
  const forcedPinned = dockState.openSidecars.length > 1 && !isNarrow
  const effectivePinned = (dockState.sidecarPinned || forcedPinned) && !isNarrow
  const sessionsPanelId = `${workspaceId}-sessions-sidecar`
  const filesPanelId = `${workspaceId}-files-sidecar`

  const updateDockState = useCallback((update: (previous: WorkspaceDockState) => WorkspaceDockState) => {
    setDockState(previous => update(previous))
  }, [])

  useEffect(() => {
    writeWorkspaceDockState(workspaceId, dockState)
  }, [dockState, workspaceId])

  // Closing keeps sidecarPinned so a pinned panel reopens pinned beside the
  // terminal instead of overlaying it.
  const toggleSidecar = useCallback((sidecar: WorkspaceSidecar) => {
    updateDockState(previous => previous.openSidecars.includes(sidecar)
      ? { ...previous, openSidecars: previous.openSidecars.filter(openSidecar => openSidecar !== sidecar) }
      : { ...previous, openSidecars: [...previous.openSidecars, sidecar] })
  }, [updateDockState])

  const closeSidecar = useCallback((sidecar: WorkspaceSidecar) => {
    updateDockState(previous => ({
      ...previous,
      openSidecars: previous.openSidecars.filter(openSidecar => openSidecar !== sidecar),
    }))
  }, [updateDockState])

  const closeAllSidecars = useCallback(() => {
    updateDockState(previous => ({ ...previous, openSidecars: [] }))
  }, [updateDockState])

  const togglePin = useCallback(() => {
    if (isNarrow) return
    updateDockState(previous => previous.openSidecars.length === 0 || previous.openSidecars.length > 1
      ? previous
      : { ...previous, sidecarPinned: !previous.sidecarPinned })
  }, [isNarrow, updateDockState])

  useEffect(() => {
    if (!active || dockState.openSidecars.length === 0 || effectivePinned) return
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key !== 'Escape' || event.defaultPrevented) return
      if (hasVisibleEscapeBlocker()) return
      closeAllSidecars()
    }
    window.addEventListener('keydown', closeOnEscape)
    return () => window.removeEventListener('keydown', closeOnEscape)
  }, [active, closeAllSidecars, dockState.openSidecars.length, effectivePinned])

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
        onClick={() => toggleSidecar('sessions')}
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
        onClick={() => toggleSidecar('files')}
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
      data-sidecar={dockState.openSidecars.join(',') || 'closed'}
      data-sidecar-count={dockState.openSidecars.length}
      data-sidecar-pinned={effectivePinned}
      style={{ display: active ? 'flex' : 'none' }}
    >
      {active && dockState.openSidecars.length > 0 && !effectivePinned && (
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
          width={dockState.sessionsWidth}
          pinned={effectivePinned}
          canPin={!isNarrow && !forcedPinned}
          panelId={sessionsPanelId}
          onTogglePin={togglePin}
          onClose={() => closeSidecar('sessions')}
          onWidthChange={sessionsWidth => updateDockState(previous => ({ ...previous, sessionsWidth }))}
        />
      )}
      {active && filesOpen && (
        <TerminalFilesPanel
          workspaceId={workspaceId}
          collapsed={false}
          width={dockState.filesWidth}
          pinned={effectivePinned}
          canPin={!isNarrow && !forcedPinned}
          panelId={filesPanelId}
          onTogglePin={togglePin}
          onClose={() => closeSidecar('files')}
          onWidthChange={filesWidth => updateDockState(previous => ({ ...previous, filesWidth }))}
          onOpenInFiles={onOpenInFiles}
        />
      )}
      <TerminalArea workspaceId={workspaceId} sidecarControls={sidecarControls} />
    </div>
  )
}

// Memoized so App-level drag state changes don't reconcile the whole terminal
// subtree; all props are primitives or stable useCallback references.
export default memo(TerminalWorkspaceDock)
