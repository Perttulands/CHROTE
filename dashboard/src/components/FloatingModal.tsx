import { useEffect, useRef, useState, useCallback } from 'react'
import { useSession } from '../context/SessionContext'

function FloatingModal() {
  const { floatingSession, closeFloatingModal, settings } = useSession()
  const [loaded, setLoaded] = useState(false)
  const [position, setPosition] = useState({ x: 80, y: 60 })
  const [size] = useState({ width: 1000, height: 700 })
  const [isDragging, setIsDragging] = useState(false)
  const dragOffset = useRef({ x: 0, y: 0 })
  const iframeRef = useRef<HTMLIFrameElement>(null)
  const bodyRef = useRef<HTMLDivElement>(null)

  // Reset loaded state when session changes
  useEffect(() => {
    setLoaded(false)
  }, [floatingSession])

  // Trigger xterm fit() by dispatching resize event to iframe
  const triggerFit = useCallback(() => {
    try {
      const iframe = iframeRef.current
      if (!iframe?.contentWindow) return
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

  // Apply font size when iframe loads, and trigger fit after delays
  const handleIframeLoad = useCallback(() => {
    setLoaded(true)
    applyFontSize(settings.fontSize)
    setTimeout(triggerFit, 100)
    setTimeout(triggerFit, 300)
    setTimeout(triggerFit, 500)
  }, [applyFontSize, settings.fontSize, triggerFit])

  // Apply font size when setting changes
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
      timeoutId = setTimeout(triggerFit, 100)
    })

    observer.observe(body)
    return () => {
      clearTimeout(timeoutId)
      observer.disconnect()
    }
  }, [triggerFit])

  // Dragging handlers
  const handleMouseDown = (e: React.MouseEvent) => {
    if ((e.target as HTMLElement).closest('.modal-close')) return
    setIsDragging(true)
    dragOffset.current = {
      x: e.clientX - position.x,
      y: e.clientY - position.y,
    }
  }

  useEffect(() => {
    if (!isDragging) return

    const handleMouseMove = (e: MouseEvent) => {
      setPosition({
        x: e.clientX - dragOffset.current.x,
        y: e.clientY - dragOffset.current.y,
      })
    }

    const handleMouseUp = () => {
      setIsDragging(false)
    }

    document.addEventListener('mousemove', handleMouseMove)
    document.addEventListener('mouseup', handleMouseUp)

    return () => {
      document.removeEventListener('mousemove', handleMouseMove)
      document.removeEventListener('mouseup', handleMouseUp)
    }
  }, [isDragging])

  if (!floatingSession) return null

  // Extract display name
  const displayName = floatingSession.includes('-')
    ? floatingSession.split('-').slice(-1)[0]
    : floatingSession

  return (
    <div className="floating-modal-overlay" onClick={closeFloatingModal}>
      <div
        className="floating-modal"
        style={{
          left: position.x,
          top: position.y,
          width: size.width,
          height: size.height,
        }}
        onClick={e => e.stopPropagation()}
      >
        <div
          className="floating-modal-header"
          onMouseDown={handleMouseDown}
        >
          <span className="modal-title">{displayName}</span>
          <div className="modal-controls">
            <span className={`status-dot ${loaded ? '' : 'disconnected'}`} />
            <button className="modal-close" onClick={closeFloatingModal}>×</button>
          </div>
        </div>
        <div ref={bodyRef} className="floating-modal-body">
          <iframe
            ref={iframeRef}
            key={floatingSession}
            src={`/terminal/?arg=${encodeURIComponent(floatingSession)}`}
            onLoad={handleIframeLoad}
            style={{
              width: '100%',
              height: '100%',
              border: 'none',
              backgroundColor: '#0a0a0a',
            }}
            title={`Terminal - ${floatingSession}`}
          />
        </div>
      </div>
    </div>
  )
}

export default FloatingModal
