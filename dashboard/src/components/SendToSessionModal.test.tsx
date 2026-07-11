import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SendToSessionModal from './SendToSessionModal'

const mockState = vi.hoisted(() => ({
  closeSendToSession: vi.fn(),
  sendToSession: vi.fn(),
}))

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({
    sendToSessionTarget: 'alice:alice-shell',
    sessions: [
      { name: 'alice-shell', windows: 1, attached: true, group: 'main', unixUser: 'alice' },
    ],
    closeSendToSession: mockState.closeSendToSession,
    sendToSession: mockState.sendToSession,
  }),
}))

describe('SendToSessionModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockState.sendToSession.mockResolvedValue(true)
  })

  it('sends editable text and dropped files to the selected session', async () => {
    const file = new File(['hello'], 'notes.txt', { type: 'text/plain' })
    render(<SendToSessionModal />)

    expect(screen.getByRole('heading', { name: /Send to Session: alice-shell/i })).toBeInTheDocument()
    expect(screen.getByText(/stored on disk until removed/i)).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/Message to send/i), {
      target: { value: 'Please inspect this.' },
    })
    fireEvent.drop(screen.getByLabelText(/Drop files or paste images/i), {
      dataTransfer: { files: [file] },
    })

    expect(screen.getByText('notes.txt')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /^Send$/i }))

    await waitFor(() => expect(mockState.sendToSession).toHaveBeenCalled())
    expect(mockState.sendToSession).toHaveBeenCalledWith('alice-shell', {
      text: 'Please inspect this.',
      files: [file],
      submit: true,
    }, 'alice')
    expect(mockState.closeSendToSession).toHaveBeenCalled()
  })

  it('adds pasted clipboard images without touching terminal paste handling', () => {
    const image = new File(['png'], 'clipboard.png', { type: 'image/png' })
    render(<SendToSessionModal />)

    fireEvent.paste(screen.getByLabelText(/Drop files or paste images/i), {
      clipboardData: { files: [image] },
    })

    expect(screen.getByText('clipboard.png')).toBeInTheDocument()
  })
})
