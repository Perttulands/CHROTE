import { describe, expect, it } from 'vitest'
import { CURVE_BOW, curveAt, curveControl, curveSide } from './mapCurve'

const A = { x: 100, y: 100 }
const B = { x: 300, y: 100 }

describe('curveControl', () => {
  // The map must be the same picture on every draw, so the bow is decided from
  // the two ends and nothing else — not from the order the server listed them.
  it('gives the same curve whichever end is named first', () => {
    expect(curveControl(A, B)).toEqual(curveControl(B, A))
  })

  it('offsets the middle of the chord along its perpendicular', () => {
    const control = curveControl(A, B)
    expect(control.x).toBe(200)
    expect(control.y).toBe(100 + 200 * CURVE_BOW)
  })

  it('bows a long link further than a short one, in proportion', () => {
    const near = curveControl({ x: 0, y: 0 }, { x: 10, y: 0 })
    const far = curveControl({ x: 0, y: 0 }, { x: 100, y: 0 })
    expect(far.y / near.y).toBeCloseTo(10)
  })

  // Two pages that have landed on each other have no chord to bow off.
  it('collapses to the point when both ends are the same', () => {
    expect(curveControl(A, { ...A })).toEqual(A)
  })
})

describe('curveSide', () => {
  it('reads the chord left to right, and falls back to top to bottom', () => {
    expect(curveSide(A, B)).toBe(1)
    expect(curveSide(B, A)).toBe(-1)
    expect(curveSide({ x: 5, y: 0 }, { x: 5, y: 9 })).toBe(1)
    expect(curveSide({ x: 5, y: 9 }, { x: 5, y: 0 })).toBe(-1)
  })
})

describe('curveAt', () => {
  it('starts at one end, finishes at the other, and bows between them', () => {
    const control = curveControl(A, B)
    expect(curveAt(A, control, B, 0)).toEqual(A)
    expect(curveAt(A, control, B, 1)).toEqual(B)
    const middle = curveAt(A, control, B, 0.5)
    expect(middle.x).toBe(200)
    expect(middle.y).toBeGreaterThan(A.y)
    expect(middle.y).toBeLessThan(control.y)
  })
})
