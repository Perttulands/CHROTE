import { useState, useEffect, useMemo, useRef } from 'react'
import type { ReactNode } from 'react'
import { useSession } from '../context/SessionContext'
import TerminalWindow from './TerminalWindow'
import DismissiblePanel from './DismissiblePanel'
import { useMediaQuery } from '../hooks/useMediaQuery'
import { useViewportMenuPosition } from '../hooks/useViewportMenuPosition'
import { getSessionKey } from '../types'
import type { WorkspaceId } from '../types'
import { useIframePool } from './IframePool'

interface TerminalAreaProps {
  workspaceId: WorkspaceId
  sidecarControls?: ReactNode
  onOpenFilesAtPath?: (path: string) => void
}

function TerminalArea({ workspaceId, sidecarControls, onOpenFilesAtPath }: TerminalAreaProps) {
  const { workspaces, setWindowCount, clearStaleSessionsFromWindow, sessions, windowRevealRequest } = useSession()
  const pool = useIframePool()
  const workspace = workspaces[workspaceId]
  const windows = workspace.windows
  const windowCount = workspace.windowCount

  const isMobile = useMediaQuery('(max-width: 768px)')
  const [mobileActiveIndex, setMobileActiveIndex] = useState(0)
  const lastConsumedRevealRequestId = useRef(0)
  const [refitNonce, setRefitNonce] = useState(0)
  const [controlsMenu, setControlsMenu] = useState<{ show: boolean; x: number; y: number }>({ show: false, x: 0, y: 0 })
  const controlsMenuPosition = useViewportMenuPosition<HTMLDivElement>(
    controlsMenu.show ? { x: controlsMenu.x, y: controlsMenu.y } : null,
    { estimatedSize: { width: 220, height: 130 } },
  )
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

  // A reveal first expands windowCount in SessionContext. Consume the matching
  // request only once that canonical slot is actually part of this area's
  // visible slice, so mobile navigation lands on the revealed window.
  useEffect(() => {
    if (!windowRevealRequest || windowRevealRequest.workspaceId !== workspaceId) return
    if (windowRevealRequest.requestId <= lastConsumedRevealRequestId.current) return

    const targetIndex = windows.findIndex(window => window.id === windowRevealRequest.windowId)
    if (targetIndex < 0 || targetIndex >= windowCount) return

    lastConsumedRevealRequestId.current = windowRevealRequest.requestId
    setMobileActiveIndex(targetIndex)
  }, [windowCount, windowRevealRequest, windows, workspaceId])

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
        aria-label="Terminal workspace controls"
      >
        {sidecarControls}
        {sidecarControls && <span className="terminal-controls-divider" aria-hidden="true" />}
        {isMobile ? (
          <>
            <span className="layout-label">View:</span>
            <div className="mobile-controls-row" style={{ display: 'flex', gap: '4px', alignItems: 'center', flex: 1, overflowX: 'auto' }}>
              <div role="group" aria-label="Window view controls" style={{ display: 'contents' }}>
                {Array.from({ length: windowCount }).map((_, idx) => (
                  <button
                    key={`view-${idx}`}
                    className={`layout-btn ${mobileActiveIndex === idx ? 'active' : ''}`}
                    onClick={() => setMobileActiveIndex(idx)}
                    aria-label={`View window ${idx + 1}`}
                  >
                    {idx + 1}
                  </button>
                ))}
              </div>

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
          </>
        )}
        <button
          className="layout-btn terminal-refit-btn"
          type="button"
          aria-label="Refit terminal layout"
          title="Refit terminal layout"
          onClick={refitTerminalLayout}
        >
          Refit
        </button>
        <button
          className="layout-btn terminal-recovery-btn"
          aria-label="Terminal recovery actions"
          title="Terminal recovery actions"
          onClick={(event) => {
            const rect = event.currentTarget.getBoundingClientRect()
            setControlsMenu({ show: true, x: rect.right, y: rect.bottom + 4 })
          }}
        >
          ⋯
        </button>
      </div>

      {controlsMenu.show && (
        <DismissiblePanel onDismiss={closeControlsMenu} panelPosition="fixed">
          <div
            ref={controlsMenuPosition.ref}
            className="session-context-menu"
            style={controlsMenuPosition.style}
          >
            <button className="session-context-item" onClick={reconnectFrames} disabled={visibleWindows.every(window => window.boundSessions.length === 0)}>
              <span className="session-context-icon">↻</span>
              Reconnect frames
            </button>
            <button className="session-context-item" onClick={clearStaleSessions} disabled={staleSessionCount === 0}>
              <span className="session-context-icon">⌫</span>
              {staleSessionCount > 0 ? `Clear ${staleSessionCount} stale sessions` : 'No stale sessions'}
            </button>
          </div>
        </DismissiblePanel>
      )}

      <div className={`terminal-grid ${getGridClass()}`} data-workspace={workspaceId}>
        {visibleWindows.map((window, index) => {
          const isVisible = !isMobile || index === mobileActiveIndex
          return (
            <TerminalWindow
              key={window.id}
              workspaceId={workspaceId}
              window={window}
              refitNonce={refitNonce}
              style={{ display: isVisible ? 'flex' : 'none' }}
              onOpenFilesAtPath={onOpenFilesAtPath}
            />
          )
        })}
      </div>
    </div>
  )
}

export default TerminalArea
