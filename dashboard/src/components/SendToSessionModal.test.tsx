import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ReactElement } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { TmuxSession } from '../types'
import SendToSessionModal from './SendToSessionModal'

const mockState = vi.hoisted(() => ({
  sendToSessionTarget: 'alice:alice-shell' as string | null,
  sessions: [
    { name: 'alice-shell', windows: 1, attached: true, group: 'main', unixUser: 'alice' },
    { name: 'bob-shell', windows: 1, attached: true, group: 'main', unixUser: 'bob' },
    { name: 'codex-agent', windows: 1, attached: true, group: 'codex', unixUser: 'alice', persistent: true, persistentAgentKind: 'codex' },
    { name: 'hq-mayor', windows: 1, attached: true, group: 'hq', unixUser: 'alice' },
  ] as TmuxSession[],
  closeSendToSession: vi.fn(),
  sendToSession: vi.fn(),
}))

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({
    sendToSessionTarget: mockState.sendToSessionTarget,
    sessions: mockState.sessions,
    closeSendToSession: mockState.closeSendToSession,
    sendToSession: mockState.sendToSession,
  }),
}))

function setTarget(target: string | null, rerender: (ui: ReactElement) => void) {
  mockState.sendToSessionTarget = target
  rerender(<SendToSessionModal />)
}

function fileInput(container: HTMLElement): HTMLInputElement {
  const input = container.querySelector<HTMLInputElement>('.send-session-file-input')
  if (!input) throw new Error('Send to Session file input is missing')
  return input
}

function setStaleFileInputValue(input: HTMLInputElement, value: string) {
  Object.defineProperty(input, 'value', {
    configurable: true,
    writable: true,
    value,
  })
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>(next => { resolve = next })
  return { promise, resolve }
}

describe('SendToSessionModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockState.sendToSessionTarget = 'alice:alice-shell'
    mockState.sendToSession.mockResolvedValue(true)
  })

  it('sends editable text and dropped files to the selected session', async () => {
    const file = new File(['hello'], 'notes.txt', { type: 'text/plain' })
    render(<SendToSessionModal />)

    expect(screen.getByRole('heading', { name: /Send to Session: alice-shell/i })).toBeInTheDocument()
    expect(screen.getByText(/stored on disk until removed/i)).toBeInTheDocument()
    await waitFor(() => expect(screen.getByRole('checkbox', { name: /Press Enter after sending/i })).not.toBeChecked())

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
      submit: false,
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

  it('clears the native file input so the same file can be selected again', () => {
    const file = new File(['again'], 'repeat.txt', { type: 'text/plain' })
    const { container } = render(<SendToSessionModal />)
    const input = fileInput(container)

    setStaleFileInputValue(input, 'C:\\fakepath\\repeat.txt')
    fireEvent.change(input, { target: { files: [file] } })
    expect(input).toHaveValue('')

    setStaleFileInputValue(input, 'C:\\fakepath\\repeat.txt')
    fireEvent.change(input, { target: { files: [file] } })
    expect(input).toHaveValue('')
    expect(screen.getAllByText('repeat.txt')).toHaveLength(2)
  })

  it('starts clean when the same target is closed and reopened', async () => {
    const file = new File(['draft'], 'draft.txt', { type: 'text/plain' })
    const { container, rerender } = render(<SendToSessionModal />)

    fireEvent.change(screen.getByLabelText(/Message to send/i), { target: { value: 'unsent draft' } })
    fireEvent.change(fileInput(container), { target: { files: [file] } })
    fireEvent.click(screen.getByRole('checkbox', { name: /Press Enter after sending/i }))
    fireEvent.click(screen.getByRole('button', { name: /Cancel/i }))
    expect(mockState.closeSendToSession).toHaveBeenCalledOnce()

    setTarget(null, rerender)
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    setTarget('alice:alice-shell', rerender)

    await waitFor(() => {
      expect(screen.getByLabelText(/Message to send/i)).toHaveValue('')
      expect(screen.queryByText('draft.txt')).not.toBeInTheDocument()
      expect(screen.getByRole('checkbox', { name: /Press Enter after sending/i })).not.toBeChecked()
    })
  })

  it('does not leak text, files, native file input, or sending state when switching targets', async () => {
    const pending = deferred<boolean>()
    const file = new File(['draft'], 'target-a.txt', { type: 'text/plain' })
    mockState.sendToSession.mockReturnValueOnce(pending.promise)
    const { container, rerender } = render(<SendToSessionModal />)

    fireEvent.change(screen.getByLabelText(/Message to send/i), { target: { value: 'for target A' } })
    const input = fileInput(container)
    fireEvent.change(input, { target: { files: [file] } })
    setStaleFileInputValue(input, 'C:\\fakepath\\target-a.txt')
    fireEvent.click(screen.getByRole('button', { name: /^Send$/i }))
    expect(screen.getByRole('button', { name: /Sending/i })).toBeDisabled()

    setTarget('bob:bob-shell', rerender)

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /Send to Session: bob-shell/i })).toBeInTheDocument()
      expect(screen.getByLabelText(/Message to send/i)).toHaveValue('')
      expect(screen.queryByText('target-a.txt')).not.toBeInTheDocument()
      expect(fileInput(container)).toHaveValue('')
      expect(screen.getByRole('button', { name: /^Send$/i })).toBeDisabled()
    })

    await act(async () => pending.resolve(false))
  })

  it('retains the current target draft after a failed send so it can be retried', async () => {
    const file = new File(['retry'], 'retry.txt', { type: 'text/plain' })
    mockState.sendToSession.mockResolvedValueOnce(false)
    render(<SendToSessionModal />)

    fireEvent.change(screen.getByLabelText(/Message to send/i), { target: { value: 'retry this draft' } })
    fireEvent.drop(screen.getByLabelText(/Drop files or paste images/i), {
      dataTransfer: { files: [file] },
    })
    fireEvent.click(screen.getByRole('checkbox', { name: /Press Enter after sending/i }))
    fireEvent.click(screen.getByRole('button', { name: /^Send$/i }))

    await waitFor(() => expect(screen.getByRole('button', { name: /^Send$/i })).toBeEnabled())
    expect(mockState.closeSendToSession).not.toHaveBeenCalled()
    expect(screen.getByLabelText(/Message to send/i)).toHaveValue('retry this draft')
    expect(screen.getByText('retry.txt')).toBeInTheDocument()
    expect(screen.getByRole('checkbox', { name: /Press Enter after sending/i })).toBeChecked()
  })

  it('closes after a successful send and starts clean on the next open', async () => {
    const file = new File(['sent'], 'sent.txt', { type: 'text/plain' })
    const { rerender } = render(<SendToSessionModal />)

    fireEvent.change(screen.getByLabelText(/Message to send/i), { target: { value: 'send once' } })
    fireEvent.drop(screen.getByLabelText(/Drop files or paste images/i), {
      dataTransfer: { files: [file] },
    })
    fireEvent.click(screen.getByRole('button', { name: /^Send$/i }))
    await waitFor(() => expect(mockState.closeSendToSession).toHaveBeenCalledOnce())

    setTarget(null, rerender)
    setTarget('alice:alice-shell', rerender)

    await waitFor(() => {
      expect(screen.getByLabelText(/Message to send/i)).toHaveValue('')
      expect(screen.queryByText('sent.txt')).not.toBeInTheDocument()
      expect(screen.getByRole('checkbox', { name: /Press Enter after sending/i })).not.toBeChecked()
    })
  })

  it('defaults submit on only for resolved agent targets, never unknown or plain shells', async () => {
    const { rerender } = render(<SendToSessionModal />)
    const submit = () => screen.getByRole('checkbox', { name: /Press Enter after sending/i })

    await waitFor(() => expect(submit()).not.toBeChecked())

    setTarget('alice:codex-agent', rerender)
    await waitFor(() => expect(submit()).toBeChecked())

    setTarget('alice:hq-mayor', rerender)
    await waitFor(() => expect(submit()).toBeChecked())

    setTarget('bob:hq-mayor', rerender)
    await waitFor(() => expect(submit()).not.toBeChecked())
  })

  it('ignores a successful stale send after the same target is reopened and sending a newer draft', async () => {
    const staleSend = deferred<boolean>()
    const currentSend = deferred<boolean>()
    mockState.sendToSession
      .mockReturnValueOnce(staleSend.promise)
      .mockReturnValueOnce(currentSend.promise)
    const { rerender } = render(<SendToSessionModal />)

    fireEvent.change(screen.getByLabelText(/Message to send/i), { target: { value: 'stale draft' } })
    fireEvent.click(screen.getByRole('button', { name: /^Send$/i }))
    expect(screen.getByRole('button', { name: /Sending/i })).toBeDisabled()

    setTarget(null, rerender)
    setTarget('alice:alice-shell', rerender)
    await waitFor(() => expect(screen.getByLabelText(/Message to send/i)).toHaveValue(''))

    fireEvent.change(screen.getByLabelText(/Message to send/i), { target: { value: 'current draft' } })
    fireEvent.click(screen.getByRole('button', { name: /^Send$/i }))
    expect(screen.getByRole('button', { name: /Sending/i })).toBeDisabled()

    await act(async () => staleSend.resolve(true))

    expect(mockState.closeSendToSession).not.toHaveBeenCalled()
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByLabelText(/Message to send/i)).toHaveValue('current draft')
    expect(screen.getByRole('button', { name: /Sending/i })).toBeDisabled()

    await act(async () => currentSend.resolve(false))
  })

  it('updates an untouched submit default when exact-target metadata arrives without clearing the draft', async () => {
    const file = new File(['late'], 'late.txt', { type: 'text/plain' })
    mockState.sendToSessionTarget = 'alice:late-shell'
    const { rerender } = render(<SendToSessionModal />)
    const submit = () => screen.getByRole('checkbox', { name: /Press Enter after sending/i })

    expect(submit()).not.toBeChecked()
    fireEvent.change(screen.getByLabelText(/Message to send/i), { target: { value: 'keep this draft' } })
    fireEvent.drop(screen.getByLabelText(/Drop files or paste images/i), {
      dataTransfer: { files: [file] },
    })

    mockState.sessions = [
      ...mockState.sessions,
      { name: 'late-shell', windows: 1, attached: true, group: 'late', unixUser: 'alice', persistent: true },
    ]
    rerender(<SendToSessionModal />)

    await waitFor(() => expect(submit()).toBeChecked())
    expect(screen.getByLabelText(/Message to send/i)).toHaveValue('keep this draft')
    expect(screen.getByText('late.txt')).toBeInTheDocument()
  })

  it('does not overwrite an explicit submit choice when session metadata refreshes', async () => {
    mockState.sendToSessionTarget = 'alice:refresh-shell'
    mockState.sessions = [
      ...mockState.sessions,
      { name: 'refresh-shell', windows: 1, attached: true, group: 'refresh', unixUser: 'alice', persistent: true },
    ]
    const { rerender } = render(<SendToSessionModal />)
    const submit = () => screen.getByRole('checkbox', { name: /Press Enter after sending/i })

    await waitFor(() => expect(submit()).toBeChecked())
    fireEvent.click(submit())
    expect(submit()).not.toBeChecked()

    mockState.sessions = mockState.sessions.map(session => (
      session.name === 'refresh-shell' && session.unixUser === 'alice'
        ? { ...session, windows: 2 }
        : session
    ))
    rerender(<SendToSessionModal />)

    await waitFor(() => expect(submit()).not.toBeChecked())
  })
})
