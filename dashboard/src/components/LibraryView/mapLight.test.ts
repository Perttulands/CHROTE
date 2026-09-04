/**
 * What the pointer lights. The rule decides what the operator sees when he
 * points at a page — one hop bright, two hops dim, the rest faded — and what a
 * shelf's name asks for, so it is read here rather than in a browser.
 */

import { describe, expect, it } from 'vitest'
import { AT_REST, adjacencyOf, alphaOf, depthsFrom, edgeLight, glows, lightOf } from './mapLight'
import type { LibraryGraph } from '../../library/libraryApi'

/** A chain: a — b — c — d, with e off on its own, and a shared tag a — e. */
const graph: LibraryGraph = {
  pages: [],
  links: [['a', 'b'], ['b', 'c'], ['c', 'd']],
  tags: [['a', 'e', 'shared']],
}

const at = (path: string, shelf = 'knowledge') => ({ path, shelf })
const lighting = (from: string[]) => ({ depths: depthsFrom(adjacencyOf(graph), from), solo: null })

describe('the light around a page', () => {
  it('counts the hops from what the pointer is on, and stops at two', () => {
    const depths = depthsFrom(adjacencyOf(graph), ['a'])

    expect(depths.get('a')).toBe(0)
    expect(depths.get('b')).toBe(1)
    expect(depths.get('e')).toBe(1)
    expect(depths.get('c')).toBe(2)
    expect(depths.has('d')).toBe(false)
  })

  it('counts a page once, at the shortest way to it', () => {
    const depths = depthsFrom(adjacencyOf({ ...graph, links: [['a', 'b'], ['a', 'c'], ['b', 'c']] }), ['a'])

    expect(depths.get('c')).toBe(0 + 1)
  })

  it('draws the neighbourhood bright, its edge dim, and the rest faded', () => {
    const lit = lighting(['a'])

    expect([lightOf(at('a'), lit), lightOf(at('b'), lit), lightOf(at('c'), lit), lightOf(at('d'), lit)])
      .toEqual([0, 1, 2, 'out'])
    expect([glows(0), glows(1), glows(2), glows('out')]).toEqual([true, true, false, false])
    expect(alphaOf(2)).toBeLessThan(alphaOf(1) as number)
    expect(alphaOf('out')).toBeLessThan(alphaOf(2) as number)
  })

  it('leaves every page to its own age when nothing is pointed at', () => {
    expect(lightOf(at('a'), AT_REST)).toBeNull()
    expect(alphaOf(null)).toBeNull()
  })

  it('draws a hairline as its brighter end, so a link out is followed', () => {
    const lit = lighting(['a'])

    expect(edgeLight(0, 1, lit)).toBe(0)
    expect(edgeLight(2, 'out', lit)).toBe(2)
    expect(edgeLight('out', 1, lit)).toBe(1)
  })
})

describe('a shelf pointed at', () => {
  const solo = { depths: null, solo: 'knowledge' }

  it('lights its own pages and dims every other shelf', () => {
    expect(lightOf(at('a', 'knowledge'), solo)).toBeNull()
    expect(lightOf(at('b', 'telos'), solo)).toBe('out')
  })

  // A shelf asked about is the shelf, not what it reaches: a link that leaves
  // it is not the answer, so it is dimmed with the rest.
  it('keeps only the hairlines that stay inside it', () => {
    expect(edgeLight(null, null, solo)).toBeNull()
    expect(edgeLight(null, 'out', solo)).toBe('out')
  })
})
