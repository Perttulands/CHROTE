import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AgentsView from './index'
import { DEFAULT_SETTINGS } from '../../types'
import type { AgentContext } from '../../agents/agentContextApi'
import type { Workspace } from '../../workspaces/workspacesApi'

const mockState = vi.hoisted(() => ({
  announce: vi.fn(),
  fetchAgentContext: vi.fn(),
  fetchWorkspaces: vi.fn(),
  openSendToSession: vi.fn(),
}))

vi.mock('../../context/SessionContext', () => ({
  useSession: () => ({
    settings: DEFAULT_SETTINGS,
    terminalUsers: ['operator'],
    openSendToSession: mockState.openSendToSession,
  }),
}))

vi.mock('../../context/StatusContext', () => ({
  useStatus: () => ({ announce: mockState.announce }),
}))

vi.mock('../../agents/agentContextApi', async () => {
  const actual = await vi.importActual<typeof import('../../agents/agentContextApi')>('../../agents/agentContextApi')
  return {
    ...actual,
    fetchAgentContext: (folder: string, harness: string, user: string) =>
      mockState.fetchAgentContext(folder, harness, user),
  }
})

vi.mock('../../workspaces/workspacesApi', async () => {
  const actual = await vi.importActual<typeof import('../../workspaces/workspacesApi')>('../../workspaces/workspacesApi')
  return { ...actual, fetchWorkspaces: () => mockState.fetchWorkspaces() }
})

vi.mock('../ResidentColumn', () => ({
  default: ({ tab, reference }: { tab: string; reference: string | null }) => (
    <div data-testid="resident">{`${tab}: ${reference ?? ''}`}</div>
  ),
}))

const workspaces: Workspace[] = [
  { path: '/home/operator', sources: ['session'], sessions: ['claude-home'], instructions: 3, lastActivity: '2026-09-03T12:00:00Z' },
  { path: '/srv/chrote', sources: ['git', 'store'], sessions: [], instructions: 3 },
]

function context(folder: string, harness: string): AgentContext {
  return {
    folder,
    harness: harness as AgentContext['harness'],
    user: 'operator',
    instructions: [{ path: `${folder}/CLAUDE.md`, scope: 'project', kind: 'CLAUDE.md', readable: true, size: 10 }],
    skills: [],
    memories: [],
  }
}

describe('AgentsView', () => {
  beforeEach(() => {
    mockState.announce.mockReset()
    mockState.fetchAgentContext.mockReset()
    mockState.fetchWorkspaces.mockReset()
    mockState.openSendToSession.mockReset()
    mockState.fetchWorkspaces.mockResolvedValue(workspaces)
    mockState.fetchAgentContext.mockImplementation((folder: string, harness: string) =>
      Promise.resolve(context(folder, harness)))
  })

  it('resolves the first workspace under Claude Code and lists its stack', async () => {
    render(<AgentsView />)

    await waitFor(() => expect(screen.getByText('/home/operator/CLAUDE.md')).toBeInTheDocument())
    expect(mockState.fetchAgentContext).toHaveBeenCalledWith('/home/operator', 'claude-code', 'operator')
  })

  it('distinguishes pending discovery from an empty workspace list', async () => {
    let answer!: (found: Workspace[]) => void
    mockState.fetchWorkspaces.mockReturnValue(new Promise<Workspace[]>(resolve => { answer = resolve }))
    render(<AgentsView />)
    expect(screen.getByText('Loading workspaces…')).toBeInTheDocument()
    expect(screen.queryByText('No workspace found under the roots.')).not.toBeInTheDocument()
    await act(async () => { answer([]) })
    expect(screen.queryByText('Loading workspaces…')).not.toBeInTheDocument()
    expect(screen.getByText('No workspace found under the roots.')).toBeInTheDocument()
  })

  it('lists the folders live sessions run in before the rest', async () => {
    render(<AgentsView />)
    await waitFor(() => expect(screen.getByText('/srv/chrote')).toBeInTheDocument())

    const headings = screen.getAllByRole('heading', { level: 3 }).map(heading => heading.textContent)
    expect(headings.slice(headings.indexOf('Running'), headings.indexOf('Running') + 2)).toEqual(['Running', 'Projects'])
    const rows = [...document.querySelectorAll('.agents-workspaces .agents-rail-row[aria-pressed]')].map(row => row.textContent)
    expect(rows).toEqual(['/home/operator3', '/srv/chrote3'])
  })

  it('resolves the workspace the operator picks', async () => {
    render(<AgentsView />)
    await waitFor(() => expect(screen.getByText('/srv/chrote')).toBeInTheDocument())

    fireEvent.click(screen.getByText('/srv/chrote'))

    await waitFor(() => expect(mockState.fetchAgentContext)
      .toHaveBeenCalledWith('/srv/chrote', 'claude-code', 'operator'))
  })

  it('asks the same question of the other harness', async () => {
    render(<AgentsView />)
    await waitFor(() => expect(screen.getByText('/home/operator/CLAUDE.md')).toBeInTheDocument())

    fireEvent.click(screen.getByText('Codex'))

    await waitFor(() => expect(mockState.fetchAgentContext)
      .toHaveBeenCalledWith('/home/operator', 'codex', 'operator'))
  })

  it('refreshes workspace discovery without resetting the current stack or filter', async () => {
    render(<AgentsView />)
    await screen.findByText('/home/operator/CLAUDE.md')
    fireEvent.change(screen.getByLabelText('Filter skills and memories'), { target: { value: 'draft filter' } })
    mockState.fetchWorkspaces.mockResolvedValue([
      { path: '/srv/new-project', sources: ['git'], sessions: [], instructions: 1 },
      ...workspaces,
    ])

    fireEvent.click(screen.getByRole('button', { name: 'Refresh workspaces' }))

    await screen.findByText('/srv/new-project')
    expect(mockState.fetchWorkspaces).toHaveBeenCalledTimes(2)
    expect(mockState.fetchAgentContext).toHaveBeenCalledTimes(1)
    expect(screen.getByLabelText('Filter skills and memories')).toHaveValue('draft filter')
    expect(screen.getByTestId('resident')).toHaveTextContent('agents /home/operator claude-code')
  })

  it('hands the tender the chosen workspace and harness', async () => {
    render(<AgentsView />)
    await waitFor(() => expect(screen.getByText('/home/operator/CLAUDE.md')).toBeInTheDocument())
    expect(screen.getByTestId('resident')).toHaveTextContent('agents: agents /home/operator claude-code')
  })

  it('offers a workspace row an agent, the Files tab and a message', async () => {
    const openInFiles = vi.fn()
    render(<AgentsView onOpenInFiles={openInFiles} />)
    await waitFor(() => expect(screen.getByText('/srv/chrote')).toBeInTheDocument())

    fireEvent.contextMenu(screen.getByText('/srv/chrote'))
    expect(screen.getAllByRole('menuitem').map(item => item.querySelector('.menu-row-label')?.textContent))
      .toEqual(['Launch here', 'Open in Files', 'Send'])

    fireEvent.click(screen.getByRole('menuitem', { name: 'Open in Files' }))
    expect(openInFiles).toHaveBeenCalledWith('/srv/chrote')

    fireEvent.contextMenu(screen.getByText('/srv/chrote'))
    fireEvent.click(screen.getByRole('menuitem', { name: 'Launch here' }))
    expect(mockState.openSendToSession).toHaveBeenCalledWith({
      reference: 'agents /srv/chrote claude-code',
      launch: { label: 'Launch in chrote', folder: '/srv/chrote', harness: 'claude-code' },
    })
  })

  it('says what the workspace holds on the status line', async () => {
    render(<AgentsView />)

    await waitFor(() => expect(mockState.announce).toHaveBeenCalledWith(
      '/home/operator under Claude Code: 1 instruction file, 0 skills, 0 memories',
      'info',
    ))
  })
})
