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
  projects: [] as unknown[],
  work: new Map<string, unknown>(),
}))

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({ settings: DEFAULT_SETTINGS, openSendToSession: mockState.openSendToSession }),
}))

vi.mock('../context/StatusContext', () => ({
  useStatus: () => ({ announce: mockState.announce }),
}))

vi.mock('../beads/beadsApi', () => ({
  fetchBeadProjects: () => Promise.resolve(mockState.projects),
  fetchBeadWork: (path: string) => Promise.resolve(mockState.work.get(path)),
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
  mockState.projects = [
    { name: 'chrote', path: '/srv/chrote', beadsPath: '/srv/chrote/.beads', prefix: 'chrote' },
    { name: 'srv', path: '/srv', beadsPath: '/srv/.beads', prefix: 'ctx' },
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
  ])
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

  it('says what it loaded on the status line', async () => {
    render(<BeadsView />)
    await waitFor(() => expect(mockState.announce).toHaveBeenCalled())
    expect(mockState.announce.mock.calls[0][0]).toBe('Beads loaded · chrote 3 open · ctx 1 open')
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

  it('opens the card from a row', async () => {
    render(<><BeadsView /><CardProbe /></>)
    const row = await screen.findByText('Title of chrote-ep.1')

    fireEvent.click(row)

    expect(screen.getByTestId('card-request')).toHaveTextContent('chrote-ep.1')
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

    fireEvent.click(screen.getByRole('tab', { name: 'Ready and in progress' }))

    const inProgress = screen.getByRole('heading', { name: 'In progress' }).parentElement as HTMLElement
    expect(inProgress).toHaveTextContent('ctx-t4ak')
    expect(inProgress).not.toHaveTextContent('chrote-ep.1')
  })
})
