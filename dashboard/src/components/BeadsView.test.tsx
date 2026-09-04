import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import BeadsView from './BeadsView'
import { DEFAULT_SETTINGS } from '../types'
import { resetBeadCardForTest, useBeadCardRequest } from '../beads/beadCard'
import { resetBeadProjectsForTest } from '../beads/beadIds'
import type { BeadRow } from '../beads/beadsApi'

const mockState = vi.hoisted(() => ({
  openSendToSession: vi.fn(),
  announce: vi.fn(),
  updateSettings: vi.fn(),
  settings: null as unknown as typeof DEFAULT_SETTINGS,
  projectList: null as unknown[] | null,
  projects: [] as unknown[],
  work: new Map<string, unknown>(),
  fetchBeadWork: vi.fn(),
}))

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({
    settings: mockState.settings,
    updateSettings: mockState.updateSettings,
    openSendToSession: mockState.openSendToSession,
  }),
}))

vi.mock('../context/StatusContext', () => ({
  useStatus: () => ({ announce: mockState.announce }),
}))

vi.mock('../beads/beadsApi', () => ({
  fetchBeadProjectList: () => Promise.resolve(mockState.projectList ?? mockState.projects),
  fetchBeadProjects: () => Promise.resolve(mockState.projects),
  fetchBeadWork: (path: string) => mockState.fetchBeadWork(path),
}))

vi.mock('./ResidentColumn', () => ({
  default: ({ reference }: { reference: string | null }) => (
    <div data-testid="resident" data-reference={reference ?? ''} />
  ),
}))

function bead(overrides: Partial<BeadRow> & { id: string }): BeadRow {
  return { title: `Title of ${overrides.id}`, status: 'open', priority: 1, blocked: false, ...overrides }
}

const OLD = new Date(Date.now() - 40 * 86400000).toISOString()
const FRESH = new Date(Date.now() - 1 * 86400000).toISOString()

function CardProbe() {
  const request = useBeadCardRequest()
  return <span data-testid="card-request">{request?.id ?? 'none'}</span>
}

beforeEach(() => {
  mockState.openSendToSession.mockReset()
  mockState.announce.mockReset()
  mockState.updateSettings.mockReset()
  mockState.settings = DEFAULT_SETTINGS
  mockState.projectList = null
  mockState.projects = [
    { name: 'chrote', path: '/srv/chrote', beadsPath: '/srv/chrote/.beads', prefix: 'chrote', openBeads: 3 },
    { name: 'srv', path: '/srv', beadsPath: '/srv/.beads', prefix: 'ctx', openBeads: 1 },
    { name: 'quiet', path: '/srv/quiet', beadsPath: '/srv/quiet/.beads', prefix: 'qt', openBeads: 0 },
    { name: 'silent', path: '/srv/silent', beadsPath: '/srv/silent/.beads', prefix: 'sl', openBeads: 0 },
  ]
  mockState.work = new Map<string, unknown>([
    ['/srv/chrote', {
      prefix: 'chrote',
      projectPath: '/srv/chrote',
      beads: [
        bead({ id: 'chrote-ep', type: 'epic', acceptance: 'Everything under it lands', updated: FRESH }),
        bead({ id: 'chrote-ep.1', parent: 'chrote-ep', type: 'task', updated: FRESH }),
        bead({
          id: 'chrote-ep.2',
          parent: 'chrote-ep',
          type: 'task',
          priority: 2,
          blocked: true,
          blockedBy: ['chrote-ep.1'],
          updated: OLD,
        }),
      ],
    }],
    ['/srv', { prefix: 'ctx', projectPath: '/srv', beads: [bead({ id: 'ctx-t4ak', type: 'task', updated: OLD })] }],
    ['/srv/quiet', { prefix: 'qt', projectPath: '/srv/quiet', beads: [] }],
    ['/srv/silent', { prefix: 'sl', projectPath: '/srv/silent', beads: [] }],
  ])
  mockState.fetchBeadWork.mockReset()
  mockState.fetchBeadWork.mockImplementation((path: string) => {
    const answer = mockState.work.get(path)
    return answer instanceof Error ? Promise.reject(answer) : Promise.resolve(answer)
  })
})

afterEach(() => {
  resetBeadCardForTest()
  resetBeadProjectsForTest()
})

describe('the Beads tab', () => {
  it('draws every configured store as a map of open work', async () => {
    render(<BeadsView />)

    expect(await screen.findByText('Title of chrote-ep')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'All' })).toHaveClass('active')
    expect(screen.getByRole('button', { name: 'ctx' })).toBeInTheDocument()
    expect(screen.getByText('Acceptance criteria')).toBeInTheDocument()
    expect(screen.getByText('Everything under it lands')).toBeInTheDocument()
    expect(screen.getByText(/blocked by/)).toBeInTheDocument()
    expect(screen.getByText('Title of ctx-t4ak')).toBeInTheDocument()
  })

  it('says what it loaded on the status line, and asks nothing of a quiet store', async () => {
    render(<BeadsView />)
    await waitFor(() => expect(mockState.announce).toHaveBeenCalled())
    expect(mockState.announce.mock.calls[0][0]).toBe('Beads loaded · chrote 3 open · ctx 1 open')
  })

  it('keeps readable stores visible when another store refuses the work request', async () => {
    mockState.projects.splice(2, 0, {
      name: 'broken', path: '/srv/broken', beadsPath: '/srv/broken/.beads', prefix: 'bad', openBeads: 2,
    })
    mockState.work.set('/srv/broken', new Error('permission denied'))

    render(<BeadsView />)

    expect(await screen.findByText('Title of chrote-ep')).toBeInTheDocument()
    expect(screen.getByText('Title of ctx-t4ak')).toBeInTheDocument()
    await waitFor(() => expect(mockState.announce).toHaveBeenCalledWith(
      'Beads unavailable · bad: permission denied',
      'error',
    ))
  })

  it('lists a known unreadable store instead of loading it through All', async () => {
    mockState.projects.splice(2, 0, {
      name: 'broken', path: '/srv/broken', beadsPath: '/srv/broken/.beads', error: 'permission denied',
    })

    render(<BeadsView />)

    expect(await screen.findByRole('button', { name: 'broken · unreadable' })).toBeInTheDocument()
    expect(mockState.fetchBeadWork).not.toHaveBeenCalledWith('/srv/broken')
  })

  it('folds the stores with nothing open under one row that expands in place', async () => {
    render(<BeadsView />)
    await screen.findByText('Title of ctx-t4ak')

    const rail = screen.getByRole('navigation', { name: 'Beads projects' })
    const rows = () => [...rail.querySelectorAll('.beads-rail-item')].map(row => row.textContent)
    expect(rows()).toEqual(['All', 'chrote', 'ctx', 'More (2 quiet)'])

    fireEvent.click(screen.getByRole('button', { name: 'More (2 quiet)' }))
    expect(rows()).toEqual(['All', 'chrote', 'ctx', 'Fewer', 'qt', 'sl'])

    fireEvent.click(screen.getByRole('button', { name: 'qt' }))
    await waitFor(() => expect(screen.queryByText('Title of ctx-t4ak')).toBeNull())
    expect(screen.getByRole('button', { name: 'qt' })).toHaveClass('active')
  })

  it('narrows every view by id and title', async () => {
    render(<BeadsView />)
    await screen.findByText('Title of ctx-t4ak')

    fireEvent.change(screen.getByLabelText('Search Beads'), { target: { value: 'ctx-t4ak' } })

    expect(screen.getByText('Title of ctx-t4ak')).toBeInTheDocument()
    expect(screen.queryByText('Title of chrote-ep.1')).toBeNull()
  })

  it('shows one store at a time from the rail', async () => {
    render(<BeadsView />)
    await screen.findByText('Title of ctx-t4ak')

    fireEvent.click(screen.getByRole('button', { name: 'ctx' }))

    await waitFor(() => expect(screen.queryByText('Title of chrote-ep')).toBeNull())
    expect(screen.getByText('Title of ctx-t4ak')).toBeInTheDocument()
  })

  it('opens the card from a row, and hands the Clerk the Bead by id and title', async () => {
    render(<><BeadsView /><CardProbe /></>)
    const row = await screen.findByText('Title of chrote-ep.1')
    expect(screen.getByTestId('resident')).toHaveAttribute('data-reference', '')

    fireEvent.click(row)

    expect(screen.getByTestId('card-request')).toHaveTextContent('chrote-ep.1')
    expect(screen.getByTestId('resident')).toHaveAttribute('data-reference', 'bead chrote-ep.1: Title of chrote-ep.1')
  })

  it('hands a stale Bead over with the question already asked', async () => {
    render(<BeadsView />)
    await screen.findByText('Title of ctx-t4ak')

    fireEvent.click(screen.getByRole('tab', { name: 'Stale' }))

    expect(screen.getByText('Title of chrote-ep.2')).toBeInTheDocument()
    expect(screen.queryByText('Title of chrote-ep.1')).toBeNull()

    fireEvent.click(screen.getAllByRole('button', { name: 'Send' })[0])
    expect(mockState.openSendToSession).toHaveBeenCalledWith({
      reference: 'bead chrote-ep.2 looks stale: close it or revive it',
    })
  })

  it('splits ready from what is already claimed', async () => {
    mockState.work.set('/srv', {
      prefix: 'ctx',
      projectPath: '/srv',
      beads: [bead({ id: 'ctx-t4ak', type: 'task', status: 'in_progress', updated: FRESH })],
    })
    render(<BeadsView />)
    await screen.findByText('Title of ctx-t4ak')

    fireEvent.click(screen.getByRole('tab', { name: 'Open' }))

    expect(screen.getByRole('heading', { name: 'Ready to start' })).toBeInTheDocument()
    const inProgress = screen.getByRole('heading', { name: 'In progress' }).parentElement as HTMLElement
    expect(inProgress).toHaveTextContent('ctx-t4ak')
    expect(inProgress).not.toHaveTextContent('chrote-ep.1')
  })

  it('restores the selected store and Open view before fetching work', async () => {
    mockState.settings = {
      ...DEFAULT_SETTINGS,
      beadsSelectedProject: '/srv',
      beadsView: 'ready',
    }

    render(<BeadsView />)

    expect(await screen.findByRole('button', { name: 'ctx' })).toHaveClass('active')
    expect(screen.getByRole('tab', { name: 'Open' })).toHaveAttribute('aria-selected', 'true')
    await waitFor(() => expect(mockState.fetchBeadWork).toHaveBeenCalled())
    expect(mockState.fetchBeadWork.mock.calls[0][0]).toBe('/srv')
    expect(mockState.fetchBeadWork).not.toHaveBeenCalledWith('/srv/chrote')
  })

  it('fills counts after the first store list without another click', async () => {
    mockState.projectList = mockState.projects.map(project => {
      const { openBeads: _openBeads, ...listed } = project as { openBeads?: number }
      return { ...listed, summaryPending: true }
    })

    render(<BeadsView />)

    expect(await screen.findByRole('button', { name: 'More (2 quiet)' })).toBeInTheDocument()
  })
})
