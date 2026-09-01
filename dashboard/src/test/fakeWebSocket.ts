/** A controllable stand-in for the browser WebSocket, for terminal tests. */
export class FakeSocket {
  static readonly OPEN = 1
  static readonly CLOSED = 3
  static instances: FakeSocket[] = []

  static latest(): FakeSocket {
    const socket = FakeSocket.instances[FakeSocket.instances.length - 1]
    if (!socket) throw new Error('no WebSocket was opened')
    return socket
  }

  readyState = 0
  binaryType = ''
  sent: Uint8Array[] = []
  onopen: (() => void) | null = null
  onclose: (() => void) | null = null
  onerror: (() => void) | null = null
  onmessage: ((event: MessageEvent<ArrayBuffer>) => void) | null = null

  constructor(readonly url: string, readonly protocols?: string | string[]) {
    FakeSocket.instances.push(this)
  }

  send(data: Uint8Array) { this.sent.push(data) }

  close() {
    if (this.readyState === FakeSocket.CLOSED) return
    this.readyState = FakeSocket.CLOSED
    this.onclose?.()
  }

  /** Complete the handshake the way a live ttyd would. */
  accept() {
    this.readyState = FakeSocket.OPEN
    this.onopen?.()
  }

  /** Deliver one server frame with the given one-byte command prefix. */
  deliver(command: string, payload: Uint8Array | string) {
    const body = typeof payload === 'string' ? new TextEncoder().encode(payload) : payload
    const frame = new Uint8Array(body.length + 1)
    frame[0] = command.charCodeAt(0)
    frame.set(body, 1)
    this.onmessage?.({ data: frame.buffer } as MessageEvent<ArrayBuffer>)
  }

  /** Everything the client sent, decoded as text. */
  get sentText(): string[] {
    return this.sent.map(frame => new TextDecoder().decode(frame))
  }
}
