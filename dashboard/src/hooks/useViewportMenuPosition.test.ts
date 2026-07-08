import { describe, expect, it } from 'vitest'
import { positionMenuInViewport } from './useViewportMenuPosition'

describe('positionMenuInViewport', () => {
  it('clamps an anchored menu against the viewport bottom-right edge', () => {
    expect(positionMenuInViewport(
      { x: 390, y: 290 },
      { width: 120, height: 80 },
      { viewportWidth: 400, viewportHeight: 300, margin: 8 },
    )).toEqual({ left: 272, top: 212 })
  })

  it('keeps the requested anchor when the menu fits inside the viewport', () => {
    expect(positionMenuInViewport(
      { x: 40, y: 50 },
      { width: 120, height: 80 },
      { viewportWidth: 400, viewportHeight: 300, margin: 8 },
    )).toEqual({ left: 40, top: 50 })
  })
})
