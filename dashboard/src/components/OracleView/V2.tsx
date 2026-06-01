// Agent observability dashboard — V2 experiment

import { useCallback } from 'react'
import { RefreshCw, Radio, Activity, Zap, Coffee, CircleDot } from 'lucide-react'
import { useOracleStatus, useAgents, useOracleStream } from './hooks'
import AgentCard from './AgentCard'
import ActivityFeed from './ActivityFeed'
import { SkeletonCard } from '../LoadingSkeleton'

export default function OracleViewV2() {
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
            <Activity size={14} />
            Agents: {status?.totalAgents ?? '--'}
          </span>
          <span className="oracle-stat oracle-stat-working">
            <Zap size={14} />
            Working: {status?.workingAgents ?? '--'}
          </span>
          <span className="oracle-stat oracle-stat-idle">
            <Coffee size={14} />
            Idle: {status?.idleAgents ?? '--'}
          </span>
          <span className="oracle-stat">
            <CircleDot size={14} />
            Beads: {status?.beadsActive ?? '--'}
          </span>
        </div>
        <div className="oracle-status-right">
          <span className={`oracle-sse-indicator ${connected ? 'oracle-sse-connected' : 'oracle-sse-disconnected'}`}>
            <Radio size={12} />
            {connected ? 'LIVE' : 'OFF'}
          </span>
          <button
            className="oracle-refresh-btn"
            onClick={refreshAgents}
            disabled={isLoading}
          >
            <RefreshCw size={14} className={isLoading ? 'spinning' : ''} />
            {isLoading ? 'Loading...' : 'Refresh'}
          </button>
        </div>
      </div>

      {/* Main Content */}
      <div className="oracle-content">
        {/* Agent Grid */}
        <div className="oracle-agents-section">
          {isLoading && agents.length === 0 ? (
            <SkeletonCard count={4} />
          ) : agents.length === 0 && !agentsLoading ? (
            <div className="oracle-empty">
              <Activity size={32} className="oracle-empty-icon" />
              <p>No agent sessions detected.</p>
              <p className="oracle-empty-hint">
                Start an agent-named tmux session such as claude-*, codex*, opencode*, hermes-*, gemini-*, or agent-* to see it here.
              </p>
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
