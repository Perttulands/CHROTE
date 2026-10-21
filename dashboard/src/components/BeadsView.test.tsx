import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import BeadsView from './BeadsView'
import { DEFAULT_SETTINGS } from '../types'
import { resetBeadCardForTest, useBeadCardRequest } from '../beads/beadCard'
import { resetHostBeadsForTest, type HostBead } from '../beads/hostBeads'
import { knownBead, resetKnownBeadsForTest } from '../beads/knownBeads'

const mockState = vi.hoisted(() => ({
  openSendToSession: vi.fn(),
  announce: vi.fn(),
  updateSettings: vi.fn(),
  hostRequests: [] as string[],
  searchRequests: [] as string[],
  matches: {} as Record<string, string[]>,
}))

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({
    settings: DEFAULT_SETTINGS,
    updateSettings: mockState.updateSettings,
    openSendToSession: mockState.openSendToSession,
  }),
}))

vi.mock('../context/StatusContext', () => ({
  useStatus: () => ({ announce: mockState.announce }),
}))

vi.mock('./ResidentColumn', () => ({
  default: ({ reference }: { reference: string | null }) => (
    <div data-testid="resident" data-reference={reference ?? ''} />
  ),
}))

const NOW = Math.floor(Date.now() / 1000)
const DAY = 86400

function bead(id: string, overrides: Partial<HostBead> = {}): HostBead {
  return { id, title: `Title of ${id}`, status: 'open', priority: 2, created: NOW - 10 * DAY, updated: NOW - 2 * DAY, ...overrides }
}

const STORES = [
  {
    path: '/srv/data/context',
    prefix: 'library',
    generation: 'g1',
    pending: false,
    rows: [
      bead('library-zvi', { title: 'Grill brain ownership', type: 'epic', size: 6400 }),
      bead('library-zvi.1', { title: 'Move the remaining records', parent: 'library-zvi', blockedBy: ['ctx-xvl'], size: 7900 }),
      bead('library-zvi.2', { title: 'Project homes for every project', parent: 'library-zvi', blockedBy: ['library-zvi.1'], size: 300 }),
    ],
  },
  {
    path: '/srv/context-citadel',
    prefix: 'ctx',
    generation: 'g1',
    pending: false,
    rows: [
      bead('ctx-xvl', { title: 'Connector and Ask cite the evidence', status: 'closed', closed: NOW - DAY, size: 2800 }),
      bead('ctx-old', { title: 'An old question', updated: NOW - 40 * DAY, size: 120 }),
    ],
  },
  { path: '/srv/broken', pending: false, error: 'bd list timed out after 60s', rows: [] },
]

function respond(data: unknown): Response {
  return { json: () => Promise.resolve({ success: true, data }) } as unknown as Response
}

function CardProbe() {
  const request = useBeadCardRequest()
  return <span data-testid="card-request">{request ? `${request.id}@${request.projectPath}` : 'none'}</span>
}

const rowIds = () => screen.queryAllByRole('row').slice(1).map(row => row.querySelector('.beads-cell-id')?.textContent)

beforeEach(() => {
  window.history.replaceState(null, '', '/')
  resetHostBeadsForTest()
  resetBeadCardForTest()
  resetKnownBeadsForTest()
  mockState.hostRequests = []
  mockState.searchRequests = []
  mockState.matches = {}
  mockState.openSendToSession.mockReset()
  vi.stubGlobal('fetch', vi.fn((input: string) => {
    const url = new URL(input, 'http://chrote.test')
    if (url.pathname === '/api/beads/host') {
      mockState.hostRequests.push(url.search)
      return Promise.resolve(respond({ stores: STORES, checkedAt: new Date().toISOString() }))
    }
    if (url.pathname === '/api/beads/search') {
      mockState.searchRequests.push(url.searchParams.get('q') ?? '')
      return Promise.resolve(respond({ matches: mockState.matches }))
    }
    return Promise.reject(new Error(`unexpected ${url.pathname}`))
  }))
})

afterEach(() => {
  resetHostBeadsForTest()
  vi.unstubAllGlobals()
  window.history.replaceState(null, '', '/')
})

describe('the Beads tab', () => {
  it('draws every Bead of every store in one table, and says what it read and what failed', async () => {
    render(<BeadsView />)
    await waitFor(() => expect(rowIds()).toHaveLength(5))
    expect(screen.getByText('5 of 5 Beads')).toBeInTheDocument()
    const footer = document.querySelector('.beads-footer') as HTMLElement
    expect(footer).toHaveTextContent('2 stores read')
    expect(footer).toHaveTextContent('/srv/broken: bd list timed out after 60s')
    // A blocker in another store that is finished does not block.
    const row = screen.getAllByRole('row').find(item => item.textContent?.includes('library-zvi.1'))!
    expect(row).toHaveTextContent('open')
  })

  it('searches ids and titles as the operator types, and adds the Beads the server matched by their text', async () => {
    mockState.matches = { '/srv/context-citadel': ['ctx-old'] }
    render(<BeadsView />)
    await waitFor(() => expect(rowIds()).toHaveLength(5))
    fireEvent.change(screen.getByLabelText('Search Beads'), { target: { value: 'project' } })
    expect(rowIds()).toEqual(['library-zvi.2'])
    await waitFor(() => expect(rowIds()).toContain('ctx-old'))
    expect(mockState.searchRequests).toContain('project')
  })

  it('keeps one project by its name, and sorts by size', async () => {
    render(<BeadsView />)
    await waitFor(() => expect(rowIds()).toHaveLength(5))
    const projects = screen.getByRole('group', { name: 'Projects' })
    fireEvent.click(within(projects).getByRole('button', { name: 'library' }))
    expect(rowIds().sort()).toEqual(['library-zvi', 'library-zvi.1', 'library-zvi.2'])
    expect(screen.getByText('3 of 5 Beads')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /^Size/ }))
    expect(rowIds()).toEqual(['library-zvi.1', 'library-zvi', 'library-zvi.2'])
    fireEvent.click(within(projects).getByRole('button', { name: 'library' }))
    expect(rowIds()).toHaveLength(5)
  })

  it('follows the selected row in the graph and hands it to the Clerk', async () => {
    render(<BeadsView />)
    await waitFor(() => expect(rowIds()).toHaveLength(5))
    fireEvent.click(screen.getAllByRole('row').find(item => item.textContent?.includes('library-zvi.2'))!)
    const graph = document.querySelector('.beads-graph') as HTMLElement
    expect(within(graph).getByRole('button', { name: 'library-zvi: Grill brain ownership, open' })).toBeInTheDocument()
    expect(within(graph).getByRole('button', { name: 'library-zvi.2: Project homes for every project', pressed: true })).toBeInTheDocument()
    expect(within(graph).getByRole('button', { name: 'ctx-xvl: Connector and Ask cite the evidence' })).toBeInTheDocument()
    expect(screen.getByTestId('resident')).toHaveAttribute('data-reference', 'bead library-zvi.2: Project homes for every project')
  })

  it('opens the card from the keyboard, already drawn from what the table knows', async () => {
    render(<><BeadsView /><CardProbe /></>)
    await waitFor(() => expect(rowIds()).toHaveLength(5))
    fireEvent.click(screen.getAllByRole('row').find(item => item.textContent?.includes('library-zvi.1'))!)
    fireEvent.keyDown(document.querySelector('.beads-table-body')!, { key: 'Enter' })
    expect(screen.getByTestId('card-request')).toHaveTextContent('library-zvi.1@/srv/data/context')
    expect(knownBead('/srv/data/context', 'library-zvi.1')?.bead.parents.map(link => link.id)).toEqual(['library-zvi'])
  })

  it('opens the slice a link names, and writes the slice back into the link', async () => {
    window.history.replaceState(null, '', '/?tab=beads&q=zvi&sort=size&sel=library-zvi.1&mode=table')
    render(<BeadsView />)
    expect(screen.getByLabelText('Search Beads')).toHaveValue('zvi')
    await waitFor(() => expect(rowIds()).toEqual(['library-zvi.1', 'library-zvi', 'library-zvi.2']))
    expect(screen.getAllByRole('row').find(item => item.getAttribute('aria-selected') === 'true')).toHaveTextContent('library-zvi.1')
    expect(document.querySelector('.beads-graph')).toBeNull()
    fireEvent.click(screen.getByRole('tab', { name: 'Split' }))
    await waitFor(() => expect(window.location.search).toBe('?tab=beads&q=zvi&sort=size&sel=library-zvi.1'))
  })

  it('takes its keys out of the link when the tab is left', async () => {
    const { rerender } = render(<BeadsView />)
    fireEvent.change(screen.getByLabelText('Search Beads'), { target: { value: 'brain' } })
    await waitFor(() => expect(window.location.search).toBe('?tab=beads&q=brain'))
    rerender(<BeadsView active={false} />)
    expect(window.location.search).toBe('')
  })

  it('shows a Bead the card asked for, clearing only a slice that hides it', async () => {
    const { rerender } = render(<BeadsView />)
    await waitFor(() => expect(rowIds()).toHaveLength(5))
    fireEvent.change(screen.getByLabelText('Search Beads'), { target: { value: 'grill' } })
    expect(rowIds()).toEqual(['library-zvi'])
    rerender(<BeadsView reveal={{ projectPath: '/srv/context-citadel', id: 'ctx-old', nonce: 1 }} />)
    await waitFor(() => expect(screen.getByLabelText('Search Beads')).toHaveValue(''))
    expect(screen.getAllByRole('row').find(item => item.getAttribute('aria-selected') === 'true')).toHaveTextContent('ctx-old')
  })

  it('asks the server only for what changed after the first read', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    try {
      render(<BeadsView />)
      await waitFor(() => expect(rowIds()).toHaveLength(5))
      await act(async () => { await vi.advanceTimersByTimeAsync(3100) })
      await waitFor(() => expect(mockState.hostRequests.length).toBeGreaterThan(1))
      const known = new URLSearchParams(mockState.hostRequests[1]).getAll('known')
      expect(known.sort()).toEqual(['g1:/srv/context-citadel', 'g1:/srv/data/context'])
    } finally {
      vi.useRealTimers()
    }
  })
})
