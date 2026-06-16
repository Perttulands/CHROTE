import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import MissionsView from './MissionsView'
import { ToastProvider } from '../context/ToastContext'

/* MissionsView ties the gallery to the open-board view (canvas + formations
   session side-panel). Opening a board mounts the cockpit + side-panel; "Mission
   Boards" returns to the gallery. The side-panel is scoped to the formations
   socket. */

vi.mock('./FormationsCockpit', () => ({
  __esModule: true,
  default: vi.fn(() => <div data-testid="formations-view">cockpit</div>),
}))

function mockResizeObserver() {
  class ResizeObserverMock {
    observe = vi.fn()
    unobserve = vi.fn()
    disconnect = vi.fn()
  }
  vi.stubGlobal('ResizeObserver', ResizeObserverMock)
}

const boards = [
  {
    id: 'brd_poems',
    slug: 'poems',
    title: 'Poems',
    rev: 3,
    etag: 'etag-poems',
    mission: { id: 'mis_1', title: 'Write a poem', goal: 'Ship a daily poem generator', beadId: '' },
    latestRun: null,
  },
]

function jsonResponse(body: unknown): Response {
  return {
    ok: true,
    headers: { get: () => null },
    json: () => Promise.resolve(body),
    text: () => Promise.resolve(''),
  } as unknown as Response
}

function renderView() {
  return render(
    <ToastProvider>
      <MissionsView active />
    </ToastProvider>
  )
}

describe('MissionsView', () => {
  const fetchMock = vi.fn()

  beforeEach(() => {
    fetchMock.mockReset()
    mockResizeObserver()
    fetchMock.mockImplementation((input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/formations/boards') return Promise.resolve(jsonResponse({ success: true, data: { boards } }))
      if (url === '/api/formations/tmux/sessions') {
        return Promise.resolve(jsonResponse({ sessions: [], grouped: {}, timestamp: '' }))
      }
      return Promise.resolve(jsonResponse({ success: true, data: {} }))
    })
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it('starts on the gallery and shows no side-panel', async () => {
    renderView()
    expect(await screen.findByTestId('missions-gallery')).toBeInTheDocument()
    expect(screen.queryByTestId('mission-session-panel')).toBeNull()
  })

  it('opens a board into the canvas + formations session side-panel and returns to the gallery', async () => {
    renderView()
    fireEvent.click(await screen.findByRole('button', { name: /Poems/ }))

    // Open-board view: cockpit canvas + the formations-scoped session panel.
    expect(await screen.findByTestId('formations-view')).toBeInTheDocument()
    expect(screen.getByTestId('mission-session-panel')).toBeInTheDocument()
    expect(screen.queryByTestId('missions-gallery')).toBeNull()

    // The side-panel must have listed sessions from the FORMATIONS socket only.
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/formations/tmux/sessions', expect.anything()))
    expect(fetchMock).not.toHaveBeenCalledWith('/api/tmux/sessions', expect.anything())

    // Back to the gallery.
    fireEvent.click(screen.getByRole('button', { name: /Mission Boards/ }))
    expect(await screen.findByTestId('missions-gallery')).toBeInTheDocument()
    expect(screen.queryByTestId('mission-session-panel')).toBeNull()
  })
})
