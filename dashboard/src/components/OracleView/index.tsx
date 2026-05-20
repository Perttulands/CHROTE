// Agent observability dashboard

import { useCallback } from 'react'
import { useOracleStatus, useAgents, useOracleStream } from './hooks'
import AgentCard from './AgentCard'
import ActivityFeed from './ActivityFeed'

export default function OracleView() {
  const { status, loading: statusLoading } = useOracleStatus()
  const { agents, loading: agentsLoading, refresh: refreshAgents } = useAgents()
  const { connected, activity } = useOracleStream()

  const handleKill = useCallback(async (name: string) => {
    try {
      const response = await fetch(`/api/tmux/sessions/${encodeURIComponent(name)}`, {
        method: 'DELETE',
        signal: AbortSignal.timeout(10000),
      })
      if (response.ok) {
        refreshAgents()
      }
    } catch {
      // Silently fail — next poll will update
    }
  }, [refreshAgents])

  const isLoading = statusLoading || agentsLoading

  return (
    <div className="oracle-view">
      {/* Status Bar */}
      <div className="oracle-status-bar">
        <div className="oracle-status-items">
          <span className="oracle-stat">
            Agents: {status?.totalAgents ?? '--'}
          </span>
          <span className="oracle-stat oracle-stat-working">
            Working: {status?.workingAgents ?? '--'}
          </span>
          <span className="oracle-stat oracle-stat-idle">
            Idle: {status?.idleAgents ?? '--'}
          </span>
          <span className="oracle-stat">
            Beads: {status?.beadsActive ?? '--'}
          </span>
        </div>
        <div className="oracle-status-right">
          <span className={`oracle-sse-indicator ${connected ? 'oracle-sse-connected' : 'oracle-sse-disconnected'}`}>
            {connected ? 'LIVE' : 'OFF'}
          </span>
          <button
            className="oracle-refresh-btn"
            onClick={refreshAgents}
            disabled={isLoading}
          >
            {isLoading ? 'Loading...' : 'Refresh'}
          </button>
        </div>
      </div>

      {/* Main Content */}
      <div className="oracle-content">
        {/* Agent Grid */}
        <div className="oracle-agents-section">
          {agents.length === 0 && !agentsLoading ? (
            <div className="oracle-empty">
              No agent sessions detected. Start an agent-named tmux session such as claude-*, codex*, opencode*, hermes-*, gemini-*, or agent-* to see it here.
            </div>
          ) : (
            <div className="oracle-agents-grid">
              {agents.map(agent => (
                <AgentCard key={agent.name} agent={agent} onKill={handleKill} />
              ))}
            </div>
          )}
        </div>

        {/* Sidebar */}
        <div className="oracle-sidebar">
          {/* Activity Feed */}
          <div className="oracle-sidebar-section">
            <h3 className="oracle-section-title">Activity</h3>
            <ActivityFeed activity={activity} />
          </div>
        </div>
      </div>
    </div>
  )
}
