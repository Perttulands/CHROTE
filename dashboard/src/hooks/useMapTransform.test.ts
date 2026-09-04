import { describe, expect, it } from 'vitest'
import {
  DEFAULT_LIMITS,
  IDENTITY,
  centreTransform,
  clampScale,
  isPanned,
  panBy,
  toScreen,
  toWorld,
  wheelFactor,
  zoomAbout,
} from './useMapTransform'

describe('zoomAbout', () => {
  it('keeps the point under the pointer under the pointer', () => {
    const pointer = { x: 300, y: 180 }
    const before = toWorld(IDENTITY, pointer)

    const zoomed = zoomAbout(IDENTITY, pointer, 2)

    expect(zoomed.scale).toBe(2)
    expect(toScreen(zoomed, before).x).toBeCloseTo(pointer.x, 6)
    expect(toScreen(zoomed, before).y).toBeCloseTo(pointer.y, 6)
  })

  it('keeps it under the pointer after a pan and a second zoom about another point', () => {
    const panned = panBy(zoomAbout(IDENTITY, { x: 100, y: 100 }, 1.7), -40, 25)
    const pointer = { x: 420, y: 90 }
    const under = toWorld(panned, pointer)

    const zoomed = zoomAbout(panned, pointer, 0.6)

    expect(toScreen(zoomed, under).x).toBeCloseTo(pointer.x, 6)
    expect(toScreen(zoomed, under).y).toBeCloseTo(pointer.y, 6)
  })

  it('stops at the limits without drifting', () => {
    const far = zoomAbout(IDENTITY, { x: 10, y: 10 }, 1000)
    expect(far.scale).toBe(DEFAULT_LIMITS.max)

    const further = zoomAbout(far, { x: 500, y: 500 }, 4)
    expect(further).toBe(far)

    const near = zoomAbout(IDENTITY, { x: 10, y: 10 }, 0.001)
    expect(near.scale).toBe(DEFAULT_LIMITS.min)
    expect(zoomAbout(near, { x: 500, y: 500 }, 0.25)).toBe(near)
  })
})

describe('the wheel', () => {
  it('zooms in scrolling up and out scrolling down, by the same factor either way', () => {
    expect(wheelFactor(-100)).toBeGreaterThan(1)
    expect(wheelFactor(100)).toBeLessThan(1)
    expect(wheelFactor(-100) * wheelFactor(100)).toBeCloseTo(1, 9)
  })

  it('counts a line and a page of delta as more than a pixel of it', () => {
    expect(wheelFactor(-3, 1)).toBeGreaterThan(wheelFactor(-3, 0))
    expect(wheelFactor(-1, 2)).toBeGreaterThan(wheelFactor(-1, 1))
  })
})

describe('panBy and clampScale', () => {
  it('moves the drawing by the pointer without touching the scale', () => {
    const panned = panBy({ x: 12, y: -4, scale: 2.5 }, 30, -10)
    expect(panned).toEqual({ x: 42, y: -14, scale: 2.5 })
  })

  it('holds the scale inside the limits, and treats nonsense as the floor', () => {
    expect(clampScale(1.5)).toBe(1.5)
    expect(clampScale(99)).toBe(DEFAULT_LIMITS.max)
    expect(clampScale(0)).toBe(DEFAULT_LIMITS.min)
    expect(clampScale(Number.NaN)).toBe(DEFAULT_LIMITS.min)
  })
})

describe('centreTransform', () => {
  it('brings a point of the drawing to the middle of the box at the asked scale', () => {
    const centred = centreTransform({ x: 800, y: 200 }, 3, { width: 960, height: 600 })

    expect(centred.scale).toBe(3)
    expect(toScreen(centred, { x: 800, y: 200 })).toEqual({ x: 480, y: 300 })
  })

  it('centres at the nearest allowed scale when asked past a limit', () => {
    const centred = centreTransform({ x: 100, y: 100 }, 40, { width: 400, height: 400 })

    expect(centred.scale).toBe(DEFAULT_LIMITS.max)
    expect(toScreen(centred, { x: 100, y: 100 })).toEqual({ x: 200, y: 200 })
  })
})

describe('isPanned', () => {
  it('is false only for the fit the layout was made for', () => {
    expect(isPanned(IDENTITY)).toBe(false)
    expect(isPanned({ x: 0, y: 1, scale: 1 })).toBe(true)
    expect(isPanned({ x: 0, y: 0, scale: 1.2 })).toBe(true)
  })
})
