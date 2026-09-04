/**
 * What the pointer lights.
 *
 * Pointing at a page used to light the page and the pages it touches, all at
 * one strength, against a corpus that stayed as bright as they were. It now
 * lights a neighbourhood: the page itself and everything one hop away at full
 * strength with a glow around it, everything two hops away dimmer, and the rest
 * of the corpus faded well back, so what the operator is asking about reads as
 * the lit thing on a dark ground. A hairline takes the strength of its brighter
 * end, so a link out of the neighbourhood is drawn as far as it goes.
 *
 * Pointing at a shelf instead — its name on the map, or its row in the rail —
 * lights that shelf and the links inside it, and dims everything else.
 *
 * All of it is arithmetic over the graph the server answered with: no DOM, no
 * canvas, nothing that runs on a clock. The light changes when the pointer
 * moves and at no other time.
 */

import type { LibraryGraph } from '../../library/libraryApi'

/** How far the light reaches: the page, its neighbours, and theirs. */
export const LIGHT_REACH = 2

/**
 * How brightly a page is drawn. 0 and 1 are the neighbourhood, 2 is its edge,
 * `out` is the corpus around it, and `null` is a map at rest, where every page
 * is drawn by its own age.
 */
export type MapLight = 0 | 1 | 2 | 'out' | null

/** The strengths the drawing uses for a lit map. */
export const LIT_ALPHA = 1
export const DIM_ALPHA = 0.45
export const FAINT_ALPHA = 0.08

/** What the map is being asked about: a page's neighbourhood, or one shelf. */
export interface MapLighting {
  /** How far each page is from what the pointer is on; null at rest. */
  depths: ReadonlyMap<string, number> | null
  /** The shelf being pointed at, which is lit alone. */
  solo: string | null
}

export const AT_REST: MapLighting = { depths: null, solo: null }

/**
 * Every page's neighbours, read once per graph. Walking the whole link list for
 * each page in turn is what the map used to do, and at a corpus of thousands it
 * is the hover's whole cost.
 */
export function adjacencyOf(graph: LibraryGraph): Map<string, string[]> {
  const found = new Map<string, string[]>()
  const join = (from: string, to: string) => {
    const carried = found.get(from)
    if (carried) carried.push(to)
    else found.set(from, [to])
  }
  graph.links.forEach(([from, to]) => { join(from, to); join(to, from) })
  graph.tags.forEach(([from, to]) => { join(from, to); join(to, from) })
  return found
}

/**
 * How far each page is from the pages the light starts at, up to `reach`. A
 * page the light does not reach is not in the answer.
 */
export function depthsFrom(
  adjacency: ReadonlyMap<string, string[]>,
  from: Iterable<string>,
  reach = LIGHT_REACH,
): Map<string, number> {
  const depths = new Map<string, number>()
  let edge: string[] = []
  for (const path of from) {
    if (depths.has(path)) continue
    depths.set(path, 0)
    edge.push(path)
  }
  for (let depth = 1; depth <= reach && edge.length > 0; depth++) {
    const next: string[] = []
    edge.forEach(path => {
      adjacency.get(path)?.forEach(other => {
        if (depths.has(other)) return
        depths.set(other, depth)
        next.push(other)
      })
    })
    edge = next
  }
  return depths
}

/** How brightly one page is drawn. */
export function lightOf(page: { path: string; shelf: string }, lighting: MapLighting): MapLight {
  if (lighting.solo !== null) return page.shelf === lighting.solo ? null : 'out'
  if (!lighting.depths) return null
  const depth = lighting.depths.get(page.path)
  if (depth === undefined) return 'out'
  return depth >= LIGHT_REACH ? LIGHT_REACH : (depth as 0 | 1)
}

/**
 * How brightly a hairline is drawn: as its brighter end, so a link that leaves
 * the neighbourhood is followed as far as it goes. A shelf pointed at is a
 * different question — what is asked for is the shelf, not what it reaches —
 * so there a hairline is drawn only if it stays inside.
 */
export function edgeLight(from: MapLight, to: MapLight, lighting: MapLighting): MapLight {
  if (lighting.solo !== null) return from === null && to === null ? null : 'out'
  const rank = (light: MapLight) => (light === null ? -1 : light === 'out' ? 9 : light)
  return rank(from) <= rank(to) ? from : to
}

/** The strength a light is drawn at, or null to keep the page's own age. */
export function alphaOf(light: MapLight): number | null {
  if (light === null) return null
  if (light === 'out') return FAINT_ALPHA
  return light === LIGHT_REACH ? DIM_ALPHA : LIT_ALPHA
}

/** Whether a page at this strength carries the glow that marks the light. */
export function glows(light: MapLight): boolean {
  return light === 0 || light === 1
}
