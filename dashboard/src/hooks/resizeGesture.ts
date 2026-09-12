/**
 * The mechanics every resize gesture shares.
 *
 * One place owns pointer capture, the listener wiring a captured drag needs,
 * the 16px keyboard step, and clamping a length against limits a caller may
 * have supplied reversed or non-finite. The one-axis column gesture
 * (`useResizableWidth`) and the two-axis floating-window frame
 * (`useFloatingFrame`) both sit on this, so the drag math exists once.
 */

export const RESIZE_KEYBOARD_STEP = 16

export function finiteOr(value: number, fallback: number): number {
  return Number.isFinite(value) ? value : fallback
}

/** Clamp a length even when a caller supplies reversed or non-finite limits. */
export function clampLength(length: number, min: number, max: number): number {
  const minimum = Math.max(0, finiteOr(min, 0))
  const maximum = Math.max(minimum, finiteOr(max, Number.POSITIVE_INFINITY))
  return Math.min(maximum, Math.max(minimum, finiteOr(length, minimum)))
}

export interface PointerDragHandlers {
  move: (event: PointerEvent) => void
  /** The pointer was released: the drag's last value is the operator's. */
  finish: () => void
  /** The gesture was taken away: nothing is committed. */
  cancel: () => void
}

/**
 * Capture `pointerId` on `handle` and route its move, up and cancel events to
 * the caller. The returned function removes the listeners and releases the
 * capture; call it exactly once, whatever ended the drag.
 */
export function capturePointerDrag(
  handle: HTMLElement,
  pointerId: number,
  handlers: PointerDragHandlers,
): () => void {
  const move = (event: PointerEvent) => {
    if (event.pointerId === pointerId) handlers.move(event)
  }
  const finish = (event: PointerEvent) => {
    if (event.pointerId === pointerId) handlers.finish()
  }
  const cancel = (event: PointerEvent) => {
    if (event.pointerId === pointerId) handlers.cancel()
  }

  const cleanup = () => {
    handle.removeEventListener('pointermove', move)
    handle.removeEventListener('pointerup', finish)
    handle.removeEventListener('pointercancel', cancel)
    if (handle.hasPointerCapture?.(pointerId)) handle.releasePointerCapture(pointerId)
  }

  handle.setPointerCapture(pointerId)
  handle.addEventListener('pointermove', move)
  handle.addEventListener('pointerup', finish)
  handle.addEventListener('pointercancel', cancel)
  return cleanup
}
