import { describe, expect, it } from 'vitest'
import { inProgressRows, readyRows, type WorkRow } from './beadsTree'

function row(overrides: Partial<WorkRow> & { id: string }): WorkRow {
  return {
    title: `Title of ${overrides.id}`,
    status: 'open',
    priority: 1,
    blocked: false,
    linked: false,
    projectPath: '/srv/chrote',
    projectName: 'chrote',
    ...overrides,
  }
}

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
const LOOSE = row({ id: 'chrote-lone', type: 'bug', priority: 2, updated: '2026-08-01T00:00:00Z' })
const ACTIVE = row({ id: 'chrote-now', status: 'in_progress', type: 'task', updated: '2026-09-03T00:00:00Z' })
const DEFERRED = row({ id: 'chrote-later', type: 'task', deferUntil: '2099-01-01T00:00:00Z' })

describe('ready and in progress', () => {
  const rows = [CHILD, BLOCKED, DONE, ACTIVE, LOOSE]

  it('offers only unblocked open work as ready, newest first', () => {
    expect(readyRows(rows).map(item => item.id)).toEqual(['chrote-ep.1', 'chrote-lone'])
  })

  it('keeps future deferred work out of ready', () => {
    expect(readyRows([...rows, DEFERRED], Date.parse('2026-09-04T00:00:00Z')).map(item => item.id))
      .toEqual(['chrote-ep.1', 'chrote-lone'])
  })

  it('lists what is claimed separately', () => {
    expect(inProgressRows(rows).map(item => item.id)).toEqual(['chrote-now'])
  })
})
