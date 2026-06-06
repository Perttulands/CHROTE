import { describe, expect, it } from 'vitest'
import { clampScale, defaultPosition, endpointNodeId, screenPointToWorld, visibleWirePath, zoomTransform } from './formationsCanvas'

describe('formations canvas helpers', () => {
  it('computes stable default positions for starter nodes', () => {
    expect(defaultPosition(0)).toEqual({ id: '', x: 120, y: 120 })
    expect(defaultPosition(1)).toEqual({ id: '', x: 400, y: 300 })
  })

  it('extracts node ids from stable endpoint addresses', () => {
    expect(endpointNodeId('formation-frame:out')).toBe('formation-frame')
    expect(endpointNodeId('gate-review:judge')).toBe('gate-review')
  })

  it('keeps flat visible wires hit-testable without changing routed paths', () => {
    expect(visibleWirePath('M10,20 L30,20')).toBe('M10,20 L30,20 L30,21')
    expect(visibleWirePath('M10,20 C20,20 20,30 30,30')).toBe('M10,20 C20,20 20,30 30,30')
  })

  it('clamps zoom and preserves cursor world position', () => {
    expect(clampScale(0.1)).toBe(0.4)
    expect(clampScale(9)).toBe(2.2)
    const next = zoomTransform({ x: 10, y: 20, scale: 1 }, 1.2, { x: 110, y: 220 })
    expect(next.scale).toBe(1.2)
    expect(next.x).toBe(-10)
    expect(next.y).toBe(-20)
  })

  it('maps screen coordinates into transformed canvas world coordinates', () => {
    const viewport = {
      left: 20,
      top: 30,
      right: 820,
      bottom: 530,
      width: 800,
      height: 500,
      x: 20,
      y: 30,
      toJSON: () => ({}),
    } as DOMRect

    expect(screenPointToWorld({ x: 220, y: 330 }, viewport, { x: 40, y: 20, scale: 2 })).toEqual({
      x: 80,
      y: 140,
    })
  })
})
