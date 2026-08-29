import { useCallback, useEffect, useRef, useState } from 'react'
import type { Dispatch, SetStateAction } from 'react'
import type { LaunchUser, SessionsResponse, TerminalWindow, TerminalWorkspace, TmuxSession, WorkspaceId } from '../types'
import { getSessionKey, normalizeTerminalUsers } from '../types'
import { idsInWorkspaces } from './workspaceLayouts'

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

export function liveSessionKeys(sessions: TmuxSession[]): Set<string> {
  const live = new Set<string>()
  sessions.forEach(session => {
    live.add(getSessionKey(session.name, session.unixUser))
    live.add(session.name)
  })
  return live
}

export function pruneWindowToLiveSessions(
  window: TerminalWindow,
  live: Set<string>,
  pruneCandidates?: Set<string>,
): TerminalWindow {
  const boundSessions = window.boundSessions.filter(session => (
    session === 'INIT-PENDING' || live.has(session) || (pruneCandidates ? !pruneCandidates.has(session) : false)
  ))
  const activeSession = window.activeSession && boundSessions.includes(window.activeSession)
    ? window.activeSession
    : (pruneCandidates
        ? (boundSessions.find(sessionName => sessionName === 'INIT-PENDING' || live.has(sessionName)) ?? null)
        : (boundSessions[0] ?? null))
  if (boundSessions.length === window.boundSessions.length && activeSession === window.activeSession) return window
  return { ...window, boundSessions, activeSession }
}

function pruneWorkspacesToLiveSessions(
  workspaces: Record<WorkspaceId, TerminalWorkspace>,
  sessions: TmuxSession[],
  pruneCandidates?: Set<string>,
): Record<WorkspaceId, TerminalWorkspace> {
  const live = liveSessionKeys(sessions)
  let changed = false
  const next: Record<WorkspaceId, TerminalWorkspace> = { ...workspaces }
  idsInWorkspaces(workspaces).forEach(workspaceId => {
    const workspace = workspaces[workspaceId]
    const windows = workspace.windows.map(window => {
      const pruned = pruneWindowToLiveSessions(window, live, pruneCandidates)
      if (pruned !== window) changed = true
      return pruned
    })
    next[workspaceId] = windows === workspace.windows ? workspace : { ...workspace, windows }
  })
  return changed ? next : workspaces
}

function staleSessionKeysInWorkspaces(
  workspaces: Record<WorkspaceId, TerminalWorkspace>,
  live: Set<string>,
): Set<string> {
  const stale = new Set<string>()
  idsInWorkspaces(workspaces).forEach(workspaceId => {
    workspaces[workspaceId].windows.forEach(window => {
      window.boundSessions.forEach(sessionName => {
        if (sessionName !== 'INIT-PENDING' && !live.has(sessionName)) stale.add(sessionName)
      })
    })
  })
  return stale
}

interface SessionsPollOptions {
  autoRefreshInterval: number
  setWorkspaces: Dispatch<SetStateAction<Record<WorkspaceId, TerminalWorkspace>>>
  setFloatingSession: Dispatch<SetStateAction<string | null>>
  setSendToSessionTarget: Dispatch<SetStateAction<string | null>>
}

export function useSessionsPoll({
  autoRefreshInterval,
  setWorkspaces,
  setFloatingSession,
  setSendToSessionTarget,
}: SessionsPollOptions) {
  const [sessions, setSessions] = useState<TmuxSession[]>([])
  const [groupedSessions, setGroupedSessions] = useState<Record<string, TmuxSession[]>>({})
  const [terminalUsers, setTerminalUsers] = useState<LaunchUser[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const staleSessionCandidatesRef = useRef<Set<string>>(new Set())
  const staleSessionProtectionRef = useRef<Map<string, number>>(new Map())
  const refreshMountedRef = useRef(false)
  const refreshGenerationRef = useRef(0)
  const trailingRefreshRef = useRef(false)
  const activeRefreshRef = useRef<{
    timeout: ReturnType<typeof setTimeout>
    promise: Promise<void>
    cancel: (reason: 'timeout' | 'lifecycle') => void
  } | null>(null)

  const forgetStaleSessionAliases = useCallback((aliases: string[]) => {
    aliases.forEach(alias => staleSessionCandidatesRef.current.delete(alias))
  }, [])

  const protectStaleSessionAliases = useCallback((aliases: string[]) => {
    aliases.forEach(alias => {
      if (alias) staleSessionProtectionRef.current.set(alias, 2)
    })
  }, [])

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
        const nextSessions = Array.isArray(data.sessions) ? data.sessions : []
        setError(typeof data.error === 'string' ? data.error : null)
        setSessions(nextSessions)
        setGroupedSessions(isRecord(data.grouped) ? data.grouped as Record<string, TmuxSession[]> : {})
        if (Array.isArray(data.terminalUsers)) setTerminalUsers(normalizeTerminalUsers(data.terminalUsers))

        if (Array.isArray(data.sessions)) {
          const live = liveSessionKeys(nextSessions)
          const protectedKeys = new Set(staleSessionProtectionRef.current.keys())
          const pruneCandidates = new Set([...staleSessionCandidatesRef.current].filter(key => !protectedKeys.has(key)))
          setWorkspaces(previous => {
            const pruned = pruneWorkspacesToLiveSessions(previous, nextSessions, pruneCandidates)
            const currentStale = staleSessionKeysInWorkspaces(pruned, live)
            protectedKeys.forEach(key => currentStale.delete(key))
            staleSessionCandidatesRef.current = currentStale
            return pruned
          })
          setFloatingSession(previous => previous && (live.has(previous) || protectedKeys.has(previous) || !pruneCandidates.has(previous)) ? previous : null)
          setSendToSessionTarget(previous => previous && (live.has(previous) || protectedKeys.has(previous) || !pruneCandidates.has(previous)) ? previous : null)
          staleSessionProtectionRef.current.forEach((remaining, key) => {
            if (remaining <= 1) staleSessionProtectionRef.current.delete(key)
            else staleSessionProtectionRef.current.set(key, remaining - 1)
          })
        }
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
  }, [setFloatingSession, setSendToSessionTarget, setWorkspaces])

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
    forgetStaleSessionAliases,
    protectStaleSessionAliases,
  }
}
