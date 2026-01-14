import { useState, useEffect } from 'react'

interface SessionInfo {
  name: string
  status: 'active' | 'idle' | 'unknown'
  attached: boolean
}

function StatusView() {
  const [sessions, setSessions] = useState<SessionInfo[]>([])
  const [lastUpdated, setLastUpdated] = useState<Date | null>(null)

  // Placeholder - in a real implementation, this would fetch from an API
  // that runs `tmux list-sessions` and parses the output
  useEffect(() => {
    // Simulated session data
    setSessions([
      { name: 'main', status: 'active', attached: false },
    ])
    setLastUpdated(new Date())
  }, [])

  return (
    <div className="status-view">
      <div className="status-card">
        <h3>Gastown Status</h3>
        <div className="status-item">
          <span className="label">Dashboard</span>
          <span className="value" style={{ color: '#00ff41' }}>Online</span>
        </div>
        <div className="status-item">
          <span className="label">ttyd</span>
          <span className="value" style={{ color: '#00ff41' }}>Running</span>
        </div>
        <div className="status-item">
          <span className="label">Last Updated</span>
          <span className="value">
            {lastUpdated ? lastUpdated.toLocaleTimeString() : 'Never'}
          </span>
        </div>
      </div>

      <div className="status-card">
        <h3>tmux Sessions</h3>
        {sessions.length === 0 ? (
          <div className="status-item">
            <span className="label">No sessions found</span>
          </div>
        ) : (
          sessions.map((session) => (
            <div key={session.name} className="status-item">
              <span className="label">{session.name}</span>
              <span
                className="value"
                style={{
                  color: session.status === 'active' ? '#00ff41' : '#00cc33',
                }}
              >
                {session.status}
                {session.attached && ' (attached)'}
              </span>
            </div>
          ))
        )}
      </div>

      <div className="status-card">
        <h3>Quick Commands</h3>
        <p style={{ color: 'var(--text-secondary)', fontSize: '13px', lineHeight: '1.6' }}>
          From any terminal pane, run:
        </p>
        <ul style={{
          color: 'var(--text-dim)',
          fontSize: '12px',
          lineHeight: '2',
          listStyle: 'none',
          marginTop: '8px'
        }}>
          <li><code style={{ color: 'var(--text-primary)' }}>tmux ls</code> - List all sessions</li>
          <li><code style={{ color: 'var(--text-primary)' }}>tmux attach -t &lt;name&gt;</code> - Attach to session</li>
          <li><code style={{ color: 'var(--text-primary)' }}>gt peek</code> - Monitor Gastown activity</li>
          <li><code style={{ color: 'var(--text-primary)' }}>gt status</code> - View agent status</li>
        </ul>
      </div>
    </div>
  )
}

export default StatusView
