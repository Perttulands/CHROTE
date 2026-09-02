import { useEffect, useMemo, useRef, useState } from 'react'
import { useSession } from '../context/SessionContext'
import { getSessionKey, getSessionNameFromKey, getSessionUserFromKey } from '../types'
import TerminalSurface, { useTerminalSession } from './TerminalSurface'
import { terminalSocketUrl } from '../terminal/ttydProtocol'
import { isSessionEnded } from '../terminal/tileState'
import { useSessionEvidence } from '../context/useSessionEvidence'

const VIEWPORT_MARGIN = 16

function viewportRect() {
  const width = Math.min(1000, Math.max(280, window.innerWidth - VIEWPORT_MARGIN * 2))
  const height = Math.min(700, Math.max(240, window.innerHeight - VIEWPORT_MARGIN * 2))
  return {
    size: { width, height },
    position: {
      x: Math.max(VIEWPORT_MARGIN, (window.innerWidth - width) / 2),
      y: Math.max(VIEWPORT_MARGIN, (window.innerHeight - height) / 2),
    },
  }
}

function FloatingModal() {
  const { floatingSession, closeFloatingModal, openSendToSession, settings, sessions } = useSession()
  const initialRect = useRef(viewportRect())
  const [position, setPosition] = useState(initialRect.current.position)
  const [size, setSize] = useState(initialRect.current.size)
  const [isDragging, setIsDragging] = useState(false)
  const dragOffset = useRef({ x: 0, y: 0 })

  const displayName = floatingSession ? getSessionNameFromKey(floatingSession) : ''
  const keyUser = floatingSession ? getSessionUserFromKey(floatingSession) : ''
  const matchingSessions = !floatingSession
    ? []
    : keyUser
      ? sessions.filter(item => getSessionKey(item.name, item.unixUser) === floatingSession)
      : sessions.filter(item => item.name === displayName)
  const session = matchingSessions.length === 1 ? matchingSessions[0] : undefined
  const unixUser = session?.unixUser ?? keyUser
  const canOpenSession = Boolean(floatingSession && (session || unixUser.trim()))

  // The same join the tile makes, asked of the same answer. A glance at a
  // session tmux no longer lists is entitled to the same explanation the tile
  // gives, rather than a dead terminal and no reason for it.
  const evidence = useSessionEvidence()
  const ended = floatingSession !== null && isSessionEnded(floatingSession, evidence)

  // Peek owns its terminal for the life of the modal: it is a second observer
  // of the session, not the tile's terminal moved onto the overlay. It attaches
  // as an observer, so it never displaces the tile or resizes the window.
  const socketUrl = useMemo(
    () => (canOpenSession ? terminalSocketUrl(displayName, unixUser, 'peek') : null),
    [canOpenSession, displayName, unixUser],
  )
  // The URL is kept even once the session has ended, so the terminal holding
  // the last frame is not disposed; `connect` is what stops it dialling again.
  const { session: terminal, connectionState } = useTerminalSession(socketUrl, settings.fontSize, settings.hideScrollbar)

  useEffect(() => {
    if (!floatingSession) return
    const next = viewportRect()
    setPosition(next.position)
    setSize(next.size)
    setIsDragging(false)
  }, [floatingSession])

  useEffect(() => {
    if (!floatingSession) return
    const clampToViewport = () => {
      const next = viewportRect()
      setSize(next.size)
      setPosition(current => ({
        x: Math.min(Math.max(VIEWPORT_MARGIN, current.x), Math.max(VIEWPORT_MARGIN, window.innerWidth - next.size.width - VIEWPORT_MARGIN)),
        y: Math.min(Math.max(VIEWPORT_MARGIN, current.y), Math.max(VIEWPORT_MARGIN, window.innerHeight - next.size.height - VIEWPORT_MARGIN)),
      }))
    }
    window.addEventListener('resize', clampToViewport)
    return () => window.removeEventListener('resize', clampToViewport)
  }, [floatingSession])

  const handleMouseDown = (event: React.MouseEvent) => {
    if ((event.target as HTMLElement).closest('button')) return
    setIsDragging(true)
    dragOffset.current = {
      x: event.clientX - position.x,
      y: event.clientY - position.y,
    }
  }

  useEffect(() => {
    if (!isDragging) return
    const handleMouseMove = (event: MouseEvent) => {
      const maxX = Math.max(VIEWPORT_MARGIN, window.innerWidth - size.width - VIEWPORT_MARGIN)
      const maxY = Math.max(VIEWPORT_MARGIN, window.innerHeight - size.height - VIEWPORT_MARGIN)
      setPosition({
        x: Math.min(maxX, Math.max(VIEWPORT_MARGIN, event.clientX - dragOffset.current.x)),
        y: Math.min(maxY, Math.max(VIEWPORT_MARGIN, event.clientY - dragOffset.current.y)),
      })
    }
    const handleMouseUp = () => setIsDragging(false)
    document.addEventListener('mousemove', handleMouseMove)
    document.addEventListener('mouseup', handleMouseUp)
    return () => {
      document.removeEventListener('mousemove', handleMouseMove)
      document.removeEventListener('mouseup', handleMouseUp)
    }
  }, [isDragging, size.height, size.width])

  if (!floatingSession) return null

  return (
    <div className="floating-modal-overlay" onClick={closeFloatingModal}>
      <div
        className="floating-modal"
        style={{ left: position.x, top: position.y, width: size.width, height: size.height }}
        onClick={event => event.stopPropagation()}
      >
        <div className="floating-modal-header" onMouseDown={handleMouseDown}>
          <span className="modal-title">{displayName}</span>
          <div className="modal-controls">
            {canOpenSession && !ended && connectionState !== 'open' && (
              <span className="terminal-loading-state">
                {connectionState === 'closed' ? 'Terminal disconnected' : 'Loading terminal…'}
              </span>
            )}
            <button className="modal-send" onClick={() => openSendToSession(floatingSession)}>Send to Session</button>
            <button className="modal-close" onClick={closeFloatingModal}>×</button>
          </div>
        </div>
        <div className={ended ? 'floating-modal-body detached' : 'floating-modal-body'}>
          {canOpenSession ? (
            <>
              <TerminalSurface session={terminal} connect={!ended} />
              {ended && (
                <div className="terminal-tile-detached" data-tile-state="ended" role="status">
                  <span className="terminal-tile-detached-note">
                    {displayName} ended. This frame shows its last output.
                  </span>
                </div>
              )}
            </>
          ) : (
            <div className="empty-window-content">Ambiguous legacy session name; attach the user-qualified session from the session list.</div>
          )}
        </div>
      </div>
    </div>
  )
}

export default FloatingModal
import './FloatingModal.css'
