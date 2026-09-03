import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import TableColumn from './TableColumn'
import { DEFAULT_SETTINGS } from '../types'
import { TABLE_WIDTH_MIN, resetTableForTest } from '../context/TableContext'
import { openBeadCard } from '../beads/beadCard'
import { openAgentContext } from '../agents/agentContextPanel'

const mockState = vi.hoisted(() => ({
  tableWidth: 400,
  drawerOpen: false,
  updateSettings: vi.fn(),
}))

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({
    settings: { ...DEFAULT_SETTINGS, tableWidth: mockState.tableWidth },
    updateSettings: mockState.updateSettings,
    sendToSessionRequest: mockState.drawerOpen ? {} : null,
    floatingSession: null,
    openSendToSession: vi.fn(),
  }),
}))

vi.mock('./BeadCard', () => ({ default: () => <div>the Bead card</div> }))
vi.mock('./AgentContextSheet', () => ({ default: () => <div>what the agent sees</div> }))

const AGENT = { sessionKey: 'alice:jack', folder: '/srv/chrote', harness: 'claude-code' as const, user: 'alice', shell: false }

beforeEach(() => {
  mockState.tableWidth = 400
  mockState.drawerOpen = false
  mockState.updateSettings.mockReset()
})

afterEach(() => {
  resetTableForTest()
})

describe('the table column', () => {
  it('draws nothing with nothing on the table, and names what it shows', () => {
    const { container } = render(<TableColumn />)
    expect(container).toBeEmptyDOMElement()

    act(() => openBeadCard('chrote-5grx.47'))
    expect(screen.getByRole('complementary', { name: 'Bead chrote-5grx.47' })).toHaveTextContent('the Bead card')

    act(() => openAgentContext(AGENT))
    expect(screen.getByRole('complementary', { name: 'What jack sees' })).toHaveTextContent('what the agent sees')
  })

  // Escape belongs to the topmost surface: the table's while nothing lies
  // over it, the drawer's while the drawer is open.
  it('closes on Escape, unless the Send drawer is the surface on top', () => {
    const { rerender } = render(<TableColumn />)
    act(() => openBeadCard('chrote-5grx.47'))

    mockState.drawerOpen = true
    rerender(<TableColumn />)
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(screen.getByRole('complementary')).toBeInTheDocument()

    mockState.drawerOpen = false
    rerender(<TableColumn />)
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(screen.queryByRole('complementary')).toBeNull()
  })

  it('resizes from the keyboard on its handle, and never under the minimum', () => {
    render(<TableColumn />)
    act(() => openBeadCard('chrote-5grx.47'))
    const handle = screen.getByRole('separator', { name: 'Resize the table' })
    expect(handle).toHaveAttribute('aria-valuenow', '400')

    fireEvent.keyDown(handle, { key: 'ArrowLeft' })
    expect(mockState.updateSettings).toHaveBeenLastCalledWith({ tableWidth: 416 })

    mockState.tableWidth = TABLE_WIDTH_MIN
    act(() => openBeadCard('chrote-5grx.25'))
    fireEvent.keyDown(handle, { key: 'ArrowRight' })
    expect(mockState.updateSettings).toHaveBeenLastCalledWith({ tableWidth: TABLE_WIDTH_MIN })
  })
})
