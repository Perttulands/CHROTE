import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import AgentsView, { reachableMissionItems } from './AgentsView'
import { activeRunStorageKey } from './formationsRunState'
import type { BoardDocument, LayoutDocument } from './formationsTypes'

describe('AgentsView', () => {
  const fetchMock = vi.fn()

  beforeEach(() => {
    fetchMock.mockReset()
    window.localStorage.clear()
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it('computes mission reachability from connections across branched gate paths', () => {
    const board = missionBoard()

    expect(reachableMissionItems(board, 'mission-alpha').map(item => {
      const via = item.via ? `${item.via.gateId}/${item.via.branch}` : 'main'
      return `${item.kind}:${item.id}:${via}`
    })).toEqual([
      'formation:authoring:main',
      'gate:human-review:main',
      'formation:fix-pass:human-review/pass',
      'formation:escalate-fail:human-review/fail',
    ])
  })

  it('loads mission staffing and preserves the board patch contract for assign and unassign', async () => {
    const board = missionBoard()
    const layout = missionLayout()
    const patches: Array<{ headers: HeadersInit | undefined; body: unknown }> = []
    fetchMock.mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/agents') {
        return Promise.resolve(jsonResponse({
          success: true,
          data: {
            agents: [
              agent('coder', { displayName: 'Coder', liveness: 'live', harnessDefault: 'openai-codex' }),
              agent('susie', { displayName: 'Susie', tags: ['design'], liveness: 'offline' }),
            ],
            count: 2,
          },
        }))
      }
      if (url === '/api/agents/coder') {
        return Promise.resolve(jsonResponse({ success: true, data: persona('coder', { displayName: 'Coder', harnessDefault: 'openai-codex', harnessVariants: [{ id: 'openai-codex', sessionStem: 'coder' }] }) }, 200, { ETag: 'coder-etag' }))
      }
      if (url === '/api/agents/susie') {
        return Promise.resolve(jsonResponse({ success: true, data: persona('susie', { displayName: 'Susie', harnessVariants: [{ id: 'claude-code', sessionStem: 'susie', source: '/tmp/SUSIE.toml' }] }) }, 200, { ETag: 'susie-etag' }))
      }
      if (url === '/api/formations/boards') {
        return Promise.resolve(jsonResponse({ success: true, data: { boards: [{ id: 'board-1', slug: 'mission-board', title: 'Mission Board', rev: 7, etag: 'board-etag' }] } }))
      }
      if (url === '/api/formations/boards/mission-board/layout') {
        return Promise.resolve(jsonResponse({ success: true, data: { layout } }, 200, { ETag: 'layout-etag' }))
      }
      if (url === '/api/formations/boards/mission-board' && init?.method === 'PATCH') {
        patches.push({ headers: init.headers, body: JSON.parse(String(init.body)) })
        return Promise.resolve(jsonResponse({ success: true, data: { board: { ...board, etag: 'board-etag-2' } } }, 200, { ETag: 'board-etag-2' }))
      }
      if (url === '/api/formations/boards/mission-board') {
        return Promise.resolve(jsonResponse({ success: true, data: { board } }, 200, { ETag: 'board-etag' }))
      }
      return Promise.reject(new Error(`unexpected fetch ${url}`))
    })

    render(<AgentsView />)

    expect(await screen.findByText('Authoring')).toBeInTheDocument()
    expect(screen.getByText('Fix Pass')).toBeInTheDocument()
    expect(screen.getByText('Escalate Fail')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /assign Review slot/i }))
    expect(await screen.findByText('missing harness variant openai-codex')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /assign coder/i }))

    await waitFor(() => expect(patches).toHaveLength(1))
    expect(headerValue(patches[0].headers, 'If-Match')).toBe('board-etag')
    expect(patches[0].body).toMatchObject({
      expectedRev: 7,
      updatedBy: 'agent:ui',
      assignSlot: { formationId: 'authoring', slotId: 'reviewer', agentId: 'coder', harness: 'openai-codex' },
    })

    fireEvent.click(screen.getByRole('button', { name: /inspect Lead slot assigned to Susie/i }))
    fireEvent.click(await screen.findByRole('button', { name: /unassign Susie/i }))

    await waitFor(() => expect(patches).toHaveLength(2))
    expect(patches[1].body).toMatchObject({
      expectedRev: 7,
      updatedBy: 'agent:ui',
      assignSlot: { formationId: 'authoring', slotId: 'lead', agentId: '', harness: '' },
    })
  })

  it('labels restored runs by their mission and lets the user jump to mismatched run missions', async () => {
    const board = {
      ...missionBoard(),
      missions: [
        { id: 'mission-alpha', title: 'Mission Alpha', goal: 'Ship the redesign', beadId: 'chrt-hgc9' },
        { id: 'mission-beta', title: 'Mission Beta', goal: 'Review the fallback', beadId: 'chrt-hgc9' },
      ],
    }
    window.localStorage.setItem(activeRunStorageKey('mission-board'), 'run-beta')
    fetchMock.mockImplementation((input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/agents') {
        return Promise.resolve(jsonResponse({ success: true, data: { agents: [], count: 0 } }))
      }
      if (url === '/api/formations/boards') {
        return Promise.resolve(jsonResponse({ success: true, data: { boards: [{ id: 'board-1', slug: 'mission-board', title: 'Mission Board', rev: 7, etag: 'board-etag' }] } }))
      }
      if (url === '/api/formations/boards/mission-board/layout') {
        return Promise.resolve(jsonResponse({ success: true, data: { layout: missionLayout() } }, 200, { ETag: 'layout-etag' }))
      }
      if (url === '/api/formations/boards/mission-board') {
        return Promise.resolve(jsonResponse({ success: true, data: { board } }, 200, { ETag: 'board-etag' }))
      }
      if (url === '/api/formations/runs/run-beta') {
        return Promise.resolve(jsonResponse({ success: true, data: runStatus('run-beta', 'mission-beta') }))
      }
      if (url === '/api/formations/runs/run-beta/events') {
        return Promise.resolve(jsonResponse({ success: true, data: { events: [] } }))
      }
      return Promise.reject(new Error(`unexpected fetch ${url}`))
    })

    render(<AgentsView />)

    expect(await screen.findByText('Run: Mission Beta')).toBeInTheDocument()
    expect(screen.getByText('This run belongs to Mission Beta, not Mission Alpha.')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /view run mission/i }))

    expect(screen.getByLabelText('Mission')).toHaveValue('mission-beta')
  })

  it('offers a board retry when the selected board fails to load', async () => {
    const board = emptyBoard()
    let failBoard = true
    fetchMock.mockImplementation((input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/agents') {
        return Promise.resolve(jsonResponse({ success: true, data: { agents: [], count: 0 } }))
      }
      if (url === '/api/formations/boards') {
        return Promise.resolve(jsonResponse({ success: true, data: { boards: [{ id: 'empty', slug: 'empty', title: 'Empty', rev: 1, etag: 'empty-etag' }] } }))
      }
      if (url === '/api/formations/boards/empty/layout') {
        if (failBoard) return Promise.reject(new Error('layout unavailable'))
        return Promise.resolve(jsonResponse({ success: true, data: { layout: emptyLayout() } }, 200, { ETag: 'layout-etag' }))
      }
      if (url === '/api/formations/boards/empty') {
        if (failBoard) return Promise.reject(new Error('board unavailable'))
        return Promise.resolve(jsonResponse({ success: true, data: { board } }, 200, { ETag: 'empty-etag' }))
      }
      return Promise.reject(new Error(`unexpected fetch ${url}`))
    })

    render(<AgentsView />)

    expect(await screen.findByText(/Board load failed:/)).toBeInTheDocument()
    failBoard = false
    fireEvent.click(screen.getByRole('button', { name: /retry board/i }))

    expect(await screen.findByText('No personas')).toBeInTheDocument()
    expect(screen.queryByText(/Board load failed:/)).not.toBeInTheDocument()
  })

  it('keeps slot eligibility usable when one persona detail load fails', async () => {
    const board = missionBoard()
    fetchMock.mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/agents') {
        return Promise.resolve(jsonResponse({
          success: true,
          data: {
            agents: [
              agent('good', { displayName: 'Good', liveness: 'live', harnessDefault: 'openai-codex' }),
              agent('broken', { displayName: 'Broken', liveness: 'live', harnessDefault: 'openai-codex' }),
            ],
            count: 2,
          },
        }))
      }
      if (url === '/api/agents/good') {
        return Promise.resolve(jsonResponse({ success: true, data: persona('good', { displayName: 'Good', harnessDefault: 'openai-codex', harnessVariants: [{ id: 'openai-codex', sessionStem: 'good' }] }) }, 200, { ETag: 'good-etag' }))
      }
      if (url === '/api/agents/broken') {
        return Promise.reject(new Error('detail failed'))
      }
      if (url === '/api/formations/boards') {
        return Promise.resolve(jsonResponse({ success: true, data: { boards: [{ id: 'board-1', slug: 'mission-board', title: 'Mission Board', rev: 7, etag: 'board-etag' }] } }))
      }
      if (url === '/api/formations/boards/mission-board/layout') {
        return Promise.resolve(jsonResponse({ success: true, data: { layout: missionLayout() } }, 200, { ETag: 'layout-etag' }))
      }
      if (url === '/api/formations/boards/mission-board' && init?.method === 'PATCH') {
        return Promise.resolve(jsonResponse({ success: true, data: { board } }, 200, { ETag: 'board-etag' }))
      }
      if (url === '/api/formations/boards/mission-board') {
        return Promise.resolve(jsonResponse({ success: true, data: { board } }, 200, { ETag: 'board-etag' }))
      }
      return Promise.reject(new Error(`unexpected fetch ${url}`))
    })

    render(<AgentsView />)

    fireEvent.click(await screen.findByRole('button', { name: /assign Review slot/i }))

    expect(await screen.findByRole('button', { name: /assign Good/i })).toBeEnabled()
    expect(screen.getByText('failed detail load')).toBeInTheDocument()
    expect(screen.queryByText('Agent eligibility request failed')).not.toBeInTheDocument()
  })

  it('starts a fully staffed mission with the board ETag contract', async () => {
    const board = fullyStaffedMissionBoard()
    const posts: Array<{ headers: HeadersInit | undefined; body: unknown }> = []
    fetchMock.mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/agents') {
        return Promise.resolve(jsonResponse({
          success: true,
          data: {
            agents: [
              agent('susie', { displayName: 'Susie', harnessDefault: 'claude-code' }),
              agent('coder', { displayName: 'Coder', harnessDefault: 'openai-codex' }),
            ],
            count: 2,
          },
        }))
      }
      if (url === '/api/formations/boards') {
        return Promise.resolve(jsonResponse({ success: true, data: { boards: [{ id: 'board-1', slug: 'mission-board', title: 'Mission Board', rev: 7, etag: 'board-etag' }] } }))
      }
      if (url === '/api/formations/boards/mission-board/layout') {
        return Promise.resolve(jsonResponse({ success: true, data: { layout: missionLayout() } }, 200, { ETag: 'layout-etag' }))
      }
      if (url === '/api/formations/boards/mission-board') {
        return Promise.resolve(jsonResponse({ success: true, data: { board } }, 200, { ETag: 'board-etag' }))
      }
      if (url === '/api/formations/runs' && init?.method === 'POST') {
        posts.push({ headers: init.headers, body: JSON.parse(String(init.body)) })
        return Promise.resolve(jsonResponse({ success: true, data: { runId: 'run-started', status: runStatus('run-started', 'mission-alpha') } }))
      }
      if (url === '/api/formations/runs/run-started/events') {
        return Promise.resolve(jsonResponse({ success: true, data: { events: [] } }))
      }
      return Promise.reject(new Error(`unexpected fetch ${url}`))
    })

    render(<AgentsView />)

    fireEvent.click(await screen.findByRole('button', { name: /start mission/i }))

    await waitFor(() => expect(posts).toHaveLength(1))
    expect(headerValue(posts[0].headers, 'If-Match')).toBe('board-etag')
    expect(posts[0].body).toMatchObject({ board: 'mission-board', missionId: 'mission-alpha', actor: 'agent:ui' })
    expect(window.localStorage.getItem(activeRunStorageKey('mission-board'))).toBe('run-started')
  })

  it('renders textual liveness and binding labels instead of oracle idle or complete classes', async () => {
    const board = emptyBoard()
    fetchMock.mockImplementation((input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/agents') {
        return Promise.resolve(jsonResponse({
          success: true,
          data: {
            agents: [
              agent('attached-live', { displayName: 'Attached Live', liveness: 'live', attached: true }),
              agent('ambiguous-one', { displayName: 'Ambiguous One', liveness: 'ambiguous' }),
              agent('offline-one', { displayName: 'Offline One', liveness: 'offline' }),
              agent('retired-one', { displayName: 'Retired One', liveness: 'offline', assignable: false }),
              agent('floating-session', { displayName: 'floating-session', liveness: 'live', unbound: true, assignable: false }),
            ],
            count: 5,
          },
        }))
      }
      if (url === '/api/agents/retired-one') {
        return Promise.resolve(jsonResponse({ success: true, data: persona('retired-one', { displayName: 'Retired One', status: 'retired' }) }, 200, { ETag: 'retired-etag' }))
      }
      if (url === '/api/formations/boards') {
        return Promise.resolve(jsonResponse({ success: true, data: { boards: [{ id: 'empty', slug: 'empty', title: 'Empty', rev: 1, etag: 'empty-etag' }] } }))
      }
      if (url === '/api/formations/boards/empty/layout') {
        return Promise.resolve(jsonResponse({ success: true, data: { layout: emptyLayout() } }, 200, { ETag: 'layout-etag' }))
      }
      if (url === '/api/formations/boards/empty') {
        return Promise.resolve(jsonResponse({ success: true, data: { board } }, 200, { ETag: 'empty-etag' }))
      }
      return Promise.reject(new Error(`unexpected fetch ${url}`))
    })

    const { container } = render(<AgentsView />)

    expect(await screen.findByText('Attached Live')).toBeInTheDocument()
    expect(screen.getAllByText('live').length).toBeGreaterThan(0)
    expect(screen.getByText('ambiguous')).toBeInTheDocument()
    expect(screen.getAllByText('offline').length).toBeGreaterThan(0)
    expect(screen.getByText('attached')).toBeInTheDocument()
    expect(screen.getByText('not assignable')).toBeInTheDocument()
    expect(screen.getByText('no persona')).toBeInTheDocument()
    expect(container.querySelector('.oracle-status-complete')).toBeNull()
    expect(container.querySelector('.oracle-status-idle')).toBeNull()
    expect(container.querySelector('.oracle-badge-idle')).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: /inspect Retired One/i }))
    await waitFor(() => expect(screen.getAllByText('retired').length).toBeGreaterThan(0))

    fireEvent.click(screen.getByRole('button', { name: /inspect floating-session/i }))
    fireEvent.click(screen.getByRole('button', { name: /create persona from this session/i }))
    expect(screen.getByLabelText('Agent id')).toHaveValue('floating-session')
    expect(screen.getByLabelText('Session stem')).toHaveValue('floating-session')
  })

  it('creates a persona with canonical harness metadata instead of a legacy codex alias', async () => {
    const postedBodies: unknown[] = []
    const board = emptyBoard()
    fetchMock.mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/agents' && init?.method === 'POST') {
        postedBodies.push(JSON.parse(String(init.body)))
        return Promise.resolve(jsonResponse({ success: true, data: persona('writer', { displayName: 'Writer', harnessDefault: 'openai-codex', harnessVariants: [{ id: 'openai-codex', sessionStem: 'writer', launch: 'codex --profile writer' }] }) }, 201, { ETag: 'writer-etag' }))
      }
      if (url === '/api/agents') {
        return Promise.resolve(jsonResponse({ success: true, data: { agents: [], count: 0 } }))
      }
      if (url === '/api/formations/boards') {
        return Promise.resolve(jsonResponse({ success: true, data: { boards: [{ id: 'empty', slug: 'empty', title: 'Empty', rev: 1, etag: 'empty-etag' }] } }))
      }
      if (url === '/api/formations/boards/empty/layout') {
        return Promise.resolve(jsonResponse({ success: true, data: { layout: emptyLayout() } }, 200, { ETag: 'layout-etag' }))
      }
      if (url === '/api/formations/boards/empty') {
        return Promise.resolve(jsonResponse({ success: true, data: { board } }, 200, { ETag: 'empty-etag' }))
      }
      return Promise.reject(new Error(`unexpected fetch ${url}`))
    })

    render(<AgentsView />)

    fireEvent.click(await screen.findByRole('button', { name: /add agent/i }))
    expect(screen.queryByRole('option', { name: 'codex' })).toBeNull()
    fireEvent.change(screen.getByLabelText('Agent id'), { target: { value: 'writer' } })
    fireEvent.change(screen.getByLabelText('Display name'), { target: { value: 'Writer' } })
    fireEvent.change(screen.getByLabelText('Harness'), { target: { value: 'openai-codex' } })
    fireEvent.change(screen.getByLabelText('Summary'), { target: { value: 'Writes launch copy' } })
    fireEvent.change(screen.getByLabelText('Launch'), { target: { value: 'codex --profile writer' } })
    fireEvent.change(screen.getByLabelText('Capabilities'), { target: { value: 'writing, voice' } })
    fireEvent.click(screen.getByRole('button', { name: /^Create persona$/i }))

    await waitFor(() => expect(postedBodies).toHaveLength(1))
    expect(postedBodies[0]).toMatchObject({
      id: 'writer',
      displayName: 'Writer',
      kind: 'specialist',
      harness: 'openai-codex',
      summary: 'Writes launch copy',
      launch: 'codex --profile writer',
      capabilities: ['writing', 'voice'],
    })
  })

  it('uses persona detail ETags for edits and keeps user input visible on 409 and 428 conflicts', async () => {
    const board = emptyBoard()
    const patchStatuses = [409, 428]
    const patchHeaders: Array<HeadersInit | undefined> = []
    fetchMock.mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/agents') {
        return Promise.resolve(jsonResponse({ success: true, data: { agents: [agent('susie', { displayName: 'Susie', tags: ['design'] })], count: 1 } }))
      }
      if (url === '/api/agents/susie' && init?.method === 'PATCH') {
        patchHeaders.push(init.headers)
        const status = patchStatuses.shift() || 409
        const message = status === 428 ? 'If-Match precondition is required' : 'Agent card changed; reload and retry'
        const code = status === 428 ? 'PRECONDITION_REQUIRED' : 'CONFLICT'
        return Promise.resolve(jsonResponse({ success: false, error: { code, message } }, status))
      }
      if (url === '/api/agents/susie') {
        return Promise.resolve(jsonResponse({
          success: true,
          data: persona('susie', {
            displayName: 'Susie',
            tags: ['design'],
            harnessVariants: [{ id: 'claude-code', sessionStem: 'susie', source: '/tmp/SUSIE.toml' }],
            toml: 'CLAUDE.md contents',
          }),
        }, 200, { ETag: 'susie-etag' }))
      }
      if (url === '/api/formations/boards') {
        return Promise.resolve(jsonResponse({ success: true, data: { boards: [{ id: 'empty', slug: 'empty', title: 'Empty', rev: 1, etag: 'empty-etag' }] } }))
      }
      if (url === '/api/formations/boards/empty/layout') {
        return Promise.resolve(jsonResponse({ success: true, data: { layout: emptyLayout() } }, 200, { ETag: 'layout-etag' }))
      }
      if (url === '/api/formations/boards/empty') {
        return Promise.resolve(jsonResponse({ success: true, data: { board } }, 200, { ETag: 'empty-etag' }))
      }
      return Promise.reject(new Error(`unexpected fetch ${url}`))
    })

    render(<AgentsView />)

    fireEvent.click(await screen.findByRole('button', { name: /inspect Susie/i }))
    expect(await screen.findByText('/tmp/SUSIE.toml')).toBeInTheDocument()
    expect(screen.queryByText('CLAUDE.md contents')).not.toBeInTheDocument()

    const note = screen.getByLabelText('Add note')
    fireEvent.change(note, { target: { value: 'Keep this edit' } })
    fireEvent.click(screen.getByRole('button', { name: /save note/i }))

    expect(await screen.findByText('Agent card changed; reload and retry')).toBeInTheDocument()
    expect(headerValue(patchHeaders[0], 'If-Match')).toBe('susie-etag')
    expect(screen.getByDisplayValue('Keep this edit')).toBeInTheDocument()

    fireEvent.change(note, { target: { value: 'Still visible' } })
    fireEvent.click(screen.getByRole('button', { name: /save note/i }))

    expect(await screen.findByText('Programming error: If-Match precondition is required')).toBeInTheDocument()
    expect(screen.getByDisplayValue('Still visible')).toBeInTheDocument()
  })
})

function missionBoard(): BoardDocument {
  return {
    id: 'board-1',
    slug: 'mission-board',
    title: 'Mission Board',
    rev: 7,
    etag: 'board-etag',
    missions: [{ id: 'mission-alpha', title: 'Mission Alpha', goal: 'Ship the redesign', beadId: 'chrt-hgc9' }],
    formations: [
      {
        id: 'authoring',
        type: 'peer',
        title: 'Authoring',
        inputs: [{ id: 'in', label: 'Input' }],
        outputs: [{ id: 'out', label: 'Output' }],
        slots: [
          { id: 'lead', label: 'Lead', controller: true, agentId: 'susie', harness: 'claude-code' },
          { id: 'reviewer', label: 'Review', controller: false, harness: 'openai-codex' },
        ],
      },
      {
        id: 'fix-pass',
        type: 'solo',
        title: 'Fix Pass',
        inputs: [{ id: 'in', label: 'Input' }],
        outputs: [{ id: 'out', label: 'Output' }],
        slots: [{ id: 'builder', label: 'Builder', controller: true, harness: 'openai-codex' }],
      },
      {
        id: 'escalate-fail',
        type: 'solo',
        title: 'Escalate Fail',
        inputs: [{ id: 'in', label: 'Input' }],
        outputs: [{ id: 'out', label: 'Output' }],
        slots: [{ id: 'critic', label: 'Critic', controller: true, agentId: 'coder', harness: 'openai-codex' }],
      },
    ],
    gates: [{ id: 'human-review', title: 'Human Review', kinds: ['human'], criterion: 'Approve the branch.' }],
    connections: [
      { id: 'c1', from: 'mission-alpha:out', to: 'authoring:in' },
      { id: 'c2', from: 'authoring:out', to: 'human-review:in' },
      { id: 'c3', from: 'human-review:pass', to: 'fix-pass:in' },
      { id: 'c4', from: 'human-review:fail', to: 'escalate-fail:in' },
    ],
  }
}

function fullyStaffedMissionBoard(): BoardDocument {
  const board = missionBoard()
  return {
    ...board,
    formations: board.formations.map(formation => ({
      ...formation,
      slots: formation.slots.map(slot => ({ ...slot, agentId: slot.agentId || 'coder' })),
    })),
  }
}

function missionLayout(): LayoutDocument {
  return {
    boardId: 'board-1',
    boardRev: 7,
    etag: 'layout-etag',
    nodes: [
      { id: 'authoring', x: 100, y: 100 },
      { id: 'human-review', x: 250, y: 100 },
      { id: 'fix-pass', x: 400, y: 60 },
      { id: 'escalate-fail', x: 400, y: 180 },
    ],
  }
}

function emptyBoard(): BoardDocument {
  return {
    id: 'empty',
    slug: 'empty',
    title: 'Empty',
    rev: 1,
    etag: 'empty-etag',
    missions: [],
    formations: [],
    gates: [],
    connections: [],
  }
}

function emptyLayout(): LayoutDocument {
  return { boardId: 'empty', boardRev: 1, etag: 'layout-etag', nodes: [] }
}

function runStatus(runId: string, missionId: string) {
  return {
    runId,
    status: 'running',
    final: false,
    boardSlug: 'mission-board',
    missionId,
    eventCount: 0,
  }
}

function agent(id: string, overrides: Record<string, unknown> = {}) {
  return {
    id,
    displayName: id,
    kind: 'specialist',
    tags: [],
    liveness: 'offline',
    assignable: true,
    ...overrides,
  }
}

function persona(id: string, overrides: Record<string, unknown> = {}) {
  return {
    id,
    displayName: id,
    kind: 'specialist',
    summary: '',
    tags: [],
    status: '',
    harnessDefault: 'claude-code',
    harnessVariants: [{ id: 'claude-code', sessionStem: id, source: '/tmp/AGENT.toml' }],
    notes: [],
    etag: `${id}-etag`,
    ...overrides,
  }
}

function jsonResponse(body: unknown, status = 200, headers: Record<string, string> = {}) {
  return {
    ok: status >= 200 && status < 300,
    status,
    headers: {
      get: (name: string) => headers[name] ?? headers[name.toLowerCase()] ?? '',
    },
    json: () => Promise.resolve(body),
  } as Response
}

function headerValue(headers: HeadersInit | undefined, key: string): string {
  if (!headers) return ''
  if (headers instanceof Headers) return headers.get(key) || ''
  if (Array.isArray(headers)) {
    const pair = headers.find(([name]) => name.toLowerCase() === key.toLowerCase())
    return pair?.[1] || ''
  }
  return headers[key] || headers[key.toLowerCase()] || ''
}
