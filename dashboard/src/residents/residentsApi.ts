/**
 * The residents: the agent that lives in each of three tabs.
 *
 * The Librarian in the Library, the tender in Agents, the Clerk in Beads. The
 * host names each one's tmux session, the folder its launcher starts in and
 * the Beads project it works from, and one route reports all three. The tab
 * shows the session live in a column at its right edge; what is here is the
 * reading of that route and the arithmetic of the column's default width.
 */

import { apiErrorMessage } from '../apiErrors'

export type ResidentTab = 'library' | 'agents' | 'beads'

export interface Resident {
  tab: ResidentTab
  /** What the column's header calls the resident. */
  label: string
  /** The tmux session name; empty when the host configured none. */
  session: string
  /** Where the launcher starts the session when it is absent. */
  folder: string
  /** The project whose open Beads are the resident's proposals. */
  beads: string
}

/** What each column is called before the route has answered. */
export const RESIDENT_LABELS: Record<ResidentTab, string> = {
  library: 'Librarian',
  agents: 'Tender',
  beads: 'Clerk',
}

// The last answer, so a column opened a second time draws itself right from
// its first paint instead of reserving room for a resident that is not there.
let lastAnswer: Resident[] | null = null

export function readCachedResidents(): Resident[] | null {
  return lastAnswer
}

export async function fetchResidents(): Promise<Resident[]> {
  const response = await fetch('/api/residents', { signal: AbortSignal.timeout(20000) })
  if (!response.ok) throw new Error(apiErrorMessage(await response.text(), 'Could not read the residents'))
  const found = await response.json() as unknown
  lastAnswer = Array.isArray(found) ? found as Resident[] : []
  return lastAnswer
}

export function resetResidentsForTest(): void {
  lastAnswer = null
}

/** How many terminal columns the column shows with nothing remembered. */
export const RESIDENT_COLUMNS_DEFAULT = 44
/** The narrowest a column is honoured at: room for its header, whatever the font. */
export const RESIDENT_WIDTH_MIN = 240
/**
 * The tile font advances 600/1000 of its em, which is the cell width at every
 * size, so 44 columns is arithmetic rather than a measurement of a terminal
 * that may not exist yet.
 */
const CELL_WIDTH_EM = 0.6
/** The terminal's own padding and the column's hairline, beside the cells. */
const RESIDENT_CHROME = 9

/** The column's width with nothing remembered: 44 columns at the tile font. */
export function residentDefaultWidth(fontSize: number): number {
  return Math.ceil(RESIDENT_COLUMNS_DEFAULT * CELL_WIDTH_EM * fontSize) + RESIDENT_CHROME
}

/**
 * A remembered width is trusted only as far as it is a width: anything else,
 * including the 0 that means nothing was chosen, is the default for this font,
 * and nothing narrower than the minimum is honoured.
 */
export function clampResidentWidth(width: unknown, fontSize: number): number {
  if (typeof width !== 'number' || !Number.isFinite(width) || width <= 0) return residentDefaultWidth(fontSize)
  return Math.max(RESIDENT_WIDTH_MIN, Math.round(width))
}
