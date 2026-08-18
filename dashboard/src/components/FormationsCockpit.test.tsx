import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import FormationsCockpit from './FormationsCockpit'

/* First direct cockpit coverage: pins the reference-parity behaviors that the
   prototype (03-formations.js) defines — judge wires render, the right-click
   surface exists everywhere, menus dismiss, and gestures issue real board ops. */

const judgeFormation = {
  id: 'fmn_judge',
  type: 'solo',
  title: 'Judge',
  inputs: [{ id: 'port_judge_in', label: 'Input' }],
  outputs: [{ id: 'port_judge_out', label: 'Output' }],
  slots: [{ id: 'slot_judge', label: 'Judge' }],
  verification: undefined,
}

const formation = {
  id: 'fmn_frame',
  type: 'orchestrated',
  title: 'Frame',
  inputs: [{ id: 'port_frame_in', label: 'Input' }],
  outputs: [{ id: 'port_frame_out', label: 'Output' }],
  slots: [
    { id: 'slot_lead', label: 'Lead', controller: true, agentId: 'mason', harness: 'codex' },
    { id: 'slot_worker', label: 'Worker', controller: false },
  ],
  verification: { id: 'ver_frame', kinds: ['code'], criterion: 'Tests pass', onFail: 'block' },
}

const gate = { id: 'gate_review', title: 'Review', kinds: ['code'], criterion: 'Review the frame' }
const mission = { id: 'mis_showcase', title: 'Showcase', goal: 'Build the page', beadId: 'home-7kc4.5' }
const tool = {
  id: 'tool_normalize',
  title: 'Normalize report',
  profileId: 'json.normalize',
  profileVersion: '1',
  params: { mode: 'strict' },
  inputs: [{
    id: 'port_tool_input',
    name: 'input',
    label: 'Report',
    direction: 'input' as const,
    kind: 'work' as const,
    acceptedMediaTypes: ['application/json'],
    required: true,
    role: 'data' as const,
  }],
  outputs: [{
    id: 'port_tool_output',
    name: 'output',
    label: 'Normalized report',
    direction: 'output' as const,
    kind: 'work' as const,
    acceptedMediaTypes: ['application/json'],
  }],
}
const upstreamTool = {
  ...tool,
  id: 'tool_source',
  title: 'Source JSON',
  inputs: tool.inputs.map(port => ({ ...port, id: 'port_source_input' })),
  outputs: tool.outputs.map(port => ({ ...port, id: 'port_source_output' })),
}

function makeBoard() {
  return {
    schema: 1,
    id: 'brd_test',
    slug: 'test-board',
    title: 'Test board',
    rev: 7,
    etag: 'board-etag',
    missions: [mission],
    formations: [formation, judgeFormation],
    gates: [gate],
    tools: [] as typeof tool[],
    connections: [
      { id: 'edge_mission_frame', from: 'mis_showcase:out', to: 'fmn_frame:port_frame_in' },
      { id: 'edge_frame_gate', from: 'fmn_frame:port_frame_out', to: 'gate_review:in' },
      { id: 'edge_judge_send', from: 'gate_review:judge', to: 'fmn_judge:port_judge_in' },
      { id: 'edge_judge_return', from: 'fmn_judge:port_judge_out', to: 'gate_review:judge' },
    ],
  }
}

function makeToolBoard() {
  const base = makeBoard()
  return {
    ...base,
    schema: 2,
    tools: [upstreamTool, tool],
    connections: [
      ...base.connections.filter(connection => connection.id !== 'edge_frame_gate'),
      { id: 'edge_tool_chain', from: 'tool_source:port_source_output', to: 'tool_normalize:port_tool_input' },
      { id: 'edge_tool_gate', from: 'tool_normalize:port_tool_output', to: 'gate_review:in' },
    ],
  }
}

const layout = {
  schema: 1,
  boardId: 'brd_test',
  boardRev: 7,
  etag: 'layout-etag',
  nodes: [
    { id: 'mis_showcase', x: 100, y: 100 },
    { id: 'fmn_frame', x: 420, y: 100 },
    { id: 'fmn_judge', x: 700, y: 420 },
    { id: 'gate_review', x: 860, y: 100 },
  ],
  edges: [],
}

const agents = [
  { id: 'mason', displayName: 'Mason', harnessDefault: 'codex', liveness: 'live', assignable: true, unbound: false },
  { id: 'hazel', displayName: 'Hazel', harnessDefault: 'claude', liveness: 'live', assignable: true, unbound: false },
  { id: 'scratch', displayName: 'scratch', liveness: 'live', assignable: false, unbound: true },
]

type RecordedPatch = { url: string; body: Record<string, unknown> }
type RecordedMutation = { method: string; url: string }
type TestBoard = ReturnType<typeof makeBoard>
type TestRunEvent = { runId: string; seq: number; type: string; nodeId?: string; gateId?: string; attempt?: number; data?: Record<string, unknown> }
type TestEscalation = { runId: string; seq: number; nodeId?: string; gateId?: string; severity: string; reason: string; source: string; trigger: string; blocks: boolean }
type TestRunStatus = { status?: string; final?: boolean; resumeAllowed?: boolean }
let recordedMutations: RecordedMutation[] = []

function installFetchMock(options: {
  emptyBoards?: boolean
  freshCreateLayout?: boolean
  missionCreateFailure?: boolean
  removalFailure?: boolean
  removalGate?: Promise<void>
  boards?: TestBoard[]
  sameBoardRefreshes?: TestBoard[]
  runEvents?: TestRunEvent[]
  escalations?: TestEscalation[]
  runStatus?: TestRunStatus
  boardNotes?: { board?: string; elements?: Array<{ nodeId: string; text: string }> }
} = {}) {
  const patches: RecordedPatch[] = []
  recordedMutations = []
  let availableBoards = options.emptyBoards ? [] : (options.boards?.length ? options.boards : [makeBoard()])
  let board = availableBoards[0] || makeBoard()
  let currentLayout = layout
  let boardNotes = {
    schema: 1,
    boardId: board.id,
    rev: options.boardNotes ? 1 : 0,
    updatedAt: '2026-08-18T13:00:00Z',
    updatedBy: 'human:test',
    board: options.boardNotes?.board || '',
    elements: options.boardNotes?.elements || [],
    etag: options.boardNotes ? 'notes-etag' : '*',
  }
  ;(globalThis as Record<string, unknown>).fetch = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input)
    const method = (init?.method || 'GET').toUpperCase()
    if (method !== 'GET') recordedMutations.push({ method, url })
    const respond = (data: unknown, etag = '') => Promise.resolve({
      ok: true,
      headers: { get: (name: string) => (name.toLowerCase() === 'etag' ? etag || null : null) },
      json: () => Promise.resolve({ success: true, data }),
      text: () => Promise.resolve(''),
    })
    const reject = (message: string) => Promise.resolve({
      ok: false,
      status: 400,
      headers: { get: () => null },
      json: () => Promise.resolve({ success: false, error: { code: 'BAD_REQUEST', message } }),
      text: () => Promise.resolve(message),
    })
    if (method === 'POST' && url === '/api/formations/boards') {
      const body = JSON.parse(String(init?.body)) as { title: string }
      const created = {
        ...makeBoard(),
        id: 'brd_created',
        slug: 'release-plan',
        title: body.title,
        rev: 1,
        etag: 'created-board-etag',
        missions: [],
        formations: [],
        gates: [],
        connections: [],
      }
      availableBoards = [...availableBoards, created]
      board = created
      currentLayout = { schema: 1, boardId: created.id, boardRev: created.rev, etag: '*', nodes: [], edges: [] }
      boardNotes = { ...boardNotes, boardId: created.id, rev: 0, board: '', elements: [], etag: '*' }
      return respond({ board: created }, created.etag)
    }
    if (method === 'DELETE' && url.includes('/api/formations/boards/')) {
      availableBoards = availableBoards.filter(item => item.slug !== board.slug)
      return respond({ deletion: { id: board.id, slug: board.slug, title: board.title, archiveId: 'archive_test' } })
    }
    if (method === 'GET' && /^\/api\/formations\/boards\/[^/]+\/notes$/.test(url)) {
      return respond({ notes: boardNotes }, boardNotes.etag)
    }
    if (method === 'PATCH' && /^\/api\/formations\/boards\/[^/]+\/notes$/.test(url)) {
      const body = JSON.parse(String(init?.body)) as { target: string; text: string }
      boardNotes = {
        ...boardNotes,
        rev: boardNotes.rev + 1,
        updatedBy: 'human:ui',
        etag: `notes-etag-${boardNotes.rev + 1}`,
        board: body.target === 'board' ? body.text : boardNotes.board,
        elements: body.target === 'board'
          ? boardNotes.elements
          : body.text
            ? [...boardNotes.elements.filter(note => note.nodeId !== body.target), { nodeId: body.target, text: body.text }]
            : boardNotes.elements.filter(note => note.nodeId !== body.target),
      }
      return respond({ notes: boardNotes }, boardNotes.etag)
    }
    if (init?.method === 'PATCH') {
      const body = JSON.parse(String(init.body)) as Record<string, unknown>
      patches.push({ url, body })
      if (!url.endsWith('/layout') && typeof body.title === 'string') {
        board = { ...board, title: body.title, rev: board.rev + 1, etag: 'board-etag-2' }
        availableBoards = availableBoards.map(item => item.slug === board.slug ? board : item)
        return respond({ board }, board.etag)
      }
      if (!url.endsWith('/layout') && body.createMission) {
        if (options.missionCreateFailure) return reject('Mission create failed')
        const requested = body.createMission as { title: string; goal: string; beadId: string; x: number; y: number }
        const created = {
          id: 'mis_created',
          title: requested.title,
          goal: requested.goal,
          beadId: requested.beadId,
        }
        board = { ...board, rev: board.rev + 1, missions: [...board.missions, created] }
        currentLayout = {
          ...currentLayout,
          boardRev: board.rev,
          etag: 'layout-mission-etag',
          nodes: [...currentLayout.nodes, { id: created.id, x: requested.x, y: requested.y }],
        }
        return respond({ board, layout: currentLayout }, 'board-etag-2')
      }
      if (!url.endsWith('/layout') && body.removeVerification && options.removalFailure) {
        return reject('Legacy verification migration failed')
      }
      if (!url.endsWith('/layout') && body.removeVerification && options.removalGate) {
        return options.removalGate.then(() => {
          board = { ...board, rev: board.rev + 1 }
          return respond({ board }, 'board-etag-2')
        })
      }
      if (options.freshCreateLayout && !url.endsWith('/layout') && body.createGate) {
        const requested = body.createGate as { title: string; kinds: string[]; criterion: string }
        const created = { id: 'gate_created', title: requested.title, kinds: requested.kinds, criterion: requested.criterion }
        board = { ...board, rev: board.rev + 1, gates: [...board.gates, created] }
        currentLayout = {
          ...currentLayout,
          boardRev: board.rev,
          etag: 'layout-created-etag',
          nodes: [...currentLayout.nodes, { id: created.id, x: 1344, y: 784 }],
        }
        return respond({ board, layout: currentLayout }, 'board-etag-2')
      }
      board = { ...board, rev: board.rev + 1 }
      if (url.endsWith('/layout')) return respond({ layout: currentLayout }, 'layout-etag-2')
      return respond({ board }, 'board-etag-2')
    }
    if (/\/api\/formations\/runs\/[^/]+\/escalations$/.test(url)) return respond({ escalations: options.escalations || [] })
    if (/\/api\/formations\/runs\/[^/]+\/events$/.test(url)) return respond({ events: options.runEvents || [] })
    if (/\/api\/formations\/runs\/[^/]+$/.test(url)) {
      return respond({ status: {
        runId: 'run_legacy',
        status: options.runStatus?.status ?? 'succeeded',
        final: options.runStatus?.final ?? true,
        resumeAllowed: options.runStatus?.resumeAllowed ?? false,
        boardSlug: board.slug,
        missionId: mission.id,
        eventCount: options.runEvents?.length || 0,
      } })
    }
    if (url === '/api/formations/gate-profiles') {
      return respond({ profiles: [
        {
          profileId: 'output_absent',
          profileVersion: '1',
          displayName: 'Output excludes value',
          parameterName: 'value',
          parameterLabel: 'Forbidden text',
        },
        {
          profileId: 'output_contains',
          profileVersion: '1',
          displayName: 'Output contains value',
          parameterName: 'value',
          parameterLabel: 'Required text',
        },
      ] })
    }
    if (url === '/api/formations/boards') return respond({ boards: availableBoards.map(item => ({ id: item.id, slug: item.slug, title: item.title, rev: item.rev, etag: item.etag })) })
    if (url.includes('/changes')) {
      const refreshedBoard = options.sameBoardRefreshes?.shift()
      if (!refreshedBoard) return respond({ signal: { changed: false } })
      board = refreshedBoard
      currentLayout = { ...currentLayout, boardRev: refreshedBoard.rev }
      return respond({
        signal: {
          board: refreshedBoard.slug,
          changed: true,
          rev: refreshedBoard.rev,
          etag: refreshedBoard.etag,
        },
      })
    }
    if (url.endsWith('/layout')) return respond({ layout: currentLayout }, 'layout-etag')
    if (url.includes('/api/formations/boards/')) {
      const requested = url.includes(`/boards/${board.slug}`)
        ? board
        : availableBoards.find(item => url.includes(`/boards/${item.slug}`)) || board
      return respond({ board: requested }, requested.etag)
    }
    if (url === '/api/agents') return respond({ agents })
    return respond({})
  }) as unknown as typeof fetch
  return patches
}

async function renderCockpit() {
  const utils = render(<FormationsCockpit />)
  await screen.findByTestId('formation-node-fmn_frame')
  return utils
}

describe('FormationsCockpit reference parity', () => {
  let patches: RecordedPatch[]

  beforeEach(() => {
    localStorage.clear()
    patches = installFetchMock()
  })

  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  it('falls back to default text size outside a SessionProvider', async () => {
    await renderCockpit()
    expect(screen.getByTestId('formations-view')).toHaveAttribute('data-textsize', 'default')
  })

  it('does not fabricate a starter board when no real boards exist', async () => {
    patches = installFetchMock({ emptyBoards: true })
    render(<FormationsCockpit />)
    expect(await screen.findByTestId('formations-empty-board')).toHaveTextContent('No persisted formation boards')
    expect(screen.getByTestId('board-picker')).toHaveTextContent('No boards')
    expect(screen.queryByText('Improve session search')).toBeNull()
    expect(screen.getByTestId('new-board')).toBeEnabled()
    expect(screen.getByTestId('new-formation')).toBeDisabled()
    expect(patches).toEqual([])
  })

  it('creates and selects a named blank board from the Formations top bar', async () => {
    patches = installFetchMock({ emptyBoards: true })
    render(<FormationsCockpit />)
    await screen.findByTestId('formations-empty-board')

    fireEvent.click(screen.getByTestId('new-board'))
    const dialog = await screen.findByRole('dialog', { name: 'Create board' })
    fireEvent.change(within(dialog).getByLabelText('Board name'), { target: { value: 'Release Plan' } })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Create board' }))

    await waitFor(() => expect(screen.getByTestId('board-picker')).toHaveValue('release-plan'))
    expect(screen.getByTestId('board-picker')).toHaveTextContent('Release Plan')
    expect(screen.getByTestId('formations-empty-board')).toHaveTextContent('This board is empty')
    expect(recordedMutations).toContainEqual({ method: 'POST', url: '/api/formations/boards' })
  })

  it('renames the selected board through the top-bar board controls', async () => {
    await renderCockpit()
    fireEvent.click(screen.getByRole('button', { name: 'Rename board' }))
    const dialog = await screen.findByRole('dialog', { name: 'Rename board' })
    const input = within(dialog).getByLabelText('Board name')
    expect(input).toHaveValue('Test board')
    fireEvent.change(input, { target: { value: 'Delivery map' } })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save board name' }))

    await waitFor(() => expect(screen.getByTestId('board-picker')).toHaveTextContent('Delivery map'))
    expect(patches.some(patch => patch.body.title === 'Delivery map')).toBe(true)
  })

  it('archives a board only after explicit confirmation', async () => {
    await renderCockpit()
    fireEvent.click(screen.getByRole('button', { name: 'Delete board' }))
    const dialog = await screen.findByRole('dialog', { name: 'Delete board' })
    expect(dialog).toHaveTextContent('archived')
    expect(recordedMutations.some(mutation => mutation.method === 'DELETE')).toBe(false)

    fireEvent.click(within(dialog).getByRole('button', { name: 'Archive board' }))
    await waitFor(() => expect(screen.getByTestId('board-picker')).toHaveTextContent('No boards'))
    expect(recordedMutations).toContainEqual({ method: 'DELETE', url: '/api/formations/boards/test-board' })
  })

  it('shares board and element notes through a collapsible right-side notepad', async () => {
    patches = installFetchMock({
      boardNotes: {
        board: 'Preserve the API contract.',
        elements: [{ nodeId: 'fmn_frame', text: 'Builder owns this element.' }],
      },
    })
    await renderCockpit()

    const notepad = await screen.findByRole('complementary', { name: 'Shared board notepad' })
    expect(within(notepad).getByLabelText('Board note')).toHaveValue('Preserve the API contract.')
    expect(screen.getByTestId('formation-node-fmn_frame')).toHaveClass('has-note')

    fireEvent.click(within(screen.getByTestId('formation-node-fmn_frame')).getByRole('button', { name: 'Edit note for Frame' }))
    expect(within(notepad).getByLabelText('Element')).toHaveValue('fmn_frame')
    const elementNote = within(notepad).getByLabelText('Element note')
    expect(elementNote).toHaveValue('Builder owns this element.')
    fireEvent.change(elementNote, { target: { value: 'Builder and reviewer own this.' } })
    fireEvent.click(within(notepad).getByRole('button', { name: 'Save element note' }))

    await waitFor(() => expect(recordedMutations).toContainEqual({ method: 'PATCH', url: '/api/formations/boards/test-board/notes' }))
    expect(screen.getByTestId('formation-node-fmn_frame')).toHaveClass('has-note')

    fireEvent.click(within(notepad).getByRole('button', { name: 'Collapse shared notepad' }))
    expect(screen.queryByLabelText('Board note')).toBeNull()
    expect(screen.getByRole('button', { name: 'Expand shared notepad' })).toBeInTheDocument()
  })

  it('adds a new element note from the sticky-note affordance', async () => {
    await renderCockpit()
    const judge = screen.getByTestId('formation-node-fmn_judge')
    expect(judge).not.toHaveClass('has-note')

    fireEvent.click(within(judge).getByRole('button', { name: 'Add note for Judge' }))
    const elementNote = screen.getByLabelText('Element note')
    fireEvent.change(elementNote, { target: { value: 'Use this as the release judge.' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save element note' }))

    await waitFor(() => expect(judge).toHaveClass('has-note'))
  })

  it('shows only assignable persona cards in the formation staffing roster', async () => {
    await renderCockpit()
    const roster = screen.getByTestId('agent-roster')
    expect(screen.getByTestId('roster-count')).toHaveTextContent('2')
    expect(roster).toHaveTextContent('Mason')
    expect(roster).toHaveTextContent('Hazel')
    expect(roster).not.toHaveTextContent('scratch')
  })

  it('renders judge connections as wires anchored on the gate socket', async () => {
    const { container } = await renderCockpit()
    await waitFor(() => {
      const judgeWires = container.querySelectorAll('path.wire.judge')
      expect(judgeWires.length).toBe(2)
    })
    expect(container.querySelector('[data-gate-judge-socket="gate_review"]')).toBeTruthy()
    expect(screen.getByTestId('gate-node-gate_review').className).toContain('hasjudge')
  })

  it('renders a non-executing Tool with its frozen identity, parameters, and declared work ports', async () => {
    patches = installFetchMock({ boards: [makeToolBoard()] })

    const { container } = await renderCockpit()
    const card = await screen.findByTestId('tool-node-tool_normalize')

    expect(card).toHaveClass('toolcard')
    expect(card).not.toHaveClass('formation')
    expect(card).not.toHaveClass('missioncard')
    expect(card).not.toHaveClass('gatecard')
    expect(card).toHaveAttribute('data-kind', 'tool')
    expect(card).toHaveAttribute('data-node', 'tool_normalize')
    expect(card).toHaveAttribute('data-execution-state', 'unavailable')
    expect(screen.getByTestId('formations-world')).toContainElement(card)
    expect(card).toHaveStyle({ left: '1680px', top: '364px' })
    expect(card).toHaveTextContent('Normalize report')
    expect(card).toHaveTextContent('json.normalize@1')
    expect(card).toHaveTextContent('execution unavailable')
    expect(card).toHaveTextContent('Report')
    expect(card).toHaveTextContent('Normalized report')
    expect(container.querySelector('[data-port-in="tool_normalize:port_tool_input"]')).toBeTruthy()
    expect(container.querySelector('[data-port-out="tool_normalize:port_tool_output"]')).toBeTruthy()
    await waitFor(() => expect(screen.getByTestId('formation-wire-edge_tool_chain')).toBeInTheDocument())
    await waitFor(() => expect(screen.getByTestId('formation-wire-edge_tool_gate')).toBeInTheDocument())
    expect(within(card).getAllByRole('button').map(button => button.getAttribute('aria-label'))).toEqual([
      'Add note for Normalize report',
      'Inspect Tool Normalize report',
    ])
    fireEvent.contextMenu(card)
    expect(screen.queryByRole('menu')).toBeNull()
    expect(patches).toEqual([])
    expect(recordedMutations).toEqual([])
  })

  it('opens a read-only Tool inspector with the complete frozen projection', async () => {
    patches = installFetchMock({ boards: [makeToolBoard()] })
    await renderCockpit()

    fireEvent.click(await screen.findByRole('button', { name: 'Inspect Tool Normalize report' }))
    const dialog = await screen.findByRole('dialog', { name: 'Tool details: Normalize report' })

    expect(dialog).toHaveTextContent('tool_normalize')
    expect(dialog).toHaveTextContent('json.normalize@1')
    expect(dialog).toHaveTextContent('execution unavailable')

    const parameter = within(dialog).getByTestId('tool-parameter-mode')
    expect(parameter).toHaveTextContent('mode')
    expect(parameter).toHaveTextContent('string')
    expect(parameter).toHaveTextContent('strict')

    const input = within(dialog).getByTestId('tool-port-port_tool_input')
    expect(input).toHaveAttribute('data-direction', 'input')
    expect(input).toHaveTextContent('port_tool_input')
    expect(input).toHaveTextContent('input')
    expect(input).toHaveTextContent('Report')
    expect(input).toHaveTextContent('work')
    expect(input).toHaveTextContent('application/json')
    expect(input).toHaveTextContent('required=true')
    expect(input).toHaveTextContent('role=data')

    const output = within(dialog).getByTestId('tool-port-port_tool_output')
    expect(output).toHaveAttribute('data-direction', 'output')
    expect(output).toHaveTextContent('port_tool_output')
    expect(output).toHaveTextContent('output')
    expect(output).toHaveTextContent('Normalized report')
    expect(output).toHaveTextContent('work')
    expect(output).toHaveTextContent('application/json')
    expect(output).not.toHaveTextContent('required')
    expect(output).not.toHaveTextContent('data')
    expect(within(dialog).getAllByRole('button').map(button => button.getAttribute('aria-label'))).toEqual([
      'Close Tool details',
    ])

    fireEvent.click(within(dialog).getByRole('button', { name: 'Close Tool details' }))
    expect(dialog).not.toBeInTheDocument()
    expect(patches).toEqual([])
    expect(recordedMutations).toEqual([])
  })

  it('keeps an inspection-readable invalid Tool visible when optional arrays decode as null', async () => {
    const base = makeBoard()
    const degradedTool = {
      ...tool,
      params: null,
      inputs: [{ ...tool.inputs[0], acceptedMediaTypes: null }],
      outputs: null,
    } as unknown as typeof tool
    patches = installFetchMock({ boards: [{ ...base, schema: 2, tools: [degradedTool] }] })

    await renderCockpit()
    const card = await screen.findByTestId('tool-node-tool_normalize')
    expect(card).toHaveTextContent('Normalize report')
    expect(card).toHaveTextContent('json.normalize@1')
    expect(card).toHaveTextContent('execution unavailable')
    expect(card).toHaveTextContent('Report')

    fireEvent.click(within(card).getByRole('button', { name: 'Inspect Tool Normalize report' }))
    const dialog = await screen.findByRole('dialog', { name: 'Tool details: Normalize report' })
    expect(dialog).toHaveTextContent('tool_normalize')
    expect(within(dialog).getByTestId('tool-port-port_tool_input')).toHaveTextContent('media not declared')
    expect(within(dialog).queryByTestId('tool-parameter-mode')).toBeNull()
    expect(recordedMutations).toEqual([])
  })

  it('preserves readable Tool projection order and distinguishes false from unset input metadata', async () => {
    const base = makeBoard()
    const inspectionTool = {
      ...tool,
      params: { zeta: true, alpha: 7 },
      inputs: [
        {
          ...tool.inputs[0],
          id: 'port_false_input',
          name: 'false_input',
          label: 'False input',
          acceptedMediaTypes: ['application/json', 'text/plain'],
          required: false,
          role: undefined,
        },
        {
          ...tool.inputs[0],
          id: 'port_unset_input',
          name: 'unset_input',
          label: 'Unset input',
          acceptedMediaTypes: ['text/markdown', 'application/json'],
          required: undefined,
          role: undefined,
        },
      ],
      outputs: [
        { ...tool.outputs[0], id: 'port_first_output', name: 'first_output', label: 'First output' },
        { ...tool.outputs[0], id: 'port_second_output', name: 'second_output', label: 'Second output' },
      ],
    } as unknown as typeof tool
    patches = installFetchMock({ boards: [{ ...base, schema: 2, tools: [inspectionTool] }] })

    await renderCockpit()
    const card = await screen.findByTestId('tool-node-tool_normalize')
    expect(Array.from(card.querySelectorAll('.tool-ports.inputs .tool-port-label'), node => node.textContent)).toEqual([
      'False input',
      'Unset input',
    ])
    expect(Array.from(card.querySelectorAll('.tool-ports.outputs .tool-port-label'), node => node.textContent)).toEqual([
      'First output',
      'Second output',
    ])

    fireEvent.click(within(card).getByRole('button', { name: 'Inspect Tool Normalize report' }))
    const dialog = await screen.findByRole('dialog', { name: 'Tool details: Normalize report' })
    expect(within(dialog).getAllByTestId(/^tool-parameter-/).map(row => row.getAttribute('data-testid'))).toEqual([
      'tool-parameter-alpha',
      'tool-parameter-zeta',
    ])
    const alphaParameter = within(dialog).getByTestId('tool-parameter-alpha')
    expect(within(alphaParameter).getByText('alpha')).toBeInTheDocument()
    expect(within(alphaParameter).getByText('integer')).toBeInTheDocument()
    expect(within(alphaParameter).getByText('7')).toBeInTheDocument()
    const zetaParameter = within(dialog).getByTestId('tool-parameter-zeta')
    expect(within(zetaParameter).getByText('zeta')).toBeInTheDocument()
    expect(within(zetaParameter).getByText('boolean')).toBeInTheDocument()
    expect(within(zetaParameter).getByText('true')).toBeInTheDocument()

    const projectedPorts = within(dialog).getAllByTestId(/^tool-port-/)
    expect(projectedPorts.map(port => port.getAttribute('data-testid'))).toEqual([
      'tool-port-port_false_input',
      'tool-port-port_unset_input',
      'tool-port-port_first_output',
      'tool-port-port_second_output',
    ])
    expect(projectedPorts[0]).toHaveTextContent('application/json · text/plain')
    expect(projectedPorts[0]).toHaveTextContent('required=false')
    expect(projectedPorts[0]).toHaveTextContent('role=unset')
    expect(projectedPorts[1]).toHaveTextContent('text/markdown · application/json')
    expect(projectedPorts[1]).toHaveTextContent('required=unset')
    expect(projectedPorts[1]).toHaveTextContent('role=unset')
    expect(patches).toEqual([])
    expect(recordedMutations).toEqual([])
  })

  it('does not reopen a Tool inspector when the same board removes and later restores that node ID', async () => {
    const initialBoard = makeToolBoard()
    const refreshes: TestBoard[] = []
    const changePolls: Array<() => void> = []
    let timerId = 0
    vi.spyOn(window, 'setInterval').mockImplementation((handler, timeout) => {
      timerId += 1
      if (timeout === 600) changePolls.push(() => handler())
      return timerId as unknown as ReturnType<typeof setInterval>
    })
    const withoutTool = {
      ...initialBoard,
      rev: initialBoard.rev + 1,
      etag: 'board-without-tool-etag',
      tools: [upstreamTool],
      connections: initialBoard.connections.filter(connection => (
        !connection.from.startsWith('tool_normalize:') && !connection.to.startsWith('tool_normalize:')
      )),
    }
    const restoredTool = {
      ...initialBoard,
      rev: initialBoard.rev + 2,
      etag: 'board-restored-tool-etag',
      tools: [upstreamTool, { ...tool, title: 'Restored normalize report' }],
    }
    patches = installFetchMock({ boards: [initialBoard], sameBoardRefreshes: refreshes })
    await renderCockpit()
    await waitFor(() => expect(changePolls).toHaveLength(1))

    fireEvent.click(await screen.findByRole('button', { name: 'Inspect Tool Normalize report' }))
    await screen.findByRole('dialog', { name: 'Tool details: Normalize report' })

    refreshes.push(withoutTool)
    await act(async () => { changePolls[0]() })
    await waitFor(() => {
      expect(screen.queryByTestId('tool-node-tool_normalize')).toBeNull()
      expect(screen.queryByRole('dialog', { name: 'Tool details: Normalize report' })).toBeNull()
    })
    await waitFor(() => expect(changePolls.length).toBeGreaterThan(1))

    refreshes.push(restoredTool)
    await act(async () => { changePolls[changePolls.length - 1]() })
    const restoredCard = await screen.findByTestId('tool-node-tool_normalize')
    expect(restoredCard).toHaveTextContent('Restored normalize report')
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(patches).toEqual([])
    expect(recordedMutations).toEqual([])
  })

  it('collects Mission fields and creates only after a safe Bead ID', async () => {
    vi.spyOn(window, 'requestAnimationFrame').mockReturnValue(1)
    const { container } = await renderCockpit()
    const viewport = container.querySelector('.viewport') as HTMLElement
    fireEvent.contextMenu(viewport, { clientX: 300, clientY: 300 })
    const menu = await screen.findByRole('menu', { name: 'New' })
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Mission' }))

    const dialog = await screen.findByRole('dialog', { name: 'Create mission' })
    expect(menu).not.toBeInTheDocument()
    expect(patches.filter(patch => patch.body.createMission)).toEqual([])

    fireEvent.click(screen.getByRole('button', { name: 'Create mission' }))
    expect(await screen.findByText('Enter a Beads issue ID such as ctx-ug7.25.')).toBeInTheDocument()
    expect(screen.getByLabelText('Mission Bead ID')).toHaveAttribute('aria-invalid', 'true')
    expect(patches.filter(patch => patch.body.createMission)).toEqual([])

    fireEvent.change(screen.getByLabelText('Mission title'), { target: { value: '  Plan release  ' } })
    fireEvent.change(screen.getByLabelText('Mission goal'), { target: { value: '  Ship reduced candidate  ' } })
    fireEvent.change(screen.getByLabelText('Mission Bead ID'), { target: { value: ' home-vdki.34.1 ' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create mission' }))

    await waitFor(() => {
      const create = patches.find(patch => patch.body.createMission)
      expect(create?.body.createMission).toEqual({
        title: 'Plan release',
        goal: 'Ship reduced candidate',
        beadId: 'home-vdki.34.1',
        x: 1260,
        y: 252,
      })
    })
    expect(dialog).not.toBeInTheDocument()

    fireEvent.keyDown(window, { key: 'z', ctrlKey: true })
    await waitFor(() => {
      expect(patches.some(patch => (patch.body.deleteMission as { id?: string } | undefined)?.id === 'mis_created')).toBe(true)
    })
  })

  it('cancels Mission creation with Cancel and Escape without mutation', async () => {
    const { container } = await renderCockpit()
    const viewport = container.querySelector('.viewport') as HTMLElement

    fireEvent.contextMenu(viewport, { clientX: 300, clientY: 300 })
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Mission' }))
    await screen.findByRole('dialog', { name: 'Create mission' })
    fireEvent.change(screen.getByLabelText('Mission title'), { target: { value: 'Discard me' } })
    fireEvent.click(screen.getByRole('button', { name: 'Cancel mission creation' }))
    expect(screen.queryByRole('dialog', { name: 'Create mission' })).toBeNull()

    fireEvent.contextMenu(viewport, { clientX: 360, clientY: 360 })
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Mission' }))
    await screen.findByRole('dialog', { name: 'Create mission' })
    fireEvent.change(screen.getByLabelText('Mission goal'), { target: { value: 'Discard this too' } })
    fireEvent.keyDown(window, { key: 'Escape' })
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Create mission' })).toBeNull())

    expect(patches.filter(patch => patch.body.createMission)).toEqual([])
  })

  it('retains the Mission draft after the API rejects creation', async () => {
    patches = installFetchMock({ missionCreateFailure: true })
    const { container } = await renderCockpit()
    const viewport = container.querySelector('.viewport') as HTMLElement
    fireEvent.contextMenu(viewport, { clientX: 300, clientY: 300 })
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Mission' }))
    await screen.findByRole('dialog', { name: 'Create mission' })

    fireEvent.change(screen.getByLabelText('Mission title'), { target: { value: 'Plan release' } })
    fireEvent.change(screen.getByLabelText('Mission goal'), { target: { value: 'Ship reduced candidate' } })
    fireEvent.change(screen.getByLabelText('Mission Bead ID'), { target: { value: 'ctx-ug7.25' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create mission' }))

    expect(await screen.findByTestId('formations-error')).toHaveTextContent('Mission create failed')
    expect(screen.getByRole('dialog', { name: 'Create mission' })).toBeInTheDocument()
    expect(screen.getByLabelText('Mission title')).toHaveValue('Plan release')
    expect(screen.getByLabelText('Mission goal')).toHaveValue('Ship reduced candidate')
    expect(screen.getByLabelText('Mission Bead ID')).toHaveValue('ctx-ug7.25')
  })

  it('adopts a created gate layout without issuing a stale follow-up layout patch', async () => {
    patches = installFetchMock({ freshCreateLayout: true })
    const { container } = await renderCockpit()
    const viewport = container.querySelector('.viewport') as HTMLElement
    fireEvent.contextMenu(viewport, { clientX: 300, clientY: 300 })
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Gate' }))
    const dialog = await screen.findByRole('dialog', { name: 'Create code Gate' })
    fireEvent.change(within(dialog).getByLabelText('Forbidden text'), { target: { value: 'error' } })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Create Gate' }))

    const created = await screen.findByTestId('gate-node-gate_created')
    await waitFor(() => {
      expect(created).toHaveStyle({ left: '1344px', top: '784px' })
    })
    expect(patches.filter(patch => patch.url.endsWith('/layout'))).toEqual([])
  })

  it('creates a code Gate from a backend-registered exact profile tuple', async () => {
    patches = installFetchMock({ freshCreateLayout: true })
    const { container } = await renderCockpit()
    const viewport = container.querySelector('.viewport') as HTMLElement
    fireEvent.contextMenu(viewport, { clientX: 300, clientY: 300 })
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Gate' }))

    const dialog = await screen.findByRole('dialog', { name: 'Create code Gate' })
    fireEvent.change(within(dialog).getByLabelText('Evaluator profile'), { target: { value: 'output_contains@1' } })
    fireEvent.change(within(dialog).getByLabelText('Required text'), { target: { value: 'LINT OK' } })
    fireEvent.change(within(dialog).getByLabelText('Gate criterion'), { target: { value: 'Lint passes clean' } })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Create Gate' }))

    await waitFor(() => {
      const create = patches.find(patch => patch.body.createGate)?.body.createGate
      expect(create).toMatchObject({
        kinds: ['code'],
        criterion: 'Lint passes clean',
        check: 'output_contains',
        checkVersion: '1',
        checkValue: 'LINT OK',
      })
    })
  })

  it('dismisses context menus on Escape and outside pointerdown', async () => {
    const { container } = await renderCockpit()
    const viewport = container.querySelector('.viewport') as HTMLElement
    fireEvent.contextMenu(viewport, { clientX: 300, clientY: 300 })
    const menu = await screen.findByRole('menu', { name: 'New' })
    expect(menu.parentElement).toBe(document.body)
    fireEvent.keyDown(window, { key: 'Escape' })
    await waitFor(() => expect(screen.queryByRole('menu', { name: 'New' })).toBeNull())

    fireEvent.contextMenu(viewport, { clientX: 300, clientY: 300 })
    await screen.findByRole('menu', { name: 'New' })
    fireEvent.pointerDown(document.body)
    await waitFor(() => expect(screen.queryByRole('menu', { name: 'New' })).toBeNull())
  })

  it('unassigns a staffed slot from its context menu', async () => {
    await renderCockpit()
    fireEvent.contextMenu(screen.getByTestId('slot-fmn_frame-slot_lead'))
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Unassign mason' }))
    await waitFor(() => {
      const assignment = patches.map(patch => patch.body.assignSlot as { slotId?: string; agentId?: string } | undefined).find(Boolean)
      expect(assignment).toEqual(expect.objectContaining({ slotId: 'slot_lead', agentId: '' }))
    })
  })

  it('adds an input port from the formation context menu', async () => {
    await renderCockpit()
    fireEvent.contextMenu(screen.getByTestId('formation-node-fmn_frame'))
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Add input port' }))
    await waitFor(() => {
      const port = patches.map(patch => patch.body.addPort as { direction?: string } | undefined).find(Boolean)
      expect(port).toEqual(expect.objectContaining({ direction: 'in' }))
    })
  })

  it('shows legacy inline verification as read-only migration input', async () => {
    await renderCockpit()
    const band = screen.getByRole('button', { name: 'Inspect legacy verification for Frame' })
    expect(band).toBe(screen.getByTestId('verify-band-fmn_frame'))
    band.focus()
    fireEvent.click(band)
    const dialog = await screen.findByRole('dialog', { name: 'Legacy verification · Frame' })
		expect(dialog).toHaveAttribute('aria-modal', 'true')
		expect(dialog).toHaveAccessibleDescription(/Inline verification is retired/)
		expect(screen.getByRole('button', { name: 'Close legacy verification' })).toHaveFocus()
		expect(dialog).toHaveTextContent('Tests pass')
		expect(dialog).toHaveTextContent('Create and wire an explicit Gate')
		expect(screen.queryByRole('button', { name: 'Save verification' })).toBeNull()
		expect(screen.getByLabelText('Replacement Gate')).toHaveValue('')
		expect(screen.getByRole('button', { name: 'Remove legacy verification' })).toBeDisabled()
		fireEvent.change(screen.getByLabelText('Replacement Gate'), { target: { value: 'gate_review' } })
		fireEvent.click(screen.getByRole('button', { name: 'Remove legacy verification' }))
    await waitFor(() => {
      const removal = patches.map(patch => patch.body.removeVerification as { formationId?: string; replacementGateId?: string } | undefined).find(Boolean)
      expect(removal).toEqual({ formationId: 'fmn_frame', replacementGateId: 'gate_review' })
    })
    expect(patches.some(patch => patch.body.setVerification)).toBe(false)
    await waitFor(() => expect(dialog).not.toBeInTheDocument())
    expect(band).toHaveFocus()
  })

  it('restores the migration trigger after button and Escape dismissal', async () => {
    await renderCockpit()
    const band = screen.getByRole('button', { name: 'Inspect legacy verification for Frame' })

    band.focus()
    fireEvent.click(band)
    const close = await screen.findByRole('button', { name: 'Close legacy verification' })
    close.focus()
    fireEvent.click(close)
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Legacy verification · Frame' })).toBeNull())
    expect(band).toHaveFocus()

    fireEvent.click(band)
    await screen.findByRole('dialog', { name: 'Legacy verification · Frame' })
    fireEvent.keyDown(window, { key: 'Escape' })
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Legacy verification · Frame' })).toBeNull())
    expect(band).toHaveFocus()
  })

  it('does not offer new inline verification authoring', async () => {
    await renderCockpit()
    fireEvent.contextMenu(screen.getByTestId('formation-node-fmn_judge'))
    const menu = await screen.findByRole('menu', { name: 'Formation actions' })
    expect(menu).not.toHaveTextContent('Add verification')
  })

  it('routes legacy removal through the migration dialog and keeps it open on failure', async () => {
    patches = installFetchMock({ removalFailure: true })
    await renderCockpit()
    fireEvent.contextMenu(screen.getByTestId('formation-node-fmn_frame'))
    const menu = await screen.findByRole('menu', { name: 'Formation actions' })
    expect(menu).not.toHaveTextContent('Remove legacy verification')
		fireEvent.click(screen.getByRole('menuitem', { name: 'Migrate legacy verification' }))
		const dialog = await screen.findByRole('dialog', { name: 'Legacy verification · Frame' })
		fireEvent.change(screen.getByLabelText('Replacement Gate'), { target: { value: 'gate_review' } })
		fireEvent.click(screen.getByRole('button', { name: 'Remove legacy verification' }))
    expect(await screen.findByTestId('formations-error')).toHaveTextContent('Legacy verification migration failed')
    const localError = await within(dialog).findByRole('alert')
    expect(localError).toHaveTextContent('Could not remove legacy verification')
    expect(localError).not.toHaveTextContent('Legacy verification migration failed')
    expect(dialog).toBeInTheDocument()
  })

  it('locks Gate selection and duplicate removal while migration is pending', async () => {
    let releaseRemoval: (() => void) | undefined
    const removalGate = new Promise<void>(resolve => { releaseRemoval = resolve })
    patches = installFetchMock({ removalGate })
    await renderCockpit()
    const band = screen.getByRole('button', { name: 'Inspect legacy verification for Frame' })
    band.focus()
    fireEvent.click(band)
    const dialog = await screen.findByRole('dialog', { name: 'Legacy verification · Frame' })
    const replacement = within(dialog).getByLabelText('Replacement Gate')
    const remove = within(dialog).getByRole('button', { name: 'Remove legacy verification' })
    fireEvent.change(replacement, { target: { value: 'gate_review' } })
    fireEvent.click(remove)

    expect(await within(dialog).findByRole('status')).toHaveTextContent('Removing legacy verification')
    expect(replacement).toBeDisabled()
    expect(remove).toBeDisabled()
    fireEvent.click(remove)
    expect(patches.filter(patch => patch.body.removeVerification)).toHaveLength(1)

    await act(async () => releaseRemoval?.())
    await waitFor(() => expect(dialog).not.toBeInTheDocument())
    expect(band).toHaveFocus()
  })

  it('renders partial legacy verification without inventing missing evidence', async () => {
    const partialBoard = makeBoard()
    partialBoard.formations = [
      { ...formation, verification: { id: 'ver_partial' } as typeof formation.verification },
      judgeFormation,
    ]
    patches = installFetchMock({ boards: [partialBoard] })
    await renderCockpit()
    const band = screen.getByTestId('verify-band-fmn_frame')
    expect(band).toHaveTextContent('checks not recorded')
    fireEvent.click(band)
    const dialog = await screen.findByRole('dialog', { name: 'Legacy verification · Frame' })
    expect(dialog).toHaveTextContent('No criterion recorded')
		expect(dialog).toHaveTextContent('No failure policy recorded')
	})

	it('requires an explicit choice when multiple replacement Gates are wired', async () => {
		const multiGateBoard = makeBoard()
		multiGateBoard.gates = [
			...multiGateBoard.gates,
			{ id: 'gate_backup', title: 'Backup review', kinds: ['human'], criterion: 'Review again' },
		]
		multiGateBoard.connections = [
			...multiGateBoard.connections,
			{ id: 'edge_frame_backup', from: 'fmn_frame:port_frame_out', to: 'gate_backup:in' },
		]
		patches = installFetchMock({ boards: [multiGateBoard] })
		await renderCockpit()
		fireEvent.click(screen.getByTestId('verify-band-fmn_frame'))
		await screen.findByRole('dialog', { name: 'Legacy verification · Frame' })
		const replacement = screen.getByLabelText('Replacement Gate')
		expect(replacement).toHaveValue('')
		expect(screen.getByRole('button', { name: 'Remove legacy verification' })).toBeDisabled()
		fireEvent.change(replacement, { target: { value: 'gate_backup' } })
		fireEvent.click(screen.getByRole('button', { name: 'Remove legacy verification' }))
		await waitFor(() => {
			const removal = patches.map(patch => patch.body.removeVerification as { replacementGateId?: string } | undefined).find(Boolean)
			expect(removal).toEqual(expect.objectContaining({ replacementGateId: 'gate_backup' }))
		})
	})

	it('requires an explicitly wired replacement Gate before removal', async () => {
    const unwiredBoard = makeBoard()
    unwiredBoard.gates = []
    unwiredBoard.connections = unwiredBoard.connections.filter(connection => !connection.to.startsWith('gate_review:') && !connection.from.startsWith('gate_review:'))
    patches = installFetchMock({ boards: [unwiredBoard] })
    await renderCockpit()
    fireEvent.click(screen.getByTestId('verify-band-fmn_frame'))
    const dialog = await screen.findByRole('dialog', { name: 'Legacy verification · Frame' })
    expect(dialog).toHaveTextContent('Wire an explicit Gate from a Formation output before removal')
    expect(screen.getByRole('button', { name: 'Remove legacy verification' })).toBeDisabled()
    expect(patches.filter(patch => patch.body.removeVerification)).toEqual([])
  })

  it('closes a legacy migration dialog when the selected board changes', async () => {
    const secondBoard = { ...makeBoard(), id: 'brd_second', slug: 'second-board', title: 'Second board', etag: 'second-etag' }
    patches = installFetchMock({ boards: [makeBoard(), secondBoard] })
    await renderCockpit()
    const band = screen.getByTestId('verify-band-fmn_frame')
    band.focus()
    fireEvent.click(band)
    const dialog = await screen.findByRole('dialog', { name: 'Legacy verification · Frame' })
    within(dialog).getByLabelText('Replacement Gate').focus()
    fireEvent.change(screen.getByTestId('board-picker'), { target: { value: 'second-board' } })
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Legacy verification · Frame' })).toBeNull())
    expect(screen.getByTestId('verify-band-fmn_frame')).toHaveFocus()
    expect(patches.filter(patch => patch.body.removeVerification)).toEqual([])
  })

  it('labels historical verification verdicts as non-authorizing evidence', async () => {
    localStorage.setItem('chrote-formations-active-run-test-board', 'run_legacy')
    patches = installFetchMock({
      runEvents: [
        { runId: 'run_legacy', seq: 1, type: 'node_output', nodeId: 'fmn_frame' },
        { runId: 'run_legacy', seq: 2, type: 'verification_verdict', nodeId: 'fmn_frame', data: { verdict: 'fail', feedback: 'do not render raw feedback' } },
      ],
    })
    await renderCockpit()
    await waitFor(() => expect(fetch).toHaveBeenCalledWith('/api/formations/runs/run_legacy/events', expect.anything()))
    fireEvent.click(screen.getByTestId('verify-band-fmn_frame'))
    const dialog = await screen.findByRole('dialog', { name: 'Legacy verification · Frame' })
    expect(dialog).toHaveTextContent('Legacy verification evidence · non-authorizing')
    expect(dialog).toHaveTextContent('seq 2 · fail')
    expect(dialog).not.toHaveTextContent('do not render raw feedback')
  })

  it('opens a node evidence inspector with attempts, inline output, and reportRef', async () => {
    localStorage.setItem('chrote-formations-active-run-test-board', 'run_legacy')
    patches = installFetchMock({
      runEvents: [
        { runId: 'run_legacy', seq: 1, type: 'node_started', nodeId: 'fmn_frame', attempt: 1, data: { reason: 'single-formation' } },
        { runId: 'run_legacy', seq: 2, type: 'slot_dispatch', nodeId: 'fmn_frame', attempt: 1, data: { slotId: 'slot_lead', agentId: 'mason', harness: 'codex', promptSha256: 'deadbeefcafebabe0011' } },
        { runId: 'run_legacy', seq: 3, type: 'node_output', nodeId: 'fmn_frame', data: { status: 'done', text: 'the framed output', reportRef: 'reports/fmn_frame.md', outputs: { port_frame_out: { text: 'the framed output', reportRef: 'reports/fmn_frame.md' } } } },
      ],
    })
    await renderCockpit()
    await waitFor(() => expect(fetch).toHaveBeenCalledWith('/api/formations/runs/run_legacy/events', expect.anything()))

    fireEvent.click(await screen.findByTestId('inspect-node-fmn_frame'))
    const dialog = await screen.findByTestId('node-inspector')
    expect(within(dialog).getByTestId('node-evidence-state')).toHaveTextContent('done')
    expect(within(dialog).getByTestId('node-attempt-1')).toHaveTextContent('single-formation')
    expect(dialog).toHaveTextContent('mason')
    expect(dialog).toHaveTextContent('codex')
    expect(within(dialog).getByTestId('node-output-value')).toHaveTextContent('the framed output')
    expect(within(dialog).getByTestId('node-output-reportref')).toHaveTextContent('reports/fmn_frame.md')
    expect(within(dialog).getByTestId('node-output-port-port_frame_out')).toBeInTheDocument()

    fireEvent.click(within(dialog).getByRole('button', { name: 'Close run evidence' }))
    await waitFor(() => expect(screen.queryByTestId('node-inspector')).toBeNull())
  })

  it('surfaces open escalations as a needs-you banner and marks the escalated node', async () => {
    localStorage.setItem('chrote-formations-active-run-test-board', 'run_legacy')
    patches = installFetchMock({
      runStatus: { status: 'blocked', final: false, resumeAllowed: true },
      runEvents: [
        { runId: 'run_legacy', seq: 1, type: 'gate_evaluating', nodeId: 'gate_review', gateId: 'gate_review' },
      ],
      escalations: [
        { runId: 'run_legacy', seq: 5, gateId: 'gate_review', nodeId: 'gate_review', severity: 'stop', reason: 'operator taste needed', source: 'agent', trigger: 'sentinel', blocks: true },
      ],
    })
    await renderCockpit()
    await waitFor(() => expect(fetch).toHaveBeenCalledWith('/api/formations/runs/run_legacy/escalations', expect.anything()))

    const banner = await screen.findByTestId('escalations-banner')
    expect(banner).toHaveTextContent('Needs you')
    const item = within(banner).getByTestId('escalation-5')
    expect(item).toHaveTextContent('operator taste needed')
    expect(item).toHaveTextContent('gate_review')
    expect(item).toHaveTextContent('stop')
    await waitFor(() => expect(screen.getByTestId('gate-node-gate_review')).toHaveClass('needs-you'))

    fireEvent.click(item)
    expect(await screen.findByTestId('node-inspector')).toHaveTextContent('Run evidence · Review')
  })

  it('detaches the judge from the gate context menu', async () => {
    await renderCockpit()
    fireEvent.contextMenu(screen.getByTestId('gate-node-gate_review'))
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Detach judge' }))
    await waitFor(() => {
      const detach = patches.map(patch => patch.body.detachGateJudge as { gateId?: string } | undefined).find(Boolean)
      expect(detach).toEqual(expect.objectContaining({ gateId: 'gate_review' }))
    })
  })

  it('deletes a mission from its context menu', async () => {
    await renderCockpit()
    fireEvent.contextMenu(screen.getByTestId('mission-node-mis_showcase'))
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Delete mission' }))
    await waitFor(() => {
      const removal = patches.map(patch => patch.body.deleteMission as { id?: string } | undefined).find(Boolean)
      expect(removal).toEqual(expect.objectContaining({ id: 'mis_showcase' }))
    })
  })

  it('uses the shared explicit Arrange operation for whole-board movement', async () => {
    await renderCockpit()
    fireEvent.click(screen.getByTestId('arrange-layout'))
    await waitFor(() => {
      expect(patches).toContainEqual({
        url: '/api/formations/boards/test-board/layout',
        body: { arrange: true },
      })
    })
  })

  it('cancels the active interaction when the cockpit unmounts', async () => {
    const { container, unmount } = await renderCockpit()
    const viewport = container.querySelector('.viewport') as HTMLElement

    fireEvent.pointerDown(viewport, { button: 0, pointerId: 5, clientX: 800, clientY: 600 })
    fireEvent.pointerMove(window, { pointerId: 5, clientX: 830, clientY: 620 })
    expect(viewport).toHaveClass('panning')

    unmount()
    expect(viewport).not.toHaveClass('panning')
  })
})
