import { useState, useEffect, useRef } from 'react'
import { useSession } from '../context/SessionContext'
import TerminalWindow from './TerminalWindow'
import { useMediaQuery } from '../hooks/useMediaQuery'
import type { WorkspaceId } from '../types'
import { isFeatureEnabled } from '../featureFlags'
import { useIframePool } from './IframePool'

interface TerminalAreaProps {
  workspaceId: WorkspaceId
}

function TerminalArea({ workspaceId }: TerminalAreaProps) {
  const { workspaces, setWindowCount, clearStaleSessionsFromWindow, isDragging } = useSession()
  const pool = useIframePool()
  const workspace = workspaces[workspaceId]
  const windows = workspace.windows
  const windowCount = workspace.windowCount

  const isMobile = useMediaQuery('(max-width: 768px)')
  const [mobileActiveIndex, setMobileActiveIndex] = useState(0)
  const [refitNonce, setRefitNonce] = useState(0)
  const [controlsMenu, setControlsMenu] = useState<{ show: boolean; x: number; y: number }>({ show: false, x: 0, y: 0 })
  const controlsMenuRef = useRef<HTMLDivElement>(null)
  const showRefitButton = isFeatureEnabled('terminalRefitButton')
  const visibleWindows = windows.slice(0, windowCount)

  // Ensure valid mobile index when configuration changes
  useEffect(() => {
    if (mobileActiveIndex >= windowCount) {
      setMobileActiveIndex(0)
    }
  }, [windowCount, mobileActiveIndex])

  useEffect(() => {
    if (!controlsMenu.show) return
    const close = (event: MouseEvent) => {
      if (controlsMenuRef.current?.contains(event.target as Node)) return
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
          </>
        )}
      </div>

      {controlsMenu.show && (
        <div
          ref={controlsMenuRef}
          className="session-context-menu"
          style={{ left: controlsMenu.x, top: controlsMenu.y }}
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
              isDragging={isDragging}
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
