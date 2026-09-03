/**
 * One owner of dismissal.
 *
 * Every floating surface registers here while it is open, and the owner keeps
 * them as a stack: the last one opened is the one a dismissal reaches. There
 * are two classes. A glance goes away when the operator looks elsewhere: a
 * press outside it closes it, and that press is consumed rather than passed
 * through to whatever it landed on. A work surface stays until it is closed:
 * a press outside it is an ordinary press. Escape closes the topmost surface
 * of either class, and reaches the pty only when nothing is open.
 *
 * Escape is decided in two places that agree by construction. The terminal's
 * own key handler refuses it while the stack is not empty, so xterm neither
 * writes it nor sends it; and a document listener at the bubble phase then
 * closes the topmost surface, unless a control inside the surface already
 * handled the key for its own purpose — a find field clearing its query, an
 * editor asking whether to discard — which is what `defaultPrevented` and a
 * stopped propagation say. The press outside is taken at the capture phase,
 * because it has to be gone before the app, the terminal and the drag sensor
 * see it.
 *
 * The state lives at module level, like the chord registry, because the
 * terminal is built outside React and asks from there.
 */

import { useEffect, useRef, type RefObject } from 'react'

export type SurfaceKind = 'glance' | 'work'

export interface Surface {
  kind: SurfaceKind
  /** Close this surface. Called by the owner, on the operator's dismissal. */
  close: () => void
  /** Whether a pointer target is inside the surface. */
  contains: (target: Node) => boolean
}

const stack: Surface[] = []

/** Put a surface on top of the stack; the returned function takes it off. */
export function registerSurface(surface: Surface): () => void {
  stack.push(surface)
  return () => {
    const index = stack.lastIndexOf(surface)
    if (index !== -1) stack.splice(index, 1)
  }
}

/** The surface a dismissal would reach, or null while nothing is open. */
export function topSurface(): Surface | null {
  return stack[stack.length - 1] ?? null
}

/**
 * xterm's `attachCustomKeyEventHandler` asks this: true means the key belongs
 * to a surface and must not reach the pty. Every event type is refused, so a
 * keyup cannot make the terminal act on a keydown it never saw.
 */
export function ownsKey(event: KeyboardEvent): boolean {
  return event.key === 'Escape' && stack.length > 0
}

/**
 * Offer a keydown to the owner: true means it closed the topmost surface. A
 * key a control inside the surface already claimed is left alone.
 */
export function dismissKeyEvent(event: KeyboardEvent): boolean {
  if (event.type !== 'keydown' || event.key !== 'Escape' || event.defaultPrevented) return false
  const top = topSurface()
  if (!top) return false
  top.close()
  return true
}

// The press that closed a glance is consumed whole: the mousedown that would
// focus a terminal, the click that would select a tile, the contextmenu that
// would open a menu. A pointerdown that is cancelled already suppresses the
// compatibility mousedown and mouseup; the rest are stopped here until the
// click that ends the press, or the next press. A click with no press behind
// it — Enter on a button, or a scripted `.click()` — carries `detail` 0 and is
// never the one being consumed.
let consuming = false

function consume(event: Event) {
  event.preventDefault()
  event.stopImmediatePropagation()
}

/** Offer a pointerdown to the owner: true means it closed a glance and took the press. */
export function dismissPointerDown(event: PointerEvent): boolean {
  consuming = false
  const top = topSurface()
  if (!top || top.kind !== 'glance') return false
  if (event.target instanceof Node && top.contains(event.target)) return false
  consuming = true
  top.close()
  return true
}

if (typeof document !== 'undefined') {
  document.addEventListener('keydown', event => {
    if (dismissKeyEvent(event)) event.preventDefault()
  })
  document.addEventListener('pointerdown', event => {
    if (dismissPointerDown(event)) consume(event)
  }, true)
  for (const type of ['mousedown', 'mouseup', 'contextmenu'] as const) {
    document.addEventListener(type, event => {
      if (consuming) consume(event)
    }, true)
  }
  for (const type of ['click', 'auxclick'] as const) {
    document.addEventListener(type, event => {
      if (!consuming || event.detail === 0) return
      consuming = false
      consume(event)
    }, true)
  }
}

export interface UseSurfaceOptions {
  open: boolean
  kind: SurfaceKind
  onClose: () => void
  /** The surface's own element, for the press outside. */
  ref: RefObject<HTMLElement | null>
}

/**
 * Register a React surface for as long as it is open. The registration is made
 * once per opening, so a re-render never moves the surface within the stack;
 * a change of class is applied in place.
 */
export function useSurface({ open, kind, onClose, ref }: UseSurfaceOptions): void {
  const onCloseRef = useRef(onClose)
  onCloseRef.current = onClose
  const surfaceRef = useRef<Surface | null>(null)

  useEffect(() => {
    if (!open) return
    const surface: Surface = {
      kind,
      close: () => onCloseRef.current(),
      contains: target => ref.current?.contains(target) ?? false,
    }
    surfaceRef.current = surface
    const retire = registerSurface(surface)
    return () => {
      retire()
      surfaceRef.current = null
    }
    // The class is applied below without re-registering.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, ref])

  useEffect(() => {
    if (surfaceRef.current) surfaceRef.current.kind = kind
  }, [kind])
}

/** Test seam: forget every open surface. */
export function resetSurfacesForTest(): void {
  stack.length = 0
  consuming = false
}
