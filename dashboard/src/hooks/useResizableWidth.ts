/**
 * Shared horizontal-resize gesture for rails and docked columns.
 *
 * The caller owns the width and where it is stored. Pass the element whose
 * rendered width should be measured, the current limits, and an `onCommit`
 * sink. The hook owns pointer capture, direction-aware drag math, 16px arrow
 * steps, clamping, and committing only when a drag ends successfully. Widths
 * are pixels unless `pixelsPerUnit` adapts an existing caller-owned unit.
 */

import { useCallback, useEffect, useRef, useState } from 'react'
import type {
  KeyboardEventHandler,
  PointerEventHandler,
  RefObject,
} from 'react'

export type ResizeEdge = 'left' | 'right'

export interface UseResizableWidthOptions<T extends HTMLElement> {
  elementRef: RefObject<T>
  width: number
  minWidth: number
  maxWidth: () => number
  edge: ResizeEdge
  onCommit: (width: number) => void
  pixelsPerUnit?: () => number
}

export interface ResizableWidthHandleProps {
  onPointerDown: PointerEventHandler<HTMLDivElement>
  onKeyDown: KeyboardEventHandler<HTMLDivElement>
}

export interface ResizableWidth {
  width: number
  resizing: boolean
  handleProps: ResizableWidthHandleProps
}

const KEYBOARD_STEP = 16

function finiteOr(value: number, fallback: number): number {
  return Number.isFinite(value) ? value : fallback
}

/** Clamp a width even when a caller supplies reversed or non-finite limits. */
function clampResizableWidth(width: number, minWidth: number, maxWidth: number): number {
  const minimum = Math.max(0, finiteOr(minWidth, 0))
  const maximum = Math.max(minimum, finiteOr(maxWidth, Number.POSITIVE_INFINITY))
  return Math.min(maximum, Math.max(minimum, finiteOr(width, minimum)))
}

function keyDelta(key: string, edge: ResizeEdge): number {
  if (key !== 'ArrowLeft' && key !== 'ArrowRight') return 0
  const towardEdge = edge === 'right' ? key === 'ArrowRight' : key === 'ArrowLeft'
  return towardEdge ? KEYBOARD_STEP : -KEYBOARD_STEP
}

export function useResizableWidth<T extends HTMLElement>({
  elementRef,
  width,
  minWidth,
  maxWidth,
  edge,
  onCommit,
  pixelsPerUnit,
}: UseResizableWidthOptions<T>): ResizableWidth {
  const dragWidthRef = useRef<number | null>(null)
  const cleanupRef = useRef<(() => void) | null>(null)
  const [dragWidth, setDragWidth] = useState<number | null>(null)

  const limit = useCallback((next: number) => (
    clampResizableWidth(next, minWidth, maxWidth())
  ), [maxWidth, minWidth])

  const unitScale = useCallback(() => {
    const scale = pixelsPerUnit?.() ?? 1
    return Number.isFinite(scale) && scale > 0 ? scale : 1
  }, [pixelsPerUnit])

  const stopActiveDrag = useCallback((commit: boolean) => {
    const dragged = dragWidthRef.current
    cleanupRef.current?.()
    cleanupRef.current = null
    dragWidthRef.current = null
    setDragWidth(null)
    if (commit && dragged !== null) onCommit(dragged)
  }, [onCommit])

  useEffect(() => () => {
    cleanupRef.current?.()
    cleanupRef.current = null
  }, [])

  const onPointerDown = useCallback<PointerEventHandler<HTMLDivElement>>(event => {
    if (event.button !== 0) return

    const element = elementRef.current
    if (!element) return

    event.preventDefault()
    stopActiveDrag(false)

    const handle = event.currentTarget
    const pointerId = event.pointerId
    const grabbedAt = event.clientX
    const measured = element.getBoundingClientRect().width
    const scale = unitScale()
    const grabbedWidth = measured > 0 ? measured / scale : limit(width)
    const direction = edge === 'right' ? 1 : -1

    const move = (moveEvent: PointerEvent) => {
      if (moveEvent.pointerId !== pointerId) return
      const next = limit(grabbedWidth + direction * (moveEvent.clientX - grabbedAt) / scale)
      dragWidthRef.current = next
      setDragWidth(next)
    }

    const finish = (finishEvent: PointerEvent) => {
      if (finishEvent.pointerId === pointerId) stopActiveDrag(true)
    }

    const cancel = (cancelEvent: PointerEvent) => {
      if (cancelEvent.pointerId === pointerId) stopActiveDrag(false)
    }

    const cleanup = () => {
      handle.removeEventListener('pointermove', move)
      handle.removeEventListener('pointerup', finish)
      handle.removeEventListener('pointercancel', cancel)
      if (handle.hasPointerCapture?.(pointerId)) handle.releasePointerCapture(pointerId)
    }

    cleanupRef.current = cleanup
    handle.setPointerCapture(pointerId)
    handle.addEventListener('pointermove', move)
    handle.addEventListener('pointerup', finish)
    handle.addEventListener('pointercancel', cancel)
  }, [edge, elementRef, limit, stopActiveDrag, unitScale, width])

  const onKeyDown = useCallback<KeyboardEventHandler<HTMLDivElement>>(event => {
    const delta = keyDelta(event.key, edge)
    if (delta === 0) return
    event.preventDefault()
    onCommit(limit(width + delta / unitScale()))
  }, [edge, limit, onCommit, unitScale, width])

  return {
    width: dragWidth ?? limit(width),
    resizing: dragWidth !== null,
    handleProps: { onPointerDown, onKeyDown },
  }
}
