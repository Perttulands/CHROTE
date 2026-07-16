import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { useKeyboardShortcuts } from './useKeyboardShortcuts'

const closeFloatingModal = vi.fn()
const mockUseSession = vi.fn()

vi.mock('../context/SessionContext', () => ({
  useSession: () => mockUseSession(),
}))

function mount(onShowHelp = vi.fn()) {
  renderHook(() => useKeyboardShortcuts({ onShowHelp, isHelpOpen: false }))
  return onShowHelp
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

  it('shows help and focuses search only from dashboard chrome', () => {
    const onShowHelp = mount()
    const dock = document.createElement('div')
    dock.className = 'terminal-workspace-dock'
    dock.dataset.active = 'true'
    const search = document.createElement('input')
    search.className = 'session-search-input'
    dock.appendChild(search)
    document.body.appendChild(dock)

    document.body.dispatchEvent(new KeyboardEvent('keydown', { key: '?', bubbles: true, cancelable: true }))
    expect(onShowHelp).toHaveBeenCalledTimes(1)

    document.body.dispatchEvent(new KeyboardEvent('keydown', { key: '/', bubbles: true, cancelable: true }))
    expect(search).toHaveFocus()

    const terminalBody = document.createElement('div')
    terminalBody.className = 'terminal-window-body'
    document.body.appendChild(terminalBody)
    terminalBody.dispatchEvent(new KeyboardEvent('keydown', { key: '?', bubbles: true, cancelable: true }))
    expect(onShowHelp).toHaveBeenCalledTimes(1)
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
    const onShowHelp = mount()
    const input = document.createElement('input')
    document.body.appendChild(input)

    input.dispatchEvent(new KeyboardEvent('keydown', { key: '?', bubbles: true, cancelable: true }))
    expect(onShowHelp).not.toHaveBeenCalled()

    input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }))
    expect(closeFloatingModal).toHaveBeenCalledTimes(1)
  })
})
