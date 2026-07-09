import { useState, useEffect, useCallback, useRef } from 'react'
import { useDraggable } from '@dnd-kit/core'
import type { TmuxSession, WorkspaceId } from '../types'
import { useSession } from '../context/SessionContext'
import { TERMINAL_LABELS, TERMINAL_WORKSPACE_IDS, WINDOW_COLORS, getSessionKey, getTerminalUserColor, getTerminalUserInitial } from '../types'
import { isFeatureEnabled } from '../featureFlags'
import { useViewportMenuPosition } from '../hooks/useViewportMenuPosition'
import RoleBadge from './RoleBadge'

interface SessionItemProps {
  session: TmuxSession
}

interface ContextMenuState {
  show: boolean
  x: number
  y: number
}

function SessionItem({ session }: SessionItemProps) {
  const { assignedSessions, handleSessionClick, deleteSession, renameSession, makeSessionPersistent, makeSessionMortal, workspaces, addSessionToWindow, removeSessionFromWindow, openFloatingModal, settings } = useSession()
  const sessionKey = getSessionKey(session.name, session.unixUser)
  const assignment = assignedSessions.get(sessionKey) ?? assignedSessions.get(session.name)
  const isAssigned = !!assignment
  const useLocationBadges = isFeatureEnabled('sessionLocationBadges')
  const [contextMenu, setContextMenu] = useState<ContextMenuState>({ show: false, x: 0, y: 0 })
  const contextMenuPosition = useViewportMenuPosition<HTMLDivElement>(
    contextMenu.show ? { x: contextMenu.x, y: contextMenu.y } : null,
    { estimatedSize: { width: 240, height: 360 } },
  )
  const [isRenaming, setIsRenaming] = useState(false)
  const [renameValue, setRenameValue] = useState('')
  const [showAssignSubmenu, setShowAssignSubmenu] = useState(false)
  const renameInputRef = useRef<HTMLInputElement>(null)

  const windowColor = assignment
    ? WINDOW_COLORS[assignment.colorIndex % WINDOW_COLORS.length].border
    : undefined

  const badgeStyle = assignment
    ? {
        backgroundColor: windowColor,
        color: '#000',
        borderColor: windowColor
      }
    : undefined

  // Color the session name text based on window assignment
  const nameStyle = assignment
    ? { color: windowColor }
    : undefined
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

  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({
    id: sessionKey,
    data: { type: 'session', session, sessionName: session.name, sessionKey, unixUser: session.unixUser },
  })

  const style = transform
    ? {
        transform: `translate3d(${transform.x}px, ${transform.y}px, 0)`,
        zIndex: isDragging ? 1000 : undefined,
      }
    : undefined

  // Implement Long Press detection
  const longPressTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const handleTouchStart = useCallback((e: React.TouchEvent) => {
    e.persist(); // Persist the event to use its properties in the timer callback
    longPressTimer.current = setTimeout(() => {
      if (longPressTimer.current) {
        // Create a synthetic MouseEvent based on the first touch point
        const touch = e.touches[0];
        setContextMenu({ show: true, x: touch.clientX, y: touch.clientY });
        setShowAssignSubmenu(false);
      }
    }, 500); // 500ms long press threshold
  }, []);

  const handleTouchEnd = useCallback(() => {
    // If released before 500ms, clear the timer
    if (longPressTimer.current) {
      clearTimeout(longPressTimer.current);
      longPressTimer.current = null;
    }
  }, []);

  const handleTouchMove = useCallback(() => {
    // Similarly, if moving, probably dragging/scrolling, so cancel long press
    if (longPressTimer.current) {
      clearTimeout(longPressTimer.current);
      longPressTimer.current = null;
    }
  }, []);

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
    await deleteSession(session.name, session.unixUser)
  }, [deleteSession, session.name, session.unixUser, closeContextMenu])


  const persistentTitle = session.persistent
    ? `Persistent ${session.persistentAgentKind || 'agent'} agent${session.persistentIdentity ? `: ${session.persistentIdentity}` : ''}`
    : undefined

  const handleMakePersistent = useCallback(async () => {
    closeContextMenu()
    const identity = window.prompt('One-sentence identity for this persistent agent:', session.persistentIdentity || '')
    if (identity === null) return
    await makeSessionPersistent(session.name, {
      identity: identity.trim(),
    }, session.unixUser)
  }, [closeContextMenu, makeSessionPersistent, session])

  const handleMakeMortal = useCallback(async () => {
    closeContextMenu()
    const confirmed = window.confirm(`Make ${session.name} mortal? CHROTE will stop supervising it, but the live tmux session stays running.`)
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
    const workspaceId = TERMINAL_WORKSPACE_IDS.find(wsId =>
      workspaces[wsId]?.windows.some(w => w.id === windowId)
    ) as WorkspaceId | undefined
    if (!workspaceId) return
    addSessionToWindow(workspaceId, windowId, session.name, session.unixUser)
    closeContextMenu()
  }, [addSessionToWindow, workspaces, session.name, session.unixUser, closeContextMenu])

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

  // Focus rename input when it appears
  useEffect(() => {
    if (isRenaming && renameInputRef.current) {
      renameInputRef.current.focus()
      renameInputRef.current.select()
    }
  }, [isRenaming])

  // Close context menu on click outside
  useEffect(() => {
    if (!contextMenu.show) return
    const handleClick = () => closeContextMenu()
    document.addEventListener('click', handleClick)
    return () => document.removeEventListener('click', handleClick)
  }, [contextMenu.show, closeContextMenu])

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

  return (
    <>
      <div
        ref={setNodeRef}
        className={`session-item ${isAssigned ? 'assigned' : ''} ${isDragging ? 'dragging' : ''}`}
        style={style}
        {...listeners}
        {...attributes}
        onClick={() => handleSessionClick(sessionKey)}
        onContextMenu={handleContextMenu}
        onTouchStart={handleTouchStart}
        onTouchEnd={handleTouchEnd}
        onTouchMove={handleTouchMove}
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
          <span
            className={`window-badge ${useLocationBadges ? 'window-location-chip' : ''}`}
            style={badgeStyle}
            title={useLocationBadges ? `Assigned to ${locationLabel}` : undefined}
          >
            {useLocationBadges
              ? locationLabel
              : assignment.workspaceId === 'terminal1'
                ? assignment.windowIndex
                : `${assignment.workspaceId.replace('terminal', '')}-${assignment.windowIndex}`}
          </span>
        )}
        <RoleBadge sessionName={session.name} />
        {session.persistent && (
          <span className="persistent-agent-lock" aria-label="Persistent agent" title={persistentTitle}>
            🔒
          </span>
        )}
        <span className="session-name" style={nameStyle}>{session.name}</span>
        {session.attached && !isAssigned && <span className="attached-indicator" title="Attached elsewhere">●</span>}
      </div>

      {contextMenu.show && (
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
          {session.persistent ? (
            <button className="session-context-item" onClick={handleMakeMortal}>
              <span className="session-context-icon">🔓</span>
              Make mortal
            </button>
          ) : (
            <button className="session-context-item" onClick={handleMakePersistent}>
              <span className="session-context-icon">🔒</span>
              Make persistent
            </button>
          )}

          <div
            className="session-context-item session-context-submenu-trigger"
            onMouseEnter={() => setShowAssignSubmenu(true)}
            onMouseLeave={() => setShowAssignSubmenu(false)}
          >
            <span className="session-context-icon">◫</span>
            Attach to Window
            <span className="session-context-arrow">▶</span>

            {showAssignSubmenu && (
              <div className="session-context-submenu">
                {TERMINAL_WORKSPACE_IDS.flatMap((wsId) => {
                  const ws = workspaces[wsId]
                  return ws.windows.slice(0, ws.windowCount).map((w, idx) => {
                    const color = WINDOW_COLORS[w.colorIndex % WINDOW_COLORS.length]
                    const isCurrentWindow = assignment?.windowId === w.id
                    const labelPrefix = wsId === 'terminal1' ? '' : `${TERMINAL_LABELS[wsId]} - `
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

          {!session.persistent && (
            <button className="session-context-item session-context-danger" onClick={handleDelete}>
              <span className="session-context-icon">✕</span>
              Kill Session
            </button>
          )}
        </div>
      )}
    </>
  )
}

export default SessionItem
