/**
 * The dashboard's own chords, registered against the actions that exist today.
 *
 * Registration happens once, and once more whenever the number of terminal
 * tabs changes: every `run` reads the current actions through a ref, so a
 * re-render never churns the registry and the keys panel never flickers.
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
  onToggleKeysPanel: () => void
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
  const tabCount = workspaceIds.length
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

    // One chord in each direction beats a chord per window: the operator holds
    // Alt and steps, and the layout can grow past the digits he can reach.
    const cycleWindow = (delta: number) => {
      const { workspaceId, windows } = visibleWindows()
      if (!workspaceId || windows.length === 0) return
      const current = windows.findIndex(
        window => `${workspaceId}-${window.id}` === stateRef.current.session.focusedWindowKey,
      )
      const next = current < 0
        ? (delta > 0 ? 0 : windows.length - 1)
        : (current + delta + windows.length) % windows.length
      stateRef.current.session.setFocusedWindowKey(`${workspaceId}-${windows[next].id}`)
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
      { id: 'keys.beads', key: 'b', direct: { alt: true, shift: false, key: 'b' }, label: 'Beads tab', scope: 'global', run: () => stateRef.current.surfaces.onTabChange('beads') },
      { id: 'keys.library', key: 'l', direct: { alt: true, shift: false, key: 'l' }, label: 'Library tab', scope: 'global', run: () => stateRef.current.surfaces.onTabChange('library') },
      // Alt+K toggles the panel, which is also what the leader toggles. Turning
      // keys off has no chord of its own: the tab bar's own button says so and
      // is how they come back, and the panel's row runs it from here.
      { id: 'keys.agents', key: 'g', direct: { alt: true, shift: false, key: 'g' }, label: 'Agents tab', scope: 'global', run: () => stateRef.current.surfaces.onTabChange('agents') },
      { id: 'keys.panel', key: '?', direct: { alt: true, shift: false, key: 'k' }, label: 'Keybindings', scope: 'global', run: () => stateRef.current.surfaces.onToggleKeysPanel() },
      { id: 'keys.off', key: 'k', label: 'Keys off', scope: 'global', run: () => stateRef.current.session.updateSettings({ keysEnabled: false }) },

      { id: 'keys.nextWindow', key: 'w', direct: { alt: true, shift: false, key: 'w' }, label: 'Next window', scope: 'workspace', run: () => cycleWindow(1) },
      { id: 'keys.prevWindow', key: 'W', direct: { alt: true, shift: true, key: 'w' }, label: 'Previous window', scope: 'workspace', run: () => cycleWindow(-1) },
      // Plus and Minus are read from the character, not from Shift: a Finnish
      // layout types `+` unshifted where a US one types `=` with Shift, and
      // both spellings mean the same key to the operator.
      { id: 'keys.addWindow', key: '=', direct: { alt: true, key: '+', layoutKeys: ['='] }, label: 'Add a window', scope: 'workspace', run: addWindow },
      { id: 'keys.removeWindow', key: '-', direct: { alt: true, key: '-' }, label: 'Remove the last empty window', scope: 'workspace', run: removeWindow },
      { id: 'keys.search', key: '/', label: 'Session search', scope: 'workspace', run: () => { focusSessionSearch() } },
      { id: 'keys.launcher', key: 'n', direct: { alt: true, shift: false, key: 'n' }, label: 'Launcher in the focused window', scope: 'workspace', run: openLauncher },
      { id: 'keys.sessions', key: 'Tab', direct: { alt: true, shift: false, key: 'a' }, label: 'Sessions panel', scope: 'workspace', run: () => stateRef.current.surfaces.onToggleSessionsPanel() },
      { id: 'keys.files', key: 'f', direct: { alt: true, shift: false, key: 'o' }, label: 'Files panel', scope: 'workspace', run: () => clickInActiveDock('button[aria-label="Files sidecar"]') },

      // Peek is a glance, so its chord toggles it: pressed again over the tile
      // Peek shows, it closes; pressed over another tile, Peek switches.
      {
        id: 'keys.peek',
        key: 'p',
        direct: { alt: true, shift: false, key: 'p' },
        label: "Peek the tile's session",
        scope: 'tile',
        run: () => {
          const sessionKey = focusedSession()
          if (!sessionKey) return
          const { floatingSession, openFloatingModal, closeFloatingModal } = stateRef.current.session
          if (floatingSession === sessionKey) closeFloatingModal()
          else openFloatingModal(sessionKey)
        },
      },
      {
        id: 'keys.send',
        key: 's',
        direct: { alt: true, shift: false, key: 's' },
        label: "Send to the tile's session",
        scope: 'tile',
        run: () => {
          const sessionKey = focusedSession()
          if (sessionKey) stateRef.current.session.openSendToSession({ targetSessionKey: sessionKey })
        },
      },
    ]

    return registerChords(chords)
  }, [])

  // The terminal tabs are their own registration because there are as many
  // chords as there are tabs: registering Alt+4 while three tabs exist would
  // put a dead entry in the panel.
  useEffect(() => {
    const chords: Chord[] = Array.from({ length: tabCount }, (_, index): Chord => ({
      id: `keys.tab${index + 1}`,
      key: String(index + 1),
      direct: { alt: true, shift: false, key: String(index + 1) },
      label: `Terminal tab ${index + 1}`,
      scope: 'global',
      run: () => {
        const target = stateRef.current.session.workspaceIds[index]
        if (target) stateRef.current.surfaces.onTabChange(target)
      },
    }))
    return registerChords(chords)
  }, [tabCount])
}
