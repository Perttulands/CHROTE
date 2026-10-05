import { afterEach, describe, expect, it } from 'vitest'
import { invalidateKnownBeads, knownBead, rememberBeadDetail, rememberBeadRows, resetKnownBeadsForTest } from './knownBeads'
import type { BeadDetail, BeadRow } from './beadsApi'
const row: BeadRow = { id: 'test-one', title: 'Previous', status: 'open', priority: 1, blocked: false, linked: false }
const detail: BeadDetail = { ...row, description: 'Previous text', parents: [], children: [], blockedBy: [], blocks: [] }
afterEach(resetKnownBeadsForTest)
describe('generation-aware remembered rows', () => {
  it('prefers current Closed over stale Work before the current unfinished projection arrives', () => {
    rememberBeadRows('/one', [row], 'one', 'work')
    rememberBeadDetail('/one', detail, 'one')
    invalidateKnownBeads('/one', 'two')
    rememberBeadRows('/one', [{ ...row, status: 'closed', title: 'Closed now' }], 'two', 'closed')
    expect(knownBead('/one', row.id)).toMatchObject({ complete: false, bead: { status: 'closed', title: 'Closed now', description: 'Previous text' } })
    rememberBeadRows('/one', [], 'two', 'work')
    expect(knownBead('/one', row.id)?.bead.status).toBe('closed')
  })
  it('prefers current Work over stale Closed when a Bead reopens', () => {
    rememberBeadRows('/one', [{ ...row, status: 'closed' }], 'one', 'closed')
    rememberBeadDetail('/one', { ...detail, status: 'closed' }, 'one')
    invalidateKnownBeads('/one', 'two')
    rememberBeadRows('/one', [{ ...row, title: 'Reopened now' }], 'two', 'work')
    expect(knownBead('/one', row.id)).toMatchObject({ complete: false, bead: { status: 'open', title: 'Reopened now' } })
    rememberBeadRows('/one', [], 'two', 'closed')
    expect(knownBead('/one', row.id)?.bead.status).toBe('open')
  })
})

it('keeps the newer last-successful closed row ahead of an older work response while a third generation is pending', () => {
  rememberBeadRows('/one', [row], 'one', 'work')
  invalidateKnownBeads('/one', 'two')
  rememberBeadRows('/one', [{ ...row, status: 'closed', title: 'Closed now' }], 'two', 'closed')
  invalidateKnownBeads('/one', 'three')
  rememberBeadRows('/one', [row], 'one', 'work')
  expect(knownBead('/one', row.id)).toMatchObject({ complete: false, bead: { status: 'closed', title: 'Closed now' } })
})

it('reindexes a projection that arrived before its generation was observed by state', () => {
  rememberBeadRows('/one', [{ ...row, status: 'closed' }], 'one', 'closed')
  rememberBeadRows('/one', [{ ...row, title: 'Reopened now' }], 'two', 'work')
  invalidateKnownBeads('/one', 'two')
  expect(knownBead('/one', row.id)).toMatchObject({ complete: false, bead: { status: 'open', title: 'Reopened now' } })
})
