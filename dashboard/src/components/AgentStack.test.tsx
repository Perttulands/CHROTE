import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AgentStack from './AgentStack'
import type { AgentContext } from '../agents/agentContextApi'

const mockState = vi.hoisted(() => ({
  announce: vi.fn(),
  fetchAgentFile: vi.fn(),
  writeTextFile: vi.fn(),
}))

vi.mock('../context/StatusContext', () => ({
  useStatus: () => ({ announce: mockState.announce }),
}))

vi.mock('./FilesView/fileService', () => ({
  writeTextFile: (path: string, content: string) => mockState.writeTextFile(path, content),
  getDownloadUrl: (path: string) => `/api/files/raw${path}`,
}))

vi.mock('../agents/agentContextApi', async () => {
  const actual = await vi.importActual<typeof import('../agents/agentContextApi')>('../agents/agentContextApi')
  return {
    ...actual,
    fetchAgentFile: (path: string) => mockState.fetchAgentFile(path),
  }
})

function context(overrides: Partial<AgentContext> = {}): AgentContext {
  return {
    folder: '/srv/chrote',
    harness: 'claude-code',
    user: 'operator',
    instructions: [
      { path: '/home/operator/CLAUDE.md', scope: 'user', kind: 'CLAUDE.md', readable: true, size: 2169, link: 'AGENTS.md' },
      { path: '/home/operator/.claude/settings.json', scope: 'user', kind: 'settings', readable: false, size: 0 },
      { path: '/srv/chrote/CLAUDE.md', scope: 'project', kind: 'CLAUDE.md', readable: true, size: 3709 },
    ],
    skills: [
      { name: 'dashboard-development', description: 'Change CHROTE dashboard views.', path: '/srv/chrote/skills/dashboard-development', source: 'project' },
      { name: 'beads', description: 'Use Beads as the durable work system.', path: '/home/operator/skills/beads', source: 'shared' },
    ],
    memories: [
      { kind: 'claude-auto', path: '/home/operator/.claude/projects/-srv-chrote/memory/MEMORY.md', title: 'MEMORY.md', updated: '2026-09-03T15:31:00Z', readable: true },
      { kind: 'bd', path: '', title: 'beads-acl-no-chmod-700', updated: '', readable: true },
    ],
    ...overrides,
  }
}

describe('AgentStack', () => {
  beforeEach(() => {
    mockState.announce.mockReset()
    mockState.fetchAgentFile.mockReset()
    mockState.writeTextFile.mockReset()
    mockState.fetchAgentFile.mockResolvedValue('# CHROTE\n\nThe project.\n')
    mockState.writeTextFile.mockResolvedValue(undefined)
  })

  it('names every row of the three layers and where it came from', () => {
    render(<AgentStack context={context()} />)

    expect(screen.getByText('/srv/chrote/CLAUDE.md')).toBeInTheDocument()
    expect(screen.getAllByText('user')).toHaveLength(2)
    expect(screen.getByText('project')).toBeInTheDocument()
    expect(screen.getByText('3 709 B')).toBeInTheDocument()
    expect(screen.getByText('dashboard-development')).toBeInTheDocument()
    expect(screen.getByText('project /srv/chrote/skills')).toBeInTheDocument()
    expect(screen.getByText('MEMORY.md')).toBeInTheDocument()
    expect(screen.getByText('claude index')).toBeInTheDocument()
    expect(screen.getByText('beads-acl-no-chmod-700')).toBeInTheDocument()
  })

  it('says a file is not readable rather than leaving it out', () => {
    render(<AgentStack context={context()} />)

    expect(screen.getByText('/home/operator/.claude/settings.json')).toBeInTheDocument()
    expect(screen.getByText('not readable by the server')).toBeInTheDocument()
  })

  it('says when two rows of the stack are the same file', () => {
    render(<AgentStack context={context()} />)

    expect(screen.getByText('links to AGENTS.md')).toBeInTheDocument()
  })

  it('reads a file only when its row is opened', async () => {
    render(<AgentStack context={context()} />)
    expect(mockState.fetchAgentFile).not.toHaveBeenCalled()

    fireEvent.click(screen.getByText('/srv/chrote/CLAUDE.md'))

    await waitFor(() => expect(screen.getByText('The project.')).toBeInTheDocument())
    expect(mockState.fetchAgentFile).toHaveBeenCalledWith('/srv/chrote/CLAUDE.md')
  })

  it('edits an opened file through the shared editor and saves it', async () => {
    render(<AgentStack context={context()} />)
    fireEvent.click(screen.getByText('/srv/chrote/CLAUDE.md'))
    await waitFor(() => expect(screen.getByText('The project.')).toBeInTheDocument())

    fireEvent.click(screen.getByText('Edit'))
    const field = await screen.findByLabelText('Edit /srv/chrote/CLAUDE.md')
    fireEvent.change(field, { target: { value: '# CHROTE\n\nCorrected.\n' } })
    fireEvent.click(screen.getByText('Save'))

    await waitFor(() => expect(mockState.writeTextFile)
      .toHaveBeenCalledWith('/srv/chrote/CLAUDE.md', '# CHROTE\n\nCorrected.\n'))
    expect(mockState.announce).toHaveBeenCalledWith('Saved /srv/chrote/CLAUDE.md', 'success')
  })

  it('filters skills and memories by the query, and leaves the stack whole', () => {
    render(<AgentStack context={context()} query="beads" />)

    expect(screen.getByText('beads')).toBeInTheDocument()
    expect(screen.queryByText('dashboard-development')).not.toBeInTheDocument()
    expect(screen.getByText('/srv/chrote/CLAUDE.md')).toBeInTheDocument()
  })

  it('offers the rest of a long list rather than showing all of it at once', () => {
    const many = Array.from({ length: 20 }, (_, index) => ({
      name: `skill-${index}`,
      description: `Skill ${index}.`,
      path: `/home/operator/skills/skill-${index}`,
      source: 'shared',
    }))
    render(<AgentStack context={context({ skills: many })} />)

    expect(screen.queryByText('skill-19')).not.toBeInTheDocument()
    fireEvent.click(screen.getByText('14 more'))
    expect(screen.getByText('skill-19')).toBeInTheDocument()
  })
})
