// Beads Module Context - Self-contained state management
// This module is designed to be removable without affecting the rest of the application

import {
  createContext,
  useContext,
  useState,
  useCallback,
  useMemo,
  useEffect,
  ReactNode,
} from 'react';

import type {
  BeadsContextType,
  BeadsIssue,
  BeadsTriage,
  BeadsInsights,
  BeadsPlan,
  BeadsGraphData,
  BeadsGraphNode,
  BeadsGraphLink,
  BeadsSubTab,
  BeadsFilters,
  BeadsApiError,
} from './types';

import {
  DEFAULT_BEADS_FILTERS,
} from './types';

import * as api from './api';

// ============================================================================
// CONTEXT CREATION
// ============================================================================

const BeadsContext = createContext<BeadsContextType | null>(null);

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

function buildGraphData(issues: BeadsIssue[]): BeadsGraphData {
  const nodes: BeadsGraphNode[] = issues.map((issue) => ({
    id: issue.id,
    name: issue.title,
    status: issue.status,
    priority: issue.priority,
    type: issue.type,
    labels: issue.labels,
  }));

  const issueIds = new Set(issues.map((i) => i.id));
  const links: BeadsGraphLink[] = [];

  issues.forEach((issue) => {
    // Dependencies (this issue depends on others)
    issue.dependencies?.forEach((depId) => {
      if (issueIds.has(depId)) {
        links.push({
          source: depId,
          target: issue.id,
          type: 'depends',
        });
      }
    });

    // Blocking (this issue blocks others)
    issue.blocking?.forEach((blockId) => {
      if (issueIds.has(blockId)) {
        links.push({
          source: issue.id,
          target: blockId,
          type: 'blocks',
        });
      }
    });

    // Related
    issue.related?.forEach((relId) => {
      if (issueIds.has(relId)) {
        // Avoid duplicate related links
        const existing = links.find(
          (l) =>
            l.type === 'related' &&
            ((l.source === issue.id && l.target === relId) ||
              (l.source === relId && l.target === issue.id))
        );
        if (!existing) {
          links.push({
            source: issue.id,
            target: relId,
            type: 'related',
          });
        }
      }
    });
  });

  return { nodes, links };
}

// ============================================================================
// PROVIDER COMPONENT
// ============================================================================

interface BeadsProviderProps {
  children: ReactNode;
  initialProjectPath?: string;
}

export function BeadsProvider({ children, initialProjectPath = '/workspace' }: BeadsProviderProps) {
  // Data state
  const [issues, setIssues] = useState<BeadsIssue[]>([]);
  const [triage, setTriage] = useState<BeadsTriage | null>(null);
  const [insights, setInsights] = useState<BeadsInsights | null>(null);
  const [plan, setPlan] = useState<BeadsPlan | null>(null);
  const [availableProjects, setAvailableProjects] = useState<string[]>([initialProjectPath]);

  // UI state
  const [activeSubTab, setActiveSubTab] = useState<BeadsSubTab>('graph');
  const [projectPath, setProjectPathState] = useState(initialProjectPath);
  const [filters, setFiltersState] = useState<BeadsFilters>(DEFAULT_BEADS_FILTERS);
  const [selectedIssueId, setSelectedIssueId] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<BeadsApiError | null>(null);

  // Computed graph data
  const graphData = useMemo(() => {
    if (issues.length === 0) return null;
    return buildGraphData(issues);
  }, [issues]);

  // Discover projects on mount
  useEffect(() => {
    const discover = async () => {
      try {
        const response = await api.discoverProjects();
        if (response.success && response.data?.projects) {
          const newProjects = response.data.projects;
          setAvailableProjects((prev) => {
            const set = new Set([...prev, ...newProjects]);
            return Array.from(set);
          });
        }
      } catch (e) {
        console.error('Failed to discover projects:', e);
      }
    };
    
    discover();
  }, []);

  // ============================================================================
  // DATA FETCHING ACTIONS
  // ============================================================================

  const fetchIssues = useCallback(async (path: string) => {
    setIsLoading(true);
    setError(null);

    const response = await api.fetchIssues(path);

    if (response.success && response.data) {
      setIssues(response.data.issues);
    } else if (response.error) {
      setError(response.error);
      setIssues([]);
    }

    setIsLoading(false);
  }, []);

  const fetchTriage = useCallback(async (path: string) => {
    setIsLoading(true);
    setError(null);

    const response = await api.fetchTriage(path);

    if (response.success && response.data) {
      setTriage(response.data);
    } else if (response.error) {
      setError(response.error);
      setTriage(null);
    }

    setIsLoading(false);
  }, []);

  const fetchInsights = useCallback(async (path: string) => {
    setIsLoading(true);
    setError(null);

    const response = await api.fetchInsights(path);

    if (response.success && response.data) {
      setInsights(response.data);
    } else if (response.error) {
      setError(response.error);
      setInsights(null);
    }

    setIsLoading(false);
  }, []);

  const fetchPlan = useCallback(async (path: string) => {
    setIsLoading(true);
    setError(null);

    const response = await api.fetchPlan(path);

    if (response.success && response.data) {
      setPlan(response.data);
    } else if (response.error) {
      setError(response.error);
      setPlan(null);
    }

    setIsLoading(false);
  }, []);

  const refreshAll = useCallback(async (path: string) => {
    setIsLoading(true);
    setError(null);

    // Fetch issues first (required for graph)
    const issuesResponse = await api.fetchIssues(path);

    if (issuesResponse.success && issuesResponse.data) {
      setIssues(issuesResponse.data.issues);
    } else if (issuesResponse.error) {
      setError(issuesResponse.error);
      setIssues([]);
      setIsLoading(false);
      return;
    }

    // Fetch other data in parallel
    const [triageResponse, insightsResponse] = await Promise.all([
      api.fetchTriage(path),
      api.fetchInsights(path),
    ]);

    if (triageResponse.success && triageResponse.data) {
      setTriage(triageResponse.data);
    }

    if (insightsResponse.success && insightsResponse.data) {
      setInsights(insightsResponse.data);
    }

    // Check for errors from parallel requests
    if (!triageResponse.success && triageResponse.error) {
      console.error('Failed to fetch triage:', triageResponse.error);
    }
    if (!insightsResponse.success && insightsResponse.error) {
      console.error('Failed to fetch insights:', insightsResponse.error);
    }

    setIsLoading(false);
  }, []);

  // ============================================================================
  // UI ACTIONS
  // ============================================================================

  const setProjectPath = useCallback((path: string) => {
    setProjectPathState(path);
    // Clear data when project changes
    setIssues([]);
    setTriage(null);
    setInsights(null);
    setPlan(null);
    setSelectedIssueId(null);
    setError(null);
  }, []);

  const setFilters = useCallback((newFilters: Partial<BeadsFilters>) => {
    setFiltersState((prev) => ({ ...prev, ...newFilters }));
  }, []);

  const selectIssue = useCallback((issueId: string | null) => {
    setSelectedIssueId(issueId);
  }, []);

  const clearError = useCallback(() => {
    setError(null);
  }, []);

  // ============================================================================
  // CONTEXT VALUE
  // ============================================================================

  const contextValue: BeadsContextType = useMemo(
    () => ({
      // Data
      issues,
      triage,
      insights,
      plan,
      graphData,
      availableProjects,

      // UI State
      viewState: {
        activeSubTab,
        projectPath,
        filters,
        selectedIssueId,
      },
      isLoading,
      error,

      // Actions
      fetchIssues,
      fetchTriage,
      fetchInsights,
      fetchPlan,
      refreshAll,
      setActiveSubTab,
      setProjectPath,
      setFilters,
      selectIssue,
      clearError,
    }),
    [
      issues,
      triage,
      insights,
      plan,
      graphData,
      availableProjects,
      activeSubTab,
      projectPath,
      filters,
      selectedIssueId,
      isLoading,
      error,
      fetchIssues,
      fetchTriage,
      fetchInsights,
      fetchPlan,
      refreshAll,
      setProjectPath,
      setFilters,
      selectIssue,
      clearError,
    ]
  );

  return (
    <BeadsContext.Provider value={contextValue}>
      {children}
    </BeadsContext.Provider>
  );
}

// ============================================================================
// HOOK
// ============================================================================

export function useBeads(): BeadsContextType {
  const context = useContext(BeadsContext);
  if (!context) {
    throw new Error('useBeads must be used within a BeadsProvider');
  }
  return context;
}

// ============================================================================
// OPTIONAL: Hook for components that might be outside provider
// ============================================================================

export function useBeadsOptional(): BeadsContextType | null {
  return useContext(BeadsContext);
}
