/**
 * Where everything sits in the night sky of the brain.
 *
 * A subject is a star system. Its topic page is the bright star at its centre;
 * the research and notes that link somewhere orbit it, the better connected
 * the closer in; its evidence is the dust around them, so how much a subject
 * rests on shows as how much nebula it has. A page that links nowhere has
 * drifted out past its system's edge, which is what tells the operator to send
 * someone to tidy it.
 *
 * Systems are placed by their links: two subjects that cite each other are
 * pulled together, everything else only keeps its distance, so related work
 * forms constellations. The arithmetic is a fixed number of steps from a fixed
 * start, with no randomness, so the same corpus is always the same sky, and
 * filters, the clock and the viewport never move anything.
 *
 * A growing corpus would still reshuffle a sky placed from scratch: one page
 * changes a system's size, and the settling goes another way. So a sky drawn
 * before is passed back in, and its systems stay where they were, a newcomer
 * joining the systems it links to; only overlaps are settled.
 */

import type { LibraryGraph, LibraryGraphPage } from '../../library/libraryApi'
import { kindOf, subjectName, subjectOf } from './brainSubjects'

export type StarRole = 'core' | 'star' | 'dust' | 'stray'

export interface SkyRegion {
  id: string
  name: string
  collection: string
  x: number
  y: number
  radius: number
  /** The topic page at the centre, or null for a subject nobody has summed up. */
  core: string | null
  pages: number
  evidence: number
}

export interface SkyStar {
  page: LibraryGraphPage
  subject: string
  role: StarRole
  x: number
  y: number
}

/** How many written links run between two subjects. */
export interface SkyBridge { from: string; to: string; weight: number }

/** Where each system was put last time, by subject id. */
export type SkyPlacement = ReadonlyMap<string, { x: number; y: number }>

export interface SkyLayout {
  regions: SkyRegion[]
  stars: SkyStar[]
  bridges: SkyBridge[]
  bounds: { x: number; y: number; width: number; height: number }
}

const GOLDEN = 2.399963229728653
/** The space kept between two systems, enough for strays and a label. */
const GAP = 54
const STEPS = 420

/** Whether a page is a record kept as evidence rather than understanding. */
export function isEvidence(page: LibraryGraphPage): boolean {
  return kindOf(page) === 'source' || page.shelf === 'evidence'
}

/** A stable number from a string, so placement never depends on order. */
function hash(text: string): number {
  let value = 2166136261
  for (let index = 0; index < text.length; index++) {
    value ^= text.charCodeAt(index)
    value = Math.imul(value, 16777619)
  }
  return (value >>> 0) / 4294967296
}

function radiusOf(maintained: number, evidence: number): number {
  return 34 + Math.sqrt(maintained) * 10 + Math.sqrt(evidence) * 7
}

/** Spring the systems that link together, keep every pair apart, and settle. */
function place(regions: SkyRegion[], bridges: SkyBridge[], previous: SkyPlacement) {
  const order = [...regions].sort((a, b) => b.radius - a.radius || a.id.localeCompare(b.id))
  const at = new Map(regions.map(region => [region.id, region]))
  const springs = bridges.flatMap(bridge => {
    const a = at.get(bridge.from), b = at.get(bridge.to)
    return a && b ? [{ a, b, pull: Math.log1p(bridge.weight) }] : []
  })
  const separate = (strength: number) => {
    for (let i = 0; i < order.length; i++) {
      for (let j = i + 1; j < order.length; j++) {
        const a = order[i], b = order[j]
        const dx = b.x - a.x, dy = b.y - a.y
        const distance = Math.hypot(dx, dy) || 0.01
        const overlap = a.radius + b.radius + GAP - distance
        if (overlap <= 0) continue
        const push = overlap * strength / distance
        const share = b.radius / (a.radius + b.radius)
        a.x -= dx * push * share; a.y -= dy * push * share
        b.x += dx * push * (1 - share); b.y += dy * push * (1 - share)
      }
    }
  }
  if (order.some(region => previous.has(region.id))) {
    settleInto(order, springs, previous)
    for (let pass = 0; pass < 60; pass++) separate(1)
    return
  }
  order.forEach((region, index) => {
    const distance = 90 * Math.sqrt(index)
    region.x = Math.cos(index * GOLDEN) * distance
    region.y = Math.sin(index * GOLDEN) * distance
  })
  for (let step = 0; step < STEPS; step++) {
    const heat = 1 - step / STEPS
    for (const { a, b, pull } of springs) {
      const dx = b.x - a.x, dy = b.y - a.y
      const distance = Math.hypot(dx, dy) || 0.01
      const stretch = distance - (a.radius + b.radius + GAP)
      if (stretch <= 0) continue
      const move = Math.min(stretch / 2, stretch * 0.018 * pull) * heat / distance
      a.x += dx * move; a.y += dy * move
      b.x -= dx * move; b.y -= dy * move
    }
    for (const region of order) {
      region.x -= region.x * 0.012 * heat
      region.y -= region.y * 0.012 * heat
    }
    separate(0.5)
  }
  for (let pass = 0; pass < 60; pass++) separate(1)
}

/**
 * Puts every system where it was before. A new one starts among the systems it
 * links to, weighted by how much, or past the sky's edge if it links nowhere.
 */
function settleInto(order: SkyRegion[], springs: { a: SkyRegion; b: SkyRegion; pull: number }[], previous: SkyPlacement) {
  const known = order.filter(region => previous.has(region.id))
  for (const region of known) {
    const was = previous.get(region.id) as { x: number; y: number }
    region.x = was.x
    region.y = was.y
  }
  const edge = Math.max(...known.map(region => Math.hypot(region.x, region.y) + region.radius))
  for (const region of order.filter(entry => !previous.has(entry.id))) {
    let x = 0, y = 0, total = 0
    for (const { a, b, pull } of springs) {
      const other = a === region ? b : b === region ? a : null
      if (!other || !previous.has(other.id)) continue
      x += other.x * pull; y += other.y * pull; total += pull
    }
    const angle = hash(region.id) * Math.PI * 2
    const distance = total > 0 ? region.radius : edge + GAP + region.radius
    region.x = (total > 0 ? x / total : 0) + Math.cos(angle) * distance
    region.y = (total > 0 ? y / total : 0) + Math.sin(angle) * distance
  }
}

/**
 * The sky for a whole corpus; filters only hide what is drawn on it. Given the
 * placement of a sky drawn before, its systems keep their places.
 */
export function skyLayout(graph: Pick<LibraryGraph, 'pages' | 'links'>, previous: SkyPlacement = new Map()): SkyLayout {
  const subjectOfPath = new Map(graph.pages.map(page => [page.path, subjectOf(page)]))
  const degree = new Map<string, number>()
  const between = new Map<string, number>()
  for (const [from, to] of graph.links) {
    if (from === to || !subjectOfPath.has(from) || !subjectOfPath.has(to)) continue
    degree.set(from, (degree.get(from) ?? 0) + 1)
    degree.set(to, (degree.get(to) ?? 0) + 1)
    const a = subjectOfPath.get(from) as string, b = subjectOfPath.get(to) as string
    if (a === b) continue
    const key = a < b ? `${a}\n${b}` : `${b}\n${a}`
    between.set(key, (between.get(key) ?? 0) + 1)
  }
  const bridges = [...between].map(([key, weight]) => {
    const [from, to] = key.split('\n')
    return { from, to, weight }
  }).sort((a, b) => a.from.localeCompare(b.from) || a.to.localeCompare(b.to))

  const groups = new Map<string, LibraryGraphPage[]>()
  for (const page of graph.pages) {
    const id = subjectOfPath.get(page.path)
    if (!id) continue
    const group = groups.get(id)
    if (group) group.push(page)
    else groups.set(id, [page])
  }

  const linked = (page: LibraryGraphPage) => (degree.get(page.path) ?? 0) > 0
  const regions: SkyRegion[] = [...groups].sort(([a], [b]) => a.localeCompare(b)).map(([id, pages]) => {
    const name = id.split('/').pop() ?? id
    // The page that sums the subject up: its topic, or a page named for it.
    const topics = pages.filter(page => !isEvidence(page) && (kindOf(page) === 'topic' || page.slug === name))
    const core = topics.find(page => page.slug === name && kindOf(page) === 'topic')
      ?? topics.find(page => kindOf(page) === 'topic')
      ?? topics.find(page => linked(page))
    const evidence = pages.filter(isEvidence).length
    const maintained = pages.filter(page => !isEvidence(page) && page !== core && linked(page)).length
    return {
      id, name: subjectName(id), collection: id.includes('/') ? id.split('/')[0] : pages[0].shelf, x: 0, y: 0,
      radius: radiusOf(maintained + (core ? 1 : 0), evidence),
      core: core?.path ?? null, pages: pages.length, evidence,
    }
  })
  place(regions, bridges, previous)

  const stars: SkyStar[] = regions.flatMap(region => {
    const pages = groups.get(region.id) ?? []
    const turn = hash(region.id) * Math.PI * 2
    const placed: SkyStar[] = []
    const core = pages.find(page => page.path === region.core)
    if (core) placed.push({ page: core, subject: region.id, role: 'core', x: region.x, y: region.y })

    const orbit = pages.filter(page => page !== core && !isEvidence(page) && linked(page))
      .sort((a, b) => (degree.get(b.path) ?? 0) - (degree.get(a.path) ?? 0) || a.path.localeCompare(b.path))
    const dust = pages.filter(isEvidence).sort((a, b) => a.path.localeCompare(b.path))
    const inner = core ? 15 : 0
    const outer = region.radius * (dust.length > 0 ? 0.5 : 0.86)
    orbit.forEach((page, index) => {
      const distance = inner + (outer - inner) * Math.sqrt((index + 0.5) / orbit.length)
      const angle = turn + index * GOLDEN
      placed.push({ page, subject: region.id, role: 'star', x: region.x + Math.cos(angle) * distance, y: region.y + Math.sin(angle) * distance })
    })

    // Dust fills the ring outside the orbits evenly by area, with a little
    // scatter so it reads as a cloud rather than a pattern.
    const from = orbit.length > 0 || core ? region.radius * 0.5 : region.radius * 0.08
    const to = region.radius * 0.94
    dust.forEach((page, index) => {
      const share = (index + 0.5) / dust.length
      const jitter = 1 + (hash(page.path) - 0.5) * 0.08
      const distance = Math.sqrt(from * from + (to * to - from * from) * share) * jitter
      const angle = turn + index * GOLDEN + (hash(`${page.path}#`) - 0.5) * 0.35
      placed.push({ page, subject: region.id, role: 'dust', x: region.x + Math.cos(angle) * distance, y: region.y + Math.sin(angle) * distance })
    })

    // A maintained page with no link in or out has drifted out of its system;
    // in a system with nothing else in it, it is all there is, and stays.
    const lone = placed.length === 0
    for (const page of pages.filter(entry => entry !== core && !isEvidence(entry) && !linked(entry))) {
      const angle = hash(page.path) * Math.PI * 2
      const distance = lone ? region.radius * 0.5 * hash(`${page.path}~`)
        : region.radius + 12 + hash(`${page.path}~`) * (GAP - 26)
      placed.push({ page, subject: region.id, role: 'stray', x: region.x + Math.cos(angle) * distance, y: region.y + Math.sin(angle) * distance })
    }
    return placed
  })

  if (regions.length === 0) return { regions, stars, bridges, bounds: { x: -100, y: -100, width: 200, height: 200 } }
  const reach = (region: SkyRegion) => region.radius + GAP / 2
  const left = Math.min(...regions.map(region => region.x - reach(region)))
  const top = Math.min(...regions.map(region => region.y - reach(region)))
  const right = Math.max(...regions.map(region => region.x + reach(region)))
  const bottom = Math.max(...regions.map(region => region.y + reach(region)))
  return { regions, stars, bridges, bounds: { x: left, y: top, width: right - left, height: bottom - top } }
}

/** The places a sky gave its systems, to hand to the next one. */
export function placementOf(layout: SkyLayout): Map<string, { x: number; y: number }> {
  return new Map(layout.regions.map(region => [region.id, { x: region.x, y: region.y }]))
}
