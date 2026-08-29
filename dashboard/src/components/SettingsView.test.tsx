import { fireEvent, render, screen, within } from '@testing-library/react'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import SettingsView from './SettingsView'
import { DEFAULT_SETTINGS, TERMINAL_WORKSPACE_IDS } from '../types'

const mockUseSession = vi.fn()
const refreshSessions = vi.fn()
const addToast = vi.fn()

vi.mock('../context/SessionContext', () => ({
  useSession: () => mockUseSession(),
}))

vi.mock('../context/ToastContext', () => ({
  useToast: () => ({ addToast }),
}))

vi.mock('./FolderPickerModal', () => ({
  default: () => <div data-testid="folder-picker" />,
}))

const settings = {
  ...DEFAULT_SETTINGS,
  terminalLaunchUsers: {},
  terminalSessionPrefixes: {},
  terminalUserColors: {},
}

const terminalUsers = ['alice', 'bob']

function sessionReturn(updateSettings: ReturnType<typeof vi.fn>, overrides: Record<string, unknown> = {}) {
  return {
    settings,
    terminalUsers,
    updateSettings,
    sessions: [],
    refreshSessions,
    workspaceIds: TERMINAL_WORKSPACE_IDS,
    ...overrides,
  }
}

describe('SettingsView terminal launch users', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    Object.assign(navigator, { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } })
  })

  it('exposes the terminal tab count as a bounded select and routes it through updateSettings', () => {
    const updateSettings = vi.fn()
    mockUseSession.mockReturnValue(sessionReturn(updateSettings))

    render(<SettingsView />)

    const countSelect = screen.getByRole('combobox', { name: 'Terminal tabs' })
    expect(countSelect).toHaveValue('3')
    const options = within(countSelect).getAllByRole('option').map(option => option.getAttribute('value'))
    expect(options).toEqual(['1', '2', '3', '4', '5', '6'])

    fireEvent.change(countSelect, { target: { value: '5' } })

    expect(updateSettings).toHaveBeenCalledWith({ terminalTabCount: 5 })
  })

  it('lets each terminal tab choose the Unix user used for new shells from configured users', () => {
    const updateSettings = vi.fn()
    mockUseSession.mockReturnValue(sessionReturn(updateSettings))

    render(<SettingsView />)

    const terminal3Select = screen.getByRole('combobox', { name: 'Terminal 3 launch user' })
    expect(screen.getByRole('combobox', { name: 'Terminal launch user' })).toHaveValue('alice')
    expect(screen.getByRole('combobox', { name: 'Terminal 2 launch user' })).toHaveValue('alice')
    expect(terminal3Select).toHaveValue('bob')

    fireEvent.change(terminal3Select, { target: { value: 'alice' } })

    expect(updateSettings).toHaveBeenCalledWith({
      terminalLaunchUsers: {
        terminal3: 'alice',
      },
    })
  })

  it('lets each configured Unix user have an independent session prefix', () => {
    const updateSettings = vi.fn()
    mockUseSession.mockReturnValue(sessionReturn(updateSettings))

    render(<SettingsView />)

    expect(screen.queryByLabelText('Default Session Prefix')).not.toBeInTheDocument()
    expect(screen.getByLabelText('alice session prefix')).toHaveValue('shell')
    expect(screen.getByLabelText('bob session prefix')).toHaveValue('bob')

    fireEvent.change(screen.getByLabelText('bob session prefix'), { target: { value: 'forge' } })

    expect(updateSettings).toHaveBeenCalledWith({
      terminalSessionPrefixes: {
        bob: 'forge',
      },
    })
  })

  it('lets each configured Unix user have an independent session indicator color', () => {
    const updateSettings = vi.fn()
    mockUseSession.mockReturnValue(sessionReturn(updateSettings, {
      settings: {
        ...settings,
        terminalUserColors: { alice: '#123456' },
      },
    }))

    render(<SettingsView />)

    const bobColor = screen.getByLabelText('bob badge color value')
    fireEvent.change(bobColor, { target: { value: '#abcdef' } })

    expect(updateSettings).toHaveBeenCalledWith({
      terminalUserColors: {
        alice: '#123456',
        bob: '#abcdef',
      },
    })
  })

  it('toggles tmux mouse-wheel scrolling explicitly', () => {
    const updateSettings = vi.fn()
    mockUseSession.mockReturnValue(sessionReturn(updateSettings, {
      settings: {
        ...settings,
        mouseScroll: true,
      },
    }))

    render(<SettingsView />)

    const checkbox = screen.getByLabelText('Mouse-wheel scrolling')
    expect(checkbox).toBeChecked()
    fireEvent.click(checkbox)

    expect(updateSettings).toHaveBeenCalledWith({ mouseScroll: false })
  })

  it('toggles the terminal scrollbar visibility setting', () => {
    const updateSettings = vi.fn()
    mockUseSession.mockReturnValue(sessionReturn(updateSettings, {
      settings: {
        ...settings,
        hideScrollbar: true,
      },
    }))

    render(<SettingsView />)

    const checkbox = screen.getByLabelText('Hide terminal scrollbar')
    expect(checkbox).toBeChecked()
    fireEvent.click(checkbox)

    expect(updateSettings).toHaveBeenCalledWith({ hideScrollbar: false })
  })

  it('keeps bulk session destruction in advanced Settings', () => {
    const updateSettings = vi.fn()
    mockUseSession.mockReturnValue(sessionReturn(updateSettings, {
      sessions: [{ name: 'shell', windows: 1, attached: false, group: 'shell' }],
    }))

    render(<SettingsView />)
    expect(screen.getByRole('button', { name: /Nuke All/i })).toBeInTheDocument()
  })
})

describe('SettingsView Beads project paths', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    Object.assign(navigator, { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } })
  })

  it('adds a typed Beads project path without needing the folder picker', () => {
    const updateSettings = vi.fn()
    mockUseSession.mockReturnValue(sessionReturn(updateSettings, {
      settings: {
        ...settings,
        beadsProjectPaths: ['/home/operator/chrote'],
      },
    }))

    render(<SettingsView />)

    expect(screen.getByLabelText('Beads project path')).toHaveAttribute('placeholder', '/workspace/project')

    fireEvent.change(screen.getByLabelText('Beads project path'), {
      target: { value: ' /home/secondary/velvetwood/ ' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Add Path' }))

    expect(updateSettings).toHaveBeenCalledWith({
      beadsProjectPaths: ['/home/operator/chrote', '/home/secondary/velvetwood'],
    })
  })

  it('does not add duplicate Beads project paths after normalizing trailing slashes', () => {
    const updateSettings = vi.fn()
    mockUseSession.mockReturnValue(sessionReturn(updateSettings, {
      settings: {
        ...settings,
        beadsProjectPaths: ['/home/secondary/velvetwood'],
      },
    }))

    render(<SettingsView />)

    fireEvent.change(screen.getByLabelText('Beads project path'), {
      target: { value: '/home/secondary/velvetwood/' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Add Path' }))

    expect(updateSettings).not.toHaveBeenCalled()
  })

  it('removes a configured Beads project path with an accessible remove button', () => {
    const updateSettings = vi.fn()
    mockUseSession.mockReturnValue(sessionReturn(updateSettings, {
      settings: {
        ...settings,
        beadsProjectPaths: ['/home/operator/chrote', '/home/secondary/velvetwood'],
      },
    }))

    render(<SettingsView />)

    fireEvent.click(screen.getByRole('button', { name: 'Remove /home/secondary/velvetwood' }))

    expect(updateSettings).toHaveBeenCalledWith({
      beadsProjectPaths: ['/home/operator/chrote'],
    })
  })
})
