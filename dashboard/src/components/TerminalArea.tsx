import { useState, useEffect, useMemo } from 'react'
import { useSession } from '../context/SessionContext'
import TerminalWindow from './TerminalWindow'
import { useMediaQuery } from '../hooks/useMediaQuery'
import { useViewportMenuPosition } from '../hooks/useViewportMenuPosition'
import { getSessionKey } from '../types'
import type { WorkspaceId } from '../types'
import { isFeatureEnabled } from '../featureFlags'
import { useIframePool } from './IframePool'

interface TerminalAreaProps {
  workspaceId: WorkspaceId
  active?: boolean
}

function TerminalArea({ workspaceId, active = true }: TerminalAreaProps) {
  const { workspaces, setWindowCount, clearStaleSessionsFromWindow, isDragging, sessions } = useSession()
  const pool = useIframePool()
  const workspace = workspaces[workspaceId]
  const windows = workspace.windows
  const windowCount = workspace.windowCount

  const isMobile = useMediaQuery('(max-width: 768px)')
  const [mobileActiveIndex, setMobileActiveIndex] = useState(0)
  const [refitNonce, setRefitNonce] = useState(0)
  const [controlsMenu, setControlsMenu] = useState<{ show: boolean; x: number; y: number }>({ show: false, x: 0, y: 0 })
  const controlsMenuPosition = useViewportMenuPosition<HTMLDivElement>(
    controlsMenu.show ? { x: controlsMenu.x, y: controlsMenu.y } : null,
    { estimatedSize: { width: 220, height: 130 } },
  )
  const showRefitButton = isFeatureEnabled('terminalRefitButton')
  const visibleWindows = windows.slice(0, windowCount)
  const liveSessions = useMemo(() => {
    const live = new Set<string>()
    sessions.forEach(session => {
      live.add(getSessionKey(session.name, session.unixUser))
      live.add(session.name)
    })
    return live
  }, [sessions])
  const staleSessionCount = useMemo(() => visibleWindows.reduce((count, window) => (
    count + window.boundSessions.filter(sessionName => sessionName !== 'INIT-PENDING' && !liveSessions.has(sessionName)).length
  ), 0), [liveSessions, visibleWindows])

  // Ensure valid mobile index when configuration changes
  useEffect(() => {
    if (mobileActiveIndex >= windowCount) {
      setMobileActiveIndex(0)
    }
  }, [windowCount, mobileActiveIndex])

  useEffect(() => {
    if (!controlsMenu.show) return
    const close = (event: MouseEvent) => {
      if (controlsMenuPosition.ref.current?.contains(event.target as Node)) return
      setControlsMenu({ show: false, x: 0, y: 0 })
    }
    document.addEventListener('mousedown', close)
    return () => document.removeEventListener('mousedown', close)
  }, [controlsMenu.show])

  const closeControlsMenu = () => setControlsMenu({ show: false, x: 0, y: 0 })

  const reconnectFrames = () => {
    const sessionNames = new Set<string>()
    visibleWindows.forEach(window => window.boundSessions.forEach(sessionName => {
      if (sessionName && sessionName !== 'INIT-PENDING') sessionNames.add(sessionName)
    }))
    sessionNames.forEach(sessionName => pool.reconnectIframe(sessionName))
    closeControlsMenu()
  }

  const clearStaleSessions = () => {
    visibleWindows.forEach(window => clearStaleSessionsFromWindow(workspaceId, window.id))
    closeControlsMenu()
  }

  const refitTerminalLayout = () => {
    setRefitNonce(n => n + 1)
    closeControlsMenu()
  }

  const cleanStaleControl = staleSessionCount > 0 ? (
    <button
      className="layout-btn terminal-clean-stale-btn"
      onClick={clearStaleSessions}
      title={`Clean ${staleSessionCount} stale terminal session${staleSessionCount === 1 ? '' : 's'}`}
      aria-label={`Clean ${staleSessionCount} stale session${staleSessionCount === 1 ? '' : 's'}`}
    >
      Clean stale · {staleSessionCount}
    </button>
  ) : null

  // Get grid class based on window count
  const getGridClass = () => {
    if (isMobile) return 'grid-1'

    switch (windowCount) {
      case 1: return 'grid-1'
      case 2: return 'grid-2'
      case 3: return 'grid-3'
      case 4: return 'grid-4'
      default: return 'grid-2'
    }
  }

  return (
    <div className="terminal-area">
      <div
        className="terminal-area-controls"
        aria-label="Terminal layout controls"
        onContextMenu={(event) => {
          event.preventDefault()
          setControlsMenu({ show: true, x: event.clientX, y: event.clientY })
        }}
      >
        {isMobile ? (
          <>
            <span className="layout-label">View:</span>
            <div className="mobile-controls-row" style={{ display: 'flex', gap: '4px', alignItems: 'center', flex: 1, overflowX: 'auto' }}>
              {Array.from({ length: windowCount }).map((_, idx) => (
                <button
                  key={`view-${idx}`}
                  className={`layout-btn ${mobileActiveIndex === idx ? 'active' : ''}`}
                  onClick={() => setMobileActiveIndex(idx)}
                >
                  {idx + 1}
                </button>
              ))}

              <div style={{ width: '1px', height: '16px', background: 'var(--divider)', margin: '0 8px' }}></div>

              <span className="layout-label">Count:</span>
              {[1, 2, 3, 4].map(count => (
                <button
                  key={`count-${count}`}
                  className={`layout-btn ${windowCount === count ? 'active' : ''}`}
                  onClick={() => setWindowCount(workspaceId, count)}
                  style={{ opacity: windowCount === count ? 1 : 0.7 }}
                >
                  {count}
                </button>
              ))}
              {showRefitButton && (
                <button
                  className="layout-btn terminal-refit-btn"
                  onClick={refitTerminalLayout}
                  title="Refit terminal layout"
                  aria-label="Refit terminal layout"
                >
                  <span aria-hidden="true">↻</span>
                </button>
              )}
              {cleanStaleControl}
            </div>
          </>
        ) : (
          <>
            <span className="layout-label">Layout:</span>
            {[1, 2, 3, 4].map(count => (
              <button
                key={count}
                className={`layout-btn ${windowCount === count ? 'active' : ''}`}
                onClick={() => setWindowCount(workspaceId, count)}
                title={`${count} window${count > 1 ? 's' : ''}`}
              >
                {count}
              </button>
            ))}
            {showRefitButton && (
              <button
                className="layout-btn terminal-refit-btn"
                onClick={refitTerminalLayout}
                title="Refit terminal layout"
                aria-label="Refit terminal layout"
              >
                <span aria-hidden="true">↻</span>
              </button>
            )}
            {cleanStaleControl}
          </>
        )}
      </div>

      {controlsMenu.show && (
        <div
          ref={controlsMenuPosition.ref}
          className="session-context-menu"
          style={controlsMenuPosition.style}
        >
          <button className="session-context-item" onClick={reconnectFrames} disabled={visibleWindows.every(window => window.boundSessions.length === 0)}>
            <span className="session-context-icon">↻</span>
            Reconnect frames
          </button>
          <button className="session-context-item" onClick={clearStaleSessions}>
            <span className="session-context-icon">⌫</span>
            Clear stale sessions
          </button>
          <button className="session-context-item" onClick={refitTerminalLayout}>
            <span className="session-context-icon">⤢</span>
            Refit terminal layout
          </button>
        </div>
      )}

      <div className={`terminal-grid ${getGridClass()}`} data-workspace={workspaceId}>
        {visibleWindows.map((window, index) => {
          const isVisible = !isMobile || index === mobileActiveIndex
          return (
            <TerminalWindow
              key={window.id}
              workspaceId={workspaceId}
              window={window}
              isDragging={active && isVisible && isDragging}
              refitNonce={refitNonce}
              style={{ display: isVisible ? 'flex' : 'none' }}
            />
          )
        })}
      </div>
    </div>
  )
}

export default TerminalArea
