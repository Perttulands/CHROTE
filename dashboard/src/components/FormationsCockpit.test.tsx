import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
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
    connections: [
      { id: 'edge_mission_frame', from: 'mis_showcase:out', to: 'fmn_frame:port_frame_in' },
      { id: 'edge_frame_gate', from: 'fmn_frame:port_frame_out', to: 'gate_review:in' },
      { id: 'edge_judge_send', from: 'gate_review:judge', to: 'fmn_judge:port_judge_in' },
      { id: 'edge_judge_return', from: 'fmn_judge:port_judge_out', to: 'gate_review:judge' },
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

function installFetchMock(options: { emptyBoards?: boolean } = {}) {
  const patches: RecordedPatch[] = []
  let board = makeBoard()
  ;(globalThis as Record<string, unknown>).fetch = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input)
    const respond = (data: unknown, etag = '') => Promise.resolve({
      ok: true,
      headers: { get: (name: string) => (name.toLowerCase() === 'etag' ? etag || null : null) },
      json: () => Promise.resolve({ success: true, data }),
      text: () => Promise.resolve(''),
    })
    if (init?.method === 'PATCH') {
      const body = JSON.parse(String(init.body)) as Record<string, unknown>
      patches.push({ url, body })
      board = { ...board, rev: board.rev + 1 }
      if (url.endsWith('/layout')) return respond({ layout }, 'layout-etag-2')
      return respond({ board }, 'board-etag-2')
    }
    if (url === '/api/formations/boards') return respond({ boards: options.emptyBoards ? [] : [{ slug: board.slug, title: board.title }] })
    if (url.includes('/changes')) return respond({ signal: { changed: false } })
    if (url.endsWith('/layout')) return respond({ layout }, 'layout-etag')
    if (url.includes('/api/formations/boards/')) return respond({ board }, 'board-etag')
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
    expect(screen.getByTestId('new-formation')).toBeDisabled()
    expect(patches).toEqual([])
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

  it('opens a New menu on canvas right-click and creates a mission from it', async () => {
    const { container } = await renderCockpit()
    const viewport = container.querySelector('.viewport') as HTMLElement
    fireEvent.contextMenu(viewport, { clientX: 300, clientY: 300 })
    const menu = await screen.findByRole('menu', { name: 'New' })
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Mission' }))
    await waitFor(() => {
      expect(patches.some(patch => (patch.body.createMission as { title?: string } | undefined)?.title === 'New mission')).toBe(true)
    })
    expect(menu).not.toBeInTheDocument()
  })

  it('dismisses context menus on Escape and outside pointerdown', async () => {
    const { container } = await renderCockpit()
    const viewport = container.querySelector('.viewport') as HTMLElement
    fireEvent.contextMenu(viewport, { clientX: 300, clientY: 300 })
    await screen.findByRole('menu', { name: 'New' })
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

  it('edits verification through the verify band editor', async () => {
    await renderCockpit()
    fireEvent.click(screen.getByTestId('verify-band-fmn_frame'))
    const dialog = await screen.findByRole('dialog', { name: 'Verification · Frame' })
    fireEvent.change(screen.getByLabelText('Criterion for Frame'), { target: { value: 'lint passes' } })
    fireEvent.change(screen.getByLabelText('On fail for Frame'), { target: { value: 'pushback' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save verification' }))
    await waitFor(() => {
      const verification = patches.map(patch => patch.body.setVerification as { criterion?: string; onFail?: string } | undefined).find(Boolean)
      expect(verification).toEqual(expect.objectContaining({ criterion: 'lint passes', onFail: 'pushback' }))
    })
    expect(dialog).not.toBeInTheDocument()
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
})
