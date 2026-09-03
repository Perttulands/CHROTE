import { afterEach, describe, expect, it } from 'vitest'
import {
  FALLBACK_BEAD_PREFIXES,
  beadPrefixes,
  beadProjectPath,
  findBeadIds,
  resetBeadProjectsForTest,
  setBeadProjects,
} from './beadIds'

afterEach(() => {
  resetBeadProjectsForTest()
})

describe('Bead ids in written text', () => {
  it('finds the shapes bd writes, wherever they sit in the line', () => {
    const line = 'Bead: chrote-5grx.15 blocked by ctx-t4ak, see chrote-abc.1.2'
    expect(findBeadIds(line)).toEqual([
      { id: 'chrote-5grx.15', index: 6 },
      { id: 'ctx-t4ak', index: 32 },
      { id: 'chrote-abc.1.2', index: 46 },
    ])
  })

  it('leaves alone what only looks like an id', () => {
    expect(findBeadIds('mychrote-5grx and chrote-toolongtail and other-5grx')).toEqual([])
  })

  it('matches the prefixes of the configured projects once they are known', () => {
    expect(beadPrefixes()).toEqual(FALLBACK_BEAD_PREFIXES)
    setBeadProjects([{ name: 'work', path: '/code/work', beadsPath: '/code/work/.beads', prefix: 'work' }])
    expect(beadPrefixes()).toEqual(['work'])
    expect(findBeadIds('work-9ab and chrote-5grx')).toEqual([{ id: 'work-9ab', index: 0 }])
  })

  it('keeps the fallback prefixes when no project reports one', () => {
    setBeadProjects([{ name: 'work', path: '/code/work', beadsPath: '/code/work/.beads' }])
    expect(beadPrefixes()).toEqual(FALLBACK_BEAD_PREFIXES)
  })

  it('resolves an id to the store that owns its prefix', () => {
    setBeadProjects([
      { name: 'srv', path: '/srv', beadsPath: '/srv/.beads', prefix: 'ctx' },
      { name: 'chrote', path: '/srv/chrote', beadsPath: '/srv/chrote/.beads', prefix: 'chrote' },
    ])
    expect(beadProjectPath('chrote-5grx.15')).toBe('/srv/chrote')
    expect(beadProjectPath('ctx-t4ak')).toBe('/srv')
    expect(beadProjectPath('other-abc')).toBeNull()
  })
})
