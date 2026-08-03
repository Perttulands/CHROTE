import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import TabBar from './TabBar'
import { DEFAULT_SETTINGS, TERMINAL_WORKSPACE_IDS } from '../types'

const mockState = vi.hoisted(() => ({
  updateSettings: vi.fn(),
  saveCurrentLayout: vi.fn(),
  loadPreset: vi.fn(),
  clearWorkspaceAssignments: vi.fn(),
}))

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({
    settings: DEFAULT_SETTINGS,
    updateSettings: mockState.updateSettings,
    saveCurrentLayout: mockState.saveCurrentLayout,
    loadPreset: mockState.loadPreset,
    layoutPresets: [{ id: 'preset-1', name: 'Focus Layout' }],
    clearWorkspaceAssignments: mockState.clearWorkspaceAssignments,
    workspaceIds: TERMINAL_WORKSPACE_IDS,
  }),
}))

vi.mock('./MusicPlayer', () => ({
  default: () => <div data-testid="music-player" />,
}))

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

describe('TabBar Services navigation', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.stubGlobal('prompt', vi.fn(() => 'Ops'))
  })

  it('shows Terminal 3 in desktop navigation and routes through tab change', () => {
    mockMatchMedia(false)
    const onTabChange = vi.fn()

    render(<TabBar activeTab="terminal1" onTabChange={onTabChange} />)
    fireEvent.click(screen.getByRole('button', { name: 'Terminal 3' }))

    expect(onTabChange).toHaveBeenCalledWith('terminal3')
  })

  it('renders exactly the default terminal tabs, in order, with canonical labels', () => {
    mockMatchMedia(false)

    render(<TabBar activeTab="terminal1" onTabChange={vi.fn()} />)

    const labels = screen.getAllByRole('button').map(button => button.textContent)
    const terminalLabels = labels.filter(label => label?.startsWith('Terminal'))
    expect(terminalLabels).toEqual(['Terminal', 'Terminal 2', 'Terminal 3'])
  })

  it('shows Services in desktop navigation', () => {
    mockMatchMedia(false)
    const onTabChange = vi.fn()

    render(<TabBar activeTab="terminal1" onTabChange={onTabChange} />)
    fireEvent.click(screen.getByRole('button', { name: 'Services' }))

    expect(onTabChange).toHaveBeenCalledWith('services')
  })

  it('shows Scheduled Tasks in desktop navigation', () => {
    mockMatchMedia(false)
    const onTabChange = vi.fn()

    render(<TabBar activeTab="terminal1" onTabChange={onTabChange} />)
    fireEvent.click(screen.getByRole('button', { name: 'Scheduled' }))

    expect(onTabChange).toHaveBeenCalledWith('scheduled')
  })

  it('shows Services in mobile navigation', () => {
    mockMatchMedia(true)
    const onTabChange = vi.fn()

    render(<TabBar activeTab="terminal1" onTabChange={onTabChange} />)
    fireEvent.click(screen.getByRole('button', { name: '☰' }))
    fireEvent.click(screen.getByRole('button', { name: 'Services' }))

    expect(onTabChange).toHaveBeenCalledWith('services')
  })

  it('opens terminal tab context actions without adding session defaults', () => {
    mockMatchMedia(false)

    render(<TabBar activeTab="terminal1" onTabChange={vi.fn()} />)
    fireEvent.click(screen.getByRole('button', { name: '⋯ Tab' }))

    expect(screen.getByRole('button', { name: /Rename tab label/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Save layout as preset/i })).toBeInTheDocument()
    expect(screen.getByText(/Restore layout preset/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Clear tab assignments/i })).toBeInTheDocument()
    expect(screen.queryByText(/defaults/i)).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /Clear tab assignments/i }))
    expect(mockState.clearWorkspaceAssignments).toHaveBeenCalledWith('terminal1')
  })

  it('closes the visible terminal tab menu with Escape', () => {
    mockMatchMedia(false)
    render(<TabBar activeTab="terminal1" onTabChange={vi.fn()} />)

    fireEvent.click(screen.getByRole('button', { name: '⋯ Tab' }))
    expect(screen.getByRole('button', { name: /Rename tab label/i })).toBeInTheDocument()
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(screen.queryByRole('button', { name: /Rename tab label/i })).not.toBeInTheDocument()
  })

  it('keeps help and terminal tab menus mutually exclusive', () => {
    mockMatchMedia(false)
    const { container } = render(<TabBar activeTab="terminal1" onTabChange={vi.fn()} />)

    fireEvent.click(screen.getByRole('button', { name: '?' }))
    expect(screen.getByRole('button', { name: 'Dashboard Help' })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '⋯ Tab' }))
    expect(screen.queryByRole('button', { name: 'Dashboard Help' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Rename tab label/i })).toBeInTheDocument()
    expect(container.querySelectorAll('.floating-panel-dismiss-layer')).toHaveLength(1)

    fireEvent.keyDown(document, { key: 'Escape' })
    expect(screen.queryByRole('button', { name: /Rename tab label/i })).not.toBeInTheDocument()
  })
})
