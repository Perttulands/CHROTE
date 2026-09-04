/**
 * Finding the page under the pointer without asking every page.
 *
 * The drawing is a canvas, so nothing under the pointer answers for itself:
 * the map has to work out what was drawn there. A uniform grid over the
 * layout's own coordinates is enough — the pages are spread across the frame
 * by the clustering, not piled in one cell — and it turns a scan of the whole
 * corpus into a scan of nine cells. Pure arithmetic, no DOM, so the rule can
 * be read at the size the corpus will reach rather than the size it has.
 */

import type { MapNode } from './mapLayout'

/** The side of one cell, in the layout's coordinates. */
const CELL = 64

/** How far outside its own radius a page still answers the pointer. */
const REACH = 6

/** The smallest a page's answer may be, so a three-pixel dot is catchable. */
const MIN_REACH = 10

export interface MapIndex {
  /** The nodes this index was built over, in the order it was given them. */
  nodes: readonly MapNode[]
  cells: Map<number, number[]>
  left: number
  top: number
}

/**
 * One number per cell. Columns are spaced far enough apart that a neighbour
 * asked for outside the grid — column -1, row -1 — cannot land on another
 * cell's key.
 */
const COLUMN_STRIDE = 1 << 16

function key(column: number, row: number): number {
  return column * COLUMN_STRIDE + row
}

/** The radius at which a page answers the pointer, in layout coordinates. */
export function reachOf(node: MapNode): number {
  return Math.max(node.r + REACH, MIN_REACH)
}

export function buildIndex(nodes: readonly MapNode[]): MapIndex {
  let left = 0
  let top = 0
  if (nodes.length > 0) {
    left = Infinity
    top = Infinity
    nodes.forEach(node => {
      left = Math.min(left, node.x)
      top = Math.min(top, node.y)
    })
  }
  const cells = new Map<number, number[]>()
  nodes.forEach((node, position) => {
    const column = Math.floor((node.x - left) / CELL)
    const row = Math.floor((node.y - top) / CELL)
    const at = key(column, row)
    const bucket = cells.get(at)
    if (bucket) bucket.push(position)
    else cells.set(at, [position])
  })
  return { nodes, cells, left, top }
}

/**
 * The page at a point of the layout's coordinates, or null. Where two pages
 * both reach the point the nearer one answers, so a small dot drawn over a
 * large one is still reachable.
 */
export function hitTest(index: MapIndex, x: number, y: number): MapNode | null {
  const column = Math.floor((x - index.left) / CELL)
  const row = Math.floor((y - index.top) / CELL)
  let best: MapNode | null = null
  let bestDistance = Infinity
  for (let dc = -1; dc <= 1; dc++) {
    for (let dr = -1; dr <= 1; dr++) {
      const bucket = index.cells.get(key(column + dc, row + dr))
      if (!bucket) continue
      for (const position of bucket) {
        const node = index.nodes[position]
        const distance = Math.hypot(node.x - x, node.y - y)
        const reach = reachOf(node)
        if (distance <= reach && distance < bestDistance) {
          best = node
          bestDistance = distance
        }
      }
    }
  }
  return best
}
