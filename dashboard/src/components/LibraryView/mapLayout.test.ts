import { describe, expect, it } from 'vitest'
import type { LibraryGraph } from '../../library/libraryApi'
import { LABEL_LINE, LANDMARK_LABELS, layoutMap, neighboursOf, nodeOpacity, placeLabels, withinWindow, type MapNode } from './mapLayout'

const NOW = Date.parse('2026-09-03T12:00:00Z')
const DAY = 86_400_000

function page(path: string, words = 40, daysAgo = 1, candidate = false) {
  return {
    path,
    shelf: path.includes('/') ? path.split('/')[0] : '',
    title: path.replace(/^.*\//, '').replace(/\.md$/, ''),
    words,
    updated: new Date(NOW - daysAgo * DAY).toISOString(),
    candidate,
  }
}

/** Three shelves, a link across two of them, a tag within one. */
function corpus(): LibraryGraph {
  return {
    pages: [
      page('knowledge/alpha.md', 400),
      page('knowledge/beta.md', 20),
      page('preferences/gamma.md', 90),
      page('preferences/delta.md', 60, 90),
      page('telos/epsilon.md', 30, 5, true),
      page('README.md', 10),
    ],
    links: [['knowledge/alpha.md', 'preferences/gamma.md'], ['knowledge/beta.md', 'knowledge/alpha.md']],
    tags: [['preferences/gamma.md', 'preferences/delta.md', 'shared']],
  }
}

describe('layoutMap', () => {
  it('lays the same corpus out the same way every time', () => {
    const first = layoutMap(corpus(), 960, 600, NOW)
    const second = layoutMap(corpus(), 960, 600, NOW)

    expect(second.nodes).toEqual(first.nodes)
    expect(second.clusters).toEqual(first.clusters)
  })

  it('keeps every page inside the frame and under its own shelf label', () => {
    const layout = layoutMap(corpus(), 960, 600, NOW)

    // A page at the root has no shelf, so it has no place on the map.
    expect(layout.nodes.map(node => node.path)).not.toContain('README.md')
    layout.nodes.forEach(node => {
      expect(node.x).toBeGreaterThanOrEqual(0)
      expect(node.x).toBeLessThanOrEqual(960)
      expect(node.y).toBeGreaterThanOrEqual(0)
      expect(node.y).toBeLessThanOrEqual(600)
    })
    expect(layout.clusters.map(cluster => `${cluster.shelf} · ${cluster.count}`)).toEqual([
      'knowledge · 2', 'preferences · 2', 'telos · 1',
    ])
    layout.clusters.forEach(cluster => {
      const members = layout.nodes.filter(node => node.shelf === cluster.shelf)
      members.forEach(member => expect(member.y).toBeGreaterThan(cluster.y))
    })
  })

  it('draws written links solid and shared tags as tag edges', () => {
    const layout = layoutMap(corpus(), 960, 600, NOW)

    expect(layout.edges).toEqual([
      { from: 'knowledge/alpha.md', to: 'preferences/gamma.md', tag: false },
      { from: 'knowledge/beta.md', to: 'knowledge/alpha.md', tag: false },
      { from: 'preferences/gamma.md', to: 'preferences/delta.md', tag: true },
    ])
  })

  it('sizes a page by its words and fades it by its age', () => {
    const layout = layoutMap(corpus(), 960, 600, NOW)
    const byPath = new Map(layout.nodes.map(node => [node.path, node]))

    expect(byPath.get('knowledge/alpha.md')?.r).toBe(9)
    expect(byPath.get('knowledge/beta.md')?.r).toBe(3)
    expect(byPath.get('preferences/delta.md')?.opacity).toBe(0.35)
    expect(byPath.get('knowledge/alpha.md')?.opacity).toBeCloseTo(0.975)
    expect(nodeOpacity('', NOW)).toBe(0.35)
  })
})

// The dive's Neighbours list is this function's only reader with a list to
// show, so what counts as one hop is worth pinning: a written link either
// way, a shared tag, each page once.
describe('neighboursOf', () => {
  it('gathers every page one written link or one shared tag away', () => {
    expect(neighboursOf(corpus(), 'knowledge/alpha.md')).toEqual(['preferences/gamma.md', 'knowledge/beta.md'])
    expect(neighboursOf(corpus(), 'preferences/gamma.md')).toEqual(['knowledge/alpha.md', 'preferences/delta.md'])
    expect(neighboursOf(corpus(), 'telos/epsilon.md')).toEqual([])
  })
})

describe('placeLabels', () => {
  const at = (path: string, x: number, y: number, r = 4, candidate = false): MapNode => ({
    path, shelf: 'knowledge', title: path, x, y, r, opacity: 1, updated: '', candidate,
  })

  it('names the largest accepted pages up to the landmark limit, and every hot page besides', () => {
    const nodes = Array.from({ length: 20 }, (_, index) => at(`page-${index}`, 40, 40 + index * 30, 3 + (index % 7)))
    const hot = new Set(['page-1'])

    const labels = placeLabels(nodes, hot, hot, { landmarks: LANDMARK_LABELS, maxChars: 26 })

    expect(labels.map(label => label.path)).toContain('page-1')
    expect(labels).toHaveLength(LANDMARK_LABELS + 1)
    expect(labels.find(label => label.path === 'page-1')?.primary).toBe(true)
  })

  // The bug the operator saw: pointing at a dot re-placed the landmarks, so
  // text he was reading moved. The landmarks are placed from the layout alone,
  // so no hot set can shift one.
  it('leaves every landmark where it was, whatever the pointer is on', () => {
    const nodes = Array.from({ length: 20 }, (_, index) => at(`page-${index}`, 40, 40 + index * 26, 3 + (index % 7)))
    const options = { landmarks: LANDMARK_LABELS, maxChars: 26 }

    const cold = placeLabels(nodes, new Set(), new Set(), options)
    const placed = (labels: ReturnType<typeof placeLabels>) => labels.map(label => `${label.path}@${label.x},${label.y}`)

    nodes.forEach(node => {
      const hot = new Set([node.path, 'page-3', 'page-17'])
      expect(placed(placeLabels(nodes, hot, hot, options))).toEqual(expect.arrayContaining(placed(cold)))
    })
  })

  // The other half of the same bug: the page under the pointer was named by
  // the readout and by its label at once. Suppressing it keeps its place in
  // the packing, so nothing else moves in exchange.
  it('drops a suppressed name without moving any other', () => {
    const nodes = Array.from({ length: 8 }, (_, index) => at(`page-${index}`, 40, 40 + index * 26, 3 + index))
    const options = { landmarks: 8, maxChars: 26 }

    const all = placeLabels(nodes, new Set(), new Set(), options)
    const without = placeLabels(nodes, new Set(), new Set(), { ...options, suppress: new Set(['page-5']) })

    expect(without.map(label => label.path)).toEqual(all.map(label => label.path).filter(path => path !== 'page-5'))
    without.forEach(label => {
      const before = all.find(other => other.path === label.path)
      expect([label.x, label.y]).toEqual([before?.x, before?.y])
    })
  })

  it('moves a colliding name down one line, and drops a landmark that still collides', () => {
    const nodes = [at('first', 40, 100), at('second', 44, 104), at('third', 42, 102, 4, false)]

    const labels = placeLabels(nodes, new Set(['first']), new Set(['first']), { landmarks: 12, maxChars: 26 })

    const first = labels.find(label => label.path === 'first')
    const second = labels.find(label => label.path === 'second')
    expect(first?.y).toBe(104)
    expect(second?.y).toBe(104 + LABEL_LINE + 4)
    expect(labels.map(label => label.path)).not.toContain('third')
  })

  it('keeps a hot name even where it cannot sit clear', () => {
    const nodes = [at('a', 40, 100), at('b', 40, 100), at('c', 40, 100)]
    const hot = new Set(['a', 'b', 'c'])

    const labels = placeLabels(nodes, hot, new Set(), { landmarks: 0, maxChars: 26 })

    expect(labels.map(label => label.path)).toEqual(['a', 'b', 'c'])
  })

  it('never names a candidate as a landmark', () => {
    const nodes = [at('accepted', 40, 40, 9), at('candidate', 40, 200, 9, true)]

    const labels = placeLabels(nodes, new Set(), new Set(), { landmarks: 12, maxChars: 26 })

    expect(labels.map(label => label.path)).toEqual(['accepted'])
  })

  it('shortens a long title to the label measure', () => {
    const nodes = [at('a-very-long-title-that-runs-past-the-measure-of-a-label', 40, 40)]

    const [label] = placeLabels(nodes, new Set(['a-very-long-title-that-runs-past-the-measure-of-a-label']), new Set(), { landmarks: 0, maxChars: 26 })

    expect(label.text).toHaveLength(26)
    expect(label.text.endsWith('…')).toBe(true)
  })
})

describe('withinWindow', () => {
  const ago = (days: number) => new Date(NOW - days * DAY).toISOString()

  it('holds a page the window reaches back to and drops one it does not', () => {
    expect(withinWindow(ago(0.5), 'day', NOW)).toBe(true)
    expect(withinWindow(ago(2), 'day', NOW)).toBe(false)
    expect(withinWindow(ago(6.9), 'week', NOW)).toBe(true)
    expect(withinWindow(ago(8), 'week', NOW)).toBe(false)
    expect(withinWindow(ago(8), 'month', NOW)).toBe(true)
    expect(withinWindow(ago(40), 'month', NOW)).toBe(false)
  })

  it('keeps the page the window ends on, to the millisecond', () => {
    expect(withinWindow(ago(7), 'week', NOW)).toBe(true)
    expect(withinWindow(new Date(NOW - 7 * DAY - 1).toISOString(), 'week', NOW)).toBe(false)
  })

  it('holds everything under all, including a page git never dated', () => {
    expect(withinWindow('', 'all', NOW)).toBe(true)
    expect(withinWindow(ago(4000), 'all', NOW)).toBe(true)
  })

  it('drops a page with no date, or an unreadable one, from any narrower window', () => {
    expect(withinWindow('', 'month', NOW)).toBe(false)
    expect(withinWindow('not a date', 'week', NOW)).toBe(false)
  })
})
