// Kanban board view for Beads issues

import type { BeadsIssue, IssueStatus } from './types'
import IssueCard from './IssueCard'

interface KanbanViewProps {
  issues: BeadsIssue[]
  loading?: boolean
  error?: string | null
}

// Define column order and display names
const COLUMNS: { status: IssueStatus; label: string }[] = [
  { status: 'open', label: 'Open' },
  { status: 'ready', label: 'Ready' },
  { status: 'in_progress', label: 'In Progress' },
  { status: 'blocked', label: 'Blocked' },
  { status: 'closed', label: 'Closed' },
]

export default function KanbanView({ issues, loading, error }: KanbanViewProps) {
  if (loading) {
    return (
      <div className="beads-kanban loading">
        <div className="loading-message">Loading issues...</div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="beads-kanban error">
        <div className="error-message">{error}</div>
      </div>
    )
  }

  if (issues.length === 0) {
    return (
      <div className="beads-kanban empty">
        <div className="empty-message">No issues found</div>
      </div>
    )
  }

  // Group issues by status
  const issuesByStatus = new Map<IssueStatus, BeadsIssue[]>()
  for (const status of COLUMNS.map(c => c.status)) {
    issuesByStatus.set(status, [])
  }

  // Add an "other" bucket for statuses not in our columns
  const otherIssues: BeadsIssue[] = []

  for (const issue of issues) {
    const status = issue.status
    if (issuesByStatus.has(status)) {
      issuesByStatus.get(status)!.push(issue)
    } else {
      otherIssues.push(issue)
    }
  }

  // Sort issues within each column by priority
  for (const [, columnIssues] of issuesByStatus) {
    columnIssues.sort((a, b) => (a.priority || 99) - (b.priority || 99))
  }
  otherIssues.sort((a, b) => (a.priority || 99) - (b.priority || 99))

  return (
    <div className="beads-kanban">
      {COLUMNS.map(column => {
        const columnIssues = issuesByStatus.get(column.status) || []
        return (
          <div key={column.status} className={`kanban-column status-${column.status}`}>
            <div className="kanban-column-header">
              <span className="column-title">{column.label}</span>
              <span className="column-count">{columnIssues.length}</span>
            </div>
            <div className="kanban-column-content">
              {columnIssues.map(issue => (
                <IssueCard key={issue.id} issue={issue} showDependencies />
              ))}
            </div>
          </div>
        )
      })}
      {otherIssues.length > 0 && (
        <div className="kanban-column status-other">
          <div className="kanban-column-header">
            <span className="column-title">Other</span>
            <span className="column-count">{otherIssues.length}</span>
          </div>
          <div className="kanban-column-content">
            {otherIssues.map(issue => (
              <IssueCard key={issue.id} issue={issue} showDependencies />
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
