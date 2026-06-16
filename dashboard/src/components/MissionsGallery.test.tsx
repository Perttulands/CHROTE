import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import MissionsGallery from './MissionsGallery'

/* The Missions tab landing view: a gallery of Mission Boards. Each card shows the
   board's mission goal, latest-run status, and bead anchor; the "New Mission Board"
   form maps to POST /api/formations/boards; clicking a card opens that board.
   Failures must surface a clear message, never fake data. */

function jsonResponse(body: unknown, options: { ok?: boolean; status?: number; etag?: string } = {}): Response {
  return {
    ok: options.ok ?? true,
    status: options.status ?? 200,
    headers: { get: (name: string) => (name.toLowerCase() === 'etag' ? options.etag ?? null : null) },
    json: () => Promise.resolve(body),
    text: () => Promise.resolve(''),
  } as Response
}

const boards = [
  {
    id: 'brd_poems',
    slug: 'poems',
    title: 'Poems',
    rev: 3,
    etag: 'etag-poems',
    mission: { id: 'mis_1', title: 'Write a poem', goal: 'Ship a daily poem generator', beadId: 'home-7kc4.5' },
    latestRun: { runId: 'run_9', status: 'succeeded', final: true, epoch: 2 },
  },
  {
    id: 'brd_search',
    slug: 'session-search',
    title: 'Session search',
    rev: 1,
    etag: 'etag-search',
    mission: { id: 'mis_2', title: 'Mission', goal: 'Add fuzzy session search', beadId: '' },
    latestRun: null,
  },
]

describe('MissionsGallery', () => {
  const fetchMock = vi.fn()

  beforeEach(() => {
    fetchMock.mockReset()
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it('renders one card per board with its mission goal, run status and bead anchor', async () => {
    fetchMock.mockImplementation((input: RequestInfo | URL) => {
      if (String(input) === '/api/formations/boards') {
        return Promise.resolve(jsonResponse({ success: true, data: { boards } }))
      }
      return Promise.reject(new Error(`unexpected fetch ${String(input)}`))
    })

    render(<MissionsGallery onOpenBoard={vi.fn()} />)

    expect(await screen.findByText('Ship a daily poem generator')).toBeInTheDocument()
    expect(screen.getByText('Add fuzzy session search')).toBeInTheDocument()
    // Latest-run status pill comes from latestRun, honestly absent when never run.
    expect(screen.getByText('succeeded')).toBeInTheDocument()
    expect(screen.getByText('never run')).toBeInTheDocument()
    // Bead anchor shown only when present.
    expect(screen.getByText('home-7kc4.5')).toBeInTheDocument()
  })

  it('opens a board when its card is clicked', async () => {
    const onOpenBoard = vi.fn()
    fetchMock.mockImplementation((input: RequestInfo | URL) => {
      if (String(input) === '/api/formations/boards') {
        return Promise.resolve(jsonResponse({ success: true, data: { boards } }))
      }
      return Promise.reject(new Error(`unexpected fetch ${String(input)}`))
    })

    render(<MissionsGallery onOpenBoard={onOpenBoard} />)

    fireEvent.click(await screen.findByRole('button', { name: /Poems/ }))
    expect(onOpenBoard).toHaveBeenCalledWith('poems')
  })

  it('creates a board through POST /boards and opens it', async () => {
    const onOpenBoard = vi.fn()
    const posted: Array<Record<string, unknown>> = []
    fetchMock.mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/formations/boards' && init?.method === 'POST') {
        posted.push(JSON.parse(String(init.body)) as Record<string, unknown>)
        return Promise.resolve(jsonResponse({
          success: true,
          data: { board: { id: 'brd_new', slug: 'new-board', title: 'New board', rev: 1, etag: 'e', missions: [], formations: [], connections: [] } },
        }, { etag: 'e' }))
      }
      if (url === '/api/formations/boards') {
        return Promise.resolve(jsonResponse({ success: true, data: { boards } }))
      }
      return Promise.reject(new Error(`unexpected fetch ${url}`))
    })

    render(<MissionsGallery onOpenBoard={onOpenBoard} />)
    await screen.findByText('Ship a daily poem generator')

    fireEvent.change(screen.getByLabelText('Board slug'), { target: { value: 'new-board' } })
    fireEvent.change(screen.getByLabelText('Board title'), { target: { value: 'New board' } })
    fireEvent.change(screen.getByLabelText('Mission goal'), { target: { value: 'Do the thing' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create Mission Board' }))

    await waitFor(() => expect(posted).toHaveLength(1))
    expect(posted[0]).toMatchObject({
      slug: 'new-board',
      title: 'New board',
      mission: { goal: 'Do the thing' },
      updatedBy: 'agent:ui',
    })
    await waitFor(() => expect(onOpenBoard).toHaveBeenCalledWith('new-board'))
  })

  it('surfaces a duplicate-slug error precisely instead of failing silently', async () => {
    fetchMock.mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/formations/boards' && init?.method === 'POST') {
        return Promise.resolve(jsonResponse({
          success: false,
          error: { code: 'CONFLICT', message: 'a board with slug "poems" already exists' },
        }, { ok: false, status: 409 }))
      }
      if (url === '/api/formations/boards') {
        return Promise.resolve(jsonResponse({ success: true, data: { boards } }))
      }
      return Promise.reject(new Error(`unexpected fetch ${url}`))
    })

    render(<MissionsGallery onOpenBoard={vi.fn()} />)
    await screen.findByText('Ship a daily poem generator')

    fireEvent.change(screen.getByLabelText('Board slug'), { target: { value: 'poems' } })
    fireEvent.change(screen.getByLabelText('Board title'), { target: { value: 'Poems' } })
    fireEvent.change(screen.getByLabelText('Mission goal'), { target: { value: 'dupe' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create Mission Board' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('a board with slug "poems" already exists')
  })

  it('surfaces a load failure clearly instead of an empty gallery', async () => {
    fetchMock.mockImplementation((input: RequestInfo | URL) => {
      if (String(input) === '/api/formations/boards') {
        return Promise.resolve(jsonResponse({
          success: false,
          error: { code: 'INTERNAL', message: 'boards index unreadable' },
        }, { ok: false, status: 500 }))
      }
      return Promise.reject(new Error(`unexpected fetch ${String(input)}`))
    })

    render(<MissionsGallery onOpenBoard={vi.fn()} />)
    expect(await screen.findByRole('alert')).toHaveTextContent('boards index unreadable')
  })
})
