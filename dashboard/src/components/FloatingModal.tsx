import { useEffect, useRef, useState, useCallback } from 'react'
import { useSession } from '../context/SessionContext'
import { getSessionKey, getSessionNameFromKey, getSessionUserFromKey } from '../types'

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
  const [loaded, setLoaded] = useState(false)
  const [position, setPosition] = useState(initialRect.current.position)
  const [size, setSize] = useState(initialRect.current.size)
  const [isDragging, setIsDragging] = useState(false)
  const dragOffset = useRef({ x: 0, y: 0 })
  const iframeRef = useRef<HTMLIFrameElement>(null)
  const bodyRef = useRef<HTMLDivElement>(null)

  const triggerFit = useCallback(() => {
    try {
      // Direct term.fit(): the ttyd client only hears dispatched resize
      // events after its WebSocket opens; term.fit exists from open() onward.
      const frameWindow = iframeRef.current?.contentWindow as (Window & { term?: { fit?: () => void } }) | null
      frameWindow?.term?.fit?.()
    } catch {
      // Cross-origin or not ready.
    }
  }, [])

  useEffect(() => {
    if (!floatingSession) return
    const next = viewportRect()
    setLoaded(false)
    setPosition(next.position)
    setSize(next.size)
    setIsDragging(false)
  }, [floatingSession])

  useEffect(() => {
    if (!loaded) return
    let cancelled = false
    let attempts = 0
    let timer: ReturnType<typeof setTimeout> | undefined
    const apply = () => {
      if (cancelled) return
      try {
        const frameWindow = iframeRef.current?.contentWindow as (Window & { term?: { options: { fontSize: number } } }) | null
        if (frameWindow?.term) {
          frameWindow.term.options.fontSize = settings.fontSize
          // Font-then-fit: size the grid with the final cell metrics. This
          // also covers the initial fit — term.fit() works pre-socket.
          triggerFit()
          return
        }
      } catch {
        // Cross-origin or not ready.
      }
      attempts += 1
      if (attempts < 20) timer = setTimeout(apply, 50)
    }
    apply()
    return () => {
      cancelled = true
      if (timer) clearTimeout(timer)
    }
  }, [floatingSession, loaded, settings.fontSize, triggerFit])

  useEffect(() => {
    const body = bodyRef.current
    if (!body) return
    let timer: ReturnType<typeof setTimeout> | undefined
    const observer = new ResizeObserver(() => {
      if (timer) clearTimeout(timer)
      timer = setTimeout(triggerFit, 100)
    })
    observer.observe(body)
    return () => {
      if (timer) clearTimeout(timer)
      observer.disconnect()
    }
  }, [floatingSession, triggerFit])

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

  const displayName = getSessionNameFromKey(floatingSession)
  const keyUser = getSessionUserFromKey(floatingSession)
  const matchingSessions = keyUser
    ? sessions.filter(item => getSessionKey(item.name, item.unixUser) === floatingSession)
    : sessions.filter(item => item.name === displayName)
  const session = matchingSessions.length === 1 ? matchingSessions[0] : undefined
  const unixUser = session?.unixUser ?? keyUser
  const canOpenSession = Boolean(session || unixUser.trim())
  const userArg = unixUser.trim() ? `&arg=${encodeURIComponent(unixUser)}` : ''

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
            {!loaded && canOpenSession && <span className="terminal-loading-state">Loading terminal…</span>}
            <button className="modal-send" onClick={() => openSendToSession(floatingSession)}>Send to Session</button>
            <button className="modal-close" onClick={closeFloatingModal}>×</button>
          </div>
        </div>
        <div ref={bodyRef} className="floating-modal-body">
          {canOpenSession ? (
            <iframe
              ref={iframeRef}
              key={floatingSession}
              src={`/terminal/?arg=${encodeURIComponent(displayName)}${userArg}`}
              scrolling="no"
              onLoad={() => setLoaded(true)}
              style={{ width: '100%', height: '100%', border: 'none', backgroundColor: '#0a0a0a', overflow: 'hidden' }}
              title={`Terminal - ${displayName}`}
            />
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
