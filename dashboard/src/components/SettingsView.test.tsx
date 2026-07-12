import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import SettingsView from './SettingsView'
import { DEFAULT_SETTINGS } from '../types'

const mockUseSession = vi.fn()
const refreshSessions = vi.fn()
const createSession = vi.fn()
const addToast = vi.fn()
const fetchMock = vi.fn()

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
    sessionBank: [],
    sessions: [],
    refreshSessions,
    createSession,
    ...overrides,
  }
}

function agentBankedSession(overrides: Record<string, unknown> = {}) {
  return {
    id: '$7',
    name: 'codex-alpha',
    unixUser: 'alice',
    windows: 1,
    attached: false,
    group: 'codex',
    live: false,
    firstSeen: '2026-07-07T20:00:00Z',
    lastSeen: '2026-07-07T21:00:00Z',
    recoveryKind: 'agent',
    agentKind: 'codex',
    agentSessionId: '019f45ec-f88b-7f70-88dc-b5b99a9e94c6',
    resumeCommand: 'codex resume 019f45ec-f88b-7f70-88dc-b5b99a9e94c6',
    cwd: '/srv/chrote',
    ...overrides,
  }
}

function shellBankedSession(overrides: Record<string, unknown> = {}) {
  return {
    id: '$8',
    name: 'shell-archive',
    unixUser: 'alice',
    windows: 1,
    attached: false,
    group: 'shell',
    live: false,
    firstSeen: '2026-07-07T20:00:00Z',
    lastSeen: '2026-07-07T21:00:00Z',
    recoveryKind: 'shell',
    ...overrides,
  }
}

describe('SettingsView terminal launch users', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    createSession.mockResolvedValue('created')
    fetchMock.mockResolvedValue({ ok: true, json: async () => ({ success: true, removed: true }) })
    vi.stubGlobal('fetch', fetchMock)
    Object.assign(navigator, { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } })
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

  it('keeps bulk session destruction in advanced Settings', () => {
    const updateSettings = vi.fn()
    mockUseSession.mockReturnValue(sessionReturn(updateSettings, {
      sessions: [{ name: 'shell', windows: 1, attached: false, group: 'shell' }],
    }))

    render(<SettingsView />)
    expect(screen.getByRole('button', { name: /Nuke All/i })).toBeInTheDocument()
  })
})

describe('SettingsView Session Bank', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    createSession.mockResolvedValue('created')
    fetchMock.mockResolvedValue({ ok: true, json: async () => ({ success: true, removed: true }) })
    vi.stubGlobal('fetch', fetchMock)
    Object.assign(navigator, { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } })
  })

  it('renders the Session Bank as a collapsed Settings section, not a Terminal-tab card list', () => {
    const updateSettings = vi.fn()
    mockUseSession.mockReturnValue(sessionReturn(updateSettings, {
      sessionBank: [agentBankedSession(), shellBankedSession()],
    }))

    render(<SettingsView />)

    expect(screen.getByRole('heading', { name: 'Session Bank' })).toBeInTheDocument()
    expect(screen.getByText('2 recoverable')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Expand session bank' })).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByText('codex-alpha')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Expand session bank' }))

    expect(screen.getByText('codex-alpha')).toBeInTheDocument()
    expect(screen.getByText('shell-archive')).toBeInTheDocument()
    expect(screen.getByText('Recoverable agent')).toBeInTheDocument()
    expect(screen.getByText('Shell only')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Resume agent for codex-alpha' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Copy resume command for shell-archive' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Recreate shell for shell-archive' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Remove shell-archive from session bank' })).toBeInTheDocument()
  })

  it('keeps Session Bank resume, copy, recreate, and remove actions working from Settings', async () => {
    const updateSettings = vi.fn()
    mockUseSession.mockReturnValue(sessionReturn(updateSettings, {
      sessionBank: [agentBankedSession()],
    }))

    render(<SettingsView />)
    fireEvent.click(screen.getByRole('button', { name: 'Expand session bank' }))

    fireEvent.click(screen.getByRole('button', { name: 'Copy resume command for codex-alpha' }))
    await waitFor(() => expect(navigator.clipboard.writeText).toHaveBeenCalledWith('codex resume 019f45ec-f88b-7f70-88dc-b5b99a9e94c6'))
    expect(addToast).toHaveBeenCalledWith('Resume command copied', 'success')

    fireEvent.click(screen.getByRole('button', { name: 'Recreate shell for codex-alpha' }))
    await waitFor(() => expect(createSession).toHaveBeenCalledWith({ workspaceId: 'terminal1', unixUser: 'alice', name: 'codex-alpha' }))

    fireEvent.click(screen.getByRole('button', { name: 'Resume agent for codex-alpha' }))
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(
      '/api/tmux/session-bank/codex-alpha/recover?unixUser=alice',
      expect.objectContaining({
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ mouseScroll: DEFAULT_SETTINGS.mouseScroll }),
      }),
    ))
    expect(addToast).toHaveBeenCalledWith('Resumed agent codex-alpha', 'success')
    expect(refreshSessions).toHaveBeenCalled()

    fetchMock.mockClear()
    fireEvent.click(screen.getByRole('button', { name: 'Remove codex-alpha from session bank' }))
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(
      '/api/tmux/session-bank/codex-alpha?unixUser=alice',
      expect.objectContaining({ method: 'DELETE' }),
    ))
    expect(addToast).toHaveBeenCalledWith('Removed codex-alpha from session bank', 'success')
  })
})

describe('SettingsView Beads project paths', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    createSession.mockResolvedValue('created')
    fetchMock.mockResolvedValue({ ok: true, json: async () => ({ success: true, removed: true }) })
    vi.stubGlobal('fetch', fetchMock)
    Object.assign(navigator, { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } })
  })

  it('adds a typed Beads project path without needing the folder picker', () => {
    const updateSettings = vi.fn()
    mockUseSession.mockReturnValue(sessionReturn(updateSettings, {
      settings: {
        ...settings,
        beadsProjectPaths: ['/home/perttu/chrote'],
      },
    }))

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
    mockUseSession.mockReturnValue(sessionReturn(updateSettings, {
      settings: {
        ...settings,
        beadsProjectPaths: ['/home/tavern/velvetwood'],
      },
    }))

    render(<SettingsView />)

    fireEvent.change(screen.getByLabelText('Beads project path'), {
      target: { value: '/home/tavern/velvetwood/' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Add Path' }))

    expect(updateSettings).not.toHaveBeenCalled()
  })

  it('removes a configured Beads project path with an accessible remove button', () => {
    const updateSettings = vi.fn()
    mockUseSession.mockReturnValue(sessionReturn(updateSettings, {
      settings: {
        ...settings,
        beadsProjectPaths: ['/home/perttu/chrote', '/home/tavern/velvetwood'],
      },
    }))

    render(<SettingsView />)

    fireEvent.click(screen.getByRole('button', { name: 'Remove /home/tavern/velvetwood' }))

    expect(updateSettings).toHaveBeenCalledWith({
      beadsProjectPaths: ['/home/perttu/chrote'],
    })
  })
})
