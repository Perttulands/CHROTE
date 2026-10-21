import { describe, expect, it } from 'vitest'
import {
  SLICE_STATUSES,
  buildSliceIndex,
  dayISO,
  filterSlice,
  localDay,
  orderedRows,
  parseDayISO,
  projectIdentities,
  searchTerms,
  storeRowsForCard,
  textMatchMask,
  type SliceFilter,
  type SliceIndex,
} from './beadsSlice'
import type { HostBead, HostStore } from './hostBeads'

// Noon UTC, so the local calendar day is the same date in any zone a test runs in.
const DAY = 86400
const at = (iso: string) => Date.parse(`${iso}T12:00:00Z`) / 1000
const NOW = Date.parse('2026-10-21T12:00:00Z')

function bead(id: string, overrides: Partial<HostBead> = {}): HostBead {
  return { id, title: `Title of ${id}`, status: 'open', priority: 2, created: at('2026-10-01'), updated: at('2026-10-01'), ...overrides }
}

function store(path: string, prefix: string, rows: HostBead[]): HostStore {
  return { path, prefix, generation: 'g1', pending: false, rows }
}

const ALL: SliceFilter = { terms: [], textMatches: null, projects: null, statuses: null, range: null }
const rowOf = (index: SliceIndex, id: string) => index.rows.findIndex(row => row.id === id)
const visibleIds = (index: SliceIndex, filter: SliceFilter) => {
  const result = filterSlice(index, filter)
  return index.rows.filter((_, i) => result.visible[i]).map(row => row.id).sort()
}
const status = (index: SliceIndex, id: string) => SLICE_STATUSES[index.status[rowOf(index, id)]]

describe('links across stores', () => {
  const index = buildSliceIndex([
    store('/srv/data/context', 'library', [
      bead('library-zvi', { type: 'epic' }),
      bead('library-zvi.1', { parent: 'library-zvi', blockedBy: ['ctx-xvl', 'srv-lbpn'] }),
      bead('library-zvi.2', { parent: 'library-zvi', blockedBy: ['library-zvi.1'] }),
    ]),
    store('/srv/context-citadel', 'ctx', [
      bead('ctx-xvl', { status: 'closed', closed: at('2026-10-20') }),
      bead('ctx-child', { parent: 'library-zvi' }),
    ]),
    store('/srv', 'srv', [bead('srv-lbpn')]),
  ], NOW)

  it('counts a child in another store among its parent\'s kids', () => {
    expect(index.kids[rowOf(index, 'library-zvi')]).toBe(3)
    expect(index.parent[rowOf(index, 'ctx-child')]).toBe(rowOf(index, 'library-zvi'))
  })

  it('reads a Bead as blocked while any blocker in any store is unfinished', () => {
    expect(status(index, 'library-zvi.1')).toBe('blocked')
    expect(status(index, 'library-zvi.2')).toBe('blocked')
    const done = buildSliceIndex([
      store('/a', 'a', [bead('a-1', { blockedBy: ['b-1'] })]),
      store('/b', 'b', [bead('b-1', { status: 'closed' })]),
    ], NOW)
    expect(status(done, 'a-1')).toBe('open')
  })

  it('counts a blocker no store holds as unfinished', () => {
    const lost = buildSliceIndex([store('/a', 'a', [bead('a-1', { blockedBy: ['gone-1'] })])], NOW)
    expect(status(lost, 'a-1')).toBe('blocked')
    expect(lost.unresolvedBlockers.get(0)).toEqual(['gone-1'])
  })

  it('resolves an id in its own store first and refuses a guess between two others', () => {
    const shared = buildSliceIndex([
      store('/home/alice', 'home', [bead('home-1'), bead('home-2', { parent: 'home-1' })]),
      store('/home/bob', 'home', [bead('home-1'), bead('home-3', { parent: 'home-1' })]),
    ], NOW)
    expect(shared.parent[3]).toBe(2)
    expect(shared.parent[1]).toBe(0)
    expect(shared.find('home-1')).toBe(-1)
    expect(shared.find('home-1', 1)).toBe(2)
  })

  it('seeds a card from its own store with unfinished blockers only', () => {
    const rows = storeRowsForCard(index, 0)
    expect(rows.map(row => row.id)).toEqual(['library-zvi', 'library-zvi.1', 'library-zvi.2'])
    expect(rows[1]).toMatchObject({ blockedBy: ['srv-lbpn'], blocked: true, parent: 'library-zvi', linked: true })
  })
})

describe('store names', () => {
  it('names a shared prefix by its folder, so the two homes never read alike', () => {
    expect(projectIdentities([
      { path: '/home/alice', prefix: 'home' },
      { path: '/home/bob', prefix: 'home' },
      { path: '/srv', prefix: 'srv' },
      { path: '/srv/unnamed' },
    ])).toEqual([
      { key: 'home:alice', label: 'home (alice)' },
      { key: 'home:bob', label: 'home (bob)' },
      { key: 'srv', label: 'srv' },
      { key: ':unnamed', label: 'unnamed' },
    ])
  })
})

describe('the strip', () => {
  // An old store with one Bead open since the start, and a store that arrives
  // on Oct 10 already holding three Beads, one closed the next day.
  const index = buildSliceIndex([
    store('/old', 'old', [bead('old-1', { created: at('2026-10-01') })]),
    store('/new', 'new', [
      bead('new-1', { created: at('2026-10-10') }),
      bead('new-2', { created: at('2026-10-10') }),
      bead('new-3', { created: at('2026-10-10'), status: 'closed', closed: at('2026-10-11') }),
    ]),
  ], NOW)
  const first = localDay(at('2026-10-01'))
  const day = (iso: string) => localDay(at(iso)) - first

  it('counts a Bead open only from its own creation', () => {
    const result = filterSlice(index, ALL)
    expect(index.firstDay).toBe(first)
    expect(result.open[day('2026-10-09')]).toBe(1)
    expect(result.open[day('2026-10-10')]).toBe(4)
    expect(result.open[day('2026-10-11')]).toBe(3)
    expect(result.open[day('2026-10-21')]).toBe(3)
    expect(result.created[day('2026-10-10')]).toBe(3)
    expect(result.closed[day('2026-10-11')]).toBe(1)
  })

  it('marks the day a store was first seen', () => {
    expect(index.projects.map(project => project.firstDay - first)).toEqual([0, day('2026-10-10')])
  })

  it('closes a Bead stamped closed before it was created on its creation day', () => {
    const odd = buildSliceIndex([store('/a', 'a', [
      bead('a-0', { created: at('2026-10-01') }),
      bead('a-1', { created: at('2026-10-05'), status: 'closed', closed: at('2026-10-03') }),
    ])], NOW)
    const result = filterSlice(odd, ALL)
    const d = localDay(at('2026-10-05')) - odd.firstDay
    expect(result.closed[d]).toBe(1)
    expect(result.open[d]).toBe(1)
    expect(Math.min(...result.open)).toBeGreaterThanOrEqual(0)
  })

  it('closes a finished Bead with no close time on its last update', () => {
    const odd = buildSliceIndex([store('/a', 'a', [
      bead('a-1', { created: at('2026-10-01'), updated: at('2026-10-04'), status: 'closed' }),
    ])], NOW)
    const result = filterSlice(odd, ALL)
    expect(result.open[localDay(at('2026-10-05')) - odd.firstDay]).toBe(0)
  })
})

describe('filtering', () => {
  const index = buildSliceIndex([
    store('/a', 'a', [
      bead('a-1', { title: 'Grill brain ownership', created: at('2026-10-02'), updated: at('2026-10-02') }),
      bead('a-2', { title: 'Something else', status: 'closed', created: at('2026-10-02'), updated: at('2026-10-20'), closed: at('2026-10-20') }),
      bead('a-3', { title: 'Move records', status: 'in_progress', created: at('2026-10-05'), updated: at('2026-10-06') }),
    ]),
    store('/b', 'b', [
      bead('b-1', { title: 'Brain catalogue', created: at('2026-10-03'), updated: at('2026-10-03') }),
    ]),
  ], NOW)
  const closed = SLICE_STATUSES.indexOf('closed')
  const open = SLICE_STATUSES.indexOf('open')

  it('keeps a Bead only when every term is in its id or title', () => {
    expect(visibleIds(index, { ...ALL, terms: searchTerms('BRAIN') })).toEqual(['a-1', 'b-1'])
    expect(visibleIds(index, { ...ALL, terms: searchTerms('brain grill') })).toEqual(['a-1'])
  })

  it('adds the Beads the server matched by their text', () => {
    const textMatches = textMatchMask(index, { '/a': ['a-3'], '/elsewhere': ['x-1'] })
    expect(visibleIds(index, { ...ALL, terms: searchTerms('evidence'), textMatches })).toEqual(['a-3'])
  })

  it('counts each facet without its own filter', () => {
    const result = filterSlice(index, { ...ALL, projects: new Set([1]), statuses: new Set([open]) })
    expect(result.count).toBe(1)
    // Project counts honour the status filter but not the project filter.
    expect([...result.projectCounts]).toEqual([1, 1])
    // Status counts honour the project filter but not the status filter.
    expect(result.statusCounts[open]).toBe(1)
    expect(result.statusCounts[closed]).toBe(0)
  })

  it('keeps a Bead created, updated or closed in the range, and leaves the strip whole', () => {
    const range = { from: localDay(at('2026-10-20')), to: localDay(at('2026-10-21')) }
    expect(visibleIds(index, { ...ALL, range })).toEqual(['a-2'])
    const result = filterSlice(index, { ...ALL, range })
    expect(result.created.reduce((sum, n) => sum + n, 0)).toBe(4)
    expect(visibleIds(index, { ...ALL, range: { from: localDay(at('2026-10-06')), to: localDay(at('2026-10-06')) } })).toEqual(['a-3'])
  })
})

describe('sorting', () => {
  const index = buildSliceIndex([store('/a', 'a', [
    bead('a-2', { size: 900, updated: at('2026-10-01') }),
    bead('a-10', { size: 6400, updated: at('2026-10-20') }),
    bead('a-1', { size: 900, updated: at('2026-10-11') }),
  ])], NOW)
  const all = filterSlice(index, ALL)
  const ids = (key: Parameters<typeof orderedRows>[2], direction: 'asc' | 'desc') =>
    [...orderedRows(index, all, key, direction)].map(row => index.rows[row].id)

  it('orders ids naturally', () => {
    expect(ids('id', 'asc')).toEqual(['a-1', 'a-2', 'a-10'])
  })

  it('orders by size and by idle days, largest first, ties by id', () => {
    expect(ids('size', 'desc')).toEqual(['a-10', 'a-2', 'a-1'])
    expect(ids('idle', 'desc')).toEqual(['a-2', 'a-1', 'a-10'])
    expect([...index.idle]).toEqual([20, 1, 10])
  })

  it('keeps only the visible rows in order', () => {
    const narrow = filterSlice(index, { ...ALL, terms: ['a-1'] })
    expect([...orderedRows(index, narrow, 'size', 'desc')].map(row => index.rows[row].id)).toEqual(['a-10', 'a-1'])
  })
})

describe('days', () => {
  it('writes and reads a day as the link carries it', () => {
    const day = localDay(at('2026-10-20'))
    expect(dayISO(day)).toBe('2026-10-20')
    expect(parseDayISO('2026-10-20')).toBe(day)
    expect(parseDayISO('20.10.2026')).toBeNull()
  })

  it('takes the local calendar day', () => {
    expect(localDay(at('2026-10-21')) - localDay(at('2026-10-20'))).toBe(1)
    expect(localDay(at('2026-10-21') + DAY / 4)).toBeGreaterThanOrEqual(localDay(at('2026-10-21')))
  })
})
