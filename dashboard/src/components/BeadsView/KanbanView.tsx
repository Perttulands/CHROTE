// Kanban board view for Beads issues

import { useState } from 'react'
import type { BeadsIssue, IssueStatus } from './types'
import IssueCard from './IssueCard'
import IssueDetailModal from './IssueDetailModal'

interface KanbanViewProps {
  issues: BeadsIssue[]
  loading?: boolean
  error?: string | null
  projectPath?: string | null
  enableDetailModal?: boolean
  onIssueUpdated?: () => void
}

const COLUMNS: { status: IssueStatus; label: string }[] = [
  { status: 'open', label: 'Open' },
  { status: 'ready', label: 'Ready' },
  { status: 'in_progress', label: 'In Progress' },
  { status: 'hooked', label: 'Hooked' },
  { status: 'blocked', label: 'Blocked' },
  { status: 'closed', label: 'Closed' },
]

export default function KanbanView({
  issues,
  loading,
  error,
  projectPath,
  enableDetailModal = false,
  onIssueUpdated,
}: KanbanViewProps) {
  const [selectedIssue, setSelectedIssue] = useState<BeadsIssue | null>(null)
  const openIssue = enableDetailModal && projectPath
    ? (issue: BeadsIssue) => setSelectedIssue(issue)
    : undefined

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

  const issuesByStatus = new Map<IssueStatus, BeadsIssue[]>()
  for (const status of COLUMNS.map(c => c.status)) {
    issuesByStatus.set(status, [])
  }

  const otherIssues: BeadsIssue[] = []
  for (const issue of issues) {
    const status = issue.status
    if (issuesByStatus.has(status)) {
      issuesByStatus.get(status)!.push(issue)
    } else {
      otherIssues.push(issue)
    }
  }

  for (const [, columnIssues] of issuesByStatus) {
    columnIssues.sort((a, b) => (a.priority || 99) - (b.priority || 99))
  }
  otherIssues.sort((a, b) => (a.priority || 99) - (b.priority || 99))

  return (
    <>
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
                  <IssueCard key={issue.id} issue={issue} showDependencies onClick={openIssue} />
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
                <IssueCard key={issue.id} issue={issue} showDependencies onClick={openIssue} />
              ))}
            </div>
          </div>
        )}
      </div>
      {selectedIssue && projectPath && (
        <IssueDetailModal
          projectPath={projectPath}
          issue={selectedIssue}
          onClose={() => setSelectedIssue(null)}
          onIssueUpdated={onIssueUpdated}
        />
      )}
    </>
  )
}
