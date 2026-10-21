/**
 * The Beads tab's slice: every Bead on the host, indexed once per read, and the
 * filter, counts, strip and order the view draws from it.
 *
 * The index is built when rows change and never during an interaction. A filter
 * is one pass over typed arrays that also produces each facet's counts (every
 * facet counted without its own filter, so a facet always says what choosing it
 * would give) and the strip's daily buckets (counted without the date range, so
 * the brush keeps its context). Sorting walks a per-column order computed once
 * per index. Links between Beads resolve across stores: a parent, child or
 * blocker in another store is the same Bead it is anywhere else.
 */

import type { BeadRow } from './beadsApi'
import type { HostBead, HostStore } from './hostBeads'
import { isBeadClosed } from './beadStatus'
import { beadReference } from './beadReference'

export type SliceStatus = 'open' | 'in_progress' | 'blocked' | 'deferred' | 'closed'

/** The status partition, in the order the facet lists it and a sort ranks it. */
export const SLICE_STATUSES: readonly SliceStatus[] = ['open', 'in_progress', 'blocked', 'deferred', 'closed']

const BLOCKED = 2
const CLOSED = 4

export const STATUS_LABELS: Record<SliceStatus, string> = {
  open: 'open',
  in_progress: 'in progress',
  blocked: 'blocked',
  deferred: 'deferred',
  closed: 'closed',
}

export const SLICE_SORT_KEYS = ['id', 'title', 'project', 'status', 'updated', 'idle', 'size', 'kids'] as const
export type SliceSortKey = typeof SLICE_SORT_KEYS[number]
export type SortDirection = 'asc' | 'desc'

/** The direction a column takes on its first click: text reads A to Z, numbers largest first. */
export const NATURAL_DIRECTION: Record<SliceSortKey, SortDirection> = {
  id: 'asc', title: 'asc', project: 'asc', status: 'asc', updated: 'desc', idle: 'desc', size: 'desc', kids: 'desc',
}

export interface SliceProject {
  /** The project's identity in a link: the prefix, or prefix:folder when two stores share it. */
  key: string
  /** What the column and the facet show: the prefix, or "prefix (folder)" when shared. */
  label: string
  path: string
  /** Every Bead the store holds. */
  total: number
  /** The local day the store's earliest Bead was created, or -1. */
  firstDay: number
}

export interface SliceIndex {
  size: number
  rows: readonly HostBead[]
  projects: readonly SliceProject[]
  project: Uint16Array
  status: Uint8Array
  createdDay: Int32Array
  updatedDay: Int32Array
  closedDay: Int32Array
  /** Whole days since the last update, or -1 when unknown. */
  idle: Int32Array
  kids: Int32Array
  parent: Int32Array
  children: readonly (readonly number[])[]
  /** Resolved blockers; an id no store holds is kept by name in `unresolvedBlockers`. */
  blockers: readonly (readonly number[])[]
  dependents: readonly (readonly number[])[]
  unresolvedBlockers: ReadonlyMap<number, readonly string[]>
  /** Lowercased id and title, the part of a search the browser answers alone. */
  haystack: readonly string[]
  /** The strip's domain: the earliest created day to today, inclusive. */
  firstDay: number
  today: number
  maxSize: number
  maxIdle: number
  maxKids: number
  /** Row of a Bead id, preferring the named store; -1 when unknown or ambiguous. */
  find: (id: string, projectIndex?: number) => number
  /** Row of a Bead named as store path and id. */
  rowOf: (path: string, id: string) => number
  orders: Map<SliceSortKey, Uint32Array>
}

const DAY_MS = 86400000
/** 2000-01-01. A time before it is a missing date written as zero, not a day to draw. */
const EARLIEST = 946684800

function dated(seconds: number | undefined): seconds is number {
  return seconds !== undefined && seconds >= EARLIEST
}

/** The local calendar day of a Unix time, as days since 1970-01-01 of that calendar. */
export function localDay(seconds: number): number {
  const at = seconds * 1000
  return Math.floor((at - new Date(at).getTimezoneOffset() * 60000) / DAY_MS)
}

/** A day index as YYYY-MM-DD, the form a link carries. */
export function dayISO(day: number): string {
  return new Date(day * DAY_MS).toISOString().slice(0, 10)
}

export function parseDayISO(value: string | null): number | null {
  if (!value || !/^\d{4}-\d{2}-\d{2}$/.test(value)) return null
  const at = Date.parse(`${value}T00:00:00Z`)
  return Number.isFinite(at) ? Math.floor(at / DAY_MS) : null
}

const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']

/** A day index as the strip and the table read it: "Oct 21". */
export function dayLabel(day: number): string {
  const at = new Date(day * DAY_MS)
  return `${MONTHS[at.getUTCMonth()]} ${at.getUTCDate()}`
}

function basename(path: string): string {
  return path.split('/').filter(Boolean).slice(-1)[0] || path
}

/** Name every store so two stores never read alike, and a link names one exactly. */
export function projectIdentities(stores: readonly Pick<HostStore, 'path' | 'prefix'>[]): { key: string; label: string }[] {
  const shared = new Map<string, number>()
  stores.forEach(store => { if (store.prefix) shared.set(store.prefix, (shared.get(store.prefix) ?? 0) + 1) })
  const named = stores.map(store => {
    const folder = basename(store.path)
    if (store.prefix && shared.get(store.prefix) === 1) return { key: store.prefix, label: store.prefix }
    if (store.prefix) return { key: `${store.prefix}:${folder}`, label: `${store.prefix} (${folder})` }
    return { key: `:${folder}`, label: folder }
  })
  const seen = new Map<string, number>()
  named.forEach(name => seen.set(name.key, (seen.get(name.key) ?? 0) + 1))
  return named.map((name, i) => seen.get(name.key) === 1 ? name : { key: `${stores[i].prefix ?? ''}:${stores[i].path}`, label: `${name.label} ${stores[i].path}` })
}

/** A dated or explicit deferral that has not yet expired. */
function deferred(bead: HostBead, now: number): boolean {
  const until = bead.deferUntil ? Date.parse(bead.deferUntil) : NaN
  if (bead.status === 'deferred') return !Number.isFinite(until) || until > now
  return Number.isFinite(until) && until > now
}

export function buildSliceIndex(stores: readonly HostStore[], now: number = Date.now()): SliceIndex {
  const identities = projectIdentities(stores)
  const size = stores.reduce((sum, store) => sum + store.rows.length, 0)
  const rows: HostBead[] = new Array(size)
  const project = new Uint16Array(size)
  const byStore = new Map<string, number>()
  const byId = new Map<string, number>()
  // Every row of an id more than one store holds, as a Bead moved between
  // stores leaves it.
  const shared = new Map<string, number[]>()
  const storeIndex = new Map<string, number>()
  let at = 0
  stores.forEach((store, p) => {
    storeIndex.set(store.path, p)
    store.rows.forEach(row => {
      rows[at] = row
      project[at] = p
      byStore.set(`${p}\u0000${row.id}`, at)
      const first = byId.get(row.id)
      if (first === undefined) byId.set(row.id, at)
      else {
        if (first >= 0) shared.set(row.id, [first])
        shared.get(row.id)!.push(at)
        byId.set(row.id, -2)
      }
      at += 1
    })
  })
  const find = (id: string, projectIndex?: number) => {
    if (projectIndex !== undefined) {
      const own = byStore.get(`${projectIndex}\u0000${id}`)
      if (own !== undefined) return own
    }
    const anywhere = byId.get(id)
    return anywhere === undefined || anywhere < 0 ? -1 : anywhere
  }

  const createdDay = new Int32Array(size).fill(-1)
  const updatedDay = new Int32Array(size).fill(-1)
  const closedDay = new Int32Array(size).fill(-1)
  const idle = new Int32Array(size).fill(-1)
  const parent = new Int32Array(size).fill(-1)
  const kids = new Int32Array(size)
  const status = new Uint8Array(size)
  const children: number[][] = Array.from({ length: size }, () => [])
  const blockers: number[][] = Array.from({ length: size }, () => [])
  const dependents: number[][] = Array.from({ length: size }, () => [])
  const unresolvedBlockers = new Map<number, string[]>()
  const haystack: string[] = new Array(size)
  const today = localDay(now / 1000)
  let firstDay = today
  let maxSize = 0
  let maxIdle = 0
  let maxKids = 0
  const firstOfProject = new Int32Array(stores.length).fill(-1)

  for (let i = 0; i < size; i += 1) {
    const row = rows[i]
    haystack[i] = `${row.id} ${row.title}`.toLowerCase()
    if (dated(row.created)) {
      const day = localDay(row.created)
      createdDay[i] = day
      if (day < firstDay) firstDay = day
      const p = project[i]
      if (firstOfProject[p] < 0 || day < firstOfProject[p]) firstOfProject[p] = day
    }
    if (dated(row.updated)) {
      updatedDay[i] = localDay(row.updated)
      idle[i] = Math.max(0, Math.floor((now / 1000 - row.updated) / 86400))
      if (idle[i] > maxIdle) maxIdle = idle[i]
    }
    if (dated(row.closed)) closedDay[i] = localDay(row.closed)
    // A finished Bead with no close time finished at its last update, or the
    // open area would carry it forever.
    else if (isBeadClosed(row.status) && dated(row.updated)) closedDay[i] = localDay(row.updated)
    if ((row.size ?? 0) > maxSize) maxSize = row.size ?? 0
    if (row.parent) {
      const j = find(row.parent, project[i])
      if (j >= 0 && j !== i) {
        parent[i] = j
        children[j].push(i)
      }
    }
    row.blockedBy?.forEach(id => {
      const j = find(id, project[i])
      // An id two other stores hold could name either: it waits on both, so
      // it is finished only when both are.
      const named = j >= 0 ? [j] : shared.get(id)
      if (named) {
        named.forEach(k => {
          if (k === i) return
          blockers[i].push(k)
          dependents[k].push(i)
        })
      } else {
        const missing = unresolvedBlockers.get(i) ?? []
        missing.push(id)
        unresolvedBlockers.set(i, missing)
      }
    })
  }
  for (let i = 0; i < size; i += 1) {
    kids[i] = children[i].length
    if (kids[i] > maxKids) maxKids = kids[i]
    const row = rows[i]
    // The same partition the store counts use: closed, in progress, blocked
    // (by status or by any unfinished blocker, wherever it lives; a blocker no
    // store holds is unfinished), deferred, open.
    if (isBeadClosed(row.status)) status[i] = CLOSED
    else if (row.status === 'in_progress') status[i] = 1
    else if (row.status === 'blocked' || unresolvedBlockers.has(i) || blockers[i].some(j => !isBeadClosed(rows[j].status))) status[i] = BLOCKED
    else if (deferred(row, now)) status[i] = 3
    else status[i] = 0
  }

  const projects = stores.map((store, p) => ({
    key: identities[p].key,
    label: identities[p].label,
    path: store.path,
    total: store.rows.length,
    firstDay: firstOfProject[p],
  }))

  return {
    size, rows, projects, project, status, createdDay, updatedDay, closedDay, idle, kids, parent,
    children, blockers, dependents, unresolvedBlockers, haystack, firstDay, today, maxSize, maxIdle, maxKids,
    find,
    rowOf: (path: string, id: string) => {
      const p = storeIndex.get(path)
      return p === undefined ? -1 : byStore.get(`${p}\u0000${id}`) ?? -1
    },
    orders: new Map(),
  }
}

export interface SliceFilter {
  /** Lowercased search terms; every one must match. */
  terms: readonly string[]
  /** Rows the server found by their text, for the current terms; null while unknown. */
  textMatches: Uint8Array | null
  projects: ReadonlySet<number> | null
  statuses: ReadonlySet<number> | null
  /** Inclusive local days. A Bead is in range when it was created, updated or closed inside it. */
  range: { from: number; to: number } | null
}

export interface SliceResult {
  visible: Uint8Array
  count: number
  projectCounts: Int32Array
  statusCounts: Int32Array
  /** Per day from `firstDay` to `today`, for rows passing every filter but the range. */
  created: Int32Array
  closed: Int32Array
  /** Open at the end of each day, counted only from each Bead's own creation. */
  open: Int32Array
}

export function searchTerms(query: string): string[] {
  return query.toLowerCase().split(/\s+/).filter(Boolean)
}

function inRange(index: SliceIndex, i: number, from: number, to: number): boolean {
  const created = index.createdDay[i]
  const updated = index.updatedDay[i]
  const closed = index.closedDay[i]
  return (created >= from && created <= to) || (updated >= from && updated <= to) || (closed >= from && closed <= to)
}

export function filterSlice(index: SliceIndex, filter: SliceFilter): SliceResult {
  const { size } = index
  const visible = new Uint8Array(size)
  const projectCounts = new Int32Array(index.projects.length)
  const statusCounts = new Int32Array(SLICE_STATUSES.length)
  const days = Math.max(1, index.today - index.firstDay + 1)
  const created = new Int32Array(days)
  const closed = new Int32Array(days)
  const delta = new Int32Array(days + 1)
  const { terms, textMatches, projects, statuses, range } = filter
  let count = 0
  for (let i = 0; i < size; i += 1) {
    if (terms.length > 0 && textMatches?.[i] !== 1) {
      const text = index.haystack[i]
      let all = true
      for (const term of terms) {
        if (!text.includes(term)) { all = false; break }
      }
      if (!all) continue
    }
    const projectOk = projects === null || projects.has(index.project[i])
    const statusOk = statuses === null || statuses.has(index.status[i])
    if (projectOk && statusOk) {
      const c = index.createdDay[i]
      if (c >= index.firstDay) {
        created[c - index.firstDay] += 1
        delta[c - index.firstDay] += 1
        const d = index.closedDay[i]
        if (d >= 0) {
          // A close stamped before its own creation closes on the creation day.
          const at = Math.max(c, d) - index.firstDay
          if (at < days) closed[at] += 1
          delta[Math.min(days, at)] -= 1
        }
      }
    }
    if (range && !inRange(index, i, range.from, range.to)) continue
    if (statusOk) projectCounts[index.project[i]] += 1
    if (projectOk) statusCounts[index.status[i]] += 1
    if (projectOk && statusOk) {
      visible[i] = 1
      count += 1
    }
  }
  const open = new Int32Array(days)
  let running = 0
  for (let d = 0; d < days; d += 1) {
    running += delta[d]
    open[d] = running
  }
  return { visible, count, projectCounts, statusCounts, created, closed, open }
}

const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' })

/** The ascending order of every row by one column, computed once per index. */
export function sortOrder(index: SliceIndex, key: SliceSortKey): Uint32Array {
  const cached = index.orders.get(key)
  if (cached) return cached
  const order = new Uint32Array(index.size)
  for (let i = 0; i < index.size; i += 1) order[i] = i
  const { rows } = index
  const updated = (i: number) => rows[i].updated ?? 0
  const byId = (a: number, b: number) => collator.compare(rows[a].id, rows[b].id)
  let compare: (a: number, b: number) => number
  switch (key) {
    case 'id': compare = byId; break
    case 'title': compare = (a, b) => collator.compare(rows[a].title, rows[b].title) || byId(a, b); break
    case 'project': compare = (a, b) => collator.compare(index.projects[index.project[a]].label, index.projects[index.project[b]].label) || byId(a, b); break
    case 'status': compare = (a, b) => index.status[a] - index.status[b] || updated(b) - updated(a) || byId(a, b); break
    case 'updated': compare = (a, b) => updated(a) - updated(b) || byId(a, b); break
    case 'idle': compare = (a, b) => index.idle[a] - index.idle[b] || byId(a, b); break
    case 'size': compare = (a, b) => (rows[a].size ?? 0) - (rows[b].size ?? 0) || byId(a, b); break
    case 'kids': compare = (a, b) => index.kids[a] - index.kids[b] || byId(a, b); break
  }
  order.sort(compare)
  index.orders.set(key, order)
  return order
}

/** The visible rows in the chosen order. */
export function orderedRows(index: SliceIndex, result: SliceResult, key: SliceSortKey, direction: SortDirection): Int32Array {
  const order = sortOrder(index, key)
  const rows = new Int32Array(result.count)
  let at = 0
  if (direction === 'asc') {
    for (let k = 0; k < order.length; k += 1) if (result.visible[order[k]]) rows[at++] = order[k]
  } else {
    for (let k = order.length - 1; k >= 0; k -= 1) if (result.visible[order[k]]) rows[at++] = order[k]
  }
  return rows
}

/** Rows whose text the server matched, as a mask over the index. */
export function textMatchMask(index: SliceIndex, matches: Record<string, readonly string[]>): Uint8Array {
  const mask = new Uint8Array(index.size)
  Object.entries(matches).forEach(([path, ids]) => {
    ids.forEach(id => {
      const i = index.rowOf(path, id)
      if (i >= 0) mask[i] = 1
    })
  })
  return mask
}

/** A Bead size as the table reads it: 940, 6.4k. */
export function formatSize(size: number): string {
  if (size < 1000) return String(size)
  return `${(size / 1000).toFixed(size < 9950 ? 1 : 0)}k`
}

/**
 * One store's Beads as rows the card can seed itself from, so a card opened
 * from the table draws its parent, children and blockers before bd answers.
 * A blocker counts only while it is unfinished, as the card's own read says.
 */
export function storeRowsForCard(index: SliceIndex, project: number): BeadRow[] {
  const rows: BeadRow[] = []
  for (let i = 0; i < index.size; i += 1) {
    if (index.project[i] !== project) continue
    const bead = index.rows[i]
    const blockedBy = [...new Set([
      ...index.blockers[i].filter(j => index.status[j] !== CLOSED).map(j => index.rows[j].id),
      ...(index.unresolvedBlockers.get(i) ?? []),
    ])]
    rows.push({
      id: bead.id,
      title: bead.title,
      status: bead.status,
      type: bead.type,
      priority: bead.priority,
      updated: bead.updated ? new Date(bead.updated * 1000).toISOString() : undefined,
      deferUntil: bead.deferUntil,
      parent: bead.parent,
      blockedBy,
      blocked: index.status[i] === BLOCKED,
      linked: !!bead.parent || blockedBy.length > 0 || index.kids[i] > 0 || index.dependents[i].length > 0,
    })
  }
  return rows
}

/**
 * The line that names a row's Bead to an agent. A store whose prefix another
 * store shares, or an id another store also holds, adds the store's folder:
 * the receiver runs bd from that folder, and the id alone would not say which.
 */
export function rowReference(index: SliceIndex, row: number): string {
  const bead = index.rows[row]
  const project = index.projects[index.project[row]]
  const ambiguous = project.key.includes(':') || index.find(bead.id) !== row
  return beadReference(bead, ambiguous ? project.path : undefined)
}
