import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import TabBar from './TabBar'
import { DEFAULT_SETTINGS, TERMINAL_WORKSPACE_IDS } from '../types'

const mockState = vi.hoisted(() => ({
  updateSettings: vi.fn(),
  saveCurrentLayout: vi.fn(),
  loadPreset: vi.fn(),
  clearWorkspaceAssignments: vi.fn(),
  deletePreset: vi.fn(),
  workspaceIds: null as readonly string[] | null,
}))

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({
    settings: DEFAULT_SETTINGS,
    updateSettings: mockState.updateSettings,
    saveCurrentLayout: mockState.saveCurrentLayout,
    loadPreset: mockState.loadPreset,
    layoutPresets: [{ id: 'preset-1', name: 'Focus Layout' }],
    clearWorkspaceAssignments: mockState.clearWorkspaceAssignments,
    deletePreset: mockState.deletePreset,
    workspaceIds: mockState.workspaceIds ?? TERMINAL_WORKSPACE_IDS,
  }),
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
    mockState.deletePreset = vi.fn()
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
    expect(screen.getByRole('button', { name: 'Settings' })).toBeInTheDocument()
  })

  it('follows the resolved workspace id list instead of a fixed tab set', () => {
    mockMatchMedia(false)
    mockState.workspaceIds = ['terminal1', 'terminal2', 'terminal3', 'terminal4', 'terminal5']

    render(<TabBar activeTab="terminal1" onTabChange={vi.fn()} />)

    const labels = screen.getAllByRole('button').map(button => button.textContent)
    const terminalLabels = labels.filter(label => label?.startsWith('Terminal'))
    expect(terminalLabels).toEqual(['Terminal', 'Terminal 2', 'Terminal 3', 'Terminal 4', 'Terminal 5'])
    mockState.workspaceIds = null
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

    const { container } = render(<TabBar activeTab="terminal1" onTabChange={onTabChange} />)
    expect(container.querySelector('.tab-bar')).toHaveClass('mobile-mode')
    expect(container.querySelector('.tab-bar-tabs')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '☰' })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '☰' }))
    expect(screen.getByRole('button', { name: 'Terminal' })).toHaveClass('active')
    expect(screen.getByRole('button', { name: 'Settings' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Services' }))

    expect(onTabChange).toHaveBeenCalledWith('services')
  })

  it('opens terminal tab context actions without adding session defaults', () => {
    mockMatchMedia(false)

    render(<TabBar activeTab="terminal1" onTabChange={vi.fn()} />)
    fireEvent.click(screen.getByRole('button', { name: '⋯ Tab' }))

    expect(screen.getByRole('menuitem', { name: /Rename tab/i })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: /Save layout as preset/i })).toBeInTheDocument()
    expect(screen.getByText(/Restore preset/i)).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: /Clear tab assignments/i })).toBeInTheDocument()
    expect(screen.queryByText(/defaults/i)).not.toBeInTheDocument()

    // Clearing is destructive, so it confirms in the row it was chosen from.
    fireEvent.click(screen.getByRole('menuitem', { name: /Clear tab assignments/i }))
    expect(mockState.clearWorkspaceAssignments).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('menuitem', { name: 'Confirm clear' }))
    expect(mockState.clearWorkspaceAssignments).toHaveBeenCalledWith('terminal1')
  })

  it('closes the visible terminal tab menu with Escape', () => {
    mockMatchMedia(false)
    render(<TabBar activeTab="terminal1" onTabChange={vi.fn()} />)

    fireEvent.click(screen.getByRole('button', { name: '⋯ Tab' }))
    expect(screen.getByRole('menuitem', { name: /Rename tab/i })).toBeInTheDocument()
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(screen.queryByRole('menuitem', { name: /Rename tab/i })).not.toBeInTheDocument()
  })

  it('toggles keys from the tab bar and offers the keys panel on its context menu', () => {
    mockMatchMedia(false)
    render(<TabBar activeTab="terminal1" onTabChange={vi.fn()} onShowKeys={vi.fn()} />)

    fireEvent.click(screen.getByRole('button', { name: 'Keys on' }))
    expect(mockState.updateSettings).toHaveBeenCalledWith({ keysEnabled: false })
    expect(screen.queryByRole('menuitem', { name: 'Keybindings' })).not.toBeInTheDocument()
  })

  it('renames a tab in the tab, and abandons the rename on Escape', () => {
    mockMatchMedia(false)
    render(<TabBar activeTab="terminal1" onTabChange={vi.fn()} />)

    fireEvent.click(screen.getByRole('button', { name: '\u22ef Tab' }))
    fireEvent.click(screen.getByRole('menuitem', { name: 'Rename tab' }))

    // The input takes the tab's own place: nothing floats over the workspace.
    const input = screen.getByRole('textbox', { name: 'Rename Terminal' })
    expect(input).toHaveValue('Terminal')
    fireEvent.change(input, { target: { value: 'Ops' } })
    fireEvent.keyDown(input, { key: 'Enter' })

    expect(mockState.updateSettings).toHaveBeenCalledWith({
      terminalLabels: { ...DEFAULT_SETTINGS.terminalLabels, terminal1: 'Ops' },
    })
    expect(screen.getByRole('button', { name: 'Terminal' })).toBeInTheDocument()

    mockState.updateSettings.mockClear()
    fireEvent.click(screen.getByRole('button', { name: '\u22ef Tab' }))
    fireEvent.click(screen.getByRole('menuitem', { name: 'Rename tab' }))
    fireEvent.change(screen.getByRole('textbox', { name: 'Rename Terminal' }), { target: { value: 'Discarded' } })
    fireEvent.keyDown(screen.getByRole('textbox', { name: 'Rename Terminal' }), { key: 'Escape' })

    expect(mockState.updateSettings).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: 'Terminal' })).toBeInTheDocument()
  })

  it('names a preset in the menu and restores one from its submenu', () => {
    mockMatchMedia(false)
    render(<TabBar activeTab="terminal1" onTabChange={vi.fn()} />)

    fireEvent.click(screen.getByRole('button', { name: '\u22ef Tab' }))
    fireEvent.click(screen.getByRole('menuitem', { name: 'Save layout as preset' }))

    // The menu becomes the editor rather than handing off to a dialog.
    const input = screen.getByPlaceholderText('Preset name')
    fireEvent.change(input, { target: { value: 'Focus' } })
    fireEvent.keyDown(input, { key: 'Enter' })
    expect(mockState.saveCurrentLayout).toHaveBeenCalledWith('Focus')

    fireEvent.click(screen.getByRole('button', { name: '\u22ef Tab' }))
    fireEvent.click(screen.getByRole('menuitem', { name: 'Restore preset' }))
    fireEvent.click(screen.getByRole('menuitem', { name: 'Focus Layout' }))
    expect(mockState.loadPreset).toHaveBeenCalledWith('preset-1')
  })

  it('keeps the keys menu and terminal tab menus mutually exclusive', () => {
    mockMatchMedia(false)
    render(<TabBar activeTab="terminal1" onTabChange={vi.fn()} onShowKeys={vi.fn()} />)

    fireEvent.contextMenu(screen.getByRole('button', { name: 'Keys on' }))
    expect(screen.getByRole('menuitem', { name: 'Dashboard Help' })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '⋯ Tab' }))
    expect(screen.queryByRole('menuitem', { name: 'Dashboard Help' })).not.toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: /Rename tab/i })).toBeInTheDocument()
    expect(document.querySelectorAll('.floating-panel-dismiss-layer')).toHaveLength(1)

    fireEvent.keyDown(document, { key: 'Escape' })
    expect(screen.queryByRole('menuitem', { name: /Rename tab/i })).not.toBeInTheDocument()
  })
})
