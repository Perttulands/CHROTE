import { describe, expect, it } from 'vitest'
import { flowMembers, flowNeighbour, layoutFlow, NODE_WIDTH, PADDING } from './flowLayout'
import type { WorkRow } from './beadsTree'

function row(id: string, extra: Partial<WorkRow> = {}): WorkRow {
  return {
    id,
    title: id,
    status: 'open',
    type: 'task',
    priority: 2,
    blocked: false,
    projectPath: '/code/p',
    projectName: 'p',
    ...extra,
  }
}

/**
 * An epic with two independent chains that meet: a1 → a2 and b1 → b2, and a
 * join that waits for both ends. Three waves, two lanes in the first two.
 */
const epic = row('e', { type: 'epic' })
const twoChains: WorkRow[] = [
  epic,
  row('a1', { parent: 'e' }),
  row('a2', { parent: 'e', blockedBy: ['a1'], blocked: true }),
  row('b1', { parent: 'e' }),
  row('b2', { parent: 'e', blockedBy: ['b1'], blocked: true }),
  row('j', { parent: 'e', blockedBy: ['a2', 'b2'], blocked: true }),
]

const waveOf = (rows: WorkRow[], id: string) =>
  layoutFlow(rows, epic).nodes.find(node => node.row.id === id)?.wave

describe('layoutFlow', () => {
  it('puts a Bead one wave past the deepest Bead blocking it', () => {
    expect(waveOf(twoChains, 'a1')).toBe(0)
    expect(waveOf(twoChains, 'a2')).toBe(1)
    expect(waveOf(twoChains, 'j')).toBe(2)
  })

  it('stacks work that can run together in one column', () => {
    const graph = layoutFlow(twoChains, epic)
    const first = graph.nodes.filter(node => node.wave === 0)
    expect(first.map(node => node.row.id)).toEqual(['a1', 'b1'])
    expect(first.map(node => node.lane)).toEqual([0, 1])
    expect(new Set(first.map(node => node.x)).size).toBe(1)
    expect(graph.waves).toBe(3)
  })

  it('draws an edge for every blocking pair it holds, and none for one it does not', () => {
    const rows = [...twoChains, row('c', { parent: 'e', blockedBy: ['elsewhere-9'], blocked: true })]
    const graph = layoutFlow(rows, epic)
    const named = (key: string) => key.split('\u0000')[1]
    expect(graph.edges.map(edge => `${named(edge.from)} to ${named(edge.to)}`).sort())
      .toEqual(['a1 to a2', 'a2 to j', 'b1 to b2', 'b2 to j'])
    // A blocker outside the epic leaves the Bead ready in this drawing.
    expect(waveOf(rows, 'c')).toBe(0)
  })

  it('breaks the edge that closes a loop rather than looping on it', () => {
    const cyclic = [
      epic,
      row('x', { parent: 'e', blockedBy: ['z'], blocked: true }),
      row('y', { parent: 'e', blockedBy: ['x'], blocked: true }),
      row('z', { parent: 'e', blockedBy: ['y'], blocked: true }),
    ]
    const graph = layoutFlow(cyclic, epic)
    expect(graph.nodes).toHaveLength(3)
    expect(graph.edges).toHaveLength(3)
    expect(graph.edges.filter(edge => edge.back)).toHaveLength(1)
    expect(graph.nodes.map(node => node.wave).sort()).toEqual([0, 1, 2])
  })

  it('lays the same rows out the same way whatever order they arrive in', () => {
    const shuffled = [twoChains[3], twoChains[5], twoChains[0], twoChains[4], twoChains[2], twoChains[1]]
    expect(layoutFlow(shuffled, epic)).toEqual(layoutFlow(twoChains, epic))
  })

  it('holds a sub-epic\'s children in a band, and gives the epic itself none', () => {
    const nested = [
      epic,
      row('s', { parent: 'e', type: 'epic' }),
      row('s1', { parent: 's' }),
      row('s2', { parent: 's', blockedBy: ['s1'], blocked: true }),
    ]
    const graph = layoutFlow(nested, epic)
    expect(graph.nodes.map(node => node.row.id)).toEqual(['s', 's1', 's2'])
    expect(graph.bands.map(band => band.key)).toEqual(['s'])
  })

  it('follows the parent chain to every generation and stops at the epic', () => {
    const outside = row('other', { parent: 'nowhere' })
    expect(flowMembers([...twoChains, outside], epic).map(member => member.id))
      .toEqual(['a1', 'a2', 'b1', 'b2', 'j'])
  })
})

describe('flowNeighbour', () => {
  const graph = layoutFlow(twoChains, epic)
  const at = (id: string) => graph.nodes.find(node => node.row.id === id)?.key as string

  it('travels the column and the waves, and stops at the edges', () => {
    expect(flowNeighbour(graph, at('a1'), 'down')?.row.id).toBe('b1')
    expect(flowNeighbour(graph, at('a1'), 'up')).toBeNull()
    expect(flowNeighbour(graph, at('a1'), 'right')?.row.id).toBe('a2')
    expect(flowNeighbour(graph, at('j'), 'right')).toBeNull()
  })

  it('takes the last Bead of a shorter column rather than nothing', () => {
    expect(flowNeighbour(graph, at('b2'), 'right')?.row.id).toBe('j')
    expect(flowNeighbour(graph, at('j'), 'left')?.row.id).toBe('a2')
  })

  it('leaves a column a fixed width apart, so a wave is one stride', () => {
    const a2 = graph.nodes.find(node => node.row.id === 'a2')
    expect(a2?.x).toBe(PADDING + NODE_WIDTH + 64)
  })
})
