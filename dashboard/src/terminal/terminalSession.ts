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
import { copyTextToClipboard } from '../utils/clipboard'
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
  dispose(): void
}

export interface TerminalSessionOptions {
  url: string
  fontSize: number
  hideScrollbar: boolean
  onStateChange?: (state: TerminalConnectionState) => void
}

const TERMINAL_BACKGROUND = '#0a0a0a'

// A grid is only meaningful once the container has real layout. Detached and
// display:none terminals report zero, and fitting them would push a bogus size
// to the shared tmux window.
const MIN_VISIBLE_PX = 10

export function createTerminalSession(options: TerminalSessionOptions): TerminalSession {
  const element = document.createElement('div')
  element.className = 'terminal-surface'

  const terminal = new Terminal({
    fontSize: options.fontSize,
    fontFamily: 'Menlo, Monaco, Consolas, "Courier New", monospace',
    theme: { background: TERMINAL_BACKGROUND },
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

  let connection: TtydConnection | null = null
  let opened = false
  let disposed = false
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
    if (painted && painted !== selectionAtPress) void copyTextToClipboard(painted)
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
      container.appendChild(element)
      if (!opened) {
        terminal.open(element)
        opened = true
      }
      fit()
      if (!connection && attachOptions?.connect !== false) connect()
    },
    detach() {
      element.remove()
    },
    fit,
    focus() {
      terminal.focus()
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
