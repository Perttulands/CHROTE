import { act, render } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TerminalPoolProvider, useTerminalPool } from './TerminalPool'
import { DEFAULT_SETTINGS } from '../types'
import { DEFAULT_THEME, TERMINAL_FONT_FAMILY } from '../theme/theme'
import { ThemeContext } from '../theme/ThemeContext'
import type { TerminalConnectionState } from '../terminal/terminalSession'

interface CreatedSession {
  url: string
  fontSize: number
  hideScrollbar: boolean
  terminalBackground: string
  fontFamily: string
  disposed: boolean
  report: (state: TerminalConnectionState) => void
}

const created = vi.hoisted(() => [] as CreatedSession[])

vi.mock('../terminal/terminalSession', () => ({
  createTerminalSession: (options: {
    url: string
    fontSize: number
    hideScrollbar: boolean
    terminalTheme: { background: string }
    fontFamily: string
    onStateChange?: (state: TerminalConnectionState) => void
  }) => {
    const record: CreatedSession = {
      url: options.url,
      fontSize: options.fontSize,
      hideScrollbar: options.hideScrollbar,
      terminalBackground: options.terminalTheme.background,
      fontFamily: options.fontFamily,
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
      applyAppearance: (theme: { background: string }, fontFamily: string) => {
        record.terminalBackground = theme.background
        record.fontFamily = fontFamily
      },
      dispose: () => { record.disposed = true },
    }
  },
}))

const sessionState = vi.hoisted(() => ({
  settings: { fontSize: 14, hideScrollbar: false },
  workspaces: {} as Record<string, { windows: { boundSessions: string[] }[] }>,
}))

// The host's theme lands after startup, so the pool is always rendered under a
// provider whose value can change without the tree around it changing.
let currentTheme = DEFAULT_THEME

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

function poolTree() {
  return (
    <ThemeContext.Provider value={currentTheme}>
      <TerminalPoolProvider><Probe /></TerminalPoolProvider>
    </ThemeContext.Provider>
  )
}

function renderPool() {
  return render(poolTree())
}

describe('terminal pool', () => {
  beforeEach(() => {
    created.length = 0
    currentTheme = DEFAULT_THEME
    sessionState.settings = { fontSize: 14, hideScrollbar: false }
    sessionState.workspaces = {
      terminal1: { windows: [{ boundSessions: ['alice:alpha'] }, { boundSessions: [] }] },
      // A workspace whose tab is not on screen still owns its bindings.
      terminal2: { windows: [{ boundSessions: ['bob:beta'] }] },
    }
  })

  it('holds one terminal per bound session in every workspace', () => {
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
    rerender(poolTree())

    expect(created.map(session => session.fontSize)).toEqual([18, 18])
    expect(created.map(session => session.hideScrollbar)).toEqual([true, true])
  })

  // The theme arrives from the host after the pool has already built its
  // terminals from stored bindings, so every live one has to take it.
  it('paints every pooled terminal in the host theme and the one font stack', () => {
    renderPool()

    expect(created.map(session => session.terminalBackground)).toEqual([
      DEFAULT_THEME.terminal.background,
      DEFAULT_THEME.terminal.background,
    ])
    expect(created.map(session => session.fontFamily)).toEqual([
      TERMINAL_FONT_FAMILY,
      TERMINAL_FONT_FAMILY,
    ])
    expect(TERMINAL_FONT_FAMILY).toBe('"JetBrains Mono", "CHROTE Term Symbols", monospace')
  })

  it('hands a theme that lands after startup to terminals that already exist', () => {
    const { rerender } = renderPool()
    expect(created.map(session => session.terminalBackground)).toEqual(['#0a0a0a', '#0a0a0a'])

    currentTheme = { ...DEFAULT_THEME, terminal: { ...DEFAULT_THEME.terminal, background: '#123456' } }
    rerender(poolTree())

    expect(created.map(session => session.terminalBackground)).toEqual(['#123456', '#123456'])
    // Repainting is not rebuilding: a live connection must survive it.
    expect(created).toHaveLength(2)
    expect(created.every(session => !session.disposed)).toBe(true)
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
