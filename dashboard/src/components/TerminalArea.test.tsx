import { fireEvent, render, screen, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import TerminalArea from './TerminalArea'

const setWindowCount = vi.fn()
const clearStaleSessionsFromWindow = vi.fn()
const reconnectIframe = vi.fn()

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({
    sessions: [
      { name: 'alpha', windows: 1, attached: false, group: 'shell', unixUser: 'alice' },
    ],
    workspaces: {
      terminal1: {
        windowCount: 2,
        windows: [
          { id: 'terminal1-window-0', boundSessions: ['alice:alpha', 'bare-session'], activeSession: 'alice:alpha', colorIndex: 0 },
          { id: 'terminal1-window-1', boundSessions: ['bob:beta'], activeSession: 'bob:beta', colorIndex: 1 },
        ],
      },
    },
    setWindowCount,
    clearStaleSessionsFromWindow,
    isDragging: false,
  }),
}))

vi.mock('../hooks/useMediaQuery', () => ({
  useMediaQuery: () => false,
}))

vi.mock('./IframePool', () => ({
  useIframePool: () => ({ reconnectIframe }),
}))

vi.mock('./TerminalWindow', () => ({
  default: ({ window, refitNonce }: { window: { id: string }, refitNonce: number }) => (
    <div data-testid={`terminal-window-${window.id}`} data-refit-nonce={refitNonce} />
  ),
}))

describe('TerminalArea layout controls context menu', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('reconnects all visible session frames from the layout controls menu', () => {
    render(<TerminalArea workspaceId="terminal1" />)

    fireEvent.contextMenu(screen.getByLabelText('Terminal layout controls'))
    fireEvent.click(screen.getByRole('button', { name: /Reconnect frames/i }))

    expect(reconnectIframe).toHaveBeenCalledWith('alice:alpha')
    expect(reconnectIframe).toHaveBeenCalledWith('bare-session')
    expect(reconnectIframe).toHaveBeenCalledWith('bob:beta')
  })

  it('clears stale sessions and refits layout from the layout controls menu', () => {
    render(<TerminalArea workspaceId="terminal1" />)

    fireEvent.click(screen.getByRole('button', { name: /Clean 2 stale sessions/i }))

    expect(clearStaleSessionsFromWindow).toHaveBeenCalledWith('terminal1', 'terminal1-window-0')
    expect(clearStaleSessionsFromWindow).toHaveBeenCalledWith('terminal1', 'terminal1-window-1')
    clearStaleSessionsFromWindow.mockClear()

    fireEvent.contextMenu(screen.getByLabelText('Terminal layout controls'))
    fireEvent.click(screen.getByRole('button', { name: /Clear stale sessions/i }))

    expect(clearStaleSessionsFromWindow).toHaveBeenCalledWith('terminal1', 'terminal1-window-0')
    expect(clearStaleSessionsFromWindow).toHaveBeenCalledWith('terminal1', 'terminal1-window-1')

    fireEvent.contextMenu(screen.getByLabelText('Terminal layout controls'))
    const menu = document.querySelector('.session-context-menu') as HTMLElement
    fireEvent.click(within(menu).getByRole('button', { name: /Refit terminal layout/i }))

    expect(screen.getByTestId('terminal-window-terminal1-window-0')).toHaveAttribute('data-refit-nonce', '1')
    expect(screen.getByTestId('terminal-window-terminal1-window-1')).toHaveAttribute('data-refit-nonce', '1')
  })
})
