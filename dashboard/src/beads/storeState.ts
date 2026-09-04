/**
 * A store's state, as the Beads rail's lower half reads it.
 *
 * Every number here comes from the server's manifest-keyed counts projection,
 * so this module only turns those counts into rows a bar can be drawn from and
 * decides which warnings the store has earned. Nothing counts the work payload:
 * that payload omits the closed rows, so it cannot answer these questions.
 */

import type { BeadsCounts } from '../workspaces/workspacesApi'
import { daysSince } from './beadStatus'
import { beadTypeLabel } from './beadType'

/** A store with no update in this many days is stale enough to say so. */
export const STORE_STALE_DAYS = 30

/** One line of a counts block: its word, its number, and its share of the widest. */
export interface CountRow {
  key: string
  label: string
  count: number
  /** 0 to 1, against the largest count in the same block, for the bar's width. */
  share: number
}

export type StoreWarningKind = 'unreadable' | 'stale' | 'blocked'

export interface StoreWarning {
  kind: StoreWarningKind
  text: string
}

const STATUS_ORDER: { key: keyof BeadsCounts['status']; label: string }[] = [
  { key: 'open', label: 'open' },
  { key: 'inProgress', label: 'in progress' },
  { key: 'blocked', label: 'blocked' },
  { key: 'closed', label: 'closed' },
  { key: 'deferred', label: 'deferred' },
]

const TYPE_ORDER: (keyof BeadsCounts['type'])[] = ['epic', 'task', 'bug', 'feature', 'decision', 'chore']

function withShares(rows: { key: string; label: string; count: number }[]): CountRow[] {
  const widest = rows.reduce((most, row) => Math.max(most, row.count), 0)
  return rows.map(row => ({ ...row, share: widest > 0 ? row.count / widest : 0 }))
}

/**
 * The five states in their fixed order, zeros included: the shape of the block
 * is the same for every store, so two stores can be compared at a glance.
 */
export function statusRows(counts: BeadsCounts): CountRow[] {
  return withShares(STATUS_ORDER.map(({ key, label }) => ({ key, label, count: counts.status[key] })))
}

/**
 * The types a store actually holds, largest first, ties in the canonical order.
 * A type nobody used is left out rather than drawn as an empty bar.
 */
export function typeRows(counts: BeadsCounts): CountRow[] {
  const present = TYPE_ORDER
    .map((key, index) => ({ key, label: beadTypeLabel(key), count: counts.type[key], index }))
    .filter(row => row.count > 0)
    .sort((left, right) => right.count - left.count || left.index - right.index)
    .map(({ key, label, count }) => ({ key, label, count }))
  return withShares(present)
}

/** Every Bead the store holds, which is the sum of the exclusive status groups. */
export function totalBeads(counts: BeadsCounts): number {
  return STATUS_ORDER.reduce((sum, { key }) => sum + counts.status[key], 0)
}

export interface StoreStateInput {
  error?: string
  counts?: BeadsCounts
  newestUpdate?: string
}

/**
 * What the panel warns about, worst first. An unreadable store says only that,
 * because with no projection there is nothing else true to say about it.
 */
export function storeWarnings(store: StoreStateInput, now: number = Date.now()): StoreWarning[] {
  if (store.error) return [{ kind: 'unreadable', text: `Store unreadable · ${store.error}` }]
  const warnings: StoreWarning[] = []
  const age = daysSince(store.newestUpdate, now)
  if (age >= STORE_STALE_DAYS) {
    warnings.push({ kind: 'stale', text: `Stale · no update in ${age} days` })
  }
  const blocked = store.counts?.status.blocked ?? 0
  if (blocked > 0) {
    warnings.push({ kind: 'blocked', text: `Blocked · ${blocked} ${blocked === 1 ? 'Bead is' : 'Beads are'} waiting` })
  }
  return warnings
}
