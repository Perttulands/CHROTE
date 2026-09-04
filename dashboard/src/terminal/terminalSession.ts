// One in-page terminal: an xterm.js instance plus its ttyd connection.
//
// The element is owned here, not by React, so a terminal survives being
// detached from the document — a tile scrolled out of the window count, a
// workspace tab switch. Detaching a plain element neither reloads it nor
// touches the WebSocket, which is what the retired iframe layer had to work
// around.

import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { Unicode11Addon } from '@xterm/addon-unicode11'
import { WebLinksAddon } from '@xterm/addon-web-links'
import { connectTtyd, type TtydConnection } from './ttydProtocol'
import { createBeadLinkProvider } from './beadLinks'
import { createPathLinkProvider } from './pathLinks'
import { ensureBeadProjects } from '../beads/beadIds'
import { terminalKeyEvent } from '../keys/chords'
import { copyAndAnnounce, type CopyAnnouncer } from '../utils/clipboard'
import type { TerminalTheme } from '../theme/theme'
import '@xterm/xterm/css/xterm.css'
import './terminal.css'

/**
 * `closed` and `dropped` are both "no connection", and the difference is the
 * whole point: `closed` is the host ending this terminal, which is what a
 * takeover and a killed session look like, while `dropped` is the connection
 * being lost with the terminal still on the other end — which is what every
 * `chrote-srv` restart does to every open tile (ADR-0013).
 */
export type TerminalConnectionState = 'idle' | 'connecting' | 'open' | 'closed' | 'dropped'

export interface TerminalSession {
  /**
   * Attach into a live container, connecting on first attach. An ended tile
   * passes `connect: false` so its last frame can be shown without dialling a
   * session that is no longer there.
   */
  attach(container: HTMLElement, options?: { connect?: boolean }): void
  /** Detach from the document, keeping the connection and the rendered frame. */
  detach(): void
  /** Resize the grid to the container. A no-op while detached or hidden. */
  fit(): void
  focus(): void
  /** Put the viewport at the newest output, where an answer arrives. */
  scrollToBottom(): void
  setFontSize(fontSize: number): void
  setScrollbarHidden(hidden: boolean): void
  /** Drop the connection and open a new one, without reloading anything. */
  reconnect(): void
  /**
   * Become this session's one sizing client, so the tmux window takes this
   * terminal's dimensions. Every other viewer keeps watching, at the new size.
   * A terminal with no connection dials first and claims on arrival, which is
   * what the operator asks for when he claims a tile whose connection went.
   */
  claim(): void
  /**
   * Dial again if the last connection was lost rather than ended, and the
   * terminal is on screen. Called when the operator puts the terminal in front
   * of himself; each such moment is worth one attempt and no more. Nothing here
   * retries on its own, and a dial that fails leaves the tile's own Reconnect
   * control as the way back. The dial takes nothing from anyone: it attaches
   * alongside whoever else is watching, without the sizing seat.
   */
  redialIfDropped(): void
  /**
   * Repaint in the host's theme. The palette arrives after the terminal does —
   * GET /api/theme is a fetch, terminals are created from bindings already in
   * local storage — so every live terminal takes it when it lands.
   */
  applyAppearance(terminalTheme: TerminalTheme, fontFamily: string): void
  /**
   * The grid as drawn: its columns and rows, and the CSS size of one cell.
   * Null while the terminal is not open or not on screen, because a hidden
   * grid measures nothing. Peek sizes itself by this.
   */
  grid(): TerminalGrid | null
  dispose(): void
}

export interface TerminalGrid {
  cols: number
  rows: number
  cellWidth: number
  cellHeight: number
}

export interface TerminalSessionOptions {
  url: string
  fontSize: number
  hideScrollbar: boolean
  /** The theme's terminal object: background, foreground, cursor, selection, 16 ansi. */
  terminalTheme: TerminalTheme
  fontFamily: string
  onStateChange?: (state: TerminalConnectionState) => void
  /** Where a painted selection reports whether it reached the clipboard. */
  announce: CopyAnnouncer
}

// xterm names the 16 ansi entries; the theme carries them as an ordered array,
// because that is the order every palette in the world is written in.
function xtermTheme(theme: TerminalTheme) {
  const [
    black, red, green, yellow, blue, magenta, cyan, white,
    brightBlack, brightRed, brightGreen, brightYellow, brightBlue, brightMagenta, brightCyan, brightWhite,
  ] = theme.ansi
  return {
    background: theme.background,
    foreground: theme.foreground,
    cursor: theme.cursor,
    selectionBackground: theme.selectionBackground,
    black, red, green, yellow, blue, magenta, cyan, white,
    brightBlack, brightRed, brightGreen, brightYellow, brightBlue, brightMagenta, brightCyan, brightWhite,
  }
}

// A grid is only meaningful once the container has real layout. Detached and
// display:none terminals report zero, and fitting them would push a bogus size
// to the shared tmux window.
const MIN_VISIBLE_PX = 10

export function createTerminalSession(options: TerminalSessionOptions): TerminalSession {
  const element = document.createElement('div')
  element.className = 'terminal-surface'

  const terminal = new Terminal({
    fontSize: options.fontSize,
    fontFamily: options.fontFamily,
    theme: xtermTheme(options.terminalTheme),
    // xterm files its Unicode version handling as proposed API, so the width
    // table below cannot be swapped without this. ttyd's client set it too.
    allowProposedApi: true,
  })
  const fitAddon = new FitAddon()
  terminal.loadAddon(fitAddon)

  // tmux lays the pane out with its own width table, so the browser has to
  // measure characters the same way or every character after an emoji sits a
  // cell left of where tmux put it — closing box-drawing that does not close,
  // and status lines that overlap, on the agent sessions the operator watches
  // most. Measured on a scratch tmux 3.6a socket: printing U+1F680 at column 0
  // leaves tmux's cursor at column 2. xterm defaults to a Unicode 6 provider,
  // under which every emoji is one column. ttyd's client ran Unicode 11, which
  // is why this only started drifting when ttyd left.
  terminal.loadAddon(new Unicode11Addon())
  terminal.unicode.activeVersion = '11'

  // Agent output is full of URLs the operator has to open. ttyd's client
  // loaded this addon, so they were clickable until the transport changed.
  // Activation is a plain click, as it was under ttyd: under tmux mouse mode
  // that click also reaches tmux, where CHROTE's settings make it harmless.
  terminal.loadAddon(new WebLinksAddon())

  // The other thing agents print that the operator wants to open: the Bead id
  // of the work in hand. It is not a URL, so it gets its own provider, and
  // activation opens the card rather than a tab. The prefixes it matches are
  // the configured projects', which is why they are asked for as soon as there
  // is a terminal to print them in.
  terminal.registerLinkProvider(createBeadLinkProvider(terminal))
  void ensureBeadProjects()

  // And the path of the file the agent changed, the test that failed, the log
  // it wrote: activation hands it to Files, which reports plainly if it is not
  // there. Nothing is checked or fetched to offer the link.
  terminal.registerLinkProvider(createPathLinkProvider(terminal))

  // The leader is the one keystroke a focused terminal does not own. Returning
  // false here is what keeps it out of the pty: xterm neither writes it nor
  // sends it, and the chord registry has already taken it. Every other key,
  // and every key at all while keys are off, comes back true and belongs to
  // the shell as it always did.
  terminal.attachCustomKeyEventHandler(terminalKeyEvent)

  let connection: TtydConnection | null = null
  let opened = false
  let disposed = false
  let attachment = 0
  let fontReady: Promise<FontFace[]> | null = null
  // The last connection was lost rather than ended, so dialling again reaches
  // the same terminal instead of taking a session from whoever holds it.
  let dropped = false
  // The operator claimed a terminal that had no connection, so the claim is
  // owed to the connection now on its way up.
  let claimOnOpen = false

  const setState = (state: TerminalConnectionState) => {
    if (!disposed) options.onStateChange?.(state)
  }

  terminal.onData(data => connection?.sendInput(data))
  terminal.onBinary(data => connection?.sendInput(Uint8Array.from(data, char => char.charCodeAt(0) & 0xff)))
  terminal.onResize(({ cols, rows }) => connection?.sendResize(cols, rows))

  // Painting a selection puts it on the clipboard. This was ttyd's client
  // doing `document.execCommand('copy')` on every selection change, not
  // anything xterm does, so it left with the iframe; the operator asked for it
  // back, including the part where painting overwrites the system clipboard
  // without asking. Under tmux mouse mode the gesture that paints is Shift and
  // left-drag, which is what xterm's force-selection escape hatch listens for.
  //
  // The copy waits for the drag to settle rather than following
  // onSelectionChange, which fires once per mousemove. The mouseup that ends a
  // drag can land anywhere, so it is watched on the document, and only while a
  // press that began in this terminal is in flight. The press-time selection is
  // remembered so that a plain click on a terminal that still holds an older
  // selection does not silently put it back over whatever the operator copied
  // since. Reading it needs the capture phase: xterm stops propagation of the
  // very mousedown that forces a selection under mouse mode.
  let selectionAtPress = ''
  const copySettledSelection = () => {
    document.removeEventListener('mouseup', copySettledSelection)
    const painted = terminal.getSelection()
    if (painted && painted !== selectionAtPress) void copyAndAnnounce(painted, 'selection', options.announce, { quiet: true })
  }
  const watchSelectionDrag = () => {
    selectionAtPress = terminal.getSelection()
    document.addEventListener('mouseup', copySettledSelection)
  }
  element.addEventListener('mousedown', watchSelectionDrag, true)

  const isMeasurable = () => element.offsetWidth >= MIN_VISIBLE_PX && element.offsetHeight >= MIN_VISIBLE_PX

  const fit = () => {
    if (!opened || !isMeasurable()) return
    fitAddon.fit()
  }

  // xterm measures its cell when open() runs. A swapped web font arriving
  // later leaves that measurement stale until a resize, so the first fit uses
  // the fallback cell and the resize itself only then teaches xterm the real
  // one. Open after the terminal font is available instead. The browser's
  // FontFaceSet is the completion event; no timer or repeated fit is needed.
  const afterFontReady = (ready: () => void) => {
    const fonts = document.fonts
    const font = `${terminal.options.fontSize}px ${terminal.options.fontFamily}`
    if (!fonts || fonts.check(font)) {
      ready()
      return
    }
    fontReady ??= fonts.load(font)
    void fontReady.then(ready, ready)
  }

  const connect = () => {
    dropped = false
    setState('connecting')
    connection = connectTtyd(options.url, { cols: terminal.cols, rows: terminal.rows }, terminal, {
      onOpen: () => {
        if (claimOnOpen) {
          claimOnOpen = false
          connection?.claimSizing()
        }
        setState('open')
      },
      onClose: ({ terminalEnded }) => {
        connection = null
        // A claim the connection never got to make is not owed to the next one:
        // the operator asked this terminal to take the size, and by the time it
        // dials again the answer may well be different.
        claimOnOpen = false
        dropped = !terminalEnded
        setState(terminalEnded ? 'closed' : 'dropped')
      },
    })
  }

  const setScrollbarHidden = (hidden: boolean) => {
    element.classList.toggle('terminal-surface--no-scrollbar', hidden)
  }
  setScrollbarHidden(options.hideScrollbar)

  return {
    attach(container, attachOptions) {
      if (disposed) return
      const thisAttachment = ++attachment
      container.appendChild(element)
      const finishAttach = () => {
        if (disposed || thisAttachment !== attachment || !element.parentElement) return
        if (!opened) {
          terminal.open(element)
          opened = true
        }
        fit()
        if (!connection && attachOptions?.connect !== false) connect()
      }
      if (opened) finishAttach()
      else afterFontReady(finishAttach)
    },
    detach() {
      attachment += 1
      element.remove()
    },
    fit,
    focus() {
      terminal.focus()
    },
    scrollToBottom() {
      terminal.scrollToBottom()
    },
    setFontSize(fontSize) {
      if (terminal.options.fontSize === fontSize) return
      terminal.options.fontSize = fontSize
      fit()
    },
    setScrollbarHidden,
    reconnect() {
      if (disposed) return
      connection?.close()
      connection = null
      terminal.reset()
      connect()
    },
    claim() {
      if (disposed) return
      if (connection) {
        connection.claimSizing()
        return
      }
      claimOnOpen = true
      connect()
    },
    redialIfDropped() {
      // Off screen is not a moment worth an attempt: a terminal nobody is
      // looking at has nothing to show for the connection it would open.
      if (disposed || connection || !dropped || !isMeasurable()) return
      connect()
    },
    applyAppearance(terminalTheme, fontFamily) {
      if (disposed) return
      terminal.options.theme = xtermTheme(terminalTheme)
      terminal.options.fontFamily = fontFamily
      fit()
    },
    grid() {
      // xterm sizes its screen element to exactly cols by rows cells, so the
      // cell is read from the element rather than from anything private.
      if (!opened || !isMeasurable() || terminal.cols === 0 || terminal.rows === 0) return null
      const screen = element.querySelector<HTMLElement>('.xterm-screen')
      const rect = screen?.getBoundingClientRect()
      if (!rect || rect.width < MIN_VISIBLE_PX || rect.height < MIN_VISIBLE_PX) return null
      return {
        cols: terminal.cols,
        rows: terminal.rows,
        cellWidth: rect.width / terminal.cols,
        cellHeight: rect.height / terminal.rows,
      }
    },
    dispose() {
      disposed = true
      element.removeEventListener('mousedown', watchSelectionDrag, true)
      document.removeEventListener('mouseup', copySettledSelection)
      connection?.close()
      connection = null
      terminal.dispose()
      element.remove()
    },
  }
}
