import type { BoardDocument, FormationNode, LayoutNode, ViewTransform } from './formationsTypes'

export function defaultPosition(index: number): LayoutNode {
  return { id: '', x: 120 + index * 280, y: 120 + (index % 2) * 180 }
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
  return { x: 140 + index * 300, y: 160 + (index % 2) * 200 }
}

export function estimatedNodeBox(item: LayoutItem): { w: number; h: number } {
  if (item.kind === 'mission') return { w: 236, h: 136 }
  if (item.kind === 'gate') return { w: 300, h: 124 }
  if (item.kind === 'flow') return { w: Math.min(560, Math.max(300, 172 + (item.slots || 1) * 132)), h: 270 }
  if (item.kind === 'peer') return { w: 330, h: 286 }
  if (item.kind === 'orchestrated') return { w: 320, h: 372 }
  return { w: 300, h: 270 }
}

export function overlaps(a: LayoutBox, b: LayoutBox, gap = 36): boolean {
  return a.x < b.x + b.w + gap &&
    a.x + a.w + gap > b.x &&
    a.y < b.y + b.h + gap &&
    a.y + a.h + gap > b.y
}

/** Resolve display positions, nudging overlapping cards apart (the cockpit's first-render layout). */
export function displayLayoutFor(board: BoardDocument, layoutByNode: Map<string, LayoutNode>): Map<string, LayoutNode> {
  const missions = board.missions || []
  const formations = board.formations || []
  const gates = board.gates || []
  const items: LayoutItem[] = [
    ...missions.map((node, index) => ({ id: node.id, index, kind: 'mission' as const })),
    ...formations.map((node, index) => ({ id: node.id, index: missions.length + index, kind: node.type, slots: node.slots.length })),
    ...gates.map((node, index) => ({ id: node.id, index: missions.length + formations.length + index, kind: 'gate' as const })),
  ]
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
      const right = blocker.x + blocker.w + 56
      if (right + size.w <= 1900) {
        x = right
      } else {
        x = Math.max(120, Math.min(x, blocker.x))
        y = blocker.y + blocker.h + 56
      }
    }
    out.set(item.id, { id: item.id, x, y })
    placed.push({ id: item.id, x, y, ...size })
  }
  return out
}
