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

/* ---------------------------------------------------------------------------
 * Reference router, ported from the D7 prototype (03-formations.js:652-770):
 * committed wires are orthogonal stub-routed elbows with rounded corners —
 * 24px stubs, a vertical mid-split, r=10 corner rounding, a dead-straight line
 * when endpoints are level and unobstructed, clearance lanes around cards and
 * for genuinely-backward (pushback) targets, and judge wires as brackets above
 * the gate socket. Free-form Beziers are reserved for temp drag wires only.
 * ------------------------------------------------------------------------- */

export interface OrthoRouteOptions {
  fromId: string
  toId: string
  obstacles: ObstacleRect[]
  /** A node is being dragged: skip obstacle detours so wires don't "gravitate". */
  frozen?: boolean
  /** Hand-routed lane Y (the prototype's `via.y`); overrides automatic routing. */
  laneY?: number | null
}

const STUB = 24
const CORNER_RADIUS = 10
const LANE_CLEARANCE = 32
const LANE_SPAN_PAD = 14
const OBSTACLE_PAD = 6
const LEVEL_EPSILON = 6
const JUDGE_RISE = 26
const JUDGE_SIDE = 30

function fmt(value: number): string {
  return String(Math.round(value * 100) / 100)
}

/** Orthogonal polyline → SVG path with lightly-rounded 90° corners. */
export function roundedOrthoPath(rawPoints: Point[], radius: number): string {
  const points: Point[] = []
  for (const point of rawPoints) {
    const previous = points[points.length - 1]
    if (!previous || Math.abs(previous.x - point.x) > 0.5 || Math.abs(previous.y - point.y) > 0.5) {
      points.push(point)
    }
  }
  if (points.length < 2) return ''
  if (points.length === 2) return `M${fmt(points[0].x)},${fmt(points[0].y)} L${fmt(points[1].x)},${fmt(points[1].y)}`
  let d = `M${fmt(points[0].x)},${fmt(points[0].y)}`
  for (let i = 1; i < points.length - 1; i += 1) {
    const p0 = points[i - 1]
    const p1 = points[i]
    const p2 = points[i + 1]
    const inX = Math.sign(p1.x - p0.x)
    const inY = Math.sign(p1.y - p0.y)
    const outX = Math.sign(p2.x - p1.x)
    const outY = Math.sign(p2.y - p1.y)
    const d1 = Math.min(radius, Math.hypot(p1.x - p0.x, p1.y - p0.y) / 2)
    const d2 = Math.min(radius, Math.hypot(p2.x - p1.x, p2.y - p1.y) / 2)
    const a = { x: p1.x - inX * d1, y: p1.y - inY * d1 }
    const b = { x: p1.x + outX * d2, y: p1.y + outY * d2 }
    d += ` L${fmt(a.x)},${fmt(a.y)} Q${fmt(p1.x)},${fmt(p1.y)} ${fmt(b.x)},${fmt(b.y)}`
  }
  const last = points[points.length - 1]
  d += ` L${fmt(last.x)},${fmt(last.y)}`
  return d
}

/** Liang–Barsky: does segment a→b cross rect inflated by margin? */
export function segmentHitsRect(a: Point, b: Point, rect: ObstacleRect, margin: number): boolean {
  const x1 = rect.x - margin
  const y1 = rect.y - margin
  const x2 = rect.x + rect.width + margin
  const y2 = rect.y + rect.height + margin
  let t0 = 0
  let t1 = 1
  const dx = b.x - a.x
  const dy = b.y - a.y
  const p = [-dx, dx, -dy, dy]
  const q = [a.x - x1, x2 - a.x, a.y - y1, y2 - a.y]
  for (let i = 0; i < 4; i += 1) {
    if (p[i] === 0) {
      if (q[i] < 0) return false
    } else {
      const t = q[i] / p[i]
      if (p[i] < 0) {
        if (t > t1) return false
        if (t > t0) t0 = t
      } else {
        if (t < t0) return false
        if (t < t1) t1 = t
      }
    }
  }
  return true
}

/** Pick a horizontal lane (above or below) clear of cards spanning [xL, xR]. */
export function clearLaneY(obstacles: ObstacleRect[], xL: number, xR: number, preferY: number): number {
  const blocking = obstacles.filter(rect => rect.x < xR + LANE_SPAN_PAD && rect.x + rect.width > xL - LANE_SPAN_PAD)
  if (!blocking.length) return preferY
  const above = Math.min(...blocking.map(rect => rect.y)) - LANE_CLEARANCE
  const below = Math.max(...blocking.map(rect => rect.y + rect.height)) + LANE_CLEARANCE
  return Math.abs(above - preferY) <= Math.abs(below - preferY) ? above : below
}

/** Orthogonal committed-wire router: out the source's right, into the target's left. */
export function routeOrthoWire(source: Point, target: Point, options: OrthoRouteOptions): string {
  const s2 = { x: source.x + STUB, y: source.y }
  const t2 = { x: target.x - STUB, y: target.y }
  if (options.laneY !== undefined && options.laneY !== null) {
    return roundedOrthoPath([source, s2, { x: s2.x, y: options.laneY }, { x: t2.x, y: options.laneY }, t2, target], CORNER_RADIUS)
  }
  const obstacles = options.obstacles.filter(rect => rect.id !== options.fromId && rect.id !== options.toId)
  const clear = (points: Point[]): boolean => {
    if (options.frozen) return true
    for (let i = 0; i < points.length - 1; i += 1) {
      for (const rect of obstacles) {
        if (segmentHitsRect(points[i], points[i + 1], rect, OBSTACLE_PAD)) return false
      }
    }
    return true
  }
  const level = Math.abs(source.y - target.y) < LEVEL_EPSILON
  const backward = target.x < source.x - STUB
  let points: Point[]
  if (!backward) {
    if (level && clear([source, target])) return roundedOrthoPath([source, target], CORNER_RADIUS)
    const midX = Math.round((s2.x + t2.x) / 2)
    const direct = [source, s2, { x: midX, y: s2.y }, { x: midX, y: t2.y }, t2, target]
    if (clear(direct)) {
      points = direct
    } else {
      const laneY = clearLaneY(obstacles, s2.x, t2.x, (source.y + target.y) / 2)
      points = [source, s2, { x: s2.x, y: laneY }, { x: t2.x, y: laneY }, { x: t2.x, y: t2.y }, t2, target]
    }
  } else {
    const laneY = options.frozen
      ? Math.min(source.y, target.y) - 46
      : clearLaneY(obstacles, t2.x, s2.x, (source.y + target.y) / 2)
    points = [source, s2, { x: s2.x, y: laneY }, { x: t2.x, y: laneY }, { x: t2.x, y: t2.y }, t2, target]
  }
  return roundedOrthoPath(points, CORNER_RADIUS)
}

export interface JudgeRouteOptions {
  direction: 'send' | 'return'
  /** Card box of the non-gate endpoint (the judge formation), if known. */
  nodeRect?: ObstacleRect | null
}

/** Judge wires bracket just above the gate's top socket (reference routeJudge). */
export function routeJudgeWire(a: Point, b: Point, options: JudgeRouteOptions): string {
  if (options.direction === 'send') {
    // SEND: gate socket (a, top) → judge input (b, left side)
    const riseY = a.y - JUDGE_RISE
    const laneX = options.nodeRect ? options.nodeRect.x - JUDGE_SIDE : b.x - 40
    return roundedOrthoPath([a, { x: a.x, y: riseY }, { x: laneX, y: riseY }, { x: laneX, y: b.y }, b], CORNER_RADIUS)
  }
  // RETURN: judge output (a, right side) → gate socket (b, top)
  const riseY = b.y - JUDGE_RISE
  const laneX = options.nodeRect ? options.nodeRect.x + options.nodeRect.width + JUDGE_SIDE : a.x + 40
  return roundedOrthoPath([a, { x: laneX, y: a.y }, { x: laneX, y: riseY }, { x: b.x, y: riseY }, b], CORNER_RADIUS)
}
