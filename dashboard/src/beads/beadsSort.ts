import type { BeadTreeNode, WorkRow } from './beadsTree'

export const BEAD_SORTS = [
  { id: 'default', label: 'View default' },
  { id: 'updated-newest', label: 'Updated · newest first' },
  { id: 'updated-oldest', label: 'Updated · oldest first' },
  { id: 'priority', label: 'Priority · highest first' },
  { id: 'title', label: 'Title · A–Z' },
  { id: 'id', label: 'ID' },
  { id: 'type', label: 'Type' },
] as const

export type BeadSort = typeof BEAD_SORTS[number]['id']

function compareRows(a: WorkRow, b: WorkRow, sort: BeadSort): number {
  let order = 0
  if (sort === 'updated-newest' || sort === 'updated-oldest') {
    const left = Date.parse(a.updated ?? '')
    const right = Date.parse(b.updated ?? '')
    // Unknown dates belong last in either direction. Compare instants, since
    // bd timestamps can use different UTC offsets.
    if (!Number.isFinite(left)) return Number.isFinite(right) ? 1 : tie(a, b)
    if (!Number.isFinite(right)) return -1
    order = sort === 'updated-newest' ? right - left : left - right
  } else if (sort === 'priority') {
    order = a.priority - b.priority
  } else if (sort === 'title') {
    order = a.title.localeCompare(b.title, undefined, { sensitivity: 'base', numeric: true })
  } else if (sort === 'type') {
    order = (a.type ?? '').localeCompare(b.type ?? '')
  }
  return order || tie(a, b)
}

function tie(a: WorkRow, b: WorkRow): number {
  return a.id.localeCompare(b.id, undefined, { numeric: true }) || a.projectPath.localeCompare(b.projectPath)
}

export function sortBeadRows(rows: readonly WorkRow[], sort: BeadSort): WorkRow[] {
  return sort === 'default' ? [...rows] : [...rows].sort((a, b) => compareRows(a, b, sort))
}

/** Sort siblings while retaining the parent/child relationships of the Map. */
export function sortBeadTree(nodes: readonly BeadTreeNode[], sort: BeadSort): BeadTreeNode[] {
  if (sort === 'default') return [...nodes]
  return [...nodes]
    .sort((a, b) => compareRows(a.row, b.row, sort))
    .map(node => ({ ...node, children: sortBeadTree(node.children, sort) }))
}
