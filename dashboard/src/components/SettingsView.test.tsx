import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
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
    managedSessions: [],
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

function recoveryDescriptor(overrides: Record<string, unknown> = {}) {
  return {
    mode: 'agent',
    owner: { kind: 'session_bank', ref: 'alice/codex-alpha', mayRestart: true },
    topology: {
      sessionName: 'codex-alpha',
      sessionId: '$7',
      windowIndex: 0,
      windowName: 'agents',
      windowLayout: 'b25f,80x24,0,0',
      paneIndex: 0,
      paneId: '%1',
      paneCurrentPath: '/home/alice/chrote',
    },
    workloadKind: 'codex',
    agent: {
      kind: 'codex',
      nativeSessionId: '019f45ec-f88b-7f70-88dc-b5b99a9e94c6',
    },
    evidenceSource: 'argv',
    confidence: 'high',
    ...overrides,
  }
}

function descriptorBankedSession(overrides: Record<string, unknown> = {}) {
  return agentBankedSession({
    resumeCommand: '',
    recoveryKind: 'descriptor-plan',
    recoveryPlan: [recoveryDescriptor()],
    ...overrides,
  })
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
      sessionBank: [descriptorBankedSession(), shellBankedSession()],
    }))

    render(<SettingsView />)

    expect(screen.getByRole('heading', { name: 'Session Bank' })).toBeInTheDocument()
    expect(screen.getByText('2 banked')).toBeInTheDocument()
    expect(screen.getByText('1 workload recoverable')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Expand session bank' })).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByText('codex-alpha')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Expand session bank' }))

    expect(screen.getByText('codex-alpha')).toBeInTheDocument()
    expect(screen.getByText('shell-archive')).toBeInTheDocument()
    expect(screen.getByText('Workload recoverable')).toBeInTheDocument()
    expect(screen.getByText('Legacy no plan')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Recover workload for codex-alpha' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Restore topology only for codex-alpha' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Resume agent for codex-alpha' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Copy resume command for shell-archive' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Recreate shell for shell-archive' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Remove shell-archive from session bank' })).toBeInTheDocument()
  })

  it('makes nonzero Session Bank topology-only, managed, and unresolved counts visible in the header', () => {
    const updateSettings = vi.fn()
    mockUseSession.mockReturnValue(sessionReturn(updateSettings, {
      sessionBank: [
        descriptorBankedSession(),
        descriptorBankedSession({
          name: 'shape-only',
          recoveryPlan: [
            recoveryDescriptor({
              mode: 'topology',
              owner: { kind: 'session_bank', ref: 'alice/shape-only', mayRestart: true },
              topology: { ...recoveryDescriptor().topology, sessionName: 'shape-only' },
              workloadKind: 'shell',
              agent: undefined,
              evidenceSource: 'topology',
              confidence: 'medium',
            }),
          ],
        }),
        descriptorBankedSession({
          name: 'systemd-worker',
          recoveryPlan: [
            recoveryDescriptor({
              mode: 'managed',
              owner: { kind: 'external_manager', ref: 'systemd:user/velis.service', mayRestart: false },
              topology: { ...recoveryDescriptor().topology, sessionName: 'systemd-worker' },
              workloadKind: 'managed',
              agent: undefined,
              evidenceSource: 'manager',
            }),
          ],
        }),
        descriptorBankedSession({
          name: 'mixed-agent',
          resumeCommand: 'codex resume stale-legacy-id',
          recoveryPlan: [
            recoveryDescriptor({
              owner: { kind: 'session_bank', ref: 'alice/mixed-agent', mayRestart: true },
              topology: { ...recoveryDescriptor().topology, sessionName: 'mixed-agent' },
            }),
            recoveryDescriptor({
              mode: 'unresolved',
              owner: { kind: 'session_bank', ref: 'alice/mixed-agent', mayRestart: false },
              workloadKind: 'unknown',
              agent: undefined,
              evidenceSource: 'process',
              confidence: 'low',
              unresolvedReason: 'conflicting_evidence',
              topology: { ...recoveryDescriptor().topology, sessionName: 'mixed-agent', paneIndex: 1, paneId: '%2' },
            }),
          ],
        }),
        shellBankedSession(),
      ],
    }))

    render(<SettingsView />)

    expect(screen.getByText('5 banked')).toBeInTheDocument()
    expect(screen.getByText('1 workload recoverable')).toBeInTheDocument()
    expect(screen.getByText('1 topology only')).toBeInTheDocument()
    expect(screen.getByText('1 managed')).toBeInTheDocument()
    expect(screen.getByText('1 unresolved')).toBeInTheDocument()
  })

  it('keeps Session Bank workload recovery, topology-only restore, and remove actions working from Settings', async () => {
    const updateSettings = vi.fn()
    mockUseSession.mockReturnValue(sessionReturn(updateSettings, {
      sessionBank: [descriptorBankedSession()],
    }))

    render(<SettingsView />)
    fireEvent.click(screen.getByRole('button', { name: 'Expand session bank' }))

    expect(screen.queryByRole('button', { name: 'Copy resume command for codex-alpha' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Recreate shell for codex-alpha' })).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Recover workload for codex-alpha' }))
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(
      '/api/tmux/session-bank/codex-alpha/recover?unixUser=alice',
      expect.objectContaining({
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ mouseScroll: DEFAULT_SETTINGS.mouseScroll }),
      }),
    ))
    expect(addToast).toHaveBeenCalledWith('Recovered workload codex-alpha', 'success')
    expect(refreshSessions).toHaveBeenCalled()

    fetchMock.mockClear()
    fireEvent.click(screen.getByRole('button', { name: 'Restore topology only for codex-alpha' }))
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(
      '/api/tmux/session-bank/codex-alpha/recover?unixUser=alice',
      expect.objectContaining({
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ topologyOnly: true }),
      }),
    ))
    expect(addToast).toHaveBeenCalledWith('Restored topology for codex-alpha without launching workloads', 'success')

    fetchMock.mockClear()
    fireEvent.click(screen.getByRole('button', { name: 'Remove codex-alpha from session bank' }))
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(
      '/api/tmux/session-bank/codex-alpha?unixUser=alice',
      expect.objectContaining({ method: 'DELETE' }),
    ))
    expect(addToast).toHaveBeenCalledWith('Removed codex-alpha from session bank', 'success')
  })

  it('keeps legacy recreate shell and resume-copy fallback limited to entries without a typed plan', async () => {
    const updateSettings = vi.fn()
    mockUseSession.mockReturnValue(sessionReturn(updateSettings, {
      sessionBank: [
        shellBankedSession({ resumeCommand: 'tmux attach -t shell-archive' }),
        shellBankedSession({ name: 'shell-fallback', resumeCommand: '' }),
      ],
    }))

    render(<SettingsView />)
    fireEvent.click(screen.getByRole('button', { name: 'Expand session bank' }))

    fireEvent.click(screen.getByRole('button', { name: 'Copy resume command for shell-archive' }))
    await waitFor(() => expect(navigator.clipboard.writeText).toHaveBeenCalledWith('tmux attach -t shell-archive'))
    expect(addToast).toHaveBeenCalledWith('Resume command copied', 'success')

    expect(screen.getByText('/resume shell-fallback')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Copy resume command for shell-fallback' })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Recreate shell for shell-archive' }))
    await waitFor(() => expect(createSession).toHaveBeenCalledWith({ workspaceId: 'terminal1', unixUser: 'alice', name: 'shell-archive' }))
  })

  it('fails closed when malformed-present recovery plans include stale resume commands', () => {
    const updateSettings = vi.fn()
    mockUseSession.mockReturnValue(sessionReturn(updateSettings, {
      sessionBank: [
        shellBankedSession({
          name: 'wrapped-plan',
          resumeCommand: 'tmux attach -t wrapped-plan',
          recoveryPlan: { descriptors: [recoveryDescriptor()] },
        }),
        shellBankedSession({
          name: 'empty-plan',
          resumeCommand: 'tmux attach -t empty-plan',
          recoveryPlan: [],
        }),
        shellBankedSession({
          name: 'bad-descriptor',
          resumeCommand: 'tmux attach -t bad-descriptor',
          recoveryPlan: [null],
        }),
      ] as unknown as ReturnType<typeof shellBankedSession>[],
    }))

    render(<SettingsView />)

    expect(screen.getByText('3 banked')).toBeInTheDocument()
    expect(screen.getByText('3 unresolved')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Expand session bank' }))

    ;[
      ['wrapped-plan', 'tmux attach -t wrapped-plan'],
      ['empty-plan', 'tmux attach -t empty-plan'],
      ['bad-descriptor', 'tmux attach -t bad-descriptor'],
    ].forEach(([name, staleCommand]) => {
      const entry = within(screen.getByRole('article', { name: `Session Bank entry ${name}` }))
      expect(entry.getByText('Unresolved / unsafe')).toBeInTheDocument()
      expect(entry.queryByRole('button', { name: `Restore topology only for ${name}` })).not.toBeInTheDocument()
      expect(entry.queryByRole('button', { name: `Copy resume command for ${name}` })).not.toBeInTheDocument()
      expect(entry.queryByRole('button', { name: `Recreate shell for ${name}` })).not.toBeInTheDocument()
      expect(entry.getByRole('button', { name: `Remove ${name} from session bank` })).toBeInTheDocument()
      expect(entry.queryByText(staleCommand)).not.toBeInTheDocument()
    })
  })

  it('renders typed plan-shape failures as unresolved cleanup-only entries', () => {
    const updateSettings = vi.fn()
    mockUseSession.mockReturnValue(sessionReturn(updateSettings, {
      sessionBank: [
        descriptorBankedSession({
          name: 'duplicate-pane-id',
          resumeCommand: 'tmux attach -t duplicate-pane-id',
          recoveryPlan: [
            recoveryDescriptor({
              owner: { kind: 'session_bank', ref: 'alice/duplicate-pane-id', mayRestart: true },
              topology: { ...recoveryDescriptor().topology, sessionName: 'duplicate-pane-id', paneId: '%same' },
            }),
            recoveryDescriptor({
              owner: { kind: 'session_bank', ref: 'alice/duplicate-pane-id', mayRestart: true },
              topology: { ...recoveryDescriptor().topology, sessionName: 'duplicate-pane-id', paneIndex: 1, paneId: '%same' },
            }),
          ],
        }),
      ],
    }))

    render(<SettingsView />)

    expect(screen.getByText('1 unresolved')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Expand session bank' }))

    const entry = within(screen.getByRole('article', { name: 'Session Bank entry duplicate-pane-id' }))
    expect(entry.getByText('Unresolved / unsafe')).toBeInTheDocument()
    expect(entry.queryByRole('button', { name: 'Restore topology only for duplicate-pane-id' })).not.toBeInTheDocument()
    expect(entry.queryByRole('button', { name: 'Recover workload for duplicate-pane-id' })).not.toBeInTheDocument()
    expect(entry.queryByRole('button', { name: 'Copy resume command for duplicate-pane-id' })).not.toBeInTheDocument()
    expect(entry.queryByRole('button', { name: 'Recreate shell for duplicate-pane-id' })).not.toBeInTheDocument()
    expect(entry.getByRole('button', { name: 'Remove duplicate-pane-id from session bank' })).toBeInTheDocument()
    expect(entry.queryByText('tmux attach -t duplicate-pane-id')).not.toBeInTheDocument()
  })

  it('renders managed registry entries as read-only and unresolved bank plans as cleanup-only', () => {
    const updateSettings = vi.fn()
    mockUseSession.mockReturnValue(sessionReturn(updateSettings, {
      sessionBank: [
        descriptorBankedSession({
          name: 'mixed-agent',
          resumeCommand: 'codex resume stale-legacy-id',
          recoveryPlan: [
            recoveryDescriptor({
              owner: { kind: 'session_bank', ref: 'alice/mixed-agent', mayRestart: true },
              topology: { ...recoveryDescriptor().topology, sessionName: 'mixed-agent' },
            }),
            recoveryDescriptor({
              mode: 'unresolved',
              owner: { kind: 'session_bank', ref: 'alice/mixed-agent', mayRestart: false },
              workloadKind: 'unknown',
              agent: undefined,
              evidenceSource: 'process',
              confidence: 'low',
              unresolvedReason: 'conflicting_evidence',
              topology: { ...recoveryDescriptor().topology, sessionName: 'mixed-agent', paneIndex: 1, paneId: '%2' },
            }),
          ],
        }),
      ],
      managedSessions: [
        {
          name: 'systemd-worker',
          sessionName: 'systemd-worker',
          unixUser: 'alice',
          owner: { kind: 'external_manager', ref: 'systemd:user/velis.service', mayRestart: false },
          managerKind: 'systemd-user',
          managerRef: 'velis.service',
          status: { ok: true, activeState: 'active', checkedAt: '2026-07-15T10:00:00Z' },
          storageKind: 'managed-status',
          sourceKind: 'restore',
        },
      ],
    }))

    render(<SettingsView />)
    fireEvent.click(screen.getByRole('button', { name: 'Expand session bank' }))

    expect(screen.getByText('1 managed')).toBeInTheDocument()

    const managed = within(screen.getByRole('article', { name: 'Managed session systemd-worker' }))
    expect(managed.getByText('Managed read-only')).toBeInTheDocument()
    expect(managed.getByText('Owner external_manager · systemd:user/velis.service')).toBeInTheDocument()
    expect(managed.getByText('Manager systemd-user · velis.service')).toBeInTheDocument()
    expect(managed.getByText('Status active · OK')).toBeInTheDocument()
    expect(managed.queryByRole('button', { name: /Recover workload/i })).not.toBeInTheDocument()
    expect(managed.queryByRole('button', { name: /Restore topology only/i })).not.toBeInTheDocument()
    expect(managed.queryByRole('button', { name: /Copy resume command/i })).not.toBeInTheDocument()
    expect(managed.queryByRole('button', { name: /Recreate shell/i })).not.toBeInTheDocument()
    expect(managed.queryByRole('button', { name: /Remove/i })).not.toBeInTheDocument()

    const unresolved = within(screen.getByRole('article', { name: 'Session Bank entry mixed-agent' }))
    expect(unresolved.getByText('Unresolved / unsafe')).toBeInTheDocument()
    expect(unresolved.getByText(/conflicting_evidence/)).toBeInTheDocument()
    expect(unresolved.queryByRole('button', { name: 'Recover workload for mixed-agent' })).not.toBeInTheDocument()
    expect(unresolved.queryByRole('button', { name: 'Restore topology only for mixed-agent' })).not.toBeInTheDocument()
    expect(unresolved.queryByRole('button', { name: 'Copy resume command for mixed-agent' })).not.toBeInTheDocument()
    expect(unresolved.queryByRole('button', { name: 'Recreate shell for mixed-agent' })).not.toBeInTheDocument()
    expect(unresolved.getByRole('button', { name: 'Remove mixed-agent from session bank' })).toBeInTheDocument()
    expect(unresolved.queryByText('codex resume stale-legacy-id')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Make Persistent/i })).not.toBeInTheDocument()
  })

  it('surfaces precise backend messages for Session Bank recover and forget failures', async () => {
    const updateSettings = vi.fn()
    mockUseSession.mockReturnValue(sessionReturn(updateSettings, {
      sessionBank: [descriptorBankedSession()],
    }))

    render(<SettingsView />)
    fireEvent.click(screen.getByRole('button', { name: 'Expand session bank' }))

    fetchMock.mockResolvedValueOnce({
      ok: false,
      text: async () => JSON.stringify({ error: { message: 'unresolved recovery descriptor requires topologyOnly' } }),
    })
    fireEvent.click(screen.getByRole('button', { name: 'Recover workload for codex-alpha' }))
    await waitFor(() => expect(addToast).toHaveBeenCalledWith('unresolved recovery descriptor requires topologyOnly', 'error'))

    fetchMock.mockResolvedValueOnce({
      ok: false,
      text: async () => JSON.stringify({ message: 'session bank: open /srv/data/chrote/session-bank/sessions.json: permission denied' }),
    })
    fireEvent.click(screen.getByRole('button', { name: 'Remove codex-alpha from session bank' }))
    await waitFor(() => expect(addToast).toHaveBeenCalledWith('session bank: open /srv/data/chrote/session-bank/sessions.json: permission denied', 'error'))
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
