import { useState, useEffect, useRef, useCallback } from 'react'
import { useDroppable, useDraggable } from '@dnd-kit/core'
import { useSession } from '../context/SessionContext'
import { WINDOW_COLORS } from '../types'
import type { TerminalWindow as TerminalWindowType } from '../types'

interface CreateSessionButtonProps {
  windowId: string
  accentColor: string
}

function CreateSessionButton({ windowId, accentColor }: CreateSessionButtonProps) {
  const [creating, setCreating] = useState(false)
  const { settings, refreshSessions, addSessionToWindow } = useSession()

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
        await refreshSessions()
        addSessionToWindow(windowId, sessionName)
      }
    } catch (e) {
      console.error('Failed to create session:', e)
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
  windowId: string
  isVisible: boolean
}

// Full-window drop overlay that appears during drag
function DropOverlay({ windowId, isVisible }: DropOverlayProps) {
  const { setNodeRef, isOver } = useDroppable({
    id: `drop-${windowId}`,
    data: { type: 'window', windowId },
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
  windowId: string
  onRemove: () => void
  onClick: () => void
}

function SessionTag({ sessionName, isActive, windowId, onRemove, onClick }: SessionTagProps) {
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({
    id: `tag-${windowId}-${sessionName}`,
    data: { type: 'tag', sessionName, sourceWindowId: windowId },
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
  window: TerminalWindowType
  isDragging?: boolean
  isFocused?: boolean
  onFocus?: () => void
}

function TerminalWindow({ window: windowConfig, isDragging = false, isFocused = false, onFocus }: TerminalWindowProps) {
  // Track loaded state per session
  const [loadedSessions, setLoadedSessions] = useState<Set<string>>(new Set())
  // Store refs for all iframes by session name
  const iframeRefs = useRef<Map<string, HTMLIFrameElement>>(new Map())
  const bodyRef = useRef<HTMLDivElement>(null)

  const {
    removeSessionFromWindow,
    setActiveSession,
    cycleSession,
    settings,
  } = useSession()

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
    removeSessionFromWindow(windowConfig.id, sessionName)
  }

  const handleTagClick = (sessionName: string) => {
    setActiveSession(windowConfig.id, sessionName)
  }

  const hasSessions = windowConfig.boundSessions.length > 0

  // Helper to generate terminal URL for a session
  const getTerminalUrl = (sessionName: string) =>
    `/terminal/?arg=${encodeURIComponent(sessionName)}&theme=${encodeURIComponent('{"background":"rgba(0,0,0,0)"}')}`

  return (
    <div
      className={`terminal-window ${isFocused ? 'focused' : ''}`}
      style={{
        '--window-accent': colorTheme.accent,
        '--window-bg': colorTheme.bg,
        '--window-border': colorTheme.border,
      } as React.CSSProperties}
      onClick={onFocus}
    >
      <div className="terminal-window-header">
        <div className="session-tags">
          {windowConfig.boundSessions.map(sessionName => (
            <SessionTag
              key={sessionName}
              sessionName={sessionName}
              isActive={sessionName === activeSession}
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
                onClick={() => cycleSession(windowConfig.id, 'prev')}
                title="Previous session"
              >
                ←
              </button>
              <button
                className="cycle-btn"
                onClick={() => cycleSession(windowConfig.id, 'next')}
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
            <CreateSessionButton windowId={windowConfig.id} accentColor={colorTheme.accent} />
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
        <DropOverlay windowId={windowConfig.id} isVisible={isDragging} />
      </div>
    </div>
  )
}

export default TerminalWindow
