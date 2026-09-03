/**
 * What the browser knows about Beads, and how it asks.
 *
 * Every answer comes from `bd` through the server: there is no second store
 * here, nothing is written from the dashboard, and nothing is cached beyond the
 * request that asked for it.
 */

export interface BeadProject {
  name: string
  path: string
  beadsPath: string
  source?: string
  /** The prefix this project's ids carry, which is what terminal output shows. */
  prefix?: string
}

/** A Bead as a link from somewhere else: a row, a parent, a blocker. */
export interface BeadLink {
  id: string
  title: string
  status: string
  type?: string
  priority: number
}

/** A Bead as the map, the ready lists and the stale list draw it. */
export interface BeadRow extends BeadLink {
  updated?: string
  parent?: string
  blockedBy?: string[]
  blocked: boolean
  /** Only epics carry it: an epic row expanded is its definition of done. */
  acceptance?: string
}

/** A Bead as the card reads it, with every neighbour it links to. */
export interface BeadDetail extends BeadLink {
  updated?: string
  created?: string
  assignee?: string
  description?: string
  design?: string
  acceptance?: string
  notes?: string
  parents: BeadLink[]
  children: BeadLink[]
  blockedBy: BeadLink[]
  blocks: BeadLink[]
}

export interface BeadWork {
  beads: BeadRow[]
  prefix: string
  projectPath: string
}

interface ApiEnvelope<T> {
  success?: boolean
  data?: T
  error?: { code?: string; message?: string }
}

const API_BASE = '/api/beads'

async function get<T>(path: string, params: Record<string, string | string[]>): Promise<T> {
  const url = new URL(`${API_BASE}${path}`, window.location.origin)
  Object.entries(params).forEach(([key, value]) => {
    if (Array.isArray(value)) value.forEach(item => url.searchParams.append(key, item))
    else url.searchParams.set(key, value)
  })
  const response = await fetch(url.toString(), { signal: AbortSignal.timeout(30000) })
  const envelope = await response.json().catch(() => null) as ApiEnvelope<T> | null
  if (!envelope || envelope.success !== true || envelope.data === undefined) {
    throw new Error(envelope?.error?.message || `Beads request failed (${response.status})`)
  }
  return envelope.data
}

export async function fetchBeadProjects(manualPaths: readonly string[] = []): Promise<BeadProject[]> {
  const data = await get<{ projects: BeadProject[] }>('/projects', manualPaths.length > 0 ? { path: [...manualPaths] } : {})
  return data.projects ?? []
}

/** The open work of one project, with the finished children of its open epics. */
export async function fetchBeadWork(projectPath: string): Promise<BeadWork> {
  return get<BeadWork>('/work', { path: projectPath })
}

export async function fetchBead(projectPath: string, id: string): Promise<BeadDetail> {
  const data = await get<{ bead: BeadDetail }>('/issue', { path: projectPath, id })
  return data.bead
}
