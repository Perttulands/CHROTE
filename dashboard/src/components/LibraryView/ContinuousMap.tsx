import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { CSSProperties, MouseEvent } from 'react'
import type { LibraryGraph } from '../../library/libraryApi'
import { useMapTransform, panBy, zoomAbout, type MapTransform } from '../../hooks/useMapTransform'
import { useTheme } from '../../theme/ThemeContext'
import { continuousLayout, type PagePoint, type Region } from './continuousLayout'
import { kindOf, subjectHue } from './brainSubjects'
import { currentAt, momentOf } from './mapMoment'
import './ContinuousMap.css'

const LIMITS = { min: .08, max: 5 }
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

export default function ContinuousMap({ graph, visible, selected, subject, matches, hoverPath, moment, camera, cameraRevision, onCameraChange, onSelect, onSubject }: Props) {
  const theme = useTheme()
  const canvas = useRef<HTMLCanvasElement>(null)
  const [size, setSize] = useState({ width: 960, height: 600, measured: false })
  const [hovered, setHovered] = useState<string | null>(null)
  const layout = useMemo(() => continuousLayout(graph.pages), [graph.pages])
  const view = useMapTransform<HTMLDivElement>({ ...size, limits: LIMITS, initialTransform: camera })
  const { ref, restore, transform, centreOn } = view
  const visiblePaths = useMemo(() => new Set(visible.pages.map(page => page.path)), [visible.pages])
  const points = useMemo(() => layout.points.filter(point => visiblePaths.has(point.page.path)), [layout, visiblePaths])
  const positions = useMemo(() => new Map(layout.points.map(point => [point.page.path, point])), [layout])
  const regions = useMemo(() => layout.regions.filter(region => region.pages.some(page => visiblePaths.has(page.path))), [layout, visiblePaths])
  const hot = hovered ?? hoverPath ?? selected
  const links = useMemo(() => hot ? visible.links.filter(([a, b]) => a === hot || b === hot) : [], [hot, visible.links])
  const related = useMemo(() => new Set(links.flat()), [links])
  const fit = useCallback(() => {
    const { bounds } = layout
    const scale = Math.min((size.width - 40) / bounds.width, (size.height - 40) / bounds.height, 1.5)
    centreOn({ x: bounds.x + bounds.width / 2, y: bounds.y + bounds.height / 2 }, scale)
  }, [centreOn, layout, size])
  useLayoutEffect(() => {
    const element = ref.current
    if (!element) return
    const observer = new ResizeObserver(([entry]) => {
      const { width, height } = entry.contentRect
      if (width > 0 && height > 0) setSize({ width, height, measured: true })
    })
    observer.observe(element)
    return () => observer.disconnect()
  }, [ref])
  const focusRegion = useCallback((region: Region) => {
    const scale = Math.min((size.width - 50) / (region.radius * 2), (size.height - 60) / (region.radius * 2), 2.5)
    centreOn(region, scale)
  }, [centreOn, size])
  const applied = useRef<{ revision: number; subject: string | null; selected: string | null; width: number; height: number } | null>(null)
  useEffect(() => {
    if (!size.measured) return
    const previous = applied.current
    applied.current = { revision: cameraRevision, subject, selected, width: size.width, height: size.height }
    const point = selected ? positions.get(selected) : null
    const region = regions.find(entry => entry.id === subject)
    if (!previous || previous.revision !== cameraRevision) {
      if (camera) restore(camera)
      else if (!previous && point) centreOn(point, .9)
      else if (!previous && region) focusRegion(region)
      else fit()
    } else if (selected !== previous.selected && point) centreOn(point, Math.max(.9, transform.scale))
    else if (subject !== previous.subject && region) focusRegion(region)
    else if (size.width !== previous.width || size.height !== previous.height) {
      restore(panBy(transform, (size.width - previous.width) / 2, (size.height - previous.height) / 2))
    }
  }, [camera, cameraRevision, centreOn, fit, focusRegion, positions, regions, restore, selected, size, subject, transform])
  useEffect(() => { if (size.measured) onCameraChange(transform) }, [onCameraChange, size.measured, transform])

  const regionLabels = useMemo(() => {
    const occupied: { x: number; y: number; width: number; height: number }[] = []
    return regions.flatMap(region => {
      const x = region.x * transform.scale + transform.x
      const y = (region.y - (transform.scale >= .65 || region.id === subject ? region.radius - 12 : 0)) * transform.scale + transform.y
      const width = Math.min(150, Math.max(95, region.radius * transform.scale * 1.8))
      const height = region.name.length * 7 > width - 14 ? 54 : 39
      const box = { x: x - width / 2, y: y - height / 2, width, height }
      if (box.x < 4 || box.x + width > size.width - 4 || box.y < 4 || box.y + height > size.height - 4) return []
      if (occupied.some(other => box.x < other.x + other.width + 5 && box.x + width + 5 > other.x
        && box.y < other.y + other.height + 5 && box.y + height + 5 > other.y)) return []
      occupied.push(box)
      return [{ region, x, y, width, height }]
    })
  }, [regions, size, subject, transform])

  const screenPoints = useMemo(() => points.map(point => ({ ...point, screen: {
    x: point.x * transform.scale + transform.x, y: point.y * transform.scale + transform.y,
  } })).filter(point => point.screen.x > 12 && point.screen.x < size.width - 12 && point.screen.y > 12 && point.screen.y < size.height - 12), [points, size, transform])
  const labels = useMemo(() => {
    if (transform.scale < .65 && !subject && !hot && !matches) return []
    const occupied: { x: number; y: number; width: number }[] = regionLabels.map(label => ({ x: label.x - label.width / 2, y: label.y - label.height / 2, width: label.width }))
    const placed: { x: number; y: number; width: number }[] = []
    return [...screenPoints].sort((a, b) => Number(b.page.path === hot) - Number(a.page.path === hot)
      || Number(matches?.has(b.page.path) ?? false) - Number(matches?.has(a.page.path) ?? false)
      || Number(b.subject === subject) - Number(a.subject === subject)
      || Number(kindOf(b.page) === 'topic') - Number(kindOf(a.page) === 'topic'))
      .filter(point => {
        if (transform.scale < .65 && point.subject !== subject && point.page.path !== hot && !matches?.has(point.page.path)) return false
        const width = Math.min(210, point.page.title.length * 6.5 + 18)
        const x = Math.min(point.screen.x + 8, size.width - width - 8)
        const y = point.screen.y - 9
        if (occupied.length >= 65 || occupied.some(box => Math.abs(box.y - y) < 23 && x < box.x + box.width + 12 && x + width + 12 > box.x)) return false
        occupied.push({ x, y, width }); placed.push({ x, y, width })
        return true
      }).map((point, index) => ({ point, ...placed[index] }))
  }, [hot, matches, regionLabels, screenPoints, size.width, subject, transform.scale])

  useEffect(() => {
    const element = canvas.current
    const context = element?.getContext('2d')
    if (!element || !context) return
    const dpr = window.devicePixelRatio || 1
    element.width = Math.round(size.width * dpr)
    element.height = Math.round(size.height * dpr)
    context.scale(dpr, dpr)
    context.clearRect(0, 0, size.width, size.height)
    const hue = (id: string) => subjectHue(id, theme.shelves)
    for (const region of regions) {
      const x = region.x * transform.scale + transform.x
      const y = region.y * transform.scale + transform.y
      const radius = region.radius * transform.scale
      context.beginPath()
      context.arc(x, y, radius, 0, Math.PI * 2)
      context.fillStyle = hue(region.id)
      context.globalAlpha = .045
      context.fill()
      context.globalAlpha = region.id === subject ? .65 : .25
      context.strokeStyle = hue(region.id)
      context.lineWidth = 1
      context.setLineDash([2, 5])
      context.stroke()
    }
    context.setLineDash([])
    for (const [from, to] of links) {
      const a = positions.get(from), b = positions.get(to)
      if (!a || !b) continue
      context.beginPath()
      context.moveTo(a.x * transform.scale + transform.x, a.y * transform.scale + transform.y)
      context.lineTo(b.x * transform.scale + transform.x, b.y * transform.scale + transform.y)
      context.strokeStyle = hue(a.subject)
      context.globalAlpha = .55
      context.stroke()
    }
    for (const point of screenPoints) {
      const { x, y } = point.screen
      const kind = kindOf(point.page)
      const active = point.page.path === hot
      const radius = active ? 5 : kind === 'topic' ? Math.max(1.7, Math.min(3.8, transform.scale * 12)) : Math.max(.8, Math.min(2.2, transform.scale * 8))
      context.globalAlpha = (matches && !matches.has(point.page.path)) || (hot && point.page.path !== hot && !related.has(point.page.path)) ? .2 : currentAt(momentOf(point.page.updated), moment) ? .85 : .3
      context.fillStyle = hue(point.subject)
      context.beginPath()
      if (kind === 'research') context.rect(x - radius, y - radius, radius * 2, radius * 2)
      else if (kind === 'source') { context.moveTo(x, y - radius - 1); context.lineTo(x + radius + 1, y); context.lineTo(x, y + radius + 1); context.lineTo(x - radius - 1, y); context.closePath() }
      else context.arc(x, y, radius, 0, Math.PI * 2)
      context.fill()
      if (active) { context.beginPath(); context.arc(x, y, 9, 0, Math.PI * 2); context.strokeStyle = hue(point.subject); context.stroke() }
    }
    context.globalAlpha = 1
  }, [links, positions, regions, related, hot, matches, moment, screenPoints, size, subject, theme.shelves, transform])

  const hit = (event: MouseEvent): PagePoint | undefined => {
    const box = ref.current?.getBoundingClientRect()
    if (!box) return
    const x = event.clientX - box.left, y = event.clientY - box.top
    return [...screenPoints].sort((a, b) => Math.hypot(a.screen.x - x, a.screen.y - y) - Math.hypot(b.screen.x - x, b.screen.y - y))
      .find(point => Math.hypot(point.screen.x - x, point.screen.y - y) < 12)
  }
  return <div className="brain-continuous" ref={ref} {...view.handlers} tabIndex={0}
    aria-label="Continuous Brain map. Drag to pan; scroll to zoom. Arrow keys pan; plus and minus zoom."
    onKeyDown={event => {
      if (event.target !== event.currentTarget) return
      const movement = { ArrowLeft: [70, 0], ArrowRight: [-70, 0], ArrowUp: [0, 70], ArrowDown: [0, -70] }[event.key]
      if (movement) { event.preventDefault(); restore(panBy(transform, movement[0], movement[1])) }
      if (['+', '=', '-'].includes(event.key)) { event.preventDefault(); restore(zoomAbout(transform, { x: size.width / 2, y: size.height / 2 }, event.key === '-' ? .8 : 1.25, LIMITS)) }
      if (event.key === 'Home') { event.preventDefault(); fit() }
    }} onMouseLeave={() => setHovered(null)}>
    <canvas ref={canvas} aria-hidden="true" onMouseMove={event => setHovered(hit(event)?.page.path ?? null)}
      onClick={event => {
        if (view.panned()) return
        const point = hit(event)
        if (point) { onSelect(point.page.path); return }
        const box = ref.current?.getBoundingClientRect()
        if (!box) return
        const world = view.toWorld({ x: event.clientX - box.left, y: event.clientY - box.top })
        const region = regions.find(entry => Math.hypot(entry.x - world.x, entry.y - world.y) <= entry.radius)
        if (region) { onSubject(region.id); focusRegion(region) }
      }} />
    <div className="brain-region-labels">
      {regionLabels.map(({ region, x, y, width }) => {
        const count = region.pages.filter(page => visiblePaths.has(page.path)).length
        return <button type="button" key={region.id} className="brain-region-label" title={`${region.collection} / ${region.name}`}
          style={{ left: x, top: y, width, '--subject-color': subjectHue(region.id, theme.shelves) } as CSSProperties}
          aria-label={`Explore ${region.name}, ${count} pages`} onClick={event => { if (event.detail === 0 || !view.panned()) { onSubject(region.id); focusRegion(region) } }}>
          <span>{region.name}</span><small>{region.collection} · {count}</small>
        </button>
      })}
    </div>
    <div className="brain-continuous-pages">
      {labels.map(({ point, x, y, width }) => <button type="button" key={point.page.path} className={point.page.path === hot ? 'is-hot' : ''}
        style={{ left: x, top: y, maxWidth: width }} aria-label={point.page.title} title={point.page.title}
        onFocus={() => setHovered(point.page.path)} onBlur={() => setHovered(null)}
        onClick={event => { if (event.detail === 0 || !view.panned()) onSelect(point.page.path) }}>{point.page.title}</button>)}
    </div>
    {points.length === 0 && <p className="brain-map-empty">No pages match these filters.</p>}
    <span className="brain-map-scale">{Math.round(transform.scale * 100)}%</span>
  </div>
}
