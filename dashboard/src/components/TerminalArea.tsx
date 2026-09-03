import { useState, useEffect, useRef } from 'react'
import type { ReactNode } from 'react'
import { useSession } from '../context/SessionContext'
import TerminalWindow, { CLAIM_EXPLANATION } from './TerminalWindow'
import Menu, { type MenuGroup } from './Menu'
import { useMediaQuery } from '../hooks/useMediaQuery'
import type { WorkspaceId } from '../types'
import { useTerminalPool } from './TerminalPool'

interface TerminalAreaProps {
  workspaceId: WorkspaceId
  sidecarControls?: ReactNode
  onOpenFilesAtPath?: (path: string) => void
  workspaceActive?: boolean
}

function TerminalArea({ workspaceId, sidecarControls, onOpenFilesAtPath, workspaceActive = true }: TerminalAreaProps) {
  const { workspaces, setWindowCount, windowRevealRequest } = useSession()
  const pool = useTerminalPool()
  const workspace = workspaces[workspaceId]
  const windows = workspace.windows
  const windowCount = workspace.windowCount

  const isMobile = useMediaQuery('(max-width: 768px)')
  const [mobileActiveIndex, setMobileActiveIndex] = useState(0)
  const lastConsumedRevealRequestId = useRef(0)
  const [refitNonce, setRefitNonce] = useState(0)
  const [controlsMenu, setControlsMenu] = useState<{ show: boolean; x: number; y: number }>({ show: false, x: 0, y: 0 })
  const visibleWindows = windows.slice(0, windowCount)

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
      if (sessionName) sessionNames.add(sessionName)
    }))
    sessionNames.forEach(sessionName => pool.terminals.get(sessionName)?.reconnect())
  }

  // The sessions this device is actually showing: the active binding of each
  // window on screen. Claiming resizes a tmux window for everyone watching it,
  // so it is offered only for the frames in front of the operator — a bound
  // session behind a tab, or on a mobile carousel slide he is not on, would be
  // resized by a device that cannot show him the result.
  const sessionsInView = () => {
    const inView = new Set<string>()
    visibleWindows.forEach((window, index) => {
      if (isMobile && index !== mobileActiveIndex) return
      if (window.activeSession) inView.add(window.activeSession)
    })
    return inView
  }

  const claimSessionsInView = () => {
    sessionsInView().forEach(sessionName => pool.terminals.get(sessionName)?.claim())
  }

  const refitTerminalLayout = () => {
    setRefitNonce(n => n + 1)
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

  const maintenanceGroups: MenuGroup[] = [
    {
      id: 'frames',
      rows: [
        {
          id: 'reconnect',
          label: 'Reconnect frames',
          disabled: visibleWindows.every(window => window.boundSessions.length === 0),
          onSelect: reconnectFrames,
        },
        {
          id: 'claim',
          label: 'Claim all sessions in view',
          reason: CLAIM_EXPLANATION,
          disabled: sessionsInView().size === 0,
          onSelect: claimSessionsInView,
        },
        { id: 'refit', label: 'Refit terminal layout', onSelect: refitTerminalLayout },
      ],
    },
  ]

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
          className="layout-btn terminal-maintenance-btn"
          aria-label="Terminal maintenance actions"
          title="Terminal maintenance actions"
          onClick={(event) => {
            const rect = event.currentTarget.getBoundingClientRect()
            setControlsMenu({ show: true, x: rect.right, y: rect.bottom })
          }}
        >
          ⋯
        </button>
      </div>

      {controlsMenu.show && (
        <Menu
          at={{ x: controlsMenu.x, y: controlsMenu.y }}
          label="Terminal maintenance actions"
          estimatedSize={{ width: 220, height: 130 }}
          onClose={closeControlsMenu}
          groups={maintenanceGroups}
        />
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
              workspaceActive={workspaceActive}
            />
          )
        })}
      </div>
    </div>
  )
}

export default TerminalArea
