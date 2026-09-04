/**
 * What kind of work a Bead is.
 *
 * The sixth colour rule gives each type one of the theme's ANSI tokens, so the
 * one server-side theme owns the hues and the dashboard owns only the mapping.
 * The colour is never the only signal: the type word is written beside it, in
 * the colour, so a row reads the same without it. A type the store spells that
 * this list does not know keeps the word and takes `--text-dim` rather than an
 * invented hue, the same bargain the identity colours strike.
 *
 * The token per type lives in `BeadTypeLabel.css`; this module owns the word.
 */

/** The types the doctrine's sixth row names, in token order. */
export const BEAD_TYPES = ['bug', 'feature', 'chore', 'decision', 'epic', 'task'] as const

export type BeadType = (typeof BEAD_TYPES)[number]

/** A Bead with no type recorded is a task, which is what `bd` defaults to. */
export function beadTypeName(type: string | undefined): string {
  const named = (type ?? '').trim()
  return named === '' ? 'task' : named
}

/** The word as a row shows it: small, upright, and readable on its own. */
export function beadTypeLabel(type: string | undefined): string {
  return beadTypeName(type).replace(/_/g, ' ').toUpperCase()
}

export function isEpic(type: string | undefined): boolean {
  return beadTypeName(type) === 'epic'
}
