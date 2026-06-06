import type { LayoutNode, ViewTransform } from './formationsTypes'

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

export function clampScale(scale: number, max = 2.2): number {
  return Math.max(0.4, Math.min(max, Number(scale.toFixed(2))))
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
