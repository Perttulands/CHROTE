/**
 * The work graph beside the table: the family the selected Bead belongs to.
 *
 * The family's head is the selected Bead when it has children, otherwise its
 * parent, otherwise the Bead itself; so selecting one child after another keeps
 * the same picture and only moves the highlight. The nodes are the head's
 * children (or the lone Bead), plus every Bead outside the family that blocks
 * one of them or waits on one, in any store. Edges are the blocking edges among
 * the nodes. Layout runs in waves by longest blocking path, across the pane,
 * across in two or more bands one under the next (as a long line of text
 * wraps), or down it: whichever draws the family largest in the pane it has,
 * so a long chain of a few Beads is seen whole in a pane of any shape. A family
 * no shape shows whole at READABLE_SCALE opens whole down to WHOLE_SCALE, since
 * every card in view beats a readable part; each card's words are a hover or a
 * click away. A wave longer than the pane wraps into sub-columns as tall as the
 * pane (or sub-rows as wide), unfinished work first, so an epic of a hundred
 * children opens legible, at no less than READABLE_SCALE, and pans rather than
 * shrinking further.
 */

import { isBeadClosed } from './beadStatus'
import type { SliceIndex } from './beadsSlice'

export const NODE_WIDTH = 196
export const NODE_HEIGHT = 40
/** The smallest scale a graph opens at: below it the words on a card stop being read. */
export const READABLE_SCALE = 0.85
/** The smallest scale a family is shrunk to so that it is seen whole. */
export const WHOLE_SCALE = 0.7
const GAP_X = 44
const GAP_Y = 10
/** Between waves drawn down the pane, and between bands: room for an edge to turn. */
const GAP_DOWN = 30
/** The most bands a family is wrapped into. */
const MOST_BANDS = 4
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

/** Waves left to right, or top to bottom. */
export type GraphOrientation = 'across' | 'down'

export interface WorkGraph {
  orientation: GraphOrientation
  /** The scale the family opens at in the pane it was laid out for. */
  scale: number
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

/** How large a drawing `width` by `height` can be shown whole in the pane. */
export function fitScale(pane: { width: number; height: number }, width: number, height: number): number {
  return Math.min(1, pane.width / Math.max(1, width), pane.height / Math.max(1, height))
}

/**
 * The family of `selected`, laid out at scale 1 for a pane of the given size.
 */
export function buildWorkGraph(index: SliceIndex, selected: number, pane: { width: number; height: number }): WorkGraph {
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
  const columns = waves.filter(Boolean).map(column => column.sort((a, b) => order(rows[a], rows[b])))
  const across = placeAcross(columns, pane.height, 1)
  const down = placeDown(columns, pane.width)
  // Another shape only when it draws the whole family readable and larger
  // than one band across does. When none is readable whole, whichever shape
  // is largest, if it is whole at WHOLE_SCALE; an epic too big for any keeps
  // the one band at READABLE_SCALE, which pans sideways.
  const alternatives = [down]
  for (let bands = 2; bands <= Math.min(MOST_BANDS, columns.length); bands += 1) alternatives.push(placeAcross(columns, pane.height, bands))
  const pick = (floor: number, from: Placement) => {
    let chosen = from
    let best = fitScale(pane, from.width, from.height)
    alternatives.forEach(candidate => {
      const fit = fitScale(pane, candidate.width, candidate.height)
      if (fit >= floor && fit > best + 0.01) { chosen = candidate; best = fit }
    })
    return { chosen, best }
  }
  let { chosen, best } = pick(READABLE_SCALE, across)
  if (best < READABLE_SCALE) ({ chosen, best } = pick(WHOLE_SCALE, across))
  return {
    orientation: chosen === down ? 'down' : 'across',
    scale: best >= WHOLE_SCALE ? best : READABLE_SCALE,
    head,
    headParent: index.parent[head],
    nodes: rows.map((row, node) => ({ row, x: chosen.x[node], y: chosen.y[node], outside: inRing.has(row) })),
    edges,
    width: chosen.width,
    height: chosen.height,
    omitted,
  }
}

interface Placement {
  x: Float64Array
  y: Float64Array
  width: number
  height: number
}

function placement(count: number): Placement {
  return { x: new Float64Array(count), y: new Float64Array(count), width: 0, height: 0 }
}

/**
 * Waves as columns, each wrapping into sub-columns as tall as the pane, in
 * `bands` bands of about equal width, each under the one before.
 */
function placeAcross(waves: readonly (readonly number[])[], paneHeight: number, bands: number): Placement {
  // As many cards as the pane holds, top to bottom, at the smallest scale a
  // graph opens at.
  const perColumn = Math.max(4, Math.floor((paneHeight / READABLE_SCALE - 2 * PAD + GAP_Y) / (NODE_HEIGHT + GAP_Y)))
  const span = (column: readonly number[]) => Math.ceil(column.length / perColumn) * (NODE_WIDTH + WRAP_GAP_X) - WRAP_GAP_X
  const target = (waves.reduce((sum, column) => sum + span(column), 0) + (waves.length - 1) * GAP_X) / bands
  const placed = placement(waves.reduce((sum, column) => sum + column.length, 0))
  let band = 0
  let x = PAD
  let top = PAD
  let bottom = PAD
  waves.forEach((column, w) => {
    // A new band when this one is full, while bands and waves remain for it.
    if (x > PAD && band < bands - 1 && x - PAD + span(column) > target + 1 && waves.length - w >= bands - band - 1) {
      band += 1
      x = PAD
      top = bottom + GAP_DOWN
    }
    column.forEach((node, i) => {
      placed.x[node] = x + Math.floor(i / perColumn) * (NODE_WIDTH + WRAP_GAP_X)
      placed.y[node] = top + (i % perColumn) * (NODE_HEIGHT + GAP_Y)
      bottom = Math.max(bottom, placed.y[node] + NODE_HEIGHT)
    })
    x += span(column) + GAP_X
    placed.width = Math.max(placed.width, x - GAP_X + PAD)
  })
  placed.height = bottom + PAD
  return placed
}

/** Waves as rows, each wrapping into sub-rows as wide as the pane, centred. */
function placeDown(waves: readonly (readonly number[])[], paneWidth: number): Placement {
  const perRow = Math.max(2, Math.floor((paneWidth / READABLE_SCALE - 2 * PAD + WRAP_GAP_X) / (NODE_WIDTH + WRAP_GAP_X)))
  const placed = placement(waves.reduce((sum, row) => sum + row.length, 0))
  const widest = Math.min(perRow, Math.max(1, ...waves.map(row => row.length)))
  const span = (count: number) => count * NODE_WIDTH + (count - 1) * WRAP_GAP_X
  let y = PAD
  waves.forEach(row => {
    const lines = Math.ceil(row.length / perRow)
    row.forEach((node, i) => {
      const line = Math.floor(i / perRow)
      const inLine = Math.min(perRow, row.length - line * perRow)
      placed.x[node] = PAD + (span(widest) - span(inLine)) / 2 + (i % perRow) * (NODE_WIDTH + WRAP_GAP_X)
      placed.y[node] = y + line * (NODE_HEIGHT + GAP_Y)
    })
    y += lines * (NODE_HEIGHT + GAP_Y) - GAP_Y + GAP_DOWN
  })
  placed.width = span(widest) + 2 * PAD
  placed.height = y - GAP_DOWN + PAD
  return placed
}

/**
 * The curve from a blocker to the Bead it blocks: right edge to left edge
 * across the pane, bottom edge to top edge down it, and down too from the end
 * of one band to the next band's start.
 */
export function edgePath(from: GraphNode, to: GraphNode, orientation: GraphOrientation = 'across'): string {
  const nextBand = to.y >= from.y + NODE_HEIGHT && to.x < from.x + NODE_WIDTH
  if (orientation === 'down' || nextBand) {
    const x1 = from.x + NODE_WIDTH / 2
    const y1 = from.y + NODE_HEIGHT
    const x2 = to.x + NODE_WIDTH / 2
    const y2 = to.y
    const bend = Math.max(12, Math.abs(y2 - y1) / 2)
    return `M${x1},${y1} C${x1},${y1 + bend} ${x2},${y2 - bend} ${x2},${y2}`
  }
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
