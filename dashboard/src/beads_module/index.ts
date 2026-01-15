// Beads Module - Main entry point
// ============================================================================
// This is a self-contained module for beads_viewer integration.
// It can be safely removed without affecting the rest of the application.
//
// To remove this module:
// 1. Delete the entire beads_module folder
// 2. Remove the import and usage from App.tsx (see REMOVAL_GUIDE.md)
// 3. Remove the beads routes from api/beads-routes.js
// 4. Remove the CSS import from theme.css (if added)
//
// See ../../../beads_viewer_integration/REMOVAL_GUIDE.md for detailed instructions.
// ============================================================================

// Components
export { BeadsTab } from './components';
export {
  BeadsGraphView,
  BeadsKanbanView,
  BeadsTriageView,
  BeadsInsightsView,
} from './components';

// Context & State
export { BeadsProvider, useBeads, useBeadsOptional } from './context';

// API
export * as beadsApi from './api';
export { BeadsApiException, isBeadsApiError, formatError } from './api';

// Types
export type {
  // Core types
  BeadsIssue,
  BeadsIssueStatus,
  BeadsIssueType,
  BeadsDependencyType,

  // Robot protocol types
  BeadsTriage,
  BeadsTriageRecommendation,
  BeadsInsights,
  BeadsGraphMetrics,
  BeadsHealthIndicator,
  BeadsPlan,
  BeadsPlanTrack,

  // Graph types
  BeadsGraphData,
  BeadsGraphNode,
  BeadsGraphLink,

  // Kanban types
  BeadsKanbanColumn,
  BeadsKanbanConfig,

  // UI types
  BeadsSubTab,
  BeadsFilters,
  BeadsViewState,

  // Context types
  BeadsContextType,
  BeadsContextState,
  BeadsContextActions,

  // API types
  BeadsApiResponse,
  BeadsApiError,
  BeadsIssuesResponse,

  // Component props
  BeadsGraphViewProps,
  BeadsKanbanViewProps,
  BeadsTriageViewProps,
  BeadsInsightsViewProps,
  BeadsIssueDetailProps,
} from './types';

// Constants
export {
  BEADS_STATUS_COLORS,
  BEADS_STATUS_LABELS,
  BEADS_TYPE_ICONS,
  BEADS_PRIORITY_COLORS,
  DEFAULT_BEADS_FILTERS,
  DEFAULT_BEADS_VIEW_STATE,
} from './types';
