// Insights/metrics dashboard view for Beads

import type { InsightsResponse } from './types'

interface InsightsViewProps {
  insights: InsightsResponse | null
  loading?: boolean
}

function HealthGauge({ score }: { score: number }) {
  const getColor = (score: number) => {
    if (score >= 80) return 'var(--beads-health-good)'
    if (score >= 50) return 'var(--beads-health-warning)'
    return 'var(--beads-health-critical)'
  }

  return (
    <div className="health-gauge">
      <div className="gauge-circle">
        <svg viewBox="0 0 100 100">
          <circle
            cx="50"
            cy="50"
            r="45"
            fill="none"
            stroke="var(--bg-tertiary)"
            strokeWidth="8"
          />
          <circle
            cx="50"
            cy="50"
            r="45"
            fill="none"
            stroke={getColor(score)}
            strokeWidth="8"
            strokeLinecap="round"
            strokeDasharray={`${score * 2.83} 283`}
            transform="rotate(-90 50 50)"
          />
        </svg>
        <div className="gauge-value">{score}</div>
      </div>
      <div className="gauge-label">Health Score</div>
    </div>
  )
}

function StatCard({ label, value, variant }: { label: string; value: number | string; variant?: string }) {
  return (
    <div className={`stat-card ${variant || ''}`}>
      <div className="stat-value">{value}</div>
      <div className="stat-label">{label}</div>
    </div>
  )
}

export default function InsightsView({ insights, loading }: InsightsViewProps) {
  if (loading) {
    return (
      <div className="beads-insights loading">
        <div className="loading-message">Calculating insights...</div>
      </div>
    )
  }

  if (!insights) {
    return (
      <div className="beads-insights empty">
        <div className="empty-message">No insights available</div>
      </div>
    )
  }

  return (
    <div className="beads-insights">
      <div className="insights-grid">
        {/* Health Score */}
        <div className="insights-section health-section">
          <HealthGauge score={insights.health.score} />
        </div>

        {/* Stats Overview */}
        <div className="insights-section stats-section">
          <h3>Overview</h3>
          <div className="stats-grid">
            <StatCard label="Total Issues" value={insights.issueCount} />
            <StatCard label="Open" value={insights.openCount} variant="open" />
            <StatCard label="Blocked" value={insights.blockedCount} variant="blocked" />
            <StatCard label="Closed" value={insights.closedCount || 0} variant="closed" />
          </div>
        </div>

        {/* By Status */}
        {insights.byStatus && Object.keys(insights.byStatus).length > 0 && (
          <div className="insights-section breakdown-section">
            <h3>By Status</h3>
            <div className="breakdown-list">
              {Object.entries(insights.byStatus).map(([status, count]) => (
                <div key={status} className="breakdown-item">
                  <span className="breakdown-label">{status.replace(/_/g, ' ')}</span>
                  <span className="breakdown-value">{count}</span>
                  <div
                    className="breakdown-bar"
                    style={{ width: `${(count / insights.issueCount) * 100}%` }}
                  />
                </div>
              ))}
            </div>
          </div>
        )}

        {/* By Type */}
        {insights.byType && Object.keys(insights.byType).length > 0 && (
          <div className="insights-section breakdown-section">
            <h3>By Type</h3>
            <div className="breakdown-list">
              {Object.entries(insights.byType).map(([type, count]) => (
                <div key={type} className="breakdown-item">
                  <span className="breakdown-label">{type}</span>
                  <span className="breakdown-value">{count}</span>
                  <div
                    className="breakdown-bar type-bar"
                    style={{ width: `${(count / insights.issueCount) * 100}%` }}
                  />
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Risks */}
        {insights.health.risks.length > 0 && (
          <div className="insights-section risks-section">
            <h3>Risks</h3>
            <ul className="risks-list">
              {insights.health.risks.map((risk, i) => (
                <li key={i} className="risk-item">{risk}</li>
              ))}
            </ul>
          </div>
        )}

        {/* Warnings */}
        {insights.health.warnings.length > 0 && (
          <div className="insights-section warnings-section">
            <h3>Warnings</h3>
            <ul className="warnings-list">
              {insights.health.warnings.map((warning, i) => (
                <li key={i} className="warning-item">{warning}</li>
              ))}
            </ul>
          </div>
        )}

        {/* Advanced Metrics (if available from bv) */}
        {insights.metrics && (
          <div className="insights-section metrics-section">
            <h3>Graph Metrics</h3>
            <div className="metrics-content">
              {insights.metrics.density !== undefined && (
                <div className="metric-item">
                  <span className="metric-label">Graph Density</span>
                  <span className="metric-value">{(insights.metrics.density * 100).toFixed(1)}%</span>
                </div>
              )}
              {insights.metrics.cycles && insights.metrics.cycles.length > 0 && (
                <div className="metric-item warning">
                  <span className="metric-label">Circular Dependencies</span>
                  <span className="metric-value">{insights.metrics.cycles.length}</span>
                </div>
              )}
              {insights.metrics.criticalPath && insights.metrics.criticalPath.length > 0 && (
                <div className="metric-item">
                  <span className="metric-label">Critical Path Length</span>
                  <span className="metric-value">{insights.metrics.criticalPath.length}</span>
                </div>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
