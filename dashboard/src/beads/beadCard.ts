/**
 * Asking for the Bead card, from anywhere.
 *
 * A Bead id is a link in a terminal, in a row of the map, and inside another
 * Bead's text. None of those places is a React parent of the card, and the
 * terminal is not React at all, so the request travels as a fact the card
 * subscribes to rather than as a prop passed down a tree. The fact is the
 * table's: opening a Bead puts it on the table, and the card is what the
 * table shows for a Bead.
 */

import {
  clearTable,
  putOnTable,
  readTable,
  resetTableForTest,
  stepBackOnTable,
  useTableObject,
  type BeadOnTable,
} from '../context/TableContext'

export type BeadCardRequest = BeadOnTable

export function openBeadCard(id: string, projectPath?: string, title?: string): void {
  putOnTable({ kind: 'bead', id, projectPath, title })
}

/** The card following an id in its own text: the Bead in hand joins the trail. */
export function followBeadFromCard(id: string, projectPath?: string): void {
  const current = readTable()
  const trail = current?.kind === 'bead' ? [...current.trail, current.id] : []
  putOnTable({ kind: 'bead', id, projectPath, trail })
}

/** Back along the trail; nothing happens with nothing behind. */
export function backInBeadCard(): void {
  stepBackOnTable()
}

export function closeBeadCard(): void {
  if (readTable()?.kind !== 'bead') return
  clearTable()
}

export function useBeadCardRequest(): BeadCardRequest | null {
  const object = useTableObject()
  return object?.kind === 'bead' ? object : null
}

export function resetBeadCardForTest(): void {
  resetTableForTest()
}
