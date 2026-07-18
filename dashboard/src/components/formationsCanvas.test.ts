import { describe, expect, it } from 'vitest'
import type { BoardDocument, LayoutNode } from './formationsTypes'
import { GRID, clampScale, defaultPosition, displayLayoutFor, endpointNodeId, screenPointToWorld, snapToGrid, tidyLayout, visibleWirePath, zoomTransform } from './formationsCanvas'

describe('formations canvas helpers', () => {
  it('computes stable grid-aligned default positions for starter nodes', () => {
    expect(defaultPosition(0)).toEqual({ id: '', x: 112, y: 112 })
    expect(defaultPosition(1)).toEqual({ id: '', x: 392, y: 308 })
    expect(defaultPosition(0).x % GRID).toBe(0)
    expect(defaultPosition(1).y % GRID).toBe(0)
  })

  it('snaps values to the canvas grid', () => {
    expect(snapToGrid(0)).toBe(0)
    expect(snapToGrid(13)).toBe(0)
    expect(snapToGrid(14)).toBe(28)
    expect(snapToGrid(430)).toBe(420)
  })

  it('renders authored non-overlapping layouts verbatim (no phantom shoves)', () => {
    // Mirrors an archon-authored board: missions left, a formation row, gates
    // in their own top row. Every position must survive display untouched.
    const board = {
      missions: [{ id: 'mis_a' }, { id: 'mis_b' }],
      formations: [
        { id: 'fmn_orch', type: 'orchestrated', slots: [{}, {}, {}] },
        { id: 'fmn_peer', type: 'peer', slots: [{}, {}] },
        { id: 'fmn_solo', type: 'solo', slots: [{}] },
      ],
      gates: [{ id: 'gate_a' }, { id: 'gate_b' }],
    } as unknown as BoardDocument
    const authored: Array<[string, number, number]> = [
      ['mis_a', 80, 80], ['mis_b', 80, 260],
      ['fmn_orch', 420, 220], ['fmn_peer', 784, 220], ['fmn_solo', 1148, 220],
      ['gate_a', 420, 56], ['gate_b', 784, 56],
    ]
    const layoutByNode = new Map<string, LayoutNode>(authored.map(([id, x, y]) => [id, { id, x, y }]))
    const resolved = displayLayoutFor(board, layoutByNode)
    for (const [id, x, y] of authored) {
      expect(resolved.get(id)).toEqual({ id, x, y })
    }
  })

  it('spreads cards stacked on identical coordinates onto grid-aligned spots', () => {
    const board = {
      missions: [{ id: 'mis_a' }],
      formations: [
        { id: 'fmn_a', type: 'solo', slots: [{}] },
        { id: 'fmn_b', type: 'solo', slots: [{}] },
      ],
      gates: [],
    } as unknown as BoardDocument
    const layoutByNode = new Map<string, LayoutNode>(
      ['mis_a', 'fmn_a', 'fmn_b'].map(id => [id, { id, x: 220, y: 160 }]),
    )
    const resolved = displayLayoutFor(board, layoutByNode)
    const positions = ['mis_a', 'fmn_a', 'fmn_b'].map(id => resolved.get(id)!)
    expect(new Set(positions.map(p => `${p.x}:${p.y}`)).size).toBe(3)
    // Nudged cards land on the visible grid; the first keeps its authored spot.
    expect(positions[0]).toEqual({ id: 'mis_a', x: 220, y: 160 })
    expect(positions[1].x % GRID).toBe(0)
    expect(positions[2].x % GRID).toBe(0)
  })

  it('tidyLayout arranges columns by graph depth with grid-aligned, non-overlapping rows', () => {
    const board = {
      missions: [{ id: 'mis_a' }],
      formations: [
        { id: 'fmn_build', type: 'peer', slots: [{}, {}] },
        { id: 'fmn_report', type: 'solo', slots: [{}] },
      ],
      gates: [{ id: 'gate_check' }],
      connections: [
        { id: 'c1', from: 'mis_a:out', to: 'fmn_build:in' },
        { id: 'c2', from: 'fmn_build:out', to: 'gate_check:in' },
        { id: 'c3', from: 'gate_check:pass', to: 'fmn_report:in' },
      ],
    } as unknown as BoardDocument
    const arranged = tidyLayout(board, new Map())
    const byId = new Map(arranged.map(node => [node.id, node]))
    expect(arranged).toHaveLength(4)
    for (const node of arranged) {
      expect(node.x % GRID).toBe(0)
      expect(node.y % GRID).toBe(0)
    }
    // Depth ordering: mission → formation → gate → downstream formation.
    expect(byId.get('mis_a')!.x).toBeLessThan(byId.get('fmn_build')!.x)
    expect(byId.get('fmn_build')!.x).toBeLessThan(byId.get('gate_check')!.x)
    expect(byId.get('gate_check')!.x).toBeLessThan(byId.get('fmn_report')!.x)
    // Deterministic and stable across calls.
    expect(tidyLayout(board, new Map())).toEqual(arranged)
  })

  it('tidyLayout survives judge-style cycles without infinite recursion', () => {
    const board = {
      missions: [],
      formations: [{ id: 'fmn_judge', type: 'solo', slots: [{}] }],
      gates: [{ id: 'gate_final' }],
      connections: [
        { id: 'c1', from: 'gate_final:judge', to: 'fmn_judge:in' },
        { id: 'c2', from: 'fmn_judge:out', to: 'gate_final:judge-return' },
      ],
    } as unknown as BoardDocument
    const arranged = tidyLayout(board, new Map())
    expect(arranged).toHaveLength(2)
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
    expect(clampScale(0.1)).toBe(0.2)
    expect(clampScale(9)).toBe(1.9)
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
