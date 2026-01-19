// Beads data fetching hooks

import { useState, useEffect, useCallback } from 'react'
import { useSession } from '../../context/SessionContext'
import type {
  BeadsProject,
  BeadsIssue,
  TriageResponse,
  InsightsResponse,
  GraphResponse,
  ApiResponse,
} from './types'

const API_BASE = '/api/beads'

async function fetchApi<T>(endpoint: string, params?: Record<string, string>): Promise<ApiResponse<T>> {
  const url = new URL(endpoint, window.location.origin)
  if (params) {
    Object.entries(params).forEach(([key, value]) => {
      url.searchParams.set(key, value)
    })
  }

  const response = await fetch(url.toString())
  return response.json()
}

// Hook to fetch available projects
export function useProjects() {
  const { settings } = useSession()
  const [projects, setProjects] = useState<BeadsProject[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      // Fetch auto-discovered projects
      const result = await fetchApi<{ projects: BeadsProject[] }>(`${API_BASE}/projects`)
      let allProjects: BeadsProject[] = []

      if (result.success && result.data) {
        allProjects = [...result.data.projects]
      }

      // Add manually configured paths from settings
      const manualPaths = settings.beadsProjectPaths || []
      for (const path of manualPaths) {
        // Check if path is already in auto-discovered list
        if (allProjects.some(p => p.path === path)) {
          continue
        }

        // Create a project entry for the manual path
        const name = path.split('/').filter(Boolean).pop() || path
        allProjects.push({
          name,
          path,
          beadsPath: `${path}/.beads`
        })
      }

      setProjects(allProjects)

      if (!result.success && manualPaths.length === 0) {
        setError(result.error?.message || 'Failed to fetch projects')
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Network error')
    } finally {
      setLoading(false)
    }
  }, [settings.beadsProjectPaths])

  useEffect(() => {
    refresh()
  }, [refresh])

  return { projects, loading, error, refresh }
}

// Hook to fetch issues for a project
export function useIssues(projectPath: string | null) {
  const [issues, setIssues] = useState<BeadsIssue[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    if (!projectPath) {
      setIssues([])
      return
    }

    setLoading(true)
    setError(null)
    try {
      const result = await fetchApi<{ issues: BeadsIssue[]; totalCount: number }>(
        `${API_BASE}/issues`,
        { path: projectPath }
      )
      if (result.success && result.data) {
        setIssues(result.data.issues)
      } else {
        setError(result.error?.message || 'Failed to fetch issues')
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Network error')
    } finally {
      setLoading(false)
    }
  }, [projectPath])

  useEffect(() => {
    refresh()
  }, [refresh])

  return { issues, loading, error, refresh }
}

// Hook to fetch triage recommendations
export function useTriage(projectPath: string | null) {
  const [triage, setTriage] = useState<TriageResponse | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    if (!projectPath) {
      setTriage(null)
      return
    }

    setLoading(true)
    setError(null)
    try {
      const result = await fetchApi<TriageResponse>(`${API_BASE}/triage`, { path: projectPath })
      if (result.success && result.data) {
        setTriage(result.data)
      } else {
        setError(result.error?.message || 'Failed to fetch triage')
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Network error')
    } finally {
      setLoading(false)
    }
  }, [projectPath])

  useEffect(() => {
    refresh()
  }, [refresh])

  return { triage, loading, error, refresh }
}

// Hook to fetch insights/metrics
export function useInsights(projectPath: string | null) {
  const [insights, setInsights] = useState<InsightsResponse | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    if (!projectPath) {
      setInsights(null)
      return
    }

    setLoading(true)
    setError(null)
    try {
      const result = await fetchApi<InsightsResponse>(`${API_BASE}/insights`, { path: projectPath })
      if (result.success && result.data) {
        setInsights(result.data)
      } else {
        setError(result.error?.message || 'Failed to fetch insights')
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Network error')
    } finally {
      setLoading(false)
    }
  }, [projectPath])

  useEffect(() => {
    refresh()
  }, [refresh])

  return { insights, loading, error, refresh }
}

// Hook to fetch graph data
export function useGraph(projectPath: string | null) {
  const [graph, setGraph] = useState<GraphResponse | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    if (!projectPath) {
      setGraph(null)
      return
    }

    setLoading(true)
    setError(null)
    try {
      const result = await fetchApi<GraphResponse>(`${API_BASE}/graph`, { path: projectPath })
      if (result.success && result.data) {
        setGraph(result.data)
      } else {
        setError(result.error?.message || 'Failed to fetch graph')
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Network error')
    } finally {
      setLoading(false)
    }
  }, [projectPath])

  useEffect(() => {
    refresh()
  }, [refresh])

  return { graph, loading, error, refresh }
}

// Combined hook for all beads data
export function useBeadsData(projectPath: string | null) {
  const issues = useIssues(projectPath)
  const triage = useTriage(projectPath)
  const insights = useInsights(projectPath)
  const graph = useGraph(projectPath)

  const refreshAll = useCallback(() => {
    issues.refresh()
    triage.refresh()
    insights.refresh()
    graph.refresh()
  }, [issues, triage, insights, graph])

  return {
    issues: issues.issues,
    triage: triage.triage,
    insights: insights.insights,
    graph: graph.graph,
    loading: issues.loading || triage.loading || insights.loading || graph.loading,
    error: issues.error || triage.error || insights.error || graph.error,
    refresh: refreshAll,
  }
}
