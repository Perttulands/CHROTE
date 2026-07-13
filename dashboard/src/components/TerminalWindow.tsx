import { useState, useEffect, useRef, useCallback } from 'react'
import { useDroppable, useDraggable } from '@dnd-kit/core'
import { useSession } from '../context/SessionContext'
import { useIframePool } from './IframePool'
import { WINDOW_COLORS, getSessionKey, getSessionNameFromKey, getSessionUserFromKey, getTerminalUserColor, getTerminalUserInitial } from '../types'
import type { TerminalWindow as TerminalWindowType, WorkspaceId } from '../types'

interface CreateSessionButtonProps {
  workspaceId: WorkspaceId
  windowId: string
  accentColor: string
}

function CreateSessionButton({ workspaceId, windowId, accentColor }: CreateSessionButtonProps) {
  const [creating, setCreating] = useState(false)
  const { createSession } = useSession()

  const handleCreate = async () => {
    setCreating(true)
    try {
      await createSession({
        workspaceId,
        attachTo: { workspaceId, windowId },
      })
    } finally {
      setCreating(false)
    }
  }

  return (
    <button
      className="create-session-btn"
      onClick={handleCreate}
      disabled={creating}
      style={{ '--btn-accent': accentColor } as React.CSSProperties}
      title="Create new session"
    >
      <span className="create-session-icon">{creating ? '...' : '+'}</span>
      <span className="create-session-label">New Session</span>
    </button>
  )
}

interface DropOverlayProps {
  isVisible: boolean
  isOver: boolean
}

// Full-window drop overlay that appears during drag
function DropOverlay({ isVisible, isOver }: DropOverlayProps) {
  if (!isVisible) return null

  return (
    <div
      className={`terminal-drop-overlay ${isOver ? 'is-over' : ''}`}
      style={{ inset: 0, pointerEvents: 'none' }}
    >
      <span className="drop-hint">{isOver ? 'Release to add' : 'Drop here'}</span>
    </div>
  )
}

interface SessionTagProps {
  sessionName: string
  isActive: boolean
  workspaceId: WorkspaceId
  windowId: string
  onRemove: () => void
  onClick: () => void
}

function SessionTag({ sessionName, isActive, workspaceId, windowId, onRemove, onClick }: SessionTagProps) {
  const { sessions, settings } = useSession()
  const actualName = getSessionNameFromKey(sessionName)
  const unixUser = getSessionUserFromKey(sessionName)
  const matchingSessions = unixUser
    ? sessions.filter(s => getSessionKey(s.name, s.unixUser) === sessionName)
    : sessions.filter(s => s.name === actualName)
  const session = matchingSessions.length === 1 ? matchingSessions[0] : undefined
  const resolvedUser = session?.unixUser || unixUser
  const sessionKey = getSessionKey(actualName, resolvedUser)
  const { listeners, setNodeRef, isDragging } = useDraggable({
    id: `tag-${workspaceId}-${windowId}-${sessionKey}`,
    data: { type: 'tag', sessionName: actualName, sessionKey, unixUser: resolvedUser, sourceWindowId: windowId, sourceWorkspaceId: workspaceId },
  })

  const style = isDragging
    ? { opacity: 0, transition: 'none' }
    : undefined

  // Show full tmux session name, including prefixes (e.g. critique-codex).
  const displayName = actualName
  const dragLabel = `Drag ${displayName}${resolvedUser ? ` (Unix user ${resolvedUser})` : ''}`

  // Handle click on the tag - only fire if not dragging
  const handleClick = (e: React.MouseEvent) => {
    // Don't trigger click if we're dragging
    if (isDragging) return
    // Don't trigger if clicking the remove button
    if ((e.target as HTMLElement).closest('.tag-remove')) return
    onClick()
  }

  return (
    <div
      ref={setNodeRef}
      className={`session-tag ${isActive ? 'active' : ''} ${isDragging ? 'dragging' : ''}`}
      style={style}
      title={dragLabel}
      {...listeners}
      onClick={handleClick}
    >
      {resolvedUser && (
        <span
          className="session-user-badge"
          style={{ backgroundColor: getTerminalUserColor(settings, resolvedUser) }}
          title={`Unix user: ${resolvedUser}`}
        >
          {getTerminalUserInitial(resolvedUser)}
        </span>
      )}
      <span className="tag-name">{displayName}</span>
      <button
        className="tag-remove"
        onPointerDown={event => event.stopPropagation()}
        onClick={(event) => { event.stopPropagation(); onRemove() }}
      >
        ×
      </button>
    </div>
  )
}

interface TerminalWindowProps {
  workspaceId: WorkspaceId
  window: TerminalWindowType
  isDragging?: boolean
  refitNonce?: number
  style?: React.CSSProperties
}

function TerminalWindow({ workspaceId, window: windowConfig, isDragging = false, refitNonce = 0, style }: TerminalWindowProps) {
  const bodyRef = useRef<HTMLDivElement | null>(null)
  const windowRef = useRef<HTMLDivElement>(null)
  const { setNodeRef: setDropNodeRef, isOver } = useDroppable({
    id: `drop-${workspaceId}-${windowConfig.id}`,
    data: { type: 'window', workspaceId, windowId: windowConfig.id },
  })
  const setBodyRef = useCallback((node: HTMLDivElement | null) => {
    bodyRef.current = node
    setDropNodeRef(node)
  }, [setDropNodeRef])

  const pool = useIframePool()

  const {
    removeSessionFromWindow,
    setActiveSession,
    cycleSession,
    focusedWindowKey,
    setFocusedWindowKey,
  } = useSession()

  // Generate a unique key for this window
  const windowKey = `${workspaceId}-${windowConfig.id}`
  const isFocused = focusedWindowKey === windowKey

  const activeSession = windowConfig.activeSession

  // Check if active session is loaded via pool
  const activeSessionLoaded = activeSession ? pool.loadedSessions.has(activeSession) : false

  // Claim/release iframes from the pool as boundSessions change
  useEffect(() => {
    const body = bodyRef.current
    if (!body) return

    const cleanups: (() => void)[] = []
    windowConfig.boundSessions.forEach(sessionName => {
      if (sessionName && sessionName !== 'INIT-PENDING') {
        const cleanup = pool.claimIframe(sessionName, body)
        cleanups.push(cleanup)
      }
    })

    return () => {
      cleanups.forEach(fn => fn())
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps -- pool.claimIframe is a stable ref
  }, [windowConfig.boundSessions])

  // Manage visibility of claimed iframes based on active session.
  // CSS (.terminal-window-body iframe) handles position/size via position:absolute + inset.
  // This effect only toggles display to show/hide the correct iframe.
  useEffect(() => {
    windowConfig.boundSessions.forEach(sessionName => {
      const iframe = pool.getIframe(sessionName)
      if (!iframe) return
      const isActive = sessionName === activeSession
      iframe.style.display = isActive ? 'block' : 'none'
    })

  // eslint-disable-next-line react-hooks/exhaustive-deps -- pool functions are stable refs
  }, [activeSession, windowConfig.boundSessions])

  useEffect(() => {
    if (!activeSession || !activeSessionLoaded) return
    const rafId = requestAnimationFrame(() => pool.triggerFit(activeSession))
    return () => cancelAnimationFrame(rafId)
  // eslint-disable-next-line react-hooks/exhaustive-deps -- pool.triggerFit is a stable ref
  }, [activeSession, activeSessionLoaded, refitNonce])

  // Focus iframe when this window is focused
  useEffect(() => {
    if (isFocused && activeSession) {
      pool.focusIframe(activeSession)
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps -- pool.focusIframe is a stable ref
  }, [isFocused, activeSession])

  // Handle click on this window to focus it for keyboard navigation
  const handleWindowClick = useCallback(() => {
    setFocusedWindowKey(windowKey)
  }, [windowKey, setFocusedWindowKey])


  // Store activeSession in ref for ResizeObserver callback
  const activeSessionRef = useRef(activeSession)
  useEffect(() => { activeSessionRef.current = activeSession }, [activeSession])

  // ResizeObserver to trigger fit() when container size changes
  useEffect(() => {
    const body = bodyRef.current
    if (!body) return

    let timeoutId: ReturnType<typeof setTimeout>
    const observer = new ResizeObserver(() => {
      clearTimeout(timeoutId)
      timeoutId = setTimeout(() => {
        if (activeSessionRef.current) pool.triggerFit(activeSessionRef.current)
      }, 100)
    })

    observer.observe(body)
    return () => {
      clearTimeout(timeoutId)
      observer.disconnect()
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps -- pool.triggerFit is a stable ref
  }, [])

  const colorTheme = WINDOW_COLORS[windowConfig.colorIndex % WINDOW_COLORS.length]

  const handleRemoveSession = (sessionName: string) => {
    removeSessionFromWindow(workspaceId, windowConfig.id, sessionName)
  }

  const handleTagClick = (sessionName: string) => {
    setActiveSession(workspaceId, windowConfig.id, sessionName)
  }


  const hasSessions = windowConfig.boundSessions.length > 0

  return (
    <div
      ref={windowRef}
      className={`terminal-window ${isFocused ? 'focused' : ''}`}
      tabIndex={-1}
      style={{
        '--window-accent': colorTheme.accent,
        '--window-bg': colorTheme.bg,
        '--window-border': colorTheme.border,
        ...style,
      } as React.CSSProperties}
    >
      <div className="terminal-window-header">
        <div className="session-tags">
          {windowConfig.boundSessions.map(sessionName => (
            <SessionTag
              key={sessionName}
              sessionName={sessionName}
              isActive={sessionName === activeSession}
              workspaceId={workspaceId}
              windowId={windowConfig.id}
              onRemove={() => handleRemoveSession(sessionName)}
              onClick={() => handleTagClick(sessionName)}

            />
          ))}
        </div>

        <div className="window-controls">
          {hasSessions && windowConfig.boundSessions.length > 1 && (
            <>
              <button
                className="cycle-btn"
                onClick={() => cycleSession(workspaceId, windowConfig.id, 'prev')}
                title="Previous session"
              >
                ←
              </button>
              <button
                className="cycle-btn"
                onClick={() => cycleSession(workspaceId, windowConfig.id, 'next')}
                title="Next session"
              >
                →
              </button>
            </>
          )}
          {activeSession && !activeSessionLoaded && (
            <span className="terminal-loading-state">Loading terminal…</span>
          )}
        </div>
      </div>

      <div ref={setBodyRef} className="terminal-window-body" onClick={handleWindowClick}>
        {activeSession === 'INIT-PENDING' ? (
          <div style={{
            display: 'flex',
            justifyContent: 'center',
            alignItems: 'center',
            height: '100%',
            color: colorTheme.accent
          }}>
            <span>Initializing Session...</span>
          </div>
        ) : !hasSessions ? (
          <div className="empty-window-state">
            <CreateSessionButton workspaceId={workspaceId} windowId={windowConfig.id} accentColor={colorTheme.accent} />
            <span className="empty-window-hint">or drag a session here</span>
          </div>
        ) : null}
        {/* Iframes are injected here by the IframePool via DOM manipulation */}
        <DropOverlay isVisible={isDragging} isOver={isOver} />
      </div>
    </div>
  )
}

export default TerminalWindow
