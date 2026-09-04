/**
 * The resident's column: the agent that lives in a tab, at its far right.
 *
 * The Librarian in the Library, the tender in Agents, the Clerk in Beads. Each
 * is a tmux session the host names, shown live in a column 44 terminal columns
 * wide by default under a one-line header that says who it is and how it is.
 * The terminal is the input: Alt+S pastes the tab's reference into the prompt
 * without submitting it, Alt+Enter or a click puts the keyboard there, and the
 * operator types the rest where the agent will read it. When the session is
 * absent the header offers Launch with the resident's folder; when the host
 * configured nothing, the header says so and nothing else.
 *
 * The column owns the terminal it shows rather than taking the pool's: a
 * pooled terminal belongs to the tile that bound it, and a terminal is attached
 * in one place at a time. Peek answers the same problem the same way.
 */

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { CSSProperties, MouseEvent as ReactMouseEvent } from 'react'
import Launcher from './Launcher'
import Menu, { type MenuGroup } from './Menu'
import TerminalSurface, { useTerminalSession } from './TerminalSurface'
import { harnessIdForCommand } from './harnessMarks'
import { SessionCommandMark } from './sessionLabel'
import { useSession } from '../context/SessionContext'
import { useStatus } from '../context/StatusContext'
import { TABLE_CONTENT_MIN, TABLE_WIDTH_MIN, useTableObject } from '../context/TableContext'
import { registerChords } from '../keys/chords'
import { mountResident } from '../residents/residentPresence'
import {
  RESIDENT_LABELS,
  RESIDENT_WIDTH_MIN,
  clampResidentWidth,
  fetchResidents,
  readCachedResidents,
  type Resident,
  type ResidentTab,
} from '../residents/residentsApi'
import { terminalSocketUrl } from '../terminal/ttydProtocol'
import { getSessionKey } from '../types'
import { useResizableWidth } from '../hooks/useResizableWidth'
import './ResidentColumn.css'

export interface ResidentColumnProps {
  tab: ResidentTab
  /**
   * The one line Alt+S pastes into the resident's prompt: the page open in
   * the Library, the workspace and harness chosen in Agents, what is on the
   * table in Beads. Null when the tab has nothing in hand.
   */
  reference: string | null
}

/** What the state word can read, in the order the column decides it. */
export type ResidentState = 'live' | 'idle' | 'not running' | 'not configured'

function collapsedKey(tab: ResidentTab): string {
  return `chrote.resident.${tab}.collapsed`
}

function readCollapsed(tab: ResidentTab): boolean {
  try {
    return window.localStorage.getItem(collapsedKey(tab)) === 'true'
  } catch {
    return false
  }
}

function writeCollapsed(tab: ResidentTab, collapsed: boolean): void {
  try {
    window.localStorage.setItem(collapsedKey(tab), collapsed ? 'true' : 'false')
  } catch {
    // A device that refuses storage still gets a working column.
  }
}

export default function ResidentColumn({ tab, reference }: ResidentColumnProps) {
  const {
    sessions,
    settings,
    updateSettings,
    openSendToSession,
    openFloatingModal,
    sendToSession,
    deleteSession,
    refreshSessions,
  } = useSession()
  const { announce } = useStatus()
  const table = useTableObject()

  const [resident, setResident] = useState<Resident | null>(
    () => readCachedResidents()?.find(entry => entry.tab === tab) ?? null,
  )
  const [known, setKnown] = useState(() => readCachedResidents() !== null)
  const [launching, setLaunching] = useState(false)
  // The harness the launcher is prefilled with: what the session was last seen
  // running, so a restart comes back as the same kind of agent.
  const [launchHarness, setLaunchHarness] = useState<string | undefined>(undefined)
  const [collapsed, setCollapsed] = useState(() => readCollapsed(tab))
  const [squeezed, setSqueezed] = useState(false)
  const [menu, setMenu] = useState<{ x: number; y: number } | null>(null)
  const columnRef = useRef<HTMLElement>(null)

  // One request when the tab opens; nothing polls the host for its residents.
  useEffect(() => {
    let current = true
    fetchResidents()
      .then(found => {
        if (!current) return
        setResident(found.find(entry => entry.tab === tab) ?? null)
        setKnown(true)
      })
      .catch((cause: unknown) => {
        if (!current) return
        setKnown(true)
        announce(cause instanceof Error ? cause.message : 'Could not read the residents', 'error')
      })
    return () => { current = false }
  }, [announce, tab])

  useEffect(() => {
    setCollapsed(readCollapsed(tab))
    setLaunching(false)
  }, [tab])

  const label = resident?.label || RESIDENT_LABELS[tab]
  const sessionName = resident?.session ?? ''
  const session = useMemo(
    () => (sessionName ? sessions.find(candidate => candidate.name === sessionName) ?? null : null),
    [sessionName, sessions],
  )
  const state: ResidentState | null = !known
    ? null
    : sessionName === ''
      ? 'not configured'
      : session === null
        ? 'not running'
        : session.attached ? 'live' : 'idle'
  const running = state === 'live' || state === 'idle'
  const sessionKey = session ? getSessionKey(session.name, session.unixUser) : ''

  useEffect(() => {
    const harness = harnessIdForCommand(session?.currentCommand)
    if (harness && harness !== 'shell') setLaunchHarness(harness)
  }, [session?.currentCommand])

  // Width: the remembered one, or 44 columns at the tile font, and never more
  // than the tab can spare once the content keeps its 480px and the table its
  // minimum.
  const remembered = clampResidentWidth(settings.residentWidth, settings.fontSize)
  const tableOpen = table !== null

  const room = useCallback(() => {
    const parent = columnRef.current?.parentElement
    if (!parent) return Number.POSITIVE_INFINITY
    return parent.clientWidth - TABLE_CONTENT_MIN - (tableOpen ? TABLE_WIDTH_MIN : 0)
  }, [tableOpen])
  const widest = useCallback(() => Math.max(RESIDENT_WIDTH_MIN, room()), [room])
  const commitWidth = useCallback((residentWidth: number) => {
    updateSettings({ residentWidth })
  }, [updateSettings])
  const resize = useResizableWidth({
    elementRef: columnRef,
    width: remembered,
    minWidth: RESIDENT_WIDTH_MIN,
    maxWidth: widest,
    edge: 'left',
    onCommit: commitWidth,
  })
  const width = resize.width

  // The squeeze: when the tab runs out of room the column collapses to its
  // header rather than narrowing the content, and comes back when room does.
  useEffect(() => {
    const parent = columnRef.current?.parentElement
    if (!parent || typeof ResizeObserver === 'undefined') return
    const measure = () => setSqueezed(room() < width)
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(parent)
    return () => observer.disconnect()
  }, [room, width])

  // The table beside this column keeps the content at 480px, and the Send
  // drawer overlays the table rather than this column, only if both know how
  // much this column takes: the workspace carries the figure for them.
  useEffect(() => {
    const column = columnRef.current
    const holder = column?.closest<HTMLElement>('.dashboard-content') ?? column?.parentElement
    if (!column || !holder || typeof ResizeObserver === 'undefined') return
    const tell = () => holder.style.setProperty('--resident-width', `${column.offsetWidth}px`)
    tell()
    const observer = new ResizeObserver(tell)
    observer.observe(column)
    return () => {
      observer.disconnect()
      holder.style.removeProperty('--resident-width')
    }
  }, [])

  // Until the host has answered, the column keeps its full width: a tab that
  // laid itself out and then moved would break the rule that nothing shifts.
  const showsHeaderOnly = collapsed || squeezed || state === 'not configured'
  const showsTerminal = running && !showsHeaderOnly
  const showsLauncher = launching && !showsHeaderOnly && resident !== null && !running

  const socketUrl = useMemo(
    () => (showsTerminal && session ? terminalSocketUrl(session.name, session.unixUser ?? '', 'tile') : null),
    [showsTerminal, session],
  )
  const { session: terminal } = useTerminalSession(socketUrl, settings.fontSize, settings.hideScrollbar)

  const focus = useCallback(() => {
    if (terminal) terminal.focus()
    else columnRef.current?.focus()
  }, [terminal])

  // One line into the prompt, unsubmitted, ending with the line break the
  // drawer would put between a reference and a note. The tab's own rows hand
  // their line here through the presence, so a page sent from a row and the
  // page sent by Alt+S arrive the same way.
  const pasteLine = useCallback(async (line: string): Promise<boolean> => {
    if (!session) return false
    const report = await sendToSession(session.name, { text: `${line}\n`, files: [], submit: false }, session.unixUser)
    if (report.outcome !== 'sent') return false
    terminal?.scrollToBottom()
    focus()
    return true
  }, [focus, sendToSession, session, terminal])

  // Alt+S: the reference goes into the resident's prompt and stays there for
  // the operator to finish. Without a resident to paste into, the drawer
  // serves whichever session the operator picks.
  const paste = useCallback(async () => {
    if (!session) {
      openSendToSession(reference ? { reference } : {})
      return
    }
    if (!reference) {
      focus()
      return
    }
    await pasteLine(reference)
  }, [focus, openSendToSession, pasteLine, reference, session])

  useEffect(() => registerChords([{
    id: `resident.${tab}.send`,
    key: 's',
    direct: { alt: true, shift: false, key: 's' },
    label: `Paste into the ${label}`,
    scope: 'global',
    run: () => { void paste() },
  }]), [label, paste, tab])

  useEffect(() => mountResident({ tab, focus, paste: pasteLine }), [focus, pasteLine, tab])

  const toggleCollapsed = useCallback(() => {
    setCollapsed(open => {
      writeCollapsed(tab, !open)
      return !open
    })
  }, [tab])

  // Restart is the one destructive thing here, and it is exact: the resident's
  // own session, confirmed in the menu row, then the launcher prefilled with
  // the same name, folder and harness so the operator sees what comes back.
  const restart = useCallback(async () => {
    if (!session) return
    const deleted = await deleteSession(session.name, session.unixUser)
    if (!deleted) return
    announce(`Stopped ${session.name}; launch it again from the column`, 'info')
    setLaunching(true)
  }, [announce, deleteSession, session])

  const openMenu = useCallback((event: ReactMouseEvent) => {
    if (state === null || state === 'not configured') return
    event.preventDefault()
    setMenu({ x: event.clientX, y: event.clientY })
  }, [state])

  const menuGroups: MenuGroup[] = [
    {
      id: 'resident',
      rows: [
        {
          id: 'peek',
          label: 'Peek',
          disabled: !running,
          reason: running ? undefined : 'The session is not running',
          onSelect: () => { if (sessionKey) openFloatingModal(sessionKey) },
        },
        {
          id: 'send',
          label: `Send to the ${label}`,
          chord: 'Alt+S',
          disabled: !running,
          reason: running ? undefined : 'The session is not running',
          onSelect: () => { void paste() },
        },
      ],
    },
    {
      id: 'lifecycle',
      rows: [
        {
          id: 'launch',
          label: 'Launch',
          disabled: running,
          reason: running ? 'The session is already running' : undefined,
          onSelect: () => setLaunching(true),
        },
        {
          id: 'restart',
          label: 'Restart',
          danger: true,
          confirmLabel: `Stop ${sessionName} and relaunch`,
          disabled: !running,
          reason: running ? undefined : 'The session is not running',
          onSelect: () => { void restart() },
        },
      ],
    },
  ]

  const style = { '--resident-column-width': `${width}px` } as CSSProperties

  return (
    <aside
      ref={columnRef}
      className={`resident-column${showsHeaderOnly ? ' collapsed' : ''}`}
      role="complementary"
      aria-label={`The ${label}`}
      data-ui="resident.column"
      data-resident-tab={tab}
      tabIndex={-1}
      style={style}
    >
      {!showsHeaderOnly && (
        <div
          {...resize.handleProps}
          className={`resident-column-handle${resize.resizing ? ' dragging' : ''}`}
          role="separator"
          aria-orientation="vertical"
          aria-label={`Resize the ${label}'s column`}
          aria-valuenow={width}
          aria-valuemin={RESIDENT_WIDTH_MIN}
          tabIndex={0}
        />
      )}
      <div className="resident-header" data-ui="resident.header" onContextMenu={openMenu}>
        <span className="resident-label">{label}</span>
        {session && <SessionCommandMark command={session.currentCommand} />}
        {sessionName && state !== 'not configured' && <span className="resident-session">{sessionName}</span>}
        <span className="resident-state">{state ?? ''}</span>
        <span className="resident-header-spacer" />
        {state === 'not running' && !showsHeaderOnly && (
          <button type="button" className="resident-action" onClick={() => setLaunching(open => !open)}>
            {launching ? 'Cancel' : 'Launch'}
          </button>
        )}
        {running && !showsHeaderOnly && (
          <button type="button" className="resident-action" aria-keyshortcuts="Alt+S" onClick={() => { void paste() }}>
            Send<span className="resident-chord" aria-hidden="true">Alt+S</span>
          </button>
        )}
        {state !== null && state !== 'not configured' && (
          <button type="button" className="resident-action" onClick={toggleCollapsed}>
            {collapsed ? 'Expand' : squeezed ? 'Expand' : 'Collapse'}
          </button>
        )}
      </div>
      {showsLauncher && resident && (
        <div className="resident-launcher">
          <Launcher
            workspaceId="terminal1"
            initialFolder={resident.folder || undefined}
            initialHarness={launchHarness}
            initialName={resident.session}
            onLaunched={() => { setLaunching(false); void refreshSessions() }}
          />
        </div>
      )}
      {showsTerminal && (
        <div className="resident-body">
          <TerminalSurface session={terminal} />
        </div>
      )}
      {menu && (
        <Menu
          at={menu}
          label={`${label} actions`}
          groups={menuGroups}
          onClose={() => setMenu(null)}
          estimatedSize={{ width: 240, height: 160 }}
        />
      )}
    </aside>
  )
}
