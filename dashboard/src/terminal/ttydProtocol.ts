// ttyd's browser protocol, spoken directly by CHROTE (ADR-0018).
//
// Frames are binary with a one-byte ASCII command prefix. The client sends
// `0` input, `1` resize as JSON, `2`/`3` flow control, and an unprefixed JSON
// object as the opening handshake; ttyd spawns no pty until that handshake
// arrives. The server sends `0` output, `1` window title and `2` preferences.
// The constants are byte-identical across ttyd 1.6.3 through 1.7.7 and main.

const CLIENT_INPUT = 0x30 // '0'
const CLIENT_RESIZE = '1'
const CLIENT_PAUSE = '2'
const CLIENT_RESUME = '3'

const SERVER_OUTPUT = 0x30 // '0'

// ttyd's own flow-control thresholds. Above the byte limit, output is written
// with a drain callback and ttyd is told to stop reading the pty until the
// renderer catches up, so a firehose cannot grow an unbounded write queue.
const FLOW_BYTE_LIMIT = 100_000
const FLOW_HIGH_WATER = 10
const FLOW_LOW_WATER = 4

export interface TerminalOutputSink {
  /** Write pty output. `onDrained` fires once the renderer has consumed it. */
  write(data: Uint8Array, onDrained?: () => void): void
}

export interface TtydConnection {
  /** Strings are sent as UTF-8; byte arrays (xterm's `onBinary`) are sent raw. */
  sendInput(data: string | Uint8Array): void
  sendResize(cols: number, rows: number): void
  /** Close without reporting `onClose`; the caller already knows. */
  close(): void
}

// The close code CHROTE sends, and the only one it sends: the pty hung up, or
// the attach was refused. Anything else reaching the browser is a connection
// nobody closed on purpose.
const NORMAL_CLOSURE = 1000

export interface TtydClose {
  /**
   * The server said the terminal ended, by closing with its own close frame.
   * Every other close is the connection being lost with the terminal still on
   * the other end — the service restarting (1006, no frame at all), the network
   * dropping, the device sleeping, an intermediary giving up. The session is
   * untouched by those, so they are worth dialling again and this is not.
   */
  terminalEnded: boolean
}

export interface TtydConnectionEvents {
  onOpen?: () => void
  onClose?: (close: TtydClose) => void
}

export function connectTtyd(
  url: string,
  initialSize: { cols: number; rows: number },
  sink: TerminalOutputSink,
  events: TtydConnectionEvents = {},
): TtydConnection {
  const socket = new WebSocket(url, ['tty'])
  socket.binaryType = 'arraybuffer'
  const encoder = new TextEncoder()
  const send = (payload: string) => {
    if (socket.readyState === WebSocket.OPEN) socket.send(encoder.encode(payload))
  }

  let writtenSinceLimit = 0
  let pendingWrites = 0

  socket.onopen = () => {
    // ttyd runs without -c, so the handshake token is empty and no /token
    // fetch is needed.
    send(JSON.stringify({ AuthToken: '', columns: initialSize.cols, rows: initialSize.rows }))
    events.onOpen?.()
  }
  socket.onerror = () => socket.close()
  socket.onclose = event => events.onClose?.({ terminalEnded: event.code === NORMAL_CLOSURE })
  socket.onmessage = (event: MessageEvent<ArrayBuffer>) => {
    const frame = new Uint8Array(event.data)
    // Window title and preferences are not consumed: CHROTE titles its own
    // tiles and owns its own xterm options.
    if (frame.length === 0 || frame[0] !== SERVER_OUTPUT) return
    const data = frame.subarray(1)

    writtenSinceLimit += data.length
    if (writtenSinceLimit <= FLOW_BYTE_LIMIT) {
      sink.write(data)
      return
    }
    writtenSinceLimit = 0
    pendingWrites += 1
    sink.write(data, () => {
      pendingWrites = Math.max(pendingWrites - 1, 0)
      if (pendingWrites < FLOW_LOW_WATER) send(CLIENT_RESUME)
    })
    if (pendingWrites > FLOW_HIGH_WATER) send(CLIENT_PAUSE)
  }

  return {
    sendInput: data => {
      if (socket.readyState !== WebSocket.OPEN) return
      const bytes = typeof data === 'string' ? encoder.encode(data) : data
      const frame = new Uint8Array(bytes.length + 1)
      frame[0] = CLIENT_INPUT
      frame.set(bytes, 1)
      socket.send(frame)
    },
    sendResize: (cols, rows) => send(CLIENT_RESIZE + JSON.stringify({ columns: cols, rows })),
    close: () => {
      socket.onclose = null
      socket.close()
    },
  }
}

/**
 * How this connection views the session. A `tile` is the operator's viewing
 * seat and takes the session over, so it is the window's one sizing client; a
 * `peek` observes without displacing the tile or resizing the window.
 */
export type TerminalViewingMode = 'tile' | 'peek'

/**
 * WebSocket URL for a tmux session, carrying ttyd's `-a` argument fragments.
 * The mode leads because the Unix user is optional, so the launch script would
 * have no way to tell a trailing mode from a user.
 */
export function terminalSocketUrl(sessionName: string, unixUser: string, mode: TerminalViewingMode): string {
  const scheme = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const userArg = unixUser.trim() ? `&arg=${encodeURIComponent(unixUser)}` : ''
  return `${scheme}//${window.location.host}/terminal/ws?arg=${mode}&arg=${encodeURIComponent(sessionName)}${userArg}`
}
