import type { LibraryGraphPage } from '../../library/libraryApi'
import { subjectsOf, type BrainSubject } from './brainSubjects'

export interface Region extends BrainSubject { x: number; y: number; radius: number }
export interface PagePoint { page: LibraryGraphPage; x: number; y: number; subject: string }
export interface ContinuousLayout {
  regions: Region[]
  points: PagePoint[]
  bounds: { x: number; y: number; width: number; height: number }
}

/** Layout uses the complete corpus, never filters, viewport size or the clock. */
export function continuousLayout(pages: LibraryGraphPage[]): ContinuousLayout {
  const groups = subjectsOf(pages)
  const regions: Region[] = []
  // Place large regions first. A fixed candidate grid gives a compact, repeatable
  // packing without a force simulation moving regions while the operator reads.
  for (const group of [...groups].sort((a, b) => b.pages.length - a.pages.length || a.id.localeCompare(b.id))) {
    const radius = Math.max(140, Math.sqrt(group.pages.length) * 12 + 40)
    let best = { x: 0, y: 0, score: Infinity }
    if (regions.length) {
      for (let ring = 1; !Number.isFinite(best.score); ring++) {
        const distance = ring * 30
        const steps = Math.max(12, Math.ceil(distance / 12))
        for (let step = 0; step < steps; step++) {
          const angle = step / steps * Math.PI * 2
          const x = Math.cos(angle) * distance * 1.9
          const y = Math.sin(angle) * distance
          if (regions.some(other => Math.hypot(x - other.x, y - other.y) < radius + other.radius + 58)) continue
          const score = Math.hypot(x / 1.9, y)
          if (score < best.score) best = { x, y, score }
        }
        if (Number.isFinite(best.score)) break
      }
    }
    regions.push({ ...group, x: best.x, y: best.y, radius })
  }
  const points = regions.flatMap(region => [...region.pages].sort((a, b) => a.path.localeCompare(b.path)).map((page, index, all) => {
    const angle = index * 2.3999632297
    const distance = Math.sqrt((index + .5) / all.length) * (region.radius - 35)
    return { page, subject: region.id, x: region.x + Math.cos(angle) * distance, y: region.y + Math.sin(angle) * distance }
  }))
  if (!regions.length) return { regions, points, bounds: { x: -100, y: -100, width: 200, height: 200 } }
  const left = Math.min(...regions.map(region => region.x - region.radius)) - 30
  const top = Math.min(...regions.map(region => region.y - region.radius)) - 30
  const right = Math.max(...regions.map(region => region.x + region.radius)) + 30
  const bottom = Math.max(...regions.map(region => region.y + region.radius)) + 30
  return { regions, points, bounds: { x: left, y: top, width: right - left, height: bottom - top } }
}
