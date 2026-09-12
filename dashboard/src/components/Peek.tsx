/**
 * Peek: a second look at a session, as a floating window centred over the
 * workspace.
 *
 * It is a glance. Its default size comes from the session it shows — the
 * pane's own columns and rows at the tile font size, so the peek shows the
 * pane as it is — capped at 70% of the workspace's width and 80% of its
 * height; a session no tile is showing is taken at 100 columns. The operator
 * resizes it from any edge or corner through the shared floating-window
 * frame, and that size is remembered for every later peek until Reset size
 * gives the session the say again.
 *
 * The terminal fills whatever size the window has, the way a tile's does,
 * rather than showing a pane of fixed size inside a scrolling window: this is
 * the same TerminalSurface, which fits its grid to its box and sends the
 * columns and rows it arrived at down its own connection. Whether the tmux
 * pane reflows to them is tmux's answer and not Peek's, and that is what
 * makes filling safe. A peek attaches with `-f ignore-size`
 * (src/internal/proxy/terminal.go), so while a tile holds the sizing seat the
 * pane keeps the tile's size and a resized peek only shows more or less room
 * around it — a resized peek cannot fight the tile showing the same session.
 * When nothing else sizes the window, the peek is the client tmux sizes it
 * by, and the pane reflows to the window the operator drew. That is the rule
 * a tile already obeys.
 *
 * Dismissal is the owner's: a press outside closes it and is consumed, Escape
 * closes it from anywhere including its own terminal, and Alt+P toggles it.
 * The header carries the mark, the name, Send and Close as words.
 */

import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useSession } from '../context/SessionContext'
import { getSessionKey, getSessionNameFromKey, getSessionUserFromKey } from '../types'
import TerminalSurface, { useTerminalSession } from './TerminalSurface'
import FloatingFrameHandles from './FloatingFrameHandles'
import { useTerminalPool } from './TerminalPool'
import { SessionCommandMark } from './sessionLabel'
import { terminalSocketUrl } from '../terminal/ttydProtocol'
import { isSessionEnded } from '../terminal/tileState'
import { useSessionEvidence } from '../context/useSessionEvidence'
import { useSurface } from '../keys/dismiss'
import { registerChords, type Chord } from '../keys/chords'
import { useFloatingFrame } from '../hooks/useFloatingFrame'
import type { FrameSize } from '../hooks/floatingWindowSize'
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

/**
 * What the session asks the window to be: the pane's grid, at the cell the
 * peek's own terminal draws it with.
 */
interface PeekContent extends PeekGrid {
  /** True once the cell was taken from the peek's own terminal. */
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
  const [content, setContent] = useState<PeekContent | null>(null)

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

  // The grid the window is sized for, taken before the first paint: the
  // pane's own as the tile showing the session draws it, at whatever cell is
  // at hand — a tile's, or the font measured directly. The cell is provisional
  // while the peek's own terminal has yet to measure one, because a terminal
  // opened before the terminal font landed keeps the fallback font's cell, and
  // only the peek's own says what its columns will cost. The workspace is the
  // frame's to measure, and it measures it again on a window resize.
  useLayoutEffect(() => {
    if (!floatingSession) {
      setContent(null)
      return
    }
    const tile = pool.terminals.get(floatingSession)?.grid() ?? null
    const any = tile
      ?? Array.from(pool.terminals.values()).map(entry => entry.grid()).find(grid => grid !== null)
      ?? null
    const cell = any ? { width: any.cellWidth, height: any.cellHeight } : measureCell(settings.fontSize)
    setContent({
      cols: tile?.cols ?? PEEK_FALLBACK_COLS,
      rows: tile?.rows ?? null,
      cellWidth: cell.width,
      cellHeight: cell.height,
      settled: !canOpenSession,
    })
  }, [floatingSession, pool.terminals, settings.fontSize, canOpenSession])

  // Settle against the peek's own terminal once it has opened and measured.
  // The child surface attaches it in its own effect, which runs before this
  // one, so the cell is known here; the window shows only once it is.
  useEffect(() => {
    if (!floatingSession || !terminal || content === null || content.settled) return
    const cell = terminal.grid()
    setContent(cell
      ? { ...content, cellWidth: cell.cellWidth, cellHeight: cell.cellHeight, settled: true }
      : { ...content, settled: true })
  }, [floatingSession, terminal, content])

  // The session asks for its own size; a size the operator dragged overrides
  // it, for this session and every later one.
  const contentSize = useCallback(
    (workspace: FrameSize) => (content ? peekSize(content, workspace) : null),
    [content],
  )
  const frame = useFloatingFrame({
    kind: 'peek',
    elementRef: peekRef,
    open: floatingSession !== null,
    label: 'session',
    contentSize,
  })

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

  // A remembered size is right the moment it is read; a size derived from the
  // session is not shown until the peek's own terminal has said what a cell
  // costs, so the window is never drawn at one size and corrected to another.
  const shown = frame.remembered || content?.settled === true

  return (
    <div
      ref={peekRef}
      className="peek"
      data-ui="peek"
      role="dialog"
      aria-label={`Peek ${displayName}`}
      style={frame.size
        ? { width: frame.size.width, height: frame.size.height, visibility: shown ? undefined : 'hidden' }
        : undefined}
    >
      <FloatingFrameHandles frame={frame} />
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
        {frame.remembered && (
          <button type="button" className="peek-word" onClick={frame.resetSize}>Reset size</button>
        )}
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
