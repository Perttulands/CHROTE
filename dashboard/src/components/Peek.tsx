/**
 * Peek: a second look at a session, as a floating window centred over the
 * workspace.
 *
 * It is a glance. Its size comes from the session it shows — the pane's own
 * columns and rows at the tile font size, so the peek shows the pane as it
 * is — capped at 70% of the workspace's width and 80% of its height; a
 * session no tile is showing is taken at 100 columns. There is no drag and
 * no resize. Dismissal is the owner's: a press outside closes it and is
 * consumed, Escape closes it from anywhere including its own terminal, and
 * Alt+P toggles it. The header carries the mark, the name, Send and Close
 * as words.
 */

import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useSession } from '../context/SessionContext'
import { getSessionKey, getSessionNameFromKey, getSessionUserFromKey } from '../types'
import TerminalSurface, { useTerminalSession } from './TerminalSurface'
import { useTerminalPool } from './TerminalPool'
import { SessionCommandMark } from './sessionLabel'
import { terminalSocketUrl } from '../terminal/ttydProtocol'
import { isSessionEnded } from '../terminal/tileState'
import { useSessionEvidence } from '../context/useSessionEvidence'
import { useSurface } from '../keys/dismiss'
import { registerChords, type Chord } from '../keys/chords'
import { TERMINAL_FONT_FAMILY } from '../theme/theme'
import './Peek.css'

/** The most of the workspace Peek takes, in each direction. */
export const PEEK_MAX_WIDTH_SHARE = 0.7
export const PEEK_MAX_HEIGHT_SHARE = 0.8
/** The width of a session no tile is showing, in columns. */
export const PEEK_FALLBACK_COLS = 100
/** The header's fixed height, as Peek.css draws it. */
export const PEEK_HEADER_PX = 30
/**
 * Around the grid: the terminal's own padding (2px 4px), the 14px the fit
 * addon always reserves for a scrollbar, and the window's hairline.
 */
const PEEK_CHROME = { width: 8 + 14 + 2, height: 4 + 2 }

export interface PeekGrid {
  cols: number
  /** Null when no tile shows the session, in which case only the cap decides. */
  rows: number | null
  cellWidth: number
  cellHeight: number
}

/**
 * The window's size for a grid, inside the workspace's caps. The grid is
 * rounded up to whole pixels, so the terminal fitted into the window gets
 * exactly the columns and rows asked for and never one fewer.
 */
export function peekSize(grid: PeekGrid, workspace: { width: number; height: number }): { width: number; height: number } {
  const width = Math.min(
    Math.ceil(grid.cols * grid.cellWidth) + PEEK_CHROME.width,
    Math.floor(workspace.width * PEEK_MAX_WIDTH_SHARE),
  )
  const wanted = grid.rows === null
    ? Infinity
    : Math.ceil(grid.rows * grid.cellHeight) + PEEK_CHROME.height + PEEK_HEADER_PX
  const height = Math.min(wanted, Math.floor(workspace.height * PEEK_MAX_HEIGHT_SHARE))
  return { width, height }
}

interface PeekFrame {
  width: number
  height: number
  /** True once the size was taken against the peek's own terminal. */
  settled: boolean
}

// With no terminal on screen to read a cell from, the font is measured
// directly; the height is the usual line box of a monospace face.
function measureCell(fontSize: number): { width: number; height: number } {
  const height = Math.ceil(fontSize * 1.2)
  const context = document.createElement('canvas').getContext('2d')
  if (context) {
    context.font = `${fontSize}px ${TERMINAL_FONT_FAMILY}`
    const width = context.measureText('W').width
    if (width > 0) return { width, height }
  }
  return { width: fontSize * 0.6, height }
}

function Peek() {
  const { floatingSession, closeFloatingModal, openSendToSession, settings, sessions } = useSession()
  const pool = useTerminalPool()
  const peekRef = useRef<HTMLDivElement>(null)
  const [frame, setFrame] = useState<PeekFrame | null>(null)

  const displayName = floatingSession ? getSessionNameFromKey(floatingSession) : ''
  const keyUser = floatingSession ? getSessionUserFromKey(floatingSession) : ''
  const matchingSessions = !floatingSession
    ? []
    : keyUser
      ? sessions.filter(item => getSessionKey(item.name, item.unixUser) === floatingSession)
      : sessions.filter(item => item.name === displayName)
  const session = matchingSessions.length === 1 ? matchingSessions[0] : undefined
  const unixUser = session?.unixUser ?? keyUser
  const canOpenSession = Boolean(floatingSession && (session || unixUser.trim()))

  // The same join the tile makes, asked of the same answer. A glance at a
  // session tmux no longer lists is entitled to the same explanation the tile
  // gives, rather than a dead terminal and no reason for it.
  const evidence = useSessionEvidence()
  const ended = floatingSession !== null && isSessionEnded(floatingSession, evidence)

  // Peek owns its terminal for the life of the window: it is a second
  // observer of the session, not the tile's terminal moved onto the overlay.
  // It attaches as an observer, so it never displaces the tile or resizes the
  // window.
  const socketUrl = useMemo(
    () => (canOpenSession ? terminalSocketUrl(displayName, unixUser, 'peek') : null),
    [canOpenSession, displayName, unixUser],
  )
  // The URL is kept even once the session has ended, so the terminal holding
  // the last frame is not disposed; `connect` is what stops it dialling again.
  const { session: terminal, connectionState } = useTerminalSession(socketUrl, settings.fontSize, settings.hideScrollbar)

  useSurface({ open: floatingSession !== null, kind: 'glance', onClose: closeFloatingModal, ref: peekRef })

  // The grid the window is sized for: the pane's own, as the tile showing the
  // session draws it, or the fallback when no tile shows it.
  const wantedGrid = (): { cols: number; rows: number | null } => {
    const tile = floatingSession ? pool.terminals.get(floatingSession)?.grid() ?? null : null
    return { cols: tile?.cols ?? PEEK_FALLBACK_COLS, rows: tile?.rows ?? null }
  }

  const sizeFor = (cell: { width: number; height: number }): { width: number; height: number } | null => {
    const workspace = peekRef.current?.parentElement
    if (!workspace) return null
    const wanted = wantedGrid()
    return peekSize(
      { cols: wanted.cols, rows: wanted.rows, cellWidth: cell.width, cellHeight: cell.height },
      { width: workspace.clientWidth, height: workspace.clientHeight },
    )
  }

  // A first size before the first paint, from whatever cell is at hand: a
  // tile's, or the font measured directly. It is provisional while the peek's
  // own terminal has yet to measure its cell, because a terminal opened before
  // the terminal font landed keeps the fallback font's cell, and only the
  // peek's own says what its columns will cost.
  useLayoutEffect(() => {
    if (!floatingSession) return
    const provisional = () => {
      const any = pool.terminals.get(floatingSession)?.grid()
        ?? Array.from(pool.terminals.values()).map(entry => entry.grid()).find(grid => grid !== null)
        ?? null
      const cell = any ? { width: any.cellWidth, height: any.cellHeight } : measureCell(settings.fontSize)
      const next = sizeFor(cell)
      if (next) setFrame({ ...next, settled: !canOpenSession })
    }
    provisional()
    // The operator's own resize is the one thing that moves the window after
    // that, and it is measured against the peek's terminal by then.
    const resize = () => {
      const cell = terminal?.grid()
      const next = cell ? sizeFor({ width: cell.cellWidth, height: cell.cellHeight }) : null
      if (next) setFrame({ ...next, settled: true })
    }
    window.addEventListener('resize', resize)
    return () => window.removeEventListener('resize', resize)
    // sizeFor reads refs and the pool; the effect keys on what can change them.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [floatingSession, pool.terminals, settings.fontSize, canOpenSession])

  // Settle against the peek's own terminal once it has opened and measured.
  // The child surface attaches it in its own effect, which runs before this
  // one, so the cell is known here; the window shows only once it is.
  useEffect(() => {
    if (!floatingSession || !terminal || frame === null || frame.settled) return
    const cell = terminal.grid()
    const next = cell ? sizeFor({ width: cell.cellWidth, height: cell.cellHeight }) : null
    setFrame(next ? { ...next, settled: true } : { ...frame, settled: true })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [floatingSession, terminal, frame])

  // While Peek is open, Alt+S sends to the session it shows, and Alt+P with no
  // tile focused closes it; over a focused tile the tile's own chord decides,
  // which is what makes Alt+P a toggle there and a switch elsewhere.
  useEffect(() => {
    if (!floatingSession) return
    const send = () => openSendToSession({ targetSessionKey: floatingSession })
    const chords: Chord[] = [
      ...(['global', 'tile'] as const).map((scope): Chord => ({
        id: `peek.send.${scope}`,
        key: 's',
        direct: { alt: true, shift: false, key: 's' },
        label: `Send to ${displayName}`,
        scope,
        run: send,
      })),
      { id: 'peek.close', key: 'p', direct: { alt: true, shift: false, key: 'p' }, label: 'Close Peek', scope: 'global', run: closeFloatingModal },
    ]
    return registerChords(chords)
  }, [floatingSession, displayName, openSendToSession, closeFloatingModal])

  if (!floatingSession) return null

  return (
    <div
      ref={peekRef}
      className="peek"
      data-ui="peek"
      role="dialog"
      aria-label={`Peek ${displayName}`}
      style={frame ? { width: frame.width, height: frame.height, visibility: frame.settled ? undefined : 'hidden' } : undefined}
    >
      <div className="peek-header">
        <SessionCommandMark command={session?.currentCommand} />
        <span className="peek-name">{displayName}</span>
        {canOpenSession && !ended && connectionState !== 'open' && (
          <span className="terminal-loading-state">
            {connectionState === 'closed' || connectionState === 'dropped' ? 'Terminal disconnected' : 'Loading terminal…'}
          </span>
        )}
        <button type="button" className="peek-word peek-send" onClick={() => openSendToSession({ targetSessionKey: floatingSession })}>
          Send<span className="peek-chord" aria-hidden="true">Alt+S</span>
        </button>
        <button type="button" className="peek-word" onClick={closeFloatingModal}>Close</button>
      </div>
      <div className={ended ? 'peek-body detached' : 'peek-body'}>
        {canOpenSession ? (
          <>
            <TerminalSurface session={terminal} connect={!ended} />
            {ended && (
              <div className="terminal-tile-detached" data-tile-state="ended" role="status">
                <span className="terminal-tile-detached-note">
                  {displayName} ended. This frame shows its last output.
                </span>
              </div>
            )}
          </>
        ) : (
          <div className="empty-window-content">Ambiguous legacy session name; attach the user-qualified session from the session list.</div>
        )}
      </div>
    </div>
  )
}

export default Peek
