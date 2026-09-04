import { useState, useEffect, useCallback, useMemo, useRef } from 'react'
import { useDraggable } from '@dnd-kit/core'
import type { TmuxSession } from '../types'
import { useSession } from '../context/SessionContext'
import { getSessionBadges, getSessionKey, getTerminalLabel, getTerminalUserInitial } from '../types'
import { SessionCommandMark, SessionLabel } from './sessionLabel'
import { identityColorFor } from '../theme/theme'
import { useTheme } from '../theme/ThemeContext'
import Menu, { type MenuAction, type MenuGroup } from './Menu'
import { harnessOfCommand, openAgentContext } from '../agents/agentContextPanel'
import { useAgentEventMarks } from '../agents/AgentEventsProvider'
import { summaryLine } from '../agents/agentEvents'

interface SessionItemProps {
  session: TmuxSession
}

interface ContextMenuState {
  show: boolean
  x: number
  y: number
}

function SessionItem({ session }: SessionItemProps) {
  const { assignedSessions, handleSessionClick, deleteSession, renameSession, workspaces, workspaceIds, focusedWindowKey, addSessionToWindow, removeSessionFromWindow, openFloatingModal, openSendToSession, terminalUsers } = useSession()
  const theme = useTheme()
  const sessionKey = getSessionKey(session.name, session.unixUser)
  // What the agent inside last reported, while it is news the operator has
  // not looked at: the row carries a dot and the report's first line.
  const mark = useAgentEventMarks().has(sessionKey) ? session.lastEvent : undefined
  const assignmentKey = assignedSessions.has(sessionKey) ? sessionKey : session.name
  const assignment = assignedSessions.get(assignmentKey)
  const isAssigned = !!assignment
  const [contextMenu, setContextMenu] = useState<ContextMenuState>({ show: false, x: 0, y: 0 })
  const [isRenaming, setIsRenaming] = useState(false)
  const [renameValue, setRenameValue] = useState('')
  const renameInputRef = useRef<HTMLInputElement>(null)


  // Where the operator is typing, said once, in the list he steers from: the
  // row of the session the focused tile is showing. No chip repeats the tile's
  // address back at him.
  const isInFocusedTile = useMemo(() => {
    if (!focusedWindowKey) return false
    return workspaceIds.some(wsId => workspaces[wsId]?.windows.some(
      w => `${wsId}-${w.id}` === focusedWindowKey && w.activeSession === sessionKey,
    ))
  }, [focusedWindowKey, sessionKey, workspaceIds, workspaces])

  const userBadgeColor = session.unixUser ? identityColorFor(session.unixUser, terminalUsers, theme) : undefined
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
  }, [])

  const closeContextMenu = useCallback(() => {
    setContextMenu({ show: false, x: 0, y: 0 })
  }, [])

  const handleDelete = useCallback(() => {
    void deleteSession(session.name, session.unixUser)
  }, [deleteSession, session.name, session.unixUser])


  const handleStartRename = useCallback(() => {
    setRenameValue(session.name)
    setIsRenaming(true)
  }, [session.name])

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
  }, [addSessionToWindow, workspaces, workspaceIds, session.name, session.unixUser])

  const handleUnassign = useCallback(() => {
    if (assignment) {
      removeSessionFromWindow(assignment.workspaceId, assignment.windowId, sessionKey)
    }
  }, [assignment, removeSessionFromWindow, sessionKey])

  const handlePeek = useCallback(() => {
    openFloatingModal(sessionKey)
  }, [openFloatingModal, sessionKey])

  const handleOpenSendToSession = useCallback(() => {
    openSendToSession({ targetSessionKey: sessionKey })
  }, [openSendToSession, sessionKey])

  // What this agent sees is asked of the session, so the folder and the harness
  // come from the session itself: its working directory and its pane command.
  const handleShowAgentContext = useCallback(() => {
    const { harness, shell } = harnessOfCommand(session.currentCommand)
    openAgentContext({
      sessionKey,
      folder: session.cwd ?? '',
      harness,
      user: session.unixUser ?? '',
      shell,
    })
  }, [session.currentCommand, session.cwd, session.unixUser, sessionKey])

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
  const badges = getSessionBadges(session)

  const windowRows: MenuAction[] = workspaceIds.flatMap(wsId => {
    const ws = workspaces[wsId]
    return ws.windows.slice(0, ws.windowCount).map((w, index): MenuAction => ({
      id: `${wsId}-${w.id}`,
      label: `${wsId === 'terminal1' ? '' : `${getTerminalLabel(wsId)} - `}Window ${index + 1}`,
      checked: assignment?.windowId === w.id,
      onSelect: () => handleAssignToWindow(w.id),
    }))
  })

  const menuGroups: MenuGroup[] = [
    {
      id: 'look',
      rows: [
        { id: 'peek', label: 'Peek', chord: 'Alt+P', onSelect: handlePeek },
        { id: 'send', label: 'Send to session', chord: 'Alt+S', onSelect: handleOpenSendToSession },
        { id: 'agent-context', label: 'What this agent sees', onSelect: handleShowAgentContext },
      ],
    },
    {
      id: 'place',
      rows: [
        { id: 'attach', label: 'Attach to window', submenu: windowRows },
        ...(isAssigned ? [{ id: 'unassign', label: 'Unassign', onSelect: handleUnassign }] : []),
        { id: 'rename', label: 'Rename', onSelect: handleStartRename },
      ],
    },
    {
      id: 'end',
      rows: [
        { id: 'kill', label: 'Kill session', danger: true, confirmLabel: 'Confirm kill', onSelect: handleDelete },
      ],
    },
  ]

  return (
    <>
      <div
        ref={setNodeRef}
        className={`session-item ${isAssigned ? 'assigned' : ''} ${isInFocusedTile ? 'in-focused-tile' : ''} ${isDragging ? 'dragging' : ''} ${mark ? 'has-event' : ''}`}
        data-ui="session.row"
        data-event={mark?.event}
        style={style}
        title={dragLabel}
        aria-current={isInFocusedTile ? 'true' : undefined}
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
        <SessionCommandMark command={session.currentCommand} />
        {mark && (
          <span
            className="session-event-mark"
            role="img"
            aria-label={mark.event === 'finished' ? 'Finished' : 'Needs input'}
          />
        )}
        <span className="session-item-text">
          <SessionLabel name={session.name} className="session-name" />
          {mark?.summary && <span className="session-event-summary">{summaryLine(mark.summary)}</span>}
        </span>
        {badges.length > 0 && (
          <span className="session-badges">
            {badges.map(badge => (
              <span
                key={badge.id}
                className="session-badge"
                data-badge={badge.id}
                title={`${badge.label}: ${badge.detail}`}
                aria-label={`${badge.label}: ${badge.detail}`}
                role="img"
              >
                {badge.marker}
              </span>
            ))}
          </span>
        )}
        <button
          type="button"
          className="session-item-menu-btn"
          aria-label={`Session actions for ${session.name}`}
          onPointerDown={event => event.stopPropagation()}
          onClick={(event) => {
            event.preventDefault()
            event.stopPropagation()
            const rect = event.currentTarget.getBoundingClientRect()
            setContextMenu({ show: true, x: rect.right, y: rect.bottom })
          }}
        >
          ⋯
        </button>
      </div>

      {contextMenu.show && (
        <Menu
          at={{ x: contextMenu.x, y: contextMenu.y }}
          label={`Session actions for ${session.name}`}
          estimatedSize={{ width: 240, height: 240 }}
          onClose={closeContextMenu}
          groups={menuGroups}
        />
      )}
    </>
  )
}

export default SessionItem
