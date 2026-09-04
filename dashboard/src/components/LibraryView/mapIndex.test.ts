/**
 * The drawing is one element, so nothing under the pointer answers for itself.
 * This is the rule that decides which page was drawn where the pointer is, and
 * it is the whole of the map's hit-testing: if it is wrong, pointing at a dot
 * opens the wrong page or none.
 */

import { describe, expect, it } from 'vitest'
import { buildIndex, hitTest } from './mapIndex'
import type { MapNode } from './mapLayout'

const at = (path: string, x: number, y: number, r = 4): MapNode => ({
  path, shelf: 'knowledge', title: path, x, y, r, words: 40, opacity: 1, updated: '', candidate: false,
})

describe('the map index', () => {
  it('answers with the page the point is inside, and with nothing where none is', () => {
    const index = buildIndex([at('near', 100, 100), at('far', 400, 300)])

    expect(hitTest(index, 100, 100)?.path).toBe('near')
    expect(hitTest(index, 108, 100)?.path).toBe('near')
    expect(hitTest(index, 402, 297)?.path).toBe('far')
    expect(hitTest(index, 250, 200)).toBeNull()
  })

  it('reaches past a small dot so a three-pixel page is still catchable', () => {
    const index = buildIndex([at('small', 100, 100, 3)])

    expect(hitTest(index, 109, 100)?.path).toBe('small')
    expect(hitTest(index, 120, 100)).toBeNull()
  })

  it('gives the nearer page where two reach the same point', () => {
    const index = buildIndex([at('large', 100, 100, 9), at('small', 112, 100, 3)])

    expect(hitTest(index, 111, 100)?.path).toBe('small')
    expect(hitTest(index, 99, 100)?.path).toBe('large')
  })

  // Cells are looked up by one number, and a point at the grid's edge asks for
  // the cells outside it. Those must not fall on another cell's pages.
  it('finds a page at the very corner of the grid, and invents none', () => {
    const index = buildIndex([at('corner', -40, -40), at('other', 500, 500)])

    expect(hitTest(index, -40, -40)?.path).toBe('corner')
    expect(hitTest(index, -300, -300)).toBeNull()
  })

  it('scans a neighbourhood rather than the corpus', () => {
    const nodes = Array.from({ length: 5000 }, (_, index) => at(`page-${index}`, (index % 100) * 12, Math.floor(index / 100) * 12))
    const index = buildIndex(nodes)

    expect(hitTest(index, 12 * 42, 12 * 30)?.path).toBe('page-3042')
    expect(index.cells.size).toBeGreaterThan(1)
  })
})
