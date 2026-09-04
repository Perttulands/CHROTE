/**
 * What the map says at each distance.
 *
 * Zooming used to make the circles bigger and nothing else. It now changes what
 * is written: far away the shelves and the landmarks, the way a map of a country
 * names its cities; closer in every page's own name; closer still a card per
 * page, with what the page is, when it last moved, how long it is and what it
 * shares a tag with. The dot itself stops growing past the landing scale,
 * because a map read closely should say more, not draw fatter.
 *
 * The thresholds live here, once, and so does the rule that places a card. Both
 * are arithmetic: no DOM, no canvas, so the placement can be read at a bench.
 */

import type { MapNode } from './mapLayout'
import type { MapTransform } from '../../hooks/useMapTransform'

/** How far in the map is taken before every page is named, and before cards. */
export const MID_SCALE = 1.6
export const NEAR_SCALE = 3

/** How long one band takes to give way to the next, in milliseconds. */
export const BAND_FADE = 180

export type MapBand = 'far' | 'mid' | 'near'

export function bandAt(scale: number): MapBand {
  if (scale >= NEAR_SCALE) return 'near'
  if (scale >= MID_SCALE) return 'mid'
  return 'far'
}

/**
 * How many pages the middle band names. Every page in a library of any size a
 * reader keeps is inside this; past it the names would be a wall rather than a
 * reading, and the largest pages are the ones kept.
 */
export const MID_LABELS = 600

/** How many cards the near band draws before the picture is a wall of cards. */
export const MAX_CARDS = 60

/** The card's measure: an 11px monospaced face, and the room around it. */
const CARD_CHAR = 6.7
const CARD_LINE = 14
const CARD_PAD = 6
const CARD_GAP = 10

/** A page's card, placed in the box's own coordinates. */
export interface MapCard {
  path: string
  x: number
  y: number
  width: number
  height: number
  lines: string[]
}

/** The size of the card a page's lines would take. */
export function cardSize(lines: readonly string[]): { width: number; height: number } {
  const longest = lines.reduce((most, line) => Math.max(most, line.length), 0)
  return { width: Math.round(longest * CARD_CHAR) + CARD_PAD * 2, height: lines.length * CARD_LINE + CARD_PAD * 2 }
}

export interface CardOptions {
  transform: MapTransform
  width: number
  height: number
  /** What the card says about a page, one line at a time. */
  describe: (node: MapNode) => string[]
  /** Pages named elsewhere — the one under the pointer — which take no card. */
  suppress?: ReadonlySet<string>
  /** Ground already taken by the readout, which no card may sit on. */
  reserved?: readonly { x: number; y: number; width: number; height: number }[]
  maxCards?: number
}

/**
 * A card for each page in view, beside its dot.
 *
 * The largest pages are served first, so which page keeps its card where two
 * cannot both have one is a property of the corpus rather than of the order the
 * server happened to answer in. A card that would run off the right edge is
 * turned back on itself, and one that would sit on another is dropped rather
 * than drawn over it.
 */
export function placeCards(nodes: readonly MapNode[], options: CardOptions): MapCard[] {
  const { transform, width, height, describe } = options
  const limit = options.maxCards ?? MAX_CARDS
  const placed: MapCard[] = (options.reserved ?? []).map(box => ({ ...box, path: '', lines: [] }))
  const taken = placed.length
  const ordered = nodes
    .filter(node => !options.suppress?.has(node.path))
    .slice()
    .sort((a, b) => b.r - a.r || (a.path < b.path ? -1 : 1))

  for (const node of ordered) {
    if (placed.length - taken >= limit) break
    const point = { x: node.x * transform.scale + transform.x, y: node.y * transform.scale + transform.y }
    if (point.x < 0 || point.x > width || point.y < 0 || point.y > height) continue
    const lines = describe(node)
    const { width: boxWidth, height: boxHeight } = cardSize(lines)
    const radius = dotRadius(node, transform.scale)
    const right = point.x + radius + CARD_GAP
    const x = right + boxWidth > width ? point.x - radius - CARD_GAP - boxWidth : right
    const y = Math.max(0, Math.min(height - boxHeight, point.y - boxHeight / 2))
    if (x < 0) continue
    const card = { path: node.path, x, y, width: boxWidth, height: boxHeight, lines }
    const clash = placed.some(other => (
      x < other.x + other.width && other.x < x + boxWidth && y < other.y + other.height && other.y < y + boxHeight
    ))
    if (clash) continue
    placed.push(card)
  }
  return placed.slice(taken)
}

/**
 * How large a dot is drawn, in the box's own pixels.
 *
 * Up to the landing scale a page grows with the drawing, so zooming out shrinks
 * the whole picture together. Past it the dot keeps the size it has: taking the
 * map closer in is asking to read it, not asking for blobs.
 */
export function dotRadius(node: MapNode, scale: number): number {
  return node.r * Math.min(scale, 1)
}
