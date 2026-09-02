import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { TerminalWorkspace, UserSettings, WindowRevealRequest, WorkspaceId } from '../types'
import { DEFAULT_SETTINGS, MAX_PRESETS, normalizeTerminalTabCount } from '../types'
import { useToast } from './ToastContext'
import {
  CANONICAL_WINDOW_COUNT,
  clampWindowCount,
  cloneWorkspaces,
  deduplicateWorkspaceBindings,
  defaultWorkspacesFor,
  generatePresetId,
  getCurrentViewportBucket,
  idsInWorkspaces,
  loadStoredPresets,
  loadStoredState,
  sanitizeWorkspaces,
  savePresets,
  saveState,
  visibleWorkspaceIds,
  type ViewportBucket,
} from './workspaceLayouts'

async function applyTmuxMouse(enabled: boolean): Promise<void> {
  try {
    await fetch('/api/tmux/mouse', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ enabled }),
      signal: AbortSignal.timeout(10000),
    })
  } catch (e) {
    console.warn('Failed to apply tmux mouse mode:', e)
  }
}

export function useWorkspaceLayouts() {
  const { addToast } = useToast()
  const [viewportBucket, setViewportBucket] = useState<ViewportBucket>(() => getCurrentViewportBucket())
  const stored = useMemo(() => loadStoredState(viewportBucket), [])
  const [workspaces, setWorkspaces] = useState<Record<WorkspaceId, TerminalWorkspace>>(
    stored?.workspaces ?? defaultWorkspacesFor(visibleWorkspaceIds(stored?.settings ?? DEFAULT_SETTINGS)),
  )
  const [sidebarCollapsed, setSidebarCollapsed] = useState(stored?.sidebarCollapsed ?? false)
  const [settings, setSettings] = useState<UserSettings>(stored?.settings ?? DEFAULT_SETTINGS)
  const [focusedWindowKey, setFocusedWindowKey] = useState<string | null>(null)
  const [windowRevealRequest, setWindowRevealRequest] = useState<WindowRevealRequest | null>(null)
  const [layoutPresets, setLayoutPresets] = useState(() => loadStoredPresets())
  const viewportBucketRef = useRef(viewportBucket)
  const workspacesRef = useRef(workspaces)
  const windowRevealRequestIdRef = useRef(0)
  viewportBucketRef.current = viewportBucket
  workspacesRef.current = workspaces

  useEffect(() => {
    const handleViewportChange = () => {
      const next = getCurrentViewportBucket()
      if (next === viewportBucketRef.current) return
      const storedNext = loadStoredState(next)
      const nextLayout = storedNext?.layoutsByViewport[next]
      setViewportBucket(next)
      if (nextLayout) setWorkspaces(nextLayout.workspaces)
    }
    window.addEventListener('resize', handleViewportChange)
    window.addEventListener('orientationchange', handleViewportChange)
    return () => {
      window.removeEventListener('resize', handleViewportChange)
      window.removeEventListener('orientationchange', handleViewportChange)
    }
  }, [])

  const workspaceIds = useMemo(() => visibleWorkspaceIds(settings), [settings])

  useEffect(() => {
    setWorkspaces(previous => {
      const missing = workspaceIds.filter(workspaceId => !previous[workspaceId])
      return missing.length === 0 ? previous : { ...previous, ...defaultWorkspacesFor(missing) }
    })
  }, [workspaceIds])

  const assignedSessions = useMemo(() => {
    const assigned = new Map<string, { workspaceId: WorkspaceId; windowId: string; colorIndex: number; windowIndex: number }>()
    idsInWorkspaces(workspaces).forEach(workspaceId => {
      workspaces[workspaceId].windows.forEach((window, index) => {
        window.boundSessions.forEach(session => {
          assigned.set(session, {
            workspaceId,
            windowId: window.id,
            colorIndex: window.colorIndex,
            windowIndex: index + 1,
          })
        })
      })
    })
    return assigned
  }, [workspaces])

  useEffect(() => {
    saveState({ workspaces, sidebarCollapsed, settings }, viewportBucket)
  }, [workspaces, sidebarCollapsed, settings, viewportBucket])

  useEffect(() => savePresets(layoutPresets), [layoutPresets])

  const setWindowCount = useCallback((workspaceId: WorkspaceId, count: number) => {
    const newCount = clampWindowCount(count)
    setWorkspaces(previous => {
      const workspace = previous[workspaceId]
      if (!workspace || workspace.windowCount === newCount) return previous
      return { ...previous, [workspaceId]: { ...workspace, windowCount: newCount } }
    })
  }, [])

  const revealWindow = useCallback((workspaceId: WorkspaceId, windowId: string) => {
    if (!workspacesRef.current[workspaceId]) return
    const windowIndex = Array.from(
      { length: CANONICAL_WINDOW_COUNT },
      (_, index) => `${workspaceId}-window-${index}`,
    ).indexOf(windowId)
    if (windowIndex < 0) return
    setWorkspaces(previous => {
      const workspace = previous[workspaceId]
      if (!workspace) return previous
      const visibleCount = Math.max(workspace.windowCount, windowIndex + 1)
      return visibleCount === workspace.windowCount
        ? previous
        : { ...previous, [workspaceId]: { ...workspace, windowCount: visibleCount } }
    })
    windowRevealRequestIdRef.current += 1
    setWindowRevealRequest({ workspaceId, windowId, requestId: windowRevealRequestIdRef.current })
  }, [])

  const clearWorkspaceAssignments = useCallback((workspaceId: WorkspaceId) => {
    setWorkspaces(previous => {
      const workspace = previous[workspaceId]
      if (!workspace) return previous
      return {
        ...previous,
        [workspaceId]: {
          ...workspace,
          windows: workspace.windows.map(window => ({ ...window, boundSessions: [], activeSession: null })),
        },
      }
    })
  }, [])

  const removeSessionFromWindow = useCallback((workspaceId: WorkspaceId, windowId: string, sessionName: string) => {
    setWorkspaces(previous => {
      const workspace = previous[workspaceId]
      if (!workspace) return previous
      return {
        ...previous,
        [workspaceId]: {
          ...workspace,
          windows: workspace.windows.map(window => {
            if (window.id !== windowId || !window.boundSessions.includes(sessionName)) return window
            const boundSessions = window.boundSessions.filter(session => session !== sessionName)
            return {
              ...window,
              boundSessions,
              activeSession: window.activeSession === sessionName ? (boundSessions[0] ?? null) : window.activeSession,
            }
          }),
        },
      }
    })
  }, [])

  const setActiveSession = useCallback((workspaceId: WorkspaceId, windowId: string, sessionName: string) => {
    setWorkspaces(previous => {
      const workspace = previous[workspaceId]
      if (!workspace) return previous
      return {
        ...previous,
        [workspaceId]: {
          ...workspace,
          windows: workspace.windows.map(window =>
            window.id === windowId && window.boundSessions.includes(sessionName)
              ? { ...window, activeSession: sessionName }
              : window,
          ),
        },
      }
    })
  }, [])

  const cycleSession = useCallback((workspaceId: WorkspaceId, windowId: string, direction: 'prev' | 'next') => {
    setWorkspaces(previous => {
      const workspace = previous[workspaceId]
      if (!workspace) return previous
      return {
        ...previous,
        [workspaceId]: {
          ...workspace,
          windows: workspace.windows.map(window => {
            if (window.id !== windowId || window.boundSessions.length <= 1) return window
            const currentIndex = window.activeSession ? window.boundSessions.indexOf(window.activeSession) : 0
            const nextIndex = direction === 'next'
              ? (currentIndex + 1) % window.boundSessions.length
              : (currentIndex - 1 + window.boundSessions.length) % window.boundSessions.length
            return { ...window, activeSession: window.boundSessions[nextIndex] }
          }),
        },
      }
    })
  }, [])

  const toggleSidebar = useCallback(() => setSidebarCollapsed(previous => !previous), [])

  const updateSettings = useCallback((newSettings: Partial<UserSettings>) => {
    setSettings(previous => {
      const updated = { ...previous, ...newSettings }
      if (newSettings.terminalTabCount !== undefined) {
        updated.terminalTabCount = normalizeTerminalTabCount(newSettings.terminalTabCount)
      }
      if (newSettings.mouseScroll !== undefined) applyTmuxMouse(updated.mouseScroll)
      return updated
    })
  }, [])

  const saveCurrentLayout = useCallback((name: string): boolean => {
    if (layoutPresets.length >= MAX_PRESETS) {
      addToast(`Maximum ${MAX_PRESETS} presets reached`, 'warning')
      return false
    }
    setLayoutPresets(previous => [...previous, {
      id: generatePresetId(),
      name,
      createdAt: Date.now(),
      workspaces: sanitizeWorkspaces(workspaces, []),
    }])
    addToast(`Layout '${name}' saved`, 'success')
    return true
  }, [layoutPresets.length, workspaces, addToast])

  const loadPreset = useCallback((presetId: string) => {
    const preset = layoutPresets.find(candidate => candidate.id === presetId)
    if (!preset) {
      addToast('Preset not found', 'error')
      return
    }
    setWorkspaces(previous => deduplicateWorkspaceBindings({
      ...previous,
      ...sanitizeWorkspaces(cloneWorkspaces(preset.workspaces), []),
    }))
    addToast(`Layout '${preset.name}' loaded`, 'info')
  }, [layoutPresets, addToast])

  const deletePreset = useCallback((presetId: string) => {
    const preset = layoutPresets.find(candidate => candidate.id === presetId)
    if (preset) {
      setLayoutPresets(previous => previous.filter(candidate => candidate.id !== presetId))
      addToast(`Preset '${preset.name}' deleted`, 'info')
    }
  }, [layoutPresets, addToast])

  const renamePreset = useCallback((presetId: string, newName: string) => {
    setLayoutPresets(previous => previous.map(preset => preset.id === presetId ? { ...preset, name: newName } : preset))
  }, [])

  return {
    workspaces,
    setWorkspaces,
    workspacesRef,
    workspaceIds,
    sidebarCollapsed,
    settings,
    assignedSessions,
    focusedWindowKey,
    setFocusedWindowKey,
    windowRevealRequest,
    layoutPresets,
    setWindowCount,
    revealWindow,
    clearWorkspaceAssignments,
    removeSessionFromWindow,
    setActiveSession,
    cycleSession,
    toggleSidebar,
    updateSettings,
    saveCurrentLayout,
    loadPreset,
    deletePreset,
    renamePreset,
  }
}
