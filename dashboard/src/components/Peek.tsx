/**
 * Peek: a second look at a session, as a floating window centred over the
 * workspace, good enough to drive the session from.
 *
 * Peek shows the tmux window at the window's own size. Its terminal holds
 * the window's grid — the window's columns, and its rows plus the status
 * lines under them, as the session inventory reports them — and fits its font
 * to the room Peek gives it instead of its grid: the largest font, no larger
 * than the operator's, at which every row and column fits. A pane another
 * client sized larger than the room is therefore shown entire rather than the
 * part of it around the cursor, down to a floor of 11px or the operator's own
 * font. Below that the grid is drawn at the floor, anchored to the bottom
 * left, so the prompt, a menu at the bottom and the status line stay in view
 * and the oldest rows are the ones cut off. There is no scrolling to them:
 * the wheel is tmux's. Dragging the window larger shows more.
 *
 * Holding the window's grid is also what keeps Peek from resizing anything.
 * A peek attaches with `-f ignore-size` (src/internal/proxy/terminal.go), so
 * while another client sizes the window the grid it sends changes nothing;
 * and when it is the only client, tmux sizes the window by it, so it sends
 * the size the window already is. Dragging the window changes only the font.
 * A session the inventory has no size for is fitted the way a tile is, grid
 * to box, which is the one case where a sole-client peek can still reflow.
 *
 * The room is the caps, 90% of the workspace in each direction, or the size
 * the operator dragged, less the header and hairline, and the window is the
 * grid at the fitted font inside it. The font is fitted to the room and never
 * to the window, so the window wrapping the grid cannot move the font, and a
 * peek opens at one size. The operator resizes it from any edge or corner
 * through the shared floating-window frame, and that size is remembered for
 * every later peek until Reset size gives the session the say again; slack
 * inside a remembered size is terminal background.
 *
 * Opening Peek focuses its terminal, and Escape typed there is the session's,
 * so a menu can be cancelled and an agent interrupted from here. Dismissal is
 * otherwise the owner's: a press outside closes it and is consumed, Escape
 * with focus anywhere else closes it, and Alt+P closes it from inside its own
 * terminal. The header carries the mark, the name, Send and Close as words.
 */

import { useEffect, useMemo, useRef, useState } from 'react'
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
import type { FixedGridBox } from '../terminal/terminalSession'
import { TERMINAL_FONT_FAMILY } from '../theme/theme'
import './Peek.css'

/** The most of the workspace Peek takes, in each direction. */
export const PEEK_MAX_WIDTH_SHARE = 0.9
export const PEEK_MAX_HEIGHT_SHARE = 0.9
/** The width of a session the inventory has no size for, in columns. */
export const PEEK_FALLBACK_COLS = 100
/** The header's fixed height, as Peek.css draws it. */
export const PEEK_HEADER_PX = 30
/** The window's hairline, on each side. */
const PEEK_HAIRLINE_PX = 1
/**
 * Around a grid fitted to the box rather than the font: the terminal's own
 * padding (2px 4px) and the 14px the fit addon reserves for a scrollbar.
 */
const FALLBACK_TERMINAL_CHROME_PX = 8 + 14

/**
 * The window around a terminal box of this size, inside the workspace's caps:
 * the header above it and the hairline around both. With no box yet, the caps
 * themselves, which is the most room a font fit can be given.
 */
export function peekSize(terminalBox: FrameSize | null, workspace: FrameSize): FrameSize {
  const cap = {
    width: Math.floor(workspace.width * PEEK_MAX_WIDTH_SHARE),
    height: Math.floor(workspace.height * PEEK_MAX_HEIGHT_SHARE),
  }
  if (!terminalBox) return cap
  return {
    width: Math.min(Math.ceil(terminalBox.width) + 2 * PEEK_HAIRLINE_PX, cap.width),
    height: Math.min(Math.ceil(terminalBox.height) + PEEK_HEADER_PX + 2 * PEEK_HAIRLINE_PX, cap.height),
  }
}

/** The room a window of this size leaves its terminal, inside the header and hairline. */
export function peekRoom(window: FrameSize): FrameSize {
  return {
    width: window.width - 2 * PEEK_HAIRLINE_PX,
    height: window.height - PEEK_HEADER_PX - 2 * PEEK_HAIRLINE_PX,
  }
}

/**
 * The terminal box for a session the inventory has no size for: it is fitted
 * grid to box like a tile, so it is offered a width in columns at the
 * operator's font, a cell measured as xterm measures one, and every row the
 * height cap allows.
 */
export function fallbackTerminalBox(fontSize: number): FrameSize {
  const probe = document.createElement('span')
  probe.textContent = 'W'.repeat(32)
  probe.style.position = 'absolute'
  probe.style.visibility = 'hidden'
  probe.style.whiteSpace = 'pre'
  probe.style.fontFamily = TERMINAL_FONT_FAMILY
  probe.style.fontSize = `${fontSize}px`
  document.body.appendChild(probe)
  const cellWidth = probe.offsetWidth / 32 || fontSize * 0.6
  probe.remove()
  return { width: PEEK_FALLBACK_COLS * cellWidth + FALLBACK_TERMINAL_CHROME_PX, height: Infinity }
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
  useSurface({ open: floatingSession !== null, kind: 'glance', onClose: closeFloatingModal, ref: peekRef })

  // The session asks for its own size, and a size the operator dragged
  // overrides it, for this session and every later one. With the grid known,
  // the terminal is fitted to a room and says what box its grid took there,
  // and a box counts only for the grid, room and font ceiling it was fitted
  // for. Without a dragged size the window wraps that box, capped.
  const [box, setBox] = useState<FixedGridBox | null>(null)
  const fittedIn = (room: FrameSize | null) => (
    box && windowGrid && room
      && box.cols === windowGrid.cols && box.rows === windowGrid.rows
      && box.room.width === room.width && box.room.height === room.height
      && box.maxFontSize === settings.fontSize
      ? box
      : null
  )
  const fallbackBox = useMemo(
    () => (windowGrid ? null : fallbackTerminalBox(settings.fontSize)),
    [windowGrid, settings.fontSize],
  )
  const contentSize = (workspace: FrameSize) => (
    peekSize(fittedIn(peekRoom(peekSize(null, workspace))) ?? fallbackBox, workspace)
  )
  const frame = useFloatingFrame({
    kind: 'peek',
    elementRef: peekRef,
    open: floatingSession !== null,
    label: 'session',
    contentSize,
  })
  const outer = frame.remembered || frame.resizing ? frame.size : frame.bounds && peekSize(null, frame.bounds)
  const room = outer && peekRoom(outer)
  const fitted = fittedIn(room)

  // The URL is kept even once the session has ended, so the terminal holding
  // the last frame is not disposed; `connect` is what stops it dialling again.
  const { session: terminal, connectionState } = useTerminalSession(
    socketUrl, settings.fontSize, settings.hideScrollbar, windowGrid && room ? { ...windowGrid, room } : null, setBox,
  )

  // A peek is opened to be used: the keys go to the session it shows.
  useEffect(() => { terminal?.focus() }, [terminal])

  // Until the terminal has fitted in its room, the window is laid out but not
  // shown, so a peek appears once, at its size. Transparent rather than
  // hidden: a hidden textarea cannot hold the focus opening Peek puts in its
  // terminal. A session with no size in the inventory, fitted grid to box, is
  // right the moment it is drawn.
  const measuring = canOpenSession && windowGrid !== null && fitted === null

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
      style={frame.size
        ? { width: frame.size.width, height: frame.size.height, opacity: measuring ? 0 : undefined }
        : undefined}
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
            <div className="peek-terminal" style={fitted ? { width: fitted.width, height: fitted.height } : undefined}>
              <TerminalSurface session={terminal} connect={!ended} />
            </div>
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
