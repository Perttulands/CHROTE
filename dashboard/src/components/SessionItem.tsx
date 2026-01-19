import { useState, useEffect, useCallback, useRef } from 'react'
import { useDraggable } from '@dnd-kit/core'
import type { TmuxSession } from '../types'
import { useSession } from '../context/SessionContext'
import { WINDOW_COLORS } from '../types'
import type { WorkspaceId } from '../types'
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
  const { assignedSessions, handleSessionClick, deleteSession, renameSession, workspaces, addSessionToWindow, removeSessionFromWindow } = useSession()
  const assignment = assignedSessions.get(session.name)
  const isAssigned = !!assignment
  const [contextMenu, setContextMenu] = useState<ContextMenuState>({ show: false, x: 0, y: 0 })
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

  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({
    id: session.name,
    data: { type: 'session', session },
  })

  const style = transform
    ? {
        transform: `translate3d(${transform.x}px, ${transform.y}px, 0)`,
        zIndex: isDragging ? 1000 : undefined,
      }
    : undefined

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
    await deleteSession(session.name)
  }, [deleteSession, session.name, closeContextMenu])

  const handleStartRename = useCallback(() => {
    setRenameValue(session.name)
    setIsRenaming(true)
    closeContextMenu()
  }, [session.name, closeContextMenu])

  const handleRenameSubmit = useCallback(async () => {
    if (renameValue && renameValue !== session.name) {
      await renameSession(session.name, renameValue)
    }
    setIsRenaming(false)
  }, [renameValue, session.name, renameSession])

  const handleRenameKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      handleRenameSubmit()
    } else if (e.key === 'Escape') {
      setIsRenaming(false)
    }
  }, [handleRenameSubmit])

  const handleAssignToWindow = useCallback((windowId: string) => {
    const workspaceId = (windowId.startsWith('terminal2-') ? 'terminal2' : 'terminal1') as WorkspaceId
    addSessionToWindow(workspaceId, windowId, session.name)
    closeContextMenu()
  }, [addSessionToWindow, session.name, closeContextMenu])

  const handleUnassign = useCallback(() => {
    if (assignment) {
      removeSessionFromWindow(assignment.workspaceId, assignment.windowId, session.name)
    }
    closeContextMenu()
  }, [assignment, removeSessionFromWindow, session.name, closeContextMenu])

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
        onClick={() => handleSessionClick(session.name)}
        onContextMenu={handleContextMenu}
      >
        {assignment && (
          <span className="window-badge" style={badgeStyle}>
            {assignment.workspaceId === 'terminal2'
              ? `2-${assignment.windowIndex}`
              : assignment.windowIndex}
          </span>
        )}
        <RoleBadge sessionName={session.name} />
        <span className="session-name" style={nameStyle}>{session.name}</span>
        {session.attached && !isAssigned && <span className="attached-indicator" title="Attached elsewhere">●</span>}
      </div>

      {contextMenu.show && (
        <div
          className="session-context-menu"
          style={{ left: contextMenu.x, top: contextMenu.y }}
          onClick={e => e.stopPropagation()}
        >
          <button className="session-context-item" onClick={handleStartRename}>
            <span className="session-context-icon">✎</span>
            Rename
          </button>

          <div
            className="session-context-item session-context-submenu-trigger"
            onMouseEnter={() => setShowAssignSubmenu(true)}
            onMouseLeave={() => setShowAssignSubmenu(false)}
          >
            <span className="session-context-icon">◫</span>
            Assign to Window
            <span className="session-context-arrow">▶</span>

            {showAssignSubmenu && (
              <div className="session-context-submenu">
                {(['terminal1', 'terminal2'] as WorkspaceId[]).flatMap((wsId) => {
                  const ws = workspaces[wsId]
                  return ws.windows.slice(0, ws.windowCount).map((w, idx) => {
                    const color = WINDOW_COLORS[w.colorIndex % WINDOW_COLORS.length]
                    const isCurrentWindow = assignment?.windowId === w.id
                    const labelPrefix = wsId === 'terminal2' ? 'Terminal 2 - ' : ''
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
            Delete Session
          </button>
        </div>
      )}
    </>
  )
}

export default SessionItem
