import { createContext, useCallback, useContext, useMemo, useState } from 'react'
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
import { useToast } from './ToastContext'
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

const SessionContext = createContext<DashboardContextType | null>(null)

export function SessionProvider({ children }: { children: ReactNode }) {
  const { addToast } = useToast()
  const layouts = useWorkspaceLayouts()
  const send = useSendToSession()
  const [floatingSession, setFloatingSession] = useState<string | null>(null)
  const poll = useSessionsPoll({ autoRefreshInterval: layouts.settings.autoRefreshInterval })

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
        body: JSON.stringify({ name: sessionName, unixUser, mouseScroll: options.mouseScroll ?? layouts.settings.mouseScroll }),
        signal: AbortSignal.timeout(10000),
      })
      if (!response.ok) {
        addToast('Failed to create session', 'error')
        return null
      }
      addToast(`Session '${sessionName}' created`, 'success')
      if (options.attachTo) {
        addSessionToWindow(options.attachTo.workspaceId, options.attachTo.windowId, sessionName, unixUser)
      }
      void poll.refreshSessions()
      return sessionName
    } catch (e) {
      console.error('Failed to create session:', e)
      addToast('Failed to create session', 'error')
      return null
    }
  }, [addSessionToWindow, addToast, layouts.settings, poll.refreshSessions, poll.sessions, poll.terminalUsers])

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
        addToast('Failed to delete session', 'error')
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
      addToast(`Session '${sessionName}' deleted`, 'info')
      poll.refreshSessions()
      return true
    } catch (e) {
      console.error('Failed to delete session:', e)
      addToast('Failed to delete session', 'error')
      return false
    }
  }, [addToast, layouts.setWorkspaces, poll.refreshSessions])

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
        addToast('Failed to rename session', 'error')
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
      addToast(`Session renamed to '${newName}'`, 'success')
      poll.refreshSessions()
      return true
    } catch (e) {
      console.error('Failed to rename session:', e)
      addToast('Failed to rename session', 'error')
      return false
    }
  }, [addToast, layouts.setWorkspaces, layouts.workspaces, poll.refreshSessions])

  const contextValue: DashboardContextType = useMemo(() => ({
    sessions: poll.sessions,
    groupedSessions: poll.groupedSessions,
    terminalUsers: poll.terminalUsers,
    loading: poll.loading,
    error: poll.error,
    workspaces: layouts.workspaces,
    workspaceIds: layouts.workspaceIds,
    sidebarCollapsed: layouts.sidebarCollapsed,
    floatingSession,
    sendToSessionTarget: send.sendToSessionTarget,
    sendToSessionPrefill: send.sendToSessionPrefill,
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
    poll.sessions, poll.groupedSessions, poll.terminalUsers, poll.loading, poll.error, poll.refreshSessions,
    layouts.workspaces, layouts.workspaceIds, layouts.sidebarCollapsed, layouts.assignedSessions,
    layouts.settings, layouts.focusedWindowKey, layouts.windowRevealRequest, layouts.layoutPresets,
    layouts.setWindowCount, layouts.clearWorkspaceAssignments, layouts.removeSessionFromWindow,
    layouts.setActiveSession, layouts.cycleSession, layouts.toggleSidebar, layouts.updateSettings,
    layouts.setFocusedWindowKey, layouts.revealWindow, layouts.saveCurrentLayout, layouts.loadPreset,
    layouts.deletePreset, layouts.renamePreset, floatingSession, send.sendToSessionTarget,
    send.sendToSessionPrefill, send.sendToSessionRequestId, send.openSendToSession,
    send.closeSendToSession, send.listSessionPanes, send.sendToSession,
    addSessionToWindow, restartSession, openFloatingModal, closeFloatingModal, handleSessionClick,
    focusSessionAssignment, createSession, deleteSession, renameSession,
  ])

  return <SessionContext.Provider value={contextValue}>{children}</SessionContext.Provider>
}

export function useSession(): DashboardContextType {
  const context = useContext(SessionContext)
  if (!context) throw new Error('useSession must be used within a SessionProvider')
  return context
}

export function useSessionOptional(): DashboardContextType | null {
  return useContext(SessionContext)
}
