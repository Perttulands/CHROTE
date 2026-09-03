/**
 * The dashboard's own chords, registered against the actions that exist today.
 *
 * Registration happens once: every `run` reads the current actions through a
 * ref, so a re-render never churns the registry and the strip never flickers.
 * Two chords reach a control this hook cannot hold — the workspace Files panel
 * and the Sessions panel's launcher both live inside a dock — so they are run
 * by clicking the dock's own button, the way `/` has always reached the session
 * search. Send opens today's modal; the wave-1 drawer lane retargets it here.
 */

import { useEffect, useRef } from 'react'
import { useSession } from '../context/SessionContext'
import type { Tab } from '../components/TabBar'
import { isTerminalWorkspaceId } from '../types'
import type { WorkspaceId } from '../types'
import { registerChords, setActiveScopes, setKeysEnabled, type Chord } from './chords'
import { focusSessionSearch } from './focusSessionSearch'

const MAX_WINDOWS = 4

export interface AppChordSurfaces {
  /** The tab the operator is looking at, terminal workspace or otherwise. */
  activeTab: Tab
  onTabChange: (tab: Tab) => void
  onToggleSessionsPanel: () => void
  onOpenSessionsPanel: () => void
  onOpenKeysPanel: () => void
}

function clickInActiveDock(selector: string) {
  document.querySelector<HTMLElement>(`.terminal-workspace-dock[data-active="true"] ${selector}`)?.click()
}

export function useAppChords(surfaces: AppChordSurfaces): void {
  const session = useSession()
  const state = { surfaces, session }
  const stateRef = useRef(state)
  stateRef.current = state

  const { workspaceIds, focusedWindowKey, settings } = session
  const activeWorkspaceId = isTerminalWorkspaceId(surfaces.activeTab, workspaceIds) ? surfaces.activeTab : null

  useEffect(() => {
    setActiveScopes({
      workspace: activeWorkspaceId !== null,
      tile: activeWorkspaceId !== null && (focusedWindowKey?.startsWith(`${activeWorkspaceId}-`) ?? false),
    })
  }, [activeWorkspaceId, focusedWindowKey])

  useEffect(() => {
    setKeysEnabled(settings.keysEnabled)
  }, [settings.keysEnabled])

  useEffect(() => {
    // The active terminal workspace, or null when the operator is on another
    // tab — in which case no workspace or tile chord is reachable anyway.
    const workspaceNow = (): WorkspaceId | null => {
      const { surfaces: now, session: live } = stateRef.current
      return isTerminalWorkspaceId(now.activeTab, live.workspaceIds) ? now.activeTab : null
    }

    const visibleWindows = () => {
      const workspaceId = workspaceNow()
      const workspace = workspaceId ? stateRef.current.session.workspaces[workspaceId] : null
      if (!workspaceId || !workspace) return { workspaceId: null, windows: [], count: 0 } as const
      return {
        workspaceId,
        windows: workspace.windows.slice(0, workspace.windowCount),
        count: workspace.windowCount,
      } as const
    }

    const focusedWindow = () => {
      const { workspaceId, windows } = visibleWindows()
      if (!workspaceId) return null
      const key = stateRef.current.session.focusedWindowKey
      return windows.find(window => `${workspaceId}-${window.id}` === key) ?? null
    }

    const focusWindow = (index: number) => {
      const { workspaceId, windows } = visibleWindows()
      const target = windows[index]
      if (!workspaceId || !target) return
      stateRef.current.session.setFocusedWindowKey(`${workspaceId}-${target.id}`)
    }

    const stepTab = (delta: number) => {
      const { surfaces: now, session: live } = stateRef.current
      const workspaceId = workspaceNow()
      if (!workspaceId) return
      const index = live.workspaceIds.indexOf(workspaceId)
      if (index < 0) return
      const length = live.workspaceIds.length
      now.onTabChange(live.workspaceIds[(index + delta + length) % length])
    }

    const addWindow = () => {
      const { workspaceId, count } = visibleWindows()
      if (!workspaceId || count >= MAX_WINDOWS) return
      stateRef.current.session.setWindowCount(workspaceId, count + 1)
    }

    // A window holding a session is somebody's live terminal; the layout does
    // not shrink over it.
    const removeWindow = () => {
      const { workspaceId, windows, count } = visibleWindows()
      const last = windows[count - 1]
      if (!workspaceId || count <= 1 || !last || last.boundSessions.length > 0) return
      stateRef.current.session.setWindowCount(workspaceId, count - 1)
    }

    // An empty window is the launcher, so the chord puts the operator in one.
    // With none empty, the Sessions panel's plus opens the same launcher.
    const openLauncher = () => {
      const { workspaceId, windows } = visibleWindows()
      if (!workspaceId) return
      const focused = focusedWindow()
      const target = focused && focused.boundSessions.length === 0
        ? focused
        : windows.find(window => window.boundSessions.length === 0)
      if (target) {
        stateRef.current.session.setFocusedWindowKey(`${workspaceId}-${target.id}`)
        requestAnimationFrame(() => {
          document
            .querySelector<HTMLElement>('.terminal-workspace-dock[data-active="true"] .terminal-window.focused .launcher button, .terminal-workspace-dock[data-active="true"] .terminal-window.focused .launcher input')
            ?.focus()
        })
        return
      }
      stateRef.current.surfaces.onOpenSessionsPanel()
      requestAnimationFrame(() => clickInActiveDock('.session-panel .add-btn'))
    }

    const focusedSession = () => focusedWindow()?.activeSession ?? null

    const chords: Chord[] = [
      { id: 'keys.beads', key: 'b', label: 'Beads tab', scope: 'global', run: () => stateRef.current.surfaces.onTabChange('beads') },
      { id: 'keys.panel', key: '?', label: 'Keys panel', scope: 'global', run: () => stateRef.current.surfaces.onOpenKeysPanel() },
      { id: 'keys.off', key: 'k', label: 'Keys off', scope: 'global', run: () => stateRef.current.session.updateSettings({ keysEnabled: false }) },
      // The window is already shut by the time a chord runs, which is the whole
      // action; listing it is how the strip says so.
      { id: 'keys.cancel', key: 'Escape', label: 'Cancel', scope: 'global', run: () => {} },

      ...[1, 2, 3, 4].map((n): Chord => ({
        id: `keys.window${n}`,
        key: String(n),
        label: `Focus window ${n}`,
        scope: 'workspace',
        run: () => focusWindow(n - 1),
      })),
      { id: 'keys.prevTab', key: '[', label: 'Previous terminal tab', scope: 'workspace', run: () => stepTab(-1) },
      { id: 'keys.nextTab', key: ']', label: 'Next terminal tab', scope: 'workspace', run: () => stepTab(1) },
      { id: 'keys.addWindow', key: '=', label: 'Add a window', scope: 'workspace', run: addWindow },
      { id: 'keys.removeWindow', key: '-', label: 'Remove the last empty window', scope: 'workspace', run: removeWindow },
      { id: 'keys.search', key: '/', label: 'Session search', scope: 'workspace', run: () => { focusSessionSearch() } },
      { id: 'keys.launcher', key: 'n', label: 'Launcher', scope: 'workspace', run: openLauncher },
      { id: 'keys.sessions', key: 'Tab', label: 'Sessions panel', scope: 'workspace', run: () => stateRef.current.surfaces.onToggleSessionsPanel() },
      { id: 'keys.files', key: 'f', label: 'Files panel', scope: 'workspace', run: () => clickInActiveDock('button[aria-label="Files sidecar"]') },

      {
        id: 'keys.peek',
        key: 'p',
        label: 'Peek this session',
        scope: 'tile',
        run: () => {
          const sessionKey = focusedSession()
          if (sessionKey) stateRef.current.session.openFloatingModal(sessionKey)
        },
      },
      {
        id: 'keys.send',
        key: 's',
        label: 'Send to this session',
        scope: 'tile',
        run: () => {
          const sessionKey = focusedSession()
          if (sessionKey) stateRef.current.session.openSendToSession(sessionKey)
        },
      },
    ]

    return registerChords(chords)
  }, [])
}
