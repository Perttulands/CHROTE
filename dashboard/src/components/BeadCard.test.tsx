import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import BeadCard from './BeadCard'
import { DEFAULT_SETTINGS } from '../types'
import { openBeadCard, resetBeadCardForTest } from '../beads/beadCard'
import { resetBeadProjectsForTest, setBeadProjects } from '../beads/beadIds'
import { resetChordsForTest } from '../keys/chords'
import type { BeadDetail } from '../beads/beadsApi'

const mockState = vi.hoisted(() => ({
  openSendToSession: vi.fn(),
  announce: vi.fn(),
  fetchBead: vi.fn(),
  copy: vi.fn(),
}))

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({ settings: DEFAULT_SETTINGS, openSendToSession: mockState.openSendToSession }),
}))

vi.mock('../context/StatusContext', () => ({
  useStatus: () => ({ announce: mockState.announce }),
}))

vi.mock('../beads/beadsApi', () => ({
  fetchBeadProjects: () => Promise.resolve([]),
  fetchBead: (path: string, id: string) => mockState.fetchBead(path, id),
}))

vi.mock('../utils/clipboard', () => ({
  copyAndAnnounce: (text: string, what: string, announce: unknown) => mockState.copy(text, what, announce),
}))

function detail(overrides: Partial<BeadDetail> & { id: string }): BeadDetail {
  return {
    title: `Title of ${overrides.id}`,
    status: 'in_progress',
    type: 'task',
    priority: 1,
    updated: '2026-09-03T09:47:00Z',
    parents: [],
    children: [],
    blockedBy: [],
    blocks: [],
    ...overrides,
  }
}

const CARD = detail({
  id: 'chrote-5grx.15',
  description: 'Wave 2. From chrote-5grx.6: ids as links.',
  acceptance: 'The card opens from a terminal id.',
  notes: 'Approved on the mock.',
  parents: [{ id: 'chrote-5grx', title: 'The epic', status: 'open', type: 'epic', priority: 1 }],
  blockedBy: [{ id: 'chrote-5grx.11', title: 'Journeys', status: 'closed', type: 'task', priority: 1 }],
})

beforeEach(() => {
  mockState.openSendToSession.mockReset()
  mockState.announce.mockReset()
  mockState.copy.mockReset()
  mockState.fetchBead.mockReset()
  mockState.fetchBead.mockResolvedValue(CARD)
  setBeadProjects([{ name: 'chrote', path: '/srv/chrote', beadsPath: '/srv/chrote/.beads', prefix: 'chrote' }])
})

afterEach(() => {
  resetBeadCardForTest()
  resetBeadProjectsForTest()
  resetChordsForTest()
})

describe('the Bead card', () => {
  it('shows nothing until a Bead is asked for', () => {
    render(<BeadCard />)
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  it('reads the Bead from the store its prefix names', async () => {
    render(<BeadCard />)
    act(() => openBeadCard('chrote-5grx.15'))

    expect(await screen.findByText('Title of chrote-5grx.15')).toBeInTheDocument()
    expect(mockState.fetchBead).toHaveBeenCalledWith('/srv/chrote', 'chrote-5grx.15')
    expect(screen.getByRole('dialog', { name: 'Bead chrote-5grx.15' })).toBeInTheDocument()
    expect(screen.getByText('in progress')).toBeInTheDocument()
    expect(screen.getByText('The card opens from a terminal id.')).toBeInTheDocument()
    expect(screen.getByText('Approved on the mock.')).toBeInTheDocument()
  })

  it('says so when no configured store owns the id', async () => {
    render(<BeadCard />)
    act(() => openBeadCard('other-abc'))

    expect(await screen.findByText('No configured Beads project owns other-abc')).toBeInTheDocument()
    expect(mockState.fetchBead).not.toHaveBeenCalled()
  })

  it('hands the Bead to an agent with the reference the contract states', async () => {
    render(<BeadCard />)
    act(() => openBeadCard('chrote-5grx.15'))
    await screen.findByText('Title of chrote-5grx.15')

    fireEvent.click(screen.getByRole('button', { name: 'Send' }))

    expect(mockState.openSendToSession).toHaveBeenCalledWith({
      reference: 'bead chrote-5grx.15: Title of chrote-5grx.15',
    })
  })

  it('sends on Alt+S while it is open, taking the key from the focused tile', async () => {
    render(<BeadCard />)
    act(() => openBeadCard('chrote-5grx.15'))
    await screen.findByText('Title of chrote-5grx.15')

    fireEvent.keyDown(document, { key: 's', altKey: true })

    await waitFor(() => expect(mockState.openSendToSession).toHaveBeenCalledWith({
      reference: 'bead chrote-5grx.15: Title of chrote-5grx.15',
    }))
  })

  it('follows an id inside the text and comes back', async () => {
    render(<BeadCard />)
    act(() => openBeadCard('chrote-5grx.15'))
    await screen.findByText('Title of chrote-5grx.15')

    mockState.fetchBead.mockResolvedValue(detail({ id: 'chrote-5grx.6', status: 'closed' }))
    fireEvent.click(screen.getByRole('button', { name: 'chrote-5grx.6' }))

    expect(await screen.findByText('Title of chrote-5grx.6')).toBeInTheDocument()
    expect(mockState.fetchBead).toHaveBeenLastCalledWith('/srv/chrote', 'chrote-5grx.6')

    mockState.fetchBead.mockResolvedValue(CARD)
    fireEvent.click(screen.getByRole('button', { name: 'Back' }))
    expect(await screen.findByText('Title of chrote-5grx.15')).toBeInTheDocument()
  })

  it('follows a link to the parent it lists', async () => {
    render(<BeadCard />)
    act(() => openBeadCard('chrote-5grx.15'))
    await screen.findByText('Title of chrote-5grx.15')

    mockState.fetchBead.mockResolvedValue(detail({ id: 'chrote-5grx', type: 'epic', status: 'open' }))
    fireEvent.click(screen.getByRole('button', { name: /chrote-5grx$/ }))

    expect(await screen.findByText('Title of chrote-5grx')).toBeInTheDocument()
  })

  it('copies the id through the helper that reports how it went', async () => {
    render(<BeadCard />)
    act(() => openBeadCard('chrote-5grx.15'))
    await screen.findByText('Title of chrote-5grx.15')

    fireEvent.click(screen.getByRole('button', { name: 'Copy id' }))

    expect(mockState.copy).toHaveBeenCalledWith('chrote-5grx.15', 'chrote-5grx.15', mockState.announce)
  })

  it('offers the Bead to the tab that maps its project', async () => {
    const onOpenInBeads = vi.fn()
    render(<BeadCard onOpenInBeads={onOpenInBeads} />)
    act(() => openBeadCard('chrote-5grx.15'))
    await screen.findByText('Title of chrote-5grx.15')

    fireEvent.click(screen.getByRole('button', { name: 'Open in Beads' }))

    expect(onOpenInBeads).toHaveBeenCalledWith('/srv/chrote', 'chrote-5grx.15')
  })
})
