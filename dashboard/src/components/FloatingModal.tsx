import { useEffect, useRef, useState } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { WebglAddon } from '@xterm/addon-webgl'
import { AttachAddon } from '@xterm/addon-attach'
import '@xterm/xterm/css/xterm.css'
import { useSession } from '../context/SessionContext'

function FloatingModal() {
  const { floatingSession, closeFloatingModal } = useSession()
  const terminalRef = useRef<HTMLDivElement>(null)
  const terminalInstance = useRef<Terminal | null>(null)
  const wsRef = useRef<WebSocket | null>(null)
  const [connected, setConnected] = useState(false)
  const [position, setPosition] = useState({ x: 100, y: 100 })
  const [size] = useState({ width: 600, height: 400 })
  const [isDragging, setIsDragging] = useState(false)
  const dragOffset = useRef({ x: 0, y: 0 })

  // Initialize terminal when modal opens
  useEffect(() => {
    if (!floatingSession || !terminalRef.current) return

    const terminal = new Terminal({
      cursorBlink: true,
      cursorStyle: 'block',
      fontFamily: "'Courier Prime', 'Courier New', monospace",
      fontSize: 13,
      lineHeight: 1.2,
      theme: {
        background: '#0a0a0a',
        foreground: '#00ff41',
        cursor: '#00ff41',
        cursorAccent: '#000000',
        selectionBackground: '#00ff4140',
        black: '#000000',
        red: '#ff4141',
        green: '#00ff41',
        yellow: '#ffff41',
        blue: '#4141ff',
        magenta: '#ff41ff',
        cyan: '#41ffff',
        white: '#ffffff',
        brightBlack: '#666666',
        brightRed: '#ff6666',
        brightGreen: '#66ff66',
        brightYellow: '#ffff66',
        brightBlue: '#6666ff',
        brightMagenta: '#ff66ff',
        brightCyan: '#66ffff',
        brightWhite: '#ffffff',
      },
    })

    terminalInstance.current = terminal

    const fitAddon = new FitAddon()
    terminal.loadAddon(fitAddon)

    terminal.open(terminalRef.current)

    try {
      const webglAddon = new WebglAddon()
      terminal.loadAddon(webglAddon)
      webglAddon.onContextLoss(() => webglAddon.dispose())
    } catch (e) {
      console.warn('WebGL addon failed to load')
    }

    fitAddon.fit()

    // Connect to ttyd
    const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsUrl = `${protocol}//${location.host}/terminal/ws`
    const ws = new WebSocket(wsUrl)
    wsRef.current = ws

    ws.onopen = () => {
      setConnected(true)
      const attachAddon = new AttachAddon(ws)
      terminal.loadAddon(attachAddon)

      // Attach to the session
      setTimeout(() => {
        terminal.paste(`tmux attach-session -t ${floatingSession}\n`)
      }, 100)
    }

    ws.onclose = () => setConnected(false)
    ws.onerror = () => setConnected(false)

    const handleResize = () => fitAddon.fit()
    window.addEventListener('resize', handleResize)

    return () => {
      window.removeEventListener('resize', handleResize)
      ws.close()
      terminal.dispose()
    }
  }, [floatingSession])

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
            <span className={`status-dot ${connected ? '' : 'disconnected'}`} />
            <button className="modal-close" onClick={closeFloatingModal}>×</button>
          </div>
        </div>
        <div className="floating-modal-body" ref={terminalRef} />
      </div>
    </div>
  )
}

export default FloatingModal
