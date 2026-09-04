import { describe, expect, it } from 'vitest'
import { shelfHues, shelfOrder } from './mapShelves'

const PALETTE = ['#aa0000', '#00aa00', '#0000aa']

describe('shelfOrder', () => {
  it('names each shelf once, in the one order every surface reads them in', () => {
    expect(shelfOrder(['telos', 'identity', 'telos', 'knowledge'])).toEqual(['identity', 'knowledge', 'telos'])
  })

  // A page at the root of the corpus is on no shelf, and no shelf takes a hue
  // for it.
  it('leaves out the pages that sit on no shelf', () => {
    expect(shelfOrder(['', 'telos', ''])).toEqual(['telos'])
  })
})

describe('shelfHues', () => {
  it('hands out hues in shelf order, so a corpus keeps its colours', () => {
    const hues = shelfHues(['telos', 'identity', 'knowledge'], PALETTE)
    expect(hues.get('identity')).toBe('#aa0000')
    expect(hues.get('knowledge')).toBe('#00aa00')
    expect(hues.get('telos')).toBe('#0000aa')
  })

  // The whole point of ordering by name: a shelf added later must not take the
  // colour another shelf has already been learned by.
  it('gives a shelf the same hue however the pages were listed', () => {
    const one = shelfHues(['knowledge', 'identity'], PALETTE)
    const again = shelfHues(['identity', 'knowledge', 'identity'], PALETTE)
    expect(again.get('identity')).toBe(one.get('identity'))
    expect(again.get('knowledge')).toBe(one.get('knowledge'))
  })

  it('repeats from the start when the corpus has more shelves than the theme has hues', () => {
    const hues = shelfHues(['a', 'b', 'c', 'd'], PALETTE)
    expect(hues.get('d')).toBe(hues.get('a'))
  })

  // A theme that names no hues leaves the map in the greys it was drawn in.
  it('gives no hue at all when the theme names none', () => {
    expect(shelfHues(['a', 'b'], []).size).toBe(0)
  })
})
