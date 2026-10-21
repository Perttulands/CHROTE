import { describe, expect, it } from 'vitest'
import { mergeHostAnswer, type HostRead } from './hostBeads'

const EMPTY: HostRead = { stores: [], rowStores: [], loading: true }
const rows = [{ id: 'a-1', title: 'One', status: 'open', priority: 2 }]

describe('merging a host answer', () => {
  const first = mergeHostAnswer(EMPTY, {
    stores: [
      { path: '/b', prefix: 'b', generation: 'g1', pending: false, rows: [] },
      { path: '/a', prefix: 'a', generation: 'g1', pending: false, rows },
    ],
    checkedAt: '2026-10-21T12:00:00Z',
  })

  it('holds the stores in path order and is no longer loading', () => {
    expect(first.loading).toBe(false)
    expect(first.stores.map(store => store.path)).toEqual(['/a', '/b'])
    expect(first.rowStores).toBe(first.stores)
  })

  it('keeps the rows, and the index\'s input, of stores the answer says are unchanged', () => {
    const next = mergeHostAnswer(first, {
      stores: [
        { path: '/a', generation: 'g1', pending: false, unchanged: true },
        { path: '/b', generation: 'g1', pending: true, unchanged: true },
      ],
    })
    expect(next.stores[0].rows).toBe(rows)
    expect(next.stores[1].pending).toBe(true)
    expect(next.rowStores).toBe(first.rowStores)
  })

  it('takes new rows when a store moved to a new generation', () => {
    const fresh = [{ id: 'a-2', title: 'Two', status: 'open', priority: 2 }]
    const next = mergeHostAnswer(first, {
      stores: [
        { path: '/a', generation: 'g2', pending: false, rows: fresh },
        { path: '/b', generation: 'g1', pending: false, unchanged: true },
      ],
    })
    expect(next.stores[0].rows).toBe(fresh)
    expect(next.rowStores).not.toBe(first.rowStores)
  })

  it('keeps a store\'s last rows while a restarted server has not read it yet', () => {
    const next = mergeHostAnswer(first, {
      stores: [
        { path: '/a', pending: true },
        { path: '/b', generation: 'g1', pending: false, unchanged: true },
      ],
    })
    expect(next.stores[0]).toMatchObject({ path: '/a', prefix: 'a', generation: 'g1', pending: true, rows })
    expect(next.rowStores).toBe(first.rowStores)
  })

  it('keeps a failed store\'s last rows beside its error, and drops a store that went away', () => {
    const next = mergeHostAnswer(first, {
      stores: [{ path: '/a', generation: 'g1', pending: false, error: 'bd timed out', unchanged: true }],
    })
    expect(next.stores).toHaveLength(1)
    expect(next.stores[0]).toMatchObject({ error: 'bd timed out', rows })
    expect(next.rowStores).not.toBe(first.rowStores)
  })
})
