export interface Point {
  x: number
  y: number
}

export interface ObstacleRect {
  id: string
  x: number
  y: number
  width: number
  height: number
}

export interface RouteOptions {
  fromId: string
  toId: string
  draggingNodeId?: string | null
  obstacles: ObstacleRect[]
}

export interface FormationRoute {
  points: Point[]
  path: string
}

export function routeFormationWire(source: Point, target: Point, options: RouteOptions): FormationRoute {
  if (options.draggingNodeId === options.fromId || options.draggingNodeId === options.toId) {
    return toRoute([source, target])
  }

  const direct = [source, target]
  const blocking = options.obstacles.find(rect => pathIntersectsRect(direct, rect, 6))
  if (!blocking) {
    return toRoute(direct)
  }

  const above = blocking.y - 32
  const below = blocking.y + blocking.height + 32
  const preferredY = Math.abs(above - source.y) <= Math.abs(below - source.y) ? above : below
  const stub = 24
  const points = [
    source,
    { x: source.x + stub, y: source.y },
    { x: source.x + stub, y: preferredY },
    { x: target.x - stub, y: preferredY },
    { x: target.x - stub, y: target.y },
    target,
  ]
  return toRoute(points)
}

export function pathIntersectsRect(points: Point[], rect: ObstacleRect, padding = 0): boolean {
  const padded = {
    x: rect.x - padding,
    y: rect.y - padding,
    width: rect.width + padding * 2,
    height: rect.height + padding * 2,
  }
  for (let i = 0; i < points.length - 1; i += 1) {
    if (segmentIntersectsRect(points[i], points[i + 1], padded)) return true
  }
  return false
}

function toRoute(points: Point[]): FormationRoute {
  return {
    points,
    path: points.map((point, index) => `${index === 0 ? 'M' : 'L'}${point.x},${point.y}`).join(' '),
  }
}

function segmentIntersectsRect(a: Point, b: Point, rect: Omit<ObstacleRect, 'id'>): boolean {
  if (pointInsideRect(a, rect) || pointInsideRect(b, rect)) return true

  const left = rect.x
  const right = rect.x + rect.width
  const top = rect.y
  const bottom = rect.y + rect.height
  return (
    segmentsIntersect(a, b, { x: left, y: top }, { x: right, y: top }) ||
    segmentsIntersect(a, b, { x: right, y: top }, { x: right, y: bottom }) ||
    segmentsIntersect(a, b, { x: right, y: bottom }, { x: left, y: bottom }) ||
    segmentsIntersect(a, b, { x: left, y: bottom }, { x: left, y: top })
  )
}

function pointInsideRect(point: Point, rect: Omit<ObstacleRect, 'id'>): boolean {
  return point.x >= rect.x && point.x <= rect.x + rect.width && point.y >= rect.y && point.y <= rect.y + rect.height
}

function segmentsIntersect(a: Point, b: Point, c: Point, d: Point): boolean {
  const denominator = (b.x - a.x) * (d.y - c.y) - (b.y - a.y) * (d.x - c.x)
  if (denominator === 0) return false
  const ua = ((d.x - c.x) * (a.y - c.y) - (d.y - c.y) * (a.x - c.x)) / denominator
  const ub = ((b.x - a.x) * (a.y - c.y) - (b.y - a.y) * (a.x - c.x)) / denominator
  return ua >= 0 && ua <= 1 && ub >= 0 && ub <= 1
}
