import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import FloatingModal from './FloatingModal'
import { FakeSocket } from '../test/fakeWebSocket'

const mockState = vi.hoisted(() => ({
  closeFloatingModal: vi.fn(),
  openSendToSession: vi.fn(),
}))

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({
    floatingSession: 'alice:alice-shell',
    closeFloatingModal: mockState.closeFloatingModal,
    openSendToSession: mockState.openSendToSession,
    settings: { fontSize: 14, hideScrollbar: false },
    sessions: [{ name: 'alice-shell', windows: 1, attached: true, group: 'main', unixUser: 'alice' }],
  }),
}))

describe('FloatingModal Send to Session action', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    FakeSocket.instances = []
    vi.stubGlobal('WebSocket', FakeSocket)
    vi.stubGlobal('ResizeObserver', class {
      observe() {}
      disconnect() {}
    })
  })

  it('offers Send to Session from the peek panel without closing the terminal peek', () => {
    render(<FloatingModal />)

    fireEvent.click(screen.getByRole('button', { name: /Send to Session/i }))

    expect(mockState.openSendToSession).toHaveBeenCalledWith('alice:alice-shell')
    expect(mockState.closeFloatingModal).not.toHaveBeenCalled()
  })

  it('uses truthful loading text, responsive geometry, and does not drag from header controls', () => {
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 640 })
    Object.defineProperty(window, 'innerHeight', { configurable: true, value: 480 })
    const { container } = render(<FloatingModal />)

    const modal = container.querySelector('.floating-modal') as HTMLElement
    expect(screen.getByText('Loading terminal…')).toBeInTheDocument()
    expect(Number.parseInt(modal.style.width, 10)).toBeLessThanOrEqual(608)
    expect(Number.parseInt(modal.style.height, 10)).toBeLessThanOrEqual(448)

    const initialLeft = modal.style.left
    fireEvent.mouseDown(screen.getByRole('button', { name: /Send to Session/i }), { clientX: 20, clientY: 20 })
    fireEvent.mouseMove(document, { clientX: 300, clientY: 300 })
    expect(modal.style.left).toBe(initialLeft)
  })

  it('peeks through the same in-page terminal as a tile, on its own connection', () => {
    const { unmount } = render(<FloatingModal />)

    expect(document.querySelector('.terminal-surface-host .terminal-surface')).not.toBeNull()
    expect(FakeSocket.latest().url).toContain('/terminal/ws?arg=alice-shell&arg=alice')

    unmount()

    expect(FakeSocket.latest().readyState).toBe(FakeSocket.CLOSED)
  })
})
