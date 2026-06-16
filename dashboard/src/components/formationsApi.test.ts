import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  ApiRequestError,
  createBoard,
  fetchApi,
  fetchBoardChanged,
  fetchBoardDocument,
  normalizeBoard,
  normalizeLayout,
  patchBoardDocument,
  startRun,
  updateMissionOp,
} from './formationsApi'
import { undoBoardPatch } from './formationsBoardModel'
import type { BoardDocument, LayoutDocument } from './formationsTypes'

function jsonResponse(body: unknown, options: { ok?: boolean; status?: number; etag?: string } = {}): Response {
  return {
    ok: options.ok ?? true,
    status: options.status ?? 200,
    headers: {
      get: (name: string) => name.toLowerCase() === 'etag' ? options.etag ?? null : null,
    },
    json: () => Promise.resolve(body),
  } as Response
}

describe('formations API helpers', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('returns response data and the API ETag', async () => {
    vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      expect(String(input)).toBe('/api/formations/boards')
      expect(init?.headers).toMatchObject({ 'Content-Type': 'application/json' })
      return Promise.resolve(jsonResponse({
        success: true,
        data: { boards: [{ slug: 'session-search' }] },
      }, { etag: 'board-list-etag' }))
    }) as unknown as typeof fetch)

    const result = await fetchApi<{ boards: Array<{ slug: string }> }>('/api/formations/boards')

    expect(result).toEqual({
      data: { boards: [{ slug: 'session-search' }] },
      etag: 'board-list-etag',
    })
  })

  it('throws an ApiRequestError for explicit API failures', async () => {
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve(jsonResponse({
      success: false,
      error: { code: 'NOT_FOUND', message: 'board missing' },
    }, { ok: false, status: 404 }))) as unknown as typeof fetch)

    await expect(fetchApi('/api/formations/boards/missing')).rejects.toMatchObject({
      status: 404,
      code: 'NOT_FOUND',
      message: 'board missing',
    } satisfies Partial<ApiRequestError>)
  })

  it('fails loudly when a successful envelope omits data', async () => {
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve(jsonResponse({
      success: true,
    }, { status: 200 }))) as unknown as typeof fetch)

    await expect(fetchApi('/api/formations/boards')).rejects.toBeInstanceOf(ApiRequestError)
  })

  it('normalizes optional board and layout arrays at the API boundary', () => {
    const board = normalizeBoard({
      id: 'brd_1',
      slug: 'session-search',
      title: 'Session search',
      rev: 1,
      etag: 'board-etag',
      formations: [],
      connections: [],
    } as BoardDocument, 'response-etag')
    const layout = normalizeLayout({
      boardId: 'brd_1',
      boardRev: 1,
      etag: 'layout-etag',
      nodes: [],
    } as LayoutDocument, 'layout-response-etag')

    expect(board).toMatchObject({
      etag: 'response-etag',
      missions: [],
      gates: [],
      formations: [],
      connections: [],
    })
    expect(layout).toMatchObject({
      etag: 'layout-response-etag',
      nodes: [],
      edges: [],
    })
  })

  it('fetches and normalizes a board document', async () => {
    vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL) => {
      expect(String(input)).toBe('/api/formations/boards/session-search')
      return Promise.resolve(jsonResponse({
        success: true,
        data: {
          board: {
            id: 'brd_1',
            slug: 'session-search',
            title: 'Session search',
            rev: 1,
            etag: 'board-etag',
            formations: [],
            connections: [],
          },
        },
      }, { etag: 'response-etag' }))
    }) as unknown as typeof fetch)

    await expect(fetchBoardDocument('session-search')).resolves.toMatchObject({
      etag: 'response-etag',
      missions: [],
      gates: [],
      formations: [],
      connections: [],
    })
  })

  it('checks board changes against the current revision', async () => {
    vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL) => {
      expect(String(input)).toBe('/api/formations/boards/session-search/changes?rev=7')
      return Promise.resolve(jsonResponse({
        success: true,
        data: { signal: { changed: true } },
      }))
    }) as unknown as typeof fetch)

    await expect(fetchBoardChanged('session-search', 7)).resolves.toBe(true)
  })

  it('patches a board with explicit optimistic concurrency inputs', async () => {
    const calls: Array<{ url: string; init?: RequestInit }> = []
    vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      calls.push({ url: String(input), init })
      return Promise.resolve(jsonResponse({
        success: true,
        data: {
          board: {
            id: 'brd_1',
            slug: 'session-search',
            title: 'Session search',
            rev: 2,
            etag: 'board-etag-2',
            formations: [],
            connections: [],
          },
          layout: {
            boardId: 'brd_1',
            boardRev: 2,
            etag: 'layout-etag-2',
            nodes: [],
          },
        },
      }, { etag: 'board-response-etag' }))
    }) as unknown as typeof fetch)

    const result = await patchBoardDocument('session-search', 'board-etag', 1, { renameFormation: { id: 'fmn_1', title: 'Next' } })

    expect(calls[0].url).toBe('/api/formations/boards/session-search')
    expect(calls[0].init?.method).toBe('PATCH')
    expect(calls[0].init?.headers).toMatchObject({ 'If-Match': 'board-etag' })
    expect(JSON.parse(String(calls[0].init?.body))).toMatchObject({
      expectedRev: 1,
      updatedBy: 'agent:ui',
      renameFormation: { id: 'fmn_1', title: 'Next' },
    })
    expect(result.board.etag).toBe('board-response-etag')
    expect(result.layout?.edges).toEqual([])
  })

  it('builds an updateMission patch op matching the backend full-replace contract', () => {
    // The backend op is { updateMission: { missionId, title, goal, beadId } } with full-replace
    // semantics; beadId is optional and an empty string clears it.
    expect(updateMissionOp({ missionId: 'mis_1', title: 'Ship it', goal: 'Land the page', beadId: 'home-7kc4.5' })).toEqual({
      updateMission: { missionId: 'mis_1', title: 'Ship it', goal: 'Land the page', beadId: 'home-7kc4.5' },
    })
    expect(updateMissionOp({ missionId: 'mis_1', title: 'Ship it', goal: '', beadId: '' })).toEqual({
      updateMission: { missionId: 'mis_1', title: 'Ship it', goal: '', beadId: '' },
    })
  })

  it('creates a Mission Board with the {slug,title,mission,updatedBy} contract', async () => {
    const calls: Array<{ url: string; init?: RequestInit }> = []
    vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      calls.push({ url: String(input), init })
      return Promise.resolve(jsonResponse({
        success: true,
        data: {
          board: {
            id: 'brd_new',
            slug: 'poems',
            title: 'Poems',
            rev: 1,
            etag: 'board-etag',
            missions: [{ id: 'mis_1', title: 'Write a poem', goal: 'Ship a poem', beadId: '' }],
            formations: [],
            connections: [],
          },
        },
      }, { etag: 'create-etag' }))
    }) as unknown as typeof fetch)

    const board = await createBoard({
      slug: 'poems',
      title: 'Poems',
      goal: 'Ship a poem',
      beadId: '',
    })

    expect(calls[0].url).toBe('/api/formations/boards')
    expect(calls[0].init?.method).toBe('POST')
    const body = JSON.parse(String(calls[0].init?.body)) as Record<string, unknown>
    // mission.goal is the meaningful field; title defaults backend-side, beadId optional.
    expect(body).toMatchObject({
      slug: 'poems',
      title: 'Poems',
      mission: { goal: 'Ship a poem' },
      updatedBy: 'agent:ui',
    })
    // An empty bead must NOT be sent (the backend 400s on a malformed bead, "" is just "no anchor").
    expect((body.mission as Record<string, unknown>).beadId).toBeUndefined()
    expect(board.etag).toBe('create-etag')
    expect(board.missions?.[0].goal).toBe('Ship a poem')
  })

  it('forwards an optional bead anchor on create', async () => {
    let captured: Record<string, unknown> = {}
    vi.stubGlobal('fetch', vi.fn((_input: RequestInfo | URL, init?: RequestInit) => {
      captured = JSON.parse(String(init?.body)) as Record<string, unknown>
      return Promise.resolve(jsonResponse({
        success: true,
        data: { board: { id: 'b', slug: 's', title: 'T', rev: 1, etag: 'e', formations: [], connections: [] } },
      }, { etag: 'e' }))
    }) as unknown as typeof fetch)

    await createBoard({ slug: 's', title: 'T', goal: 'g', beadId: 'home-7kc4.5' })
    expect((captured.mission as Record<string, unknown>).beadId).toBe('home-7kc4.5')
  })

  it('surfaces the 409 duplicate-slug error precisely', async () => {
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve(jsonResponse({
      success: false,
      error: { code: 'CONFLICT', message: 'a board with slug "poems" already exists' },
    }, { ok: false, status: 409 }))) as unknown as typeof fetch)

    await expect(createBoard({ slug: 'poems', title: 'Poems', goal: 'x', beadId: '' })).rejects.toMatchObject({
      status: 409,
      code: 'CONFLICT',
      message: 'a board with slug "poems" already exists',
    } satisfies Partial<ApiRequestError>)
  })

  it('starts runs with the board ETag precondition', async () => {
    const calls: Array<{ url: string; init?: RequestInit }> = []
    vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      calls.push({ url: String(input), init })
      return Promise.resolve(jsonResponse({
        success: true,
        data: {
          runId: 'run_1',
          status: {
            runId: 'run_1',
            status: 'running',
            final: false,
            boardSlug: 'session-search',
            missionId: 'mis_showcase',
            eventCount: 1,
          },
        },
      }))
    }) as unknown as typeof fetch)

    await startRun('board-etag', { board: 'session-search', missionId: 'mis_showcase', actor: 'agent:ui' })

    expect(calls[0].url).toBe('/api/formations/runs')
    expect(calls[0].init?.method).toBe('POST')
    expect(calls[0].init?.headers).toMatchObject({ 'If-Match': 'board-etag' })
  })
})

// F2 cross-language contract lock (UI-side half).
//
// WHY THIS EXISTS: the dashboard's e2e specs mock the API and only assert
// request SHAPE, never backend ACCEPTANCE. That let two contract bugs ship: the
// UI sent addPort direction:'in' (the Go backend only accepts 'input'/'output')
// and mission-create assumed a required bead the backend treats as optional.
//
// These tests pin the EXACT field names and values the UI emits for every
// previously-broken write path. The matching backend-side half lives in
// src/internal/api/formations_roundtrip_test.go, which sends these same bodies
// to the real handler and re-reads the persisted board. Together they fail a
// test on contract drift from EITHER side: change the UI to 'in'/'out' and this
// file goes red; loosen the backend to accept 'in'/'out' and the Go round-trip
// goes red.
//
// addPort, createMission and wireConnection are built inline inside
// FormationsCockpit.tsx (coupled to React refs/undo state), so they cannot be
// imported as pure functions. We lock those by asserting the literal wire body
// that the real transport (patchBoardDocument) puts on the request — the exact
// bytes that reach the Go backend. updateMissionOp and the undo inverse builder
// undoBoardPatch are exported pure functions and are asserted directly.
describe('formations op-builder contract lock', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  function capturePatchBody(op: Record<string, unknown>): Promise<Record<string, unknown>> {
    let captured: Record<string, unknown> = {}
    vi.stubGlobal('fetch', vi.fn((_input: RequestInfo | URL, init?: RequestInit) => {
      captured = JSON.parse(String(init?.body)) as Record<string, unknown>
      return Promise.resolve(jsonResponse({
        success: true,
        data: {
          board: { id: 'brd_1', slug: 'session-search', title: 'T', rev: 2, etag: 'e2', formations: [], connections: [] },
        },
      }, { etag: 'e2' }))
    }) as unknown as typeof fetch)
    return patchBoardDocument('session-search', 'board-etag', 1, op).then(() => captured)
  }

  it('sends addPort with direction "input"/"output", never "in"/"out"', async () => {
    // Exact body FormationsCockpit.addPortOp builds for an input port.
    const input = await capturePatchBody({ addPort: { formationId: 'fmn_frame', direction: 'input', label: 'Input' } })
    expect(input.addPort).toEqual({ formationId: 'fmn_frame', direction: 'input', label: 'Input' })
    // The previously-shipped bug used 'in'; this asserts we never emit it.
    expect((input.addPort as { direction: string }).direction).toBe('input')
    expect((input.addPort as { direction: string }).direction).not.toBe('in')

    const output = await capturePatchBody({ addPort: { formationId: 'fmn_frame', direction: 'output', label: 'Output' } })
    expect((output.addPort as { direction: string }).direction).toBe('output')
    expect((output.addPort as { direction: string }).direction).not.toBe('out')
  })

  it('sends createMission with NO beadId field (backend treats bead as optional)', async () => {
    // Exact body FormationsCockpit.createMissionAt builds; the old bug assumed a
    // required bead, which is why this asserts beadId is absent, not just empty.
    const body = await capturePatchBody({ createMission: { title: 'New mission', goal: '', x: 120, y: 80 } })
    expect(body.createMission).toEqual({ title: 'New mission', goal: '', x: 120, y: 80 })
    expect(Object.keys(body.createMission as object)).not.toContain('beadId')
  })

  it('sends wireConnection/unwireConnection as { from, to } endpoints', async () => {
    const wire = await capturePatchBody({ wireConnection: { from: 'fmn_frame:port_out', to: 'fmn_ship:port_in' } })
    expect(wire.wireConnection).toEqual({ from: 'fmn_frame:port_out', to: 'fmn_ship:port_in' })

    const unwire = await capturePatchBody({ unwireConnection: { from: 'fmn_frame:port_out', to: 'fmn_ship:port_in' } })
    expect(unwire.unwireConnection).toEqual({ from: 'fmn_frame:port_out', to: 'fmn_ship:port_in' })
  })

  it('builds updateMission with exactly { missionId, title, goal, beadId }', () => {
    // Full-replace contract: an empty beadId clears the link (backend 400s on a
    // malformed one). These are the exact field names store.UpdateMission reads.
    expect(updateMissionOp({ missionId: 'mis_1', title: 'Ship', goal: 'Land', beadId: 'home-7kc4.5' })).toEqual({
      updateMission: { missionId: 'mis_1', title: 'Ship', goal: 'Land', beadId: 'home-7kc4.5' },
    })
    const cleared = updateMissionOp({ missionId: 'mis_1', title: 'Ship', goal: '', beadId: '' })
    expect(Object.keys(cleared.updateMission).sort()).toEqual(['beadId', 'goal', 'missionId', 'title'])
    expect(cleared.updateMission.beadId).toBe('')
  })

  it('builds the removePort and unwireConnection undo inverses with backend field names', () => {
    // undoBoardPatch is the source of the UI's inverse PATCH ops (ADR-0003: undo
    // is React-orchestrated inverse PATCHes, not server-side history). These must
    // match exactly what store.RemovePort / UnwireConnection accept.
    expect(undoBoardPatch({ kind: 'removePort', formationId: 'fmn_frame', portId: 'port_in' })).toEqual({
      removePort: { formationId: 'fmn_frame', portId: 'port_in' },
    })
    expect(undoBoardPatch({ kind: 'unwireConnection', from: 'fmn_frame:port_out', to: 'fmn_ship:port_in' })).toEqual({
      unwireConnection: { from: 'fmn_frame:port_out', to: 'fmn_ship:port_in' },
    })
  })
})
