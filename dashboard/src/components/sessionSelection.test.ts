import { describe, expect, it } from 'vitest'
import {
  EMPTY_SELECTION,
  killTargets,
  plainClick,
  retainExisting,
  selectRange,
  toggleSession,
  withoutKilled,
} from './sessionSelection'

const visible = ['a', 'b', 'c', 'd', 'e']
const keysOf = (selection: { keys: ReadonlySet<string> }) => Array.from(selection.keys).sort()

describe('session selection', () => {
  it('toggles one row in and out with Ctrl or Cmd', () => {
    const one = toggleSession(EMPTY_SELECTION, 'b')
    const two = toggleSession(one, 'd')
    expect(keysOf(two)).toEqual(['b', 'd'])
    expect(keysOf(toggleSession(two, 'b'))).toEqual(['d'])
  })

  it('adds the visible rows between the last clicked row and the Shift-clicked one, in either direction', () => {
    const fromB = toggleSession(EMPTY_SELECTION, 'b')
    expect(keysOf(selectRange(fromB, 'd', visible))).toEqual(['b', 'c', 'd'])
    const fromE = toggleSession(EMPTY_SELECTION, 'e')
    expect(keysOf(selectRange(fromE, 'c', visible))).toEqual(['c', 'd', 'e'])
    // A range starts where the last click landed, plain or not.
    expect(keysOf(selectRange(plainClick('a'), 'b', visible))).toEqual(['a', 'b'])
  })

  it('selects only the clicked row when the last clicked row is no longer visible', () => {
    const hiddenAnchor = toggleSession(EMPTY_SELECTION, 'z')
    expect(keysOf(selectRange(hiddenAnchor, 'c', visible))).toEqual(['c', 'z'])
  })

  it('empties the selection on a plain click', () => {
    expect(keysOf(plainClick('d'))).toEqual([])
  })

  it('kills only the selected rows the operator can see, in the order shown', () => {
    const selection = { keys: new Set(['d', 'hidden', 'b']), anchor: 'b' }
    expect(killTargets(selection, visible)).toEqual(['b', 'd'])
    expect(killTargets(EMPTY_SELECTION, visible)).toEqual([])
  })

  it('drops a session that disappeared, so a new one under its name is not selected', () => {
    const selection = { keys: new Set(['a', 'gone']), anchor: 'a' }
    expect(keysOf(retainExisting(selection, new Set(['a', 'b'])))).toEqual(['a'])
  })

  it('keeps failed and hidden rows selected after a kill', () => {
    const selection = { keys: new Set(['a', 'b', 'hidden']), anchor: 'a' }
    expect(keysOf(withoutKilled(selection, ['a']))).toEqual(['b', 'hidden'])
  })
})
