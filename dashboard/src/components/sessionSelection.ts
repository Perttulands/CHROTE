/**
 * Which session rows the operator has picked out to kill together.
 *
 * Device-local and never persisted. Keys are session keys, so two Unix users'
 * sessions of the same name are two rows. The anchor is the last row clicked,
 * where the next Shift+click range starts.
 *
 * The set may hold rows the operator cannot currently see — filtered out, or
 * in a collapsed group. They stay selected, but only what is visible is ever a
 * kill target: the operator kills what he can see.
 */
export interface SessionSelection {
  keys: ReadonlySet<string>
  anchor: string | null
}

export const EMPTY_SELECTION: SessionSelection = { keys: new Set(), anchor: null }

/** Ctrl or Cmd+click: the row flips in or out of the set. */
export function toggleSession(selection: SessionSelection, key: string): SessionSelection {
  const keys = new Set(selection.keys)
  if (keys.has(key)) keys.delete(key)
  else keys.add(key)
  return { keys, anchor: key }
}

/**
 * Shift+click: every visible row from the last clicked row to this one joins
 * the set. Without an anchor the operator can see, only this row joins.
 */
export function selectRange(selection: SessionSelection, key: string, visibleKeys: readonly string[]): SessionSelection {
  const to = visibleKeys.indexOf(key)
  const from = selection.anchor === null ? -1 : visibleKeys.indexOf(selection.anchor)
  const keys = new Set(selection.keys)
  if (to === -1 || from === -1) {
    keys.add(key)
  } else {
    const [start, end] = from <= to ? [from, to] : [to, from]
    for (const visible of visibleKeys.slice(start, end + 1)) keys.add(visible)
  }
  return { keys, anchor: key }
}

/** A plain click empties the set; the clicked row is where a range would start. */
export function plainClick(key: string): SessionSelection {
  return { keys: new Set(), anchor: key }
}

/**
 * Sessions that no longer exist leave the set, so a later session that takes
 * the same name is not killed for its predecessor. Returns the same object
 * when nothing left.
 */
export function retainExisting(selection: SessionSelection, existingKeys: ReadonlySet<string>): SessionSelection {
  if (Array.from(selection.keys).every(key => existingKeys.has(key))) return selection
  return {
    keys: new Set(Array.from(selection.keys).filter(key => existingKeys.has(key))),
    anchor: selection.anchor,
  }
}

/** The selected rows the operator can see, in the order he sees them. */
export function killTargets(selection: SessionSelection, visibleKeys: readonly string[]): string[] {
  return visibleKeys.filter(key => selection.keys.has(key))
}

/** Killed rows leave the set; rows that failed, and hidden rows, stay. */
export function withoutKilled(selection: SessionSelection, killedKeys: readonly string[]): SessionSelection {
  if (killedKeys.length === 0) return selection
  const keys = new Set(selection.keys)
  for (const key of killedKeys) keys.delete(key)
  return { keys, anchor: selection.anchor }
}
