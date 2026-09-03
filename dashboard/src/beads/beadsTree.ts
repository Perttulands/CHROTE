/**
 * Open work, arranged.
 *
 * The three views of the Beads tab are three readings of the same rows: the map
 * hangs them under the epics they belong to, the ready lists split them by
 * whether they can be started, and the stale list orders them by how long they
 * have been untouched. The arranging is here, away from the drawing, because it
 * is the part with rules worth testing.
 */

import type { BeadRow } from './beadsApi'
import { daysSince, isBeadClosed } from './beadStatus'

/** A row and the store it came from: "All" draws several projects at once. */
export interface WorkRow extends BeadRow {
  projectPath: string
  projectName: string
}

export interface BeadTreeNode {
  row: WorkRow
  children: BeadTreeNode[]
}

/**
 * A row's identity across stores. Two projects can spell the same id, and "All"
 * draws them side by side, so nothing here keys on the id alone.
 */
export function beadRowKey(row: { projectPath: string; id: string }): string {
  return `${row.projectPath}\u0000${row.id}`
}

function byPriorityThenId(a: WorkRow, b: WorkRow): number {
  if (a.priority !== b.priority) return a.priority - b.priority
  return a.id.localeCompare(b.id)
}

function byUpdatedNewestFirst(a: WorkRow, b: WorkRow): number {
  return (b.updated ?? '').localeCompare(a.updated ?? '')
}

/**
 * The map: every open epic as a root with its children beneath it, then the
 * open work that hangs under no open epic, so that nothing open is invisible.
 */
export function buildBeadMap(rows: readonly WorkRow[]): BeadTreeNode[] {
  const known = new Map(rows.map(row => [beadRowKey(row), row]))
  const childrenOf = new Map<string, WorkRow[]>()
  rows.forEach(row => {
    if (!row.parent) return
    const parentKey = beadRowKey({ projectPath: row.projectPath, id: row.parent })
    if (!known.has(parentKey)) return
    const siblings = childrenOf.get(parentKey) ?? []
    siblings.push(row)
    childrenOf.set(parentKey, siblings)
  })

  const seen = new Set<string>()
  const nodeFor = (row: WorkRow): BeadTreeNode => {
    seen.add(beadRowKey(row))
    const children = (childrenOf.get(beadRowKey(row)) ?? [])
      .filter(child => !seen.has(beadRowKey(child)))
      .sort(byPriorityThenId)
      .map(nodeFor)
    return { row, children }
  }

  const roots: BeadTreeNode[] = []
  rows
    .filter(row => row.type === 'epic' && !isBeadClosed(row.status))
    .sort(byPriorityThenId)
    .forEach(epic => { if (!seen.has(beadRowKey(epic))) roots.push(nodeFor(epic)) })
  rows
    .filter(row => !isBeadClosed(row.status))
    .sort(byPriorityThenId)
    .forEach(row => { if (!seen.has(beadRowKey(row))) roots.push(nodeFor(row)) })
  return roots
}

function matchesQuery(row: WorkRow, query: string): boolean {
  const needle = query.trim().toLowerCase()
  if (needle === '') return true
  return row.id.toLowerCase().includes(needle) || row.title.toLowerCase().includes(needle)
}

/** Keep what matches, and the branches that lead to it. */
export function filterBeadTree(nodes: readonly BeadTreeNode[], query: string): BeadTreeNode[] {
  if (query.trim() === '') return [...nodes]
  return nodes.reduce<BeadTreeNode[]>((kept, node) => {
    const children = filterBeadTree(node.children, query)
    if (children.length > 0 || matchesQuery(node.row, query)) kept.push({ row: node.row, children })
    return kept
  }, [])
}

export function filterBeadRows(rows: readonly WorkRow[], query: string): WorkRow[] {
  return rows.filter(row => matchesQuery(row, query))
}

/** Ready is open work with nothing in its way; in progress is what is claimed. */
export function readyRows(rows: readonly WorkRow[]): WorkRow[] {
  return rows
    .filter(row => !isBeadClosed(row.status) && row.status !== 'in_progress' && !row.blocked)
    .sort(byUpdatedNewestFirst)
}

export function inProgressRows(rows: readonly WorkRow[]): WorkRow[] {
  return rows.filter(row => row.status === 'in_progress').sort(byUpdatedNewestFirst)
}

/** Open work nobody has touched in N days, the most neglected first. */
export function staleRows(rows: readonly WorkRow[], days: number, now: number = Date.now()): WorkRow[] {
  return rows
    .filter(row => !isBeadClosed(row.status) && daysSince(row.updated, now) >= days)
    .sort((a, b) => daysSince(b.updated, now) - daysSince(a.updated, now))
}
