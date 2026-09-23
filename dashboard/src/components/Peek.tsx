/**
 * Peek: a second look at a session, as a floating window centred over the
 * workspace, good enough to drive the session from.
 *
 * Peek shows the tmux window whole, at the window's own size. Its terminal
 * holds the window's grid — the window's columns, and its rows plus the
 * status lines under them, as the session inventory reports them — and fits
 * its font to the box instead of its grid: the largest font, no larger than
 * the operator's, at which every row and column fits. A pane another client
 * sized larger than the box is therefore shown entire, the prompt, a menu at
 * the bottom and the status line included, rather than the part of it around
 * the cursor.
 *
 * Holding the window's grid is also what keeps Peek from resizing anything.
 * A peek attaches with `-f ignore-size` (src/internal/proxy/terminal.go), so
 * while another client sizes the window the grid it sends changes nothing;
 * and when it is the only client, tmux sizes the window by it, so it sends
 * the size the window already is. Dragging the window changes only the font.
 * A session the inventory has no size for is fitted the way a tile is, grid
 * to box, which is the one case where a sole-client peek can still reflow.
 *
 * Its default size is that grid at the operator's font plus the header,
 * capped at 90% of the workspace in each direction, past which the font
 * shrinks. The operator resizes it from any edge or corner through the shared
 * floating-window frame, and that size is remembered for every later peek
 * until Reset size gives the session the say again; slack inside a remembered
 * size is terminal background.
 *
 * Opening Peek focuses its terminal, and Escape typed there is the session's,
 * so a menu can be cancelled and an agent interrupted from here. Dismissal is
 * otherwise the owner's: a press outside closes it and is consumed, Escape
 * with focus anywhere else closes it, and Alt+P closes it from inside its own
 * terminal. The header carries the mark, the name, Send and Close as words.
 */

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useSession } from '../context/SessionContext'
import { getSessionKey, getSessionNameFromKey, getSessionUserFromKey } from '../types'
import TerminalSurface, { useTerminalSession } from './TerminalSurface'
import FloatingFrameHandles from './FloatingFrameHandles'
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
export const PEEK_MAX_WIDTH_SHARE = 0.9
export const PEEK_MAX_HEIGHT_SHARE = 0.9
/** The width of a session the inventory has no size for, in columns. */
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
  /** Null when the inventory has no size for the session; only the cap decides. */
  rows: number | null
  cellWidth: number
  cellHeight: number
}

/** The window's size for a grid, inside the workspace's caps. */
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

// The window's default size is asked before its terminal has measured
// anything, so the cell is the font measured directly; the height is the usual
// line box of a monospace face. It only has to be close: the terminal fits its
// font to whatever box results, so an estimate a little off costs a slightly
// smaller font or a little background, never a row.
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
  const peekRef = useRef<HTMLDivElement>(null)

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

  // The window's grid as the inventory last reported it: a client showing the
  // whole window is its height plus the status lines under it. It follows the
  // session list's own refresh; nothing here asks tmux again.
  const cols = session?.width
  const rows = session?.height ? session.height + (session.statusLines ?? 0) : undefined
  const windowGrid = useMemo(
    () => (cols && rows ? { cols, rows } : null),
    [cols, rows],
  )

  // Peek owns its terminal for the life of the window: it is a second
  // observer of the session, not the tile's terminal moved onto the overlay.
  // It attaches as an observer, so it never displaces the tile.
  const socketUrl = useMemo(
    () => (canOpenSession ? terminalSocketUrl(displayName, unixUser, 'peek') : null),
    [canOpenSession, displayName, unixUser],
  )
  // The URL is kept even once the session has ended, so the terminal holding
  // the last frame is not disposed; `connect` is what stops it dialling again.
  const { session: terminal, connectionState } = useTerminalSession(
    socketUrl, settings.fontSize, settings.hideScrollbar, windowGrid,
  )

  // A peek is opened to be used: the keys go to the session it shows.
  useEffect(() => { terminal?.focus() }, [terminal])

  useSurface({ open: floatingSession !== null, kind: 'glance', onClose: closeFloatingModal, ref: peekRef })

  // The session asks for its own size; a size the operator dragged overrides
  // it, for this session and every later one.
  const cell = useMemo(() => measureCell(settings.fontSize), [settings.fontSize])
  const contentSize = useCallback(
    (workspace: FrameSize) => peekSize({
      cols: windowGrid?.cols ?? PEEK_FALLBACK_COLS,
      rows: windowGrid?.rows ?? null,
      cellWidth: cell.width,
      cellHeight: cell.height,
    }, workspace),
    [windowGrid, cell],
  )
  const frame = useFloatingFrame({
    kind: 'peek',
    elementRef: peekRef,
    open: floatingSession !== null,
    label: 'session',
    contentSize,
  })

  // Whether the focus is inside the window, which is what decides Alt+P below.
  const [holdsFocus, setHoldsFocus] = useState(false)

  // While Peek is open, Alt+S sends to the session it shows, and Alt+P with no
  // tile focused closes it; over a focused tile the tile's own chord decides,
  // which is what makes Alt+P a toggle there and a switch elsewhere. From
  // inside Peek's own terminal Alt+P closes it whatever tile is focused: the
  // operator is in Peek, so that is what the chord is about. Registered after
  // the tile's, it wins the tile scope while the focus is here.
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
      ...(holdsFocus ? ['global', 'tile'] as const : ['global'] as const).map((scope): Chord => ({
        id: `peek.close.${scope}`,
        key: 'p',
        direct: { alt: true, shift: false, key: 'p' },
        label: 'Close Peek',
        scope,
        run: closeFloatingModal,
      })),
    ]
    return registerChords(chords)
  }, [floatingSession, displayName, holdsFocus, openSendToSession, closeFloatingModal])

  if (!floatingSession) return null

  return (
    <div
      ref={peekRef}
      className="peek"
      data-ui="peek"
      role="dialog"
      aria-label={`Peek ${displayName}`}
      style={frame.size ? { width: frame.size.width, height: frame.size.height } : undefined}
      onFocus={() => setHoldsFocus(true)}
      onBlur={event => {
        if (!(event.relatedTarget instanceof Node && event.currentTarget.contains(event.relatedTarget))) setHoldsFocus(false)
      }}
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
