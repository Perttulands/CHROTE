import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import App from '../App'
import { ToastProvider } from '../context/ToastContext'

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

function mockFetch() {
  vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL) => {
    const url = String(input)
	    if (url === '/api/tmux/sessions') {
	      return Promise.resolve({
	        ok: true,
	        json: () => Promise.resolve({ sessions: [], grouped: {}, timestamp: new Date().toISOString() }),
	        text: () => Promise.resolve(''),
	      })
	    }
	    if (url === '/api/agents') {
	      return Promise.resolve({
	        ok: true,
	        json: () => Promise.resolve({
	          success: true,
	          data: {
	            agents: [{ id: 'susie', displayName: 'Susie', kind: 'specialist', tags: ['react'], liveness: 'offline', assignable: true }],
	            count: 1,
	          },
	        }),
	        text: () => Promise.resolve(''),
	      })
	    }
    return Promise.resolve({
      ok: true,
      json: () => Promise.resolve({}),
      text: () => Promise.resolve(''),
    })
  }) as any)
}

function mockResizeObserver() {
  class ResizeObserverMock {
    observe = vi.fn()
    unobserve = vi.fn()
    disconnect = vi.fn()
  }

  vi.stubGlobal('ResizeObserver', ResizeObserverMock)
}

function mockEventSource() {
  class EventSourceMock {
    onerror: (() => void) | null = null
    addEventListener = vi.fn()
    close = vi.fn()
  }

  vi.stubGlobal('EventSource', EventSourceMock)
}

function renderDashboard() {
  return render(
    <ToastProvider>
      <App />
    </ToastProvider>
  )
}

describe('Formations tab feature flag', () => {
  beforeEach(() => {
    localStorage.clear()
    mockMatchMedia(false)
    mockFetch()
    mockResizeObserver()
    mockEventSource()
  })

  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it('does not render the tab or mount Formations when the flag is unset', () => {
    renderDashboard()

    expect(screen.queryByRole('button', { name: 'Formations' })).not.toBeInTheDocument()
    expect(screen.queryByTestId('formations-view')).not.toBeInTheDocument()
  })

  it('renders the tab and mounts Formations only after opt-in and selection', async () => {
    localStorage.setItem('chrote-formations', '1')
    renderDashboard()

    expect(screen.queryByTestId('formations-view')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Formations' }))

    expect(await screen.findByTestId('formations-view')).toBeInTheDocument()
  })

	  it('drops the active Formations tab after the flag is disabled', async () => {
	    localStorage.setItem('chrote-formations', '1')
	    renderDashboard()

    fireEvent.click(screen.getByRole('button', { name: 'Formations' }))
    expect(await screen.findByTestId('formations-view')).toBeInTheDocument()

    localStorage.setItem('chrote-formations', '0')
    fireEvent.click(screen.getByTitle('Layout Presets'))

	    await waitFor(() => {
	      expect(screen.queryByTestId('formations-view')).not.toBeInTheDocument()
	      expect(screen.queryByRole('button', { name: 'Formations' })).not.toBeInTheDocument()
	      expect(screen.getByRole('button', { name: 'Terminal' })).toHaveClass('active')
	    })
	  })

	  it('mounts the Formations Agents surface instead of Oracle only when opted in', async () => {
	    localStorage.setItem('chrote-formations', '1')
	    renderDashboard()

	    fireEvent.click(screen.getByRole('button', { name: 'Agents' }))

	    expect(await screen.findByTestId('agents-view')).toBeInTheDocument()
	    expect(await screen.findByText('Susie')).toBeInTheDocument()
	  })
	})
