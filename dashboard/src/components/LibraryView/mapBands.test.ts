/**
 * What the map says at each distance, and where a card goes. The bands decide
 * whether the operator sees shelves, names or cards, and the placement decides
 * whether two cards sit on top of each other; both are arithmetic, so both are
 * read here rather than in a browser.
 */

import { describe, expect, it } from 'vitest'
import { MID_SCALE, NEAR_SCALE, bandAt, cardSize, dotRadius, placeCards } from './mapBands'
import type { MapNode } from './mapLayout'

const at = (path: string, x: number, y: number, r = 4): MapNode => ({
  path, shelf: 'knowledge', title: path, x, y, r, words: 40, opacity: 1, updated: '', createdAt: 0, updatedAt: 0, candidate: false,
})

const describeShort = () => ['a page', 'knowledge · 1 day ago', '40 words']

describe('the zoom bands', () => {
  it('names the band by how far in the map is taken', () => {
    expect(bandAt(1)).toBe('far')
    expect(bandAt(MID_SCALE - 0.01)).toBe('far')
    expect(bandAt(MID_SCALE)).toBe('mid')
    expect(bandAt(NEAR_SCALE - 0.01)).toBe('mid')
    expect(bandAt(NEAR_SCALE)).toBe('near')
    expect(bandAt(8)).toBe('near')
  })

  // The complaint the band answers: zooming in only made the circles bigger.
  it('shrinks a dot with the drawing but never grows it past the landing size', () => {
    const node = at('a', 0, 0, 6)

    expect(dotRadius(node, 0.5)).toBe(3)
    expect(dotRadius(node, 1)).toBe(6)
    expect(dotRadius(node, 3)).toBe(6)
    expect(dotRadius(node, 8)).toBe(6)
  })
})

describe('placeCards', () => {
  const options = (extra: Partial<Parameters<typeof placeCards>[1]> = {}) => ({
    transform: { x: 0, y: 0, scale: 1 },
    width: 800,
    height: 600,
    describe: describeShort,
    ...extra,
  })

  it('puts a card beside its dot and inside the box', () => {
    const [card] = placeCards([at('a', 100, 100)], options())
    const size = cardSize(describeShort())

    expect(card.path).toBe('a')
    expect(card.x).toBeGreaterThan(100)
    expect(card.y + card.height).toBeLessThanOrEqual(600)
    expect([card.width, card.height]).toEqual([size.width, size.height])
  })

  it('turns a card at the right edge back on itself rather than off the box', () => {
    const [card] = placeCards([at('a', 780, 300)], options())

    expect(card.x).toBeLessThan(780)
    expect(card.x + card.width).toBeLessThanOrEqual(800)
  })

  it('leaves out a page the box does not show', () => {
    const cards = placeCards([at('here', 100, 100), at('away', 100, 2000)], options())

    expect(cards.map(card => card.path)).toEqual(['here'])
  })

  it('drops a card that would sit on one already placed, largest page first', () => {
    const cards = placeCards([at('small', 100, 100, 3), at('large', 104, 104, 9)], options())

    expect(cards.map(card => card.path)).toEqual(['large'])
  })

  it('names no page the readout is already naming', () => {
    const cards = placeCards([at('a', 100, 100), at('b', 100, 300)], options({ suppress: new Set(['a']) }))

    expect(cards.map(card => card.path)).toEqual(['b'])
  })

  it('draws no more cards than its budget', () => {
    const nodes = Array.from({ length: 40 }, (_, index) => at(`page-${index}`, 60 + (index % 8) * 90, 40 + Math.floor(index / 8) * 100))

    expect(placeCards(nodes, options({ maxCards: 5 }))).toHaveLength(5)
  })

  it('follows the drawing when it is moved', () => {
    const moved = placeCards([at('a', 100, 100)], options({ transform: { x: 40, y: -20, scale: 2 } }))

    expect(moved[0].x).toBeGreaterThan(240)
    expect(moved[0].y).toBeCloseTo(180 - cardSize(describeShort()).height / 2)
  })
})
