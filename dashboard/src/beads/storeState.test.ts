import { describe, it, expect } from 'vitest'
import { statusRows, storeWarnings, totalBeads, typeRows } from './storeState'
import type { BeadsCounts } from '../workspaces/workspacesApi'

const counts = (status: Partial<BeadsCounts['status']>, type: Partial<BeadsCounts['type']> = {}): BeadsCounts => ({
  status: { open: 0, inProgress: 0, blocked: 0, closed: 0, deferred: 0, ...status },
  type: { epic: 0, task: 0, bug: 0, feature: 0, decision: 0, chore: 0, ...type },
})

const NOW = Date.parse('2026-09-04T12:00:00Z')

describe('the store state projection', () => {
  it('keeps the five states in order with zeros, scaled against the widest', () => {
    const rows = statusRows(counts({ open: 10, inProgress: 2, closed: 20 }))
    expect(rows.map(row => [row.key, row.count])).toEqual([
      ['open', 10], ['inProgress', 2], ['blocked', 0], ['closed', 20], ['deferred', 0],
    ])
    expect(rows.map(row => row.share)).toEqual([0.5, 0.1, 0, 1, 0])
  })

  it('gives every state a zero share when the store is empty', () => {
    expect(statusRows(counts({})).every(row => row.share === 0)).toBe(true)
  })

  it('ranks the types the store holds and leaves out the ones it does not', () => {
    const rows = typeRows(counts({}, { task: 12, epic: 3, bug: 3 }))
    expect(rows.map(row => [row.key, row.label, row.count])).toEqual([
      ['task', 'TASK', 12], ['epic', 'EPIC', 3], ['bug', 'BUG', 3],
    ])
  })

  it('sums the exclusive status groups into the store total', () => {
    expect(totalBeads(counts({ open: 4, inProgress: 1, blocked: 2, closed: 30, deferred: 3 }))).toBe(40)
  })
})

describe('the store warnings', () => {
  it('says only that a store is unreadable, with the error the server gave', () => {
    expect(storeWarnings({ error: 'permission denied', counts: counts({ blocked: 5 }) }, NOW)).toEqual([
      { kind: 'unreadable', text: 'Store unreadable · permission denied' },
    ])
  })

  it('warns about stale work and blocked work together', () => {
    const warnings = storeWarnings({
      counts: counts({ open: 3, blocked: 2 }),
      newestUpdate: '2026-06-04T12:00:00Z',
    }, NOW)
    expect(warnings.map(warning => warning.kind)).toEqual(['stale', 'blocked'])
    expect(warnings[0].text).toBe('Stale · no update in 92 days')
    expect(warnings[1].text).toBe('Blocked · 2 Beads are waiting')
  })

  it('says nothing about a store touched inside the stale window with nothing blocked', () => {
    expect(storeWarnings({ counts: counts({ open: 3 }), newestUpdate: '2026-09-01T12:00:00Z' }, NOW)).toEqual([])
  })

  it('counts one blocked Bead in the singular', () => {
    const warnings = storeWarnings({ counts: counts({ blocked: 1 }), newestUpdate: '2026-09-04T09:00:00Z' }, NOW)
    expect(warnings).toEqual([{ kind: 'blocked', text: 'Blocked · 1 Bead is waiting' }])
  })
})
