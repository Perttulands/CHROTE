/**
 * Moving through a drawing: zoom, pan, pinch, and the way back.
 *
 * A drawing laid out in its own coordinates is shown through one transform —
 * a scale and an offset. The wheel scales about the pointer so the thing
 * under it stays under it, a drag moves the offset, two fingers do both at
 * once, and a reset restores the fit the layout was made for.
 *
 * The arithmetic is pure and exported on its own, so a view can test what it
 * shows without a pointer, and so more than one drawing can be moved through
 * the same way: the Library map and the Beads flow both use this.
 */

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { PointerEvent as ReactPointerEvent, RefObject } from 'react'

/** A point in either the drawing's coordinates or the box's, as named. */
export interface MapPoint {
  x: number
  y: number
}

/** What is shown: the drawing scaled by `scale` and moved by `x`, `y`. */
export interface MapTransform {
  x: number
  y: number
  scale: number
}

export interface MapScaleLimits {
  min: number
  max: number
}

export const IDENTITY: MapTransform = { x: 0, y: 0, scale: 1 }

/** How far in and out a drawing may be taken. */
export const DEFAULT_LIMITS: MapScaleLimits = { min: 0.4, max: 8 }

/** One wheel notch's share of a doubling; a trackpad sends many small ones. */
const WHEEL_RATE = 0.0015

/** A line of wheel delta in pixels, and a page of it. */
const LINE_HEIGHT = 16
const PAGE_HEIGHT = 400

/** A pointer that moved less than this never panned; the click still counts. */
const DRAG_SLOP = 4

export function clampScale(scale: number, limits: MapScaleLimits = DEFAULT_LIMITS): number {
  if (!Number.isFinite(scale)) return limits.min
  return Math.max(limits.min, Math.min(limits.max, scale))
}

/** Where a point of the drawing lands in the box. */
export function toScreen(transform: MapTransform, point: MapPoint): MapPoint {
  return { x: point.x * transform.scale + transform.x, y: point.y * transform.scale + transform.y }
}

/** Which point of the drawing sits under a point of the box. */
export function toWorld(transform: MapTransform, point: MapPoint): MapPoint {
  return { x: (point.x - transform.x) / transform.scale, y: (point.y - transform.y) / transform.scale }
}

/**
 * Scale by `factor` about a point of the box, keeping what is under that
 * point under it. Clamping the scale first means a gesture past the limit
 * neither zooms nor drifts.
 */
export function zoomAbout(
  transform: MapTransform,
  pointer: MapPoint,
  factor: number,
  limits: MapScaleLimits = DEFAULT_LIMITS,
): MapTransform {
  const scale = clampScale(transform.scale * factor, limits)
  if (scale === transform.scale) return transform
  const world = toWorld(transform, pointer)
  return { scale, x: pointer.x - world.x * scale, y: pointer.y - world.y * scale }
}

export function panBy(transform: MapTransform, dx: number, dy: number): MapTransform {
  return { ...transform, x: transform.x + dx, y: transform.y + dy }
}

/** The transform that puts a point of the drawing in the middle of the box. */
export function centreTransform(
  point: MapPoint,
  scale: number,
  viewport: { width: number; height: number },
  limits: MapScaleLimits = DEFAULT_LIMITS,
): MapTransform {
  const clamped = clampScale(scale, limits)
  return { scale: clamped, x: viewport.width / 2 - point.x * clamped, y: viewport.height / 2 - point.y * clamped }
}

/** The factor one wheel event asks for, whatever unit it counts in. */
export function wheelFactor(deltaY: number, deltaMode = 0): number {
  const unit = deltaMode === 1 ? LINE_HEIGHT : deltaMode === 2 ? PAGE_HEIGHT : 1
  return Math.exp(-deltaY * unit * WHEEL_RATE)
}

export function isPanned(transform: MapTransform): boolean {
  return transform.scale !== 1 || transform.x !== 0 || transform.y !== 0
}

export interface UseMapTransformOptions {
  /** The box the drawing is shown in, which `centreOn` and `reset` need. */
  width: number
  height: number
  /** Whether the gestures are live; a strip or a thumbnail passes false. */
  enabled?: boolean
  limits?: MapScaleLimits
}

export interface MapTransformControls<T extends Element = SVGSVGElement> {
  /** Put this on the element the gestures happen over. */
  ref: RefObject<T>
  transform: MapTransform
  /** Ready for an SVG group's `transform` attribute. */
  groupTransform: string
  /** True once the operator has moved the drawing at all. */
  moved: boolean
  reset: () => void
  /** Bring a point of the drawing to the middle, at `scale` if given. */
  centreOn: (point: MapPoint, scale?: number) => void
  toScreen: (point: MapPoint) => MapPoint
  toWorld: (point: MapPoint) => MapPoint
  /** Whether the pointer gesture that just ended was a pan, not a click. */
  panned: () => boolean
  handlers: {
    onPointerDown: (event: ReactPointerEvent<T>) => void
    onPointerMove: (event: ReactPointerEvent<T>) => void
    onPointerUp: (event: ReactPointerEvent<T>) => void
    onPointerCancel: (event: ReactPointerEvent<T>) => void
  }
}

interface Contact {
  x: number
  y: number
}

/** Where an event happened inside the element, in the box's coordinates. */
function inBox(element: Element | null, clientX: number, clientY: number): MapPoint {
  const box = element?.getBoundingClientRect()
  return box ? { x: clientX - box.left, y: clientY - box.top } : { x: clientX, y: clientY }
}

function midpoint(points: Contact[]): MapPoint {
  const sum = points.reduce((total, point) => ({ x: total.x + point.x, y: total.y + point.y }), { x: 0, y: 0 })
  return { x: sum.x / points.length, y: sum.y / points.length }
}

function spread(points: Contact[]): number {
  return Math.hypot(points[0].x - points[1].x, points[0].y - points[1].y)
}

export function useMapTransform<T extends Element = SVGSVGElement>(
  options: UseMapTransformOptions,
): MapTransformControls<T> {
  const { width, height, enabled = true, limits = DEFAULT_LIMITS } = options
  const ref = useRef<T>(null)
  const [transform, setTransform] = useState<MapTransform>(IDENTITY)
  const contacts = useRef(new Map<number, Contact>())
  const pinch = useRef<number | null>(null)
  const dragged = useRef(false)
  const travelled = useRef(0)
  const pendingPan = useRef<MapPoint>({ x: 0, y: 0 })

  // The wheel is bound by hand: React's is passive, and a passive listener
  // may not keep the page from scrolling under a zoom.
  useEffect(() => {
    const element = ref.current
    if (!element || !enabled) return
    const onWheel = (raw: Event) => {
      const event = raw as WheelEvent
      event.preventDefault()
      const pointer = inBox(element, event.clientX, event.clientY)
      setTransform(current => zoomAbout(current, pointer, wheelFactor(event.deltaY, event.deltaMode), limits))
    }
    element.addEventListener('wheel', onWheel, { passive: false })
    return () => element.removeEventListener('wheel', onWheel)
  }, [enabled, limits])

  // A drawing that is no longer moved through keeps nothing to restore.
  useEffect(() => {
    if (enabled) return
    contacts.current.clear()
    pinch.current = null
    pendingPan.current = { x: 0, y: 0 }
    setTransform(IDENTITY)
  }, [enabled])

  const reset = useCallback(() => setTransform(IDENTITY), [])

  const centreOn = useCallback((point: MapPoint, scale?: number) => {
    setTransform(current => centreTransform(point, scale ?? current.scale, { width, height }, limits))
  }, [height, limits, width])

  const handlers = useMemo(() => {
    const track = (event: ReactPointerEvent<T>) => {
      contacts.current.set(event.pointerId, inBox(ref.current, event.clientX, event.clientY))
    }

    // The pointer is captured only once the gesture has become a pan, so that
    // a plain click still reaches the node it was aimed at: a capture set on
    // the way down would take the click with it.
    const holdOn = (event: ReactPointerEvent<T>) => {
      if (dragged.current) return
      dragged.current = true
      if (event.currentTarget.hasPointerCapture?.(event.pointerId) === false) {
        event.currentTarget.setPointerCapture?.(event.pointerId)
      }
    }

    return {
      onPointerDown: (event: ReactPointerEvent<T>) => {
        if (!enabled || event.button !== 0) return
        dragged.current = false
        travelled.current = 0
        pendingPan.current = { x: 0, y: 0 }
        track(event)
        if (contacts.current.size === 2) pinch.current = spread([...contacts.current.values()])
      },
      onPointerMove: (event: ReactPointerEvent<T>) => {
        if (!enabled || !contacts.current.has(event.pointerId)) return
        const previous = contacts.current.get(event.pointerId) as Contact
        const next = inBox(ref.current, event.clientX, event.clientY)
        contacts.current.set(event.pointerId, next)
        const points = [...contacts.current.values()]

        if (points.length >= 2 && pinch.current !== null) {
          const now = spread(points.slice(0, 2))
          const factor = pinch.current > 0 ? now / pinch.current : 1
          pinch.current = now
          holdOn(event)
          setTransform(current => zoomAbout(current, midpoint(points.slice(0, 2)), factor, limits))
          return
        }

        const dx = next.x - previous.x
        const dy = next.y - previous.y
        if (dx === 0 && dy === 0) return
        // A gesture is a pan once it has travelled past the slop; under that
        // it is a click that wobbled, and the node under it still opens.
        travelled.current += Math.hypot(dx, dy)
        pendingPan.current = {
          x: pendingPan.current.x + dx,
          y: pendingPan.current.y + dy,
        }
        if (!dragged.current && travelled.current < DRAG_SLOP) return
        holdOn(event)
        const movement = pendingPan.current
        pendingPan.current = { x: 0, y: 0 }
        setTransform(current => panBy(current, movement.x, movement.y))
      },
      onPointerUp: (event: ReactPointerEvent<T>) => {
        contacts.current.delete(event.pointerId)
        if (contacts.current.size < 2) pinch.current = null
        if (contacts.current.size === 0) {
          travelled.current = 0
          pendingPan.current = { x: 0, y: 0 }
        }
        if (event.currentTarget.hasPointerCapture?.(event.pointerId)) {
          event.currentTarget.releasePointerCapture?.(event.pointerId)
        }
      },
      onPointerCancel: (event: ReactPointerEvent<T>) => {
        contacts.current.delete(event.pointerId)
        if (contacts.current.size < 2) pinch.current = null
        if (contacts.current.size === 0) {
          travelled.current = 0
          pendingPan.current = { x: 0, y: 0 }
        }
      },
    }
  }, [enabled, limits])

  return {
    ref,
    transform,
    groupTransform: `translate(${transform.x} ${transform.y}) scale(${transform.scale})`,
    moved: isPanned(transform),
    reset,
    centreOn,
    toScreen: useCallback((point: MapPoint) => toScreen(transform, point), [transform]),
    toWorld: useCallback((point: MapPoint) => toWorld(transform, point), [transform]),
    panned: useCallback(() => dragged.current, []),
    handlers,
  }
}
