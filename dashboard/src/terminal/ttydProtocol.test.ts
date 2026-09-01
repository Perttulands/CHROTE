import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { FakeSocket } from '../test/fakeWebSocket'
import { connectTtyd, terminalSocketUrl, type TerminalOutputSink } from './ttydProtocol'

const decode = (frame: Uint8Array) => new TextDecoder().decode(frame)

function recordingSink() {
  const written: string[] = []
  const drains: (() => void)[] = []
  const sink: TerminalOutputSink = {
    write(data, onDrained) {
      written.push(decode(data))
      if (onDrained) drains.push(onDrained)
    },
  }
  return { sink, written, drains }
}

describe('ttyd transport', () => {
  beforeEach(() => {
    FakeSocket.instances = []
    vi.stubGlobal('WebSocket', FakeSocket)
  })
  afterEach(() => { vi.unstubAllGlobals() })

  const connect = (sink: TerminalOutputSink, events = {}) =>
    connectTtyd('ws://host/terminal/ws?arg=main', { cols: 80, rows: 24 }, sink, events)

  it('asks for the tty subprotocol and announces the grid before ttyd spawns a pty', () => {
    const { sink } = recordingSink()
    const onOpen = vi.fn()
    connect(sink, { onOpen })
    const socket = FakeSocket.instances[0]

    expect(socket.protocols).toEqual(['tty'])
    expect(socket.binaryType).toBe('arraybuffer')
    expect(socket.sent).toHaveLength(0)

    socket.accept()

    expect(JSON.parse(decode(socket.sent[0]))).toEqual({ AuthToken: '', columns: 80, rows: 24 })
    expect(onOpen).toHaveBeenCalled()
  })

  it('sends keystrokes to ttyd', () => {
    const { sink } = recordingSink()
    const connection = connect(sink)
    const socket = FakeSocket.instances[0]
    socket.accept()

    connection.sendInput('ls -l\r')

    expect(decode(socket.sent[1])).toBe('0ls -l\r')
  })

  it('sends binary key sequences byte for byte rather than as text', () => {
    const { sink } = recordingSink()
    const connection = connect(sink)
    const socket = FakeSocket.instances[0]
    socket.accept()

    connection.sendInput(Uint8Array.from([0x00, 0xff]))

    expect(Array.from(socket.sent[1])).toEqual([0x30, 0x00, 0xff])
  })

  it('reports a new grid when the terminal resizes', () => {
    const { sink } = recordingSink()
    const connection = connect(sink)
    const socket = FakeSocket.instances[0]
    socket.accept()

    connection.sendResize(120, 50)

    expect(decode(socket.sent[1])).toBe('1{"columns":120,"rows":50}')
  })

  it('drops input while the socket is not open instead of throwing', () => {
    const { sink } = recordingSink()
    const connection = connect(sink)
    const socket = FakeSocket.instances[0]

    connection.sendInput('typed before connect')
    connection.sendResize(10, 10)

    expect(socket.sent).toHaveLength(0)
  })

  it('renders pty output and ignores window title and preference frames', () => {
    const { sink, written } = recordingSink()
    connect(sink)
    const socket = FakeSocket.instances[0]
    socket.accept()

    socket.deliver('1', 'some tmux title')
    socket.deliver('2', '{"fontSize":9}')
    socket.deliver('0', 'hello from the pty')

    expect(written).toEqual(['hello from the pty'])
  })

  it('pauses ttyd when output outruns the renderer and resumes once it drains', () => {
    const { sink, drains } = recordingSink()
    connect(sink)
    const socket = FakeSocket.instances[0]
    socket.accept()

    const firehoseFrame = new Uint8Array(100_001)
    for (let i = 0; i < 11; i++) socket.deliver('0', firehoseFrame)

    const control = () => socket.sent.slice(1).map(decode)
    expect(control()).toContain('2')
    expect(control()).not.toContain('3')

    drains.forEach(drain => drain())

    expect(control()).toContain('3')
  })

  it('reports a connection that ttyd or the network closed', () => {
    const { sink } = recordingSink()
    const onClose = vi.fn()
    connect(sink, { onClose })
    const socket = FakeSocket.instances[0]
    socket.accept()

    socket.close()

    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('does not report a close the caller asked for', () => {
    const { sink } = recordingSink()
    const onClose = vi.fn()
    const connection = connect(sink, { onClose })
    FakeSocket.instances[0].accept()

    connection.close()

    expect(onClose).not.toHaveBeenCalled()
    expect(FakeSocket.instances[0].readyState).toBe(FakeSocket.CLOSED)
  })
})

describe('terminalSocketUrl', () => {
  it('carries the session and the Unix user as ttyd argument fragments', () => {
    expect(terminalSocketUrl('my session', 'alice'))
      .toBe(`ws://${window.location.host}/terminal/ws?arg=my%20session&arg=alice`)
  })

  it('omits the user fragment when the session carries no Unix user', () => {
    expect(terminalSocketUrl('main', '  ')).toBe(`ws://${window.location.host}/terminal/ws?arg=main`)
  })
})
