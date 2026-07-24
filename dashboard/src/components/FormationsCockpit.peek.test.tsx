import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

/* RED: grab-the-wheel peek control in the node inspector (chrote-a7k).
   A running node whose live agent tmux session is attachable shows a
   "Peek / grab the wheel" button that reuses the existing floating terminal
   (openFloatingModal) to attach to that exact session. When the node is not
   running, or the ephemeral session is gone from the live registry, peek is
   not offered / not enabled. */

const mocks = vi.hoisted(() => ({ session: null as unknown }))

vi.mock('../context/SessionContext', () => ({
  useSessionOptional: () => mocks.session,
  useSession: () => mocks.session,
}))

import FormationsCockpit from './FormationsCockpit'

const SLUG = 'peek-board'
const MISSION_SESSION = 'mission-run1-slota-ab12'

const board = {
  schema: 1,
  id: 'brd_peek',
  slug: SLUG,
  title: 'Peek board',
  rev: 1,
  etag: 'etag-1',
  missions: [{ id: 'mis_p', title: 'Mission', goal: 'Do the thing', beadId: 'home-1.1' }],
  formations: [{
    id: 'fmn_work',
    type: 'solo',
    title: 'Work',
    inputs: [{ id: 'port_in', label: 'In' }],
    outputs: [{ id: 'port_out', label: 'Out' }],
    slots: [{ id: 'slot_a', label: 'A', agentId: 'mason', harness: 'codex' }],
  }],
  gates: [],
  tools: [],
  connections: [],
}

const layout = {
  schema: 1,
  boardId: 'brd_peek',
  boardRev: 1,
  etag: 'l1',
  nodes: [{ id: 'mis_p', x: 100, y: 100 }, { id: 'fmn_work', x: 420, y: 100 }],
  edges: [],
}

const agents = [{ id: 'mason', displayName: 'Mason', harnessDefault: 'codex', liveness: 'live', assignable: true, unbound: false }]

type Ev = { runId: string; seq: number; type: string; nodeId?: string; attempt?: number; data?: Record<string, unknown> }

function installFetchMock(runEvents: Ev[], runStatus: { status: string; final: boolean }) {
  const respond = (data: unknown, etag = '') => Promise.resolve({
    ok: true,
    headers: { get: (name: string) => (name.toLowerCase() === 'etag' ? etag || null : null) },
    json: () => Promise.resolve({ success: true, data }),
    text: () => Promise.resolve(''),
  })
  ;(globalThis as Record<string, unknown>).fetch = vi.fn((input: RequestInfo | URL) => {
    const url = String(input)
    if (/\/api\/formations\/runs\/[^/]+\/escalations$/.test(url)) return respond({ escalations: [] })
    if (/\/api\/formations\/runs\/[^/]+\/events$/.test(url)) return respond({ events: runEvents })
    if (/\/api\/formations\/runs\/[^/]+$/.test(url)) return respond({ status: { runId: 'run1', status: runStatus.status, final: runStatus.final, resumeAllowed: false, boardSlug: SLUG, missionId: 'mis_p', eventCount: runEvents.length } })
    if (url === '/api/formations/boards') return respond({ boards: [{ slug: board.slug, title: board.title }] })
    if (url.includes('/changes')) return respond({ signal: { changed: false } })
    if (url.endsWith('/layout')) return respond({ layout }, 'l1')
    if (url.includes('/api/formations/boards/')) return respond({ board }, board.etag)
    if (url === '/api/agents') return respond({ agents })
    return respond({})
  }) as unknown as typeof fetch
}

const runningEvents: Ev[] = [
  { runId: 'run1', seq: 1, type: 'node_started', nodeId: 'fmn_work', attempt: 1, data: { reason: 'single-formation' } },
  { runId: 'run1', seq: 2, type: 'slot_dispatch', nodeId: 'fmn_work', attempt: 1, data: { slotId: 'slot_a', agentId: 'mason', harness: 'codex', sessionRef: `tmux:${MISSION_SESSION}` } },
]

function stubSession(sessionNames: string[]) {
  return {
    settings: { formationsTextSize: 'm' },
    sessions: sessionNames.map(name => ({ name, unixUser: '' })),
    openFloatingModal: vi.fn(),
  }
}

async function openInspector() {
  render(<FormationsCockpit />)
  await screen.findByTestId('formation-node-fmn_work')
  await waitFor(() => expect(fetch).toHaveBeenCalledWith('/api/formations/runs/run1/events', expect.anything()))
  fireEvent.click(await screen.findByTestId('inspect-node-fmn_work'))
  return screen.findByTestId('node-inspector')
}

describe('FormationsCockpit peek / grab the wheel', () => {
  beforeEach(() => {
    localStorage.setItem('chrote-formations-active-run-peek-board', 'run1')
  })
  afterEach(() => {
    cleanup()
    localStorage.clear()
    vi.restoreAllMocks()
    mocks.session = null
  })

  it('offers an enabled peek button that attaches to the live session', async () => {
    mocks.session = stubSession([MISSION_SESSION])
    installFetchMock(runningEvents, { status: 'running', final: false })
    const dialog = await openInspector()
    const peek = within(dialog).getByTestId('peek-node-fmn_work')
    expect(peek).toBeEnabled()
    expect(dialog).toHaveTextContent(/grab the wheel/i)
    fireEvent.click(peek)
    expect((mocks.session as ReturnType<typeof stubSession>).openFloatingModal).toHaveBeenCalledWith(MISSION_SESSION)
  })

  it('does not offer peek before a running node has dispatched a session', async () => {
    mocks.session = stubSession([MISSION_SESSION])
    installFetchMock([{ runId: 'run1', seq: 1, type: 'node_started', nodeId: 'fmn_work', attempt: 1 }], { status: 'running', final: false })
    const dialog = await openInspector()
    expect(within(dialog).getByTestId('node-evidence-state')).toHaveTextContent('running')
    expect(within(dialog).queryByTestId('peek-node-fmn_work')).toBeNull()
  })

  it('does not offer peek for a finished node', async () => {
    mocks.session = stubSession([MISSION_SESSION])
    installFetchMock([...runningEvents, { runId: 'run1', seq: 3, type: 'node_output', nodeId: 'fmn_work', data: { status: 'done', text: 'done' } }], { status: 'succeeded', final: true })
    const dialog = await openInspector()
    expect(within(dialog).queryByTestId('peek-node-fmn_work')).toBeNull()
  })

  it('disables peek when the running node has no live session to attach to', async () => {
    mocks.session = stubSession([])
    installFetchMock(runningEvents, { status: 'running', final: false })
    const dialog = await openInspector()
    const peek = within(dialog).getByTestId('peek-node-fmn_work')
    expect(peek).toBeDisabled()
    fireEvent.click(peek)
    expect((mocks.session as ReturnType<typeof stubSession>).openFloatingModal).not.toHaveBeenCalled()
  })
})
