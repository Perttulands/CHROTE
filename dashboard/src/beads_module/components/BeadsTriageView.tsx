// Beads Triage View - AI-powered issue triage and recommendations
// This component is part of the self-contained beads_module

import { useEffect, useCallback, useMemo, useState } from 'react';
import { useBeads } from '../context';
import type {
  BeadsTriageRecommendation,
  BeadsTriageViewProps,
  BeadsIssue,
} from '../types';
import {
  BEADS_STATUS_COLORS,
  BEADS_TYPE_ICONS,
  BEADS_PRIORITY_COLORS,
} from '../types';

// ============================================================================
// RECOMMENDATION CARD COMPONENT
// ============================================================================

interface RecommendationCardProps {
  recommendation: BeadsTriageRecommendation;
  issue: BeadsIssue | undefined;
  onClick: () => void;
  isSelected: boolean;
}

function RecommendationCard({
  recommendation,
  issue,
  onClick,
  isSelected,
}: RecommendationCardProps) {
  const impactColors = {
    high: '#f44336',
    medium: '#FF9800',
    low: '#4CAF50',
  };

  return (
    <div
      className={`beads-triage-card ${isSelected ? 'selected' : ''}`}
      onClick={onClick}
    >
      <div className="beads-triage-card-rank">#{recommendation.rank}</div>
      <div className="beads-triage-card-content">
        <div className="beads-triage-card-header">
          {issue && (
            <span className="beads-triage-card-type">
              {BEADS_TYPE_ICONS[issue.type]}
            </span>
          )}
          <span className="beads-triage-card-id">{recommendation.issueId}</span>
          <span
            className="beads-triage-card-impact"
            style={{ color: impactColors[recommendation.estimatedImpact] }}
          >
            {recommendation.estimatedImpact.toUpperCase()}
          </span>
        </div>
        {issue && (
          <div className="beads-triage-card-title">{issue.title}</div>
        )}
        <div className="beads-triage-card-reasoning">
          {recommendation.reasoning}
        </div>
        {recommendation.unblockChain.length > 0 && (
          <div className="beads-triage-card-chain">
            <span className="beads-triage-card-chain-label">Unblock chain:</span>
            <div className="beads-triage-card-chain-items">
              {recommendation.unblockChain.map((id, idx) => (
                <span key={id} className="beads-triage-card-chain-item">
                  {id}
                  {idx < recommendation.unblockChain.length - 1 && ' → '}
                </span>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

// ============================================================================
// QUICK WINS SECTION
// ============================================================================

interface QuickWinsSectionProps {
  quickWins: string[];
  issues: BeadsIssue[];
  onIssueClick: (issueId: string) => void;
}

function QuickWinsSection({ quickWins, issues, onIssueClick }: QuickWinsSectionProps) {
  if (quickWins.length === 0) return null;

  const quickWinIssues = quickWins
    .map((id) => issues.find((i) => i.id === id))
    .filter((i): i is BeadsIssue => !!i);

  return (
    <div className="beads-triage-section">
      <h3 className="beads-triage-section-title">
        <span className="beads-triage-section-icon">⚡</span>
        Quick Wins
        <span className="beads-triage-section-count">{quickWins.length}</span>
      </h3>
      <p className="beads-triage-section-desc">
        Low-effort issues that can be completed quickly
      </p>
      <div className="beads-triage-quick-wins">
        {quickWinIssues.map((issue) => (
          <div
            key={issue.id}
            className="beads-triage-quick-win"
            onClick={() => onIssueClick(issue.id)}
          >
            <span className="beads-triage-quick-win-type">
              {BEADS_TYPE_ICONS[issue.type]}
            </span>
            <span className="beads-triage-quick-win-id">{issue.id}</span>
            <span className="beads-triage-quick-win-title">{issue.title}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

// ============================================================================
// BLOCKERS SECTION
// ============================================================================

interface BlockersSectionProps {
  blockers: string[];
  issues: BeadsIssue[];
  onIssueClick: (issueId: string) => void;
}

function BlockersSection({ blockers, issues, onIssueClick }: BlockersSectionProps) {
  if (blockers.length === 0) return null;

  const blockerIssues = blockers
    .map((id) => issues.find((i) => i.id === id))
    .filter((i): i is BeadsIssue => !!i);

  return (
    <div className="beads-triage-section beads-triage-blockers">
      <h3 className="beads-triage-section-title">
        <span className="beads-triage-section-icon">🚧</span>
        Current Blockers
        <span className="beads-triage-section-count">{blockers.length}</span>
      </h3>
      <p className="beads-triage-section-desc">
        Issues blocking progress on other work
      </p>
      <div className="beads-triage-blocker-list">
        {blockerIssues.map((issue) => (
          <div
            key={issue.id}
            className="beads-triage-blocker"
            onClick={() => onIssueClick(issue.id)}
          >
            <div className="beads-triage-blocker-header">
              <span className="beads-triage-blocker-type">
                {BEADS_TYPE_ICONS[issue.type]}
              </span>
              <span className="beads-triage-blocker-id">{issue.id}</span>
              <span
                className="beads-triage-blocker-priority"
                style={{ color: BEADS_PRIORITY_COLORS[issue.priority] }}
              >
                P{issue.priority}
              </span>
            </div>
            <div className="beads-triage-blocker-title">{issue.title}</div>
            {issue.blocking.length > 0 && (
              <div className="beads-triage-blocker-impact">
                Blocking {issue.blocking.length} issue(s)
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}

// ============================================================================
// MAIN COMPONENT
// ============================================================================

export function BeadsTriageView({ projectPath, onIssueSelect }: BeadsTriageViewProps) {
  const {
    issues,
    triage,
    isLoading,
    error,
    fetchIssues,
    fetchTriage,
    selectIssue,
    viewState,
  } = useBeads();

  const [hasLoadedTriage, setHasLoadedTriage] = useState(false);

  // Fetch data on mount
  useEffect(() => {
    const loadData = async () => {
      await fetchIssues(projectPath);
      await fetchTriage(projectPath);
      setHasLoadedTriage(true);
    };
    loadData();
  }, [projectPath, fetchIssues, fetchTriage]);

  const handleIssueClick = useCallback(
    (issueId: string) => {
      selectIssue(issueId);
      onIssueSelect?.(issueId);
    },
    [selectIssue, onIssueSelect]
  );

  const handleRefresh = useCallback(async () => {
    await fetchIssues(projectPath);
    await fetchTriage(projectPath);
  }, [fetchIssues, fetchTriage, projectPath]);

  // Get issue by ID
  const getIssue = useCallback(
    (id: string) => issues.find((i) => i.id === id),
    [issues]
  );

  // Loading state
  if (isLoading && !hasLoadedTriage) {
    return (
      <div className="beads-view beads-triage-view">
        <div className="beads-loading">
          <div className="beads-spinner" />
          <span>Running AI triage analysis...</span>
          <span className="beads-loading-hint">This may take a moment</span>
        </div>
      </div>
    );
  }

  // Error state
  if (error && !triage) {
    return (
      <div className="beads-view beads-triage-view">
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
  if (!triage) {
    return (
      <div className="beads-view beads-triage-view">
        <div className="beads-empty">
          <span className="beads-empty-icon">🎯</span>
          <span className="beads-empty-message">No triage data available</span>
          <span className="beads-empty-hint">
            Run the triage analysis to get AI-powered recommendations
          </span>
          <button className="beads-btn beads-btn-primary" onClick={handleRefresh}>
            Run Triage
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="beads-view beads-triage-view">
      {/* Toolbar */}
      <div className="beads-toolbar">
        <div className="beads-toolbar-left">
          <span className="beads-toolbar-label">
            AI Triage Results
          </span>
          {triage.timestamp && (
            <span className="beads-toolbar-timestamp">
              Updated: {new Date(triage.timestamp).toLocaleString()}
            </span>
          )}
        </div>
        <div className="beads-toolbar-right">
          <button
            className="beads-btn beads-btn-primary"
            onClick={handleRefresh}
            disabled={isLoading}
          >
            {isLoading ? '⏳ Analyzing...' : '🔄 Re-run Triage'}
          </button>
        </div>
      </div>

      {/* Triage content */}
      <div className="beads-triage-content">
        {/* Sidebar with quick wins and blockers */}
        <div className="beads-triage-sidebar">
          <QuickWinsSection
            quickWins={triage.quickWins}
            issues={issues}
            onIssueClick={handleIssueClick}
          />
          <BlockersSection
            blockers={triage.blockers}
            issues={issues}
            onIssueClick={handleIssueClick}
          />
        </div>

        {/* Main recommendations */}
        <div className="beads-triage-main">
          <div className="beads-triage-section">
            <h3 className="beads-triage-section-title">
              <span className="beads-triage-section-icon">📊</span>
              Prioritized Recommendations
              <span className="beads-triage-section-count">
                {triage.recommendations.length}
              </span>
            </h3>
            <p className="beads-triage-section-desc">
              AI-ranked issues based on impact, dependencies, and project health
            </p>
          </div>

          <div className="beads-triage-recommendations">
            {triage.recommendations.length === 0 ? (
              <div className="beads-triage-no-recommendations">
                No recommendations at this time
              </div>
            ) : (
              triage.recommendations.map((rec) => (
                <RecommendationCard
                  key={rec.issueId}
                  recommendation={rec}
                  issue={getIssue(rec.issueId)}
                  onClick={() => handleIssueClick(rec.issueId)}
                  isSelected={viewState.selectedIssueId === rec.issueId}
                />
              ))
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

export default BeadsTriageView;
