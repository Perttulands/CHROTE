import { describe, expect, it } from 'vitest'
import { buildSliceIndex } from './beadsSlice'
import { beadsSky } from './beadsSky'
import type { HostBead } from './hostBeads'

function bead(id: string, overrides: Partial<HostBead> = {}): HostBead {
  return { id, title: `Title of ${id}`, status: 'open', priority: 2, created: 1_790_000_000, ...overrides }
}

describe('the Beads sky', () => {
  const index = buildSliceIndex([{
    path: '/work/app', prefix: 'app', generation: 'g1', pending: false,
    rows: [
      bead('app-1', { type: 'epic', title: 'Ship the reader' }),
      bead('app-1.1', { parent: 'app-1' }),
      bead('app-1.1.1', { parent: 'app-1.1', status: 'closed' }),
      bead('app-1.2', { parent: 'app-1', blockedBy: ['app-1.1'] }),
      bead('app-2', { type: 'bug' }),
    ],
  }], Date.parse('2026-10-21T12:00:00Z'))
  const all = index.rows.map((_, row) => row)
  const sky = beadsSky(index, all)
  const page = (id: string) => sky.graph.pages.find(entry => entry.path === `app:${id}`)!

  it('makes each epic a system named for its title, its finished work the dust', () => {
    expect(page('app-1')).toMatchObject({ subject: 'app/app-1', kind: 'topic', slug: 'app-1' })
    expect(page('app-1.1.1')).toMatchObject({ subject: 'app/app-1', kind: 'source' })
    expect(sky.nameOf('app/app-1')).toBe('Ship the reader')
  })

  it('gathers a Bead filed under nothing as unfiled work of its project', () => {
    expect(page('app-2')).toMatchObject({ subject: 'app/~unfiled', kind: 'bug' })
    expect(sky.nameOf('app/~unfiled')).toBe('Unfiled in app')
  })

  it('draws parent and blocker links, and finds a row by its path', () => {
    expect(sky.graph.links).toContainEqual(['app:app-1.1', 'app:app-1'])
    expect(sky.graph.links).toContainEqual(['app:app-1.2', 'app:app-1.1'])
    expect(sky.rowOf('app:app-2')).toBe(index.rows.findIndex(row => row.id === 'app-2'))
  })
})
