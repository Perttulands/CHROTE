import { describe, expect, it } from 'vitest'
import { buildSliceIndex } from './beadsSlice'
import type { HostBead, HostStore } from './hostBeads'
import { NODE_HEIGHT, NODE_WIDTH, READABLE_SCALE, WHOLE_SCALE, buildWorkGraph, edgePath, familyHead, fitScale, projectColours } from './workGraph'

const NOW = Date.parse('2026-10-21T12:00:00Z')

function bead(id: string, overrides: Partial<HostBead> = {}): HostBead {
  return { id, title: id, status: 'open', priority: 2, created: NOW / 1000 - 86400, ...overrides }
}

function store(path: string, prefix: string, rows: HostBead[]): HostStore {
  return { path, prefix, generation: 'g', pending: false, rows }
}

const index = buildSliceIndex([
  store('/srv/data/context', 'library', [
    bead('library-zvi', { type: 'epic' }),
    bead('library-zvi.1', { parent: 'library-zvi', blockedBy: ['ctx-v8m', 'srv-lbpn'] }),
    bead('library-zvi.2', { parent: 'library-zvi', blockedBy: ['library-zvi.1'] }),
    bead('library-lone'),
  ]),
  store('/srv/context-citadel', 'ctx', [
    bead('ctx-5t7', { status: 'closed' }),
    bead('ctx-v8m', { status: 'closed', blockedBy: ['ctx-5t7'] }),
  ]),
  store('/srv', 'srv', [bead('srv-lbpn'), bead('srv-later', { blockedBy: ['library-zvi.2'] })]),
], NOW)
const row = (id: string) => index.rows.findIndex(item => item.id === id)
const WIDE = { width: 1600, height: 600 }

describe('the family a Bead is drawn in', () => {
  it('is the Bead\'s own children when it has any, its parent\'s otherwise, and itself alone last', () => {
    expect(familyHead(index, row('library-zvi'))).toBe(row('library-zvi'))
    expect(familyHead(index, row('library-zvi.2'))).toBe(row('library-zvi'))
    expect(familyHead(index, row('library-lone'))).toBe(row('library-lone'))
  })

  it('keeps the same drawing as the selection moves between siblings', () => {
    const one = buildWorkGraph(index, row('library-zvi.1'), WIDE)
    const two = buildWorkGraph(index, row('library-zvi.2'), WIDE)
    expect(two.nodes).toEqual(one.nodes)
    expect(two.edges).toEqual(one.edges)
  })

  it('brings in what blocks the family or waits on it from other stores, with its edges', () => {
    const graph = buildWorkGraph(index, row('library-zvi'), WIDE)
    expect(graph.orientation).toBe('across')
    const drawn = graph.nodes.map(node => index.rows[node.row].id)
    expect(drawn.sort()).toEqual(['ctx-v8m', 'library-zvi.1', 'library-zvi.2', 'srv-later', 'srv-lbpn'])
    const at = (id: string) => graph.nodes.find(node => index.rows[node.row].id === id)!
    expect(at('ctx-v8m').outside).toBe(true)
    expect(at('library-zvi.1').outside).toBe(false)
    const edges = graph.edges.map(edge => `${index.rows[graph.nodes[edge.from].row].id}>${index.rows[graph.nodes[edge.to].row].id}`)
    expect(edges.sort()).toEqual(['ctx-v8m>library-zvi.1', 'library-zvi.1>library-zvi.2', 'library-zvi.2>srv-later', 'srv-lbpn>library-zvi.1'])
    // Work runs left to right along its blocking edges.
    expect(at('ctx-v8m').x).toBeLessThan(at('library-zvi.1').x)
    expect(at('library-zvi.1').x).toBeLessThan(at('library-zvi.2').x)
    expect(at('library-zvi.2').x).toBeLessThan(at('srv-later').x)
  })
})

describe('a family in a narrow pane', () => {
  // library-zvi as it stood on 2026-10-21: its children live in four stores,
  // in four waves with three Beads in the widest.
  const kid = (id: string, overrides: Partial<HostBead> = {}) => bead(id, { parent: 'library-zvi', status: 'closed', ...overrides })
  const zvi = buildSliceIndex([
    store('/srv/data/context', 'library', [
      bead('library-zvi', { type: 'epic', status: 'in_progress' }),
      kid('library-zvi.1', { status: 'open', blockedBy: ['ctx-2bn'] }),
    ]),
    store('/srv/context-citadel', 'ctx', [
      kid('ctx-5t7'),
      kid('ctx-xvl', { blockedBy: ['ctx-5t7'] }),
      kid('ctx-v8m', { blockedBy: ['ctx-xvl'] }),
      kid('ctx-2bn', { blockedBy: ['ctx-v8m', 'desk-cih', 'srv-lbpn'] }),
    ]),
    store('/srv/desk', 'desk', [kid('desk-cih', { blockedBy: ['ctx-5t7'] })]),
    store('/srv', 'srv', [kid('srv-lbpn', { blockedBy: ['ctx-5t7'] })]),
  ], NOW)
  const at = (id: string) => zvi.rows.findIndex(item => item.id === id)

  it('runs down the pane when that shows the whole family readable and across would not', () => {
    const pane = { width: 600, height: 300 }
    const graph = buildWorkGraph(zvi, at('library-zvi'), pane)
    expect(graph.orientation).toBe('down')
    expect(fitScale(pane, graph.width, graph.height)).toBeGreaterThanOrEqual(READABLE_SCALE)
    const node = (id: string) => graph.nodes.find(item => zvi.rows[item.row].id === id)!
    // Work runs top to bottom along its blocking edges, and the edges leave a
    // card's bottom for the next card's top.
    expect(node('ctx-5t7').y).toBeLessThan(node('ctx-xvl').y)
    expect(node('ctx-xvl').y).toBeLessThan(node('ctx-v8m').y)
    expect(node('ctx-v8m').y).toBeLessThan(node('ctx-2bn').y)
    const from = node('ctx-5t7')
    const to = node('desk-cih')
    expect(edgePath(from, to, 'down')).toMatch(new RegExp(`^M${from.x + NODE_WIDTH / 2},${from.y + NODE_HEIGHT} `))
    expect(edgePath(from, to, 'down').endsWith(`${to.x + NODE_WIDTH / 2},${to.y}`)).toBe(true)
  })

  it('stays across in a wide pane', () => {
    expect(buildWorkGraph(zvi, at('library-zvi'), { width: 1400, height: 300 }).orientation).toBe('across')
  })
})

describe('a long chain in a short pane', () => {
  // library-zvi on 2026-10-22: six waves, its last two its own children.
  const kid = (id: string, overrides: Partial<HostBead> = {}) => bead(id, { parent: 'library-zvi', status: 'closed', ...overrides })
  const zvi = buildSliceIndex([
    store('/srv/data/context', 'library', [
      bead('library-zvi', { type: 'epic', status: 'in_progress' }),
      kid('library-zvi.1', { blockedBy: ['ctx-2bn'] }),
      kid('library-zvi.2', { status: 'open', blockedBy: ['library-zvi.1'] }),
    ]),
    store('/srv/context-citadel', 'ctx', [
      kid('ctx-5t7'),
      kid('ctx-xvl', { blockedBy: ['ctx-5t7'] }),
      kid('ctx-v8m', { blockedBy: ['ctx-xvl'] }),
      kid('ctx-2bn', { blockedBy: ['ctx-v8m', 'desk-cih', 'srv-lbpn'] }),
    ]),
    store('/srv/desk', 'desk', [kid('desk-cih', { blockedBy: ['ctx-5t7'] })]),
    store('/srv', 'srv', [kid('srv-lbpn', { blockedBy: ['ctx-5t7'] })]),
  ], NOW)
  const at = (id: string) => zvi.rows.findIndex(item => item.id === id)
  const whole = (graph: ReturnType<typeof buildWorkGraph>, pane: { width: number; height: number }) => {
    expect(graph.width * graph.scale).toBeLessThanOrEqual(pane.width)
    expect(graph.height * graph.scale).toBeLessThanOrEqual(pane.height)
    graph.nodes.forEach(node => {
      expect(node.x + NODE_WIDTH).toBeLessThanOrEqual(graph.width)
      expect(node.y + NODE_HEIGHT).toBeLessThanOrEqual(graph.height)
    })
  }

  it('wraps into bands, one under the next, to show the whole chain at full size', () => {
    const pane = { width: 900, height: 300 }
    const graph = buildWorkGraph(zvi, at('library-zvi'), pane)
    expect(graph.orientation).toBe('across')
    expect(graph.scale).toBe(1)
    whole(graph, pane)
    const node = (id: string) => graph.nodes.find(item => zvi.rows[item.row].id === id)!
    expect(new Set(graph.nodes.map(item => item.y >= node('ctx-2bn').y)).size).toBe(2)
    // The edge from one band's last wave to the next band's first leaves the
    // bottom of its card for the top of the next.
    const from = node('ctx-v8m')
    const to = node('ctx-2bn')
    expect(to.y).toBeGreaterThan(from.y + NODE_HEIGHT)
    expect(to.x).toBeLessThan(from.x)
    expect(edgePath(from, to, graph.orientation)).toMatch(new RegExp(`^M${from.x + NODE_WIDTH / 2},${from.y + NODE_HEIGHT} `))
  })

  it('shrinks a little below readable rather than hide part of a small family', () => {
    const pane = { width: 400, height: 230 }
    const graph = buildWorkGraph(zvi, at('library-zvi'), pane)
    expect(graph.nodes).toHaveLength(8)
    expect(graph.scale).toBeGreaterThanOrEqual(WHOLE_SCALE)
    expect(graph.scale).toBeLessThan(READABLE_SCALE)
    whole(graph, pane)
  })

  it('opens readable and pans when even that would not show it whole', () => {
    const graph = buildWorkGraph(zvi, at('library-zvi'), { width: 300, height: 160 })
    expect(graph.scale).toBe(READABLE_SCALE)
    expect(graph.orientation).toBe('across')
  })
})

describe('a big family', () => {
  const children = Array.from({ length: 116 }, (_, i) => bead(`aqg-0gy.${i + 1}`, {
    parent: 'aqg-0gy',
    status: i % 3 === 0 ? 'closed' : 'open',
    blockedBy: i >= 100 ? [`aqg-0gy.${i - 99}`] : undefined,
  }))
  const big = buildSliceIndex([store('/games/agentquest', 'aqg', [bead('aqg-0gy', { type: 'epic' }), ...children])], NOW)

  it('wraps its columns so a hundred children stay readable at full size in a pane', () => {
    const graph = buildWorkGraph(big, 0, { width: 900, height: 640 })
    expect(graph.orientation).toBe('across')
    expect(graph.nodes).toHaveLength(116)
    // Every card lies inside the drawing, and none overlaps another.
    const seen = new Set<string>()
    graph.nodes.forEach(node => {
      expect(node.x + NODE_WIDTH).toBeLessThanOrEqual(graph.width)
      expect(node.y + NODE_HEIGHT).toBeLessThanOrEqual(graph.height)
      const key = `${node.x},${node.y}`
      expect(seen.has(key)).toBe(false)
      seen.add(key)
    })
    // Its columns are as tall as the pane at the scale it opens at, so it pans
    // one way only.
    expect(graph.scale).toBe(READABLE_SCALE)
    expect(graph.height * READABLE_SCALE).toBeLessThanOrEqual(640)
    expect(graph.width).toBeLessThanOrEqual(11 * (NODE_WIDTH + 60))
  })

  it('puts unfinished work first in each column', () => {
    const graph = buildWorkGraph(big, 0, { width: 900, height: 640 })
    const firstColumn = graph.nodes.filter(node => node.x === graph.nodes[0].x).sort((a, b) => a.y - b.y)
    const closedAt = firstColumn.findIndex(node => big.rows[node.row].status === 'closed')
    expect(closedAt).toBeGreaterThan(0)
    expect(firstColumn.slice(closedAt).every(node => big.rows[node.row].status === 'closed')).toBe(true)
  })
})

describe('project colours', () => {
  it('gives each project in one graph its own shelf, the same one every time', () => {
    const first = projectColours(['library', 'ctx', 'srv', 'desk'], 10)
    const again = projectColours(['desk', 'srv', 'ctx', 'library'], 10)
    expect(again).toEqual(first)
    expect(new Set(first.values()).size).toBe(4)
    expect([...first.values()].every(colour => colour >= 0 && colour < 10)).toBe(true)
  })
})
