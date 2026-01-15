// Beads Kanban View - Kanban board for issue management
// This component is part of the self-contained beads_module

import { useEffect, useCallback, useMemo, useState } from 'react';
import { useBeads } from '../context';
import type {
  BeadsIssue,
  BeadsIssueStatus,
  BeadsKanbanViewProps,
} from '../types';
import {
  BEADS_STATUS_COLORS,
  BEADS_STATUS_LABELS,
  BEADS_TYPE_ICONS,
  BEADS_PRIORITY_COLORS,
} from '../types';

// ============================================================================
// KANBAN COLUMN COMPONENT
// ============================================================================

interface KanbanColumnProps {
  status: BeadsIssueStatus;
  issues: BeadsIssue[];
  onIssueClick: (issueId: string) => void;
  selectedIssueId: string | null;
}

function KanbanColumn({
  status,
  issues,
  onIssueClick,
  selectedIssueId,
}: KanbanColumnProps) {
  const color = BEADS_STATUS_COLORS[status];
  const label = BEADS_STATUS_LABELS[status];

  return (
    <div className="beads-kanban-column">
      <div className="beads-kanban-column-header" style={{ borderColor: color }}>
        <span className="beads-kanban-column-title">{label}</span>
        <span className="beads-kanban-column-count">{issues.length}</span>
      </div>
      <div className="beads-kanban-column-body">
        {issues.map((issue) => (
          <KanbanCard
            key={issue.id}
            issue={issue}
            onClick={() => onIssueClick(issue.id)}
            isSelected={issue.id === selectedIssueId}
          />
        ))}
        {issues.length === 0 && (
          <div className="beads-kanban-empty">No issues</div>
        )}
      </div>
    </div>
  );
}

// ============================================================================
// KANBAN CARD COMPONENT
// ============================================================================

interface KanbanCardProps {
  issue: BeadsIssue;
  onClick: () => void;
  isSelected: boolean;
}

function KanbanCard({ issue, onClick, isSelected }: KanbanCardProps) {
  const priorityColor = BEADS_PRIORITY_COLORS[issue.priority] || BEADS_PRIORITY_COLORS[5];
  const typeIcon = BEADS_TYPE_ICONS[issue.type];

  return (
    <div
      className={`beads-kanban-card ${isSelected ? 'selected' : ''}`}
      onClick={onClick}
      style={{ borderLeftColor: priorityColor }}
    >
      <div className="beads-kanban-card-header">
        <span className="beads-kanban-card-type">{typeIcon}</span>
        <span className="beads-kanban-card-id">{issue.id}</span>
        <span
          className="beads-kanban-card-priority"
          style={{ color: priorityColor }}
        >
          P{issue.priority}
        </span>
      </div>
      <div className="beads-kanban-card-title">{issue.title}</div>
      {issue.labels.length > 0 && (
        <div className="beads-kanban-card-labels">
          {issue.labels.slice(0, 2).map((label) => (
            <span key={label} className="beads-kanban-card-label">
              {label}
            </span>
          ))}
          {issue.labels.length > 2 && (
            <span className="beads-kanban-card-label-more">
              +{issue.labels.length - 2}
            </span>
          )}
        </div>
      )}
      {(issue.dependencies.length > 0 || issue.blocking.length > 0) && (
        <div className="beads-kanban-card-deps">
          {issue.dependencies.length > 0 && (
            <span className="beads-kanban-card-dep" title="Dependencies">
              ⬅️ {issue.dependencies.length}
            </span>
          )}
          {issue.blocking.length > 0 && (
            <span className="beads-kanban-card-dep" title="Blocking">
              ➡️ {issue.blocking.length}
            </span>
          )}
        </div>
      )}
      {issue.assignees.length > 0 && (
        <div className="beads-kanban-card-assignees">
          {issue.assignees.slice(0, 2).map((assignee) => (
            <span key={assignee} className="beads-kanban-card-assignee">
              {assignee.charAt(0).toUpperCase()}
            </span>
          ))}
        </div>
      )}
    </div>
  );
}

// ============================================================================
// ISSUE DETAIL PANEL
// ============================================================================

interface IssueDetailPanelProps {
  issue: BeadsIssue;
  onClose: () => void;
}

function IssueDetailPanel({ issue, onClose }: IssueDetailPanelProps) {
  const statusColor = BEADS_STATUS_COLORS[issue.status];
  const priorityColor = BEADS_PRIORITY_COLORS[issue.priority] || BEADS_PRIORITY_COLORS[5];

  return (
    <div className="beads-issue-detail">
      <div className="beads-issue-detail-header">
        <div className="beads-issue-detail-title-row">
          <span className="beads-issue-detail-type">{BEADS_TYPE_ICONS[issue.type]}</span>
          <span className="beads-issue-detail-id">{issue.id}</span>
          <button className="beads-btn beads-btn-icon" onClick={onClose}>
            ✕
          </button>
        </div>
        <h3 className="beads-issue-detail-title">{issue.title}</h3>
      </div>
      <div className="beads-issue-detail-body">
        <div className="beads-issue-detail-meta">
          <div className="beads-issue-detail-meta-item">
            <span className="beads-issue-detail-meta-label">Status</span>
            <span
              className="beads-issue-detail-meta-value beads-issue-status"
              style={{ color: statusColor }}
            >
              {BEADS_STATUS_LABELS[issue.status]}
            </span>
          </div>
          <div className="beads-issue-detail-meta-item">
            <span className="beads-issue-detail-meta-label">Priority</span>
            <span
              className="beads-issue-detail-meta-value"
              style={{ color: priorityColor }}
            >
              P{issue.priority}
            </span>
          </div>
          <div className="beads-issue-detail-meta-item">
            <span className="beads-issue-detail-meta-label">Type</span>
            <span className="beads-issue-detail-meta-value">{issue.type}</span>
          </div>
        </div>

        {issue.description && (
          <div className="beads-issue-detail-section">
            <h4>Description</h4>
            <p className="beads-issue-detail-description">{issue.description}</p>
          </div>
        )}

        {issue.labels.length > 0 && (
          <div className="beads-issue-detail-section">
            <h4>Labels</h4>
            <div className="beads-issue-detail-labels">
              {issue.labels.map((label) => (
                <span key={label} className="beads-label">
                  {label}
                </span>
              ))}
            </div>
          </div>
        )}

        {issue.assignees.length > 0 && (
          <div className="beads-issue-detail-section">
            <h4>Assignees</h4>
            <div className="beads-issue-detail-assignees">
              {issue.assignees.map((assignee) => (
                <span key={assignee} className="beads-assignee">
                  {assignee}
                </span>
              ))}
            </div>
          </div>
        )}

        {issue.dependencies.length > 0 && (
          <div className="beads-issue-detail-section">
            <h4>Dependencies ({issue.dependencies.length})</h4>
            <div className="beads-issue-detail-links">
              {issue.dependencies.map((depId) => (
                <span key={depId} className="beads-issue-link">
                  {depId}
                </span>
              ))}
            </div>
          </div>
        )}

        {issue.blocking.length > 0 && (
          <div className="beads-issue-detail-section">
            <h4>Blocking ({issue.blocking.length})</h4>
            <div className="beads-issue-detail-links">
              {issue.blocking.map((blockId) => (
                <span key={blockId} className="beads-issue-link">
                  {blockId}
                </span>
              ))}
            </div>
          </div>
        )}

        <div className="beads-issue-detail-timestamps">
          <div className="beads-issue-detail-timestamp">
            <span className="beads-issue-detail-timestamp-label">Created</span>
            <span className="beads-issue-detail-timestamp-value">
              {new Date(issue.created).toLocaleString()}
            </span>
          </div>
          <div className="beads-issue-detail-timestamp">
            <span className="beads-issue-detail-timestamp-label">Updated</span>
            <span className="beads-issue-detail-timestamp-value">
              {new Date(issue.updated).toLocaleString()}
            </span>
          </div>
          {issue.closed && (
            <div className="beads-issue-detail-timestamp">
              <span className="beads-issue-detail-timestamp-label">Closed</span>
              <span className="beads-issue-detail-timestamp-value">
                {new Date(issue.closed).toLocaleString()}
              </span>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

// ============================================================================
// MAIN COMPONENT
// ============================================================================

const KANBAN_COLUMNS: BeadsIssueStatus[] = [
  'open',
  'in_progress',
  'blocked',
  'closed',
];

export function BeadsKanbanView({ projectPath, onIssueSelect }: BeadsKanbanViewProps) {
  const {
    issues,
    isLoading,
    error,
    fetchIssues,
    selectIssue,
    viewState,
  } = useBeads();

  const [sortBy, setSortBy] = useState<'priority' | 'created' | 'updated'>('priority');
  const [showClosed, setShowClosed] = useState(false);

  // Fetch data on mount
  useEffect(() => {
    fetchIssues(projectPath);
  }, [projectPath, fetchIssues]);

  // Group issues by status
  const columnData = useMemo(() => {
    const columns: Record<BeadsIssueStatus, BeadsIssue[]> = {
      open: [],
      in_progress: [],
      blocked: [],
      deferred: [],
      pinned: [],
      hooked: [],
      closed: [],
      tombstone: [],
    };

    // Filter and sort issues
    const filteredIssues = issues.filter((issue) => {
      if (!showClosed && (issue.status === 'closed' || issue.status === 'tombstone')) {
        return false;
      }
      return true;
    });

    // Sort
    const sortedIssues = [...filteredIssues].sort((a, b) => {
      switch (sortBy) {
        case 'priority':
          return a.priority - b.priority;
        case 'created':
          return new Date(b.created).getTime() - new Date(a.created).getTime();
        case 'updated':
          return new Date(b.updated).getTime() - new Date(a.updated).getTime();
        default:
          return 0;
      }
    });

    // Group by status
    sortedIssues.forEach((issue) => {
      columns[issue.status].push(issue);
    });

    return columns;
  }, [issues, sortBy, showClosed]);

  const handleIssueClick = useCallback(
    (issueId: string) => {
      selectIssue(issueId);
      onIssueSelect?.(issueId);
    },
    [selectIssue, onIssueSelect]
  );

  const handleCloseDetail = useCallback(() => {
    selectIssue(null);
  }, [selectIssue]);

  const handleRefresh = useCallback(() => {
    fetchIssues(projectPath);
  }, [fetchIssues, projectPath]);

  // Get selected issue
  const selectedIssue = useMemo(() => {
    if (!viewState.selectedIssueId) return null;
    return issues.find((i) => i.id === viewState.selectedIssueId) || null;
  }, [issues, viewState.selectedIssueId]);

  // Columns to display
  const displayColumns = showClosed ? KANBAN_COLUMNS : KANBAN_COLUMNS.filter((c) => c !== 'closed');

  // Loading state
  if (isLoading && issues.length === 0) {
    return (
      <div className="beads-view beads-kanban-view">
        <div className="beads-loading">
          <div className="beads-spinner" />
          <span>Loading issues...</span>
        </div>
      </div>
    );
  }

  // Error state
  if (error && issues.length === 0) {
    return (
      <div className="beads-view beads-kanban-view">
        <div className="beads-error">
          <span className="beads-error-icon">⚠️</span>
          <span className="beads-error-message">{error.message}</span>
          {error.details && (
            <span className="beads-error-details">{error.details}</span>
          )}
          <button className="beads-btn beads-btn-primary" onClick={handleRefresh}>
            Retry
          </button>
        </div>
      </div>
    );
  }

  // Empty state
  if (issues.length === 0) {
    return (
      <div className="beads-view beads-kanban-view">
        <div className="beads-empty">
          <span className="beads-empty-icon">📋</span>
          <span className="beads-empty-message">No issues found</span>
          <span className="beads-empty-hint">
            Make sure the project has a .beads/issues.jsonl file
          </span>
        </div>
      </div>
    );
  }

  return (
    <div className="beads-view beads-kanban-view">
      {/* Toolbar */}
      <div className="beads-toolbar">
        <div className="beads-toolbar-left">
          <span className="beads-toolbar-label">{issues.length} issues</span>
        </div>
        <div className="beads-toolbar-right">
          <label className="beads-toolbar-control">
            <span>Sort:</span>
            <select
              value={sortBy}
              onChange={(e) => setSortBy(e.target.value as typeof sortBy)}
              className="beads-select"
            >
              <option value="priority">Priority</option>
              <option value="created">Created</option>
              <option value="updated">Updated</option>
            </select>
          </label>
          <label className="beads-toolbar-checkbox">
            <input
              type="checkbox"
              checked={showClosed}
              onChange={(e) => setShowClosed(e.target.checked)}
            />
            <span>Show closed</span>
          </label>
          <button
            className="beads-btn beads-btn-icon"
            onClick={handleRefresh}
            disabled={isLoading}
            title="Refresh"
          >
            {isLoading ? '⏳' : '🔄'}
          </button>
        </div>
      </div>

      {/* Kanban board */}
      <div className="beads-kanban-board">
        <div className="beads-kanban-columns">
          {displayColumns.map((status) => (
            <KanbanColumn
              key={status}
              status={status}
              issues={columnData[status]}
              onIssueClick={handleIssueClick}
              selectedIssueId={viewState.selectedIssueId}
            />
          ))}
        </div>

        {/* Issue detail panel */}
        {selectedIssue && (
          <IssueDetailPanel issue={selectedIssue} onClose={handleCloseDetail} />
        )}
      </div>
    </div>
  );
}

export default BeadsKanbanView;
