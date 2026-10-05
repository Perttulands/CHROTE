/**
 * What the browser already knows about a Bead before the server answers.
 *
 * The map holds a row for every Bead it draws — id, title, status, type,
 * priority, parent, blockers, updated — and a card opened from a row should
 * not wait for a bd process to repeat what the row already said. Rows are
 * remembered here as the map loads them, and cards as they arrive, so a card
 * opens from the best the session holds and fills its text in beneath. A
 * remembered card is refreshed every time it is opened; nothing here outlives
 * the page.
 */

import { fetchBead, type BeadDetail, type BeadLink, type BeadRow } from './beadsApi'

const rowsByProject = new Map<string, Map<string, BeadRow>>()
interface RowGroup { rows: readonly BeadRow[]; generation?: string; order: number }
const rowGroups = new Map<string, { work?: RowGroup; closed?: RowGroup }>()
const generations = new Map<string, string>()
const cardGenerations = new Map<string, string | undefined>()
const generationOrders = new Map<string, { next: number; orders: Map<string, number> }>()
const cards = new Map<string, BeadDetail>()

/** How far up a parent chain a seed walks; bd ids nest no deeper in practice. */
const MAX_PARENT_CHAIN = 4

function cardKey(projectPath: string, id: string): string {
  return `${projectPath} ${id}`
}

function linkOf(row: BeadRow): BeadLink {
  return { id: row.id, title: row.title, status: row.status, type: row.type, priority: row.priority }
}

function byPriorityThenId(a: BeadLink, b: BeadLink): number {
  if (a.priority !== b.priority) return a.priority - b.priority
  return a.id.localeCompare(b.id)
}

export function rememberBeadRows(projectPath: string, rows: readonly BeadRow[], generation?: string, group: 'work' | 'closed' = 'work'): void {
  if (generation && !generations.has(projectPath)) invalidateKnownBeads(projectPath, generation)
  const groups = rowGroups.get(projectPath) ?? {}
  const order = generation ? generationOrders.get(projectPath)?.orders.get(generation)
    ?? (groups[group]?.generation === generation ? groups[group]?.order : 0) : 0
  groups[group] = { rows, generation, order: order ?? 0 }
  rowGroups.set(projectPath, groups)
  indexRowGroups(projectPath)
}

function indexRowGroups(projectPath: string): void {
  const groups = rowGroups.get(projectPath)
  if (!groups) return
  const current = generations.get(projectPath)
  // Preserve previous context while projections arrive independently, but a
  // stale group can never overwrite the current group's version of an id.
  const ordered = [groups.closed, groups.work].filter((value): value is RowGroup => !!value)
    .sort((left, right) => Number(left.generation === current) - Number(right.generation === current) || left.order - right.order)
  rowsByProject.set(projectPath, new Map(ordered.flatMap(value => value.rows.map(row => [row.id, row] as const))))
}

export function invalidateKnownBeads(projectPath: string, generation?: string): void {
  if (generation) {
    generations.set(projectPath, generation)
    const observed = generationOrders.get(projectPath) ?? { next: 0, orders: new Map<string, number>() }
    if (!observed.orders.has(generation)) {
      observed.next += 1
      observed.orders.set(generation, observed.next)
      while (observed.orders.size > 16) observed.orders.delete(observed.orders.keys().next().value!)
    }
    generationOrders.set(projectPath, observed)
    const groups = rowGroups.get(projectPath)
    for (const group of [groups?.work, groups?.closed]) {
      if (group?.generation === generation) group.order = observed.orders.get(generation) ?? group.order
    }
    indexRowGroups(projectPath)
    return
  }
  generations.delete(projectPath)
  generationOrders.delete(projectPath)
  rowsByProject.delete(projectPath)
  rowGroups.delete(projectPath)
  for (const key of cards.keys()) if (key.startsWith(`${projectPath} `)) { cards.delete(key); cardGenerations.delete(key) }
}

export function forgetBeadDetail(projectPath: string, id: string): void {
  const key = cardKey(projectPath, id)
  cards.delete(key)
  cardGenerations.delete(key)
}

export function rememberBeadDetail(projectPath: string, detail: BeadDetail, generation?: string): void {
  const key = cardKey(projectPath, detail.id)
  cards.set(key, detail)
  cardGenerations.set(key, generation)
}

/** A Bead the map drew, as much of a card as its row can say. */
function seedFromRow(rows: Map<string, BeadRow>, row: BeadRow): BeadDetail {
  const parents: BeadLink[] = []
  const seen = new Set<string>()
  let parentId = row.parent
  while (parentId && parents.length < MAX_PARENT_CHAIN && !seen.has(parentId)) {
    seen.add(parentId)
    const parent = rows.get(parentId)
    if (!parent) break
    parents.push(linkOf(parent))
    parentId = parent.parent
  }
  const children: BeadLink[] = []
  const blocks: BeadLink[] = []
  rows.forEach(other => {
    if (other.parent === row.id) children.push(linkOf(other))
    if (other.blockedBy?.includes(row.id)) blocks.push(linkOf(other))
  })
  // A blocker the map does not hold is still open: the server drops finished
  // blockers before it says a row is blocked.
  const blockedBy = (row.blockedBy ?? []).map(id => {
    const blocker = rows.get(id)
    return blocker ? linkOf(blocker) : { id, title: '', status: 'open', priority: 3 }
  })
  return {
    ...linkOf(row),
    updated: row.updated,
    parents,
    children: children.sort(byPriorityThenId),
    blockedBy,
    blocks: blocks.sort(byPriorityThenId),
  }
}

export interface KnownBead {
  bead: BeadDetail
  /** False when the Bead is only what its row said and its text is still to come. */
  complete: boolean
}

/** The best the session holds for a Bead, or null when it has never seen it. */
export function knownBead(projectPath: string, id: string): KnownBead | null {
  const card = cards.get(cardKey(projectPath, id))
  const current = !generations.has(projectPath) || cardGenerations.get(cardKey(projectPath, id)) === generations.get(projectPath)
  if (card && current) return { bead: card, complete: true }
  const rows = rowsByProject.get(projectPath)
  const row = rows?.get(id)
  if (!rows || !row) return card ? { bead: card, complete: false } : null
  return { bead: { ...card, ...seedFromRow(rows, row) }, complete: false }
}

/** Read the card from the server and remember it for the session. */
export async function readBead(projectPath: string, id: string): Promise<BeadDetail> {
  const detail = await fetchBead(projectPath, id)
  rememberBeadDetail(projectPath, detail)
  return detail
}

export function resetKnownBeadsForTest(): void {
  rowsByProject.clear()
  cards.clear()
  rowGroups.clear()
  generations.clear()
  generationOrders.clear()
  cardGenerations.clear()
}
