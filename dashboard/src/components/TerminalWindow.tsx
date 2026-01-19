import { useState, useEffect, useRef, useCallback } from 'react'
import { useDroppable, useDraggable } from '@dnd-kit/core'
import { useSession } from '../context/SessionContext'
import { useToast } from '../context/ToastContext'
import { WINDOW_COLORS } from '../types'
import type { TerminalWindow as TerminalWindowType, WorkspaceId } from '../types'

interface CreateSessionButtonProps {
  workspaceId: WorkspaceId
  windowId: string
  accentColor: string
}

function CreateSessionButton({ workspaceId, windowId, accentColor }: CreateSessionButtonProps) {
  const [creating, setCreating] = useState(false)
  const { settings, refreshSessions, addSessionToWindow } = useSession()
  const { addToast } = useToast()

  const handleCreate = async () => {
    setCreating(true)
    try {
      const sessionName = `${settings.defaultSessionPrefix}-${Date.now().toString(36)}`
      const response = await fetch('/api/tmux/sessions', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: sessionName })
      })
      if (response.ok) {
        addToast(`Session '${sessionName}' created`, 'success')
        await refreshSessions()
        addSessionToWindow(workspaceId, windowId, sessionName)
      } else {
        addToast('Failed to create session', 'error')
      }
    } catch (e) {
      console.error('Failed to create session:', e)
      addToast('Failed to create session', 'error')
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

  // Extract just the agent name for display
  const displayName = sessionName.includes('-')
    ? sessionName.split('-').slice(-1)[0]
    : sessionName

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
}

function TerminalWindow({ workspaceId, window: windowConfig, isDragging = false }: TerminalWindowProps) {
  // Track loaded state per session
  const [loadedSessions, setLoadedSessions] = useState<Set<string>>(new Set())
  // Store refs for all iframes by session name
  const iframeRefs = useRef<Map<string, HTMLIFrameElement>>(new Map())
  const bodyRef = useRef<HTMLDivElement>(null)
  const windowRef = useRef<HTMLDivElement>(null)

  const {
    removeSessionFromWindow,
    setActiveSession,
    cycleSession,
    settings,
    focusedWindowKey,
    setFocusedWindowKey,
  } = useSession()

  // Generate a unique key for this window
  const windowKey = `${workspaceId}-${windowConfig.id}`
  const isFocused = focusedWindowKey === windowKey

  const activeSession = windowConfig.activeSession

  // Check if active session is loaded
  const activeSessionLoaded = activeSession ? loadedSessions.has(activeSession) : false

  // Trigger xterm fit() by dispatching resize event to the active iframe
  const triggerFit = useCallback(() => {
    if (!activeSession) return
    try {
      const iframe = iframeRefs.current.get(activeSession)
      if (!iframe?.contentWindow) return
      // Dispatch resize event - ttyd listens for this and calls fit()
      iframe.contentWindow.dispatchEvent(new Event('resize'))
    } catch {
      // Cross-origin or not ready - ignore
    }
  }, [activeSession])

  // Apply font size to a specific iframe
  const applyFontSizeToIframe = useCallback((iframe: HTMLIFrameElement, fontSize: number) => {
    if (!iframe?.contentWindow) return

    let attempts = 0
    const maxAttempts = 20 // 20 * 50ms = 1 second max wait

    const tryApply = () => {
      try {
        const iframeWindow = iframe.contentWindow as Window & { term?: { options: { fontSize: number } } }
        if (iframeWindow.term) {
          iframeWindow.term.options.fontSize = fontSize
          return // Success - stop polling
        }
      } catch {
        // Cross-origin or not ready - continue polling
      }

      attempts++
      if (attempts < maxAttempts) {
        setTimeout(tryApply, 50) // Poll every 50ms
      }
    }

    tryApply()
  }, [])

  // Handle iframe load for a specific session
  const handleIframeLoad = useCallback((sessionName: string) => {
    setLoadedSessions(prev => new Set(prev).add(sessionName))
    const iframe = iframeRefs.current.get(sessionName)
    if (iframe) {
      applyFontSizeToIframe(iframe, settings.fontSize)
      // Trigger fit after delays to ensure container has settled
      setTimeout(() => {
        if (sessionName === windowConfig.activeSession) {
          triggerFit()
        }
      }, 100)
      setTimeout(() => {
        if (sessionName === windowConfig.activeSession) {
          triggerFit()
        }
      }, 300)
    }
  }, [applyFontSizeToIframe, settings.fontSize, triggerFit, windowConfig.activeSession])

  // Apply font size when setting changes (for all loaded iframes)
  useEffect(() => {
    loadedSessions.forEach(sessionName => {
      const iframe = iframeRefs.current.get(sessionName)
      if (iframe) {
        applyFontSizeToIframe(iframe, settings.fontSize)
      }
    })
  }, [settings.fontSize, loadedSessions, applyFontSizeToIframe])

  // Trigger fit when active session changes (switching to already-loaded session)
  useEffect(() => {
    if (activeSession && loadedSessions.has(activeSession)) {
      setTimeout(triggerFit, 50)
    }
  }, [activeSession, loadedSessions, triggerFit])

  // Clean up refs for removed sessions
  useEffect(() => {
    const currentSessions = new Set(windowConfig.boundSessions)
    iframeRefs.current.forEach((_, sessionName) => {
      if (!currentSessions.has(sessionName)) {
        iframeRefs.current.delete(sessionName)
      }
    })
    // Also clean up loaded state
    setLoadedSessions(prev => {
      const newSet = new Set<string>()
      prev.forEach(s => {
        if (currentSessions.has(s)) newSet.add(s)
      })
      return newSet
    })
  }, [windowConfig.boundSessions])

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

  // ResizeObserver to trigger fit() when container size changes
  useEffect(() => {
    const body = bodyRef.current
    if (!body) return

    let timeoutId: ReturnType<typeof setTimeout>
    const observer = new ResizeObserver(() => {
      clearTimeout(timeoutId)
      // Debounce to wait for CSS transitions to complete
      timeoutId = setTimeout(triggerFit, 100)
    })

    observer.observe(body)
    return () => {
      clearTimeout(timeoutId)
      observer.disconnect()
    }
  }, [triggerFit])

  const colorTheme = WINDOW_COLORS[windowConfig.colorIndex % WINDOW_COLORS.length]

  const handleRemoveSession = (sessionName: string) => {
    removeSessionFromWindow(workspaceId, windowConfig.id, sessionName)
  }

  const handleTagClick = (sessionName: string) => {
    setActiveSession(workspaceId, windowConfig.id, sessionName)
  }

  const hasSessions = windowConfig.boundSessions.length > 0

  // Helper to generate terminal URL for a session
  const getTerminalUrl = (sessionName: string) =>
    `/terminal/?arg=${encodeURIComponent(sessionName)}&theme=${encodeURIComponent('{"background":"transparent"}')}`

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
          /* Empty window - show create button */
          <div className="empty-window-state">
            <CreateSessionButton workspaceId={workspaceId} windowId={windowConfig.id} accentColor={colorTheme.accent} />
            <span className="empty-window-hint">or drag a session here</span>
          </div>
        ) : (
          /* Render persistent iframes for all bound sessions, toggle visibility */
          windowConfig.boundSessions.map(sessionName => {
            const isActive = sessionName === activeSession
            return (
              <iframe
                key={sessionName}
                ref={(el) => {
                  if (el) {
                    iframeRefs.current.set(sessionName, el)
                  }
                }}
                src={getTerminalUrl(sessionName)}
                onLoad={() => handleIframeLoad(sessionName)}
                allow="clipboard-read; clipboard-write"
                style={{
                  width: '100%',
                  height: '100%',
                  border: 'none',
                  backgroundColor: 'transparent',
                  display: isActive ? 'block' : 'none',
                  position: isActive ? 'relative' : 'absolute',
                }}
                title={`Terminal ${windowConfig.id} - ${sessionName}`}
              />
            )
          })
        )}
        <DropOverlay workspaceId={workspaceId} windowId={windowConfig.id} isVisible={isDragging} />
      </div>
    </div>
  )
}

export default TerminalWindow
