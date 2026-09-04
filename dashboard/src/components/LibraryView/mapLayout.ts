/**
 * Where the map puts things.
 *
 * Pure arithmetic over the graph the server answers with: no DOM, no
 * dependency, and no randomness that is not seeded. The same corpus at the
 * same size lays out the same way every time, so the map is a place the
 * operator learns rather than a picture that reshuffles under him.
 *
 * Two layouts live here. The full map clusters every page around its shelf's
 * anchor with a small force simulation. The strip puts the open page at the
 * left and its neighbours in rows beside it, which needs no simulation at all.
 * Both hand their nodes to the one label rule.
 */

import type { LibraryGraph, LibraryGraphPage } from '../../library/libraryApi'

export interface MapNode {
  path: string
  shelf: string
  title: string
  x: number
  y: number
  r: number
  /** 1 for a page that moved today, down to 0.35 after forty days. */
  opacity: number
  /** When git last saw the page, as the graph gives it; '' if never. */
  updated: string
  candidate: boolean
}

export interface MapEdge {
  from: string
  to: string
  /** A shared tag rather than a written link. */
  tag: boolean
}

/** A shelf's label, above its cluster. */
export interface MapCluster {
  shelf: string
  count: number
  x: number
  y: number
}

export interface MapLayout {
  nodes: MapNode[]
  edges: MapEdge[]
  clusters: MapCluster[]
  /** Neighbours the strip had no room for. */
  more: number
}

export interface MapLabel {
  path: string
  text: string
  x: number
  y: number
  /** The page the operator is on or pointing at, or one his search found. */
  primary: boolean
}

const DAY = 86_400_000
/** How many days it takes a page to fade to the floor. */
const FADE_DAYS = 40
const OPACITY_FLOOR = 0.35

const MIN_RADIUS = 3
const MAX_RADIUS = 9

/** Room for the top row's cluster label, and a margin elsewhere. */
const MAP_TOP = 40
const MAP_BOTTOM = 24
const MAP_SIDE = 40

/**
 * Label metrics: a monospaced 12px and 11px face, one line apart. The advance
 * is 0.6em on paper, but the browser rounds it up at these sizes, so the
 * estimate errs wide and keeps a little air between neighbours.
 */
const PRIMARY_CHAR = 7.6
const DIM_CHAR = 7
const CLUSTER_CHAR = 8.2
const LABEL_HALF = 7
const LABEL_GAP = 9
const LABEL_PAD = 6
export const LABEL_LINE = 13

/** How hard a page is held to its shelf, and how hard a link pulls. A link
 * across shelves is drawn long rather than allowed to drag two clusters into
 * one. */
const ANCHOR_PULL = 0.05
const LINK_PULL = 0.03
const CROSS_SHELF_PULL = 0.008
const LINK_LENGTH = 90
const TAG_LENGTH = 120
const CROSS_SHELF_LENGTH = 160

/** Labels the full map carries beyond the hot ones. */
export const LANDMARK_LABELS = 12
export const MAP_LABEL_CHARS = 26

/** How deterministic randomness is seeded: one constant, the same every run. */
const SEED = 0x9e3779b9

/** mulberry32: a small seeded generator, uniform on [0, 1). */
function seeded(seed: number): () => number {
  let state = seed >>> 0
  return () => {
    state = (state + 0x6d2b79f5) >>> 0
    let t = state
    t = Math.imul(t ^ (t >>> 15), t | 1)
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61)
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

/** How far back the map counts a page as recent. */
export type RecencyWindow = 'day' | 'week' | 'month' | 'all'

/** The windows the map bar offers, in the order it offers them. */
export const RECENCY_WINDOWS: readonly { id: RecencyWindow; label: string; days: number }[] = [
  { id: 'day', label: 'Day', days: 1 },
  { id: 'week', label: 'Week', days: 7 },
  { id: 'month', label: 'Month', days: 30 },
  { id: 'all', label: 'All', days: 0 },
]

/**
 * Whether a page moved inside the window. `all` holds everything, including
 * a page git never dated; any narrower window drops that page, because a
 * date nobody knows is not a date inside seven days.
 */
export function withinWindow(updated: string, window: RecencyWindow, now: number): boolean {
  if (window === 'all') return true
  const days = RECENCY_WINDOWS.find(entry => entry.id === window)?.days ?? 0
  if (days <= 0) return true
  const at = Date.parse(updated)
  if (!updated || Number.isNaN(at)) return false
  return now - at <= days * DAY
}

export function nodeRadius(words: number): number {
  return Math.max(MIN_RADIUS, Math.min(MAX_RADIUS, Math.sqrt(Math.max(0, words)) / 2.2))
}

/** A page git has never dated is drawn at the floor, like one it forgot. */
export function nodeOpacity(updated: string, now: number): number {
  const at = Date.parse(updated)
  if (!updated || Number.isNaN(at)) return OPACITY_FLOOR
  const days = Math.max(0, now - at) / DAY
  return Math.max(OPACITY_FLOOR, 1 - days / FADE_DAYS)
}

function node(page: LibraryGraphPage, x: number, y: number, now: number, r = nodeRadius(page.words)): MapNode {
  return {
    path: page.path,
    shelf: page.shelf,
    title: page.title,
    x,
    y,
    r,
    opacity: nodeOpacity(page.updated, now),
    updated: page.updated,
    candidate: page.candidate,
  }
}

function edgesAmong(graph: LibraryGraph, present: ReadonlySet<string>): MapEdge[] {
  const edges: MapEdge[] = []
  graph.links.forEach(([from, to]) => {
    if (present.has(from) && present.has(to)) edges.push({ from, to, tag: false })
  })
  graph.tags.forEach(([from, to]) => {
    if (present.has(from) && present.has(to)) edges.push({ from, to, tag: true })
  })
  return edges
}

/** Every page one link or one shared tag away, in the order the graph names them. */
export function neighboursOf(graph: LibraryGraph, path: string): string[] {
  const found: string[] = []
  const seen = new Set<string>([path])
  const add = (other: string) => {
    if (seen.has(other)) return
    seen.add(other)
    found.push(other)
  }
  graph.links.forEach(([from, to]) => {
    if (from === path) add(to)
    else if (to === path) add(from)
  })
  graph.tags.forEach(([from, to]) => {
    if (from === path) add(to)
    else if (to === path) add(from)
  })
  return found
}

/**
 * The shelves' anchors: a grid as square as the count allows, in shelf-name
 * order so the map reads like the rail, with a short last row centred.
 */
function shelfAnchors(shelves: string[], width: number, height: number): Map<string, [number, number]> {
  const anchors = new Map<string, [number, number]>()
  const columns = Math.max(1, Math.ceil(Math.sqrt(shelves.length)))
  const rows = Math.max(1, Math.ceil(shelves.length / columns))
  const cellWidth = (width - 2 * MAP_SIDE) / columns
  const cellHeight = (height - MAP_TOP - MAP_BOTTOM) / rows
  shelves.forEach((shelf, index) => {
    const row = Math.floor(index / columns)
    const inRow = row === rows - 1 ? shelves.length - row * columns : columns
    const column = index - row * columns + (columns - inRow) / 2
    anchors.set(shelf, [MAP_SIDE + (column + 0.5) * cellWidth, MAP_TOP + (row + 0.5) * cellHeight])
  })
  return anchors
}

/**
 * The full map. Pages sit in clusters around their shelf's anchor; links pull
 * their ends together, every pair keeps its distance, and the whole thing
 * cools over a fixed number of steps from a seeded start.
 */
export function layoutMap(graph: LibraryGraph, width: number, height: number, now = Date.now()): MapLayout {
  const pages = graph.pages.filter(page => page.shelf !== '').slice().sort((a, b) => (a.path < b.path ? -1 : 1))
  const shelves = Array.from(new Set(pages.map(page => page.shelf))).sort()
  const anchors = shelfAnchors(shelves, width, height)
  const random = seeded(SEED)
  const spread = Math.min((width - 2 * MAP_SIDE) / Math.max(1, Math.ceil(Math.sqrt(shelves.length))), height - MAP_TOP - MAP_BOTTOM) * 0.5

  const nodes = pages.map(page => {
    const [ax, ay] = anchors.get(page.shelf) ?? [width / 2, height / 2]
    return node(page, ax + (random() - 0.5) * spread, ay + (random() - 0.5) * spread, now)
  })
  const index = new Map(nodes.map((entry, position) => [entry.path, position]))
  const present = new Set(index.keys())
  const edges = edgesAmong(graph, present)
  const springs = edges.map(edge => {
    const a = index.get(edge.from) as number
    const b = index.get(edge.to) as number
    const crosses = nodes[a].shelf !== nodes[b].shelf
    return {
      a,
      b,
      want: crosses ? CROSS_SHELF_LENGTH : edge.tag ? TAG_LENGTH : LINK_LENGTH,
      pull: crosses ? CROSS_SHELF_PULL : LINK_PULL,
    }
  })

  // Enough steps for a corpus of dozens to settle; fewer for one of thousands,
  // where the pair check is what costs.
  const steps = Math.max(60, Math.min(400, Math.round(40_000 / Math.max(1, nodes.length))))
  for (let step = 0; step < steps; step++) {
    const k = 0.9 - (0.8 * step) / steps
    nodes.forEach(entry => {
      const [ax, ay] = anchors.get(entry.shelf) ?? [width / 2, height / 2]
      entry.x += (ax - entry.x) * ANCHOR_PULL * k
      entry.y += (ay - entry.y) * ANCHOR_PULL * k
    })
    for (let a = 0; a < nodes.length; a++) {
      for (let b = a + 1; b < nodes.length; b++) {
        const A = nodes[a]
        const B = nodes[b]
        const dx = B.x - A.x
        const dy = B.y - A.y
        const d2 = dx * dx + dy * dy + 0.01
        const min = A.r + B.r + 14
        if (d2 < min * min * 4) {
          const d = Math.sqrt(d2)
          const f = ((min * 2 - d) / d) * 0.25 * k
          A.x -= dx * f
          A.y -= dy * f
          B.x += dx * f
          B.y += dy * f
        }
      }
    }
    springs.forEach(({ a, b, want, pull }) => {
      const A = nodes[a]
      const B = nodes[b]
      const dx = B.x - A.x
      const dy = B.y - A.y
      const d = Math.sqrt(dx * dx + dy * dy) + 0.01
      const f = ((d - want) / d) * pull * k
      A.x += dx * f
      A.y += dy * f
      B.x -= dx * f
      B.y -= dy * f
    })
    nodes.forEach(entry => {
      entry.x = Math.max(MAP_SIDE, Math.min(width - MAP_SIDE, entry.x))
      entry.y = Math.max(MAP_TOP, Math.min(height - MAP_BOTTOM, entry.y))
    })
  }
  nodes.forEach(entry => {
    entry.x = Math.round(entry.x * 10) / 10
    entry.y = Math.round(entry.y * 10) / 10
  })

  const clusters = shelves.map(shelf => {
    const members = nodes.filter(entry => entry.shelf === shelf)
    const x = members.reduce((sum, entry) => sum + entry.x, 0) / members.length
    const y = Math.max(12, Math.min(...members.map(entry => entry.y)) - 16)
    return { shelf, count: members.length, x: Math.round(x * 10) / 10, y: Math.round(y * 10) / 10 }
  })

  return { nodes, edges, clusters, more: 0 }
}

/** The strip's rows, and the least a column may take. */
const STRIP_ROWS = 3
const STRIP_MIN_STEP = 96
const STRIP_MAX_STEP = 200
const STRIP_NEIGHBOUR_RADIUS = 5
const STRIP_OPEN_RADIUS = 9

/**
 * The strip: the open page at the left, its neighbours in three rows beside
 * it, column by column. Neighbours beyond what the width holds are counted
 * rather than crowded.
 */
export function layoutStrip(graph: LibraryGraph, openPath: string, width: number, height: number, now = Date.now()): MapLayout {
  const byPath = new Map(graph.pages.map(page => [page.path, page]))
  const open = byPath.get(openPath)
  if (!open) return { nodes: [], edges: [], clusters: [], more: 0 }

  const neighbours = neighboursOf(graph, openPath).filter(path => byPath.has(path))
  const left = Math.round(width * 0.16)
  const start = Math.round(width * 0.3)
  const available = Math.max(0, width - start - MAP_SIDE - MAP_LABEL_CHARS * PRIMARY_CHAR)
  const columnsThatFit = Math.max(1, Math.floor(available / STRIP_MIN_STEP) + 1)
  const shown = neighbours.slice(0, columnsThatFit * STRIP_ROWS)
  const columns = Math.max(1, Math.ceil(shown.length / STRIP_ROWS))
  const step = Math.max(STRIP_MIN_STEP, Math.min(STRIP_MAX_STEP, columns > 1 ? available / (columns - 1) : STRIP_MAX_STEP))
  const rowsY = [32, Math.round(height / 2), height - 28]

  const nodes: MapNode[] = [node(open, left, Math.round(height / 2), now, STRIP_OPEN_RADIUS)]
  shown.forEach((path, position) => {
    const page = byPath.get(path) as LibraryGraphPage
    const column = Math.floor(position / STRIP_ROWS)
    const row = position % STRIP_ROWS
    nodes.push(node(page, Math.round(start + column * step), rowsY[row], now, STRIP_NEIGHBOUR_RADIUS))
  })
  const present = new Set(nodes.map(entry => entry.path))
  return { nodes, edges: edgesAmong(graph, present), clusters: [], more: neighbours.length - shown.length }
}

interface Box {
  left: number
  right: number
  top: number
  bottom: number
}

function overlaps(a: Box, b: Box): boolean {
  return a.left < b.right && b.left < a.right && a.top < b.bottom && b.top < a.bottom
}

function truncate(title: string, maxChars: number): string {
  return title.length > maxChars ? `${title.slice(0, Math.max(1, maxChars - 1))}…` : title
}

/** The box a shelf's label takes, which no page's name may sit on. */
function clusterBox(cluster: MapCluster): Box {
  const width = `${cluster.shelf} · ${cluster.count}`.length * CLUSTER_CHAR + LABEL_PAD
  return { left: cluster.x - width / 2, right: cluster.x + width / 2, top: cluster.y - 11, bottom: cluster.y + 3 }
}

/**
 * Which pages are named, and where the name goes.
 *
 * Hot pages — the one on the table, the one under the pointer, their
 * neighbours, and what a search found — are always named. After them the
 * largest accepted pages are named as landmarks, up to `landmarks` of them.
 * A name that would sit on another, or on a shelf's label, moves down one
 * line; one that still collides is dropped, unless it is hot, in which case
 * the operator asked for it and it stays on the moved line.
 */
export function placeLabels(
  nodes: readonly MapNode[],
  hot: ReadonlySet<string>,
  primary: ReadonlySet<string>,
  options: { landmarks: number; maxChars: number },
  clusters: readonly MapCluster[] = [],
): MapLabel[] {
  const placed: MapLabel[] = []
  const boxes: Box[] = clusters.map(clusterBox)

  const tryPlace = (entry: MapNode, isHot: boolean): void => {
    const isPrimary = primary.has(entry.path)
    const text = truncate(entry.title, options.maxChars)
    const charWidth = isPrimary ? PRIMARY_CHAR : DIM_CHAR
    const left = entry.x + entry.r + LABEL_GAP
    const right = left + text.length * charWidth + LABEL_PAD
    const box = (y: number): Box => ({ left, right, top: y - LABEL_HALF, bottom: y + LABEL_HALF })
    const clear = (candidate: Box) => !boxes.some(other => overlaps(other, candidate))

    let y = entry.y
    let fits = clear(box(y))
    if (!fits) {
      y = entry.y + LABEL_LINE
      fits = clear(box(y))
    }
    if (!fits && !isHot) return
    boxes.push(box(y))
    placed.push({ path: entry.path, text, x: left, y: y + 4, primary: isPrimary })
  }

  nodes.filter(entry => hot.has(entry.path)).forEach(entry => tryPlace(entry, true))

  const landmarks = nodes
    .filter(entry => !hot.has(entry.path) && !entry.candidate)
    .sort((a, b) => b.r - a.r || (a.path < b.path ? -1 : 1))
    .slice(0, options.landmarks)
  landmarks.forEach(entry => tryPlace(entry, false))

  return placed
}
