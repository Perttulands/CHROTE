/**
 * How a Bead's state reads.
 *
 * The glyphs are the ones bd itself prints, and they carry the state on their
 * own: a row says what it is without a colour, and a blocked row says why
 * beneath itself.
 */

export const BEAD_GLYPH_OPEN = '○'
export const BEAD_GLYPH_IN_PROGRESS = '◐'
export const BEAD_GLYPH_CLOSED = '✓'
export const BEAD_GLYPH_BLOCKED = '●'

export function isBeadClosed(status: string): boolean {
  return status === 'closed' || status === 'wont_fix' || status === 'duplicate'
}

export function beadGlyph(status: string, blocked = false): string {
  if (isBeadClosed(status)) return BEAD_GLYPH_CLOSED
  if (blocked) return BEAD_GLYPH_BLOCKED
  if (status === 'in_progress') return BEAD_GLYPH_IN_PROGRESS
  return BEAD_GLYPH_OPEN
}

export function beadStatusLabel(status: string, blocked = false): string {
  if (!isBeadClosed(status) && blocked) return 'blocked'
  return status.replace(/_/g, ' ')
}

/** Whole days between a bd timestamp and now; -1 when there is no timestamp. */
export function daysSince(updated: string | undefined, now: number = Date.now()): number {
  if (!updated) return -1
  const at = Date.parse(updated)
  if (Number.isNaN(at)) return -1
  return Math.floor((now - at) / 86400000)
}

/** A bd timestamp as the operator reads it: local date and time, no seconds. */
export function formatBeadTime(updated: string | undefined): string {
  if (!updated) return ''
  const at = new Date(updated)
  if (Number.isNaN(at.getTime())) return updated
  const pad = (value: number) => String(value).padStart(2, '0')
  return `${at.getFullYear()}-${pad(at.getMonth() + 1)}-${pad(at.getDate())} ${pad(at.getHours())}:${pad(at.getMinutes())}`
}
