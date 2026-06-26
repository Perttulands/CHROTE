import { useState, useEffect, useRef, useCallback } from 'react'
import { useDroppable, useDraggable } from '@dnd-kit/core'
import { useSession } from '../context/SessionContext'
import { useIframePool } from './IframePool'
import { WINDOW_COLORS, getSessionKey, getSessionNameFromKey, getSessionUserFromKey, getTerminalUserColor, getTerminalUserInitial, resolveLaunchUser } from '../types'
import type { TerminalWindow as TerminalWindowType, WorkspaceId } from '../types'

interface CreateSessionButtonProps {
  workspaceId: WorkspaceId
  windowId: string
  accentColor: string
}

function CreateSessionButton({ workspaceId, windowId, accentColor }: CreateSessionButtonProps) {
  const [creating, setCreating] = useState(false)
  const [menu, setMenu] = useState<{ show: boolean; x: number; y: number }>({ show: false, x: 0, y: 0 })
  const [namedPopup, setNamedPopup] = useState<{ show: boolean; x: number; y: number }>({ show: false, x: 0, y: 0 })
  const [namedName, setNamedName] = useState('')
  const { settings, terminalUsers, createSession } = useSession()

  const createSessionForUser = async (unixUser?: string, explicitName?: string) => {
    setCreating(true)
    try {
      const created = await createSession({
        workspaceId,
        ...(unixUser !== undefined ? { unixUser } : {}),
        ...(explicitName !== undefined ? { name: explicitName } : {}),
        attachTo: { workspaceId, windowId },
      })
      if (created) {
        setNamedName('')
        setNamedPopup({ show: false, x: 0, y: 0 })
      }
    } finally {
      setCreating(false)
      setMenu({ show: false, x: 0, y: 0 })
    }
  }

  const handleCreate = async () => {
    await createSessionForUser()
  }

  const namedSession = () => {
    setNamedPopup({ show: true, x: menu.x, y: menu.y })
    setMenu({ show: false, x: 0, y: 0 })
  }

  const submitNamedSession = async () => {
    const name = namedName.trim()
    if (!name) return
    await createSessionForUser(undefined, name)
  }

  const userChoices = terminalUsers.length > 0 ? terminalUsers : [resolveLaunchUser(settings, workspaceId, terminalUsers)]

  useEffect(() => {
    if (!menu.show) return
    const close = () => setMenu({ show: false, x: 0, y: 0 })
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') close()
    }
    document.addEventListener('click', close)
    document.addEventListener('keydown', closeOnEscape)
    return () => {
      document.removeEventListener('click', close)
      document.removeEventListener('keydown', closeOnEscape)
    }
  }, [menu.show])

  return (
    <>
      <button
        className="create-session-btn"
        onClick={handleCreate}
        onContextMenu={(event) => {
          event.preventDefault()
          setMenu({ show: true, x: event.clientX, y: event.clientY })
        }}
        disabled={creating}
        style={{ '--btn-accent': accentColor } as React.CSSProperties}
        title="Create new session"
      >
        <span className="create-session-icon">{creating ? '...' : '+'}</span>
        <span className="create-session-label">New Session</span>
      </button>
      {menu.show && (
        <div className="session-context-menu" style={{ left: menu.x, top: menu.y }}>
          {userChoices.filter(Boolean).map(user => (
            <button key={user} className="session-context-item" onClick={() => createSessionForUser(user)}>
              <span className="session-context-icon">{getTerminalUserInitial(user)}</span>
              New here as {getTerminalUserInitial(user)} {user}
            </button>
          ))}
          <div className="session-context-divider" />
          <button className="session-context-item" onClick={namedSession}>
            <span className="session-context-icon">✎</span>
            New named session
          </button>
        </div>
      )}
      {namedPopup.show && (
        <div
          role="dialog"
          aria-label="Create named tmux session"
          className="session-context-menu session-named-popup terminal-named-create"
          style={{ left: namedPopup.x, top: namedPopup.y }}
        >
          <div className="session-named-popup-title">New named session</div>
          <input
            aria-label="New session name"
            className="session-search-input"
            value={namedName}
            onChange={(event) => setNamedName(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Enter') void submitNamedSession()
              if (event.key === 'Escape') setNamedPopup({ show: false, x: 0, y: 0 })
            }}
            autoFocus
          />
          <div className="session-named-popup-actions">
            <button className="session-context-item session-inline-action" onClick={submitNamedSession} disabled={!namedName.trim()}>
              Create named session
            </button>
            <button className="session-context-item session-inline-action" onClick={() => setNamedPopup({ show: false, x: 0, y: 0 })}>
              Cancel
            </button>
          </div>
        </div>
      )}
    </>
  )
}

interface DropOverlayProps {
  workspaceId: WorkspaceId
  windowId: string
  isVisible: boolean
}

// Full-window drop overlay that appears during drag
function DropOverlay({ workspaceId, windowId, isVisible }: DropOverlayProps) {
  const { setNodeRef, isOver } = useDroppable({
    id: `drop-${workspaceId}-${windowId}`,
    data: { type: 'window', workspaceId, windowId },
  })

  if (!isVisible) return null

  return (
    <div
      ref={setNodeRef}
      className={`terminal-drop-overlay ${isOver ? 'is-over' : ''}`}
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
  const [menu, setMenu] = useState<{ show: boolean; x: number; y: number }>({ show: false, x: 0, y: 0 })
  const { sessions, deleteSession, renameSession, settings } = useSession()
  const actualName = getSessionNameFromKey(sessionName)
  const unixUser = getSessionUserFromKey(sessionName)
  const matchingSessions = unixUser
    ? sessions.filter(s => getSessionKey(s.name, s.unixUser) === sessionName)
    : sessions.filter(s => s.name === actualName)
  const session = matchingSessions.length === 1 ? matchingSessions[0] : undefined
  const resolvedUser = session?.unixUser || unixUser
  const sessionKey = getSessionKey(actualName, resolvedUser)
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({
    id: `tag-${workspaceId}-${windowId}-${sessionKey}`,
    data: { type: 'tag', sessionName: actualName, sessionKey, unixUser: resolvedUser, sourceWindowId: windowId, sourceWorkspaceId: workspaceId },
  })

  const style = transform
    ? {
      transform: `translate3d(${transform.x}px, ${transform.y}px, 0)`,
      zIndex: isDragging ? 1000 : undefined,
    }
    : undefined

  // Show full tmux session name, including prefixes (e.g. critique-codex).
  const displayName = actualName
  const canTargetSession = Boolean(session || resolvedUser.trim())

  // Handle click on the tag - only fire if not dragging
  const handleClick = (e: React.MouseEvent) => {
    // Don't trigger click if we're dragging
    if (isDragging) return
    // Don't trigger if clicking the remove button
    if ((e.target as HTMLElement).closest('.tag-remove')) return
    onClick()
  }

  const handleRename = async () => {
    const newName = window.prompt('Rename session', actualName)?.trim()
    setMenu({ show: false, x: 0, y: 0 })
    if (!newName || newName === actualName) return
    await renameSession(actualName, newName, resolvedUser)
  }

  const handleKill = async () => {
    setMenu({ show: false, x: 0, y: 0 })
    if (!window.confirm(`Kill session '${actualName}'?`)) return
    const deleted = await deleteSession(actualName, resolvedUser)
    if (deleted) onRemove()
  }

  useEffect(() => {
    if (!menu.show) return
    const close = () => setMenu({ show: false, x: 0, y: 0 })
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') close()
    }
    document.addEventListener('click', close)
    document.addEventListener('keydown', closeOnEscape)
    return () => {
      document.removeEventListener('click', close)
      document.removeEventListener('keydown', closeOnEscape)
    }
  }, [menu.show])

  return (
    <>
      <div
        ref={setNodeRef}
        className={`session-tag ${isActive ? 'active' : ''} ${isDragging ? 'dragging' : ''}`}
        style={style}
        onClick={handleClick}
        onContextMenu={(event) => {
          event.preventDefault()
          event.stopPropagation()
          setMenu({ show: true, x: event.clientX, y: event.clientY })
        }}
        {...listeners}
        {...attributes}
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
        <button className="tag-remove" onClick={(e) => { e.stopPropagation(); onRemove(); }}>×</button>
      </div>
      {menu.show && (
        <div className="session-context-menu" style={{ left: menu.x, top: menu.y }}>
          <button className="session-context-item" onClick={handleRename} disabled={!canTargetSession}>
            <span className="session-context-icon">✎</span>
            Rename
          </button>
          <button className="session-context-item session-context-danger" onClick={handleKill} disabled={!canTargetSession}>
            <span className="session-context-icon">✕</span>
            Kill
          </button>
        </div>
      )}
    </>
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
  const bodyRef = useRef<HTMLDivElement>(null)
  const windowRef = useRef<HTMLDivElement>(null)

  const pool = useIframePool()
  const [windowMenu, setWindowMenu] = useState<{ show: boolean; x: number; y: number; submenu: string | null }>({ show: false, x: 0, y: 0, submenu: null })

  const {
    sessions,
    terminalUsers,
    settings,
    layoutPresets,
    createSession,
    addSessionToWindow,
    removeSessionFromWindow,
    setActiveSession,
    cycleSession,
    setWindowCount,
    clearStaleSessionsFromWindow,
    focusedWindowKey,
    setFocusedWindowKey,
    saveCurrentLayout,
    loadPreset,
  } = useSession()

  // Generate a unique key for this window
  const windowKey = `${workspaceId}-${windowConfig.id}`
  const isFocused = focusedWindowKey === windowKey

  const activeSession = windowConfig.activeSession

  // Check if active session is loaded via pool
  const activeSessionLoaded = activeSession ? pool.isLoaded(activeSession) : false

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

  // Fit active session after layout settles. rAF fires after the first
  // paint; the 150ms fallback covers newly-connected iframes where xterm
  // isn't ready in the first frame. ResizeObserver handles everything after.
  useEffect(() => {
    if (!activeSession) return
    const fitIfLoaded = () => {
      if (pool.isLoaded(activeSession)) pool.triggerFit(activeSession)
    }
    const rafId = requestAnimationFrame(fitIfLoaded)
    const timerId = setTimeout(fitIfLoaded, 150)
    return () => {
      cancelAnimationFrame(rafId)
      clearTimeout(timerId)
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps -- pool functions are stable refs
  }, [activeSession])

  // Trigger fit() when the active session's iframe finishes loading.
  // The effect above fires before the iframe has loaded (src just set),
  // so xterm.js starts with default dimensions. This effect catches the
  // load completion and re-fits with multiple delays so xterm has time
  // to initialize its fit addon.
  useEffect(() => {
    if (!activeSession || !activeSessionLoaded) return
    const timers = [
      setTimeout(() => pool.triggerFit(activeSession), 50),
      setTimeout(() => pool.triggerFit(activeSession), 200),
      setTimeout(() => pool.triggerFit(activeSession), 500),
    ]
    return () => timers.forEach(clearTimeout)
  // eslint-disable-next-line react-hooks/exhaustive-deps -- pool functions are stable refs
  }, [activeSession, activeSessionLoaded])

  useEffect(() => {
    if (!activeSession) return
    const timers = [
      setTimeout(() => pool.triggerFit(activeSession), 0),
      setTimeout(() => pool.triggerFit(activeSession), 120),
      setTimeout(() => pool.triggerFit(activeSession), 300),
    ]
    return () => timers.forEach(clearTimeout)
  // eslint-disable-next-line react-hooks/exhaustive-deps -- pool.triggerFit is a stable ref
  }, [activeSession, refitNonce])

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

  // Store refs for values needed in keyboard handler to avoid stale closures
  const isFocusedRef = useRef(isFocused)
  const boundSessionsRef = useRef(windowConfig.boundSessions)
  useEffect(() => {
    isFocusedRef.current = isFocused
    boundSessionsRef.current = windowConfig.boundSessions
  }, [isFocused, windowConfig.boundSessions])

  // Keyboard navigation: Ctrl+Arrow to cycle sessions (only when focused)
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (!isFocusedRef.current) return
      if (!e.ctrlKey) return
      if (boundSessionsRef.current.length <= 1) return

      if (e.key === 'ArrowRight') {
        e.preventDefault()
        cycleSession(workspaceId, windowConfig.id, 'next')
      } else if (e.key === 'ArrowLeft') {
        e.preventDefault()
        cycleSession(workspaceId, windowConfig.id, 'prev')
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [workspaceId, windowConfig.id, cycleSession])

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

  const closeWindowMenu = () => setWindowMenu({ show: false, x: 0, y: 0, submenu: null })

  const createSessionHere = async (unixUser: string) => {
    await createSession({
      workspaceId,
      unixUser,
      attachTo: { workspaceId, windowId: windowConfig.id },
    })
    closeWindowMenu()
  }

  const saveLayoutFromMenu = () => {
    const name = window.prompt('Layout preset name')?.trim()
    if (name) saveCurrentLayout(name)
    closeWindowMenu()
  }

  const attachableSessions = sessions.filter(session => !windowConfig.boundSessions.includes(getSessionKey(session.name, session.unixUser)))
  const userChoices = terminalUsers.length > 0 ? terminalUsers : [resolveLaunchUser(settings, workspaceId, terminalUsers)]
  const hasSessions = windowConfig.boundSessions.length > 0

  useEffect(() => {
    if (!windowMenu.show) return
    const close = () => setWindowMenu({ show: false, x: 0, y: 0, submenu: null })
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') close()
    }
    document.addEventListener('click', close)
    document.addEventListener('keydown', closeOnEscape)
    return () => {
      document.removeEventListener('click', close)
      document.removeEventListener('keydown', closeOnEscape)
    }
  }, [windowMenu.show])

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
      <div
        className="terminal-window-header"
        onContextMenu={(event) => {
          event.preventDefault()
          setWindowMenu({ show: true, x: event.clientX, y: event.clientY, submenu: null })
        }}
      >
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
          <span
            className={`status-dot ${activeSessionLoaded ? '' : 'disconnected'}`}
            onClick={(event) => {
              event.stopPropagation()
              handleWindowClick()
            }}
          />
        </div>
      </div>

      {windowMenu.show && (
        <div className="session-context-menu" style={{ left: windowMenu.x, top: windowMenu.y }}>
          <div
            className="session-context-item session-context-submenu-trigger"
            onMouseEnter={() => setWindowMenu(prev => ({ ...prev, submenu: 'new' }))}
          >
            <span className="session-context-icon">+</span>
            New Session Here
            <span className="session-context-arrow">▶</span>
            {windowMenu.submenu === 'new' && (
              <div className="session-context-submenu">
                {userChoices.filter(Boolean).map(user => (
                  <button key={user} className="session-context-item" onClick={() => createSessionHere(user)}>
                    {getTerminalUserInitial(user)} {user}
                  </button>
                ))}
              </div>
            )}
          </div>
          <div
            className="session-context-item session-context-submenu-trigger"
            onMouseEnter={() => setWindowMenu(prev => ({ ...prev, submenu: 'attach' }))}
          >
            <span className="session-context-icon">◫</span>
            Attach Existing
            <span className="session-context-arrow">▶</span>
            {windowMenu.submenu === 'attach' && (
              <div className="session-context-submenu">
                {attachableSessions.length === 0 ? (
                  <button className="session-context-item" disabled>No sessions</button>
                ) : attachableSessions.map(session => (
                  <button
                    key={`${session.unixUser ?? ''}:${session.name}`}
                    className="session-context-item"
                    onClick={() => {
                      addSessionToWindow(workspaceId, windowConfig.id, session.name, session.unixUser)
                      closeWindowMenu()
                    }}
                  >
                    {session.unixUser && (
                      <span
                        className="session-user-badge"
                        style={{ backgroundColor: getTerminalUserColor(settings, session.unixUser) }}
                        title={`Unix user: ${session.unixUser}`}
                      >
                        {getTerminalUserInitial(session.unixUser)}
                      </span>
                    )}
                    {session.name}
                  </button>
                ))}
              </div>
            )}
          </div>
          <button
            className="session-context-item"
            onClick={() => {
              if (activeSession) removeSessionFromWindow(workspaceId, windowConfig.id, activeSession)
              closeWindowMenu()
            }}
            disabled={!activeSession}
          >
            <span className="session-context-icon">⊘</span>
            Detach Active
          </button>
          <button
            className="session-context-item"
            onClick={() => {
              if (activeSession) pool.reconnectIframe(activeSession)
              closeWindowMenu()
            }}
            disabled={!activeSession}
          >
            <span className="session-context-icon">↻</span>
            Reconnect iframe
          </button>
          <button
            className="session-context-item"
            onClick={() => {
              clearStaleSessionsFromWindow(workspaceId, windowConfig.id)
              closeWindowMenu()
            }}
          >
            <span className="session-context-icon">⌫</span>
            Clear dead/stale tags
          </button>
          <div
            className="session-context-item session-context-submenu-trigger"
            onMouseEnter={() => setWindowMenu(prev => ({ ...prev, submenu: 'count' }))}
          >
            <span className="session-context-icon">▦</span>
            Window Count
            <span className="session-context-arrow">▶</span>
            {windowMenu.submenu === 'count' && (
              <div className="session-context-submenu">
                {[1, 2, 3, 4].map(count => (
                  <button key={count} className="session-context-item" onClick={() => { setWindowCount(workspaceId, count); closeWindowMenu() }}>
                    {count} window{count === 1 ? '' : 's'}
                  </button>
                ))}
              </div>
            )}
          </div>
          <div className="session-context-divider" />
          <button className="session-context-item" onClick={saveLayoutFromMenu}>
            <span className="session-context-icon">▣</span>
            Save layout as preset
          </button>
          <div
            className="session-context-item session-context-submenu-trigger"
            onMouseEnter={() => setWindowMenu(prev => ({ ...prev, submenu: 'preset' }))}
          >
            <span className="session-context-icon">⊞</span>
            Restore layout preset
            <span className="session-context-arrow">▶</span>
            {windowMenu.submenu === 'preset' && (
              <div className="session-context-submenu">
                {layoutPresets.length === 0 ? (
                  <button className="session-context-item" disabled>No presets</button>
                ) : layoutPresets.map(preset => (
                  <button key={preset.id} className="session-context-item" onClick={() => { loadPreset(preset.id); closeWindowMenu() }}>
                    {preset.name}
                  </button>
                ))}
              </div>
            )}
          </div>
        </div>
      )}

      <div ref={bodyRef} className="terminal-window-body" onClick={handleWindowClick}>
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
        <DropOverlay workspaceId={workspaceId} windowId={windowConfig.id} isVisible={isDragging} />
      </div>
    </div>
  )
}

export default TerminalWindow
