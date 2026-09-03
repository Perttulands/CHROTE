import { createContext, useCallback, useContext, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import type {
  CreateSessionOptions,
  DashboardContextType,
  LaunchUser,
  TerminalWorkspace,
  TmuxSession,
  WorkspaceId,
} from '../types'
import { getSessionKey, getSessionNameFromKey, getSessionUserFromKey, getSessionPrefixForUser, resolveLaunchUser } from '../types'
import {
  NO_SESSION_EVIDENCE,
  retainSessionEvidence,
  sessionEvidenceFrom,
  type SessionEvidence,
} from '../terminal/tileState'
import { useStatus } from './StatusContext'
import { useSendToSession } from './useSendToSession'
import { useSessionsPoll } from './useSessionsPoll'
import { useWorkspaceLayouts } from './useWorkspaceLayouts'
import {
  deduplicateWorkspaceBindings,
  idsInWorkspaces,
  qualifiedUsersBySessionName,
  safeSessionAliases,
  sessionBindingIdentity,
} from './workspaceLayouts'

function nextSessionNameForPrefix(sessions: TmuxSession[], prefix: string): string {
  const escapedPrefix = prefix.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const regex = new RegExp(`^${escapedPrefix}(\\d+)$`)
  const existingNumbers = sessions
    .map(session => session.name.match(regex))
    .filter(Boolean)
    .map(match => parseInt(match![1], 10))
  return `${prefix}${existingNumbers.length > 0 ? Math.max(...existingNumbers) + 1 : 1}`
}

/**
 * What the server said went wrong inside a session it did create, or null. A
 * body that cannot be read is not a warning: the session was created, and the
 * dashboard has nothing to add to that.
 */
async function readCreateSessionWarning(response: Response): Promise<string | null> {
  try {
    const body = await response.json() as { warning?: unknown }
    return typeof body?.warning === 'string' && body.warning.trim() !== '' ? body.warning : null
  } catch {
    return null
  }
}

/**
 * The dashboard state plus the one join the tile layer makes on it. The join is
 * held here rather than in each reader so a tile, its peek and the Send dialog
 * cannot reach different verdicts about the same session.
 */
export type SessionContextValue = DashboardContextType & { sessionEvidence: SessionEvidence }

const SessionContext = createContext<SessionContextValue | null>(null)

export function SessionProvider({ children }: { children: ReactNode }) {
  const { announce } = useStatus()
  const layouts = useWorkspaceLayouts()
  const send = useSendToSession()
  const [floatingSession, setFloatingSession] = useState<string | null>(null)
  const poll = useSessionsPoll({ autoRefreshInterval: layouts.settings.autoRefreshInterval })

  // What the last poll that answered is entitled to say about a binding, kept
  // through one that did not. Computed once, for every reader.
  const heardEvidence = useRef<SessionEvidence>(NO_SESSION_EVIDENCE)
  heardEvidence.current = retainSessionEvidence(
    heardEvidence.current,
    sessionEvidenceFrom({
      sessions: poll.sessions,
      loading: poll.loading,
      error: poll.error,
      partialAnsweringUsers: poll.partialAnsweringUsers,
    }),
  )
  const sessionEvidence = heardEvidence.current

  const addSessionToWindow = useCallback((
    workspaceId: WorkspaceId,
    windowId: string,
    sessionName: string,
    unixUser?: LaunchUser,
  ) => {
    const sessionKey = getSessionKey(sessionName, unixUser)
    layouts.setWorkspaces(previous => {
      const targetWorkspace = previous[workspaceId]
      if (!targetWorkspace?.windows.some(window => window.id === windowId)) return previous
      const qualifiedUsers = qualifiedUsersBySessionName(previous)
      if (unixUser) {
        const users = qualifiedUsers.get(sessionName) ?? new Set<LaunchUser>()
        users.add(unixUser)
        qualifiedUsers.set(sessionName, users)
      }
      const targetIdentity = sessionBindingIdentity(sessionKey, qualifiedUsers)
      return idsInWorkspaces(previous).reduce((next, currentWorkspaceId) => {
        const workspace = previous[currentWorkspaceId]
        next[currentWorkspaceId] = {
          ...workspace,
          windows: workspace.windows.map(window => {
            const isTarget = currentWorkspaceId === workspaceId && window.id === windowId
            const boundSessions = window.boundSessions.filter(bound => (
              sessionBindingIdentity(bound, qualifiedUsers) !== targetIdentity
            ))
            if (isTarget) return { ...window, boundSessions: [...boundSessions, sessionKey], activeSession: sessionKey }
            const activeWasMoved = window.activeSession !== null &&
              sessionBindingIdentity(window.activeSession, qualifiedUsers) === targetIdentity
            return {
              ...window,
              boundSessions,
              activeSession: activeWasMoved ? (boundSessions[0] ?? null) : window.activeSession,
            }
          }),
        }
        return next
      }, {} as Record<WorkspaceId, TerminalWorkspace>)
    })
  }, [layouts.setWorkspaces])

  const createSession = useCallback(async (options: CreateSessionOptions = {}): Promise<string | null> => {
    const workspaceId = options.workspaceId ?? options.attachTo?.workspaceId ?? 'terminal1'
    const unixUser = options.unixUser ?? resolveLaunchUser(layouts.settings, workspaceId, poll.terminalUsers)
    const prefix = getSessionPrefixForUser(layouts.settings, unixUser, poll.terminalUsers)
    const sessionName = options.name?.trim() || nextSessionNameForPrefix(poll.sessions, prefix)
    try {
      const response = await fetch('/api/tmux/sessions', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          name: sessionName,
          unixUser,
          mouseScroll: options.mouseScroll ?? layouts.settings.mouseScroll,
          ...(options.cwd ? { cwd: options.cwd } : {}),
          ...(options.harness ? { harness: options.harness } : {}),
          // An empty line is a real answer — this launch takes no flags — so
          // it travels, and only an absent field leaves the server its default.
          ...(options.flags !== undefined ? { flags: options.flags } : {}),
        }),
        signal: AbortSignal.timeout(10000),
      })
      if (!response.ok) {
        announce('Failed to create session', 'error')
        return null
      }
      announce(`Session '${sessionName}' created`, 'success')
      // The session exists either way; a warning means the harness inside it
      // did not start, which is the operator's to see and act on.
      const warning = await readCreateSessionWarning(response)
      if (warning) announce(warning, 'warning')
      if (options.attachTo) {
        addSessionToWindow(options.attachTo.workspaceId, options.attachTo.windowId, sessionName, unixUser)
      }
      void poll.refreshSessions()
      return sessionName
    } catch (e) {
      console.error('Failed to create session:', e)
      announce('Failed to create session', 'error')
      return null
    }
  }, [addSessionToWindow, announce, layouts.settings, poll.refreshSessions, poll.sessions, poll.terminalUsers])

  // Restart an ended binding in place: same name, same Unix user, same tile.
  // The previous command is deliberately not re-run — the poll reports only
  // `pane_current_command`, which gives `node` or `bash` rather than the
  // invocation, and guessing wrong is worse than not offering.
  const restartSession = useCallback(async (
    workspaceId: WorkspaceId,
    windowId: string,
    sessionKey: string,
  ): Promise<boolean> => {
    const created = await createSession({
      name: getSessionNameFromKey(sessionKey),
      unixUser: getSessionUserFromKey(sessionKey) || undefined,
      workspaceId,
      attachTo: { workspaceId, windowId },
    })
    return created !== null
  }, [createSession])

  const openFloatingModal = useCallback((sessionName: string) => setFloatingSession(sessionName), [])
  const closeFloatingModal = useCallback(() => setFloatingSession(null), [])
  const handleSessionClick = useCallback((sessionName: string) => openFloatingModal(sessionName), [openFloatingModal])

  const focusSessionAssignment = useCallback((sessionName: string) => {
    const assignment = layouts.assignedSessions.get(sessionName)
    if (!assignment) return
    layouts.revealWindow(assignment.workspaceId, assignment.windowId)
    layouts.setActiveSession(assignment.workspaceId, assignment.windowId, sessionName)
    layouts.setFocusedWindowKey(`${assignment.workspaceId}-${assignment.windowId}`)
  }, [layouts.assignedSessions, layouts.revealWindow, layouts.setActiveSession, layouts.setFocusedWindowKey])

  const deleteSession = useCallback(async (sessionName: string, unixUser?: LaunchUser) => {
    try {
      const query = unixUser ? `?unixUser=${encodeURIComponent(unixUser)}` : ''
      const response = await fetch(`/api/tmux/sessions/${encodeURIComponent(sessionName)}${query}`, {
        method: 'DELETE',
        signal: AbortSignal.timeout(10000),
      })
      if (!response.ok) {
        console.error('Failed to delete session:', await response.text())
        announce('Failed to delete session', 'error')
        return false
      }
      const deletedKey = getSessionKey(sessionName, unixUser)
      layouts.setWorkspaces(previous => {
        const next: Record<WorkspaceId, TerminalWorkspace> = { ...previous }
        idsInWorkspaces(previous).forEach(workspaceId => {
          const workspace = previous[workspaceId]
          next[workspaceId] = {
            ...workspace,
            windows: workspace.windows.map(window => {
              const boundSessions = window.boundSessions.filter(bound => bound !== deletedKey && bound !== sessionName)
              return {
                ...window,
                boundSessions,
                activeSession: window.activeSession && boundSessions.includes(window.activeSession)
                  ? window.activeSession
                  : (boundSessions[0] ?? null),
              }
            }),
          }
        })
        return next
      })
      announce(`Session '${sessionName}' deleted`, 'info')
      poll.refreshSessions()
      return true
    } catch (e) {
      console.error('Failed to delete session:', e)
      announce('Failed to delete session', 'error')
      return false
    }
  }, [announce, layouts.setWorkspaces, poll.refreshSessions])

  const renameSession = useCallback(async (
    oldName: string,
    newName: string,
    unixUser?: LaunchUser,
  ): Promise<boolean> => {
    const qualifiedUsers = qualifiedUsersBySessionName(layouts.workspaces)
    const newKey = getSessionKey(newName, unixUser)
    const sourceAliasSet = new Set(safeSessionAliases(oldName, unixUser, qualifiedUsers))
    try {
      const query = unixUser ? `?unixUser=${encodeURIComponent(unixUser)}` : ''
      const response = await fetch(`/api/tmux/sessions/${encodeURIComponent(oldName)}${query}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ newName }),
        signal: AbortSignal.timeout(10000),
      })
      if (!response.ok) {
        console.error('Failed to rename session:', await response.text())
        announce('Failed to rename session', 'error')
        return false
      }
      layouts.setWorkspaces(previous => {
        const next: Record<WorkspaceId, TerminalWorkspace> = { ...previous }
        idsInWorkspaces(previous).forEach(workspaceId => {
          const workspace = previous[workspaceId]
          next[workspaceId] = {
            ...workspace,
            windows: workspace.windows.map(window => ({
              ...window,
              boundSessions: window.boundSessions.map(bound => sourceAliasSet.has(bound) ? newKey : bound),
              activeSession: window.activeSession && sourceAliasSet.has(window.activeSession) ? newKey : window.activeSession,
            })),
          }
        })
        return deduplicateWorkspaceBindings(next, newKey)
      })
      announce(`Session renamed to '${newName}'`, 'success')
      poll.refreshSessions()
      return true
    } catch (e) {
      console.error('Failed to rename session:', e)
      announce('Failed to rename session', 'error')
      return false
    }
  }, [announce, layouts.setWorkspaces, layouts.workspaces, poll.refreshSessions])

  const contextValue: SessionContextValue = useMemo(() => ({
    sessions: poll.sessions,
    sessionEvidence,
    groupedSessions: poll.groupedSessions,
    terminalUsers: poll.terminalUsers,
    loading: poll.loading,
    error: poll.error,
    partialAnsweringUsers: poll.partialAnsweringUsers,
    workspaces: layouts.workspaces,
    workspaceIds: layouts.workspaceIds,
    sidebarCollapsed: layouts.sidebarCollapsed,
    floatingSession,
    sendToSessionRequest: send.sendToSessionRequest,
    sendToSessionRequestId: send.sendToSessionRequestId,
    assignedSessions: layouts.assignedSessions,
    settings: layouts.settings,
    focusedWindowKey: layouts.focusedWindowKey,
    windowRevealRequest: layouts.windowRevealRequest,
    layoutPresets: layouts.layoutPresets,
    setWindowCount: layouts.setWindowCount,
    clearWorkspaceAssignments: layouts.clearWorkspaceAssignments,
    addSessionToWindow,
    removeSessionFromWindow: layouts.removeSessionFromWindow,
    setActiveSession: layouts.setActiveSession,
    cycleSession: layouts.cycleSession,
    toggleSidebar: layouts.toggleSidebar,
    openFloatingModal,
    closeFloatingModal,
    openSendToSession: send.openSendToSession,
    closeSendToSession: send.closeSendToSession,
    listSessionPanes: send.listSessionPanes,
    sendToSession: send.sendToSession,
    handleSessionClick,
    focusSessionAssignment,
    refreshSessions: poll.refreshSessions,
    createSession,
    restartSession,
    deleteSession,
    renameSession,
    updateSettings: layouts.updateSettings,
    setFocusedWindowKey: layouts.setFocusedWindowKey,
    revealWindow: layouts.revealWindow,
    saveCurrentLayout: layouts.saveCurrentLayout,
    loadPreset: layouts.loadPreset,
    deletePreset: layouts.deletePreset,
    renamePreset: layouts.renamePreset,
  }), [
    poll.sessions, poll.groupedSessions, poll.terminalUsers, poll.loading, poll.error,
    poll.partialAnsweringUsers, poll.refreshSessions, sessionEvidence,
    layouts.workspaces, layouts.workspaceIds, layouts.sidebarCollapsed, layouts.assignedSessions,
    layouts.settings, layouts.focusedWindowKey, layouts.windowRevealRequest, layouts.layoutPresets,
    layouts.setWindowCount, layouts.clearWorkspaceAssignments, layouts.removeSessionFromWindow,
    layouts.setActiveSession, layouts.cycleSession, layouts.toggleSidebar, layouts.updateSettings,
    layouts.setFocusedWindowKey, layouts.revealWindow, layouts.saveCurrentLayout, layouts.loadPreset,
    layouts.deletePreset, layouts.renamePreset, floatingSession, send.sendToSessionRequest,
    send.sendToSessionRequestId, send.openSendToSession,
    send.closeSendToSession, send.listSessionPanes, send.sendToSession,
    addSessionToWindow, restartSession, openFloatingModal, closeFloatingModal, handleSessionClick,
    focusSessionAssignment, createSession, deleteSession, renameSession,
  ])

  return <SessionContext.Provider value={contextValue}>{children}</SessionContext.Provider>
}

export function useSession(): SessionContextValue {
  const context = useContext(SessionContext)
  if (!context) throw new Error('useSession must be used within a SessionProvider')
  return context
}

export function useSessionOptional(): SessionContextValue | null {
  return useContext(SessionContext)
}
