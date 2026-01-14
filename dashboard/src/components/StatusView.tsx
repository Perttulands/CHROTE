import { useState, useEffect, useCallback } from 'react'
import type { TmuxSession, SessionsResponse } from '../types'

type ServiceStatus = 'online' | 'offline' | 'checking'

interface ServiceHealth {
  api: ServiceStatus
  tmux: ServiceStatus
}

function StatusView() {
  const [sessions, setSessions] = useState<TmuxSession[]>([])
  const [health, setHealth] = useState<ServiceHealth>({ api: 'checking', tmux: 'checking' })
  const [lastUpdated, setLastUpdated] = useState<Date | null>(null)
  const [error, setError] = useState<string | null>(null)

  const fetchStatus = useCallback(async () => {
    try {
      // Fetch API health
      const healthRes = await fetch('/api/health')
      if (healthRes.ok) {
        setHealth(prev => ({ ...prev, api: 'online' }))
      } else {
        setHealth(prev => ({ ...prev, api: 'offline' }))
      }
    } catch (e) {
      console.warn('API health check failed:', e)
      setHealth(prev => ({ ...prev, api: 'offline' }))
    }

    try {
      // Fetch tmux sessions
      const sessionsRes = await fetch('/api/tmux/sessions')
      if (sessionsRes.ok) {
        const data: SessionsResponse = await sessionsRes.json()
        setSessions(data.sessions)
        setHealth(prev => ({ ...prev, tmux: 'online' }))
        setError(data.error || null)
      } else {
        setHealth(prev => ({ ...prev, tmux: 'offline' }))
        setSessions([])
      }
    } catch (e) {
      console.warn('Tmux sessions fetch failed:', e)
      setHealth(prev => ({ ...prev, tmux: 'offline' }))
      setSessions([])
    }

    setLastUpdated(new Date())
  }, [])

  useEffect(() => {
    fetchStatus()
    const interval = setInterval(fetchStatus, 5000)
    return () => clearInterval(interval)
  }, [fetchStatus])

  const getStatusColor = (status: ServiceStatus): string => {
    switch (status) {
      case 'online': return '#00ff41'
      case 'offline': return '#ff4141'
      case 'checking': return '#ffaa00'
    }
  }

  const getStatusText = (status: ServiceStatus): string => {
    switch (status) {
      case 'online': return 'Online'
      case 'offline': return 'Offline'
      case 'checking': return 'Checking...'
    }
  }

  return (
    <div className="status-view">
      <div className="status-card">
        <h3>Gastown Status</h3>
        <div className="status-item">
          <span className="label">API Server</span>
          <span className="value" style={{ color: getStatusColor(health.api) }}>
            {getStatusText(health.api)}
          </span>
        </div>
        <div className="status-item">
          <span className="label">tmux</span>
          <span className="value" style={{ color: getStatusColor(health.tmux) }}>
            {health.tmux === 'online' ? `${sessions.length} session${sessions.length !== 1 ? 's' : ''}` : getStatusText(health.tmux)}
          </span>
        </div>
        <div className="status-item">
          <span className="label">Last Updated</span>
          <span className="value">
            {lastUpdated ? lastUpdated.toLocaleTimeString() : 'Never'}
          </span>
        </div>
        {error && (
          <div className="status-item">
            <span className="label">Error</span>
            <span className="value" style={{ color: '#ff4141' }}>{error}</span>
          </div>
        )}
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
              <span className="label">{session.agentName || session.name}</span>
              <span
                className="value"
                style={{
                  color: session.attached ? '#00ff41' : '#00cc33',
                }}
              >
                {session.windows} window{session.windows !== 1 ? 's' : ''}
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
