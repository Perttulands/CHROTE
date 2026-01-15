// Beads Insights View - Graph metrics and project health analytics
// This component is part of the self-contained beads_module

import { useEffect, useCallback, useMemo, useState } from 'react';
import { useBeads } from '../context';
import type { BeadsInsightsViewProps, BeadsGraphMetrics } from '../types';

// ============================================================================
// METRIC CARD COMPONENT
// ============================================================================

interface MetricCardProps {
  title: string;
  value: string | number;
  description: string;
  icon: string;
  color?: string;
  trend?: 'up' | 'down' | 'neutral';
}

function MetricCard({ title, value, description, icon, color, trend }: MetricCardProps) {
  const trendIcon = trend === 'up' ? '↑' : trend === 'down' ? '↓' : '';
  const trendColor = trend === 'up' ? '#4CAF50' : trend === 'down' ? '#f44336' : undefined;

  return (
    <div className="beads-metric-card">
      <div className="beads-metric-header">
        <span className="beads-metric-icon">{icon}</span>
        <span className="beads-metric-title">{title}</span>
      </div>
      <div className="beads-metric-value" style={{ color }}>
        {value}
        {trendIcon && (
          <span className="beads-metric-trend" style={{ color: trendColor }}>
            {trendIcon}
          </span>
        )}
      </div>
      <div className="beads-metric-description">{description}</div>
    </div>
  );
}

// ============================================================================
// TOP N LIST COMPONENT
// ============================================================================

interface TopNListProps {
  title: string;
  icon: string;
  items: Array<{ id: string; value: number }>;
  formatValue?: (value: number) => string;
  maxItems?: number;
}

function TopNList({
  title,
  icon,
  items,
  formatValue = (v) => v.toFixed(4),
  maxItems = 5,
}: TopNListProps) {
  const topItems = items.slice(0, maxItems);
  const maxValue = topItems.length > 0 ? topItems[0].value : 1;

  return (
    <div className="beads-topn-list">
      <div className="beads-topn-header">
        <span className="beads-topn-icon">{icon}</span>
        <span className="beads-topn-title">{title}</span>
      </div>
      <div className="beads-topn-items">
        {topItems.map((item, idx) => (
          <div key={item.id} className="beads-topn-item">
            <span className="beads-topn-rank">#{idx + 1}</span>
            <span className="beads-topn-id">{item.id}</span>
            <div className="beads-topn-bar-container">
              <div
                className="beads-topn-bar"
                style={{ width: `${(item.value / maxValue) * 100}%` }}
              />
            </div>
            <span className="beads-topn-value">{formatValue(item.value)}</span>
          </div>
        ))}
        {topItems.length === 0 && (
          <div className="beads-topn-empty">No data</div>
        )}
      </div>
    </div>
  );
}

// ============================================================================
// HEALTH INDICATOR COMPONENT
// ============================================================================

interface HealthIndicatorProps {
  score: number;
  risks: string[];
  warnings: string[];
}

function HealthIndicator({ score, risks, warnings }: HealthIndicatorProps) {
  const getScoreColor = (s: number) => {
    if (s >= 80) return '#4CAF50';
    if (s >= 60) return '#8BC34A';
    if (s >= 40) return '#FF9800';
    if (s >= 20) return '#FF5722';
    return '#f44336';
  };

  const getScoreLabel = (s: number) => {
    if (s >= 80) return 'Excellent';
    if (s >= 60) return 'Good';
    if (s >= 40) return 'Fair';
    if (s >= 20) return 'Poor';
    return 'Critical';
  };

  return (
    <div className="beads-health-indicator">
      <div className="beads-health-score-container">
        <div
          className="beads-health-score-ring"
          style={{
            background: `conic-gradient(${getScoreColor(score)} ${score}%, var(--surface-secondary) 0)`,
          }}
        >
          <div className="beads-health-score-inner">
            <span className="beads-health-score-value">{Math.round(score)}</span>
            <span className="beads-health-score-label">{getScoreLabel(score)}</span>
          </div>
        </div>
      </div>

      {risks.length > 0 && (
        <div className="beads-health-risks">
          <h4 className="beads-health-section-title">
            <span>🚨</span> Risks
          </h4>
          <ul className="beads-health-list">
            {risks.map((risk, idx) => (
              <li key={idx} className="beads-health-risk">{risk}</li>
            ))}
          </ul>
        </div>
      )}

      {warnings.length > 0 && (
        <div className="beads-health-warnings">
          <h4 className="beads-health-section-title">
            <span>⚠️</span> Warnings
          </h4>
          <ul className="beads-health-list">
            {warnings.map((warning, idx) => (
              <li key={idx} className="beads-health-warning">{warning}</li>
            ))}
          </ul>
        </div>
      )}

      {risks.length === 0 && warnings.length === 0 && (
        <div className="beads-health-all-clear">
          <span>✅</span> No issues detected
        </div>
      )}
    </div>
  );
}

// ============================================================================
// CYCLES DISPLAY COMPONENT
// ============================================================================

interface CyclesDisplayProps {
  cycles: string[][];
}

function CyclesDisplay({ cycles }: CyclesDisplayProps) {
  if (cycles.length === 0) {
    return (
      <div className="beads-cycles-empty">
        <span className="beads-cycles-icon">✅</span>
        <span>No circular dependencies detected</span>
      </div>
    );
  }

  return (
    <div className="beads-cycles">
      <div className="beads-cycles-header">
        <span className="beads-cycles-icon">⚠️</span>
        <span className="beads-cycles-title">
          {cycles.length} Circular Dependenc{cycles.length === 1 ? 'y' : 'ies'}
        </span>
      </div>
      <div className="beads-cycles-list">
        {cycles.map((cycle, idx) => (
          <div key={idx} className="beads-cycle">
            <span className="beads-cycle-label">Cycle {idx + 1}:</span>
            <span className="beads-cycle-path">
              {cycle.join(' → ')} → {cycle[0]}
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}

// ============================================================================
// CRITICAL PATH DISPLAY COMPONENT
// ============================================================================

interface CriticalPathDisplayProps {
  path: string[];
}

function CriticalPathDisplay({ path }: CriticalPathDisplayProps) {
  if (path.length === 0) {
    return (
      <div className="beads-critical-path-empty">
        No critical path identified
      </div>
    );
  }

  return (
    <div className="beads-critical-path">
      <div className="beads-critical-path-header">
        <span className="beads-critical-path-length">
          {path.length} issues in longest dependency chain
        </span>
      </div>
      <div className="beads-critical-path-items">
        {path.map((id, idx) => (
          <div key={id} className="beads-critical-path-item">
            <span className="beads-critical-path-step">{idx + 1}</span>
            <span className="beads-critical-path-id">{id}</span>
            {idx < path.length - 1 && (
              <span className="beads-critical-path-arrow">→</span>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

function sortMetricEntries(
  metrics: Record<string, number>
): Array<{ id: string; value: number }> {
  return Object.entries(metrics)
    .map(([id, value]) => ({ id, value }))
    .sort((a, b) => b.value - a.value);
}

// ============================================================================
// MAIN COMPONENT
// ============================================================================

export function BeadsInsightsView({ projectPath }: BeadsInsightsViewProps) {
  const {
    issues,
    insights,
    isLoading,
    error,
    fetchIssues,
    fetchInsights,
  } = useBeads();

  const [hasLoaded, setHasLoaded] = useState(false);

  // Fetch data on mount
  useEffect(() => {
    const loadData = async () => {
      await fetchIssues(projectPath);
      await fetchInsights(projectPath);
      setHasLoaded(true);
    };
    loadData();
  }, [projectPath, fetchIssues, fetchInsights]);

  const handleRefresh = useCallback(async () => {
    await fetchIssues(projectPath);
    await fetchInsights(projectPath);
  }, [fetchIssues, fetchInsights, projectPath]);

  // Sorted metric lists
  const sortedPageRank = useMemo(
    () => (insights?.metrics.pageRank ? sortMetricEntries(insights.metrics.pageRank) : []),
    [insights]
  );

  const sortedBetweenness = useMemo(
    () => (insights?.metrics.betweenness ? sortMetricEntries(insights.metrics.betweenness) : []),
    [insights]
  );

  const sortedDegree = useMemo(
    () => (insights?.metrics.degree ? sortMetricEntries(insights.metrics.degree) : []),
    [insights]
  );

  // Loading state
  if (isLoading && !hasLoaded) {
    return (
      <div className="beads-view beads-insights-view">
        <div className="beads-loading">
          <div className="beads-spinner" />
          <span>Computing graph metrics...</span>
        </div>
      </div>
    );
  }

  // Error state
  if (error && !insights) {
    return (
      <div className="beads-view beads-insights-view">
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
  if (!insights) {
    return (
      <div className="beads-view beads-insights-view">
        <div className="beads-empty">
          <span className="beads-empty-icon">📈</span>
          <span className="beads-empty-message">No insights available</span>
          <span className="beads-empty-hint">
            Run analysis to compute graph metrics
          </span>
          <button className="beads-btn beads-btn-primary" onClick={handleRefresh}>
            Compute Insights
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="beads-view beads-insights-view">
      {/* Toolbar */}
      <div className="beads-toolbar">
        <div className="beads-toolbar-left">
          <span className="beads-toolbar-label">Graph Insights</span>
          {insights.timestamp && (
            <span className="beads-toolbar-timestamp">
              Computed: {new Date(insights.timestamp).toLocaleString()}
            </span>
          )}
        </div>
        <div className="beads-toolbar-right">
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

      {/* Insights content */}
      <div className="beads-insights-content">
        {/* Overview metrics */}
        <div className="beads-insights-section">
          <h3 className="beads-insights-section-title">Overview</h3>
          <div className="beads-metrics-grid">
            <MetricCard
              title="Total Issues"
              value={insights.issueCount}
              description="Issues in the project"
              icon="📋"
            />
            <MetricCard
              title="Open Issues"
              value={insights.openCount}
              description="Issues not yet closed"
              icon="📬"
              color="#4CAF50"
            />
            <MetricCard
              title="Blocked Issues"
              value={insights.blockedCount}
              description="Issues waiting on dependencies"
              icon="🚧"
              color="#f44336"
            />
            <MetricCard
              title="Graph Density"
              value={`${(insights.metrics.density * 100).toFixed(1)}%`}
              description="How interconnected issues are"
              icon="🕸️"
            />
          </div>
        </div>

        {/* Health indicator */}
        <div className="beads-insights-section">
          <h3 className="beads-insights-section-title">Project Health</h3>
          <HealthIndicator
            score={insights.health.score}
            risks={insights.health.risks}
            warnings={insights.health.warnings}
          />
        </div>

        {/* Graph metrics */}
        <div className="beads-insights-section">
          <h3 className="beads-insights-section-title">Graph Metrics</h3>
          <div className="beads-topn-grid">
            <TopNList
              title="PageRank (Influence)"
              icon="🏆"
              items={sortedPageRank}
            />
            <TopNList
              title="Betweenness (Bottlenecks)"
              icon="🔀"
              items={sortedBetweenness}
            />
            <TopNList
              title="Degree (Connectivity)"
              icon="🔗"
              items={sortedDegree}
              formatValue={(v) => v.toString()}
            />
          </div>
        </div>

        {/* Critical path */}
        <div className="beads-insights-section">
          <h3 className="beads-insights-section-title">Critical Path</h3>
          <CriticalPathDisplay path={insights.metrics.criticalPath} />
        </div>

        {/* Cycles */}
        <div className="beads-insights-section">
          <h3 className="beads-insights-section-title">Circular Dependencies</h3>
          <CyclesDisplay cycles={insights.metrics.cycles} />
        </div>
      </div>
    </div>
  );
}

export default BeadsInsightsView;
