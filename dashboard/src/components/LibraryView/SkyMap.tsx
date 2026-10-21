/**
 * The brain as a night sky.
 *
 * Each subject is a star system (skyLayout.ts): its topic the bright star at
 * the centre, its research and notes in orbit, its evidence a nebula whose
 * brightness is how much the subject rests on, and pages that link nowhere
 * drifting loose past its edge. Systems that cite each other sit together, so
 * the sky's constellations are the brain's real neighbourhoods, and what is
 * thin, crowded or out of place shows at a glance.
 *
 * It is made to be played with, and to settle well whenever anything changes:
 * - the sky gathers out of each system's centre when it is first drawn, and a
 *   star the corpus has moved travels to its new place rather than jumping;
 * - filters, the clock and a search fade stars rather than popping them, and
 *   the camera glides to whatever was chosen;
 * - a lantern follows the pointer and names the stars around it;
 * - choosing a page traces its links outward, and links that leave the screen
 *   become signposts at its edge that travel there;
 * - played through time, every page flares as it is born, and what changed in
 *   the last day breathes.
 * Motion is skipped for an operator who has asked the system for less of it.
 *
 * Painting is skyRenderer.ts. This file keeps the camera, the pointer and the
 * animation clocks, and runs a frame loop only while something is moving.
 */

import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { CSSProperties, MouseEvent } from 'react'
import type { LibraryGraph } from '../../library/libraryApi'
import { centreTransform, panBy, useMapTransform, zoomAbout, type MapPoint, type MapTransform } from '../../hooks/useMapTransform'
import { useTheme } from '../../theme/ThemeContext'
import { isEvidence, placementOf, skyLayout, type SkyPlacement, type SkyRegion, type SkyStar } from './skyLayout'
import { subjectHue } from './brainSubjects'
import { NOW, currentAt, momentOf } from './mapMoment'
import { paintSky, type PaintedLine, type PaintedRegion, type PaintedStar } from './skyRenderer'
import './SkyMap.css'

const LIMITS = { min: .08, max: 6 }
const DAY = 86_400_000
const SETTLE_MS = 900
const GLIDE_MS = 560
const FLARE_MS = 1200
const TRACE_MS = 520
const PULSE_MS = 2800
/** How quickly a star's light follows a change, in milliseconds to two thirds. */
const FADE_MS = 140
/** The lantern's reach around the pointer, and how many names it carries. */
const LANTERN = 110
const LANTERN_NAMES = 7
/** How many of a chosen page's neighbours are named; its lines show them all. */
const RELATED_NAMES = 14
/** Below this zoom only systems are named; above it their pages are too. */
const NEAR = .65
/** A bridge shows at overview once this many links run along it. */
const STRONG = 4
const MAX_FLARES = 400
/** How many of the last day's changes breathe at once. */
const FRESH = 40

/** Where the sky put its systems, kept so the next visit finds them in place. */
const PLACEMENT_KEY = 'chrote.brain.sky.placement'

function rememberedPlacement(): SkyPlacement {
  try {
    const parsed: unknown = JSON.parse(window.localStorage.getItem(PLACEMENT_KEY) || '{}')
    if (!parsed || typeof parsed !== 'object') return new Map()
    return new Map(Object.entries(parsed)
      .filter((entry): entry is [string, [number, number]] => Array.isArray(entry[1]) && entry[1].length === 2 && entry[1].every(Number.isFinite))
      .map(([id, [x, y]]) => [id, { x, y }]))
  } catch {
    return new Map()
  }
}

function rememberPlacement(placement: SkyPlacement) {
  try {
    window.localStorage.setItem(PLACEMENT_KEY, JSON.stringify(Object.fromEntries(
      [...placement].map(([id, at]) => [id, [at.x, at.y]]),
    )))
  } catch {
    // Private mode and quota failures cost only tomorrow's sky its places.
  }
}

interface Props {
  graph: LibraryGraph
  visible: LibraryGraph
  selected: string | null
  subject: string | null
  matches: ReadonlySet<string> | null
  hoverPath?: string | null
  moment: number
  camera: MapTransform | null
  cameraRevision: number
  onCameraChange: (camera: MapTransform) => void
  onSelect: (path: string) => void
  onSubject: (id: string) => void
}

interface Signpost { subject: string; name: string; count: number; target: SkyStar; x: number; y: number; angle: number; hue: string }

const ease = (t: number) => (t <= 0 ? 0 : t >= 1 ? 1 : 1 - (1 - t) ** 3)
const easeInOut = (t: number) => (t <= 0 ? 0 : t >= 1 ? 1 : t < .5 ? 4 * t * t * t : 1 - (-2 * t + 2) ** 3 / 2)
const blend = (a: MapTransform, b: MapTransform, t: number): MapTransform => ({
  x: a.x + (b.x - a.x) * t, y: a.y + (b.y - a.y) * t, scale: a.scale + (b.scale - a.scale) * t,
})

/** Where in its breath a fresh star starts, so they do not breathe together. */
function offsetOf(path: string): number {
  let value = 0
  for (let index = 0; index < path.length; index++) value = (value * 31 + path.charCodeAt(index)) % 9973
  return value / 9973
}

function useStillness(): boolean {
  const query = useMemo(() => (typeof window === 'undefined' ? null : window.matchMedia?.('(prefers-reduced-motion: reduce)') ?? null), [])
  const [still, setStill] = useState(query?.matches ?? false)
  useEffect(() => {
    if (!query) return
    const change = () => setStill(query.matches)
    query.addEventListener?.('change', change)
    return () => query.removeEventListener?.('change', change)
  }, [query])
  return still
}

export default function SkyMap({ graph, visible, selected, subject, matches, hoverPath, moment, camera, cameraRevision, onCameraChange, onSelect, onSubject }: Props) {
  const theme = useTheme()
  const still = useStillness()
  const canvas = useRef<HTMLCanvasElement>(null)
  const [size, setSize] = useState({ width: 960, height: 600, measured: false })
  const [hovered, setHovered] = useState<string | null>(null)
  const [hoveredRegion, setHoveredRegion] = useState<string | null>(null)
  const [pointer, setPointer] = useState<MapPoint | null>(null)
  // Each sky is placed from the one before it, so a page arriving or a refresh
  // moves only what it must.
  const placement = useRef<SkyPlacement | null>(null)
  const layout = useMemo(() => skyLayout(graph, placement.current ??= rememberedPlacement()), [graph])
  useEffect(() => {
    placement.current = placementOf(layout)
    rememberPlacement(placement.current)
  }, [layout])
  const view = useMapTransform<HTMLDivElement>({ ...size, limits: LIMITS, initialTransform: camera })
  const { ref, restore, transform } = view

  const visiblePaths = useMemo(() => new Set(visible.pages.map(page => page.path)), [visible.pages])
  const starOf = useMemo(() => new Map(layout.stars.map(star => [star.page.path, star])), [layout])
  const regionOf = useMemo(() => new Map(layout.regions.map(region => [region.id, region])), [layout])
  const shownRegions = useMemo(() => new Set(layout.stars.filter(star => visiblePaths.has(star.page.path)).map(star => star.subject)), [layout, visiblePaths])
  const hot = hovered ?? hoverPath ?? selected
  const related = useMemo(() => {
    if (!hot) return new Set<string>()
    return new Set(visible.links.flatMap(([a, b]) => (a === hot ? [b] : b === hot ? [a] : [])))
  }, [hot, visible.links])
  const near = useMemo(() => {
    const focus = hoveredRegion ?? subject
    if (!focus) return new Set<string>()
    return new Set(layout.bridges.flatMap(bridge => (bridge.from === focus ? [bridge.to] : bridge.to === focus ? [bridge.from] : [])))
  }, [hoveredRegion, layout.bridges, subject])
  const hue = useCallback((id: string) => subjectHue(id, theme.shelves), [theme.shelves])
  const loadedAt = useMemo(() => Date.now(), [graph])
  // What changed in the last day breathes: the most recent few maintained
  // pages, so a day of mass moves is still a handful of lights, not a storm.
  const fresh = useMemo(() => new Set(graph.pages
    .filter(page => !isEvidence(page) && loadedAt - momentOf(page.updated) < DAY)
    .sort((a, b) => momentOf(b.updated) - momentOf(a.updated)).slice(0, FRESH)
    .map(page => page.path)), [graph, loadedAt])

  // ---- Clocks. Everything that moves keeps its start here, and the frame
  // loop reads them; React state only says what the sky should come to.
  const frame = useRef(0)
  const pulseTimer = useRef(0)
  const transformNow = useRef(transform)
  const glide = useRef<{ from: MapTransform; to: MapTransform; start: number } | null>(null)
  const moves = useRef<{ from: Map<string, MapPoint>; start: number } | null>(null)
  const shown = useRef(new Map<string, MapPoint>())
  const light = useRef(new Map<string, number>())
  const flares = useRef(new Map<string, number>())
  const trace = useRef<{ path: string; start: number } | null>(null)
  const lastTick = useRef(0)
  const screen = useRef<{ star: SkyStar; x: number; y: number }[]>([])
  const scene = useRef<() => 'busy' | 'pulse' | 'idle'>(() => 'idle')

  const tick = useCallback(() => {
    frame.current = 0
    const state = scene.current()
    if (state === 'busy') frame.current = requestAnimationFrame(tick)
    else if (state === 'pulse' && !document.hidden) {
      window.clearTimeout(pulseTimer.current)
      pulseTimer.current = window.setTimeout(() => { if (!frame.current) frame.current = requestAnimationFrame(tick) }, 60)
    }
  }, [])
  const kick = useCallback(() => { if (!frame.current) frame.current = requestAnimationFrame(tick) }, [tick])
  useEffect(() => () => { cancelAnimationFrame(frame.current); frame.current = 0; window.clearTimeout(pulseTimer.current) }, [])
  useEffect(() => {
    const wake = () => { if (!document.hidden) kick() }
    document.addEventListener('visibilitychange', wake)
    return () => document.removeEventListener('visibilitychange', wake)
  }, [kick])

  const glideTo = useCallback((to: MapTransform) => {
    if (still) { glide.current = null; restore(to); return }
    glide.current = { from: transformNow.current, to, start: performance.now() }
    kick()
  }, [kick, restore, still])
  const centreAt = useCallback((point: MapPoint, scale: number) =>
    glideTo(centreTransform(point, scale, size, LIMITS)), [glideTo, size])
  const fitBox = useCallback((box: { x: number; y: number; width: number; height: number }, most = 1.5, pad = 40) => {
    const scale = Math.min((size.width - pad) / Math.max(box.width, 1), (size.height - pad) / Math.max(box.height, 1), most)
    return centreTransform({ x: box.x + box.width / 2, y: box.y + box.height / 2 }, scale, size, LIMITS)
  }, [size])
  const fit = useCallback((animate = true) => {
    const target = fitBox(layout.bounds)
    if (animate) glideTo(target)
    else restore(target)
  }, [fitBox, glideTo, layout.bounds, restore])
  const focusRegion = useCallback((region: SkyRegion) => {
    centreAt(region, Math.min((size.width - 60) / (region.radius * 2.3), (size.height - 70) / (region.radius * 2.3), 2.5))
  }, [centreAt, size])

  // A gesture takes the camera back from a glide at once.
  const handlers = useMemo(() => ({
    ...view.handlers,
    onPointerDown: (event: Parameters<typeof view.handlers.onPointerDown>[0]) => { glide.current = null; view.handlers.onPointerDown(event) },
  }), [view.handlers])
  useEffect(() => {
    const element = ref.current
    if (!element) return
    const stop = () => { glide.current = null }
    element.addEventListener('wheel', stop, { capture: true, passive: true })
    return () => element.removeEventListener('wheel', stop, { capture: true })
  }, [ref])

  useLayoutEffect(() => {
    const element = ref.current
    if (!element) return
    const observer = new ResizeObserver(([entry]) => {
      const { width, height } = entry.contentRect
      if (width > 0 && height > 0) setSize({ width, height, measured: true })
      kick()
    })
    observer.observe(element)
    return () => observer.disconnect()
  }, [kick, ref])

  // The camera follows what was chosen; a saved view or a restore is exact
  // and immediate, a new choice is travelled to.
  const applied = useRef<{ revision: number; subject: string | null; selected: string | null; width: number; height: number } | null>(null)
  useEffect(() => {
    if (!size.measured) return
    const previous = applied.current
    applied.current = { revision: cameraRevision, subject, selected, width: size.width, height: size.height }
    const star = selected ? starOf.get(selected) : null
    const region = subject ? regionOf.get(subject) : null
    if (!previous || previous.revision !== cameraRevision) {
      glide.current = null
      if (camera) restore(camera)
      else if (!previous && star) restore(centreTransform(star, .9, size, LIMITS))
      else if (!previous && region) restore(centreTransform(region, 1, size, LIMITS))
      else fit(previous !== null)
    } else if (selected !== previous.selected && star) centreAt(star, Math.max(.9, transformNow.current.scale))
    else if (subject !== previous.subject && region) focusRegion(region)
    else if (size.width !== previous.width || size.height !== previous.height) {
      restore(panBy(transformNow.current, (size.width - previous.width) / 2, (size.height - previous.height) / 2))
    }
  }, [camera, cameraRevision, centreAt, fit, focusRegion, regionOf, restore, selected, size, starOf, subject])
  useEffect(() => { if (size.measured) onCameraChange(transform) }, [onCameraChange, size.measured, transform])

  // A changed corpus: stars travel from where they were drawn, and a first
  // drawing gathers out of each system's centre.
  useEffect(() => {
    const from = new Map<string, MapPoint>()
    for (const star of layout.stars) {
      const was = shown.current.get(star.page.path)
      const region = regionOf.get(star.subject)
      from.set(star.page.path, was ?? { x: region?.x ?? star.x, y: region?.y ?? star.y })
    }
    moves.current = still ? null : { from, start: performance.now() }
    kick()
  }, [kick, layout, regionOf, still])

  // Played forward, every page flares as it arrives.
  const lastMoment = useRef(moment)
  useEffect(() => {
    const before = lastMoment.current
    lastMoment.current = moment
    if (still || moment <= before) return
    const until = moment === NOW ? loadedAt : moment
    const now = performance.now()
    graph.pages.filter(page => { const at = momentOf(page.created); return at > before && at <= until })
      .slice(-MAX_FLARES).forEach(page => flares.current.set(page.path, now))
    kick()
  }, [graph.pages, kick, loadedAt, moment, still])

  useEffect(() => {
    if (!selected) return
    trace.current = still ? null : { path: selected, start: performance.now() }
    kick()
  }, [kick, selected, still])

  // A search lights its stars and the camera takes them all in.
  useEffect(() => {
    if (!matches || matches.size === 0 || !size.measured) return
    const found = [...matches].map(path => starOf.get(path)).filter((star): star is SkyStar => Boolean(star))
    if (found.length === 0) return
    const xs = found.map(star => star.x), ys = found.map(star => star.y)
    const box = { x: Math.min(...xs) - 60, y: Math.min(...ys) - 60, width: Math.max(...xs) - Math.min(...xs) + 120, height: Math.max(...ys) - Math.min(...ys) + 120 }
    glideTo(fitBox(box, 1.6, 80))
    // The camera answers a new search, not a resize while it stands.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [matches, starOf, size.measured])

  const maxEvidence = useMemo(() => Math.max(1, ...layout.regions.map(region => region.evidence)), [layout])
  const strongest = useMemo(() => Math.max(1, ...layout.bridges.map(bridge => bridge.weight)), [layout])

  // The frame: everything the sky should look like now, from the props and
  // the clocks. Reassigned on every render; the loop always paints the latest.
  scene.current = () => {
    const element = canvas.current
    const context = element?.getContext('2d')
    if (!element || !context || element.clientWidth === 0) return 'idle'
    const now = performance.now()
    const step = lastTick.current ? Math.min(100, now - lastTick.current) : 16
    lastTick.current = now
    let busy = false

    let at = transformNow.current = transform
    if (glide.current) {
      const t = (now - glide.current.start) / GLIDE_MS
      at = blend(glide.current.from, glide.current.to, easeInOut(t))
      transformNow.current = at
      if (t >= 1) { const to = glide.current.to; glide.current = null; restore(to) }
      else { busy = true; restore(at) }
    }
    const s = at.scale
    const sx = (x: number) => x * s + at.x
    const sy = (y: number) => y * s + at.y
    const fade = 1 - Math.exp(-step / FADE_MS)
    const follow = (key: string, target: number) => {
      const was = light.current.get(key) ?? 0
      const next = still ? target : Math.abs(target - was) < .01 ? target : was + (target - was) * fade
      if (next !== target) busy = true
      light.current.set(key, next)
      return next
    }

    const move = moves.current
    const centre = Math.max(1, ...layout.regions.map(region => Math.hypot(region.x - layout.bounds.x - layout.bounds.width / 2, region.y - layout.bounds.y - layout.bounds.height / 2)))
    let moving = false
    const placeOf = (star: SkyStar): MapPoint => {
      if (!move) return star
      const from = move.from.get(star.page.path)
      if (!from) return star
      const region = regionOf.get(star.subject)
      const delay = region ? Math.hypot(region.x - layout.bounds.x - layout.bounds.width / 2, region.y - layout.bounds.y - layout.bounds.height / 2) / centre * 280 : 0
      const t = ease((now - move.start - delay - Math.hypot(star.x - from.x, star.y - from.y) * .4) / SETTLE_MS)
      if (t < 1) moving = true
      return { x: from.x + (star.x - from.x) * t, y: from.y + (star.y - from.y) * t }
    }

    const focus = hoveredRegion ?? subject
    const regions: PaintedRegion[] = layout.regions.map(region => ({
      x: sx(region.x), y: sy(region.y), radius: region.radius * s, hue: hue(region.id),
      nebula: Math.log1p(region.evidence) / Math.log1p(maxEvidence),
      edge: region.id === focus ? .55 : near.has(region.id) ? .3 : .12,
      alpha: follow(`region:${region.id}`, shownRegions.has(region.id) ? (matches || hot ? .55 : 1) : 0),
    })).filter(region => region.x + region.radius > 0 && region.x - region.radius < size.width
      && region.y + region.radius > 0 && region.y - region.radius < size.height)

    const bridges: PaintedLine[] = layout.bridges.flatMap(bridge => {
      const a = regionOf.get(bridge.from), b = regionOf.get(bridge.to)
      if (!a || !b || !shownRegions.has(a.id) || !shownRegions.has(b.id)) return []
      const involved = bridge.from === focus || bridge.to === focus
      const strength = Math.log1p(bridge.weight) / Math.log1p(strongest)
      const target = hot || matches ? (involved ? .25 : 0) : involved ? .32 + .4 * strength : bridge.weight >= STRONG ? .05 + .18 * strength : 0
      const alpha = follow(`bridge:${bridge.from}\n${bridge.to}`, target)
      if (alpha <= .01) return []
      return [{ ax: sx(a.x), ay: sy(a.y), bx: sx(b.x), by: sy(b.y), hue: hue(a.id), alpha, width: involved ? 1.2 : 1, reach: 1, bend: .12 }]
    })

    const placed = new Map<string, MapPoint>()
    const stars: PaintedStar[] = []
    const onScreen: { star: SkyStar; x: number; y: number }[] = []
    const growth = trace.current ? Math.min(1, (now - trace.current.start) / TRACE_MS) : 1
    if (growth < 1) busy = true
    let pulsing = false
    const nearSize = Math.sqrt(s)
    for (const star of layout.stars) {
      const path = star.page.path
      const where = placeOf(star)
      placed.set(path, where)
      let target = visiblePaths.has(path) ? (currentAt(momentOf(star.page.updated), moment) ? 1 : .35) : 0
      if (matches) target *= matches.has(path) ? 1 : .14
      if (hot) target *= path === hot || related.has(path) ? 1 : .3
      const alpha = follow(path, target)
      if (alpha <= .01) continue
      const x = sx(where.x), y = sy(where.y)
      if (x < -20 || x > size.width + 20 || y < -20 || y > size.height + 20) continue
      onScreen.push({ star, x, y })
      const flareAt = flares.current.get(path)
      let flare: number | null = null
      if (flareAt !== undefined) {
        flare = (now - flareAt) / FLARE_MS
        if (flare >= 1) { flares.current.delete(path); flare = null } else busy = true
      }
      let pulse: number | null = null
      if (!still && moment === NOW && fresh.has(path)) {
        const phase = ((now + offsetOf(path) * PULSE_MS) % PULSE_MS) / PULSE_MS * 1.7
        if (phase < 1) pulse = phase
        pulsing = true
      }
      const words = Math.log10((star.page.words ?? 0) + 10)
      const radius = star.role === 'core' ? Math.min(6, Math.max(2.6, 2 + 2.6 * nearSize))
        : star.role === 'dust' ? Math.min(2.4, Math.max(.9, 1.15 * nearSize))
          : Math.min(3.8, Math.max(1, (.6 + words * .5) * nearSize))
      stars.push({ x, y, role: star.role, hue: hue(star.subject), size: radius, alpha, match: matches?.has(path) ?? false, hot: path === hot, flare, pulse })
    }
    if (moving) busy = true
    else if (move) moves.current = null
    shown.current = placed
    screen.current = onScreen

    const links: PaintedLine[] = []
    const from = hot ? placed.get(hot) : undefined
    if (hot && from) {
      for (const other of related) {
        const to = placed.get(other)
        if (!to) continue
        links.push({ ax: sx(from.x), ay: sy(from.y), bx: sx(to.x), by: sy(to.y), hue: hue(starOf.get(hot)?.subject ?? ''),
          alpha: .7, width: 1, reach: trace.current?.path === hot ? ease(growth) : 1, bend: .05 })
      }
    }

    const dpr = window.devicePixelRatio || 1
    const width = Math.round(size.width * dpr), height = Math.round(size.height * dpr)
    if (element.width !== width || element.height !== height) { element.width = width; element.height = height }
    context.setTransform(dpr, 0, 0, dpr, 0, 0)
    paintSky(context, { width: size.width, height: size.height, regions, bridges, links, stars, accent: theme.ui.accent })
    return busy ? 'busy' : pulsing ? 'pulse' : 'idle'
  }
  useLayoutEffect(() => { kick() })

  // ---- Names. Systems are always named where there is room; pages are named
  // up close, around the lantern, and wherever a search or a choice lit them.
  const regionLabels = useMemo(() => {
    const occupied: { x: number; y: number; width: number; height: number }[] = []
    const order = [...layout.regions].filter(region => shownRegions.has(region.id))
      .sort((a, b) => Number(b.id === subject) - Number(a.id === subject) || b.radius - a.radius)
    return order.flatMap(region => {
      const x = region.x * transform.scale + transform.x
      const up = transform.scale >= NEAR
      const counted = up || region.id === (hoveredRegion ?? subject)
      const width = Math.min(150, Math.max(60, region.name.length * 7 + 12))
      const height = (region.name.length * 7 > width - 12 ? 34 : 18) + (counted ? 14 : 0)
      // Up close the name rides above the system; from afar it hangs under
      // the core star, clear of it so the star stays the thing to click.
      const y = up ? (region.y - region.radius) * transform.scale + transform.y - height / 2 - 2
        : region.y * transform.scale + transform.y + 13 + height / 2
      const box = { x: x - width / 2, y: y - height / 2, width, height }
      if (box.x < 4 || box.x + width > size.width - 4 || box.y < 4 || box.y + height > size.height - 4) return []
      if (occupied.some(other => box.x < other.x + other.width + 4 && box.x + width + 4 > other.x
        && box.y < other.y + other.height + 4 && box.y + height + 4 > other.y)) return []
      occupied.push(box)
      return [{ region, x, y, width, height, counted }]
    })
  }, [hoveredRegion, layout.regions, shownRegions, size, subject, transform])

  const labels = useMemo(() => {
    const occupied = regionLabels.map(label => ({ x: label.x - label.width / 2, y: label.y - label.height / 2, width: label.width }))
    const points = layout.stars.filter(star => visiblePaths.has(star.page.path)).map(star => ({
      star, x: star.x * transform.scale + transform.x, y: star.y * transform.scale + transform.y,
    })).filter(point => point.x > 8 && point.x < size.width - 8 && point.y > 8 && point.y < size.height - 8)
    const lantern = pointer ? points.map(point => ({ point, distance: Math.hypot(point.x - pointer.x, point.y - pointer.y) }))
      .filter(entry => entry.distance < LANTERN)
      .sort((a, b) => Number(a.point.star.role === 'dust') - Number(b.point.star.role === 'dust') || a.distance - b.distance)
      .slice(0, LANTERN_NAMES).map(entry => entry.point.star.page.path) : []
    const lit = new Set(lantern)
    // A well-linked page names only its nearest neighbours; the lines show the rest.
    const origin = hot ? starOf.get(hot) : null
    const named = new Set(origin ? [...related].map(path => starOf.get(path)).filter((star): star is SkyStar => Boolean(star))
      .sort((a, b) => Math.hypot(a.x - origin.x, a.y - origin.y) - Math.hypot(b.x - origin.x, b.y - origin.y))
      .slice(0, RELATED_NAMES).map(star => star.page.path) : [])
    const rank = (path: string, role: string, own: string) => path === hot ? 0 : matches?.has(path) ? 1 : lit.has(path) ? 2
      : named.has(path) ? 3 : own === subject && role !== 'dust' ? 4 : role === 'core' ? 5 : role === 'dust' ? 7 : 6
    const placed: { star: SkyStar; x: number; y: number; width: number; kind: 'hot' | 'lantern' | 'plain' }[] = []
    for (const point of [...points].sort((a, b) => rank(a.star.page.path, a.star.role, a.star.subject) - rank(b.star.page.path, b.star.role, b.star.subject))) {
      const path = point.star.page.path
      const level = rank(path, point.star.role, point.star.subject)
      // With a page chosen, its own neighbourhood is what gets named.
      const wanted = level <= 3 || (hot ? level === 5 && transform.scale >= NEAR
        : (transform.scale >= NEAR && level <= 6) || (transform.scale >= 3.5 && level === 7))
      if (!wanted) continue
      if (placed.length >= 60) break
      const width = Math.min(220, point.star.page.title.length * 6.6 + 18)
      const x = Math.min(point.x + 9, size.width - width - 8)
      const y = point.y - 9
      if (occupied.some(box => Math.abs(box.y - y) < 21 && x < box.x + box.width + 10 && x + width + 10 > box.x)) continue
      occupied.push({ x, y, width })
      placed.push({ star: point.star, x, y, width, kind: path === hot ? 'hot' : lit.has(path) && level === 2 ? 'lantern' : 'plain' })
    }
    return placed
  }, [hot, layout.stars, matches, pointer, regionLabels, related, size, starOf, subject, transform, visiblePaths])

  // Links that leave the screen from the chosen page become signposts at the
  // edge, one per subject they lead into.
  const signposts = useMemo((): Signpost[] => {
    const origin = selected ? starOf.get(selected) : null
    if (!origin || !selected) return []
    const from = { x: origin.x * transform.scale + transform.x, y: origin.y * transform.scale + transform.y }
    const inside = (x: number, y: number) => x >= 0 && x <= size.width && y >= 0 && y <= size.height
    const bySubject = new Map<string, { count: number; target: SkyStar; distance: number }>()
    for (const [a, b] of visible.links) {
      const other = a === selected ? b : b === selected ? a : null
      const star = other ? starOf.get(other) : null
      if (!star) continue
      const x = star.x * transform.scale + transform.x, y = star.y * transform.scale + transform.y
      if (inside(x, y)) continue
      const distance = Math.hypot(star.x - origin.x, star.y - origin.y)
      const held = bySubject.get(star.subject)
      if (!held) bySubject.set(star.subject, { count: 1, target: star, distance })
      else { held.count++; if (distance < held.distance) { held.target = star; held.distance = distance } }
    }
    const cx = inside(from.x, from.y) ? from.x : size.width / 2, cy = inside(from.x, from.y) ? from.y : size.height / 2
    const posts: Signpost[] = []
    for (const [id, { count, target }] of [...bySubject].sort((a, b) => b[1].count - a[1].count).slice(0, 10)) {
      const tx = target.x * transform.scale + transform.x, ty = target.y * transform.scale + transform.y
      const angle = Math.atan2(ty - cy, tx - cx)
      const inset = 22
      const reach = Math.min(
        Math.abs(Math.cos(angle)) > 1e-6 ? ((Math.cos(angle) > 0 ? size.width - inset : inset) - cx) / Math.cos(angle) : Infinity,
        Math.abs(Math.sin(angle)) > 1e-6 ? ((Math.sin(angle) > 0 ? size.height - inset : inset) - cy) / Math.sin(angle) : Infinity,
      )
      const half = (regionOf.get(id)?.name.length ?? 8) * 3.4 + 22
      const x = Math.min(size.width - half - 6, Math.max(half + 6, cx + Math.cos(angle) * reach))
      const y = cy + Math.sin(angle) * reach
      if (posts.some(post => Math.hypot(post.x - x, post.y - y) < 30)) continue
      posts.push({ subject: id, name: regionOf.get(id)?.name ?? id, count, target, x, y, angle, hue: hue(id) })
    }
    return posts
  }, [hue, regionOf, selected, size, starOf, transform, visible.links])

  const travel = (post: Signpost) => {
    const origin = selected ? starOf.get(selected) : null
    if (!origin) return
    const box = { x: Math.min(origin.x, post.target.x) - 50, y: Math.min(origin.y, post.target.y) - 50,
      width: Math.abs(origin.x - post.target.x) + 100, height: Math.abs(origin.y - post.target.y) + 100 }
    glideTo(fitBox(box, Math.max(.9, transformNow.current.scale), 120))
  }

  const hit = (event: MouseEvent): SkyStar | undefined => {
    const box = ref.current?.getBoundingClientRect()
    if (!box) return
    const x = event.clientX - box.left, y = event.clientY - box.top
    let best: { star: SkyStar; distance: number } | undefined
    for (const point of screen.current) {
      const distance = Math.hypot(point.x - x, point.y - y) + (point.star.role === 'dust' ? 5 : 0)
      if (distance < 12 && (!best || distance < best.distance)) best = { star: point.star, distance }
    }
    return best?.star
  }
  const regionAt = (event: MouseEvent): SkyRegion | undefined => {
    const box = ref.current?.getBoundingClientRect()
    if (!box) return
    const world = view.toWorld({ x: event.clientX - box.left, y: event.clientY - box.top })
    return layout.regions.find(entry => shownRegions.has(entry.id) && Math.hypot(entry.x - world.x, entry.y - world.y) <= entry.radius)
  }
  const pointerFrame = useRef(0)
  const track = (event: MouseEvent) => {
    const box = ref.current?.getBoundingClientRect()
    if (!box) return
    const at = { x: event.clientX - box.left, y: event.clientY - box.top }
    const star = hit(event)
    const region = star ? star.subject : regionAt(event)?.id ?? null
    if (pointerFrame.current) cancelAnimationFrame(pointerFrame.current)
    pointerFrame.current = requestAnimationFrame(() => {
      pointerFrame.current = 0
      setPointer(at)
      setHovered(star?.page.path ?? null)
      setHoveredRegion(region)
    })
  }

  const stars = layout.stars.filter(star => visiblePaths.has(star.page.path)).length
  return <div className="brain-continuous brain-sky" ref={ref} {...handlers} tabIndex={0}
    aria-label="The brain as a night sky. Drag to pan; scroll to zoom. Arrow keys pan; plus and minus zoom; Home fits."
    onKeyDown={event => {
      if (event.target !== event.currentTarget) return
      const movement = { ArrowLeft: [70, 0], ArrowRight: [-70, 0], ArrowUp: [0, 70], ArrowDown: [0, -70] }[event.key]
      if (movement) { event.preventDefault(); glide.current = null; restore(panBy(transformNow.current, movement[0], movement[1])) }
      if (['+', '=', '-'].includes(event.key)) {
        event.preventDefault()
        glideTo(zoomAbout(transformNow.current, { x: size.width / 2, y: size.height / 2 }, event.key === '-' ? .8 : 1.25, LIMITS))
      }
      if (event.key === 'Home') { event.preventDefault(); fit() }
    }} onMouseLeave={() => { setHovered(null); setHoveredRegion(null); setPointer(null) }}>
    <canvas ref={canvas} aria-hidden="true" className={hovered ? 'is-over-star' : ''} onMouseMove={track}
      onClick={event => {
        if (view.panned()) return
        const star = hit(event)
        if (star) { onSelect(star.page.path); return }
        const region = regionAt(event)
        if (region) { onSubject(region.id); focusRegion(region) }
      }} />
    <div className="brain-region-labels">
      {regionLabels.map(({ region, x, y, width, counted }) => {
        const count = (regionOf.get(region.id)?.pages ?? 0)
        return <button type="button" key={region.id}
          className={`brain-region-label${region.core ? '' : ' is-unsummed'}${near.has(region.id) ? ' is-near' : ''}${region.id === (hoveredRegion ?? subject) ? ' is-focus' : ''}`}
          title={`${region.collection} / ${region.name}${region.core ? '' : ' · no topic page yet'}`}
          style={{ left: x, top: y, width, '--subject-color': hue(region.id) } as CSSProperties}
          aria-label={`Explore ${region.name}, ${count} pages`} onClick={event => { if (event.detail === 0 || !view.panned()) { onSubject(region.id); focusRegion(region) } }}>
          <span>{region.name}</span>
          {counted && <small>{region.pages - region.evidence} pages{region.evidence > 0 ? ` · ${region.evidence} evidence` : ''}</small>}
        </button>
      })}
    </div>
    <div className="brain-continuous-pages">
      {labels.map(({ star, x, y, width, kind }) => <button type="button" key={star.page.path}
        className={`${kind === 'hot' ? 'is-hot' : kind === 'lantern' ? 'is-lantern' : ''}${star.role === 'stray' ? ' is-stray' : ''}`}
        style={{ left: x, top: y, maxWidth: width }} aria-label={star.page.title} aria-current={star.page.path === selected ? 'true' : undefined} title={star.role === 'stray' ? `${star.page.title} · links nowhere` : star.page.title}
        onFocus={() => setHovered(star.page.path)} onBlur={() => setHovered(null)}
        onClick={event => { if (event.detail === 0 || !view.panned()) onSelect(star.page.path) }}>{star.page.title}</button>)}
    </div>
    <div className="brain-signposts">
      {signposts.map(post => <button type="button" key={post.subject} className="brain-signpost"
        style={{ left: post.x, top: post.y, '--subject-color': post.hue, '--angle': `${post.angle}rad` } as CSSProperties}
        aria-label={`${post.count} ${post.count === 1 ? 'link leads' : 'links lead'} to ${post.name}`} onClick={() => travel(post)}>
        <i aria-hidden="true" />{post.name}<small>{post.count}</small>
      </button>)}
    </div>
    {stars === 0 && <p className="brain-map-empty">No pages match these filters.</p>}
    <span className="brain-map-scale">{Math.round(transform.scale * 100)}%</span>
  </div>
}
