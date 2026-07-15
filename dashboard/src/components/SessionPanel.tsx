import { useEffect, useMemo, useRef, useState } from 'react'
import type { CSSProperties, PointerEvent as ReactPointerEvent } from 'react'
import { useSession } from '../context/SessionContext'
import { getDefaultLaunchUser, getGroupPriority, getTerminalUserInitial } from '../types'
import type { WorkspaceId } from '../types'
import { useViewportMenuPosition } from '../hooks/useViewportMenuPosition'
import { useMediaQuery } from '../hooks/useMediaQuery'
import SessionGroup from './SessionGroup'
import DockPanelToggle from './DockPanelToggle'
import DismissiblePanel from './DismissiblePanel'
import { summarizeSessionBankCapabilities } from '../sessionBankRecovery'

type SessionPanelProps = {
  onOpenSessionBankSettings?: () => void
  activeWorkspaceId: WorkspaceId
  collapsed?: boolean
  width?: number
  onToggle?: () => void
  onWidthChange?: (width: number) => void
}

function SessionPanel({
  onOpenSessionBankSettings,
  activeWorkspaceId,
  collapsed,
  width = 260,
  onToggle,
  onWidthChange,
}: SessionPanelProps) {
  const { groupedSessions, loading, error, sidebarCollapsed, toggleSidebar, refreshSessions, createSession: createSessionAction, sessionBank, terminalUsers } = useSession()
  const isCollapsed = collapsed ?? sidebarCollapsed
  const handleToggle = onToggle ?? toggleSidebar
  const [creating, setCreating] = useState(false)
  const [searchTerm, setSearchTerm] = useState('')
  const [newSessionMenu, setNewSessionMenu] = useState<{ show: boolean; x: number; y: number }>({ show: false, x: 0, y: 0 })
  const [namedSessionPopup, setNamedSessionPopup] = useState<{ show: boolean; x: number; y: number }>({ show: false, x: 0, y: 0 })
  const [namedSessionName, setNamedSessionName] = useState('')
  const [namedSessionUser, setNamedSessionUser] = useState('')
  const isMobile = useMediaQuery('(max-width: 768px)')
  const autoCollapsedForMobile = useRef(false)
  const newSessionMenuPosition = useViewportMenuPosition<HTMLDivElement>(
    newSessionMenu.show ? { x: newSessionMenu.x, y: newSessionMenu.y } : null,
    { estimatedSize: { width: 190, height: 130 } },
  )
  const namedSessionPopupPosition = useViewportMenuPosition<HTMLDivElement>(
    namedSessionPopup.show ? { x: namedSessionPopup.x, y: namedSessionPopup.y } : null,
    { estimatedSize: { width: 240, height: 180 } },
  )

  // Sort groups by priority and filter by search
  const sortedGroups = useMemo(() => {
    // Determine which groups/sessions to show
    const entries = Object.entries(groupedSessions)

    // Filter
    const filtered = searchTerm
      ? entries.map(([key, sessions]) => ([
          key,
          sessions.filter(s =>
            s.name.toLowerCase().includes(searchTerm.toLowerCase())
          )
        ] as [string, typeof sessions])).filter(([_, sessions]) => sessions.length > 0)
      : entries

    return filtered.sort(([a], [b]) => {
      const priorityA = getGroupPriority(a)
      const priorityB = getGroupPriority(b)
      if (priorityA !== priorityB) return priorityA - priorityB
      return a.localeCompare(b)
    })
  }, [groupedSessions, searchTerm])

  const bankedSessionSummary = useMemo(() => (
    summarizeSessionBankCapabilities(sessionBank.filter(session => !session.live))
  ), [sessionBank])

  useEffect(() => {
    if (!isMobile) {
      autoCollapsedForMobile.current = false
      return
    }
    if (autoCollapsedForMobile.current) return
    autoCollapsedForMobile.current = true
    if (!isCollapsed) handleToggle()
  }, [handleToggle, isCollapsed, isMobile])
  const closeNewSessionMenu = () => setNewSessionMenu({ show: false, x: 0, y: 0 })

  const createSessionForUser = async (unixUser?: string, explicitName?: string) => {
    closeNewSessionMenu()
    setCreating(true)
    try {
      const created = await createSessionAction({
        workspaceId: activeWorkspaceId,
        ...(unixUser !== undefined ? { unixUser } : {}),
        ...(explicitName !== undefined ? { name: explicitName } : {}),
      })
      if (created) {
        setNamedSessionName('')
        setNamedSessionPopup({ show: false, x: 0, y: 0 })
      }
    } finally {
      setCreating(false)
      closeNewSessionMenu()
    }
  }

  const createSession = async () => {
    await createSessionForUser()
  }

  const openNamedSessionField = () => {
    const unixUser = getDefaultLaunchUser(activeWorkspaceId, terminalUsers)
    setNamedSessionUser(unixUser)
    setNamedSessionPopup({ show: true, x: newSessionMenu.x, y: newSessionMenu.y })
    closeNewSessionMenu()
  }

  const submitNamedSession = async () => {
    const unixUser = namedSessionUser || getDefaultLaunchUser(activeWorkspaceId, terminalUsers)
    if (!namedSessionName.trim()) return
    await createSessionForUser(unixUser, namedSessionName)
  }

  const startPanelResize = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (!onWidthChange) return
    event.currentTarget.setPointerCapture(event.pointerId)
    const startX = event.clientX
    const startWidth = width
    const pointerId = event.pointerId
    const move = (moveEvent: PointerEvent) => {
      if (moveEvent.pointerId === pointerId) onWidthChange(Math.min(480, Math.max(220, startWidth + moveEvent.clientX - startX)))
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

  const panelStyle = isCollapsed ? undefined : ({ '--session-panel-width': `${width}px` } as CSSProperties)

  return (
    <div className={`session-panel dock-panel ${isCollapsed ? 'collapsed' : ''}`} style={panelStyle}>
      <div className="session-panel-header dock-panel-header">
        <DockPanelToggle label="Sessions" collapsed={isCollapsed} onToggle={handleToggle} />
        {!isCollapsed && (
          <>
            <button
              className="add-btn"
              onClick={createSession}
              disabled={creating}
              title="New tmux session"
            >
              +
            </button>
            <button
              className="add-btn new-session-options-btn"
              aria-label="Session creation options"
              title="Session creation options"
              onClick={(event) => {
                const rect = event.currentTarget.getBoundingClientRect()
                setNamedSessionPopup({ show: false, x: 0, y: 0 })
                setNewSessionMenu({ show: true, x: rect.left, y: rect.bottom + 4 })
              }}
            >
              ▾
            </button>
            <button className="refresh-btn" onClick={refreshSessions} title="Refresh sessions">
              ↻
            </button>
          </>
        )}
      </div>

      {newSessionMenu.show && (
        <DismissiblePanel onDismiss={closeNewSessionMenu} panelPosition="fixed">
          <div
            ref={newSessionMenuPosition.ref}
            className="session-context-menu"
            style={newSessionMenuPosition.style}
          >
            {terminalUsers.map(user => (
              <button
                key={user}
                className="session-context-item"
                onClick={() => createSessionForUser(user)}
                disabled={creating}
              >
                <span className="session-context-icon">{getTerminalUserInitial(user)}</span>
                New as {getTerminalUserInitial(user)} {user}
              </button>
            ))}
            <div className="session-context-divider" />
            <button className="session-context-item" onClick={openNamedSessionField}>
              <span className="session-context-icon">✎</span>
              New named session
            </button>
          </div>
        </DismissiblePanel>
      )}

      {!isCollapsed && namedSessionPopup.show && (
        <DismissiblePanel onDismiss={() => setNamedSessionPopup({ show: false, x: 0, y: 0 })} panelPosition="fixed">
          <div
            ref={namedSessionPopupPosition.ref}
            role="dialog"
            aria-label="Create named tmux session"
            className="session-context-menu session-named-popup"
            style={namedSessionPopupPosition.style}
          >
            <div className="session-named-popup-title">New named session</div>
            <input
              aria-label="New session name"
              type="text"
              className="session-search-input"
              placeholder="Session name..."
              value={namedSessionName}
              onChange={(e) => setNamedSessionName(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') void submitNamedSession()
                if (e.key === 'Escape') setNamedSessionPopup({ show: false, x: 0, y: 0 })
              }}
              autoFocus
            />
            {terminalUsers.length > 1 && (
              <select
                aria-label="New named session user"
                className="session-user-select"
                value={namedSessionUser || getDefaultLaunchUser(activeWorkspaceId, terminalUsers)}
                onChange={(e) => setNamedSessionUser(e.target.value)}
              >
                {terminalUsers.map(user => <option key={user} value={user}>{user}</option>)}
              </select>
            )}
            <div className="session-named-popup-actions">
              <button className="session-context-item session-inline-action" onClick={submitNamedSession} disabled={!namedSessionName.trim() || creating}>
                Create named session
              </button>
              <button className="session-context-item session-inline-action" onClick={() => setNamedSessionPopup({ show: false, x: 0, y: 0 })}>
                Cancel
              </button>
            </div>
          </div>
        </DismissiblePanel>
      )}

      {!isCollapsed && (
        <div className="session-search-container">
          <input
            type="text"
            className="session-search-input"
            placeholder="Filter sessions..."
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
          />
        </div>
      )}

      {!isCollapsed && (
        <div className="session-panel-content">
          {loading && (
            <div className="panel-status">Loading...</div>
          )}

          {error && (
            <div className="panel-error">{error}</div>
          )}

          {!loading && !error && sortedGroups.length === 0 && (
            <div className="getting-started">
              <div className="getting-started-title">Getting Started</div>
              <div className="getting-started-text">
                Click <strong>+</strong> above to create your first tmux session,
                or use the terminal to run <code>tmux new -s mysession</code>
              </div>
              <div className="getting-started-hint">
                Sessions appear here and can be dragged to terminal windows
              </div>
            </div>
          )}

          {sortedGroups.map(([groupKey, groupSessions]) => (
            <SessionGroup key={groupKey} groupKey={groupKey} sessions={groupSessions} />
          ))}
        </div>
      )}

      {!isCollapsed && bankedSessionSummary.total > 0 && (
        <div className="session-panel-footer">
          <button
            type="button"
            className="session-bank-settings-link"
            onClick={onOpenSessionBankSettings}
            aria-label={`Open Session Bank settings for ${bankedSessionSummary.total} banked ${bankedSessionSummary.total === 1 ? 'session' : 'sessions'}`}
          >
            Session Bank · {bankedSessionSummary.total} banked
          </button>
        </div>
      )}
      {!isCollapsed && onWidthChange && (
        <div
          className="dock-resizer"
          role="separator"
          aria-label="Resize Sessions panel"
          aria-orientation="vertical"
          tabIndex={0}
          onPointerDown={startPanelResize}
          onKeyDown={event => {
            if (event.key === 'ArrowLeft') onWidthChange(Math.max(220, width - 16))
            if (event.key === 'ArrowRight') onWidthChange(Math.min(480, width + 16))
          }}
        />
      )}
    </div>
  )
}

export default SessionPanel
