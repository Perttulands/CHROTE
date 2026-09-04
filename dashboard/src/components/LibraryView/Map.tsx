/**
 * The map of the library.
 *
 * Every page is a dot in its shelf's cluster, sized by how much it says and
 * faded by how long ago it moved; a written link is a hairline, a shared tag a
 * dotted one. Pointing at a page names it and lights what it touches; clicking
 * it dives in: the map takes that page to the middle and closer in, and stays
 * where it is while the page is read in the column beside it.
 *
 * The arithmetic is in mapLayout.ts. This file only measures its box, keeps
 * the pointer's state, and draws.
 */

import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { KeyboardEvent as ReactKeyboardEvent } from 'react'
import type { LibraryGraph } from '../../library/libraryApi'
import { useMapTransform } from '../../hooks/useMapTransform'
import {
  LANDMARK_LABELS,
  MAP_LABEL_CHARS,
  layoutMap,
  neighboursOf,
  placeLabels,
  withinWindow,
  type RecencyWindow,
} from './mapLayout'

export interface MapProps {
  graph: LibraryGraph
  /**
   * The page being dived into: taken to the middle, lit with its neighbours.
   * Null on the landing, and again once the dive is closed.
   */
  openPath: string | null
  /** The pages a search names, lit and labelled; null with no search. */
  matches: ReadonlySet<string> | null
  /** How recently a page must have moved to be drawn at full strength. */
  window?: RecencyWindow
  onOpen: (path: string) => void
}

/** What the layout is given before the box has been measured, and in jsdom. */
const FALLBACK_WIDTH = 960
const FALLBACK_HEIGHT = 600

/** How far in the map is taken when a page is dived into. */
export const DIVE_SCALE = 2

/** A page outside the recency window is drawn at this much of itself. */
const STALE_OPACITY = 0.14

/** The hover label's box: one 12px monospaced character, and its padding. */
const HOVER_CHAR = 7.2
const HOVER_PAD = 7
const HOVER_HEIGHT = 20
const HOVER_GAP = 10

function useMeasuredSize() {
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
  }, [])

  return { ref, width: size.width, height: size.height }
}

export default function LibraryMap({ graph, openPath, matches, window: recency = 'all', onOpen }: MapProps) {
  const { ref, width, height } = useMeasuredSize()
  const [hovered, setHovered] = useState<string | null>(null)
  const view = useMapTransform({ width, height })

  // The pointer's page is forgotten when the drawing changes under it.
  useEffect(() => { setHovered(null) }, [graph])

  // One reading of the clock per drawing, so every node is judged against
  // the same moment and nothing redraws because time passed.
  const now = useMemo(() => Date.now(), [graph])

  const layout = useMemo(() => layoutMap(graph, width, height), [graph, width, height])

  const { focus, hot, primary } = useMemo(() => {
    const focus = new Set<string>()
    if (openPath) focus.add(openPath)
    if (hovered) focus.add(hovered)
    const hot = new Set<string>(focus)
    focus.forEach(path => neighboursOf(graph, path).forEach(other => hot.add(other)))
    const primary = new Set<string>(focus)
    matches?.forEach(path => { hot.add(path); primary.add(path) })
    return { focus, hot, primary }
  }, [graph, hovered, matches, openPath])

  const labels = useMemo(
    () => placeLabels(layout.nodes, hot, primary, { landmarks: LANDMARK_LABELS, maxChars: MAP_LABEL_CHARS }, layout.clusters),
    [hot, layout, primary],
  )

  const positions = useMemo(() => new Map(layout.nodes.map(node => [node.path, node])), [layout])

  // A dive takes its page to the middle and closer in, wherever it was asked
  // for: a dot, a neighbour's link in the column, a row in the rail. It is
  // done once per page, so a map the operator has moved since stays moved.
  const { centreOn } = view
  const dived = useRef<string | null>(null)
  useEffect(() => {
    if (!openPath) {
      dived.current = null
      return
    }
    if (dived.current === openPath) return
    const found = positions.get(openPath)
    if (!found) return
    dived.current = openPath
    centreOn(found, DIVE_SCALE)
  }, [centreOn, openPath, positions])

  const open = (path: string) => (event: ReactKeyboardEvent<SVGGElement>) => {
    if (event.key !== 'Enter' && event.key !== ' ') return
    event.preventDefault()
    onOpen(path)
  }

  // A drag that moved the map is not a click on the page it started over.
  const choose = (path: string) => () => { if (!view.panned()) onOpen(path) }

  // The pointer's page named in full, in the box's own coordinates so it
  // stays legible however far in the map is taken, and turned back on itself
  // at the right edge rather than drawn off it.
  const hoverLabel = (() => {
    const node = hovered ? positions.get(hovered) : undefined
    if (!node) return null
    const point = view.toScreen(node)
    const boxWidth = node.title.length * HOVER_CHAR + HOVER_PAD * 2
    const right = point.x + node.r * view.transform.scale + HOVER_GAP
    const left = right + boxWidth > width ? point.x - node.r * view.transform.scale - HOVER_GAP - boxWidth : right
    return { title: node.title, x: left, y: point.y - HOVER_HEIGHT / 2, width: boxWidth }
  })()

  return (
    <div ref={ref} className="library-map" data-ui="library.map">
      <svg
        ref={view.ref}
        className={view.moved ? 'moved' : undefined}
        width={width}
        height={height}
        viewBox={`0 0 ${width} ${height}`}
        aria-label="The map"
        {...view.handlers}
      >
        <g transform={view.groupTransform}>
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
          {layout.nodes.map(node => {
            const lit = hot.has(node.path)
            const stale = !withinWindow(node.updated, recency, now)
            return (
              <g
                key={node.path}
                className={`library-map-node${node.candidate ? ' candidate' : ''}${lit ? ' hot' : ''}${stale ? ' stale' : ''}`}
                role="button"
                tabIndex={0}
                aria-label={node.title}
                onMouseEnter={() => setHovered(node.path)}
                onMouseLeave={() => setHovered(current => (current === node.path ? null : current))}
                onFocus={() => setHovered(node.path)}
                onBlur={() => setHovered(current => (current === node.path ? null : current))}
                onClick={choose(node.path)}
                onKeyDown={open(node.path)}
              >
                <circle className="library-map-hit" cx={node.x} cy={node.y} r={Math.max(node.r + 6, 10)} />
                <circle
                  className="library-map-dot"
                  cx={node.x}
                  cy={node.y}
                  r={node.r}
                  opacity={stale ? STALE_OPACITY : lit ? 1 : node.opacity}
                />
              </g>
            )
          })}
        </g>
        {/* Type is drawn in the box's coordinates: the drawing zooms, the
            names keep the one size they are readable at. */}
        {layout.clusters.map(cluster => {
          const point = view.toScreen(cluster)
          return (
            <text key={cluster.shelf} className="library-map-cluster" x={point.x} y={point.y} textAnchor="middle">
              {cluster.shelf} · {cluster.count}
            </text>
          )
        })}
        {labels.map(label => {
          const point = view.toScreen(label)
          return (
            <text key={label.path} className={`library-map-label${label.primary ? ' primary' : ''}`} x={point.x} y={point.y}>
              {label.text}
            </text>
          )
        })}
        {hoverLabel && (
          <g className="library-map-hover" data-ui="library.map.hover" pointerEvents="none">
            <rect x={hoverLabel.x} y={hoverLabel.y} width={hoverLabel.width} height={HOVER_HEIGHT} rx={2} />
            <text x={hoverLabel.x + HOVER_PAD} y={hoverLabel.y + 14}>{hoverLabel.title}</text>
          </g>
        )}
        {layout.more > 0 && (
          <text className="library-map-label" x={width - 40} y={height - 24} textAnchor="end">
            … {layout.more} more
          </text>
        )}
      </svg>
      {view.moved && (
        <button type="button" className="library-map-reset" onClick={view.reset} data-ui="library.map.reset">
          Reset the view
        </button>
      )}
    </div>
  )
}
