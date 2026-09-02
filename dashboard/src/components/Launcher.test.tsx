import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import Launcher, { derivedSessionName, folderBasename, recentFolders } from './Launcher'
import { DEFAULT_SETTINGS } from '../types'
import type { TmuxSession } from '../types'

const createSession = vi.fn()
const fetchMock = vi.fn()

const mockState = vi.hoisted(() => ({ sessions: [] as TmuxSession[] }))

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({
    sessions: mockState.sessions,
    settings: {
      ...DEFAULT_SETTINGS,
      terminalLaunchUsers: { ...DEFAULT_SETTINGS.terminalLaunchUsers, terminal3: 'build' },
    },
    terminalUsers: ['alice', 'build'],
    createSession,
  }),
}))

vi.mock('./FolderPickerModal', () => ({
  default: ({ onSelect }: { onSelect: (path: string) => void }) => (
    <button type="button" onClick={() => onSelect('/srv/picked')}>pick /srv/picked</button>
  ),
}))

const launchOptions = {
  harnesses: [
    { id: 'claude-code', label: 'Claude Code' },
    { id: 'codex', label: 'Codex' },
    { id: 'shell', label: 'Shell' },
  ],
  folders: ['/srv/chrote', '/srv', '~'],
}

function session(name: string, extra: Partial<TmuxSession> = {}): TmuxSession {
  return { name, windows: 1, attached: false, group: 'g', unixUser: 'build', ...extra }
}

describe('launcher name derivation', () => {
  it('says the harness and the folder, in the tmux alphabet', () => {
    expect(folderBasename('/srv/chrote')).toBe('chrote')
    expect(folderBasename('/srv/chrote/')).toBe('chrote')
    expect(folderBasename('~')).toBe('home')
    expect(folderBasename('~/work/agent formations')).toBe('agent-formations')
    expect(folderBasename('/')).toBe('root')
    expect(derivedSessionName('claude-code', '/srv/chrote', [], 'build')).toBe('claude-chrote')
    expect(derivedSessionName('shell', '~', [], 'build')).toBe('shell-home')
  })

  it('numbers past the live sessions of that user only', () => {
    const sessions = [
      session('claude-chrote'),
      session('claude-chrote-2'),
      session('claude-chrote-3', { unixUser: 'alice' }),
    ]
    expect(derivedSessionName('claude-code', '/srv/chrote', sessions, 'build')).toBe('claude-chrote-3')
    expect(derivedSessionName('claude-code', '/srv/chrote', sessions, 'alice')).toBe('claude-chrote')
  })
})

describe('launcher recent folders', () => {
  it('lists the newest distinct folders of that user, minus the pinned ones', () => {
    const sessions = [
      session('one', { id: '$3', cwd: '/srv/context-citadel' }),
      session('two', { id: '$9', cwd: '/srv/chrote-agent-formations' }),
      session('three', { id: '$7', cwd: '/srv/context-citadel' }),
      session('four', { id: '$11', cwd: '/srv/chrote' }),
      session('five', { id: '$5', cwd: '/home/alice', unixUser: 'alice' }),
      session('six', { id: '$1' }),
    ]

    expect(recentFolders(sessions, 'build', ['/srv/chrote', '~']))
      .toEqual(['/srv/chrote-agent-formations', '/srv/context-citadel'])
  })

  it('stops at five', () => {
    const sessions = Array.from({ length: 8 }, (_, index) => session(`s${index}`, {
      id: `$${index + 1}`,
      cwd: `/srv/p${index}`,
    }))

    expect(recentFolders(sessions, 'build', [])).toEqual(['/srv/p7', '/srv/p6', '/srv/p5', '/srv/p4', '/srv/p3'])
  })
})

describe('Launcher', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockState.sessions = []
    createSession.mockResolvedValue('claude-chrote')
    fetchMock.mockImplementation((input: unknown) => (
      String(input).includes('/api/launch')
        ? Promise.resolve({ ok: true, json: async () => launchOptions })
        : Promise.resolve({ ok: true, json: async () => ({}) })
    ))
    vi.stubGlobal('fetch', fetchMock)
  })

  it('offers the configured harnesses and folders, with the first of each chosen', async () => {
    render(<Launcher workspaceId="terminal3" />)

    const claude = await screen.findByRole('button', { name: 'Claude Code' })
    expect(claude).toHaveClass('selected')
    expect(screen.getByRole('button', { name: 'Codex' })).not.toHaveClass('selected')
    expect(screen.getByRole('button', { name: '/srv/chrote' })).toHaveClass('selected')
    expect(screen.getByLabelText('Session name')).toHaveValue('claude-chrote')
    expect(screen.getByRole('button', { name: 'Launch claude in chrote' })).toBeInTheDocument()
    expect(fetchMock).toHaveBeenCalledWith('/api/launch', expect.anything())
  })

  it('re-derives the name until the operator types one, then keeps his', async () => {
    render(<Launcher workspaceId="terminal3" />)

    fireEvent.click(await screen.findByRole('button', { name: 'Codex' }))
    expect(screen.getByLabelText('Session name')).toHaveValue('codex-chrote')

    fireEvent.click(screen.getByRole('button', { name: '/srv' }))
    expect(screen.getByLabelText('Session name')).toHaveValue('codex-srv')
    expect(screen.getByRole('button', { name: 'Launch codex in srv' })).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText('Session name'), { target: { value: 'nightwatch' } })
    fireEvent.click(screen.getByRole('button', { name: '~' }))
    expect(screen.getByLabelText('Session name')).toHaveValue('nightwatch')
  })

  it('says a shell is opened, not launched', async () => {
    render(<Launcher workspaceId="terminal3" />)

    fireEvent.click(await screen.findByRole('button', { name: 'Shell' }))

    expect(screen.getByRole('button', { name: 'Open shell in chrote' })).toBeInTheDocument()
    expect(screen.getByLabelText('Session name')).toHaveValue('shell-chrote')
  })

  it('launches the chosen harness in the chosen folder as the chosen user, bound to the window', async () => {
    const onLaunched = vi.fn()
    render(
      <Launcher
        workspaceId="terminal3"
        attachTo={{ workspaceId: 'terminal3', windowId: 'terminal3-window-1' }}
        onLaunched={onLaunched}
      />,
    )

    fireEvent.click(await screen.findByRole('button', { name: 'Codex' }))
    fireEvent.click(screen.getByRole('button', { name: 'alice' }))
    fireEvent.click(screen.getByRole('button', { name: 'Launch codex in chrote' }))

    await waitFor(() => expect(createSession).toHaveBeenCalledWith({
      name: 'codex-chrote',
      unixUser: 'alice',
      cwd: '/srv/chrote',
      harness: 'codex',
      workspaceId: 'terminal3',
      attachTo: { workspaceId: 'terminal3', windowId: 'terminal3-window-1' },
    }))
    await waitFor(() => expect(onLaunched).toHaveBeenCalled())
  })

  it('offers the folders this user is already working in', async () => {
    mockState.sessions = [
      session('one', { id: '$4', cwd: '/srv/context-citadel' }),
      session('two', { id: '$8', cwd: '/srv/chrote-agent-formations' }),
      session('three', { id: '$9', cwd: '/srv/chrote' }),
    ]
    render(<Launcher workspaceId="terminal3" />)

    const recents = await screen.findByText('Recent')
    expect([...recents.parentElement!.querySelectorAll('.launcher-recent-path')].map(node => node.textContent))
      .toEqual(['/srv/chrote-agent-formations', '/srv/context-citadel'])

    fireEvent.click(screen.getByRole('button', { name: '/srv/context-citadel' }))
    expect(screen.getByLabelText('Session name')).toHaveValue('claude-context-citadel')
  })

  it('takes a folder from the browser and starts there', async () => {
    render(<Launcher workspaceId="terminal3" />)

    fireEvent.click(await screen.findByRole('button', { name: 'Browse…' }))
    fireEvent.click(screen.getByRole('button', { name: 'pick /srv/picked' }))

    expect(screen.getByLabelText('Session name')).toHaveValue('claude-picked')
    fireEvent.click(screen.getByRole('button', { name: 'Launch claude in picked' }))

    await waitFor(() => expect(createSession).toHaveBeenCalledWith(expect.objectContaining({ cwd: '/srv/picked' })))
  })

  it('offers a shell at home when the server has no launch options to give', async () => {
    fetchMock.mockImplementation(() => Promise.resolve({ ok: false, status: 500, json: async () => ({}) }))
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    render(<Launcher workspaceId="terminal3" />)

    expect(await screen.findByRole('button', { name: 'Open shell in home' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Shell' })).toHaveClass('selected')
    expect(warn).toHaveBeenCalled()
    warn.mockRestore()
  })
})
