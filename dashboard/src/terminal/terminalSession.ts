// One in-page terminal: an xterm.js instance plus its ttyd connection.
//
// The element is owned here, not by React, so a terminal survives being
// detached from the document — a tile scrolled out of the window count, a
// workspace tab switch. Detaching a plain element neither reloads it nor
// touches the WebSocket, which is what the retired iframe layer had to work
// around.

import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { connectTtyd, type TtydConnection } from './ttydProtocol'
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
   * Dial again if the last connection was lost rather than ended, and the
   * terminal is on screen. Called when the operator puts the terminal in front
   * of himself; each such moment is worth one attempt and no more. Nothing here
   * retries on its own, and a dial that fails leaves the tile's own Reconnect
   * control as the way back.
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
  })
  const fitAddon = new FitAddon()
  terminal.loadAddon(fitAddon)

  let connection: TtydConnection | null = null
  let opened = false
  let disposed = false
  // The last connection was lost rather than ended, so dialling again reaches
  // the same terminal instead of taking a session from whoever holds it.
  let dropped = false

  const setState = (state: TerminalConnectionState) => {
    if (!disposed) options.onStateChange?.(state)
  }

  terminal.onData(data => connection?.sendInput(data))
  terminal.onBinary(data => connection?.sendInput(Uint8Array.from(data, char => char.charCodeAt(0) & 0xff)))
  terminal.onResize(({ cols, rows }) => connection?.sendResize(cols, rows))

  const isMeasurable = () => element.offsetWidth >= MIN_VISIBLE_PX && element.offsetHeight >= MIN_VISIBLE_PX

  const fit = () => {
    if (!opened || !isMeasurable()) return
    fitAddon.fit()
  }

  const connect = () => {
    dropped = false
    setState('connecting')
    connection = connectTtyd(options.url, { cols: terminal.cols, rows: terminal.rows }, terminal, {
      onOpen: () => setState('open'),
      onClose: ({ terminalEnded }) => {
        connection = null
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
    redialIfDropped() {
      // Off screen is not a moment worth an attempt, and it is where a wrong
      // dial would do the damage: a tile attaches with -d, so it would take a
      // session back from a client the operator can see and this one cannot.
      if (disposed || connection || !dropped || !isMeasurable()) return
      connect()
    },
    dispose() {
      disposed = true
      connection?.close()
      connection = null
      terminal.dispose()
      element.remove()
    },
  }
}
