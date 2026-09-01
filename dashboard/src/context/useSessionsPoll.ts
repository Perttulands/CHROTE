// The session poll reports what tmux lists and nothing else.
//
// It used to also own a prune: bindings, peek and the send target were removed
// when a session stopped appearing, behind a stale-candidate set and a two-tick
// protection counter that existed only to stop that prune eating sessions the
// poll had not seen yet. Bindings are operator intent now (ADR-0017 decision
// 5), so a session that disappears changes what a tile *shows*, never what it
// holds, and none of that machinery has anything left to protect.

import { useCallback, useEffect, useRef, useState } from 'react'
import type { LaunchUser, SessionsResponse, TmuxSession } from '../types'
import { normalizeTerminalUsers } from '../types'

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function mergeFailedUserSessions(
  previous: TmuxSession[],
  received: TmuxSession[],
  failedUsers: readonly LaunchUser[],
): TmuxSession[] {
  const failed = new Set(failedUsers)
  return [...received, ...previous.filter(session => session.unixUser && failed.has(session.unixUser))]
}

function groupSessions(sessions: TmuxSession[]): Record<string, TmuxSession[]> {
  return sessions.reduce<Record<string, TmuxSession[]>>((groups, session) => {
    (groups[session.group] ??= []).push(session)
    return groups
  }, {})
}

interface SessionsPollOptions {
  autoRefreshInterval: number
}

export function useSessionsPoll({ autoRefreshInterval }: SessionsPollOptions) {
  const [sessions, setSessions] = useState<TmuxSession[]>([])
  const [groupedSessions, setGroupedSessions] = useState<Record<string, TmuxSession[]>>({})
  const [terminalUsers, setTerminalUsers] = useState<LaunchUser[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const sessionsRef = useRef<TmuxSession[]>([])
  const refreshMountedRef = useRef(false)
  const refreshGenerationRef = useRef(0)
  const trailingRefreshRef = useRef(false)
  const activeRefreshRef = useRef<{
    timeout: ReturnType<typeof setTimeout>
    promise: Promise<void>
    cancel: (reason: 'timeout' | 'lifecycle') => void
  } | null>(null)

  const refreshSessions: () => Promise<void> = useCallback(() => {
    if (!refreshMountedRef.current) return Promise.resolve()
    const current = activeRefreshRef.current
    if (current) {
      trailingRefreshRef.current = true
      return current.promise.then(() => activeRefreshRef.current?.promise ?? Promise.resolve())
    }

    type CancellationReason = 'timeout' | 'lifecycle'
    const controller = new AbortController()
    const generation = refreshGenerationRef.current
    let cancellationReason: CancellationReason | null = null
    let resolveCancellation!: (reason: CancellationReason) => void
    const cancellation = new Promise<CancellationReason>(resolve => { resolveCancellation = resolve })
    const cancel = (reason: CancellationReason) => {
      if (cancellationReason) return
      cancellationReason = reason
      resolveCancellation(reason)
      controller.abort()
    }
    const active = {
      cancel,
      timeout: setTimeout(() => cancel('timeout'), 10000),
      promise: Promise.resolve(),
    }
    activeRefreshRef.current = active
    const hasCurrentCleanupAuthority = () => (
      refreshMountedRef.current && refreshGenerationRef.current === generation && activeRefreshRef.current === active
    )
    const isAuthoritative = () => hasCurrentCleanupAuthority() && cancellationReason === null
    const raceWithCancellation = <T,>(operation: Promise<T>) => Promise.race([
      operation.then(
        value => ({ kind: 'value' as const, value }),
        failure => ({ kind: 'failure' as const, failure }),
      ),
      cancellation.then(reason => ({ kind: 'cancelled' as const, reason })),
    ])
    const reportCancellation = (reason: CancellationReason) => {
      if (reason === 'timeout' && hasCurrentCleanupAuthority()) setError('Failed to fetch sessions (request timed out)')
    }

    active.promise = (async () => {
      try {
        const responseOutcome = await raceWithCancellation(fetch('/api/tmux/sessions', { signal: controller.signal }))
        if (responseOutcome.kind === 'cancelled') {
          reportCancellation(responseOutcome.reason)
          return
        }
        if (responseOutcome.kind === 'failure') {
          if (cancellationReason) reportCancellation(cancellationReason)
          else if (isAuthoritative()) {
            setError('Failed to fetch sessions')
            console.error('Failed to fetch sessions:', responseOutcome.failure)
          }
          return
        }
        if (!isAuthoritative()) return

        const response = responseOutcome.value
        const dataOutcome = await raceWithCancellation(response.json() as Promise<Partial<SessionsResponse>>)
        if (dataOutcome.kind === 'cancelled') {
          reportCancellation(dataOutcome.reason)
          return
        }
        if (dataOutcome.kind === 'failure') {
          if (cancellationReason) reportCancellation(cancellationReason)
          else if (isAuthoritative()) setError('Failed to fetch sessions')
          return
        }
        if (!isAuthoritative()) return

        const data = dataOutcome.value
        const isPartial = response.ok && data.partial === true
        if (!response.ok || (data.error && !isPartial)) {
          setError(typeof data.error === 'string' ? data.error : 'Failed to fetch sessions')
          return
        }
        const receivedSessions = Array.isArray(data.sessions) ? data.sessions : []
        const partialFailedUsers = isPartial && Array.isArray(data.successfulUsers) && Array.isArray(data.failedUsers)
          ? data.failedUsers
          : null
        const nextSessions = partialFailedUsers
          ? mergeFailedUserSessions(sessionsRef.current, receivedSessions, partialFailedUsers)
          : receivedSessions
        setError(typeof data.error === 'string' ? data.error : null)
        sessionsRef.current = nextSessions
        setSessions(nextSessions)
        setGroupedSessions(partialFailedUsers
          ? groupSessions(nextSessions)
          : (isRecord(data.grouped) ? data.grouped as Record<string, TmuxSession[]> : {}))
        if (Array.isArray(data.terminalUsers)) setTerminalUsers(normalizeTerminalUsers(data.terminalUsers))
      } catch (e) {
        if (isAuthoritative()) {
          setError('Failed to fetch sessions')
          console.error('Failed to fetch sessions:', e)
        }
      } finally {
        const hasCleanupAuthority = hasCurrentCleanupAuthority()
        clearTimeout(active.timeout)
        if (activeRefreshRef.current === active) {
          activeRefreshRef.current = null
          if (hasCleanupAuthority) setLoading(false)
          if (!refreshMountedRef.current || refreshGenerationRef.current !== generation) {
            trailingRefreshRef.current = false
          } else if (trailingRefreshRef.current) {
            trailingRefreshRef.current = false
            void refreshSessions()
          }
        }
      }
    })()
    return active.promise
  }, [])

  useEffect(() => {
    refreshMountedRef.current = true
    return () => {
      refreshMountedRef.current = false
      refreshGenerationRef.current += 1
      trailingRefreshRef.current = false
      const active = activeRefreshRef.current
      activeRefreshRef.current = null
      if (active) {
        clearTimeout(active.timeout)
        active.cancel('lifecycle')
      }
    }
  }, [])

  useEffect(() => {
    refreshSessions()
    const interval = setInterval(refreshSessions, autoRefreshInterval)
    return () => clearInterval(interval)
  }, [refreshSessions, autoRefreshInterval])

  return {
    sessions,
    groupedSessions,
    terminalUsers,
    loading,
    error,
    refreshSessions,
  }
}
