import { render, screen, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import TerminalArea from './TerminalArea'

const setWindowCount = vi.fn()
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
    windowRevealRequest: sessionState.windowRevealRequest,
  }),
}))

vi.mock('../hooks/useMediaQuery', () => ({
  useMediaQuery: () => sessionState.isMobile,
}))

vi.mock('./TerminalWindow', () => ({
  CLAIM_EXPLANATION: 'Set the tmux window to this device\'s size. Other devices keep watching, at that size.',
  default: ({ window, style }: { window: { id: string }, style?: React.CSSProperties }) => (
    <div className="terminal-window" data-testid={`terminal-window-${window.id}`} style={style} />
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

  // The strip states the layout and names the chords that change it. Nothing
  // on it is pressable any more: the counts, Refit and the overflow menu are
  // gone, and the tab's own menu holds what survived.
  it('states the layout and its chords instead of offering buttons', () => {
    const { container } = render(<TerminalArea workspaceId="terminal1" />)

    const controls = screen.getByLabelText('Terminal workspace controls')
    expect(controls).toHaveTextContent('Layout2')
    expect(controls).toHaveTextContent('Alt+= add window · Alt+- remove empty')
    expect(within(controls).queryAllByRole('button')).toHaveLength(0)
    expect(screen.queryByRole('button', { name: 'Terminal maintenance actions' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Refit/ })).not.toBeInTheDocument()

    expect(container.querySelector('.terminal-grid')).toHaveClass('grid-2')
    expect(container.querySelectorAll('[data-testid^="terminal-window-"]')).toHaveLength(2)
  })

  // No drag is in the air in a rendered frame, so no seam is either: the drop
  // zone exists only for the gesture that can use it.
  it('draws no seam between tiles while nothing is being dragged', () => {
    const { container } = render(<TerminalArea workspaceId="terminal1" />)

    expect(container.querySelectorAll('.terminal-window-gap')).toHaveLength(0)
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

  it('takes every increasing matching reveal request, on any viewport, and ignores the rest', () => {
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

    // On a desktop every window is on screen already, so a request changes
    // nothing visible. It is still taken, and read once the viewport narrows.
    sessionState.isMobile = false
    sessionState.windowRevealRequest = {
      workspaceId: 'terminal1',
      windowId: 'terminal1-window-3',
      requestId: 8,
    }
    rerender(<TerminalArea workspaceId="terminal1" />)
    expect(screen.getByTestId('terminal-window-terminal1-window-0')).toHaveStyle({ display: 'flex' })
    expect(screen.getByTestId('terminal-window-terminal1-window-3')).toHaveStyle({ display: 'flex' })

    sessionState.isMobile = true
    rerender(<TerminalArea workspaceId="terminal1" />)
    expect(viewControls().getByRole('button', { name: 'View window 4' })).toHaveClass('active')
    expect(screen.getByTestId('terminal-window-terminal1-window-3')).toHaveStyle({ display: 'flex' })
    expect(screen.getByTestId('terminal-window-terminal1-window-0')).toHaveStyle({ display: 'none' })
  })
})
