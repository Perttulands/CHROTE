import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import SettingsView from './SettingsView'
import { DEFAULT_SETTINGS } from '../types'

const mockUseSession = vi.fn()

vi.mock('../context/SessionContext', () => ({
  useSession: () => mockUseSession(),
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

describe('SettingsView terminal launch users', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('lets each terminal tab choose the Unix user used for new shells from configured users', () => {
    const updateSettings = vi.fn()
    mockUseSession.mockReturnValue({
      settings,
      terminalUsers,
      updateSettings,
    })

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
    mockUseSession.mockReturnValue({
      settings,
      terminalUsers,
      updateSettings,
    })

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
    mockUseSession.mockReturnValue({
      settings: {
        ...settings,
        terminalUserColors: { alice: '#123456' },
      },
      terminalUsers,
      updateSettings,
    })

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
    mockUseSession.mockReturnValue({
      settings: {
        ...settings,
        mouseScroll: true,
      },
      terminalUsers,
      updateSettings,
    })

    render(<SettingsView />)

    const checkbox = screen.getByLabelText('Mouse-wheel scrolling')
    expect(checkbox).toBeChecked()
    fireEvent.click(checkbox)

    expect(updateSettings).toHaveBeenCalledWith({ mouseScroll: false })
  })
})

describe('SettingsView Beads project paths', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('adds a typed Beads project path without needing the folder picker', () => {
    const updateSettings = vi.fn()
    mockUseSession.mockReturnValue({
      settings: {
        ...settings,
        beadsProjectPaths: ['/home/perttu/chrote'],
      },
      terminalUsers,
      updateSettings,
    })

    render(<SettingsView />)

    fireEvent.change(screen.getByLabelText('Beads project path'), {
      target: { value: ' /home/tavern/velvetwood/ ' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Add Path' }))

    expect(updateSettings).toHaveBeenCalledWith({
      beadsProjectPaths: ['/home/perttu/chrote', '/home/tavern/velvetwood'],
    })
  })

  it('does not add duplicate Beads project paths after normalizing trailing slashes', () => {
    const updateSettings = vi.fn()
    mockUseSession.mockReturnValue({
      settings: {
        ...settings,
        beadsProjectPaths: ['/home/tavern/velvetwood'],
      },
      terminalUsers,
      updateSettings,
    })

    render(<SettingsView />)

    fireEvent.change(screen.getByLabelText('Beads project path'), {
      target: { value: '/home/tavern/velvetwood/' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Add Path' }))

    expect(updateSettings).not.toHaveBeenCalled()
  })

  it('removes a configured Beads project path with an accessible remove button', () => {
    const updateSettings = vi.fn()
    mockUseSession.mockReturnValue({
      settings: {
        ...settings,
        beadsProjectPaths: ['/home/perttu/chrote', '/home/tavern/velvetwood'],
      },
      terminalUsers,
      updateSettings,
    })

    render(<SettingsView />)

    fireEvent.click(screen.getByRole('button', { name: 'Remove /home/tavern/velvetwood' }))

    expect(updateSettings).toHaveBeenCalledWith({
      beadsProjectPaths: ['/home/perttu/chrote'],
    })
  })
})
