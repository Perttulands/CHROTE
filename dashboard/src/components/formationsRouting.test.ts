import { describe, expect, it } from 'vitest'
import { pathIntersectsRect, routeFormationWire } from './formationsRouting'

describe('formationsRouting', () => {
  it('routes around an obstacle instead of under a card', () => {
    const obstacle = { id: 'middle', x: 90, y: 20, width: 60, height: 80 }
    const route = routeFormationWire(
      { x: 20, y: 60 },
      { x: 220, y: 60 },
      { fromId: 'source', toId: 'target', obstacles: [obstacle] }
    )

    expect(route.path).toMatch(/^M/)
    expect(pathIntersectsRect(route.points, obstacle, 6)).toBe(false)
  })

  it('freezes direct routing while a connected node is being dragged', () => {
    const obstacle = { id: 'middle', x: 90, y: 20, width: 60, height: 80 }
    const route = routeFormationWire(
      { x: 20, y: 60 },
      { x: 220, y: 60 },
      { fromId: 'source', toId: 'target', draggingNodeId: 'source', obstacles: [obstacle] }
    )

    expect(route.points).toEqual([{ x: 20, y: 60 }, { x: 220, y: 60 }])
  })
})
