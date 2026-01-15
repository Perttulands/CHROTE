import { useState, useEffect, useRef, useCallback } from 'react'
import { useDroppable, useDraggable } from '@dnd-kit/core'
import { useSession } from '../context/SessionContext'
import { WINDOW_COLORS } from '../types'
import type { TerminalWindow as TerminalWindowType } from '../types'

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
  const [loaded, setLoaded] = useState(false)
  const iframeRef = useRef<HTMLIFrameElement>(null)
  const bodyRef = useRef<HTMLDivElement>(null)

  // Reset loaded state when session changes (so status dot shows disconnected during switch)
  useEffect(() => {
    setLoaded(false)
  }, [windowConfig.activeSession])

  const {
    removeSessionFromWindow,
    setActiveSession,
    cycleSession,
    settings,
  } = useSession()

  // Trigger xterm fit() by dispatching resize event to iframe
  const triggerFit = useCallback(() => {
    try {
      const iframe = iframeRef.current
      if (!iframe?.contentWindow) return
      // Dispatch resize event - ttyd listens for this and calls fit()
      iframe.contentWindow.dispatchEvent(new Event('resize'))
    } catch {
      // Cross-origin or not ready - ignore
    }
  }, [])

  // Apply font size to xterm instance inside iframe with polling for readiness
  const applyFontSize = useCallback((fontSize: number) => {
    const iframe = iframeRef.current
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

  // Apply font size when iframe loads, and trigger fit after delays to handle layout settling
  const handleIframeLoad = useCallback(() => {
    setLoaded(true)
    applyFontSize(settings.fontSize)
    // Trigger fit after delays to ensure container has settled
    setTimeout(triggerFit, 100)
    setTimeout(triggerFit, 300)
    setTimeout(triggerFit, 500)
  }, [applyFontSize, settings.fontSize, triggerFit])

  // Apply font size when setting changes (for already loaded iframes)
  useEffect(() => {
    if (loaded) {
      applyFontSize(settings.fontSize)
    }
  }, [loaded, settings.fontSize, applyFontSize])

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
  const activeSession = windowConfig.activeSession

  // ttyd serves its own terminal UI - embed via iframe
  // If a session is active, pass it as arg to attach to that tmux session
  const terminalUrl = activeSession && activeSession !== 'INIT-PENDING'
    ? `/terminal/?arg=${encodeURIComponent(activeSession)}&theme=${encodeURIComponent('{"background":"rgba(0,0,0,0)"}')}`
    : `/terminal/?arg=&arg=${encodeURIComponent(settings.terminalMode)}&theme=${encodeURIComponent('{"background":"rgba(0,0,0,0)"}')}`

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
          {!hasSessions && <span className="no-sessions">Drag a session here to begin</span>}
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
          <span className={`status-dot ${loaded ? '' : 'disconnected'}`} />
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
        ) : (
          /* Key forces iframe reload when session changes */
          <iframe
            ref={iframeRef}
            key={activeSession || 'shell'}
            src={terminalUrl}
            onLoad={handleIframeLoad}
            style={{
              width: '100%',
              height: '100%',
              border: 'none',
              backgroundColor: colorTheme.bg,
            }}
            title={`Terminal ${windowConfig.id}${activeSession ? ` - ${activeSession}` : ''}`}
          />
        )}
        <DropOverlay windowId={windowConfig.id} isVisible={isDragging} />
      </div>
    </div>
  )
}

export default TerminalWindow
