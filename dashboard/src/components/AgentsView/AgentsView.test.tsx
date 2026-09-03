import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AgentsView from './index'
import { DEFAULT_SETTINGS } from '../../types'
import type { AgentContext, AgentWorkspaces } from '../../agents/agentContextApi'

const mockState = vi.hoisted(() => ({
  announce: vi.fn(),
  fetchAgentContext: vi.fn(),
  fetchAgentWorkspaces: vi.fn(),
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
    fetchAgentWorkspaces: (user: string) => mockState.fetchAgentWorkspaces(user),
  }
})

vi.mock('../../beads/beadsApi', () => ({
  fetchBeadWork: (path: string) => mockState.fetchBeadWork(path),
}))

vi.mock('../Desk', () => ({
  default: ({ label, sessionName, reference }: { label: string; sessionName?: string; reference: string }) => (
    <div data-testid="desk">{`${label} ${sessionName ?? 'none'} ${reference}`}</div>
  ),
}))

const workspaces: AgentWorkspaces = {
  workspaces: [
    { path: '/home/operator', source: 'home', instructions: 3 },
    { path: '/srv/chrote', source: 'root', instructions: 3 },
  ],
  tender: { session: 'tender', beads: '/srv', folder: '/srv/ops/tender' },
}

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
    mockState.fetchAgentWorkspaces.mockReset()
    mockState.fetchBeadWork.mockReset()
    mockState.fetchAgentWorkspaces.mockResolvedValue(workspaces)
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

  it('lists the tender proposals and hosts the tender desk', async () => {
    render(<AgentsView />)

    await waitFor(() => expect(screen.getByText('ctx-p3f')).toBeInTheDocument())
    expect(screen.getByTestId('desk')).toHaveTextContent('Tender tender agents /home/operator claude-code')
  })

  it('says what the workspace holds on the status line', async () => {
    render(<AgentsView />)

    await waitFor(() => expect(mockState.announce).toHaveBeenCalledWith(
      '/home/operator under Claude Code: 1 instruction file, 0 skills, 0 memories',
      'info',
    ))
  })
})
