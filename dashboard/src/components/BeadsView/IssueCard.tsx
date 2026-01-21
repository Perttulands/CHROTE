// Reusable issue card component for Beads views

import type { BeadsIssue, IssueStatus, IssueType } from './types'

interface IssueCardProps {
  issue: BeadsIssue
  compact?: boolean
  showDependencies?: boolean
  highlighted?: boolean
}

const STATUS_COLORS: Record<IssueStatus, string> = {
  open: 'var(--beads-status-open)',
  in_progress: 'var(--beads-status-progress)',
  blocked: 'var(--beads-status-blocked)',
  closed: 'var(--beads-status-closed)',
  ready: 'var(--beads-status-ready)',
  wont_fix: 'var(--beads-status-closed)',
  duplicate: 'var(--beads-status-closed)',
  deferred: 'var(--beads-status-deferred)',
  hooked: 'var(--beads-status-hooked, var(--beads-status-progress))',
}

const TYPE_ICONS: Record<IssueType, string> = {
  bug: '\u{1F41B}',
  feature: '\u{2728}',
  task: '\u{2705}',
  chore: '\u{1F9F9}',
  epic: '\u{1F3AF}',
}

function getPriorityLabel(priority?: number): string {
  if (!priority) return ''
  if (priority === 1) return 'P1'
  if (priority === 2) return 'P2'
  if (priority === 3) return 'P3'
  return `P${priority}`
}

function getPriorityClass(priority?: number): string {
  if (!priority) return ''
  if (priority === 1) return 'priority-critical'
  if (priority === 2) return 'priority-high'
  if (priority === 3) return 'priority-medium'
  return 'priority-low'
}

function formatStatus(status: IssueStatus): string {
  return status.replace(/_/g, ' ')
}

export default function IssueCard({ issue, compact = false, showDependencies = false, highlighted = false }: IssueCardProps) {
  const statusColor = STATUS_COLORS[issue.status] || 'var(--text-secondary)'
  const typeIcon = issue.type ? TYPE_ICONS[issue.type] || '' : ''

  if (compact) {
    return (
      <div className={`issue-card compact ${highlighted ? 'highlighted' : ''}`}>
        <span className="issue-id">{issue.id}</span>
        <span className="issue-title">{issue.title}</span>
        {issue.priority && (
          <span className={`issue-priority ${getPriorityClass(issue.priority)}`}>
            {getPriorityLabel(issue.priority)}
          </span>
        )}
      </div>
    )
  }

  return (
    <div className={`issue-card ${highlighted ? 'highlighted' : ''}`}>
      <div className="issue-card-header">
        <span className="issue-id">{issue.id}</span>
        <div className="issue-card-meta">
          {typeIcon && <span className="issue-type-icon" title={issue.type}>{typeIcon}</span>}
          {issue.priority && (
            <span className={`issue-priority ${getPriorityClass(issue.priority)}`}>
              {getPriorityLabel(issue.priority)}
            </span>
          )}
          <span className="issue-status" style={{ color: statusColor }}>
            {formatStatus(issue.status)}
          </span>
        </div>
      </div>
      <div className="issue-card-title">{issue.title}</div>
      {issue.assignee && (
        <div className="issue-card-assignee">
          <span className="assignee-label">Assignee:</span> {issue.assignee}
        </div>
      )}
      {showDependencies && issue.dependencies && issue.dependencies.length > 0 && (
        <div className="issue-card-deps">
          <span className="deps-label">Blocks:</span>{' '}
          {issue.dependencies.map((dep, i) => (
            <span key={dep} className="dep-id">
              {dep}{i < issue.dependencies!.length - 1 ? ', ' : ''}
            </span>
          ))}
        </div>
      )}
      {issue.labels && issue.labels.length > 0 && (
        <div className="issue-card-labels">
          {issue.labels.map(label => (
            <span key={label} className="issue-label">{label}</span>
          ))}
        </div>
      )}
    </div>
  )
}
