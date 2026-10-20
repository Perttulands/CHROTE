import { describe, expect, it } from 'vitest'
import type { LibraryGraphPage } from '../../library/libraryApi'
import { continuousLayout } from './continuousLayout'

const pages = Array.from({ length: 260 }, (_, index) => ({
  path: `knowledge/subject-${index % 8}/page-${index}.md`, title: `Page ${index}`,
  shelf: 'knowledge', subject: `knowledge/subject-${index % 8}`, words: 300,
  created: '', updated: '', candidate: false,
} satisfies LibraryGraphPage))

describe('continuous corpus layout', () => {
  it('keeps each page in a distinct subject region and within the fitted bounds', () => {
    const { regions, points, bounds } = continuousLayout(pages)
    expect(points).toHaveLength(pages.length)
    expect(new Set(points.map(point => point.page.path)).size).toBe(pages.length)
    for (const region of regions) {
      expect(region.x - region.radius).toBeGreaterThanOrEqual(bounds.x)
      expect(region.x + region.radius).toBeLessThanOrEqual(bounds.x + bounds.width)
      expect(region.y - region.radius).toBeGreaterThanOrEqual(bounds.y)
      expect(region.y + region.radius).toBeLessThanOrEqual(bounds.y + bounds.height)
      for (const other of regions.filter(candidate => candidate !== region)) {
        expect(Math.hypot(region.x - other.x, region.y - other.y)).toBeGreaterThan(region.radius + other.radius)
      }
      for (const point of points.filter(candidate => candidate.subject === region.id)) {
        expect(Math.hypot(point.x - region.x, point.y - region.y)).toBeLessThan(region.radius)
      }
    }
  })

  it('retains page positions when API ordering, titles or modification dates change', () => {
    const positions = (input: LibraryGraphPage[]) => continuousLayout(input).points
      .map(({ page, x, y }) => [page.path, x, y])
    expect(positions([...pages].reverse().map(page => ({ ...page, title: 'Updated title', updated: '2026-10-21' }))))
      .toEqual(positions(pages))
  })
})
