import { describe, expect, it } from 'vitest'
import { fuzzyScore, lastSegments, rankByFuzzy } from './fuzzy'

describe('fuzzy scoring', () => {
  it('finds every word of the query or nothing', () => {
    expect(fuzzyScore('rep VSK', 'repos/VSK-Zone')).toBeGreaterThan(0)
    expect(fuzzyScore('rep VSK', 'repos/notes')).toBe(0)
    expect(fuzzyScore('xyz', 'srv/chrote')).toBe(0)
    expect(fuzzyScore('', 'anything')).toBe(1)
  })

  it('prefers letters that open words and run unbroken', () => {
    expect(fuzzyScore('chrote', 'srv/chrote')).toBeGreaterThan(fuzzyScore('chrote', 'srv/chroma-tester'))
    expect(fuzzyScore('sk', 'srv/kiosk')).toBeGreaterThan(fuzzyScore('sk', 'srv/klub'))
    expect(fuzzyScore('vz', 'repos/VSK-Zone')).toBeGreaterThan(fuzzyScore('vz', 'repos/velvetzoo'))
  })

  it('ranks best first and keeps the incoming order between equals', () => {
    const paths = ['/srv/context-citadel', '/home/perttu/repos/VSK-Zone', '/srv/chrote', '/home/perttu/repos/vsk-notes']
    expect(rankByFuzzy('rep VSK', paths, path => lastSegments(path)))
      .toEqual(['/home/perttu/repos/VSK-Zone', '/home/perttu/repos/vsk-notes'])
    expect(rankByFuzzy('', paths, path => path)).toEqual(paths)
  })

  it('reads the last two segments of a path', () => {
    expect(lastSegments('/home/perttu/repos/VSK-Zone')).toBe('repos/VSK-Zone')
    expect(lastSegments('/srv/')).toBe('srv')
    expect(lastSegments('/')).toBe('')
  })
})
