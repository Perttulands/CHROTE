import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import BeadCard from './BeadCard'
import { DEFAULT_SETTINGS } from '../types'
import { closeBeadCard, openBeadCard, resetBeadCardForTest } from '../beads/beadCard'
import { resetBeadProjectsForTest, setBeadProjects } from '../beads/beadIds'
import { rememberBeadRows, resetKnownBeadsForTest } from '../beads/knownBeads'
import { resetChordsForTest } from '../keys/chords'
import type { BeadDetail, BeadRow } from '../beads/beadsApi'

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

function row(overrides: Partial<BeadRow> & { id: string }): BeadRow {
  return { title: `Title of ${overrides.id}`, status: 'open', type: 'task', priority: 1, blocked: false, ...overrides }
}

/** A fetch that answers only when the test says so. */
function holdFetch() {
  let release: (detail: BeadDetail) => void = () => {}
  const asked = new Promise<void>(resolveAsked => {
    mockState.fetchBead.mockImplementation(() => {
      resolveAsked()
      return new Promise<BeadDetail>(resolve => { release = resolve })
    })
  })
  return async (detail: BeadDetail) => {
    await asked
    await act(async () => { release(detail) })
  }
}

afterEach(() => {
  resetBeadCardForTest()
  resetBeadProjectsForTest()
  resetKnownBeadsForTest()
  resetChordsForTest()
})

describe('the Bead card', () => {
  it('shows nothing until a Bead is asked for', () => {
    const { container } = render(<BeadCard />)
    expect(container).toBeEmptyDOMElement()
  })

  it('reads the Bead from the store its prefix names', async () => {
    render(<BeadCard />)
    act(() => openBeadCard('chrote-5grx.15'))

    expect(await screen.findByText('Title of chrote-5grx.15')).toBeInTheDocument()
    expect(mockState.fetchBead).toHaveBeenCalledWith('/srv/chrote', 'chrote-5grx.15')
    expect(screen.getByText('chrote-5grx.15')).toBeInTheDocument()
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

  it('opens at once from what the map holds and fills the text in beneath', async () => {
    rememberBeadRows('/srv/chrote', [
      row({ id: 'chrote-5grx', title: 'The epic', type: 'epic' }),
      row({ id: 'chrote-5grx.15', parent: 'chrote-5grx', blocked: true, blockedBy: ['chrote-5grx.11'] }),
      row({ id: 'chrote-5grx.11', title: 'Journeys' }),
      row({ id: 'chrote-5grx.15.1', parent: 'chrote-5grx.15', title: 'A child' }),
    ])
    const settle = holdFetch()
    render(<BeadCard />)

    act(() => openBeadCard('chrote-5grx.15', '/srv/chrote'))

    // The header and every relation are on screen in the same frame as the request.
    expect(screen.getByText('Title of chrote-5grx.15')).toBeInTheDocument()
    expect(screen.getByText('blocked')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /chrote-5grx$/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /chrote-5grx\.15\.1$/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /chrote-5grx\.11$/ })).toBeInTheDocument()
    expect(screen.getByText('Reading…')).toBeInTheDocument()
    expect(screen.queryByText('The card opens from a terminal id.')).toBeNull()

    // What the row said is enough to hand the Bead on before the server answers.
    fireEvent.click(screen.getByRole('button', { name: 'Send' }))
    expect(mockState.openSendToSession).toHaveBeenCalledWith({
      reference: 'bead chrote-5grx.15: Title of chrote-5grx.15',
    })

    await settle(CARD)

    expect(await screen.findByText('The card opens from a terminal id.')).toBeInTheDocument()
    expect(screen.queryByText('Reading…')).toBeNull()
  })

  it('never shows the previous Bead while the next one is read', async () => {
    render(<BeadCard />)
    act(() => openBeadCard('chrote-5grx.15'))
    await screen.findByText('The card opens from a terminal id.')

    holdFetch()
    fireEvent.click(screen.getByRole('button', { name: 'chrote-5grx.6' }))

    expect(screen.getByText('Reading chrote-5grx.6…')).toBeInTheDocument()
    expect(screen.queryByText('Title of chrote-5grx.15')).toBeNull()
    expect(screen.queryByText('The card opens from a terminal id.')).toBeNull()
  })

  it('reopens from what the session remembers and refreshes it behind', async () => {
    render(<BeadCard />)
    act(() => openBeadCard('chrote-5grx.15'))
    await screen.findByText('The card opens from a terminal id.')
    act(() => closeBeadCard())
    expect(screen.queryByText('Title of chrote-5grx.15')).toBeNull()

    mockState.fetchBead.mockClear()
    holdFetch()
    act(() => openBeadCard('chrote-5grx.15'))

    expect(screen.getByText('The card opens from a terminal id.')).toBeInTheDocument()
    expect(screen.queryByText('Reading…')).toBeNull()
    await waitFor(() => expect(mockState.fetchBead).toHaveBeenCalledWith('/srv/chrote', 'chrote-5grx.15'))
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
