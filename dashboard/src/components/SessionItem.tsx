import { useState, useEffect, useCallback, useRef } from 'react'
import { useDraggable } from '@dnd-kit/core'
import type { PersistentAgentHealth, PersistentAgentPayload, TmuxSession } from '../types'
import { useSession } from '../context/SessionContext'
import { WINDOW_COLORS, getSessionKey, getTerminalLabel, getTerminalUserColor, getTerminalUserInitial } from '../types'
import { useViewportMenuPosition } from '../hooks/useViewportMenuPosition'
import DismissiblePanel from './DismissiblePanel'

interface SessionItemProps {
  session: TmuxSession
}

interface ContextMenuState {
  show: boolean
  x: number
  y: number
}

// Unit facts, phrased for someone glancing at a session list. "degraded" is
// deliberately not "unhealthy": the agent is very likely running, we just cannot
// prove it is the transcript this lock was made for.
const PERSISTENT_HEALTH_LABELS: Record<PersistentAgentHealth, string> = {
  healthy: '',
  degraded: 'unconfirmed',
  failed: 'failed',
  inactive: 'stopped',
  unlocked: '',
}

function persistentAgentKind(session: TmuxSession): string {
  return session.persistentAgentKind || 'agent'
}

function persistentAgentSessionId(session: TmuxSession): string {
  return session.persistentAgentSessionId || ''
}

function persistentHermesProfile(session: TmuxSession): string {
  return session.persistentHermesProfile || ''
}

function persistentHealthLabel(session: TmuxSession): string {
  const health = session.persistentHealth
  return health ? PERSISTENT_HEALTH_LABELS[health] ?? '' : ''
}

function persistentTitle(session: TmuxSession): string | undefined {
  if (!session.persistent) return undefined
  const parts = [`Locked ${persistentAgentKind(session)} agent, supervised by systemd`]
  const hermesProfile = persistentHermesProfile(session)
  if (hermesProfile) parts.push(`Hermes profile ${hermesProfile}`)
  if (session.persistentUnit) parts.push(session.persistentUnit)
  if (session.persistentActiveState) parts.push(`unit ${session.persistentActiveState}`)
  if (session.persistentDetail) parts.push(session.persistentDetail)
  const title = parts.join(' · ')
  return session.persistentIdentity ? `${title}: ${session.persistentIdentity}` : title
}

function persistentPrompt(session: TmuxSession): string {
  const parts = []
  const kind = persistentAgentKind(session)
  if (kind !== 'agent') parts.push(kind)
  const hermesProfile = persistentHermesProfile(session)
  if (hermesProfile) parts.push(`Hermes profile ${hermesProfile}`)
  const sessionId = persistentAgentSessionId(session)
  if (sessionId) parts.push(sessionId)
  if (parts.length === 0) return 'One-sentence identity for this persistent agent:'
  return `One-sentence identity for this persistent agent (${parts.join(' · ')}):`
}

function SessionItem({ session }: SessionItemProps) {
  const { assignedSessions, handleSessionClick, focusSessionAssignment, deleteSession, renameSession, makeSessionPersistent, makeSessionMortal, workspaces, workspaceIds, addSessionToWindow, removeSessionFromWindow, openFloatingModal, openSendToSession, settings } = useSession()
  const sessionKey = getSessionKey(session.name, session.unixUser)
  const assignmentKey = assignedSessions.has(sessionKey) ? sessionKey : session.name
  const assignment = assignedSessions.get(assignmentKey)
  const isAssigned = !!assignment
  const [contextMenu, setContextMenu] = useState<ContextMenuState>({ show: false, x: 0, y: 0 })
  const contextMenuPosition = useViewportMenuPosition<HTMLDivElement>(
    contextMenu.show ? { x: contextMenu.x, y: contextMenu.y } : null,
    { estimatedSize: { width: 240, height: 360 } },
  )
  const [isRenaming, setIsRenaming] = useState(false)
  const [renameValue, setRenameValue] = useState('')
  const [showAssignSubmenu, setShowAssignSubmenu] = useState(false)
  const renameInputRef = useRef<HTMLInputElement>(null)


  const locationLabel = assignment
    ? `${assignment.workspaceId.replace('terminal', 'T')} W${assignment.windowIndex}`
    : ''
  const userBadgeColor = session.unixUser ? getTerminalUserColor(settings, session.unixUser) : undefined
  const userBadgeStyle = userBadgeColor
    ? {
        backgroundColor: userBadgeColor,
        borderColor: userBadgeColor,
      }
    : undefined

  const { listeners, setNodeRef, isDragging } = useDraggable({
    id: sessionKey,
    data: { type: 'session', session, sessionName: session.name, sessionKey, unixUser: session.unixUser },
  })

  const style = isDragging
    ? { opacity: 0, transition: 'none' }
    : undefined

  // Implement long-press detection and arbitrate it against dnd-kit's pointer sensor.
  const longPressTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const pendingTouchPointer = useRef<{ pointerId: number; ownerDocument: Document } | null>(null)

  const clearLongPressTimer = useCallback(() => {
    if (longPressTimer.current !== null) {
      clearTimeout(longPressTimer.current)
      longPressTimer.current = null
    }
  }, [])

  const cancelPendingTouchDrag = useCallback(() => {
    const pending = pendingTouchPointer.current
    if (!pending) return
    pendingTouchPointer.current = null
    const EventConstructor = pending.ownerDocument.defaultView?.Event ?? Event
    const cancelEvent = new EventConstructor('pointercancel', { bubbles: true, cancelable: true })
    Object.defineProperties(cancelEvent, {
      pointerId: { value: pending.pointerId },
      pointerType: { value: 'touch' },
      isPrimary: { value: true },
    })
    pending.ownerDocument.dispatchEvent(cancelEvent)
  }, [])

  const handlePointerDown = useCallback((event: React.PointerEvent<HTMLDivElement>) => {
    if (event.pointerType === 'touch') {
      pendingTouchPointer.current = {
        pointerId: event.pointerId,
        ownerDocument: event.currentTarget.ownerDocument,
      }
    } else {
      pendingTouchPointer.current = null
    }
    listeners?.onPointerDown?.(event)
  }, [listeners])

  const clearPendingTouchGesture = useCallback(() => {
    clearLongPressTimer()
    pendingTouchPointer.current = null
  }, [clearLongPressTimer])

  const handleTouchStart = useCallback((e: React.TouchEvent) => {
    clearLongPressTimer()
    const touch = e.touches[0]
    if (!touch) return
    const { clientX, clientY } = touch
    longPressTimer.current = setTimeout(() => {
      longPressTimer.current = null
      cancelPendingTouchDrag()
      setContextMenu({ show: true, x: clientX, y: clientY })
      setShowAssignSubmenu(false)
    }, 500)
  }, [cancelPendingTouchDrag, clearLongPressTimer])

  useEffect(() => {
    if (isDragging) clearPendingTouchGesture()
  }, [clearPendingTouchGesture, isDragging])

  useEffect(() => () => {
    clearLongPressTimer()
    pendingTouchPointer.current = null
  }, [clearLongPressTimer])

  const handleContextMenu = useCallback((e: React.MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()
    setContextMenu({ show: true, x: e.clientX, y: e.clientY })
    setShowAssignSubmenu(false)
  }, [])

  const closeContextMenu = useCallback(() => {
    setContextMenu({ show: false, x: 0, y: 0 })
    setShowAssignSubmenu(false)
  }, [])

  const handleDelete = useCallback(async () => {
    closeContextMenu()
    if (session.persistent) {
      // Killing a locked session without unlocking it first would be undone by
      // its unit within seconds. The old UI hid the button rather than explain
      // that; do the two steps instead, in the order that actually works.
      const confirmed = window.confirm(
        `${session.name} is locked. Stop its supervising unit and kill the session? The agent will not come back.`
      )
      if (!confirmed) return
      await makeSessionMortal(session.name, session.unixUser)
    }
    await deleteSession(session.name, session.unixUser)
  }, [deleteSession, makeSessionMortal, session.name, session.persistent, session.unixUser, closeContextMenu])


  const persistentAgentTitle = persistentTitle(session)
  const persistentStatusLabel = persistentHealthLabel(session)

  const handleMakePersistent = useCallback(async () => {
    closeContextMenu()
    const identity = window.prompt(persistentPrompt(session), session.persistentIdentity || '')
    if (identity === null) return
    const payload: PersistentAgentPayload = {
      identity: identity.trim(),
    }
    const agentKind = persistentAgentKind(session)
    if (agentKind && agentKind !== 'agent') payload.agentKind = agentKind
    const agentSessionId = persistentAgentSessionId(session)
    if (agentSessionId) payload.agentSessionId = agentSessionId
    await makeSessionPersistent(session.name, payload, session.unixUser)
  }, [closeContextMenu, makeSessionPersistent, session])

  const handleMakeMortal = useCallback(async () => {
    closeContextMenu()
    const confirmed = window.confirm(`Unlock ${session.name}? This stops its supervising unit, so the agent will no longer be restarted after a crash or reboot. The running session and its agent are left alone.`)
    if (!confirmed) return
    await makeSessionMortal(session.name, session.unixUser)
  }, [closeContextMenu, makeSessionMortal, session.name, session.unixUser])


  const handleStartRename = useCallback(() => {
    setRenameValue(session.name)
    setIsRenaming(true)
    closeContextMenu()
  }, [session.name, closeContextMenu])

  const handleRenameSubmit = useCallback(async () => {
    if (renameValue && renameValue !== session.name) {
      await renameSession(session.name, renameValue, session.unixUser)
    }
    setIsRenaming(false)
  }, [renameValue, session.name, session.unixUser, renameSession])

  const handleRenameKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      handleRenameSubmit()
    } else if (e.key === 'Escape') {
      setIsRenaming(false)
    }
  }, [handleRenameSubmit])

  const handleAssignToWindow = useCallback((windowId: string) => {
    const workspaceId = workspaceIds.find(wsId =>
      workspaces[wsId]?.windows.some(w => w.id === windowId)
    )
    if (!workspaceId) return
    addSessionToWindow(workspaceId, windowId, session.name, session.unixUser)
    closeContextMenu()
  }, [addSessionToWindow, workspaces, workspaceIds, session.name, session.unixUser, closeContextMenu])

  const handleUnassign = useCallback(() => {
    if (assignment) {
      removeSessionFromWindow(assignment.workspaceId, assignment.windowId, sessionKey)
    }
    closeContextMenu()
  }, [assignment, removeSessionFromWindow, sessionKey, closeContextMenu])

  const handlePeek = useCallback(() => {
    openFloatingModal(sessionKey)
    closeContextMenu()
  }, [openFloatingModal, sessionKey, closeContextMenu])

  const handleOpenSendToSession = useCallback(() => {
    openSendToSession(sessionKey)
    closeContextMenu()
  }, [openSendToSession, sessionKey, closeContextMenu])

  const handleClick = useCallback(() => {
    handleSessionClick(sessionKey)
  }, [handleSessionClick, sessionKey])

  // Focus rename input when it appears
  useEffect(() => {
    if (isRenaming && renameInputRef.current) {
      renameInputRef.current.focus()
      renameInputRef.current.select()
    }
  }, [isRenaming])

  // Rename mode
  if (isRenaming) {
    return (
      <div className="session-item renaming">
        <input
          ref={renameInputRef}
          type="text"
          className="session-rename-input"
          value={renameValue}
          onChange={e => setRenameValue(e.target.value)}
          onKeyDown={handleRenameKeyDown}
          onBlur={handleRenameSubmit}
        />
      </div>
    )
  }

  const dragLabel = `Drag ${session.name}${session.unixUser ? ` (Unix user ${session.unixUser})` : ''}`

  return (
    <>
      <div
        ref={setNodeRef}
        className={`session-item ${isAssigned ? 'assigned' : ''} ${isDragging ? 'dragging' : ''}`}
        style={style}
        title={dragLabel}
        {...listeners}
        onPointerDown={handlePointerDown}
        onPointerUp={clearPendingTouchGesture}
        onPointerCancel={clearPendingTouchGesture}
        onClick={handleClick}
        onContextMenu={handleContextMenu}
        onTouchStart={handleTouchStart}
        onTouchEnd={clearPendingTouchGesture}
        onTouchMove={clearLongPressTimer}
        onTouchCancel={clearPendingTouchGesture}
      >
        {session.unixUser && (
          <span
            className="unix-user-badge"
            style={userBadgeStyle}
            title={`Unix user: ${session.unixUser}`}
            aria-label={`Unix user ${session.unixUser}`}
          >
            {getTerminalUserInitial(session.unixUser)}
          </span>
        )}
        {assignment && (
          <button
            type="button"
            className="window-badge window-location-chip"
            title={`Focus ${locationLabel}`}
            aria-label={`Focus assigned window ${locationLabel}`}
            onClick={(event) => {
              event.preventDefault()
              event.stopPropagation()
              focusSessionAssignment(assignmentKey)
            }}
          >
            {locationLabel}
          </button>
        )}
        {session.persistent && (
          <span className="persistent-agent-lock" aria-label="Persistent agent" title={persistentAgentTitle}>
            🔒
          </span>
        )}
        {session.persistent && persistentStatusLabel && (
          <span
            className={`persistent-agent-state persistent-agent-state-${session.persistentHealth}`}
            aria-label={`Supervision: ${persistentStatusLabel}`}
            title={session.persistentDetail || persistentStatusLabel}
          >
            {persistentStatusLabel}
          </span>
        )}
        <span className="session-name">{session.name}</span>
        <button
          type="button"
          className="session-item-menu-btn"
          aria-label={`Session actions for ${session.name}`}
          onPointerDown={event => event.stopPropagation()}
          onClick={(event) => {
            event.preventDefault()
            event.stopPropagation()
            const rect = event.currentTarget.getBoundingClientRect()
            setContextMenu({ show: true, x: rect.right, y: rect.bottom + 4 })
          }}
        >
          ⋯
        </button>
      </div>

      {contextMenu.show && (
        <DismissiblePanel onDismiss={closeContextMenu} panelPosition="fixed">
          <div
            ref={contextMenuPosition.ref}
            className="session-context-menu"
          style={contextMenuPosition.style}
          onClick={e => e.stopPropagation()}
        >
          <button className="session-context-item" onClick={handleStartRename}>
            <span className="session-context-icon">✎</span>
            Rename
          </button>
          <button className="session-context-item" onClick={handlePeek}>
            <span className="session-context-icon">◉</span>
            Peek
          </button>
          <button className="session-context-item" onClick={handleOpenSendToSession}>
            <span className="session-context-icon">↗</span>
            Send to Session
          </button>
          {session.persistent ? (
            <button className="session-context-item" onClick={handleMakeMortal}>
              <span className="session-context-icon">🔓</span>
              Make mortal (metadata only)
            </button>
          ) : (
            <button className="session-context-item" onClick={handleMakePersistent}>
              <span className="session-context-icon">🔒</span>
              Make persistent
            </button>
          )}

          <div
            className="session-context-submenu-trigger"
            onClick={(event) => event.stopPropagation()}
          >
            <button
              className="session-context-item"
              aria-expanded={showAssignSubmenu}
              onClick={() => setShowAssignSubmenu(open => !open)}
            >
              <span className="session-context-icon">◫</span>
              Attach to Window
              <span className="session-context-arrow">▶</span>
            </button>

            {showAssignSubmenu && (
              <div className="session-context-submenu">
                {workspaceIds.flatMap((wsId) => {
                  const ws = workspaces[wsId]
                  return ws.windows.slice(0, ws.windowCount).map((w, idx) => {
                    const color = WINDOW_COLORS[w.colorIndex % WINDOW_COLORS.length]
                    const isCurrentWindow = assignment?.windowId === w.id
                    const labelPrefix = wsId === 'terminal1' ? '' : `${getTerminalLabel(wsId)} - `
                    return (
                      <button
                        key={w.id}
                        className={`session-context-item ${isCurrentWindow ? 'active' : ''}`}
                        onClick={() => handleAssignToWindow(w.id)}
                        style={{ borderLeft: `3px solid ${color.border}` }}
                      >
                        {labelPrefix}Window {idx + 1}
                        {isCurrentWindow && <span className="session-context-check">✓</span>}
                      </button>
                    )
                  })
                })}
              </div>
            )}
          </div>

          {isAssigned && (
            <button className="session-context-item" onClick={handleUnassign}>
              <span className="session-context-icon">⊘</span>
              Unassign
            </button>
          )}

          <div className="session-context-divider" />

          <button className="session-context-item session-context-danger" onClick={handleDelete}>
            <span className="session-context-icon">✕</span>
            {session.persistent ? 'Stop supervision and kill' : 'Kill Session'}
          </button>
          </div>
        </DismissiblePanel>
      )}
    </>
  )
}

export default SessionItem
