/**
 * What the browser knows about Beads, and how it asks.
 *
 * Every answer comes from `bd` through the server: there is no second store
 * here, nothing is written from the dashboard, and nothing is cached beyond the
 * request that asked for it.
 */

import { fetchWorkspaces, holdsStore, workspaceName, type BeadsCounts, type Workspace } from '../workspaces/workspacesApi'

export interface BeadProject {
  name: string
  path: string
  beadsPath: string
  /** configured, discovered or manual: how the store came to be listed. */
  source?: string
  /** The prefix this project's ids carry, which is what terminal output shows. */
  prefix?: string
  /** How many Beads are not closed. Absent when the server could not count. */
  openBeads?: number
  counts?: BeadsCounts
  newestUpdate?: string
  error?: string
  summaryPending?: boolean
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
  deferUntil?: string
  parent?: string
  blockedBy?: string[]
  blocked: boolean
  /** True when bd reports any parent, child, blocking, or blocked-by relation. */
  linked: boolean
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

/** The formula registry and molecule commands intentionally return their full
 * bd objects. These small named fields let the rail identify entries while the
 * detail view keeps every additional field available to the operator. */
export type BeadsStructure = Record<string, unknown>

export interface FormulaSummary extends BeadsStructure {
  name?: string
  formula?: string
  description?: string
  source?: string
}

export interface MoleculeSummary extends BeadsStructure {
  id?: string
  title?: string
  status?: string
  is_template?: boolean
}

export interface FormulaCatalog {
  formulas: FormulaSummary[]
  projectPath: string
}

export interface MoleculeCatalog {
  molecules: MoleculeSummary[]
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

function workspaceProjects(workspaces: readonly Workspace[]): BeadProject[] {
  return workspaces.filter(holdsStore).map(workspace => ({
    name: workspaceName(workspace.path),
    path: workspace.path,
    beadsPath: `${workspace.path}/.beads`,
    source: workspace.sources.includes('beads') ? 'configured' : 'discovered',
    ...(workspace.beadsPrefix ? { prefix: workspace.beadsPrefix } : {}),
    ...(workspace.openBeads !== undefined ? { openBeads: workspace.openBeads } : {}),
    ...(workspace.beadsCounts ? { counts: workspace.beadsCounts } : {}),
    ...(workspace.beadsNewestUpdate ? { newestUpdate: workspace.beadsNewestUpdate } : {}),
    ...(workspace.beadsError ? { error: workspace.beadsError } : {}),
    ...(workspace.beadsSummaryPending ? { summaryPending: true } : {}),
  }))
}

/** The stat-only store list used for the Beads tab's first paint. */
export async function fetchBeadProjectList(): Promise<BeadProject[]> {
  return workspaceProjects(await fetchWorkspaces({ beads: true }))
}

/**
 * Every store with its manifest-keyed projection: the workspace list after its
 * background fills, then manual paths the operator saved in Settings.
 */
export async function fetchBeadProjects(manualPaths: readonly string[] = []): Promise<BeadProject[]> {
  const [workspaces, manual] = await Promise.all([
    fetchWorkspaces({ beads: true, waitForBeads: true }),
    manualPaths.length > 0
      ? get<{ projects: BeadProject[] }>('/projects', { path: [...manualPaths] }).then(data => data.projects ?? [])
      : Promise.resolve([] as BeadProject[]),
  ])
  const projects = workspaceProjects(workspaces)
  const known = new Set(projects.map(project => project.path))
  for (const project of manual) {
    if (!known.has(project.path)) projects.push(project)
  }
  return projects
}

/** The open work of one project, with the finished children of its open epics. */
export async function fetchBeadWork(projectPath: string): Promise<BeadWork> {
  return get<BeadWork>('/work', { path: projectPath })
}

/** Finished work is deliberately a separate, lazy request. */
export async function fetchClosedBeadWork(projectPath: string): Promise<BeadWork> {
  return get<BeadWork>('/closed', { path: projectPath })
}

export async function fetchFormulas(projectPath: string): Promise<FormulaCatalog> {
  return get<FormulaCatalog>('/formulas', { path: projectPath })
}

export async function fetchFormula(projectPath: string, name: string): Promise<BeadsStructure> {
  const data = await get<{ formula: BeadsStructure }>('/formula', { path: projectPath, name })
  return data.formula
}

export async function fetchMolecules(projectPath: string): Promise<MoleculeCatalog> {
  return get<MoleculeCatalog>('/molecules', { path: projectPath })
}

export async function fetchMolecule(projectPath: string, id: string): Promise<BeadsStructure> {
  const data = await get<{ molecule: BeadsStructure }>('/molecule', { path: projectPath, id })
  return data.molecule
}

export async function fetchBead(projectPath: string, id: string): Promise<BeadDetail> {
  const data = await get<{ bead: BeadDetail }>('/issue', { path: projectPath, id })
  return data.bead
}
