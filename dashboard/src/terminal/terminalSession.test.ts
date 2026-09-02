import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { FakeSocket } from '../test/fakeWebSocket'
import { createTerminalSession, type TerminalConnectionState } from './terminalSession'

// jsdom cannot measure a font, so the real fit addon has nothing to work from.
// The grid it would compute is stubbed; everything else is the real xterm.
const fittedGrid = { cols: 100, rows: 30 }
const resetFittedGrid = () => { fittedGrid.cols = 100; fittedGrid.rows = 30 }
vi.mock('@xterm/addon-fit', () => ({
  FitAddon: class {
    private terminal: { resize(cols: number, rows: number): void } | null = null
    activate(terminal: { resize(cols: number, rows: number): void }) { this.terminal = terminal }
    dispose() { this.terminal = null }
    fit() { this.terminal?.resize(fittedGrid.cols, fittedGrid.rows) }
  },
}))

const onScreen = { width: 800, height: 600 }

function sizeElements(width: number, height: number) {
  onScreen.width = width
  onScreen.height = height
}

const URL = 'ws://host/terminal/ws?arg=main'

function start(overrides: Partial<Parameters<typeof createTerminalSession>[0]> = {}) {
  const states: TerminalConnectionState[] = []
  const session = createTerminalSession({
    url: URL,
    fontSize: 14,
    hideScrollbar: false,
    onStateChange: state => states.push(state),
    ...overrides,
  })
  const host = document.createElement('div')
  document.body.appendChild(host)
  return { session, host, states }
}

describe('terminal session', () => {
  beforeEach(() => {
    FakeSocket.instances = []
    sizeElements(800, 600)
    resetFittedGrid()
    vi.stubGlobal('WebSocket', FakeSocket)
    Object.defineProperty(HTMLElement.prototype, 'offsetWidth', { configurable: true, get: () => onScreen.width })
    Object.defineProperty(HTMLElement.prototype, 'offsetHeight', { configurable: true, get: () => onScreen.height })
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    document.body.innerHTML = ''
  })

  it('does not dial ttyd until a tile actually shows the terminal', () => {
    const { session, host } = start()

    expect(FakeSocket.instances).toHaveLength(0)

    session.attach(host)

    expect(FakeSocket.latest().url).toBe(URL)
    session.dispose()
  })

  it('renders pty output where the operator can see it', async () => {
    const { session, host } = start()
    session.attach(host)
    FakeSocket.latest().accept()

    FakeSocket.latest().deliver('0', 'session ready')

    await vi.waitFor(() => expect(host.textContent).toContain('session ready'))
    session.dispose()
  })

  it('sends what the operator types to ttyd', () => {
    const { session, host } = start()
    session.attach(host)
    const socket = FakeSocket.latest()
    socket.accept()

    const textarea = host.querySelector('textarea')
    expect(textarea).not.toBeNull()
    textarea!.dispatchEvent(new KeyboardEvent('keydown', { key: 'a', keyCode: 65, bubbles: true, cancelable: true }))

    expect(socket.sentText).toContain('0a')
    session.dispose()
  })

  it('reports the grid to ttyd when the frame changes size', () => {
    const { session, host } = start()
    session.attach(host)
    const socket = FakeSocket.latest()
    socket.accept()
    // Attaching fits before it dials, so the frame size reaches ttyd in the
    // handshake rather than as a correction after the pty already exists.
    expect(JSON.parse(socket.sentText[0])).toMatchObject({ columns: 100, rows: 30 })
    socket.sent.length = 0

    fittedGrid.cols = 120
    session.fit()

    expect(socket.sentText).toEqual(['1{"columns":120,"rows":30}'])
    session.dispose()
  })

  it('never fits a terminal that is off screen, because the tmux window is shared', () => {
    const { session, host } = start()
    session.attach(host)
    const socket = FakeSocket.latest()
    socket.accept()
    socket.sent.length = 0

    sizeElements(0, 0)
    session.fit()

    expect(socket.sentText).toHaveLength(0)
    session.dispose()
  })

  it('refits after a font size change so the grid matches the new cell metrics', () => {
    const { session, host } = start()
    session.attach(host)
    const socket = FakeSocket.latest()
    socket.accept()
    socket.sent.length = 0
    fittedGrid.cols = 60

    session.setFontSize(20)

    expect(socket.sentText).toContain('1{"columns":60,"rows":30}')
    session.dispose()
  })

  it('keeps the rendered frame across a detach and reattach without reconnecting', async () => {
    const { session, host } = start()
    session.attach(host)
    FakeSocket.latest().accept()
    FakeSocket.latest().deliver('0', 'still here')
    await vi.waitFor(() => expect(host.textContent).toContain('still here'))

    session.detach()
    expect(host.textContent).toBe('')

    const elsewhere = document.createElement('div')
    document.body.appendChild(elsewhere)
    session.attach(elsewhere)

    expect(FakeSocket.instances).toHaveLength(1)
    expect(elsewhere.textContent).toContain('still here')
    session.dispose()
  })

  it('reconnects on a fresh socket without reloading anything', () => {
    const { session, host } = start()
    session.attach(host)
    const first = FakeSocket.latest()
    first.accept()
    const element = host.firstElementChild

    session.reconnect()

    expect(FakeSocket.instances).toHaveLength(2)
    expect(first.readyState).toBe(FakeSocket.CLOSED)
    expect(host.firstElementChild).toBe(element)
    session.dispose()
  })

  it('reports connection state including the close the tile state model needs', () => {
    const { session, host, states } = start()

    session.attach(host)
    expect(states).toEqual(['connecting'])

    FakeSocket.latest().accept()
    expect(states).toEqual(['connecting', 'open'])

    FakeSocket.latest().endCleanly()
    expect(states).toEqual(['connecting', 'open', 'closed'])
    session.dispose()
  })

  it('tells a lost connection apart from a terminal the host ended', () => {
    const { session, host, states } = start()
    session.attach(host)
    FakeSocket.latest().accept()

    FakeSocket.latest().close()

    expect(states).toEqual(['connecting', 'open', 'dropped'])
    session.dispose()
  })

  it('dials again once when a terminal whose connection was lost is put on screen', () => {
    const { session, host } = start()
    session.attach(host)
    FakeSocket.latest().accept()
    FakeSocket.latest().close()

    session.redialIfDropped()

    expect(FakeSocket.instances).toHaveLength(2)

    // Asking again while that dial is in flight adds nothing. Nothing retries
    // on its own; each attempt costs an on-screen moment the operator created.
    session.redialIfDropped()
    session.redialIfDropped()
    expect(FakeSocket.instances).toHaveLength(2)
    session.dispose()
  })

  it('never dials again after the host ended the terminal, because another client may hold it', () => {
    const { session, host } = start()
    session.attach(host)
    FakeSocket.latest().accept()

    FakeSocket.latest().endCleanly()
    session.redialIfDropped()

    expect(FakeSocket.instances).toHaveLength(1)
    session.dispose()
  })

  it('never dials again while off screen, where a -d attach would evict a client nobody can see', () => {
    const { session, host } = start()
    session.attach(host)
    FakeSocket.latest().accept()
    FakeSocket.latest().close()

    sizeElements(0, 0)
    session.redialIfDropped()

    expect(FakeSocket.instances).toHaveLength(1)
    session.dispose()
  })

  it('hides the dead xterm scrollbar gutter on request', () => {
    const { session, host } = start({ hideScrollbar: true })
    session.attach(host)
    const surface = host.firstElementChild!

    expect(surface.classList.contains('terminal-surface--no-scrollbar')).toBe(true)

    session.setScrollbarHidden(false)
    expect(surface.classList.contains('terminal-surface--no-scrollbar')).toBe(false)
    session.dispose()
  })

  it('closes the connection and takes the terminal off screen when disposed', () => {
    const { session, host } = start()
    session.attach(host)
    const socket = FakeSocket.latest()
    socket.accept()

    session.dispose()

    expect(socket.readyState).toBe(FakeSocket.CLOSED)
    expect(host.childElementCount).toBe(0)
  })
})
