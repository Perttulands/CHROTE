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
 * view's state and the drawing be changed apart from one another.
 *
 * One thing did outgrow the canvas: the hairlines. Measured, thirty thousand of
 * them cost about 65 ms a frame here, which is the whole of the map's miss
 * against its target at ten thousand pages. They now go to a GPU surface of
 * their own (mapEdgesGL.ts), laid under this one, and this file works out what
 * colour each of them is and hands the lot down in one array. A browser with no
 * WebGL 2 gets no layer and the same curves are stroked here instead, so the
 * map draws on a host without a GPU, in a headless run, and in a test.
 *
 * Nothing here runs on a clock. A frame is drawn when the view asks for one.
 */

import type { MapTransform } from '../../hooks/useMapTransform'
import { dotRadius, type MapCard } from './mapBands'
import { curveControl } from './mapCurve'
import { createEdgeLayer, type EdgeLayer } from './mapEdgesGL'
import { alphaOf, edgeLight, glows, lightOf, type MapLight, type MapLighting } from './mapLight'
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
  /** The hues the shelves take, in shelf order, as the theme names them. */
  shelves: string[]
}

export interface MapScene {
  nodes: readonly MapNode[]
  edges: readonly MapEdge[]
  /**
   * What is written on the map, as one layer per band. There are two of them
   * only while one band is giving way to the next, and then only until the
   * fade has run.
   */
  text: readonly MapTextLayer[]
  transform: MapTransform
  width: number
  height: number
  /** What the pointer is asking about, and how far its light reaches. */
  lighting: MapLighting
  /** Pages outside the recency window, drawn all but out of the way. */
  stale: ReadonlySet<string>
  /** Which hue each shelf is drawn in. Empty leaves the map in its greys. */
  hues: ReadonlyMap<string, string>
  palette: MapPalette
}

/** One band's writing, at the strength it is drawn with. */
export interface MapTextLayer {
  /** Names beside their dots, already placed in the layout's coordinates. */
  labels: readonly MapLabel[]
  /** Cards beside their dots, already placed in the box's coordinates. */
  cards: readonly MapCard[]
  alpha: number
}

export interface MapRenderer {
  draw(scene: MapScene): void
  destroy(): void
}

/** A page outside the recency window is drawn at this much of itself. */
export const STALE_OPACITY = 0.14

/** How far back a name steps when the light is on its page's neighbours. */
const DIM_LABEL = 0.6
const FADED_LABEL = 0.3

/** How far the glow reaches past a lit dot, in the box's pixels. */
const GLOW = 6

/**
 * How many shades of age the drawing distinguishes. A page's opacity is
 * continuous, but a batched fill needs one alpha per path, and eight steps
 * between the floor and full strength is finer than the eye reads on a dot of
 * a few pixels. This is what turns ten thousand fills into a handful.
 */
const SHADES = 8

/** The card's face, and where its first line sits inside its box. */
const CARD_SIZE = 11
const CARD_LINE = 14
const CARD_PAD = 6
const CARD_BASELINE = 11

/** A dot whose middle is this far outside the box is still stamped. */
const MARGIN = 12

/** The step the stamps' sizes are rounded to, and how many are kept. */
const SPRITE_STEP = 0.5
const MAX_SPRITES = 1600

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
const PALETTE_TOKENS: Record<Exclude<keyof MapPalette, 'shelves'>, string> = {
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
  shelves: [],
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
  const palette = { ...FALLBACK_PALETTE, shelves: [] as string[] }
  ;(Object.keys(PALETTE_TOKENS) as Exclude<keyof MapPalette, 'shelves'>[]).forEach(name => {
    const value = style.getPropertyValue(PALETTE_TOKENS[name]).trim()
    if (value) palette[name] = value
  })
  // The shelf hues are however many the theme wrote, read until one is
  // missing: the theme decides how many colours a library has, not this file.
  for (let index = 0; index < MAX_SHELF_HUES; index++) {
    const hue = style.getPropertyValue(`--shelf-${index}`).trim()
    if (!hue) break
    palette.shelves.push(hue)
  }
  return palette
}

/** How many shelf hues the map will read off a theme before it stops asking. */
const MAX_SHELF_HUES = 64

/** A colour as three bytes, whatever shape the theme wrote it in. */
function channelsOf(colour: string): [number, number, number] {
  const text = colour.trim()
  if (text.startsWith('#')) {
    const hex = text.slice(1)
    const wide = hex.length >= 6
    const read = (at: number) => {
      const piece = wide ? hex.slice(at * 2, at * 2 + 2) : hex[at] + hex[at]
      const value = parseInt(piece, 16)
      return Number.isNaN(value) ? 0 : value
    }
    return [read(0), read(1), read(2)]
  }
  const numbers = text.match(/[\d.]+/g)
  if (!numbers || numbers.length < 3) return [255, 255, 255]
  return [Number(numbers[0]) || 0, Number(numbers[1]) || 0, Number(numbers[2]) || 0]
}

/**
 * How brightly, and in what, a hairline at this strength is drawn.
 *
 * A hairline that is not lit takes the hue of the page it leaves, held well
 * back: a link then says where it comes from without ever competing with the
 * dots. One inside the light takes the accent, because there the question is
 * not which shelf but what the operator is pointing at.
 */
function edgeInk(light: MapLight, palette: MapPalette, hue?: string): { colour: string; alpha: number } {
  const lit = light === 0 || light === 1
  if (lit) return { colour: palette.accent, alpha: LIT_EDGE }
  return { colour: hue ?? palette.divider, alpha: (alphaOf(light) ?? 1) * (hue ? EDGE_HUE : 1) }
}

/** How much of itself a hairline drawn in its shelf's hue keeps. */
const EDGE_HUE = 0.55

/** How strongly a hairline inside the light is drawn. */
const LIT_EDGE = 0.8

/**
 * The alpha a page is drawn at, quantised to the shades the batch allows. A page
 * outside the recency window is held back whatever the light does, because the
 * window is the operator's own question about what has moved lately.
 */
function shadeOf(node: MapNode, light: MapLight, stale: boolean): number {
  const lit = alphaOf(light) ?? node.opacity
  const wanted = stale ? Math.min(STALE_OPACITY, lit) : lit
  return Math.max(1, Math.round(wanted * SHADES)) / SHADES
}

export function createCanvasRenderer(
  canvas: HTMLCanvasElement,
  edgeCanvas?: HTMLCanvasElement | null,
): MapRenderer | null {
  const context = canvas.getContext('2d')
  if (!context) return null
  // The hairlines go to the GPU when this browser has one to give. When it has
  // not — a headless run, a test, a browser without WebGL 2 — the layer is null
  // and the same curves are stroked here, which is the picture the corpus a
  // reader keeps has always been drawn at.
  const layer: EdgeLayer | null = edgeCanvas ? createEdgeLayer(edgeCanvas) : null
  let backingWidth = 0
  let backingHeight = 0
  // Where each page sits, rebuilt only when the layout itself changes: a pan
  // redraws the same nodes and must not pay to index them again.
  let indexed: readonly MapNode[] | null = null
  let positions = new Map<string, MapNode>()
  // Each hairline's two ends as positions in the node array, so the strength of
  // thirty thousand of them can be worked out without thirty thousand lookups
  // by name. Rebuilt only when the drawing itself changed.
  let joined: readonly MapEdge[] | null = null
  let edgeFrom = new Int32Array(0)
  let edgeTo = new Int32Array(0)
  // The colour of every hairline, four bytes each, as the GPU layer wants it.
  // A fresh array is made whenever the light or the theme changed, and the same
  // one is handed down otherwise, which is what tells the layer to leave the
  // buffer it already holds alone.
  let colours: Uint8Array | null = null
  let painted: { lighting: MapLighting; palette: MapPalette; hues: ReadonlyMap<string, string> } | null = null
  // One small image per colour, shade and size, kept until the map is thrown
  // away. A zoom asks for sizes it has not stamped before, so the sheet is
  // emptied rather than allowed to grow without end.
  let sprites = new Map<string, Stamp>()

  const sprite = (colour: string, alpha: number, radius: number, glow: number): Stamp => {
    const size = Math.max(SPRITE_STEP, Math.round(radius / SPRITE_STEP) * SPRITE_STEP)
    const key = `${colour}|${alpha}|${size}|${glow}`
    const found = sprites.get(key)
    if (found) return found
    if (sprites.size > MAX_SPRITES) sprites = new Map()
    const side = Math.ceil(size * 2) + 2 + glow * 2
    const image = document.createElement('canvas')
    image.width = side
    image.height = side
    const pen = image.getContext('2d')
    if (pen) {
      pen.fillStyle = colour
      pen.globalAlpha = alpha
      // The glow is blurred once, into the stamp, and never again: blurring
      // every lit dot on every frame is what makes a canvas map crawl.
      if (glow > 0) {
        pen.shadowColor = colour
        pen.shadowBlur = glow
      }
      pen.beginPath()
      pen.arc(side / 2, side / 2, size, 0, Math.PI * 2)
      pen.fill()
      if (glow > 0) pen.fill()
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

  /** Where each hairline's ends sit in the node array. */
  let joinedNodes: readonly MapNode[] | null = null
  const join = (scene: MapScene) => {
    if (joined === scene.edges && joinedNodes === scene.nodes) return
    joined = scene.edges
    joinedNodes = scene.nodes
    const at = new Map(scene.nodes.map((node, position) => [node.path, position]))
    edgeFrom = new Int32Array(scene.edges.length)
    edgeTo = new Int32Array(scene.edges.length)
    scene.edges.forEach((edge, position) => {
      edgeFrom[position] = at.get(edge.from) ?? -1
      edgeTo[position] = at.get(edge.to) ?? -1
    })
    colours = null
  }

  /**
   * The colour of every hairline, worked out once per light rather than once
   * per frame. Each page's strength is read once and every hairline reads its
   * two ends out of that, so a hover costs the corpus once rather than the
   * links times two lookups by name.
   */
  const repaint = (scene: MapScene) => {
    if (colours && painted && painted.lighting === scene.lighting
      && painted.palette === scene.palette && painted.hues === scene.hues) return colours
    const strengths = new Int8Array(scene.nodes.length)
    scene.nodes.forEach((node, position) => {
      const light = lightOf(node, scene.lighting)
      strengths[position] = light === null ? -1 : light === 'out' ? 3 : light
    })
    const decode = (code: number): MapLight => (code < 0 ? null : code === 3 ? 'out' : (code as 0 | 1 | 2))
    // Five strengths, and a colour worked out once for each of them: what a
    // hairline costs here is a lookup, not a parse.
    // Five strengths for each hue a shelf can be, worked out once: what a
    // hairline costs here is a lookup, not a parse.
    const shelves = Array.from(new Set(scene.nodes.map(node => node.shelf)))
    const ink = new Map<string, number[][]>()
    const inkFor = (shelf: string) => {
      const found = ink.get(shelf)
      if (found) return found
      const built = [-1, 0, 1, 2, 3].map(code => {
        const { colour, alpha } = edgeInk(decode(code), scene.palette, scene.hues.get(shelf))
        const [r, g, b] = channelsOf(colour)
        return [r, g, b, Math.round(Math.max(0, Math.min(1, alpha)) * 255)]
      })
      ink.set(shelf, built)
      return built
    }
    shelves.forEach(inkFor)
    const built = new Uint8Array(scene.edges.length * 4)
    for (let index = 0; index < scene.edges.length; index++) {
      const from = edgeFrom[index]
      const to = edgeTo[index]
      if (from < 0 || to < 0) continue
      const light = edgeLight(decode(strengths[from]), decode(strengths[to]), scene.lighting)
      const shade = inkFor(scene.nodes[from].shelf)[light === null ? 0 : light === 'out' ? 4 : light + 1]
      built[index * 4] = shade[0]
      built[index * 4 + 1] = shade[1]
      built[index * 4 + 2] = shade[2]
      built[index * 4 + 3] = shade[3]
    }
    colours = built
    painted = { lighting: scene.lighting, palette: scene.palette, hues: scene.hues }
    return built
  }

  /**
   * The hairlines on the Canvas 2D surface: what draws them when the GPU layer
   * is not there. Four kinds — a written link or a shared tag, lit or not —
   * each one path and one stroke, so the browser strokes four times rather than
   * thirty thousand. A hairline with both ends off the box is left out: it
   * cannot be seen, and the ones that can be are what the frame should cost.
   */
  const strokeEdges = (scene: MapScene, view: Viewport) => {
    const strengths: MapLight[] = [null, 0, 2, 'out']
    // One path per strength, per kind of link, and per shelf the link leaves,
    // so a corpus of a dozen shelves is a few dozen strokes however many links
    // it has rather than one stroke each.
    const paths = new Map<string, { path: Path2D; kind: number; tag: number; shelf: string }>()
    scene.edges.forEach(edge => {
      const from = positions.get(edge.from)
      const to = positions.get(edge.to)
      if (!from || !to) return
      if (Math.max(from.x, to.x) < view.left || Math.min(from.x, to.x) > view.right) return
      if (Math.max(from.y, to.y) < view.top || Math.min(from.y, to.y) > view.bottom) return
      const light = edgeLight(lightOf(from, scene.lighting), lightOf(to, scene.lighting), scene.lighting)
      const kind = light === null ? 0 : light === 'out' ? 3 : light === 2 ? 2 : 1
      const tag = edge.tag ? 1 : 0
      const at = `${kind}|${tag}|${from.shelf}`
      let batch = paths.get(at)
      if (!batch) {
        batch = { path: new Path2D(), kind, tag, shelf: from.shelf }
        paths.set(at, batch)
      }
      const control = curveControl(from, to)
      batch.path.moveTo(from.x, from.y)
      batch.path.quadraticCurveTo(control.x, control.y, to.x, to.y)
    })
    if (paths.size === 0) return
    context.lineWidth = 1 / scene.transform.scale
    paths.forEach(batch => {
      const { colour, alpha } = edgeInk(strengths[batch.kind], scene.palette, scene.hues.get(batch.shelf))
      context.setLineDash(batch.tag === 1 ? [2 / scene.transform.scale, 3 / scene.transform.scale] : [])
      context.strokeStyle = colour
      context.globalAlpha = alpha
      context.stroke(batch.path)
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
      const light = lightOf(node, scene.lighting)
      const stale = scene.stale.has(node.path)
      const lit = glows(light)
      // A page's colour says which shelf it is on. A candidate is not on one
      // yet — it is a proposal — so it stays grey, and the map reads at a
      // glance as what has been accepted, coloured, among what has not.
      const colour = lit
        ? scene.palette.accent
        : node.candidate ? scene.palette.dim : scene.hues.get(node.shelf) ?? scene.palette.dot
      const stamp = sprite(colour, shadeOf(node, light, stale), dotRadius(node, scale) * ratio, lit ? GLOW * ratio : 0)
      context.drawImage(
        stamp.image,
        Math.round((node.x * scale + offsetX) * ratio) - stamp.centre,
        Math.round((node.y * scale + offsetY) * ratio) - stamp.centre,
      )
    })
  }

  const drawLabels = (scene: MapScene, layer: MapTextLayer) => {
    // Type is drawn in the box's own coordinates: the drawing zooms, the names
    // keep the one size they are readable at. Each name is drawn over a halo
    // of the background, so a hairline passing beneath it never runs through
    // the letters.
    const { scale, x: offsetX, y: offsetY } = scene.transform
    context.textBaseline = 'alphabetic'
    context.lineJoin = 'round'
    context.lineWidth = LABEL_HALO
    context.strokeStyle = scene.palette.background
    layer.labels.forEach(label => {
      // A name steps back with the page it names: with the light on, a landmark
      // outside the neighbourhood would otherwise be the loudest thing left on
      // the map, which is the opposite of what the pointer asked for.
      const node = positions.get(label.path)
      const light = node ? lightOf(node, scene.lighting) : null
      context.globalAlpha = layer.alpha * (light === 'out' ? FADED_LABEL : light === 2 ? DIM_LABEL : 1)
      context.font = `${label.primary ? PRIMARY_SIZE : LABEL_SIZE}px ${scene.palette.family}`
      context.fillStyle = label.primary ? scene.palette.dot : scene.palette.dim
      const x = label.x * scale + offsetX
      const y = label.y * scale + offsetY
      context.strokeText(label.text, x, y)
      context.fillText(label.text, x, y)
    })
    context.globalAlpha = layer.alpha
  }

  const drawCards = (scene: MapScene, layer: MapTextLayer) => {
    // A card is already placed in the box's coordinates: what the page is, when
    // it last moved, how long it is, and what it shares a tag with, on its own
    // ground so the drawing beneath it does not run through the words.
    if (layer.cards.length === 0) return
    context.textBaseline = 'alphabetic'
    context.font = `${CARD_SIZE}px ${scene.palette.family}`
    context.lineWidth = 1
    layer.cards.forEach(card => {
      context.fillStyle = scene.palette.background
      context.fillRect(card.x, card.y, card.width, card.height)
      context.strokeStyle = scene.palette.divider
      context.strokeRect(card.x + 0.5, card.y + 0.5, card.width - 1, card.height - 1)
      card.lines.forEach((line, index) => {
        context.fillStyle = index === 0 ? scene.palette.dot : scene.palette.dim
        context.fillText(line, card.x + CARD_PAD, card.y + CARD_PAD + CARD_LINE * index + CARD_BASELINE)
      })
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
      if (layer) {
        join(scene)
        layer.draw({
          nodes: scene.nodes,
          edges: scene.edges,
          colours: repaint(scene),
          width: scene.width,
          height: scene.height,
          scale,
          offsetX,
          offsetY,
          ratio,
        })
      } else {
        context.setTransform(ratio * scale, 0, 0, ratio * scale, ratio * offsetX, ratio * offsetY)
        strokeEdges(scene, view)
      }
      context.setTransform(1, 0, 0, 1, 0, 0)
      drawNodes(scene, view, ratio)
      context.setTransform(ratio, 0, 0, ratio, 0, 0)
      scene.text.forEach(layer => {
        if (layer.alpha <= 0) return
        context.globalAlpha = layer.alpha
        drawLabels(scene, layer)
        drawCards(scene, layer)
      })
      context.globalAlpha = 1
    },
    destroy() {
      context.setTransform(1, 0, 0, 1, 0, 0)
      context.clearRect(0, 0, backingWidth, backingHeight)
      sprites = new Map()
      layer?.destroy()
    },
  }
}
