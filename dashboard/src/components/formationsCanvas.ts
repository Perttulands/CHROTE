import type { BoardDocument, FormationNode, LayoutNode, ViewTransform } from './formationsTypes'

// The world background paints a 28px dot grid (formations-d7.css); placement
// and drag-release snap to it so cards line up with the surface they sit on.
export const GRID = 28

export function snapToGrid(value: number): number {
  return Math.round(value / GRID) * GRID
}

export function freeGridPosition(
  desired: { x: number; y: number },
  occupied: ReadonlyArray<{ x: number; y: number }>,
): { x: number; y: number } {
  let x = Math.max(GRID * 4, snapToGrid(desired.x))
  let y = Math.max(GRID * 4, snapToGrid(desired.y))
  for (let attempt = 0; attempt < 24; attempt += 1) {
    const collides = occupied.some(node => Math.abs(node.x - x) < 308 && Math.abs(node.y - y) < 280)
    if (!collides) return { x, y }
    x += 336
    if (x > 1900) {
      x = GRID * 4
      y += 336
    }
  }
  return { x, y }
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

export type LayoutItem = { id: string; index: number; kind: 'mission' | 'gate' | 'tool' | FormationNode['type']; slots?: number }

export function fallbackNodePosition(index: number): { x: number; y: number } {
  return { x: 140 + index * 308, y: 168 + (index % 2) * 196 }
}

/** Resolve display positions without mutating authored geometry. */
export function displayLayoutFor(board: BoardDocument, layoutByNode: Map<string, LayoutNode>): Map<string, LayoutNode> {
  const items = boardLayoutItems(board)
  const out = new Map<string, LayoutNode>()
  for (const item of items) {
    const base = layoutByNode.get(item.id) || { id: item.id, ...fallbackNodePosition(item.index) }
    out.set(item.id, { id: item.id, x: base.x, y: base.y })
  }
  return out
}

function boardLayoutItems(board: BoardDocument): LayoutItem[] {
  const missions = board.missions || []
  const formations = board.formations || []
  const gates = board.gates || []
  const tools = board.tools || []
  return [
    ...missions.map((node, index) => ({ id: node.id, index, kind: 'mission' as const })),
    ...formations.map((node, index) => ({ id: node.id, index: missions.length + index, kind: node.type, slots: node.slots.length })),
    ...gates.map((node, index) => ({ id: node.id, index: missions.length + formations.length + index, kind: 'gate' as const })),
    ...tools.map((node, index) => ({ id: node.id, index: missions.length + formations.length + gates.length + index, kind: 'tool' as const })),
  ]
}
