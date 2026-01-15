// Beads Module Types - Self-contained type definitions
// This module is designed to be removable without affecting the rest of the application

// ============================================================================
// BEADS DATA TYPES
// ============================================================================

export type BeadsIssueStatus =
  | 'open'
  | 'in_progress'
  | 'blocked'
  | 'deferred'
  | 'pinned'
  | 'hooked'
  | 'closed'
  | 'tombstone';

export type BeadsIssueType =
  | 'bug'
  | 'feature'
  | 'task'
  | 'epic'
  | 'chore';

export type BeadsDependencyType =
  | 'blocks'
  | 'related'
  | 'parent'
  | 'discovered_from';

export interface BeadsIssue {
  id: string;
  title: string;
  description: string;
  status: BeadsIssueStatus;
  priority: number;
  type: BeadsIssueType;
  dependencies: string[];
  blocking: string[];
  related?: string[];
  labels: string[];
  assignees: string[];
  created: string;
  updated: string;
  closed?: string;
}

// ============================================================================
// ROBOT PROTOCOL TYPES (from bv --robot-* commands)
// ============================================================================

export interface BeadsTriageRecommendation {
  issueId: string;
  rank: number;
  reasoning: string;
  unblockChain: string[];
  estimatedImpact: 'high' | 'medium' | 'low';
}

export interface BeadsTriage {
  recommendations: BeadsTriageRecommendation[];
  quickWins: string[];
  blockers: string[];
  timestamp: string;
}

export interface BeadsGraphMetrics {
  pageRank: Record<string, number>;
  betweenness: Record<string, number>;
  eigenvector: Record<string, number>;
  degree: Record<string, number>;
  criticalPath: string[];
  cycles: string[][];
  density: number;
}

export interface BeadsHealthIndicator {
  score: number;
  risks: string[];
  warnings: string[];
}

export interface BeadsInsights {
  metrics: BeadsGraphMetrics;
  health: BeadsHealthIndicator;
  issueCount: number;
  openCount: number;
  blockedCount: number;
  timestamp: string;
}

export interface BeadsPlanTrack {
  name: string;
  issues: string[];
  dependencies: string[];
  estimatedComplexity: 'low' | 'medium' | 'high';
}

export interface BeadsPlan {
  tracks: BeadsPlanTrack[];
  executionOrder: string[];
  parallelizable: string[][];
  timestamp: string;
}

// ============================================================================
// API RESPONSE TYPES
// ============================================================================

export interface BeadsApiResponse<T> {
  success: boolean;
  data?: T;
  error?: BeadsApiError;
  timestamp: string;
}

export interface BeadsApiError {
  code: string;
  message: string;
  details?: string;
}

export interface BeadsIssuesResponse {
  issues: BeadsIssue[];
  totalCount: number;
  projectPath: string;
}

// ============================================================================
// GRAPH VISUALIZATION TYPES
// ============================================================================

export interface BeadsGraphNode {
  id: string;
  name: string;
  status: BeadsIssueStatus;
  priority: number;
  type: BeadsIssueType;
  labels: string[];
  // Computed metrics (populated after analysis)
  pageRank?: number;
  betweenness?: number;
  inDegree?: number;
  outDegree?: number;
}

export interface BeadsGraphLink {
  source: string;
  target: string;
  type: 'blocks' | 'depends' | 'related';
}

export interface BeadsGraphData {
  nodes: BeadsGraphNode[];
  links: BeadsGraphLink[];
}

// ============================================================================
// KANBAN TYPES
// ============================================================================

export interface BeadsKanbanColumn {
  id: BeadsIssueStatus;
  title: string;
  issues: BeadsIssue[];
  color: string;
}

export interface BeadsKanbanConfig {
  columns: BeadsIssueStatus[];
  groupBy: 'status' | 'type' | 'priority' | 'assignee';
  sortBy: 'priority' | 'created' | 'updated' | 'title';
  sortDirection: 'asc' | 'desc';
}

// ============================================================================
// UI STATE TYPES
// ============================================================================

export type BeadsSubTab = 'graph' | 'kanban' | 'triage' | 'insights';

export interface BeadsFilters {
  status: BeadsIssueStatus[];
  type: BeadsIssueType[];
  labels: string[];
  assignees: string[];
  searchQuery: string;
}

export interface BeadsViewState {
  activeSubTab: BeadsSubTab;
  projectPath: string;
  filters: BeadsFilters;
  selectedIssueId: string | null;
}

// ============================================================================
// CONTEXT TYPES
// ============================================================================

export interface BeadsContextState {
  // Data
  issues: BeadsIssue[];
  triage: BeadsTriage | null;
  insights: BeadsInsights | null;
  plan: BeadsPlan | null;
  graphData: BeadsGraphData | null;

  // UI State
  viewState: BeadsViewState;
  isLoading: boolean;
  error: BeadsApiError | null;

  // Available projects (discovered .beads directories)
  availableProjects: string[];
}

export interface BeadsContextActions {
  // Data fetching
  fetchIssues: (projectPath: string) => Promise<void>;
  fetchTriage: (projectPath: string) => Promise<void>;
  fetchInsights: (projectPath: string) => Promise<void>;
  fetchPlan: (projectPath: string) => Promise<void>;
  refreshAll: (projectPath: string) => Promise<void>;

  // UI actions
  setActiveSubTab: (tab: BeadsSubTab) => void;
  setProjectPath: (path: string) => void;
  setFilters: (filters: Partial<BeadsFilters>) => void;
  selectIssue: (issueId: string | null) => void;
  clearError: () => void;
}

export type BeadsContextType = BeadsContextState & BeadsContextActions;

// ============================================================================
// COMPONENT PROP TYPES
// ============================================================================

export interface BeadsGraphViewProps {
  projectPath: string;
  onIssueSelect?: (issueId: string) => void;
}

export interface BeadsKanbanViewProps {
  projectPath: string;
  onIssueSelect?: (issueId: string) => void;
}

export interface BeadsTriageViewProps {
  projectPath: string;
  onIssueSelect?: (issueId: string) => void;
}

export interface BeadsInsightsViewProps {
  projectPath: string;
}

export interface BeadsIssueDetailProps {
  issue: BeadsIssue;
  onClose: () => void;
}

// ============================================================================
// CONSTANTS
// ============================================================================

export const BEADS_STATUS_COLORS: Record<BeadsIssueStatus, string> = {
  open: '#4CAF50',
  in_progress: '#2196F3',
  blocked: '#f44336',
  deferred: '#FF9800',
  pinned: '#9C27B0',
  hooked: '#00BCD4',
  closed: '#9E9E9E',
  tombstone: '#607D8B',
};

export const BEADS_STATUS_LABELS: Record<BeadsIssueStatus, string> = {
  open: 'Open',
  in_progress: 'In Progress',
  blocked: 'Blocked',
  deferred: 'Deferred',
  pinned: 'Pinned',
  hooked: 'Hooked',
  closed: 'Closed',
  tombstone: 'Tombstone',
};

export const BEADS_TYPE_ICONS: Record<BeadsIssueType, string> = {
  bug: '🐛',
  feature: '✨',
  task: '📋',
  epic: '🏔️',
  chore: '🔧',
};

export const BEADS_PRIORITY_COLORS: Record<number, string> = {
  1: '#f44336', // Critical
  2: '#FF9800', // High
  3: '#FFC107', // Medium
  4: '#4CAF50', // Low
  5: '#9E9E9E', // Minimal
};

export const DEFAULT_BEADS_FILTERS: BeadsFilters = {
  status: [],
  type: [],
  labels: [],
  assignees: [],
  searchQuery: '',
};

export const DEFAULT_BEADS_VIEW_STATE: BeadsViewState = {
  activeSubTab: 'graph',
  projectPath: '/workspace',
  filters: DEFAULT_BEADS_FILTERS,
  selectedIssueId: null,
};
