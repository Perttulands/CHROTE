import type {
  AgentProjection,
  BoardDocument,
  BoardSummary,
  FormationNode,
  LayoutDocument,
  LayoutEdge,
  LayoutNode,
  OpenEscalation,
  RunEvent,
  RunStartResult,
  RunStatusProjection,
  RunStatusResult,
} from './formationsTypes'

interface ApiResponse<T> {
  success: boolean
  data?: T
  error?: { code: string; message: string }
}

export class ApiRequestError extends Error {
  status: number
  code: string

  constructor(message: string, status: number, code: string) {
    super(message)
    this.status = status
    this.code = code
  }
}

export async function fetchApi<T>(endpoint: string, init?: RequestInit): Promise<{ data: T; etag: string }> {
  const response = await fetch(endpoint, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...(init?.headers || {}),
    },
    signal: AbortSignal.timeout(10000),
  })
  const result = await response.json() as ApiResponse<T>
  if (!response.ok || !result.success || result.data === undefined || result.data === null) {
    throw new ApiRequestError(result.error?.message || `Request failed: ${response.status}`, response.status, result.error?.code || '')
  }
  return { data: result.data, etag: response.headers.get('ETag') || '' }
}

export function normalizeBoard(board: BoardDocument, etag = ''): BoardDocument {
  return {
    ...board,
    etag: etag || board.etag,
    missions: board.missions || [],
    formations: board.formations || [],
    gates: board.gates || [],
    tools: board.tools || [],
    connections: board.connections || [],
  }
}

export function normalizeLayout(layout: LayoutDocument, etag = ''): LayoutDocument {
  return {
    ...layout,
    etag: etag || layout.etag,
    nodes: layout.nodes || [],
    edges: layout.edges || [],
  }
}

export function missingLayoutForBoard(board: BoardDocument): LayoutDocument {
  return {
    boardId: board.id,
    boardRev: board.rev,
    etag: '*',
    nodes: [],
    edges: [],
  }
}

export async function fetchBoardSummaries(): Promise<BoardSummary[]> {
  const result = await fetchApi<{ boards: BoardSummary[] }>('/api/formations/boards')
  return result.data.boards || []
}

export async function fetchBoardDocument(slug: string): Promise<BoardDocument> {
  const result = await fetchApi<{ board: BoardDocument }>(`/api/formations/boards/${encodeURIComponent(slug)}`)
  return normalizeBoard(result.data.board, result.etag)
}

export async function fetchBoardLayout(slug: string): Promise<LayoutDocument> {
  const result = await fetchApi<{ layout: LayoutDocument }>(`/api/formations/boards/${encodeURIComponent(slug)}/layout`)
  return normalizeLayout(result.data.layout, result.etag)
}

export async function createBoard(title: string, template: string): Promise<BoardDocument> {
  const result = await fetchApi<{ board: BoardDocument }>('/api/formations/boards', {
    method: 'POST',
    body: JSON.stringify({ title, template }),
  })
  return normalizeBoard(result.data.board, result.etag)
}

export async function fetchAgents(): Promise<AgentProjection[]> {
  const result = await fetchApi<{ agents: AgentProjection[] }>('/api/agents')
  return result.data.agents || []
}

export async function fetchBoardChanged(slug: string, etag: string): Promise<boolean> {
  const result = await fetchApi<{ signal: { changed?: boolean } }>(
    `/api/formations/boards/${encodeURIComponent(slug)}/changes?etag=${encodeURIComponent(etag)}`
  )
  return result.data.signal?.changed === true
}

type PatchBoardResponse<TExtra extends object> = {
  board: BoardDocument
  layout?: LayoutDocument
} & TExtra

export async function patchBoardDocument<TExtra extends object = Record<string, never>>(
  slug: string,
  etag: string,
  rev: number,
  patch: Record<string, unknown>,
): Promise<PatchBoardResponse<TExtra>> {
  const result = await fetchApi<PatchBoardResponse<TExtra>>(
    `/api/formations/boards/${encodeURIComponent(slug)}`,
    {
      method: 'PATCH',
      headers: { 'If-Match': etag },
      body: JSON.stringify({
        expectedRev: rev,
        updatedBy: 'agent:ui',
        ...patch,
      }),
    }
  )
  return {
    ...result.data,
    board: normalizeBoard(result.data.board, result.etag),
    layout: result.data.layout ? normalizeLayout(result.data.layout) : undefined,
  }
}

export async function patchBoardLayout(slug: string, etag: string, patch: { nodes?: LayoutNode[]; edges?: LayoutEdge[]; arrange?: boolean }): Promise<LayoutDocument> {
  const result = await fetchApi<{ layout: LayoutDocument }>(
    `/api/formations/boards/${encodeURIComponent(slug)}/layout`,
    {
      method: 'PATCH',
      headers: { 'If-Match': etag },
      body: JSON.stringify(patch),
    }
  )
  return normalizeLayout(result.data.layout, result.etag)
}

export async function startRun(etag: string, body: { board: string; missionId?: string; formationId?: string; actor: string }): Promise<RunStartResult> {
  const result = await fetchApi<RunStartResult>('/api/formations/runs', {
    method: 'POST',
    headers: { 'If-Match': etag },
    body: JSON.stringify(body),
  })
  return result.data
}

export async function fetchRunStatus(runId: string): Promise<RunStatusProjection | RunStatusResult> {
  const result = await fetchApi<RunStatusProjection | RunStatusResult>(`/api/formations/runs/${encodeURIComponent(runId)}`)
  return result.data
}

export async function fetchRunEvents(runId: string): Promise<RunEvent[]> {
  const result = await fetchApi<{ events: RunEvent[] }>(`/api/formations/runs/${encodeURIComponent(runId)}/events`)
  return (result.data.events || []).sort((a, b) => a.seq - b.seq)
}

export async function fetchRunEscalations(runId: string): Promise<OpenEscalation[]> {
  const result = await fetchApi<{ escalations: OpenEscalation[] }>(`/api/formations/runs/${encodeURIComponent(runId)}/escalations`)
  return (result.data.escalations || []).sort((a, b) => a.seq - b.seq)
}

export async function abortRunRequest(runId: string, body: { reason: string; requestedBy: string }): Promise<RunStatusProjection | RunStatusResult> {
  const result = await fetchApi<RunStatusProjection | RunStatusResult>(
    `/api/formations/runs/${encodeURIComponent(runId)}/abort`,
    {
      method: 'POST',
      body: JSON.stringify(body),
    }
  )
  return result.data
}

export async function resumeRunRequest(runId: string, body: { actor: string; mode: string; reason: string }): Promise<RunStatusProjection | RunStatusResult> {
  const result = await fetchApi<RunStatusProjection | RunStatusResult>(
    `/api/formations/runs/${encodeURIComponent(runId)}/resume`,
    {
      method: 'POST',
      body: JSON.stringify(body),
    }
  )
  return result.data
}

export async function recordGateVerdict(runId: string, gateId: string, body: { actor: string; verdict: 'pass' | 'fail'; reason: string }): Promise<RunStatusProjection | RunStatusResult> {
  const result = await fetchApi<RunStatusProjection | RunStatusResult>(
    `/api/formations/runs/${encodeURIComponent(runId)}/gates/${encodeURIComponent(gateId)}/verdict`,
    {
      method: 'POST',
      body: JSON.stringify(body),
    }
  )
  return result.data
}

export type PatchCreateFormationResult = {
  layout: LayoutDocument
  formation: FormationNode
}
