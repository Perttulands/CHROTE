/**
 * Open work as the Beads column lists it: what is claimed, and what is ready
 * to be claimed. The rules live here, away from the drawing, because they are
 * the part worth testing.
 */

import type { BeadRow } from './beadsApi'
import { isBeadClosed } from './beadStatus'

/** A row and the store it came from: the column draws several projects at once. */
export interface WorkRow extends BeadRow {
  projectPath: string
  projectName: string
}

function byUpdatedNewestFirst(a: WorkRow, b: WorkRow): number {
  return (b.updated ?? '').localeCompare(a.updated ?? '')
}

function isDeferred(row: WorkRow, now: number): boolean {
  if (!row.deferUntil) return false
  const until = Date.parse(row.deferUntil)
  return Number.isFinite(until) && until > now
}

/** Ready is open work with nothing in its way or deferred to a later date. */
export function readyRows(rows: readonly WorkRow[], now: number = Date.now()): WorkRow[] {
  return rows
    .filter(row => !isBeadClosed(row.status) && row.status !== 'in_progress' && !row.blocked && !isDeferred(row, now))
    .sort(byUpdatedNewestFirst)
}

export function inProgressRows(rows: readonly WorkRow[]): WorkRow[] {
  return rows.filter(row => row.status === 'in_progress').sort(byUpdatedNewestFirst)
}
