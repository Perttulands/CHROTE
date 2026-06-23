import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import SessionItem from './SessionItem'
import { DEFAULT_SETTINGS } from '../types'

const mockState = vi.hoisted(() => ({
  assignedSessions: new Map<string, { workspaceId: string; windowId: string; windowIndex: number; colorIndex: number }>(),
  openFloatingModal: vi.fn(),
  addSessionToWindow: vi.fn(),
}))

vi.mock('@dnd-kit/core', () => ({
  useDraggable: () => ({
    attributes: {},
    listeners: {},
    setNodeRef: vi.fn(),
    transform: null,
    isDragging: false,
  }),
}))

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({
    assignedSessions: mockState.assignedSessions,
    handleSessionClick: vi.fn(),
    deleteSession: vi.fn(),
    renameSession: vi.fn(),
    workspaces: {
      terminal1: { windows: [{ id: 'terminal1-window-0', colorIndex: 0 }], windowCount: 1 },
      terminal2: { windows: [], windowCount: 0 },
      terminal3: { windows: [], windowCount: 0 },
    },
    addSessionToWindow: mockState.addSessionToWindow,
    removeSessionFromWindow: vi.fn(),
    openFloatingModal: mockState.openFloatingModal,
    settings: {
      ...DEFAULT_SETTINGS,
      terminalUserColors: {
        alice: '#123456',
      },
    },
  }),
}))

vi.mock('./RoleBadge', () => ({
  default: () => null,
}))

describe('SessionItem user badge and context actions', () => {
  afterEach(() => {
    mockState.assignedSessions.clear()
    mockState.openFloatingModal.mockClear()
    mockState.addSessionToWindow.mockClear()
  })

  it('renders a configured Unix user indicator from session.unixUser', () => {
    render(
      <SessionItem
        session={{
          name: 'alice-shell',
          windows: 1,
          attached: false,
          group: 'main',
          unixUser: 'alice',
        }}
      />
    )

    const badge = screen.getByLabelText('Unix user alice')
    expect(badge).toHaveTextContent('A')
    expect(badge).toHaveAttribute('title', 'Unix user: alice')
    expect(badge).toHaveStyle({ backgroundColor: '#123456' })
  })

  it('places the Unix user badge before the attached terminal/window badge', () => {
    mockState.assignedSessions.set('alice-shell', {
      workspaceId: 'terminal1',
      windowId: 'window-1',
      windowIndex: 1,
      colorIndex: 0,
    })

    const { container } = render(
      <SessionItem
        session={{
          name: 'alice-shell',
          windows: 1,
          attached: true,
          group: 'main',
          unixUser: 'alice',
        }}
      />
    )

    const rowText = Array.from(container.querySelector('.session-item')?.children ?? [])
      .map(child => child.textContent)

    expect(rowText.slice(0, 3)).toEqual(['A', 'T1 W1', 'alice-shell'])
  })

  it('offers peek and attach actions in the session context menu', () => {
    render(
      <SessionItem
        session={{
          name: 'alice-shell',
          windows: 1,
          attached: false,
          group: 'main',
          unixUser: 'alice',
        }}
      />
    )

    fireEvent.contextMenu(screen.getByText('alice-shell'))
    fireEvent.click(screen.getByRole('button', { name: /Peek/i }))
    expect(mockState.openFloatingModal).toHaveBeenCalledWith('alice:alice-shell')

    fireEvent.contextMenu(screen.getByText('alice-shell'))
    expect(screen.getByText(/Attach to Window/i)).toBeInTheDocument()
    fireEvent.mouseEnter(screen.getByText(/Attach to Window/i))
    fireEvent.click(screen.getByRole('button', { name: /Window 1/i }))
    expect(mockState.addSessionToWindow).toHaveBeenCalledWith('terminal1', 'terminal1-window-0', 'alice-shell', 'alice')
  })
})
