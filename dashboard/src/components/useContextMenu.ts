/**
 * The two gestures that open an object's menu: the pointer's right-click, and
 * a finger held on the object for half a second. One hook so every row and
 * header in CHROTE answers them the same way, and what opens is the one Menu.
 *
 * A finger that moves or lifts before the press matures was scrolling or
 * tapping, and nothing opens. A part of the object that owns its own gesture
 * — a tag inside a header — is left alone by the selector that names it.
 */

import { useCallback, useEffect, useRef, useState } from 'react'
import type { MouseEvent as ReactMouseEvent, TouchEvent as ReactTouchEvent } from 'react'

/** How long a finger rests on an object before it is asking for the menu. */
export const LONG_PRESS_MS = 500

export interface MenuAnchor {
  x: number
  y: number
}

export interface MenuTriggerProps {
  onContextMenu: (event: ReactMouseEvent) => void
  onTouchStart: (event: ReactTouchEvent) => void
  onTouchEnd: () => void
  onTouchMove: () => void
  onTouchCancel: () => void
}

export interface ContextMenuOptions {
  /** A selector for the parts of the object whose gestures belong to themselves. */
  ignore?: string
  /** False while the object has no menu to open; the browser keeps the gesture. */
  enabled?: boolean
}

export function useContextMenu({ ignore, enabled = true }: ContextMenuOptions = {}) {
  const [anchor, setAnchor] = useState<MenuAnchor | null>(null)
  const press = useRef<ReturnType<typeof setTimeout> | null>(null)

  const cancelPress = useCallback(() => {
    if (press.current === null) return
    clearTimeout(press.current)
    press.current = null
  }, [])

  useEffect(() => cancelPress, [cancelPress])

  const close = useCallback(() => setAnchor(null), [])

  const claims = useCallback((target: EventTarget | null) => (
    enabled && !(ignore !== undefined && target instanceof Element && target.closest(ignore) !== null)
  ), [enabled, ignore])

  const onContextMenu = useCallback((event: ReactMouseEvent) => {
    if (!claims(event.target)) return
    event.preventDefault()
    event.stopPropagation()
    setAnchor({ x: event.clientX, y: event.clientY })
  }, [claims])

  const onTouchStart = useCallback((event: ReactTouchEvent) => {
    cancelPress()
    const touch = event.touches[0]
    if (!touch || !claims(event.target)) return
    const at = { x: touch.clientX, y: touch.clientY }
    press.current = setTimeout(() => {
      press.current = null
      setAnchor(at)
    }, LONG_PRESS_MS)
  }, [cancelPress, claims])

  const triggerProps: MenuTriggerProps = {
    onContextMenu,
    onTouchStart,
    onTouchEnd: cancelPress,
    onTouchMove: cancelPress,
    onTouchCancel: cancelPress,
  }

  return { anchor, close, triggerProps }
}
