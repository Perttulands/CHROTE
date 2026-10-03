import { describe, expect, it } from 'vitest'
import { sortBeadRows, sortBeadTree } from './beadsSort'
import type { WorkRow } from './beadsTree'

const row = (id: string, fields: Partial<WorkRow> = {}): WorkRow => ({
  id, title: id, status: 'open', priority: 2, blocked: false, linked: false,
  projectPath: '/project', projectName: 'project', ...fields,
})

describe('Beads sorting', () => {
  it('sorts by actual update time across offsets and leaves unknown dates last in either direction', () => {
    const rows = [
      row('missing'), row('new', { updated: '2026-10-03T12:00:00Z' }),
      row('invalid', { updated: 'bad timestamp' }), row('old', { updated: '2026-10-03T13:00:00+03:00' }),
    ]
    expect(sortBeadRows(rows, 'updated-newest').map(item => item.id)).toEqual(['new', 'old', 'invalid', 'missing'])
    expect(sortBeadRows(rows, 'updated-oldest').map(item => item.id)).toEqual(['old', 'new', 'invalid', 'missing'])
    expect(rows[0].id).toBe('missing')
  })

  it('orders priorities, titles, natural IDs and types with deterministic ties across stores', () => {
    const rows = [
      row('p-10', { title: 'Alpha', priority: 3, type: 'bug' }),
      row('p-2', { title: 'Zulu', priority: 0, type: 'task' }),
      row('p-1', { title: 'beta', priority: 2, type: 'feature' }),
    ]
    expect(sortBeadRows(rows, 'priority').map(item => item.id)).toEqual(['p-2', 'p-1', 'p-10'])
    expect(sortBeadRows(rows, 'title').map(item => item.id)).toEqual(['p-10', 'p-1', 'p-2'])
    expect(sortBeadRows(rows, 'id').map(item => item.id)).toEqual(['p-1', 'p-2', 'p-10'])
    expect(sortBeadRows(rows, 'type').map(item => item.id)).toEqual(['p-10', 'p-1', 'p-2'])
    expect(sortBeadRows(rows, 'default')).toEqual(rows)
    const duplicates = [row('p-1', { projectPath: '/z' }), row('p-1', { projectPath: '/a' })]
    expect(sortBeadRows(duplicates, 'id').map(item => item.projectPath)).toEqual(['/a', '/z'])
  })

  it('sorts Map siblings without separating children from their parents', () => {
    const tree = [
      { row: row('p-2'), children: [{ row: row('p-2.10'), children: [] }, { row: row('p-2.1'), children: [] }] },
      { row: row('p-1'), children: [] },
    ]
    const sorted = sortBeadTree(tree, 'id')
    expect(sorted.map(node => node.row.id)).toEqual(['p-1', 'p-2'])
    expect(sorted[1].children.map(node => node.row.id)).toEqual(['p-2.1', 'p-2.10'])
    expect(tree[0].children[0].row.id).toBe('p-2.10')
  })
})
