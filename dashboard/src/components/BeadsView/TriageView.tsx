// Triage recommendations view for Beads

import type { BeadsIssue, TriageResponse, ImpactLevel } from './types'
import IssueCard from './IssueCard'

interface TriageViewProps {
  triage: TriageResponse | null
  issues: BeadsIssue[]
  loading?: boolean
  error?: string | null
}

const IMPACT_COLORS: Record<ImpactLevel, string> = {
  high: 'var(--beads-impact-high)',
  medium: 'var(--beads-impact-medium)',
  low: 'var(--beads-impact-low)',
}

function getIssueById(issues: BeadsIssue[], id: string): BeadsIssue | undefined {
  return issues.find(i => i.id === id)
}

export default function TriageView({ triage, issues, loading, error }: TriageViewProps) {
  if (loading) {
    return (
      <div className="beads-triage loading">
        <div className="loading-message">Analyzing issues...</div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="beads-triage error">
        <div className="error-message">{error}</div>
      </div>
    )
  }

  if (!triage) {
    return (
      <div className="beads-triage empty">
        <div className="empty-message">No triage data available</div>
      </div>
    )
  }

  return (
    <div className="beads-triage">
      <div className="triage-sections">
        {/* Recommendations */}
        <section className="triage-section recommendations">
          <h3>Recommended Next</h3>
          {triage.recommendations.length === 0 ? (
            <p className="no-items">No recommendations</p>
          ) : (
            <div className="recommendation-list">
              {triage.recommendations.map(rec => {
                const issue = getIssueById(issues, rec.issueId)
                return (
                  <div key={rec.issueId} className="recommendation-item">
                    <div className="rec-header">
                      <span className="rec-rank">#{rec.rank}</span>
                      <span
                        className="rec-impact"
                        style={{ color: IMPACT_COLORS[rec.estimatedImpact] }}
                      >
                        {rec.estimatedImpact} impact
                      </span>
                    </div>
                    {issue ? (
                      <IssueCard issue={issue} />
                    ) : (
                      <div className="issue-card missing">
                        <span className="issue-id">{rec.issueId}</span>
                        <span className="issue-missing">(issue not found)</span>
                      </div>
                    )}
                    <div className="rec-reasoning">{rec.reasoning}</div>
                  </div>
                )
              })}
            </div>
          )}
        </section>

        {/* Quick Wins */}
        <section className="triage-section quick-wins">
          <h3>Quick Wins</h3>
          <p className="section-desc">Low-effort issues with no dependencies</p>
          {triage.quickWins.length === 0 ? (
            <p className="no-items">No quick wins identified</p>
          ) : (
            <div className="quick-wins-list">
              {triage.quickWins.map(id => {
                const issue = getIssueById(issues, id)
                return issue ? (
                  <IssueCard key={id} issue={issue} compact />
                ) : (
                  <div key={id} className="issue-card compact missing">
                    <span className="issue-id">{id}</span>
                  </div>
                )
              })}
            </div>
          )}
        </section>

        {/* Blockers */}
        <section className="triage-section blockers">
          <h3>Blockers</h3>
          <p className="section-desc">Issues blocking other work</p>
          {triage.blockers.length === 0 ? (
            <p className="no-items">No blockers identified</p>
          ) : (
            <div className="blockers-list">
              {triage.blockers.map(id => {
                const issue = getIssueById(issues, id)
                return issue ? (
                  <IssueCard key={id} issue={issue} showDependencies />
                ) : (
                  <div key={id} className="issue-card missing">
                    <span className="issue-id">{id}</span>
                  </div>
                )
              })}
            </div>
          )}
        </section>
      </div>
    </div>
  )
}
