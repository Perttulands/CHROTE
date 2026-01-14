import { useState, useEffect, useCallback } from 'react'
import type { TmuxSession, SessionsResponse } from '../types'

const POLL_INTERVAL = 5000 // 5 seconds

interface UseTmuxSessionsResult {
  sessions: TmuxSession[]
  groupedSessions: Record<string, TmuxSession[]>
  loading: boolean
  error: string | null
  refresh: () => Promise<void>
}

export function useTmuxSessions(): UseTmuxSessionsResult {
  const [sessions, setSessions] = useState<TmuxSession[]>([])
  const [groupedSessions, setGroupedSessions] = useState<Record<string, TmuxSession[]>>({})
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    try {
      const response = await fetch('/api/tmux/sessions')
      const data: SessionsResponse = await response.json()

      if (data.error) {
        setError(data.error)
        setSessions([])
        setGroupedSessions({})
      } else {
        setError(null)
        setSessions(data.sessions)
        setGroupedSessions(data.grouped)
      }
    } catch (e) {
      setError('Failed to fetch sessions')
      console.error('Failed to fetch sessions:', e)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    refresh()
    const interval = setInterval(refresh, POLL_INTERVAL)
    return () => clearInterval(interval)
  }, [refresh])

  return { sessions, groupedSessions, loading, error, refresh }
}
