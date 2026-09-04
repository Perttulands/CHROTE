/**
 * How big a drawing's box actually is.
 *
 * A layout in its own coordinates still has to be fitted to the room it was
 * given, and in jsdom there is no room at all, so a fallback stands in. One
 * hook, because the Library map and the Beads flow both need the same fact and
 * a second copy of it would drift.
 */

import { useLayoutEffect, useRef, useState } from 'react'

export interface MeasuredSize {
  width: number
  height: number
}

/** What a layout is given before the box has been measured, and in jsdom. */
export const FALLBACK_SIZE: MeasuredSize = { width: 960, height: 600 }

export function useMeasuredSize<T extends HTMLElement = HTMLDivElement>(fallback: MeasuredSize = FALLBACK_SIZE) {
  const ref = useRef<T>(null)
  const [size, setSize] = useState<MeasuredSize>(fallback)

  useLayoutEffect(() => {
    const element = ref.current
    if (!element) return
    const read = () => {
      const box = element.getBoundingClientRect()
      if (box.width <= 0 || box.height <= 0) return
      const next = { width: Math.round(box.width), height: Math.round(box.height) }
      setSize(current => (current.width === next.width && current.height === next.height ? current : next))
    }
    read()
    if (typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(read)
    observer.observe(element)
    return () => observer.disconnect()
  }, [])

  return { ref, width: size.width, height: size.height }
}
