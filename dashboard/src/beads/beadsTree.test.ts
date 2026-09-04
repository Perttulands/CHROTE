import { describe, expect, it } from 'vitest'
import {
  buildBeadMap,
  filterBeadTree,
  inProgressRows,
  readyRows,
  staleRows,
  type WorkRow,
} from './beadsTree'

function row(overrides: Partial<WorkRow> & { id: string }): WorkRow {
  return {
    title: `Title of ${overrides.id}`,
    status: 'open',
    priority: 1,
    blocked: false,
    projectPath: '/srv/chrote',
    projectName: 'chrote',
    ...overrides,
  }
}

const EPIC = row({ id: 'chrote-ep', type: 'epic', acceptance: 'Everything under it lands' })
const CHILD = row({ id: 'chrote-ep.1', parent: 'chrote-ep', type: 'task', priority: 1, updated: '2026-09-02T00:00:00Z' })
const BLOCKED = row({
  id: 'chrote-ep.2',
  parent: 'chrote-ep',
  type: 'task',
  priority: 2,
  blocked: true,
  blockedBy: ['chrote-ep.1'],
  updated: '2026-09-01T00:00:00Z',
})
const DONE = row({ id: 'chrote-ep.3', parent: 'chrote-ep', status: 'closed', type: 'task', priority: 3 })
const GRANDCHILD = row({ id: 'chrote-ep.1.1', parent: 'chrote-ep.1', type: 'task', priority: 1 })
const LOOSE = row({ id: 'chrote-lone', type: 'bug', priority: 2, updated: '2026-08-01T00:00:00Z' })
const ACTIVE = row({ id: 'chrote-now', status: 'in_progress', type: 'task', updated: '2026-09-03T00:00:00Z' })
const DEFERRED = row({ id: 'chrote-later', type: 'task', deferUntil: '2099-01-01T00:00:00Z' })

describe('the map', () => {
  const roots = buildBeadMap([DONE, BLOCKED, LOOSE, CHILD, EPIC, GRANDCHILD, ACTIVE])

  it('puts open epics first and hangs their children beneath them', () => {
    expect(roots.map(node => node.row.id)).toEqual(['chrote-ep', 'chrote-now', 'chrote-lone'])
    expect(roots[0].children.map(node => node.row.id)).toEqual(['chrote-ep.1', 'chrote-ep.2', 'chrote-ep.3'])
    expect(roots[0].children[0].children.map(node => node.row.id)).toEqual(['chrote-ep.1.1'])
  })

  it('draws every Bead once', () => {
    const drawn: string[] = []
    const walk = (nodes: typeof roots) => nodes.forEach(node => { drawn.push(node.row.id); walk(node.children) })
    walk(roots)
    expect(new Set(drawn).size).toBe(drawn.length)
  })

  it('keeps the branch that leads to a match', () => {
    const filtered = filterBeadTree(roots, 'ep.1.1')
    expect(filtered.map(node => node.row.id)).toEqual(['chrote-ep'])
    expect(filtered[0].children[0].children.map(node => node.row.id)).toEqual(['chrote-ep.1.1'])
  })

  it('matches on id and on title', () => {
    expect(filterBeadTree(roots, 'Title of chrote-lone').map(node => node.row.id)).toEqual(['chrote-lone'])
  })
})

describe('ready and in progress', () => {
  const rows = [CHILD, BLOCKED, DONE, ACTIVE, LOOSE]

  it('offers only unblocked open work as ready, newest first', () => {
    expect(readyRows(rows).map(item => item.id)).toEqual(['chrote-ep.1', 'chrote-lone'])
  })

  it('keeps future deferred work on the map but out of ready', () => {
    expect(readyRows([...rows, DEFERRED], Date.parse('2026-09-04T00:00:00Z')).map(item => item.id))
      .toEqual(['chrote-ep.1', 'chrote-lone'])
    expect(buildBeadMap([DEFERRED]).map(node => node.row.id)).toEqual(['chrote-later'])
  })

  it('lists what is claimed separately', () => {
    expect(inProgressRows(rows).map(item => item.id)).toEqual(['chrote-now'])
  })
})

describe('stale', () => {
  const now = Date.parse('2026-09-10T00:00:00Z')

  it('takes open work past the threshold, the most neglected first', () => {
    expect(staleRows([CHILD, BLOCKED, LOOSE, DONE, ACTIVE], 8, now).map(item => item.id))
      .toEqual(['chrote-lone', 'chrote-ep.2', 'chrote-ep.1'])
  })

  it('narrows as the threshold grows', () => {
    expect(staleRows([CHILD, BLOCKED, LOOSE], 60, now)).toEqual([])
  })
})
