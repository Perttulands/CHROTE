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
]

type RecordedPatch = { url: string; body: Record<string, unknown> }

function installFetchMock() {
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
    if (url === '/api/formations/boards') return respond({ boards: [{ slug: board.slug, title: board.title }] })
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
      expect(port).toEqual(expect.objectContaining({ direction: 'input' }))
    })
  })

  it('shows no per-formation run button (single-formation run is not a UI affordance)', async () => {
    await renderCockpit()
    // Mission run stays; formation run is intentionally gone (no FormationID run path on real boards).
    expect(screen.getByTestId('run-mission-mis_showcase')).toBeInTheDocument()
    expect(screen.queryByTestId('run-formation-fmn_frame')).toBeNull()
    fireEvent.contextMenu(screen.getByTestId('formation-node-fmn_frame'))
    await screen.findByRole('menu', { name: 'Formation actions' })
    expect(screen.queryByRole('menuitem', { name: 'Run formation' })).toBeNull()
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

  it('edits an existing mission from its context menu and emits a full-replace updateMission patch', async () => {
    // B4: a mission must be configurable (title/goal/optional bead), not just started/deleted.
    await renderCockpit()
    fireEvent.contextMenu(screen.getByTestId('mission-node-mis_showcase'))
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Edit mission…' }))
    const dialog = await screen.findByRole('dialog', { name: 'Mission · Showcase' })
    // The editor is seeded from the current mission so a save is a faithful full replace.
    expect((screen.getByLabelText('Title for Showcase') as HTMLInputElement).value).toBe('Showcase')
    expect((screen.getByLabelText('Goal for Showcase') as HTMLTextAreaElement).value).toBe('Build the page')
    expect((screen.getByLabelText('Bead for Showcase') as HTMLInputElement).value).toBe('home-7kc4.5')

    // Goal/Bead aria-labels track the live title, so edit the title last to keep the queries stable.
    fireEvent.change(screen.getByLabelText('Goal for Showcase'), { target: { value: 'Land it today' } })
    fireEvent.change(screen.getByLabelText('Bead for Showcase'), { target: { value: 'home-9zz9.2' } })
    fireEvent.change(screen.getByLabelText('Title for Showcase'), { target: { value: 'Ship the page' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save mission' }))

    await waitFor(() => {
      const update = patches.map(patch => patch.body.updateMission as { missionId?: string; title?: string; goal?: string; beadId?: string } | undefined).find(Boolean)
      expect(update).toEqual(expect.objectContaining({
        missionId: 'mis_showcase',
        title: 'Ship the page',
        goal: 'Land it today',
        beadId: 'home-9zz9.2',
      }))
    })
    expect(dialog).not.toBeInTheDocument()
  })

  it('opens the mission editor on double-click', async () => {
    await renderCockpit()
    fireEvent.doubleClick(screen.getByTestId('mission-node-mis_showcase'))
    await screen.findByRole('dialog', { name: 'Mission · Showcase' })
  })

  it('lets an emptied bead clear the mission link', async () => {
    // beadId is optional backend-side; an empty string clears it (no malformed value sent).
    await renderCockpit()
    fireEvent.doubleClick(screen.getByTestId('mission-node-mis_showcase'))
    await screen.findByRole('dialog', { name: 'Mission · Showcase' })
    fireEvent.change(screen.getByLabelText('Bead for Showcase'), { target: { value: '   ' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save mission' }))
    await waitFor(() => {
      const update = patches.map(patch => patch.body.updateMission as { beadId?: string } | undefined).find(Boolean)
      expect(update).toEqual(expect.objectContaining({ beadId: '' }))
    })
  })

  it('does not flag a real persisted board as a demo', async () => {
    await renderCockpit()
    expect(screen.queryByTestId('starter-demo-badge')).toBeNull()
  })
})

describe('FormationsCockpit starter board honesty', () => {
  beforeEach(() => {
    localStorage.clear()
    // No persisted boards on the host → the cockpit falls back to the in-memory starter board.
    ;(globalThis as Record<string, unknown>).fetch = vi.fn((input: RequestInfo | URL) => {
      const url = String(input)
      const respond = (data: unknown) => Promise.resolve({
        ok: true,
        headers: { get: () => null },
        json: () => Promise.resolve({ success: true, data }),
        text: () => Promise.resolve(''),
      })
      if (url === '/api/formations/boards') return respond({ boards: [] })
      if (url === '/api/agents') return respond({ agents: [] })
      return respond({})
    }) as unknown as typeof fetch
  })

  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  it('marks the in-memory starter board as a non-persistent demo', async () => {
    render(<FormationsCockpit />)
    // The badge must be visible so the user never mistakes the demo for a saved board.
    const badge = await screen.findByTestId('starter-demo-badge')
    expect(badge).toHaveTextContent('Demo · not saved')
  })
})

describe('FormationsCockpit output payloads', () => {
  beforeEach(() => {
    localStorage.clear()
    const runId = 'run_outputs'
    const events = [
      { runId, seq: 1, type: 'run_started', data: { actor: 'agent:ui' } },
      // D2: routing-truth payload lives in node_output.outputs[portId]; node_output.text is
      // a display summary only and must never be shown as the per-port payload.
      {
        runId,
        seq: 2,
        type: 'node_output',
        nodeId: 'fmn_frame',
        data: {
          status: 'done',
          text: 'summary that must not appear as the port payload',
          outputs: { port_frame_out: 'the real produced frame payload' },
        },
      },
    ]
    // In-flight run: the poller fetches the ledger (including node_output payloads) and projects it.
    const status = { runId, status: 'running', final: false, boardSlug: 'test-board', missionId: 'mis_showcase', eventCount: events.length }
    ;(globalThis as Record<string, unknown>).fetch = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      const respond = (data: unknown, etag = '') => Promise.resolve({
        ok: true,
        headers: { get: (name: string) => (name.toLowerCase() === 'etag' ? etag || null : null) },
        json: () => Promise.resolve({ success: true, data }),
        text: () => Promise.resolve(''),
      })
      if (url.endsWith('/events')) return respond({ events })
      if (url === '/api/formations/runs' && init?.method === 'POST') return respond({ runId, status })
      if (url.includes('/api/formations/runs/')) return respond(status)
      if (url === '/api/formations/boards') return respond({ boards: [{ slug: 'test-board', title: 'Test board' }] })
      if (url.includes('/changes')) return respond({ signal: { changed: false } })
      if (url.endsWith('/layout')) return respond({ layout }, 'layout-etag')
      if (url.includes('/api/formations/boards/')) return respond({ board: makeBoard() }, 'board-etag')
      if (url === '/api/agents') return respond({ agents })
      return respond({})
    }) as unknown as typeof fetch
  })

  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  it('renders the real produced payload on a port that produced output, and stays idle otherwise', async () => {
    const utils = render(<FormationsCockpit />)
    await screen.findByTestId('formation-node-fmn_frame')

    // Start the mission so the run ledger (with node_output payloads) is projected onto the card.
    fireEvent.click(screen.getByTestId('run-mission-mis_showcase'))

    await waitFor(() => {
      expect(screen.getByTestId('output-payload-fmn_frame-port_frame_out')).toHaveTextContent('the real produced frame payload')
    })
    // The free-form display summary must never become the per-port payload.
    expect(utils.container.textContent).not.toContain('summary that must not appear as the port payload')

    // A port the run never produced keeps its honest idle placeholder (not a hidden fallback).
    fireEvent.contextMenu(screen.getByTestId('formation-node-fmn_frame'))
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Add output port' }))
    // The judge formation's output never produced; its first output row remains idle.
    expect(screen.getByTestId('output-payload-fmn_judge-port_judge_out')).toHaveTextContent('no output yet')
  })
})
