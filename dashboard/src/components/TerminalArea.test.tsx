import { fireEvent, render, screen, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import TerminalArea from './TerminalArea'

const setWindowCount = vi.fn()
const clearStaleSessionsFromWindow = vi.fn()
const reconnect = vi.fn()
const sessionState = vi.hoisted(() => ({
  isMobile: false,
  windowCount: 2,
  windowRevealRequest: null as { workspaceId: string; windowId: string; requestId: number } | null,
}))

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({
    sessions: [
      { name: 'alpha', windows: 1, attached: false, group: 'shell', unixUser: 'alice' },
    ],
    workspaces: {
      terminal1: {
        windowCount: sessionState.windowCount,
        windows: [
          { id: 'terminal1-window-0', boundSessions: ['alice:alpha', 'bare-session'], activeSession: 'alice:alpha', colorIndex: 0 },
          { id: 'terminal1-window-1', boundSessions: ['bob:beta'], activeSession: 'bob:beta', colorIndex: 1 },
          { id: 'terminal1-window-2', boundSessions: [], activeSession: null, colorIndex: 2 },
          { id: 'terminal1-window-3', boundSessions: ['alice:hidden'], activeSession: 'alice:hidden', colorIndex: 3 },
        ],
      },
    },
    setWindowCount,
    clearStaleSessionsFromWindow,
    windowRevealRequest: sessionState.windowRevealRequest,
  }),
}))

vi.mock('../hooks/useMediaQuery', () => ({
  useMediaQuery: () => sessionState.isMobile,
}))

const pooledTerminals = new Map<string, { reconnect: () => void }>()
vi.mock('./TerminalPool', () => ({
  useTerminalPool: () => ({
    terminals: {
      get(sessionKey: string) {
        if (!pooledTerminals.has(sessionKey)) pooledTerminals.set(sessionKey, { reconnect: () => reconnect(sessionKey) })
        return pooledTerminals.get(sessionKey)
      },
    },
    connectionStates: new Map(),
  }),
}))

vi.mock('./TerminalWindow', () => ({
  default: ({ window, refitNonce, style }: { window: { id: string }, refitNonce: number, style?: React.CSSProperties }) => (
    <div data-testid={`terminal-window-${window.id}`} data-refit-nonce={refitNonce} style={style} />
  ),
}))

describe('TerminalArea layout controls', () => {
  const viewControls = () => within(screen.getByRole('group', { name: 'Window view controls' }))

  beforeEach(() => {
    vi.clearAllMocks()
    sessionState.isMobile = false
    sessionState.windowCount = 2
    sessionState.windowRevealRequest = null
  })

  it('reconnects all visible session frames from the layout controls menu', () => {
    render(<TerminalArea workspaceId="terminal1" />)

    fireEvent.click(screen.getByRole('button', { name: 'Terminal maintenance actions' }))
    fireEvent.click(screen.getByRole('button', { name: /Reconnect frames/i }))

    expect(reconnect.mock.calls.flat().sort()).toEqual(['alice:alpha', 'bare-session', 'bob:beta'])
  })

  it('renders the current desktop layout controls and default visible window count', () => {
    const { container } = render(<TerminalArea workspaceId="terminal1" />)

    for (const count of [1, 2, 3, 4]) {
      expect(screen.getByTitle(`${count} window${count > 1 ? 's' : ''}`)).toBeInTheDocument()
    }
    expect(screen.getByTitle('2 windows')).toHaveClass('active')
    expect(container.querySelector('.terminal-grid')).toHaveClass('grid-2')
    expect(container.querySelectorAll('[data-testid^="terminal-window-"]')).toHaveLength(2)
  })

  it('keeps Refit directly visible while stale cleanup remains in the maintenance menu', () => {
    render(<TerminalArea workspaceId="terminal1" />)

    const refit = screen.getByRole('button', { name: 'Refit terminal layout' })
    expect(refit).toBeVisible()
    expect(refit).toHaveTextContent('Refit')
    fireEvent.click(refit)
    expect(screen.getByTestId('terminal-window-terminal1-window-0')).toHaveAttribute('data-refit-nonce', '1')
    expect(screen.getByTestId('terminal-window-terminal1-window-1')).toHaveAttribute('data-refit-nonce', '1')

    expect(screen.queryByRole('button', { name: /Clean 2 stale sessions/i })).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Terminal maintenance actions' }))
    const menu = document.querySelector('.session-context-menu') as HTMLElement
    expect(within(menu).queryByRole('button', { name: /Refit terminal layout/i })).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /Clear 2 stale sessions/i }))

    expect(clearStaleSessionsFromWindow).toHaveBeenCalledWith('terminal1', 'terminal1-window-0')
    expect(clearStaleSessionsFromWindow).toHaveBeenCalledWith('terminal1', 'terminal1-window-1')
  })

  it('selects a newly revealed hidden slot as the active mobile window after it enters the visible slice', () => {
    sessionState.isMobile = true
    sessionState.windowCount = 2
    sessionState.windowRevealRequest = {
      workspaceId: 'terminal1',
      windowId: 'terminal1-window-3',
      requestId: 1,
    }

    const { rerender } = render(<TerminalArea workspaceId="terminal1" />)

    expect(document.querySelector('.terminal-grid')).toHaveClass('grid-1')
    expect(viewControls().getByRole('button', { name: 'View window 1' })).toHaveClass('active')
    expect(viewControls().queryByRole('button', { name: 'View window 4' })).not.toBeInTheDocument()

    sessionState.windowCount = 4
    rerender(<TerminalArea workspaceId="terminal1" />)

    expect(viewControls().getByRole('button', { name: 'View window 4' })).toHaveClass('active')
    expect(screen.getByTestId('terminal-window-terminal1-window-3')).toHaveStyle({ display: 'flex' })
    expect(screen.getByTestId('terminal-window-terminal1-window-0')).toHaveStyle({ display: 'none' })
  })

  it('consumes two increasing matching requests while ignoring stale and other-workspace requests', () => {
    sessionState.isMobile = true
    sessionState.windowCount = 4

    const { rerender } = render(<TerminalArea workspaceId="terminal1" />)
    sessionState.windowRevealRequest = {
      workspaceId: 'terminal2',
      windowId: 'terminal2-window-3',
      requestId: 4,
    }
    rerender(<TerminalArea workspaceId="terminal1" />)
    expect(viewControls().getByRole('button', { name: 'View window 1' })).toHaveClass('active')

    sessionState.windowRevealRequest = {
      workspaceId: 'terminal1',
      windowId: 'terminal1-window-2',
      requestId: 5,
    }
    rerender(<TerminalArea workspaceId="terminal1" />)
    expect(viewControls().getByRole('button', { name: 'View window 3' })).toHaveClass('active')

    sessionState.windowRevealRequest = {
      workspaceId: 'terminal1',
      windowId: 'terminal1-window-0',
      requestId: 5,
    }
    rerender(<TerminalArea workspaceId="terminal1" />)
    expect(viewControls().getByRole('button', { name: 'View window 3' })).toHaveClass('active')

    sessionState.windowRevealRequest = {
      workspaceId: 'terminal1',
      windowId: 'terminal1-window-1',
      requestId: 3,
    }
    rerender(<TerminalArea workspaceId="terminal1" />)
    expect(viewControls().getByRole('button', { name: 'View window 3' })).toHaveClass('active')

    sessionState.windowRevealRequest = {
      workspaceId: 'terminal1',
      windowId: 'terminal1-window-1',
      requestId: 6,
    }
    rerender(<TerminalArea workspaceId="terminal1" />)
    expect(viewControls().getByRole('button', { name: 'View window 2' })).toHaveClass('active')

    sessionState.windowRevealRequest = {
      workspaceId: 'terminal2',
      windowId: 'terminal2-window-3',
      requestId: 7,
    }
    rerender(<TerminalArea workspaceId="terminal1" />)
    expect(viewControls().getByRole('button', { name: 'View window 2' })).toHaveClass('active')

    sessionState.windowRevealRequest = {
      workspaceId: 'terminal1',
      windowId: 'terminal1-window-0',
      requestId: 5,
    }
    rerender(<TerminalArea workspaceId="terminal1" />)
    expect(viewControls().getByRole('button', { name: 'View window 2' })).toHaveClass('active')
  })

  it('keeps desktop windows visible while consuming a matching reveal target for later mobile use', () => {
    sessionState.windowCount = 4
    sessionState.windowRevealRequest = {
      workspaceId: 'terminal1',
      windowId: 'terminal1-window-3',
      requestId: 8,
    }

    const { rerender } = render(<TerminalArea workspaceId="terminal1" />)
    expect(screen.getByTestId('terminal-window-terminal1-window-0')).toHaveStyle({ display: 'flex' })
    expect(screen.getByTestId('terminal-window-terminal1-window-3')).toHaveStyle({ display: 'flex' })

    sessionState.isMobile = true
    rerender(<TerminalArea workspaceId="terminal1" />)
    expect(viewControls().getByRole('button', { name: 'View window 4' })).toHaveClass('active')
    expect(screen.getByTestId('terminal-window-terminal1-window-3')).toHaveStyle({ display: 'flex' })
    expect(screen.getByTestId('terminal-window-terminal1-window-0')).toHaveStyle({ display: 'none' })
  })
})
