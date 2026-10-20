import { beforeEach, describe, expect, it } from 'vitest'
import { BRAIN_VIEWS_KEY, copyBrainView, emptyBrainViews, readBrainViews, writeBrainViews } from './brainViewState'
import type { BrainViewSnapshot } from './brainViewState'

const snapshot: BrainViewSnapshot = {
  mode: 'map', subject: 'knowledge/testing', picked: 'knowledge/testing/note.md',
  facet: 'software', collection: 'knowledge', hiddenKinds: ['source'], hiddenShelves: ['projects'], scrub: 670,
  cameras: { map: { x: -120, y: 340, scale: 1.8 }, subjects: { x: 20, y: 40, scale: 0.8 } },
}

beforeEach(() => localStorage.clear())

describe('Brain saved-view storage', () => {
  it('round-trips all four slots without changing existing device preferences', () => {
    localStorage.setItem('chrote-dashboard-state', '{"libraryHiddenShelves":["self"]}')
    const slots = emptyBrainViews()
    slots[0] = { name: 'Evidence', snapshot }
    slots[3] = { name: 'Overview', snapshot: { ...snapshot, mode: 'subjects', cameras: { map: null, subjects: null } } }
    writeBrainViews(localStorage, slots)
    expect(readBrainViews(localStorage)).toEqual(slots)
    expect(localStorage.getItem('chrote-dashboard-state')).toBe('{"libraryHiddenShelves":["self"]}')
    const copied = copyBrainView(snapshot)
    copied.hiddenKinds.push('note')
    copied.cameras.map!.x = 99
    expect(snapshot.hiddenKinds).toEqual(['source'])
    expect(snapshot.cameras.map!.x).toBe(-120)
  })

  it.each([
    '{broken',
    JSON.stringify({ version: 2, slots: emptyBrainViews() }),
    JSON.stringify({ version: 1, slots: [{ name: 'Wrong camera', snapshot: { ...snapshot, cameras: { map: { x: 0, y: 0, scale: 0 }, subjects: null } } }, null, null, null] }),
    JSON.stringify({ version: 1, slots: [{ name: 'Wrong time', snapshot: { ...snapshot, scrub: 1500 } }, null, null, null] }),
  ])('reports unusable stored views while retaining their original bytes', raw => {
    localStorage.setItem(BRAIN_VIEWS_KEY, raw)
    expect(() => readBrainViews(localStorage)).toThrow('Saved Brain views could not be read')
    expect(localStorage.getItem(BRAIN_VIEWS_KEY)).toBe(raw)
  })
})
