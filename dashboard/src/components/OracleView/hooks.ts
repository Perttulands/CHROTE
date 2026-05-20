// Oracle data fetching hooks

import { useState, useEffect, useCallback, useRef } from 'react'
import type {
  OracleAgent,
  OracleStatusData,
  OracleEvent,
  ActivityEntry,
  RalphProject,
  ApiResponse,
} from './types'

const API_BASE = '/api/oracle'

async function fetchApi<T>(endpoint: string): Promise<ApiResponse<T>> {
  const response = await fetch(endpoint, { signal: AbortSignal.timeout(10000) })
  return response.json()
}

// Hook to fetch aggregate oracle status
export function useOracleStatus(pollInterval = 10000) {
  const [status, setStatus] = useState<OracleStatusData | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    try {
      const result = await fetchApi<OracleStatusData>(`${API_BASE}/status`)
      if (result.success && result.data) {
        setStatus(result.data)
        setError(null)
      } else {
        setError(result.error?.message || 'Failed to fetch status')
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Network error')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    refresh()
    const interval = setInterval(refresh, pollInterval)
    return () => clearInterval(interval)
  }, [refresh, pollInterval])

  return { status, loading, error, refresh }
}

// Hook to fetch agent list
export function useAgents(pollInterval = 10000) {
  const [agents, setAgents] = useState<OracleAgent[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    try {
      const result = await fetchApi<{ agents: OracleAgent[]; count: number }>(`${API_BASE}/agents`)
      if (result.success && result.data) {
        setAgents(result.data.agents)
        setError(null)
      } else {
        setError(result.error?.message || 'Failed to fetch agents')
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Network error')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    refresh()
    const interval = setInterval(refresh, pollInterval)
    return () => clearInterval(interval)
  }, [refresh, pollInterval])

  return { agents, loading, error, refresh }
}

// Hook for SSE stream — feeds activity log and augments agent state
export function useOracleStream() {
  const [connected, setConnected] = useState(false)
  const [activity, setActivity] = useState<ActivityEntry[]>([])
  const eventSourceRef = useRef<EventSource | null>(null)
  const reconnectTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const idCounterRef = useRef(0)

  const addActivity = useCallback((type: string, agentName: string, message: string) => {
    const entry: ActivityEntry = {
      id: `evt-${++idCounterRef.current}`,
      type,
      agentName,
      message,
      timestamp: new Date().toISOString(),
    }
    setActivity(prev => [entry, ...prev].slice(0, 100)) // Keep last 100
  }, [])

  const connect = useCallback(() => {
    if (eventSourceRef.current) {
      eventSourceRef.current.close()
    }

    const es = new EventSource(`${API_BASE}/stream`)
    eventSourceRef.current = es

    es.addEventListener('connected', () => {
      setConnected(true)
    })

    es.addEventListener('agent_new', (e) => {
      try {
        const event: OracleEvent = JSON.parse(e.data)
        const name = (event.data as Record<string, string>).Name || 'unknown'
        addActivity('agent_new', name, `Agent ${name} started`)
      } catch { /* ignore parse errors */ }
    })

    es.addEventListener('agent_status', (e) => {
      try {
        const event: OracleEvent = JSON.parse(e.data)
        const data = event.data as Record<string, string>
        const name = data.Name || 'unknown'
        const status = data.Status || 'unknown'
        addActivity('agent_status', name, `${name} is now ${status}`)
      } catch { /* ignore parse errors */ }
    })

    es.addEventListener('agent_removed', (e) => {
      try {
        const event: OracleEvent = JSON.parse(e.data)
        const name = (event.data as Record<string, string>).Name || 'unknown'
        addActivity('agent_removed', name, `Agent ${name} removed`)
      } catch { /* ignore parse errors */ }
    })

    es.addEventListener('heartbeat', () => {
      setConnected(true)
    })

    es.onerror = () => {
      setConnected(false)
      es.close()
      eventSourceRef.current = null

      // Auto-reconnect after 5s
      reconnectTimeoutRef.current = setTimeout(() => {
        connect()
      }, 5000)
    }
  }, [addActivity])

  useEffect(() => {
    connect()

    return () => {
      if (eventSourceRef.current) {
        eventSourceRef.current.close()
        eventSourceRef.current = null
      }
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current)
      }
    }
  }, [connect])

  return { connected, activity }
}

// Hook to fetch Ralph status
export function useRalph(pollInterval = 30000) {
  const [ralph, setRalph] = useState<RalphProject[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    try {
      const result = await fetchApi<{ ralph: RalphProject[]; count: number }>(`${API_BASE}/ralph`)
      if (result.success && result.data) {
        setRalph(result.data.ralph)
        setError(null)
      } else {
        setError(result.error?.message || 'Failed to fetch Ralph status')
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Network error')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    refresh()
    const interval = setInterval(refresh, pollInterval)
    return () => clearInterval(interval)
  }, [refresh, pollInterval])

  return { ralph, loading, error, refresh }
}
