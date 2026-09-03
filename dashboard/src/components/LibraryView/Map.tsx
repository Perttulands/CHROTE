/**
 * The map of the library.
 *
 * Every page is a dot in its shelf's cluster, sized by how much it says and
 * faded by how long ago it moved; a written link is a hairline, a shared tag a
 * dotted one. Pointing at a page names it and lights what it touches; clicking
 * opens it. The same drawing, 150px tall, is the Near-this-page strip over the
 * reading room: the open page at the left and its neighbours beside it.
 *
 * The arithmetic is in mapLayout.ts. This file only measures its box, keeps
 * the pointer's state, and draws.
 */

import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { KeyboardEvent as ReactKeyboardEvent } from 'react'
import type { LibraryGraph } from '../../library/libraryApi'
import {
  LANDMARK_LABELS,
  MAP_LABEL_CHARS,
  layoutMap,
  layoutStrip,
  neighboursOf,
  placeLabels,
} from './mapLayout'

export interface MapProps {
  graph: LibraryGraph
  /** The whole corpus, or the open page and its neighbours in a strip. */
  mode: 'map' | 'strip'
  /** The page on the table, lit with its neighbours; null on the landing. */
  openPath: string | null
  /** The pages a search names, lit and labelled; null with no search. */
  matches: ReadonlySet<string> | null
  onOpen: (path: string) => void
}

/** The strip's height, which the layout and the stylesheet both know. */
export const STRIP_HEIGHT = 150

/** What the layout is given before the box has been measured, and in jsdom. */
const FALLBACK_WIDTH = 960
const FALLBACK_HEIGHT = 600

/** A strip label may run to its column's width; a map label to one measure. */
const STRIP_LABEL_CHARS = 22

function useMeasuredSize(mode: 'map' | 'strip') {
  const ref = useRef<HTMLDivElement>(null)
  const [size, setSize] = useState({ width: FALLBACK_WIDTH, height: FALLBACK_HEIGHT })

  useLayoutEffect(() => {
    const element = ref.current
    if (!element) return
    const read = () => {
      const box = element.getBoundingClientRect()
      if (box.width > 0 && box.height > 0) {
        setSize(current => (
          current.width === Math.round(box.width) && current.height === Math.round(box.height)
            ? current
            : { width: Math.round(box.width), height: Math.round(box.height) }
        ))
      }
    }
    read()
    if (typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(read)
    observer.observe(element)
    return () => observer.disconnect()
  }, [mode])

  return { ref, width: size.width, height: mode === 'strip' ? STRIP_HEIGHT : size.height }
}

export default function LibraryMap({ graph, mode, openPath, matches, onOpen }: MapProps) {
  const { ref, width, height } = useMeasuredSize(mode)
  const [hovered, setHovered] = useState<string | null>(null)

  // The pointer's page is forgotten when the drawing changes under it.
  useEffect(() => { setHovered(null) }, [mode, graph])

  const layout = useMemo(
    () => (mode === 'strip' && openPath ? layoutStrip(graph, openPath, width, height) : layoutMap(graph, width, height)),
    [graph, mode, openPath, width, height],
  )

  const { focus, hot, primary } = useMemo(() => {
    const focus = new Set<string>()
    if (openPath) focus.add(openPath)
    if (hovered) focus.add(hovered)
    const hot = new Set<string>(focus)
    focus.forEach(path => neighboursOf(graph, path).forEach(other => hot.add(other)))
    const primary = new Set<string>(focus)
    matches?.forEach(path => { hot.add(path); primary.add(path) })
    if (mode === 'strip') layout.nodes.forEach(node => hot.add(node.path))
    return { focus, hot, primary }
  }, [graph, hovered, layout, matches, mode, openPath])

  const labels = useMemo(
    () => placeLabels(layout.nodes, hot, primary, {
      landmarks: mode === 'map' ? LANDMARK_LABELS : 0,
      maxChars: mode === 'map' ? MAP_LABEL_CHARS : STRIP_LABEL_CHARS,
    }, layout.clusters),
    [hot, layout, mode, primary],
  )

  const positions = useMemo(() => new Map(layout.nodes.map(node => [node.path, node])), [layout])

  const open = (path: string) => (event: ReactKeyboardEvent<SVGGElement>) => {
    if (event.key !== 'Enter' && event.key !== ' ') return
    event.preventDefault()
    onOpen(path)
  }

  return (
    <div ref={ref} className={mode === 'map' ? 'library-map' : 'library-map library-map-strip'} data-ui="library.map">
      <svg width={width} height={height} viewBox={`0 0 ${width} ${height}`} aria-label={mode === 'map' ? 'The map' : 'Near this page'}>
        {layout.edges.map((edge, index) => {
          const from = positions.get(edge.from)
          const to = positions.get(edge.to)
          if (!from || !to) return null
          const lit = focus.has(edge.from) || focus.has(edge.to)
          return (
            <line
              key={`${edge.from}|${edge.to}|${index}`}
              className={`library-map-edge${edge.tag ? ' tag' : ''}${lit ? ' hot' : ''}`}
              x1={from.x}
              y1={from.y}
              x2={to.x}
              y2={to.y}
            />
          )
        })}
        {layout.clusters.map(cluster => (
          <text key={cluster.shelf} className="library-map-cluster" x={cluster.x} y={cluster.y} textAnchor="middle">
            {cluster.shelf} · {cluster.count}
          </text>
        ))}
        {layout.nodes.map(node => {
          const lit = hot.has(node.path)
          return (
            <g
              key={node.path}
              className={`library-map-node${node.candidate ? ' candidate' : ''}${lit ? ' hot' : ''}`}
              role="button"
              tabIndex={0}
              aria-label={node.title}
              onMouseEnter={() => setHovered(node.path)}
              onMouseLeave={() => setHovered(current => (current === node.path ? null : current))}
              onFocus={() => setHovered(node.path)}
              onBlur={() => setHovered(current => (current === node.path ? null : current))}
              onClick={() => onOpen(node.path)}
              onKeyDown={open(node.path)}
            >
              <circle className="library-map-hit" cx={node.x} cy={node.y} r={Math.max(node.r + 6, 10)} />
              <circle className="library-map-dot" cx={node.x} cy={node.y} r={node.r} opacity={lit ? 1 : node.opacity} />
            </g>
          )
        })}
        {labels.map(label => (
          <text key={label.path} className={`library-map-label${label.primary ? ' primary' : ''}`} x={label.x} y={label.y}>
            {label.text}
          </text>
        ))}
        {layout.more > 0 && (
          <text className="library-map-label" x={width - 40} y={height - 24} textAnchor="end">
            … {layout.more} more
          </text>
        )}
      </svg>
    </div>
  )
}
