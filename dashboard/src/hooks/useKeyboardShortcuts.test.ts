import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { useKeyboardShortcuts } from './useKeyboardShortcuts'

const closeFloatingModal = vi.fn()
const mockUseSession = vi.fn()

vi.mock('../context/SessionContext', () => ({
  useSession: () => mockUseSession(),
}))

function mount(onShowKeys = vi.fn()) {
  renderHook(() => useKeyboardShortcuts({ onShowKeys, isKeysPanelOpen: false }))
  return onShowKeys
}

describe('useKeyboardShortcuts', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    document.body.innerHTML = ''
    mockUseSession.mockReturnValue({ floatingSession: null, closeFloatingModal })
  })

  it('leaves browser and terminal shortcuts alone', () => {
    mount()

    for (const event of [
      new KeyboardEvent('keydown', { key: 'Tab', cancelable: true }),
      new KeyboardEvent('keydown', { key: 's', ctrlKey: true, cancelable: true }),
      new KeyboardEvent('keydown', { key: 'n', ctrlKey: true, cancelable: true }),
      new KeyboardEvent('keydown', { key: '1', ctrlKey: true, cancelable: true }),
      new KeyboardEvent('keydown', { key: 'ArrowRight', ctrlKey: true, cancelable: true }),
      new KeyboardEvent('keydown', { key: '/', cancelable: true }),
    ]) {
      window.dispatchEvent(event)
      expect(event.defaultPrevented).toBe(false)
    }
  })

  it('opens the keys panel and focuses search only from dashboard chrome', () => {
    const onShowKeys = mount()
    const dock = document.createElement('div')
    dock.className = 'terminal-workspace-dock'
    dock.dataset.active = 'true'
    const search = document.createElement('input')
    search.className = 'session-search-input'
    dock.appendChild(search)
    document.body.appendChild(dock)

    document.body.dispatchEvent(new KeyboardEvent('keydown', { key: '?', bubbles: true, cancelable: true }))
    expect(onShowKeys).toHaveBeenCalledTimes(1)

    document.body.dispatchEvent(new KeyboardEvent('keydown', { key: '/', bubbles: true, cancelable: true }))
    expect(search).toHaveFocus()

    const terminalBody = document.createElement('div')
    terminalBody.className = 'terminal-window-body'
    document.body.appendChild(terminalBody)
    terminalBody.dispatchEvent(new KeyboardEvent('keydown', { key: '?', bubbles: true, cancelable: true }))
    expect(onShowKeys).toHaveBeenCalledTimes(1)
  })

  it('opens the active Sessions sidecar and focuses its search when slash is pressed while closed', async () => {
    mount()
    const dock = document.createElement('div')
    dock.className = 'terminal-workspace-dock'
    dock.dataset.active = 'true'
    const trigger = document.createElement('button')
    trigger.setAttribute('aria-label', 'Sessions sidecar')
    trigger.setAttribute('aria-expanded', 'false')
    const click = vi.fn(() => {
      trigger.setAttribute('aria-expanded', 'true')
      const search = document.createElement('input')
      search.className = 'session-search-input'
      dock.appendChild(search)
    })
    trigger.addEventListener('click', click)
    dock.appendChild(trigger)
    document.body.appendChild(dock)

    const slash = new KeyboardEvent('keydown', { key: '/', bubbles: true, cancelable: true })
    document.body.dispatchEvent(slash)

    expect(slash.defaultPrevented).toBe(true)
    expect(click).toHaveBeenCalledTimes(1)
    await waitFor(() => expect(dock.querySelector('.session-search-input')).toHaveFocus())
  })

  it('closes Peek on Escape without handling other keys while typing', () => {
    mockUseSession.mockReturnValue({ floatingSession: 'alice:shell', closeFloatingModal })
    const onShowKeys = mount()
    const input = document.createElement('input')
    document.body.appendChild(input)

    input.dispatchEvent(new KeyboardEvent('keydown', { key: '?', bubbles: true, cancelable: true }))
    expect(onShowKeys).not.toHaveBeenCalled()

    input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }))
    expect(closeFloatingModal).toHaveBeenCalledTimes(1)
  })

  it('leaves Escape to the shell when it is typed into a terminal', () => {
    mockUseSession.mockReturnValue({ floatingSession: 'alice:shell', closeFloatingModal })
    mount()
    const peekBody = document.createElement('div')
    peekBody.className = 'peek-body'
    const terminalInput = document.createElement('textarea')
    terminalInput.className = 'xterm-helper-textarea'
    peekBody.appendChild(terminalInput)
    document.body.appendChild(peekBody)

    terminalInput.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }))

    expect(closeFloatingModal).not.toHaveBeenCalled()
  })
})
