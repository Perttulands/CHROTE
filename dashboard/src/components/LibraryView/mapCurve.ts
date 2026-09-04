/**
 * The shape of a hairline.
 *
 * A link between two pages is drawn as a soft curve rather than a straight
 * line: a bundle of straight lines through one cluster reads as a hatch, where
 * curves that all bow the same way read as the threads they are. The curve is
 * quadratic, its control point offset from the middle of the chord along the
 * chord's perpendicular by a fixed fraction of the chord's length, and the side
 * it bows to is decided from the two ends alone. That makes the picture the
 * same on every draw and on every device: no randomness, no order dependence,
 * and no state.
 *
 * The rule lives here, once, because two things draw it — the GPU layer, which
 * hands the three points to a shader, and the Canvas 2D fallback, which strokes
 * the same quadratic — and a map whose curves disagreed with themselves would
 * be a map that moved when the browser changed.
 */

/** A point in the layout's own coordinates. */
export interface MapPoint {
  x: number
  y: number
}

/**
 * How far the control point sits off the chord, as a fraction of the chord's
 * length. Enough that two pages joined twice do not draw one line, little
 * enough that a link still reads as going where it goes.
 */
export const CURVE_BOW = 0.14

/**
 * How many straight pieces a curve is drawn in. The GPU walks this many steps
 * along the quadratic; the eye stops counting corners well before eight, and
 * every step costs the whole corpus.
 */
export const CURVE_STEPS = 8

/**
 * Which side of the chord a hairline bows to, from the two ends alone.
 *
 * The chord's own direction decides it: an edge drawn left to right bows one
 * way and the same edge drawn right to left bows the other, which is the same
 * curve either way round. A vertical chord falls back to the vertical order, so
 * no pair is left without an answer.
 */
export function curveSide(from: MapPoint, to: MapPoint): 1 | -1 {
  if (from.x !== to.x) return from.x < to.x ? 1 : -1
  return from.y <= to.y ? 1 : -1
}

/**
 * The control point of the quadratic a hairline is drawn as. The same two ends
 * always give the same point, whichever end is named first.
 */
export function curveControl(from: MapPoint, to: MapPoint): MapPoint {
  const dx = to.x - from.x
  const dy = to.y - from.y
  // The offset is the chord's own perpendicular, so it grows with the chord
  // without the length ever being measured: a long link bows wide, a short one
  // barely at all, and the arithmetic is four multiplications.
  const bow = CURVE_BOW * curveSide(from, to)
  return { x: (from.x + to.x) / 2 - dy * bow, y: (from.y + to.y) / 2 + dx * bow }
}

/** A point along the curve, at t from 0 at `from` to 1 at `to`. */
export function curveAt(from: MapPoint, control: MapPoint, to: MapPoint, t: number): MapPoint {
  const u = 1 - t
  return {
    x: u * u * from.x + 2 * u * t * control.x + t * t * to.x,
    y: u * u * from.y + 2 * u * t * control.y + t * t * to.y,
  }
}
