// Agent card component for the agent monitor.

import { useState, useEffect, useCallback } from 'react'
import type { OracleAgent, AgentStatus } from './types'

interface AgentCardProps {
  agent: OracleAgent
  onKill: (name: string) => void
}

const STATUS_LABELS: Record<AgentStatus, string> = {
  working: 'Working',
  idle: 'Idle',
  complete: 'Complete',
  error: 'Error',
}

function formatElapsed(startedAt?: string): string {
  if (!startedAt) return '--'
  const start = new Date(startedAt).getTime()
  if (isNaN(start)) return '--'
  const elapsed = Math.floor((Date.now() - start) / 1000)
  if (elapsed < 60) return `${elapsed}s`
  if (elapsed < 3600) return `${Math.floor(elapsed / 60)}m ${elapsed % 60}s`
  const hours = Math.floor(elapsed / 3600)
  const mins = Math.floor((elapsed % 3600) / 60)
  return `${hours}h ${mins}m`
}

export default function AgentCard({ agent, onKill }: AgentCardProps) {
  const [elapsed, setElapsed] = useState(formatElapsed(agent.startedAt))
  const [confirmKill, setConfirmKill] = useState(false)

  // Live timer
  useEffect(() => {
    if (!agent.startedAt) return
    const interval = setInterval(() => {
      setElapsed(formatElapsed(agent.startedAt))
    }, 1000)
    return () => clearInterval(interval)
  }, [agent.startedAt])

  const handleKill = useCallback(() => {
    if (!confirmKill) {
      setConfirmKill(true)
      setTimeout(() => setConfirmKill(false), 3000)
      return
    }
    onKill(agent.name)
    setConfirmKill(false)
  }, [confirmKill, agent.name, onKill])

  const displayName = agent.name

  return (
    <div className={`oracle-agent-card oracle-status-${agent.status}`}>
      <div className="oracle-agent-header">
        <div className="oracle-agent-name" title={agent.name}>
          {displayName}
        </div>
        <span className={`oracle-status-badge oracle-badge-${agent.status}`}>
          {STATUS_LABELS[agent.status]}
        </span>
      </div>

      <div className="oracle-agent-meta">
        <span className="oracle-agent-runtime">{elapsed}</span>
        {agent.beadId && (
          <span className="oracle-agent-bead" title={`Bead: ${agent.beadId}`}>
            {agent.beadId}
          </span>
        )}
        {agent.attached && <span className="oracle-agent-attached" title="Session attached">att</span>}
      </div>

      <div className="oracle-context-bar">
        <div
          className="oracle-context-fill"
          style={{ width: `${agent.contextPct}%` }}
        />
        <span className="oracle-context-label">
          {agent.contextPct > 0 ? `${agent.contextPct}%` : '--'}
        </span>
      </div>

      {agent.lastLines.length > 0 && (
        <div className="oracle-agent-output">
          {agent.lastLines.map((line, i) => (
            <div key={i} className="oracle-output-line">{line}</div>
          ))}
        </div>
      )}

      <div className="oracle-agent-actions">
        <button
          className={`oracle-kill-btn ${confirmKill ? 'oracle-kill-confirm' : ''}`}
          onClick={handleKill}
          title={confirmKill ? 'Click again to confirm' : 'Kill session'}
        >
          {confirmKill ? 'Confirm Kill' : 'Kill'}
        </button>
      </div>
    </div>
  )
}
