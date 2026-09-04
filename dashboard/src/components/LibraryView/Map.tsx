/**
 * The map of the library.
 *
 * Every page is a dot in its shelf's cluster, sized by how much it says and
 * faded by how long ago it moved; a written link is a hairline, a shared tag a
 * dotted one. Pointing at a page names it and lights what it touches; clicking
 * it dives in: the map takes that page to the middle and closer in, and stays
 * where it is while the page is read in the column beside it.
 *
 * The drawing is a canvas, because a corpus of ten thousand pages is not a
 * corpus the browser will carry as ten thousand elements. That costs the map
 * the two things the DOM gave it for nothing, so both are answered here: the
 * page under the pointer is found through a spatial index over the layout, and
 * every page keeps a focusable element of its own in a layer over the canvas,
 * so the keyboard and a screen reader still reach the map the drawing shows.
 *
 * The arithmetic is in mapLayout.ts and the drawing in mapRenderer.ts. This
 * file measures its box, keeps the pointer's state, and asks for a frame when
 * one of those changed. Nothing here runs on a clock.
 */

import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { MutableRefObject } from 'react'
import type {
  CSSProperties,
  FocusEvent as ReactFocusEvent,
  KeyboardEvent as ReactKeyboardEvent,
  MouseEvent as ReactMouseEvent,
  PointerEvent as ReactPointerEvent,
} from 'react'
import { libraryWhen, type LibraryGraph } from '../../library/libraryApi'
import { useMapTransform } from '../../hooks/useMapTransform'
import {
  BAND_FADE,
  MID_LABELS,
  NEAR_SCALE,
  bandAt,
  cardSize,
  dotRadius,
  placeCards,
  type MapBand,
  type MapCard,
} from './mapBands'
import { buildIndex, hitTest, reachOf } from './mapIndex'
import {
  LANDMARK_LABELS,
  MAP_LABEL_CHARS,
  layoutMap,
  neighboursOf,
  placeLabels,
  withinWindow,
  type MapLabel,
  type MapLayout,
  type MapNode,
  type RecencyWindow,
} from './mapLayout'
import type { MapLayoutRequest } from './mapLayout.worker'
import { createCanvasRenderer, readPalette, type MapRenderer, type MapScene, type MapTextLayer } from './mapRenderer'

export interface MapProps {
  graph: LibraryGraph
  /**
   * The page being dived into: taken to the middle, lit with its neighbours.
   * Null on the landing, and again once the dive is closed.
   */
  openPath: string | null
  /** The pages a search names, lit and labelled; null with no search. */
  matches: ReadonlySet<string> | null
  /**
   * The page the pointer is on in the rail: brought to the middle and lit
   * with its neighbours, so a list in the rail is read against the map.
   */
  hoverPath?: string | null
  /** How recently a page must have moved to be drawn at full strength. */
  window?: RecencyWindow
  onOpen: (path: string) => void
}

/** What the layout is given before the box has been measured, and in jsdom. */
const FALLBACK_WIDTH = 960
const FALLBACK_HEIGHT = 600

/**
 * How far in the map is taken when a page is dived into: far enough that the
 * page it opened is read as a card rather than as a dot, because a dive is a
 * reading.
 */
export const DIVE_SCALE = NEAR_SCALE

/**
 * How many pages keep a focusable element over the canvas.
 *
 * Every page having one is what makes the map reachable without a pointer, and
 * a corpus of any size a reader has ever had fits well inside this. Past it the
 * layer would cost more than it gives on every pan, so it holds only the pages
 * the drawing has named or lit, and the canvas answers the pointer for the
 * rest. The number is the judgement, not a limit of the drawing.
 */
const MAX_FOCUSABLE = 1200

/**
 * Past how many pages the layout is worked out in a worker rather than here.
 *
 * Measured on this host: a thousand pages lay out in about 30 ms and ten
 * thousand in about 850 ms. Anything the operator waits a frame for is a defect,
 * so the map keeps the cheap corpus on the thread it draws on, where the picture
 * arrives with the graph, and hands the expensive one away.
 */
const WORKER_ABOVE = 400

/** What the map draws while a worker is still laying the corpus out. */
const EMPTY_LAYOUT: MapLayout = { nodes: [], edges: [], clusters: [], more: 0 }

/** How much of a long name a card carries before it is cut. */
const CARD_CHARS = 34

/** The hover readout's box: one 12px monospaced character, and its padding. */
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

/**
 * Where the pages go, worked out on the thread that can afford it.
 *
 * A corpus of dozens lays out in about a millisecond, so it is laid out here
 * and the map is drawn in the same frame the graph arrived in. A corpus of
 * thousands costs long enough that doing the same would stop the interface
 * while it ran, so past WORKER_ABOVE pages the work goes to a worker and the
 * map says it is drawing until the answer comes back. Either way the arithmetic
 * is the one function, and the picture is the same picture.
 */
function useMapLayout(graph: LibraryGraph, width: number, height: number): { layout: MapLayout; drawing: boolean } {
  const heavy = graph.pages.length > WORKER_ABOVE && typeof Worker !== 'undefined'
  const [answer, setAnswer] = useState<{ token: number; layout: MapLayout } | null>(null)
  const asked = useRef(0)

  // One reading of the clock per graph, so every page is judged against the
  // same moment and nothing redraws because time passed.
  const now = useMemo(() => Date.now(), [graph])
  const here = useMemo(() => (heavy ? null : layoutMap(graph, width, height, now)), [graph, heavy, height, now, width])

  useEffect(() => {
    if (!heavy) return
    const worker = new Worker(new URL('./mapLayout.worker.ts', import.meta.url), { type: 'module' })
    const token = ++asked.current
    worker.onmessage = (event: MessageEvent<{ token: number; layout: MapLayout }>) => {
      if (event.data.token === token) setAnswer(event.data)
    }
    worker.postMessage({ token, graph, width, height, now } satisfies MapLayoutRequest)
    return () => worker.terminate()
  }, [graph, heavy, height, now, width])

  if (here) return { layout: here, drawing: false }
  if (answer) return { layout: answer.layout, drawing: false }
  return { layout: EMPTY_LAYOUT, drawing: true }
}

export default function LibraryMap({ graph, openPath, matches, hoverPath = null, window: recency = 'all', onOpen }: MapProps) {
  const { ref, width, height } = useMeasuredSize()
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const rendererRef = useRef<MapRenderer | null>(null)
  const [hovered, setHovered] = useState<string | null>(null)
  const [palette, setPalette] = useState(() => readPalette(null))
  const view = useMapTransform<HTMLDivElement>({ width, height })

  // The pointer's page is forgotten when the drawing changes under it.
  useEffect(() => { setHovered(null) }, [graph])

  const { layout, drawing } = useMapLayout(graph, width, height)
  const index = useMemo(() => buildIndex(layout.nodes), [layout])
  const positions = useMemo(() => new Map(layout.nodes.map(node => [node.path, node])), [layout])

  const { focus, hot, primary } = useMemo(() => {
    const focus = new Set<string>()
    if (openPath) focus.add(openPath)
    if (hovered) focus.add(hovered)
    if (hoverPath) focus.add(hoverPath)
    const hot = new Set<string>(focus)
    focus.forEach(path => neighboursOf(graph, path).forEach(other => hot.add(other)))
    const primary = new Set<string>(focus)
    matches?.forEach(path => { hot.add(path); primary.add(path) })
    return { focus, hot, primary }
  }, [graph, hovered, hoverPath, matches, openPath])

  // One reading of the clock per drawing, so every page is judged against the
  // same moment and nothing redraws because time passed.
  const now = useMemo(() => Date.now(), [graph])
  const stale = useMemo(
    () => new Set(layout.nodes.filter(node => !withinWindow(node.updated, recency, now)).map(node => node.path)),
    [layout, now, recency],
  )

  // The page the readout names — the one under the pointer, or the one being
  // read — is named there and nowhere else: printing it twice is what the
  // operator saw, and its place in the packing is kept so nothing else moves.
  const named = hovered ?? openPath
  const suppress = useMemo(() => new Set(named ? [named] : []), [named])

  // Which band the map is in, and the fade from the one it left. The fade is
  // the only thing here that runs on frames, it runs for a fifth of a second,
  // and it runs because the operator zoomed.
  const scale = view.transform.scale
  const band = bandAt(scale)
  const [fade, setFade] = useState<{ from: MapBand; mix: number } | null>(null)
  const wasBand = useRef(band)
  useEffect(() => {
    if (wasBand.current === band) return
    const from = wasBand.current
    wasBand.current = band
    const started = performance.now()
    let handle = requestAnimationFrame(function step() {
      const mix = Math.min(1, (performance.now() - started) / BAND_FADE)
      setFade(mix >= 1 ? null : { from, mix })
      if (mix < 1) handle = requestAnimationFrame(step)
    })
    return () => cancelAnimationFrame(handle)
  }, [band])

  // What a card says about a page: what it is, where it sits, when it last
  // moved, how long it is, and what it shares a tag with.
  const tagsOf = useMemo(() => {
    const found = new Map<string, string[]>()
    graph.tags.forEach(([from, to, tag]) => {
      [from, to].forEach(path => {
        const carried = found.get(path)
        if (carried) { if (!carried.includes(tag)) carried.push(tag) }
        else found.set(path, [tag])
      })
    })
    return found
  }, [graph])

  const describe = useCallback((node: MapNode) => {
    const title = node.title.length > CARD_CHARS ? `${node.title.slice(0, CARD_CHARS - 1)}…` : node.title
    const tags = tagsOf.get(node.path)
    return [
      title,
      `${node.shelf} · ${libraryWhen(node.updated, now)}`,
      `${node.words} words${tags?.length ? ` · ${tags.join(' · ')}` : ''}`,
    ]
  }, [now, tagsOf])

  // The page the operator is on, named in full in the box's own coordinates so
  // it stays legible however far in the map is taken, and turned back on itself
  // at the right edge rather than drawn off it. Near in, it is that page's card:
  // the one the reader asked for, in the one place he is looking.
  const readout = (() => {
    const node = named ? positions.get(named) : undefined
    if (!node) return null
    const card = band === 'near'
    const lines = card ? describe(node) : [node.title]
    const size = card
      ? cardSize(lines)
      : { width: node.title.length * HOVER_CHAR + HOVER_PAD * 2, height: HOVER_HEIGHT }
    const point = view.toScreen(node)
    const radius = dotRadius(node, scale)
    const right = point.x + radius + HOVER_GAP
    const wanted = right + size.width > width ? point.x - radius - HOVER_GAP - size.width : right
    // A narrow map — the one left beside an open dive — may have room for the
    // readout on neither side, and a name drawn off the box is not a name.
    const left = Math.max(0, Math.min(width - size.width, wanted))
    return { card, lines, x: left, y: point.y - size.height / 2, ...size }
  })()

  // Far away, the shelves and the landmarks. Closer, every page's name. Closer
  // still, a card each: the same drawing, saying more the nearer it is read.
  const writing = useCallback((which: MapBand): { labels: MapLabel[]; cards: MapCard[] } => {
    if (which === 'near') {
      return {
        labels: [],
        cards: placeCards(layout.nodes, {
          transform: view.transform,
          width,
          height,
          describe,
          suppress,
          reserved: readout ? [readout] : [],
        }),
      }
    }
    return {
      labels: placeLabels(
        layout.nodes,
        hot,
        primary,
        { landmarks: which === 'mid' ? MID_LABELS : LANDMARK_LABELS, maxChars: MAP_LABEL_CHARS, suppress, scale },
        layout.clusters,
      ),
      cards: [],
    }
  }, [describe, height, hot, layout, primary, readout, scale, suppress, view.transform, width])

  const text: MapTextLayer[] = useMemo(() => {
    const here = { ...writing(band), alpha: fade ? fade.mix : 1 }
    return fade ? [{ ...writing(fade.from), alpha: 1 - fade.mix }, here] : [here]
  }, [band, fade, writing])

  const labels = text[text.length - 1].labels

  // The drawing surface is made once and kept; the theme's colours are read
  // off the map itself, so the canvas is painted in the same palette the rest
  // of the interface inherits.
  useLayoutEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    rendererRef.current = createCanvasRenderer(canvas)
    setPalette(readPalette(ref.current))
    return () => {
      rendererRef.current?.destroy()
      rendererRef.current = null
    }
  }, [ref])

  // A frame, whenever the scene it would draw has changed and never otherwise.
  const scene: MapScene = useMemo(() => ({
    nodes: layout.nodes,
    edges: layout.edges,
    text,
    transform: view.transform,
    width,
    height,
    hot,
    focus,
    stale,
    palette,
  }), [focus, height, hot, layout, palette, stale, text, view.transform, width])

  useEffect(() => { rendererRef.current?.draw(scene) }, [scene])

  // A dive takes its page to the middle and closer in, wherever it was asked
  // for: a dot, a neighbour's link in the column, a row in the rail. It is
  // done once per page, so a map the operator has moved since stays moved.
  //
  // The page is taken to the middle again if the drawing itself changed under
  // the dive: opening the column beside the map narrows the map, which lays the
  // corpus out afresh, and a page centred against the old width would be
  // anywhere against the new one.
  const { centreOn } = view
  const dived = useRef<{ path: string; where: typeof positions } | null>(null)
  useEffect(() => {
    if (!openPath) {
      dived.current = null
      return
    }
    if (dived.current?.path === openPath && dived.current.where === positions) return
    const found = positions.get(openPath)
    if (!found) return
    dived.current = { path: openPath, where: positions }
    centreOn(found, DIVE_SCALE)
  }, [centreOn, openPath, positions])

  // A page pointed at in the rail is brought to the middle at the scale the
  // map is already at: the operator is looking for where it sits, not diving
  // into it. Once per page, so a map moved since stays moved.
  const centred = useRef<string | null>(null)
  useEffect(() => {
    if (!hoverPath) {
      centred.current = null
      return
    }
    if (centred.current === hoverPath) return
    const found = positions.get(hoverPath)
    if (!found) return
    centred.current = hoverPath
    centreOn(found)
  }, [centreOn, hoverPath, positions])

  // The focusable layer's handlers are made once and read the page they were
  // fired on off the element, so the layer itself can be built once per
  // drawing rather than again on every hover and every pan.
  const { panned } = view
  const enter = useCallback((event: ReactFocusEvent<HTMLButtonElement> | ReactMouseEvent<HTMLButtonElement>) => {
    setHovered(event.currentTarget.dataset.path ?? null)
  }, [])
  const leave = useCallback((event: ReactFocusEvent<HTMLButtonElement>) => {
    const path = event.currentTarget.dataset.path
    setHovered(current => (current === path ? null : current))
  }, [])
  // A drag that moved the map is not a click on the page it started over.
  const choose = useCallback((event: ReactMouseEvent<HTMLButtonElement>) => {
    const path = event.currentTarget.dataset.path
    if (path && !panned()) onOpen(path)
  }, [onOpen, panned])
  const open = useCallback((event: ReactKeyboardEvent<HTMLButtonElement>) => {
    if (event.key !== 'Enter' && event.key !== ' ') return
    event.preventDefault()
    const path = event.currentTarget.dataset.path
    if (path) onOpen(path)
  }, [onOpen])

  // What the pointer is over, asked of the index rather than of the browser:
  // the drawing is one element, so the map works out for itself which page was
  // drawn where the pointer is.
  const { toWorld } = view
  const at = useCallback((clientX: number, clientY: number) => {
    const box = ref.current?.getBoundingClientRect()
    if (!box) return null
    const point = toWorld({ x: clientX - box.left, y: clientY - box.top })
    return hitTest(index, point.x, point.y, scale)
  }, [index, ref, scale, toWorld])

  const onPointerMove = (event: ReactPointerEvent<HTMLDivElement>) => {
    view.handlers.onPointerMove(event)
    const found = at(event.clientX, event.clientY)
    setHovered(current => (found?.path ?? null) === current ? current : found?.path ?? null)
  }

  // A click on the drawing opens the page it landed on. A click that landed on
  // a page's own focusable element is that element's, and is not counted here
  // as well.
  const onClick = (event: ReactMouseEvent<HTMLDivElement>) => {
    if (event.target !== canvasRef.current || view.panned()) return
    const found = at(event.clientX, event.clientY)
    if (found) onOpen(found.path)
  }


  // The pages that keep a focusable element: all of them for a corpus of any
  // size a library has, and the named and lit ones beyond that.
  const reachable = layout.nodes.length <= MAX_FOCUSABLE
  const focusable = useMemo(() => {
    if (reachable) return layout.nodes
    const named = new Set(labels.map(label => label.path))
    return layout.nodes.filter(node => named.has(node.path) || hot.has(node.path))
  }, [hot, labels, layout, reachable])

  // The layer is built when the drawing changes and not when the pointer moves:
  // what the light does to it is a class set on the pages that changed, not a
  // thousand elements made again. That is the difference between a hover the
  // operator feels and one he does not.
  const handles = useMemo(() => focusable.map(node => {
    const reach = reachOf(node)
    return (
      <button
        key={node.path}
        type="button"
        data-path={node.path}
        className={`library-map-node${node.candidate ? ' candidate' : ''}${stale.has(node.path) ? ' stale' : ''}`}
        aria-label={node.title}
        style={{ left: node.x - reach, top: node.y - reach, width: reach * 2, height: reach * 2 }}
        onMouseEnter={enter}
        onFocus={enter}
        onBlur={leave}
        onClick={choose}
        onKeyDown={open}
      />
    )
  }), [choose, enter, focusable, leave, open, stale])

  const layerRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const layer = layerRef.current
    if (!layer) return
    Array.from(layer.children).forEach(element => {
      const path = (element as HTMLElement).dataset.path
      element.classList.toggle('hot', path !== undefined && hot.has(path))
    })
  }, [handles, hot])

  // The box is one element to three readers: the one that measures it, the one
  // that moves the drawing through it, and the drawing's own coordinates.
  const box = useCallback((element: HTMLDivElement | null) => {
    (ref as MutableRefObject<HTMLDivElement | null>).current = element
    ;(view.ref as MutableRefObject<HTMLDivElement | null>).current = element
  }, [ref, view.ref])

  return (
    <div
      ref={box}
      className="library-map"
      data-ui="library.map"
      {...view.handlers}
      onPointerMove={onPointerMove}
      onClick={onClick}
      onMouseLeave={() => setHovered(null)}
    >
      <canvas ref={canvasRef} className="library-map-canvas" role="img" aria-label="The map" />
      {/* One element per page over the drawing, so the map is reachable by
          keyboard and readable by a screen reader. It carries no ink: what the
          operator sees is the canvas beneath it. */}
      <div
        ref={layerRef}
        className={`library-map-nodes${reachable ? '' : ' sparse'}`}
        style={{
          transform: `translate(${view.transform.x}px, ${view.transform.y}px) scale(${scale})`,
          // The pages a pointer aims at keep the size they are aimed at: past
          // the landing scale the drawing grows and the targets do not.
          '--map-scale': scale,
        } as CSSProperties}
      >
        {handles}
      </div>
      {/* Type in the box's coordinates: the drawing zooms, the names keep the
          one size they are readable at. */}
      {layout.clusters.map(cluster => {
        const point = view.toScreen(cluster)
        return (
          <span
            key={cluster.shelf}
            className="library-map-cluster"
            // Near in, every card names its own shelf, so the shelf's own name
            // gives way rather than sitting over the cards.
            style={{ left: point.x, top: point.y, opacity: band === 'near' ? 0 : 1 }}
          >
            {cluster.shelf} · {cluster.count}
          </span>
        )
      })}
      {readout && (
        <span
          className={`library-map-hover${readout.card ? ' card' : ''}`}
          data-ui="library.map.hover"
          style={{ left: readout.x, top: readout.y, width: readout.width, height: readout.height }}
        >
          {readout.lines.map(line => <span key={line} className="library-map-hover-line">{line}</span>)}
        </span>
      )}
      {drawing && <p className="library-empty">Drawing the map…</p>}
      {layout.more > 0 && (
        <span className="library-map-more">… {layout.more} more</span>
      )}
      {view.moved && (
        <button type="button" className="library-map-reset" onClick={view.reset} data-ui="library.map.reset">
          Reset the view
        </button>
      )}
    </div>
  )
}
