/**
 * The work graph beside the table: the family the selected Bead belongs to.
 *
 * The family's head is the selected Bead when it has children, otherwise its
 * parent, otherwise the Bead itself; so selecting one child after another keeps
 * the same picture and only moves the highlight. The nodes are the head's
 * children (or the lone Bead), plus every Bead outside the family that blocks
 * one of them or waits on one, in any store. Edges are the blocking edges among
 * the nodes. Layout runs in waves by longest blocking path, left to right; a
 * wave taller than the pane wraps into sub-columns as tall as the pane,
 * unfinished work first, so an epic of a hundred children opens legible, at
 * no less than READABLE_SCALE, and pans sideways rather than shrinking to fit.
 */

import { isBeadClosed } from './beadStatus'
import type { SliceIndex } from './beadsSlice'

export const NODE_WIDTH = 196
export const NODE_HEIGHT = 40
/** The smallest scale a graph opens at: below it the words on a card stop being read. */
export const READABLE_SCALE = 0.85
const GAP_X = 44
const GAP_Y = 10
const WRAP_GAP_X = 16
const PAD = 16
/** Beads outside the family that one graph draws at most. */
const MAX_RING = 48

export interface GraphNode {
  row: number
  x: number
  y: number
  /** Outside the family: here because it blocks a member or waits on one. */
  outside: boolean
}

export interface GraphEdge {
  from: number
  to: number
}

export interface WorkGraph {
  /** The family's head, drawn as the pane's title. */
  head: number
  /** The head's own parent, offered as the way up. */
  headParent: number
  nodes: GraphNode[]
  /** Node indices: `from` blocks `to`. */
  edges: GraphEdge[]
  width: number
  height: number
  /** Outside Beads left out past the cap. */
  omitted: number
}

/** The head of the family a Bead is shown in. */
export function familyHead(index: SliceIndex, selected: number): number {
  if (index.children[selected].length > 0) return selected
  const parent = index.parent[selected]
  return parent >= 0 ? parent : selected
}

function naturalOrder(index: SliceIndex) {
  const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' })
  return (a: number, b: number) => {
    const doneA = isBeadClosed(index.rows[a].status) ? 1 : 0
    const doneB = isBeadClosed(index.rows[b].status) ? 1 : 0
    return doneA - doneB || collator.compare(index.rows[a].id, index.rows[b].id)
  }
}

/**
 * The family of `selected`, laid out for a pane `paneHeight` tall at scale 1.
 */
export function buildWorkGraph(index: SliceIndex, selected: number, paneHeight: number): WorkGraph {
  const head = familyHead(index, selected)
  const members = index.children[head].length > 0 ? [...index.children[head]] : [head]
  const inFamily = new Set(members)
  const ring: number[] = []
  const inRing = new Set<number>()
  let omitted = 0
  const order = naturalOrder(index)
  // Unfinished members first, so a cap drops the context of finished work.
  ;[...members].sort(order).forEach(member => {
    for (const other of [...index.blockers[member], ...index.dependents[member]]) {
      if (inFamily.has(other) || inRing.has(other) || other === head) continue
      if (ring.length >= MAX_RING) { omitted += 1; continue }
      inRing.add(other)
      ring.push(other)
    }
  })
  const rows = [...members, ...ring]
  const nodeOf = new Map(rows.map((row, i) => [row, i]))
  const edges: GraphEdge[] = []
  rows.forEach((row, to) => {
    index.blockers[row].forEach(blocker => {
      const from = nodeOf.get(blocker)
      if (from !== undefined) edges.push({ from, to })
    })
  })

  // Longest blocking path from any source; a cycle's back edge is ignored.
  const incoming: number[][] = rows.map(() => [])
  edges.forEach(edge => incoming[edge.to].push(edge.from))
  const wave = new Int32Array(rows.length).fill(-1)
  const visiting = new Uint8Array(rows.length)
  const depth = (node: number): number => {
    if (wave[node] >= 0) return wave[node]
    if (visiting[node]) return 0
    visiting[node] = 1
    let deepest = 0
    incoming[node].forEach(from => { deepest = Math.max(deepest, depth(from) + 1) })
    visiting[node] = 0
    wave[node] = deepest
    return deepest
  }
  rows.forEach((_, node) => depth(node))

  const waves: number[][] = []
  rows.forEach((_, node) => {
    const w = wave[node]
    ;(waves[w] ??= []).push(node)
  })
  // As many cards as the pane holds, top to bottom, at the smallest scale a
  // graph opens at.
  const perColumn = Math.max(4, Math.floor((paneHeight / READABLE_SCALE - 2 * PAD + GAP_Y) / (NODE_HEIGHT + GAP_Y)))
  const nodes: GraphNode[] = rows.map(row => ({ row, x: 0, y: 0, outside: inRing.has(row) }))
  let x = PAD
  let height = 0
  waves.forEach(column => {
    if (!column) return
    column.sort((a, b) => order(rows[a], rows[b]))
    const columns = Math.ceil(column.length / perColumn)
    column.forEach((node, i) => {
      const c = Math.floor(i / perColumn)
      const r = i % perColumn
      nodes[node].x = x + c * (NODE_WIDTH + WRAP_GAP_X)
      nodes[node].y = PAD + r * (NODE_HEIGHT + GAP_Y)
      height = Math.max(height, nodes[node].y + NODE_HEIGHT + PAD)
    })
    x += columns * (NODE_WIDTH + WRAP_GAP_X) - WRAP_GAP_X + GAP_X
  })
  return {
    head,
    headParent: index.parent[head],
    nodes,
    edges,
    width: x - GAP_X + PAD,
    height,
    omitted,
  }
}

/** The curve from a blocker's right edge to the left edge of the Bead it blocks. */
export function edgePath(from: GraphNode, to: GraphNode): string {
  const x1 = from.x + NODE_WIDTH
  const y1 = from.y + NODE_HEIGHT / 2
  const x2 = to.x
  const y2 = to.y + NODE_HEIGHT / 2
  const bend = Math.max(16, Math.abs(x2 - x1) / 2)
  return `M${x1},${y1} C${x1 + bend},${y1} ${x2 - bend},${y2} ${x2},${y2}`
}

function hash(text: string): number {
  let value = 2166136261
  for (let i = 0; i < text.length; i += 1) {
    value ^= text.charCodeAt(i)
    value = Math.imul(value, 16777619)
  }
  return value >>> 0
}

/**
 * Shelf colours for the projects one graph shows: each project's own colour
 * by its key, moved to the next free one only when another project in the
 * same graph already holds it.
 */
export function projectColours(keys: readonly string[], shelves: number): Map<string, number> {
  const colours = new Map<string, number>()
  const taken = new Set<number>()
  ;[...new Set(keys)].sort().forEach(key => {
    let colour = hash(key) % shelves
    for (let probe = 0; probe < shelves && taken.has(colour); probe += 1) colour = (colour + 1) % shelves
    taken.add(colour)
    colours.set(key, colour)
  })
  return colours
}
