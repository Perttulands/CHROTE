import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AgentsView from './index'
import { DEFAULT_SETTINGS } from '../../types'
import type { AgentContext } from '../../agents/agentContextApi'
import type { Workspace } from '../../workspaces/workspacesApi'

const mockState = vi.hoisted(() => ({
  announce: vi.fn(),
  fetchAgentContext: vi.fn(),
  fetchWorkspaces: vi.fn(),
  fetchBeadWork: vi.fn(),
}))

vi.mock('../../context/SessionContext', () => ({
  useSession: () => ({ settings: DEFAULT_SETTINGS, terminalUsers: ['operator'] }),
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
    fetchAgentTender: () => Promise.resolve({ session: 'tender', beads: '/srv', folder: '/srv/ops/tender' }),
  }
})

vi.mock('../../workspaces/workspacesApi', async () => {
  const actual = await vi.importActual<typeof import('../../workspaces/workspacesApi')>('../../workspaces/workspacesApi')
  return { ...actual, fetchWorkspaces: () => mockState.fetchWorkspaces() }
})

vi.mock('../../beads/beadsApi', () => ({
  fetchBeadWork: (path: string) => mockState.fetchBeadWork(path),
}))

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
    mockState.fetchBeadWork.mockReset()
    mockState.fetchWorkspaces.mockResolvedValue(workspaces)
    mockState.fetchAgentContext.mockImplementation((folder: string, harness: string) =>
      Promise.resolve(context(folder, harness)))
    mockState.fetchBeadWork.mockResolvedValue({
      beads: [{ id: 'ctx-p3f', title: 'skill-doctrine-review', status: 'open', priority: 1, blocked: false }],
      prefix: 'ctx',
      projectPath: '/srv',
    })
  })

  it('resolves the first workspace under Claude Code and lists its stack', async () => {
    render(<AgentsView />)

    await waitFor(() => expect(screen.getByText('/home/operator/CLAUDE.md')).toBeInTheDocument())
    expect(mockState.fetchAgentContext).toHaveBeenCalledWith('/home/operator', 'claude-code', 'operator')
  })

  it('lists the folders live sessions run in before the rest', async () => {
    render(<AgentsView />)
    await waitFor(() => expect(screen.getByText('/srv/chrote')).toBeInTheDocument())

    const headings = screen.getAllByRole('heading', { level: 3 }).map(heading => heading.textContent)
    expect(headings.slice(headings.indexOf('Running'), headings.indexOf('Running') + 2)).toEqual(['Running', 'Projects'])
    const rows = [...document.querySelectorAll('.agents-workspaces .agents-rail-row')].map(row => row.textContent)
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

  it('lists the tender proposals and hands the tender the chosen workspace and harness', async () => {
    render(<AgentsView />)

    await waitFor(() => expect(screen.getByText('ctx-p3f')).toBeInTheDocument())
    expect(screen.getByTestId('resident')).toHaveTextContent('agents: agents /home/operator claude-code')
  })

  it('says what the workspace holds on the status line', async () => {
    render(<AgentsView />)

    await waitFor(() => expect(mockState.announce).toHaveBeenCalledWith(
      '/home/operator under Claude Code: 1 instruction file, 0 skills, 0 memories',
      'info',
    ))
  })
})
