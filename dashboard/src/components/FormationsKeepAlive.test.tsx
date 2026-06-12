import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

/* Embedding contract (bead home-1xtz): the Formations cockpit mounts inside a
   position:relative host under the TabBar (so .fmx{position:absolute;inset:0}
   contains instead of overlaying the whole dashboard), and once visited it
   stays mounted across tab switches like the terminal tabs do. */

function mockMatchMedia(matches: boolean) {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches,
      media: query,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })),
  })
}

function mockResizeObserver() {
  class ResizeObserverMock {
    observe = vi.fn()
    unobserve = vi.fn()
    disconnect = vi.fn()
  }
  vi.stubGlobal('ResizeObserver', ResizeObserverMock)
}

function mockFetch() {
  ;(globalThis as Record<string, unknown>).fetch = vi.fn((input: RequestInfo | URL) => {
    const url = String(input)
    const respond = (data: unknown) => Promise.resolve({
      ok: true,
      headers: { get: () => null },
      json: () => Promise.resolve({ success: true, data }),
      text: () => Promise.resolve(''),
    })
    if (url === '/api/tmux/sessions') {
      return Promise.resolve({
        ok: true,
        headers: { get: () => null },
        json: () => Promise.resolve({ sessions: [], grouped: {}, timestamp: new Date().toISOString() }),
        text: () => Promise.resolve(''),
      })
    }
    if (url === '/api/formations/boards') return respond({ boards: [] })
    if (url === '/api/agents') return respond({ agents: [] })
    return respond({})
  }) as unknown as typeof fetch
}

describe('formations embedding under the TabBar', () => {
  beforeEach(() => {
    vi.resetModules()
    localStorage.clear()
    mockMatchMedia(false)
    mockResizeObserver()
    mockFetch()
  })

  it('hosts the cockpit in a positioned container and keeps it alive across tab switches', async () => {
    const [{ default: App }, { ToastProvider }] = await Promise.all([
      import('../App'),
      import('../context/ToastContext'),
    ])
    render(
      <ToastProvider>
        <App />
      </ToastProvider>
    )

    // Not mounted until first visited.
    expect(screen.queryByTestId('formations-host')).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: 'Formations' }))
    const host = await screen.findByTestId('formations-host')
    // position:relative is the containment contract for .fmx{position:absolute;inset:0}.
    expect(host.style.position).toBe('relative')
    expect(host.style.display).toBe('flex')
    await screen.findByTestId('formations-view')

    // Switching away hides but does NOT unmount (canvas/viewport state survives).
    fireEvent.click(screen.getByRole('button', { name: 'Settings' }))
    await waitFor(() => expect(screen.getByTestId('formations-host').style.display).toBe('none'))
    expect(screen.getByTestId('formations-view')).toBeInTheDocument()

    // Switching back shows the same mounted instance.
    fireEvent.click(screen.getByRole('button', { name: 'Formations' }))
    await waitFor(() => expect(screen.getByTestId('formations-host').style.display).toBe('flex'))
  })
})
