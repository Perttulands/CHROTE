/**
 * The map's drawing surface.
 *
 * One interface — give it a scene, it draws it — with a Canvas 2D
 * implementation behind it. The map was an SVG element per page and per link,
 * which the browser lays out, styles and hit-tests one by one; a corpus of ten
 * thousand pages is a corpus the browser cannot carry that way. A canvas costs
 * one element and one pass over the arrays, and the pass batches: every
 * hairline of a kind is one path and one stroke, every dot of a shade is one
 * path and one fill, so the cost is the arithmetic rather than the DOM.
 *
 * The renderer knows nothing about React, the graph, or the pointer. It is
 * handed a scene and draws exactly that, which is what lets the layout, the
 * view's state and the drawing be changed apart from one another, and what
 * would let a WebGL surface take this place without either of them noticing.
 *
 * Nothing here runs on a clock. A frame is drawn when the view asks for one.
 */

import type { MapTransform } from '../../hooks/useMapTransform'
import type { MapEdge, MapLabel, MapNode } from './mapLayout'

/** The colours the drawing uses, read from the theme rather than invented. */
export interface MapPalette {
  background: string
  /** An ordinary page. */
  dot: string
  /** A candidate, and a name drawn beside a dot. */
  dim: string
  /** What the operator is on, pointing at, or searched for. */
  accent: string
  /** A hairline. */
  divider: string
  /** The family names are drawn in, as the theme sets it. */
  family: string
}

export interface MapScene {
  nodes: readonly MapNode[]
  edges: readonly MapEdge[]
  /** Names to draw beside their dots, already placed in layout coordinates. */
  labels: readonly MapLabel[]
  transform: MapTransform
  width: number
  height: number
  /** Pages drawn at full strength: the focus and everything it touches. */
  hot: ReadonlySet<string>
  /** The pages the light starts at: the ends of the hairlines that light up. */
  focus: ReadonlySet<string>
  /** Pages outside the recency window, drawn all but out of the way. */
  stale: ReadonlySet<string>
  palette: MapPalette
}

export interface MapRenderer {
  draw(scene: MapScene): void
  destroy(): void
}

/** A page outside the recency window is drawn at this much of itself. */
export const STALE_OPACITY = 0.14

/**
 * How many shades of age the drawing distinguishes. A page's opacity is
 * continuous, but a batched fill needs one alpha per path, and eight steps
 * between the floor and full strength is finer than the eye reads on a dot of
 * a few pixels. This is what turns ten thousand fills into a handful.
 */
const SHADES = 8

/** A dot whose middle is this far outside the box is still stamped. */
const MARGIN = 12

/** The step the stamps' sizes are rounded to, and how many are kept. */
const SPRITE_STEP = 0.5
const MAX_SPRITES = 400

/** A dot as one small image, and where its middle sits inside it. */
interface Stamp {
  image: HTMLCanvasElement
  centre: number
}

/** What the box shows, in the drawing's own coordinates. */
interface Viewport {
  left: number
  top: number
  right: number
  bottom: number
}

/** The sizes the names beside the dots stay readable at. */
const LABEL_SIZE = 11
const PRIMARY_SIZE = 12
const LABEL_HALO = 3

/** The colours the theme is asked for, in the order the palette lists them. */
const PALETTE_TOKENS: Record<keyof MapPalette, string> = {
  background: '--background',
  dot: '--text-secondary',
  dim: '--text-dim',
  accent: '--accent',
  divider: '--divider',
  family: 'font-family',
}

const FALLBACK_PALETTE: MapPalette = {
  background: '#000000',
  dot: '#b0b0b0',
  dim: '#707070',
  accent: '#7aa2f7',
  divider: '#303030',
  family: 'ui-monospace, monospace',
}

/**
 * The theme's own colours, read off an element inside it. The canvas cannot
 * inherit a stylesheet the way an SVG shape does, so the map reads the tokens
 * once and hands them to the drawing; a host that themes CHROTE differently
 * recolours the map with it and without a code change.
 */
export function readPalette(element: Element | null): MapPalette {
  if (!element || typeof getComputedStyle !== 'function') return FALLBACK_PALETTE
  const style = getComputedStyle(element)
  const palette = { ...FALLBACK_PALETTE }
  ;(Object.keys(PALETTE_TOKENS) as (keyof MapPalette)[]).forEach(name => {
    const value = style.getPropertyValue(PALETTE_TOKENS[name]).trim()
    if (value) palette[name] = value
  })
  return palette
}

/** The alpha a page is drawn at, quantised to the shades the batch allows. */
function shadeOf(node: MapNode, hot: boolean, stale: boolean): number {
  const wanted = stale ? STALE_OPACITY : hot ? 1 : node.opacity
  return Math.max(1, Math.round(wanted * SHADES)) / SHADES
}

export function createCanvasRenderer(canvas: HTMLCanvasElement): MapRenderer | null {
  const context = canvas.getContext('2d')
  if (!context) return null
  let backingWidth = 0
  let backingHeight = 0
  // Where each page sits, rebuilt only when the layout itself changes: a pan
  // redraws the same nodes and must not pay to index them again.
  let indexed: readonly MapNode[] | null = null
  let positions = new Map<string, MapNode>()
  // One small image per colour, shade and size, kept until the map is thrown
  // away. A zoom asks for sizes it has not stamped before, so the sheet is
  // emptied rather than allowed to grow without end.
  let sprites = new Map<string, Stamp>()

  const sprite = (colour: string, alpha: number, radius: number): Stamp => {
    const size = Math.max(SPRITE_STEP, Math.round(radius / SPRITE_STEP) * SPRITE_STEP)
    const key = `${colour}|${alpha}|${size}`
    const found = sprites.get(key)
    if (found) return found
    if (sprites.size > MAX_SPRITES) sprites = new Map()
    const side = Math.ceil(size * 2) + 2
    const image = document.createElement('canvas')
    image.width = side
    image.height = side
    const pen = image.getContext('2d')
    if (pen) {
      pen.fillStyle = colour
      pen.globalAlpha = alpha
      pen.beginPath()
      pen.arc(side / 2, side / 2, size, 0, Math.PI * 2)
      pen.fill()
    }
    const stamp = { image, centre: side / 2 }
    sprites.set(key, stamp)
    return stamp
  }

  const fit = (width: number, height: number, ratio: number) => {
    const wanted = { width: Math.round(width * ratio), height: Math.round(height * ratio) }
    if (wanted.width !== backingWidth || wanted.height !== backingHeight) {
      canvas.width = wanted.width
      canvas.height = wanted.height
      backingWidth = wanted.width
      backingHeight = wanted.height
    }
    canvas.style.width = `${width}px`
    canvas.style.height = `${height}px`
  }

  const drawEdges = (scene: MapScene, view: Viewport) => {
    // Four kinds of hairline: a written link or a shared tag, lit or not. Each
    // kind is one path, whatever the corpus size, so the browser strokes four
    // times rather than thirty thousand. A hairline with both ends off the box
    // is left out: it cannot be seen, and at a corpus of thousands the ones
    // that can be seen are what the frame should cost.
    const paths = [new Path2D(), new Path2D(), new Path2D(), new Path2D()]
    let drawn = false
    scene.edges.forEach(edge => {
      const from = positions.get(edge.from)
      const to = positions.get(edge.to)
      if (!from || !to) return
      if (Math.max(from.x, to.x) < view.left || Math.min(from.x, to.x) > view.right) return
      if (Math.max(from.y, to.y) < view.top || Math.min(from.y, to.y) > view.bottom) return
      const lit = scene.focus.has(edge.from) || scene.focus.has(edge.to)
      const path = paths[(edge.tag ? 1 : 0) + (lit ? 2 : 0)]
      path.moveTo(from.x, from.y)
      path.lineTo(to.x, to.y)
      drawn = true
    })
    if (!drawn) return
    context.lineWidth = 1 / scene.transform.scale
    paths.forEach((path, kind) => {
      const lit = kind >= 2
      context.setLineDash(kind % 2 === 1 ? [2 / scene.transform.scale, 3 / scene.transform.scale] : [])
      context.strokeStyle = lit ? scene.palette.accent : scene.palette.divider
      context.globalAlpha = lit ? 0.8 : 1
      context.stroke(path)
    })
    context.setLineDash([])
    context.globalAlpha = 1
  }

  const drawNodes = (scene: MapScene, view: Viewport, ratio: number) => {
    // A dot is stamped, not drawn. Asking the browser for ten thousand arcs
    // costs about seventy milliseconds a frame; stamping the same dots from a
    // handful of small images costs under ten, and the picture is the same one.
    // A dot's colour says what the page is — accepted, a candidate, or what the
    // operator is on — and its shade says how long ago the page moved.
    const { scale, x: offsetX, y: offsetY } = scene.transform
    scene.nodes.forEach(node => {
      if (node.x < view.left || node.x > view.right || node.y < view.top || node.y > view.bottom) return
      const hot = scene.hot.has(node.path)
      const stale = scene.stale.has(node.path)
      const colour = hot ? scene.palette.accent : node.candidate ? scene.palette.dim : scene.palette.dot
      const stamp = sprite(colour, shadeOf(node, hot, stale), node.r * scale * ratio)
      context.drawImage(
        stamp.image,
        Math.round((node.x * scale + offsetX) * ratio) - stamp.centre,
        Math.round((node.y * scale + offsetY) * ratio) - stamp.centre,
      )
    })
  }

  const drawLabels = (scene: MapScene) => {
    // Type is drawn in the box's own coordinates: the drawing zooms, the names
    // keep the one size they are readable at. Each name is drawn over a halo
    // of the background, so a hairline passing beneath it never runs through
    // the letters.
    const { scale, x: offsetX, y: offsetY } = scene.transform
    context.textBaseline = 'alphabetic'
    context.lineJoin = 'round'
    context.lineWidth = LABEL_HALO
    context.strokeStyle = scene.palette.background
    scene.labels.forEach(label => {
      context.font = `${label.primary ? PRIMARY_SIZE : LABEL_SIZE}px ${scene.palette.family}`
      context.fillStyle = label.primary ? scene.palette.dot : scene.palette.dim
      const x = label.x * scale + offsetX
      const y = label.y * scale + offsetY
      context.strokeText(label.text, x, y)
      context.fillText(label.text, x, y)
    })
  }

  return {
    draw(scene: MapScene) {
      const ratio = typeof window === 'undefined' ? 1 : window.devicePixelRatio || 1
      fit(scene.width, scene.height, ratio)
      context.setTransform(ratio, 0, 0, ratio, 0, 0)
      context.clearRect(0, 0, scene.width, scene.height)
      if (indexed !== scene.nodes) {
        indexed = scene.nodes
        positions = new Map(scene.nodes.map(node => [node.path, node]))
      }
      // What the box shows, in the drawing's own coordinates, with room for a
      // dot whose middle is just outside it.
      const { scale, x: offsetX, y: offsetY } = scene.transform
      const view: Viewport = {
        left: -offsetX / scale - MARGIN,
        top: -offsetY / scale - MARGIN,
        right: (scene.width - offsetX) / scale + MARGIN,
        bottom: (scene.height - offsetY) / scale + MARGIN,
      }
      context.setTransform(ratio * scale, 0, 0, ratio * scale, ratio * offsetX, ratio * offsetY)
      drawEdges(scene, view)
      context.setTransform(1, 0, 0, 1, 0, 0)
      drawNodes(scene, view, ratio)
      context.setTransform(ratio, 0, 0, ratio, 0, 0)
      drawLabels(scene)
    },
    destroy() {
      context.setTransform(1, 0, 0, 1, 0, 0)
      context.clearRect(0, 0, backingWidth, backingHeight)
      sprites = new Map()
    },
  }
}
