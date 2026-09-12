/**
 * The frame a floating window sits in: its size, and the gesture that changes
 * it.
 *
 * The window stays centred in the workspace, so an edge or corner under the
 * pointer moves at half the size it adds — the gesture grows the window by
 * twice the pointer's travel, and the edge lands where the pointer is. Arrow
 * keys on a focused handle step the same 16px the docked columns step, and
 * the pointer mechanics are `resizeGesture`'s, shared with them.
 *
 * The caller owns what is inside the window and what size its content asks
 * for; the frame owns the workspace measurement, the clamping, the minimum
 * for the kind, and the remembered size. A caller adopts it by passing its
 * kind, the window element, whether it is open, and a `contentSize` function
 * of the measured workspace — which is what lets a content-derived default
 * (a picture's pixels, a session's grid) be overridden by a remembered size
 * without either side knowing about the other.
 */

import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import type { KeyboardEventHandler, PointerEventHandler, RefObject } from 'react'
import { RESIZE_KEYBOARD_STEP, capturePointerDrag } from './resizeGesture'
import {
  FLOATING_WINDOW_MINIMUM,
  clampFrameSize,
  clearFloatingWindowSize,
  readFloatingWindowSize,
  resolveFrameSize,
  writeFloatingWindowSize,
  type FloatingWindowKind,
  type FrameSize,
} from './floatingWindowSize'

export type FrameHandleId = 'n' | 's' | 'e' | 'w' | 'ne' | 'nw' | 'se' | 'sw'

interface HandleDirection {
  id: FrameHandleId
  /** -1 west, 0 neither, 1 east. */
  x: -1 | 0 | 1
  /** -1 north, 0 neither, 1 south. */
  y: -1 | 0 | 1
  where: string
}

// Edges first, corners after, so a corner wins the overlap at the corner.
const HANDLES: readonly HandleDirection[] = [
  { id: 'n', x: 0, y: -1, where: 'top' },
  { id: 's', x: 0, y: 1, where: 'bottom' },
  { id: 'w', x: -1, y: 0, where: 'left' },
  { id: 'e', x: 1, y: 0, where: 'right' },
  { id: 'nw', x: -1, y: -1, where: 'top left' },
  { id: 'ne', x: 1, y: -1, where: 'top right' },
  { id: 'sw', x: -1, y: 1, where: 'bottom left' },
  { id: 'se', x: 1, y: 1, where: 'bottom right' },
]

export interface FloatingFrameHandleProps {
  onPointerDown: PointerEventHandler<HTMLDivElement>
  onKeyDown: KeyboardEventHandler<HTMLDivElement>
  role: 'separator'
  'aria-label': string
  'aria-orientation'?: 'horizontal' | 'vertical'
  tabIndex: 0
}

export interface FloatingFrameHandle {
  id: FrameHandleId
  props: FloatingFrameHandleProps
}

export interface UseFloatingFrameOptions<T extends HTMLElement> {
  kind: FloatingWindowKind
  /** The window itself. Its parent is the workspace the size is held inside. */
  elementRef: RefObject<T>
  open: boolean
  /** What the window is called in the handles' labels: "image", "session". */
  label: string
  /** The size the content asks for, given the measured workspace. */
  contentSize: (bounds: FrameSize) => FrameSize | null
  minimum?: FrameSize
  /**
   * Told the size a drag or a key step settled on, after it is remembered. A
   * caller whose content has its own idea of size — the image glance's zoom
   * level — reads the size the operator asked for from here.
   */
  onResize?: (size: FrameSize) => void
}

export interface FloatingFrame {
  /** The size to draw, or null while neither memory nor content has said. */
  size: FrameSize | null
  resizing: boolean
  /** The handle in hand, so only that one shows it is being dragged. */
  activeHandle: FrameHandleId | null
  /** True while a remembered size is deciding, so the header can offer the word. */
  remembered: boolean
  /** Forget the remembered size; the content decides again. */
  resetSize: () => void
  handles: readonly FloatingFrameHandle[]
}

export function useFloatingFrame<T extends HTMLElement>({
  kind,
  elementRef,
  open,
  label,
  contentSize,
  minimum = FLOATING_WINDOW_MINIMUM[kind],
  onResize,
}: UseFloatingFrameOptions<T>): FloatingFrame {
  const [bounds, setBounds] = useState<FrameSize | null>(null)
  const [remembered, setRemembered] = useState<FrameSize | null>(null)
  const [dragSize, setDragSize] = useState<FrameSize | null>(null)
  const dragSizeRef = useRef<FrameSize | null>(null)
  const [activeHandle, setActiveHandle] = useState<FrameHandleId | null>(null)
  const cleanupRef = useRef<(() => void) | null>(null)

  // The workspace is measured before the first paint, so the window is never
  // drawn at a size the operator did not ask for and then corrected.
  useLayoutEffect(() => {
    if (!open) {
      setBounds(null)
      return
    }
    const measure = () => {
      const workspace = elementRef.current?.parentElement
      if (!workspace) return
      setBounds({ width: workspace.clientWidth, height: workspace.clientHeight })
    }
    measure()
    window.addEventListener('resize', measure)
    return () => window.removeEventListener('resize', measure)
  }, [elementRef, open])

  // The memory is read when the window opens, so a size dragged in one tab is
  // in force for the next window opened in another.
  useEffect(() => {
    if (!open) return
    setRemembered(readFloatingWindowSize(kind))
  }, [kind, open])

  useEffect(() => {
    if (open) return
    setDragSize(null)
    setActiveHandle(null)
    dragSizeRef.current = null
    cleanupRef.current?.()
    cleanupRef.current = null
  }, [open])

  useEffect(() => () => {
    cleanupRef.current?.()
    cleanupRef.current = null
  }, [])

  // Kept in a ref so the caller may hand a fresh closure on every render
  // without every handle's props being rebuilt.
  const onResizeRef = useRef(onResize)
  onResizeRef.current = onResize

  const commit = useCallback((size: FrameSize) => {
    const held = clampFrameSize(size, minimum, bounds)
    writeFloatingWindowSize(kind, held)
    setRemembered(held)
    onResizeRef.current?.(held)
  }, [bounds, kind, minimum])

  const stopActiveDrag = useCallback((keep: boolean) => {
    const dragged = dragSizeRef.current
    cleanupRef.current?.()
    cleanupRef.current = null
    dragSizeRef.current = null
    setDragSize(null)
    setActiveHandle(null)
    if (keep && dragged) commit(dragged)
  }, [commit])

  const resetSize = useCallback(() => {
    clearFloatingWindowSize(kind)
    setRemembered(null)
  }, [kind])

  const settled = resolveFrameSize({ remembered, content: bounds ? contentSize(bounds) : null, minimum, bounds })
  const size = dragSize ?? settled

  const handleProps = useCallback((direction: HandleDirection): FloatingFrameHandleProps => ({
    role: 'separator',
    tabIndex: 0,
    'aria-label': `Resize the ${label} from the ${direction.where}`,
    ...(direction.x === 0 ? { 'aria-orientation': 'horizontal' as const } : {}),
    ...(direction.y === 0 ? { 'aria-orientation': 'vertical' as const } : {}),
    onPointerDown: event => {
      if (event.button !== 0) return
      const element = elementRef.current
      if (!element) return
      event.preventDefault()
      stopActiveDrag(false)

      const handle = event.currentTarget
      const grabbedAt = { x: event.clientX, y: event.clientY }
      const box = element.getBoundingClientRect()
      const grabbed = { width: box.width, height: box.height }
      setActiveHandle(direction.id)

      cleanupRef.current = capturePointerDrag(handle, event.pointerId, {
        move: moveEvent => {
          // Centred: the window grows by twice the travel, so the edge in hand
          // lands under the pointer.
          const next = clampFrameSize({
            width: grabbed.width + 2 * direction.x * (moveEvent.clientX - grabbedAt.x),
            height: grabbed.height + 2 * direction.y * (moveEvent.clientY - grabbedAt.y),
          }, minimum, bounds)
          dragSizeRef.current = next
          setDragSize(next)
        },
        finish: () => stopActiveDrag(true),
        cancel: () => stopActiveDrag(false),
      })
    },
    onKeyDown: event => {
      const horizontal = event.key === 'ArrowRight' ? 1 : event.key === 'ArrowLeft' ? -1 : 0
      const vertical = event.key === 'ArrowDown' ? 1 : event.key === 'ArrowUp' ? -1 : 0
      const step = {
        width: direction.x * horizontal * RESIZE_KEYBOARD_STEP,
        height: direction.y * vertical * RESIZE_KEYBOARD_STEP,
      }
      if (step.width === 0 && step.height === 0) return
      const from = dragSizeRef.current ?? size
      if (!from) return
      event.preventDefault()
      commit({ width: from.width + step.width, height: from.height + step.height })
    },
  }), [bounds, commit, elementRef, label, minimum, size, stopActiveDrag])

  return {
    size,
    resizing: dragSize !== null,
    activeHandle,
    remembered: remembered !== null,
    resetSize,
    handles: HANDLES.map(direction => ({ id: direction.id, props: handleProps(direction) })),
  }
}
