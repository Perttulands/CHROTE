import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import SettingsView from './SettingsView'
import { DEFAULT_SETTINGS, TERMINAL_WORKSPACE_IDS } from '../types'
import { DEFAULT_THEME } from '../theme/theme'

const mockUseSession = vi.fn()
const refreshSessions = vi.fn()
const announce = vi.fn()

vi.mock('../context/SessionContext', () => ({
  useSession: () => mockUseSession(),
}))

vi.mock('../context/StatusContext', () => ({
  useStatus: () => ({ announce }),
}))

vi.mock('./FolderPickerModal', () => ({
  default: () => <div data-testid="folder-picker" />,
}))

const settings = {
  ...DEFAULT_SETTINGS,
  terminalLaunchUsers: {},
  terminalSessionPrefixes: {},
}

const terminalUsers = ['alice', 'bob']
const testDir = dirname(fileURLToPath(import.meta.url))

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

  it('renders and updates font-size and refresh-interval values', () => {
    const updateSettings = vi.fn()
    mockUseSession.mockReturnValue(sessionReturn(updateSettings, {
      settings: {
        ...settings,
        fontSize: 16,
        autoRefreshInterval: 10000,
      },
    }))

    render(<SettingsView />)

    const fontSize = screen.getByRole('slider')
    expect(fontSize).toHaveValue('16')
    fireEvent.change(fontSize, { target: { value: '18' } })
    expect(updateSettings).toHaveBeenCalledWith({ fontSize: 18 })

    const refresh = screen.getByText('Auto-refresh Interval').closest('.settings-field')?.querySelector('select')
    expect(refresh).toHaveValue('10000')
    fireEvent.change(refresh as HTMLSelectElement, { target: { value: '30000' } })
    expect(updateSettings).toHaveBeenCalledWith({ autoRefreshInterval: 30000 })
  })

  // One theme, served by the host, chosen nowhere in the browser. The stylesheet
  // holds it only so first paint matches what /api/theme is about to deliver.
  it('offers no theme picker and holds one :root block matching the built-in theme', () => {
    mockUseSession.mockReturnValue(sessionReturn(vi.fn()))
    render(<SettingsView />)

    expect(screen.queryByText('Theme')).toBeNull()
    expect(screen.queryByText('tmux Appearance')).toBeNull()
    expect(screen.queryByText('Session User Indicators')).toBeNull()

    const css = readFileSync(resolve(testDir, '../styles/theme-colors.css'), 'utf8')
    expect(css).not.toContain('data-theme')
    expect(css.match(/^:root \{/gm)).toHaveLength(1)
    expect(css).toContain(`--accent: ${DEFAULT_THEME.ui.accent};`)
    expect(css).toContain(`--background: ${DEFAULT_THEME.ui.background};`)
    expect(css).toContain(`--terminal-background: ${DEFAULT_THEME.terminal.background};`)
    expect(css).toContain(`--ansi-15: ${DEFAULT_THEME.terminal.ansi[15]};`)
    expect(css).toContain(`--identity-0: ${DEFAULT_THEME.identity[0]};`)
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

  // Bulk destruction lives here, and the byte-exact confirmation header is the
  // only thing standing between a stray click and every live tmux session. The
  // browser spec that used to pin it is gone; this owns it now.
  it('arms in place, names what is preserved, and only then sends DELETE with the exact confirmation header', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, text: () => Promise.resolve('') })
    vi.stubGlobal('fetch', fetchMock)

    mockUseSession.mockReturnValue(sessionReturn(vi.fn(), {
      sessions: [
        { name: 'shell', windows: 1, attached: false, group: 'shell' },
        { name: 'chrote-chat', windows: 1, attached: false, group: 'chrote' },
      ],
    }))

    render(<SettingsView />)

    // The button that destroys is the button that asks: nothing opens over the work.
    const nukeButton = screen.getByRole('button', { name: /Nuke All/i })
    fireEvent.click(nukeButton)

    expect(nukeButton).toHaveTextContent('Confirm: destroy 1 session')
    expect(screen.getByText(/Preserved: chrote-chat/)).toBeInTheDocument()
    expect(fetchMock).not.toHaveBeenCalled()

    fireEvent.click(nukeButton)

    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('/api/tmux/sessions/all')
    expect(init.method).toBe('DELETE')
    expect(init.headers).toEqual({ 'X-Nuke-Confirm': 'DASHBOARD-NUKE-CONFIRMED' })

    await waitFor(() => expect(announce).toHaveBeenCalledWith('All sessions destroyed', 'warning'))
    expect(refreshSessions).toHaveBeenCalled()

    vi.unstubAllGlobals()
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
