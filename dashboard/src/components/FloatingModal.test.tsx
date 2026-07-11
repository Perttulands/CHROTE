import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import FloatingModal from './FloatingModal'

const mockState = vi.hoisted(() => ({
  closeFloatingModal: vi.fn(),
  openSendToSession: vi.fn(),
}))

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({
    floatingSession: 'alice:alice-shell',
    closeFloatingModal: mockState.closeFloatingModal,
    openSendToSession: mockState.openSendToSession,
    settings: { fontSize: 14 },
    sessions: [{ name: 'alice-shell', windows: 1, attached: true, group: 'main', unixUser: 'alice' }],
  }),
}))

describe('FloatingModal Send to Session action', () => {
  beforeEach(() => vi.clearAllMocks())

  it('offers Send to Session from the peek panel without closing the terminal peek', () => {
    vi.stubGlobal('ResizeObserver', class {
      observe() {}
      disconnect() {}
    })
    render(<FloatingModal />)

    fireEvent.click(screen.getByRole('button', { name: /Send to Session/i }))

    expect(mockState.openSendToSession).toHaveBeenCalledWith('alice:alice-shell')
    expect(mockState.closeFloatingModal).not.toHaveBeenCalled()
  })
})
