/**
 * Shared horizontal-resize gesture for rails and docked columns.
 *
 * The caller owns the width and where it is stored. Pass the element whose
 * rendered width should be measured, the current limits, and an `onCommit`
 * sink. The hook owns direction-aware drag math, clamping, and committing only
 * when a drag ends successfully; the pointer capture, the 16px arrow step and
 * the clamp itself come from `resizeGesture`, which the floating-window frame
 * shares. Widths are pixels unless `pixelsPerUnit` adapts an existing
 * caller-owned unit.
 */

import { useCallback, useEffect, useRef, useState } from 'react'
import type {
  KeyboardEventHandler,
  PointerEventHandler,
  RefObject,
} from 'react'
import { RESIZE_KEYBOARD_STEP, capturePointerDrag, clampLength } from './resizeGesture'

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

function keyDelta(key: string, edge: ResizeEdge): number {
  if (key !== 'ArrowLeft' && key !== 'ArrowRight') return 0
  const towardEdge = edge === 'right' ? key === 'ArrowRight' : key === 'ArrowLeft'
  return towardEdge ? RESIZE_KEYBOARD_STEP : -RESIZE_KEYBOARD_STEP
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
    clampLength(next, minWidth, maxWidth())
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

    cleanupRef.current = capturePointerDrag(handle, pointerId, {
      move: moveEvent => {
        const next = limit(grabbedWidth + direction * (moveEvent.clientX - grabbedAt) / scale)
        dragWidthRef.current = next
        setDragWidth(next)
      },
      finish: () => stopActiveDrag(true),
      cancel: () => stopActiveDrag(false),
    })
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
