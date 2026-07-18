import type { BoardDocument, FormationNode, LayoutNode, ViewTransform } from './formationsTypes'

// The world background paints a 28px dot grid (formations-d7.css); placement
// and drag-release snap to it so cards line up with the surface they sit on.
export const GRID = 28

export function snapToGrid(value: number): number {
  return Math.round(value / GRID) * GRID
}

function snapUpToGrid(value: number): number {
  return Math.ceil(value / GRID) * GRID
}

export function defaultPosition(index: number): LayoutNode {
  return { id: '', x: 112 + index * 280, y: 112 + (index % 2) * 196 }
}

export function endpointNodeId(endpoint: string): string {
  return endpoint.split(':', 1)[0]
}

export function screenPointToWorld(
  point: { x: number; y: number },
  viewport: DOMRect,
  transform: ViewTransform,
): { x: number; y: number } {
  return {
    x: Math.round((point.x - viewport.left - transform.x) / transform.scale),
    y: Math.round((point.y - viewport.top - transform.y) / transform.scale),
  }
}

export function visibleWirePath(path: string): string {
  const match = path.match(/^M(-?\d+(?:\.\d+)?),(-?\d+(?:\.\d+)?) L(-?\d+(?:\.\d+)?),(-?\d+(?:\.\d+)?)$/)
  if (!match || match[2] !== match[4]) return path
  const y = Number(match[4])
  if (!Number.isFinite(y)) return path
  return `${path} L${match[3]},${y + 1}`
}

// Reference zoom range (03-formations.js MINS=0.2, MAXS=1.9).
export function clampScale(scale: number, max = 1.9): number {
  return Math.max(0.2, Math.min(max, Number(scale.toFixed(2))))
}

export function zoomTransform(current: ViewTransform, factor: number, cursor?: { x: number; y: number }): ViewTransform {
  const scale = clampScale(current.scale * factor)
  if (!cursor) return { ...current, scale }
  const worldX = (cursor.x - current.x) / current.scale
  const worldY = (cursor.y - current.y) / current.scale
  return {
    x: Math.round(cursor.x - worldX * scale),
    y: Math.round(cursor.y - worldY * scale),
    scale,
  }
}

export type LayoutItem = { id: string; index: number; kind: 'mission' | 'gate' | FormationNode['type']; slots?: number }
export type LayoutBox = { id: string; x: number; y: number; w: number; h: number }

export function fallbackNodePosition(index: number): { x: number; y: number } {
  return { x: 140 + index * 308, y: 168 + (index % 2) * 196 }
}

// Calibrated against rendered card boxes (slot 84w + 38 arrow pitch + body
// padding); oversized estimates here read as phantom overlaps that scatter
// authored layouts.
export function estimatedNodeBox(item: LayoutItem): { w: number; h: number } {
  if (item.kind === 'mission') return { w: 236, h: 144 }
  if (item.kind === 'gate') return { w: 300, h: 124 }
  if (item.kind === 'flow') return { w: Math.min(560, Math.max(300, 120 + (item.slots || 1) * 84)), h: 300 }
  if (item.kind === 'peer') return { w: 330, h: 286 }
  if (item.kind === 'orchestrated') return { w: 320, h: 372 }
  return { w: 300, h: 270 }
}

// Sizes are estimates, so a generous gap here manufactures phantom collisions
// that shove tidy authored layouts apart. Persisted positions are the operator's
// (or archon's) intent: only genuine box intersections may move a card.
export function overlaps(a: LayoutBox, b: LayoutBox, gap = 8): boolean {
  return a.x < b.x + b.w + gap &&
    a.x + a.w + gap > b.x &&
    a.y < b.y + b.h + gap &&
    a.y + a.h + gap > b.y
}

/** Resolve display positions, nudging genuinely overlapping cards apart (the cockpit's first-render layout). */
export function displayLayoutFor(board: BoardDocument, layoutByNode: Map<string, LayoutNode>): Map<string, LayoutNode> {
  const items = boardLayoutItems(board)
  const placed: LayoutBox[] = []
  const out = new Map<string, LayoutNode>()
  for (const item of items) {
    const base = layoutByNode.get(item.id) || { id: item.id, ...fallbackNodePosition(item.index) }
    const size = estimatedNodeBox(item)
    let x = base.x
    let y = base.y
    for (let guard = 0; guard < 24; guard += 1) {
      const candidate = { id: item.id, x, y, ...size }
      const blocker = placed.find(prev => overlaps(candidate, prev))
      if (!blocker) break
      const right = snapUpToGrid(blocker.x + blocker.w + 12)
      if (right + size.w <= 1900) {
        x = right
      } else {
        x = Math.max(GRID * 4, Math.min(snapToGrid(x), blocker.x))
        y = snapUpToGrid(blocker.y + blocker.h + 12)
      }
    }
    out.set(item.id, { id: item.id, x, y })
    placed.push({ id: item.id, x, y, ...size })
  }
  return out
}

function boardLayoutItems(board: BoardDocument): LayoutItem[] {
  const missions = board.missions || []
  const formations = board.formations || []
  const gates = board.gates || []
  return [
    ...missions.map((node, index) => ({ id: node.id, index, kind: 'mission' as const })),
    ...formations.map((node, index) => ({ id: node.id, index: missions.length + index, kind: node.type, slots: node.slots.length })),
    ...gates.map((node, index) => ({ id: node.id, index: missions.length + formations.length + index, kind: 'gate' as const })),
  ]
}

/**
 * Deterministic layered arrangement for the Tidy action: columns by graph depth
 * (longest path from a source), rows stacked in current-y order, everything on
 * the grid. The result is persisted through the layout PATCH so archon and the
 * UI keep seeing the same geometry.
 */
export function tidyLayout(board: BoardDocument, layoutByNode: Map<string, LayoutNode>): LayoutNode[] {
  const items = boardLayoutItems(board)
  if (!items.length) return []
  const itemById = new Map(items.map(item => [item.id, item]))
  const inEdges = new Map<string, string[]>()
  for (const conn of board.connections || []) {
    const from = endpointNodeId(conn.from)
    const to = endpointNodeId(conn.to)
    if (!itemById.has(from) || !itemById.has(to) || from === to) continue
    inEdges.set(to, [...(inEdges.get(to) || []), from])
  }
  const depthCache = new Map<string, number>()
  const visiting = new Set<string>()
  const depthOf = (id: string): number => {
    const cached = depthCache.get(id)
    if (cached !== undefined) return cached
    if (visiting.has(id)) return 0 // cycle guard: judge loops route back into gates
    visiting.add(id)
    const preds = inEdges.get(id) || []
    const depth = preds.length ? Math.max(...preds.map(pred => depthOf(pred) + 1)) : 0
    visiting.delete(id)
    depthCache.set(id, depth)
    return depth
  }
  const columns = new Map<number, LayoutItem[]>()
  for (const item of items) {
    const depth = depthOf(item.id)
    columns.set(depth, [...(columns.get(depth) || []), item])
  }
  const currentY = (item: LayoutItem): number =>
    (layoutByNode.get(item.id) || fallbackNodePosition(item.index)).y
  const out: LayoutNode[] = []
  let x = GRID * 4
  for (const depth of [...columns.keys()].sort((a, b) => a - b)) {
    const column = columns.get(depth)!
    column.sort((a, b) => currentY(a) - currentY(b) || a.index - b.index)
    let y = GRID * 4
    let width = 0
    for (const item of column) {
      const size = estimatedNodeBox(item)
      out.push({ id: item.id, x, y })
      y = Math.ceil((y + size.h + 48) / GRID) * GRID
      width = Math.max(width, size.w)
    }
    x = Math.ceil((x + width + 84) / GRID) * GRID
  }
  return out
}
