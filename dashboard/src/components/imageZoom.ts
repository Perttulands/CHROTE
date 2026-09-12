/**
 * How big a picture is drawn, everywhere a picture is drawn.
 *
 * One level — fit, 1:1, or a percentage — held for the device rather than for
 * the file, because the operator is setting how they want to look at pictures,
 * not annotating this one. The glance, the Files panel's picture and the file
 * viewer all read it, so a level set in one is in force in the others and in
 * every later picture.
 *
 * Fit is the old rule and keeps its promise: the picture at 1:1 when it fits
 * the room it has, scaled down at its own ratio when it does not, and never
 * up. A percentage is the operator's word and is obeyed literally, which is
 * what lets a picture be bigger than the room and scroll inside it.
 *
 * The steps are a ladder rather than a fixed addition, so stepping is the same
 * gesture at 10% and at 800%: each press lands on the next stop, and the first
 * press after fit lands on the stop nearest what is already on screen.
 */

import { useSyncExternalStore } from 'react'

export interface PixelSize {
  width: number
  height: number
}

export type ImageZoomLevel =
  | { kind: 'fit' }
  | { kind: 'percent'; percent: number }

export const IMAGE_ZOOM_FIT: ImageZoomLevel = { kind: 'fit' }
export const IMAGE_ZOOM_ONE_TO_ONE: ImageZoomLevel = { kind: 'percent', percent: 100 }

export const IMAGE_ZOOM_MIN_PERCENT = 10
export const IMAGE_ZOOM_MAX_PERCENT = 800

/** The stops a step lands on, smallest first. */
const LADDER: readonly number[] = [10, 25, 33, 50, 67, 75, 100, 125, 150, 200, 300, 400, 600, 800]

export function clampZoomPercent(percent: number): number {
  if (!Number.isFinite(percent)) return 100
  return Math.min(IMAGE_ZOOM_MAX_PERCENT, Math.max(IMAGE_ZOOM_MIN_PERCENT, Math.round(percent)))
}

export function zoomPercentLevel(percent: number): ImageZoomLevel {
  return { kind: 'percent', percent: clampZoomPercent(percent) }
}

/** The picture at 1:1 when it fits the room, down at its own ratio when it
 * does not, and never up. */
export function fitImage(natural: PixelSize, room: PixelSize): PixelSize {
  const scale = Math.min(1, room.width / natural.width, room.height / natural.height)
  return {
    width: Math.max(1, Math.floor(natural.width * scale)),
    height: Math.max(1, Math.floor(natural.height * scale)),
  }
}

/** The picture as the level draws it, in whole pixels and never under one. */
export function drawImage(natural: PixelSize, room: PixelSize, level: ImageZoomLevel): PixelSize {
  if (level.kind === 'fit') return fitImage(natural, room)
  const scale = level.percent / 100
  return {
    width: Math.max(1, Math.round(natural.width * scale)),
    height: Math.max(1, Math.round(natural.height * scale)),
  }
}

/** The percent the picture is actually drawn at, which is the word the header says. */
export function drawnZoomPercent(natural: PixelSize, room: PixelSize, level: ImageZoomLevel): number {
  if (level.kind === 'percent') return level.percent
  return Math.max(1, Math.round((fitImage(natural, room).width / natural.width) * 100))
}

/** The percent a picture of this width would be drawn at to fill the room. */
export function zoomPercentForWidth(natural: PixelSize, roomWidth: number): number {
  if (!(natural.width > 0)) return 100
  return clampZoomPercent((roomWidth / natural.width) * 100)
}

/** The next stop up or down the ladder from where the picture is now drawn. */
export function stepImageZoom(
  level: ImageZoomLevel,
  natural: PixelSize,
  room: PixelSize,
  direction: 1 | -1,
): ImageZoomLevel {
  const here = drawnZoomPercent(natural, room, level)
  const stop = direction === 1
    ? LADDER.find(candidate => candidate > here)
    : [...LADDER].reverse().find(candidate => candidate < here)
  return zoomPercentLevel(stop ?? here)
}

/** How the level reads in a header: `Fit`, `1:1`, or `140%`. */
export function zoomPercentWord(percent: number): string {
  return `${Math.round(percent)}%`
}

const STORAGE_KEY = 'chrote.imageZoom.v1'
const STORAGE_VERSION = 1

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}

function readStored(): ImageZoomLevel {
  if (typeof window === 'undefined') return IMAGE_ZOOM_FIT
  try {
    const parsed: unknown = JSON.parse(window.localStorage.getItem(STORAGE_KEY) || 'null')
    if (!isRecord(parsed) || parsed.version !== STORAGE_VERSION) return IMAGE_ZOOM_FIT
    if (typeof parsed.percent === 'number' && Number.isFinite(parsed.percent)) {
      return zoomPercentLevel(parsed.percent)
    }
    return IMAGE_ZOOM_FIT
  } catch {
    return IMAGE_ZOOM_FIT
  }
}

let level: ImageZoomLevel = readStored()
const listeners = new Set<() => void>()

function subscribe(listener: () => void): () => void {
  listeners.add(listener)
  return () => { listeners.delete(listener) }
}

function read(): ImageZoomLevel {
  return level
}

function sameLevel(a: ImageZoomLevel, b: ImageZoomLevel): boolean {
  if (a.kind === 'fit' || b.kind === 'fit') return a.kind === b.kind
  return a.percent === b.percent
}

/** Set the level for this device, and for every picture drawn from now on. */
export function setImageZoom(next: ImageZoomLevel): void {
  const settled: ImageZoomLevel = next.kind === 'fit' ? IMAGE_ZOOM_FIT : zoomPercentLevel(next.percent)
  if (sameLevel(settled, level)) return
  level = settled
  if (typeof window !== 'undefined') {
    try {
      window.localStorage.setItem(STORAGE_KEY, JSON.stringify(
        settled.kind === 'fit' ? { version: STORAGE_VERSION } : { version: STORAGE_VERSION, percent: settled.percent },
      ))
    } catch {
      // Private mode and quota failures must not stop the picture being drawn.
    }
  }
  listeners.forEach(listener => listener())
}

export function useImageZoom(): ImageZoomLevel {
  return useSyncExternalStore(subscribe, read, read)
}

/**
 * The pixels to draw a picture at outside the glance, or null to leave the
 * stylesheet's fit alone. Fit is what the Files panel and the file viewer
 * already do in CSS; a percent is the operator's word and is set in pixels.
 */
export function zoomedPixels(natural: PixelSize | null, level: ImageZoomLevel): PixelSize | null {
  if (!natural || level.kind === 'fit') return null
  return drawImage(natural, natural, level)
}

/** Test seam: back to fit, with nothing remembered. */
export function resetImageZoomForTest(): void {
  level = IMAGE_ZOOM_FIT
  if (typeof window !== 'undefined') {
    try {
      window.localStorage.removeItem(STORAGE_KEY)
    } catch {
      // Nothing to clear.
    }
  }
  listeners.forEach(listener => listener())
}
