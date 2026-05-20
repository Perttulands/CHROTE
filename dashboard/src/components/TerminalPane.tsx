import { useState } from 'react'

interface TerminalPaneProps {
  sessionName: string
  onClose?: () => void
}

function TerminalPane({ sessionName, onClose }: TerminalPaneProps) {
  const [loaded, setLoaded] = useState(false)

  // ttyd serves its own terminal UI - embed it via iframe
  const terminalUrl = `/terminal/`

  return (
    <div className="terminal-pane">
      <div className="terminal-header">
        <span className="session-name">{sessionName}</span>
        <div className="status">
          <span className={`status-dot ${loaded ? '' : 'disconnected'}`} />
          <span>{loaded ? 'Connected' : 'Loading...'}</span>
          {onClose && (
            <button
              onClick={onClose}
              style={{
                background: 'transparent',
                border: 'none',
                color: 'var(--text-dim)',
                cursor: 'pointer',
                marginLeft: '8px',
                fontSize: '14px',
              }}
            >
              x
            </button>
          )}
        </div>
      </div>
      <div className="terminal-body">
        <iframe
          src={terminalUrl}
          onLoad={() => setLoaded(true)}
          style={{
            width: '100%',
            height: '100%',
            border: 'none',
            backgroundColor: '#0a0a0a',
          }}
          title={`Terminal: ${sessionName}`}
        />
      </div>
    </div>
  )
}

export default TerminalPane
