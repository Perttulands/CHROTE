import { describe, expect, it } from 'vitest'
import type { BoardDocument, LayoutNode, ToolNode } from './formationsTypes'
import { GRID, clampScale, defaultPosition, displayLayoutFor, endpointNodeId, freeGridPosition, screenPointToWorld, snapToGrid, visibleWirePath, zoomTransform } from './formationsCanvas'

function canvasTool(id: string): ToolNode {
  return {
    id,
    title: 'Normalize report',
    profileId: 'json.normalize',
    profileVersion: '1',
    params: { mode: 'strict' },
    inputs: [{
      id: `${id}_in`,
      name: 'input',
      label: 'Report',
      direction: 'input',
      kind: 'work',
      acceptedMediaTypes: ['application/json'],
      required: true,
      role: 'data',
    }],
    outputs: [{
      id: `${id}_out`,
      name: 'output',
      label: 'Normalized report',
      direction: 'output',
      kind: 'work',
      acceptedMediaTypes: ['application/json'],
    }],
  }
}

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

  it('renders intentionally overlapping persisted coordinates verbatim', () => {
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
    expect(positions).toEqual([
      { id: 'mis_a', x: 220, y: 160 },
      { id: 'fmn_a', x: 220, y: 160 },
      { id: 'fmn_b', x: 220, y: 160 },
    ])
  })

  it('appends Tools after legacy fallback nodes and ignores layout-only authority', () => {
    const board: BoardDocument = {
      id: 'brd_tool_layout',
      slug: 'tool-layout',
      title: 'Tool layout',
      rev: 2,
      etag: 'board-etag',
      missions: [{ id: 'mis_a', title: 'Mission', goal: '', beadId: '' }],
      formations: [{ id: 'fmn_a', type: 'solo', title: 'Formation', inputs: [], outputs: [], slots: [] }],
      gates: [{ id: 'gate_a', title: 'Gate', kinds: ['human'], criterion: '' }],
      tools: [canvasTool('tool_persisted'), canvasTool('tool_fallback')],
      connections: [],
    }
    const layoutByNode = new Map<string, LayoutNode>([
      ['tool_persisted', { id: 'tool_persisted', x: 980, y: 420 }],
      ['layout_only', { id: 'layout_only', x: 40, y: 40 }],
    ])

    const resolved = displayLayoutFor(board, layoutByNode)

    expect([...resolved.keys()]).toEqual(['mis_a', 'fmn_a', 'gate_a', 'tool_persisted', 'tool_fallback'])
    expect(resolved.get('mis_a')).toEqual({ id: 'mis_a', x: 140, y: 168 })
    expect(resolved.get('fmn_a')).toEqual({ id: 'fmn_a', x: 448, y: 364 })
    expect(resolved.get('gate_a')).toEqual({ id: 'gate_a', x: 756, y: 168 })
    expect(resolved.get('tool_persisted')).toEqual({ id: 'tool_persisted', x: 980, y: 420 })
    expect(resolved.get('tool_fallback')).toEqual({ id: 'tool_fallback', x: 1372, y: 168 })
    expect(resolved.has('layout_only')).toBe(false)
  })

  it('places only a newly created node onto nearby free grid space', () => {
    const occupied = [{ x: 224, y: 168 }, { x: 560, y: 168 }]
    expect(freeGridPosition({ x: 220, y: 160 }, occupied)).toEqual({ x: 896, y: 168 })
    expect(occupied).toEqual([{ x: 224, y: 168 }, { x: 560, y: 168 }])
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
