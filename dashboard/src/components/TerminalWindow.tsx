import { useState, useEffect, useRef, useCallback } from 'react'
import { useDroppable, useDraggable } from '@dnd-kit/core'
import { useSession } from '../context/SessionContext'
import { useToast } from '../context/ToastContext'
import { useIframePool } from './IframePool'
import { WINDOW_COLORS } from '../types'
import type { TerminalWindow as TerminalWindowType, WorkspaceId } from '../types'
import { createGasCitySession } from '../services/gascityClient'

interface CreateSessionButtonProps {
  workspaceId: WorkspaceId
  windowId: string
  accentColor: string
}

type CreateSessionMode = 'tmux' | 'gascity'

function CreateSessionButton({ workspaceId, windowId, accentColor }: CreateSessionButtonProps) {
  const [creating, setCreating] = useState(false)
  const [mode, setMode] = useState<CreateSessionMode>('tmux')
  const [gasCityName, setGasCityName] = useState(`agent-${Date.now().toString(36)}`)
  const [gasCityTemplate, setGasCityTemplate] = useState('planner')
  const [gasCityTitle, setGasCityTitle] = useState('')
  const { settings, refreshSessions, addSessionToWindow } = useSession()
  const { addToast } = useToast()

  const handleCreateTmux = async () => {
    const sessionName = `${settings.defaultSessionPrefix}-${Date.now().toString(36)}`
    const response = await fetch('/api/tmux/sessions', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: sessionName }),
      signal: AbortSignal.timeout(10000),
    })
    if (!response.ok) {
      addToast('Failed to create session', 'error')
      return
    }
    addToast(`Session '${sessionName}' created`, 'success')
    await refreshSessions()
    addSessionToWindow(workspaceId, windowId, sessionName)
  }

  const handleCreateGasCity = async () => {
    const name = gasCityName.trim()
    const template = gasCityTemplate.trim()
    const title = gasCityTitle.trim()
    if (!name || !template) {
      addToast('Name and template are required', 'error')
      return
    }

    const created = await createGasCitySession({
      name,
      template,
      ...(title ? { title } : {}),
    }, {
      signal: AbortSignal.timeout(150000),
    })

    const attachTarget = created.attachTarget || `gc:${created.id}`
    addToast(`Identity '${created.name || name}' created`, 'success')
    await refreshSessions()
    addSessionToWindow(workspaceId, windowId, attachTarget)
  }

  const handleCreate = async () => {
    setCreating(true)
    try {
      await (mode === 'gascity' ? handleCreateGasCity() : handleCreateTmux())
    } catch (e) {
      console.error('Failed to create session:', e)
      addToast(mode === 'gascity' ? 'Failed to create identity' : 'Failed to create session', 'error')
    } finally {
      setCreating(false)
    }
  }

  return (
    <div className="create-session-controls" style={{ '--btn-accent': accentColor } as React.CSSProperties}>
      <div className="create-session-mode" role="group" aria-label="Session type">
        <button
          type="button"
          className={`create-session-mode-btn ${mode === 'tmux' ? 'active' : ''}`}
          aria-pressed={mode === 'tmux'}
          onClick={() => setMode('tmux')}
        >
          tmux
        </button>
        <button
          type="button"
          className={`create-session-mode-btn ${mode === 'gascity' ? 'active' : ''}`}
          aria-pressed={mode === 'gascity'}
          onClick={() => setMode('gascity')}
        >
          Gas City
        </button>
      </div>

      {mode === 'gascity' && (
        <div className="gascity-create-fields">
          <label className="gascity-create-field">
            <span>Name</span>
            <input
              value={gasCityName}
              onChange={(event) => setGasCityName(event.target.value)}
              autoComplete="off"
            />
          </label>
          <label className="gascity-create-field">
            <span>Template</span>
            <input
              value={gasCityTemplate}
              onChange={(event) => setGasCityTemplate(event.target.value)}
              autoComplete="off"
            />
          </label>
          <label className="gascity-create-field">
            <span>Title</span>
            <input
              value={gasCityTitle}
              onChange={(event) => setGasCityTitle(event.target.value)}
              autoComplete="off"
            />
          </label>
        </div>
      )}

      <button
        className={`create-session-btn ${mode === 'gascity' ? 'gascity-create-submit' : ''}`}
        onClick={handleCreate}
        disabled={creating}
        title={mode === 'gascity' ? 'Create Gas City identity' : 'Create new session'}
      >
        <span className="create-session-icon">{creating ? '...' : '+'}</span>
        <span className="create-session-label">{mode === 'gascity' ? 'New Identity' : 'New Session'}</span>
      </button>
    </div>
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
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({
    id: `tag-${workspaceId}-${windowId}-${sessionName}`,
    data: { type: 'tag', sessionName, sourceWindowId: windowId, sourceWorkspaceId: workspaceId },
  })

  const style = transform
    ? {
      transform: `translate3d(${transform.x}px, ${transform.y}px, 0)`,
      zIndex: isDragging ? 1000 : undefined,
    }
    : undefined

  // Show full tmux session name, including prefixes (e.g. critique-codex).
  const displayName = sessionName

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
      onClick={handleClick}
      {...listeners}
      {...attributes}
    >
      <span className="tag-name">{displayName}</span>
      <button className="tag-remove" onClick={(e) => { e.stopPropagation(); onRemove(); }}>×</button>
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
  const bodyRef = useRef<HTMLDivElement>(null)
  const windowRef = useRef<HTMLDivElement>(null)

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

  const hasSessions = windowConfig.boundSessions.length > 0

  return (
    <div
      ref={windowRef}
      className={`terminal-window ${isFocused ? 'focused' : ''}`}
      tabIndex={-1}
      onClick={handleWindowClick}
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
          <span className={`status-dot ${activeSessionLoaded ? '' : 'disconnected'}`} />
        </div>
      </div>

      <div ref={bodyRef} className="terminal-window-body">
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
