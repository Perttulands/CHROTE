import { act, render } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TerminalPoolProvider, useTerminalPool } from './TerminalPool'
import { DEFAULT_SETTINGS } from '../types'
import type { TerminalConnectionState } from '../terminal/terminalSession'

interface CreatedSession {
  url: string
  fontSize: number
  hideScrollbar: boolean
  disposed: boolean
  report: (state: TerminalConnectionState) => void
}

const created = vi.hoisted(() => [] as CreatedSession[])

vi.mock('../terminal/terminalSession', () => ({
  createTerminalSession: (options: {
    url: string
    fontSize: number
    hideScrollbar: boolean
    onStateChange?: (state: TerminalConnectionState) => void
  }) => {
    const record: CreatedSession = {
      url: options.url,
      fontSize: options.fontSize,
      hideScrollbar: options.hideScrollbar,
      disposed: false,
      report: state => options.onStateChange?.(state),
    }
    created.push(record)
    return {
      attach: vi.fn(),
      detach: vi.fn(),
      fit: vi.fn(),
      focus: vi.fn(),
      setFontSize: (size: number) => { record.fontSize = size },
      setScrollbarHidden: (hidden: boolean) => { record.hideScrollbar = hidden },
      reconnect: vi.fn(),
      dispose: () => { record.disposed = true },
    }
  },
}))

const sessionState = vi.hoisted(() => ({
  settings: { fontSize: 14, hideScrollbar: false },
  workspaces: {} as Record<string, { windows: { boundSessions: string[] }[] }>,
}))

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({
    sessions: [{ name: 'alpha', windows: 1, attached: false, group: 'shell', unixUser: 'alice' }],
    workspaces: sessionState.workspaces,
    settings: { ...DEFAULT_SETTINGS, ...sessionState.settings },
  }),
}))

let pool: ReturnType<typeof useTerminalPool>

function Probe() {
  pool = useTerminalPool()
  return null
}

function renderPool() {
  return render(<TerminalPoolProvider><Probe /></TerminalPoolProvider>)
}

describe('terminal pool', () => {
  beforeEach(() => {
    created.length = 0
    sessionState.settings = { fontSize: 14, hideScrollbar: false }
    sessionState.workspaces = {
      terminal1: { windows: [{ boundSessions: ['alice:alpha', 'INIT-PENDING'] }, { boundSessions: [] }] },
      // A workspace whose tab is not on screen still owns its bindings.
      terminal2: { windows: [{ boundSessions: ['bob:beta'] }] },
    }
  })

  it('holds one terminal per bound session in every workspace, ignoring pending slots', () => {
    renderPool()

    expect(Array.from(pool.terminals.keys()).sort()).toEqual(['alice:alpha', 'bob:beta'])
    expect(created.map(session => session.url)).toEqual([
      expect.stringContaining('/terminal/ws?arg=tile&arg=alpha&arg=alice'),
      expect.stringContaining('/terminal/ws?arg=tile&arg=beta&arg=bob'),
    ])
  })

  it('disposes a terminal once the operator unbinds its session', () => {
    const { rerender } = renderPool()
    const beta = created[1]

    sessionState.workspaces = { terminal1: { windows: [{ boundSessions: ['alice:alpha'] }] } }
    rerender(<TerminalPoolProvider><Probe /></TerminalPoolProvider>)

    expect(beta.disposed).toBe(true)
    expect(Array.from(pool.terminals.keys())).toEqual(['alice:alpha'])
  })

  it('applies the appearance settings to every pooled terminal', () => {
    const { rerender } = renderPool()
    expect(created.map(session => session.fontSize)).toEqual([14, 14])

    sessionState.settings = { fontSize: 18, hideScrollbar: true }
    rerender(<TerminalPoolProvider><Probe /></TerminalPoolProvider>)

    expect(created.map(session => session.fontSize)).toEqual([18, 18])
    expect(created.map(session => session.hideScrollbar)).toEqual([true, true])
  })

  it('publishes each terminal connection state, including the close a tile must show', () => {
    renderPool()

    expect(pool.connectionStates.get('alice:alpha')).toBeUndefined()

    act(() => { created[0].report('open') })
    expect(pool.connectionStates.get('alice:alpha')).toBe('open')

    act(() => { created[0].report('closed') })
    expect(pool.connectionStates.get('alice:alpha')).toBe('closed')
  })

  it('disposes every terminal when the dashboard unmounts', () => {
    const { unmount } = renderPool()

    unmount()

    expect(created.every(session => session.disposed)).toBe(true)
  })
})
