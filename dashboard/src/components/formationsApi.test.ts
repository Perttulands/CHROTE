import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  ApiRequestError,
  fetchApi,
  fetchBoardChanged,
  fetchBoardDocument,
  normalizeBoard,
  normalizeLayout,
  patchBoardDocument,
  startRun,
} from './formationsApi'
import type { BoardDocument, LayoutDocument, ToolNode } from './formationsTypes'

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
      tools: [],
      formations: [],
      connections: [],
    })
    expect(layout).toMatchObject({
      etag: 'layout-response-etag',
      nodes: [],
      edges: [],
    })
  })

  it('preserves the exact Tool projection at the API boundary', () => {
    const tool: ToolNode = {
      id: 'tool_normalize',
      title: 'Normalize report',
      profileId: 'json.normalize',
      profileVersion: '1',
      params: { mode: 'strict' },
      inputs: [{
        id: 'port_tool_in',
        name: 'input',
        label: 'Report',
        direction: 'input',
        kind: 'work',
        acceptedMediaTypes: ['application/json'],
        required: true,
        role: 'data',
      }],
      outputs: [{
        id: 'port_tool_out',
        name: 'output',
        label: 'Normalized report',
        direction: 'output',
        kind: 'work',
        acceptedMediaTypes: ['application/json'],
      }],
    }
    const board = normalizeBoard({
      id: 'brd_tool',
      slug: 'tool-board',
      title: 'Tool board',
      rev: 2,
      etag: 'tool-etag',
      formations: [],
      connections: [],
      tools: [tool],
    })

    expect(board.tools).toEqual([tool])
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
      tools: [],
      formations: [],
      connections: [],
    })
  })

  it('checks board changes against the current ETag', async () => {
    vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL) => {
      expect(String(input)).toBe('/api/formations/boards/session-search/changes?etag=board-etag')
      return Promise.resolve(jsonResponse({
        success: true,
        data: { signal: { changed: true } },
      }))
    }) as unknown as typeof fetch)

    await expect(fetchBoardChanged('session-search', 'board-etag')).resolves.toBe(true)
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
