import { useLayoutEffect, useRef, useState } from 'react'
import type { CSSProperties, RefObject } from 'react'

export interface MenuAnchorPoint {
  x: number
  y: number
}

export interface MenuSize {
  width: number
  height: number
}

export interface MenuPosition {
  left: number
  top: number
}

export interface PositionMenuInViewportOptions {
  viewportWidth?: number
  viewportHeight?: number
  margin?: number
}

interface UseViewportMenuPositionOptions extends PositionMenuInViewportOptions {
  estimatedSize?: Partial<MenuSize>
}

const DEFAULT_MARGIN = 8

function finiteNumber(value: number | undefined, fallback: number): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : fallback
}

function nonNegative(value: number | undefined, fallback = 0): number {
  return Math.max(0, finiteNumber(value, fallback))
}

function getViewportWidth(override?: number): number {
  if (override !== undefined) return nonNegative(override)
  if (typeof window === 'undefined') return 0
  return nonNegative(window.innerWidth)
}

function getViewportHeight(override?: number): number {
  if (override !== undefined) return nonNegative(override)
  if (typeof window === 'undefined') return 0
  return nonNegative(window.innerHeight)
}

function getMargin(margin?: number): number {
  return nonNegative(margin, DEFAULT_MARGIN)
}

export function positionMenuInViewport(
  anchor: MenuAnchorPoint,
  menuSize: MenuSize,
  options: PositionMenuInViewportOptions = {},
): MenuPosition {
  const margin = getMargin(options.margin)
  const viewportWidth = getViewportWidth(options.viewportWidth)
  const viewportHeight = getViewportHeight(options.viewportHeight)
  const menuWidth = nonNegative(menuSize.width)
  const menuHeight = nonNegative(menuSize.height)
  const anchorX = finiteNumber(anchor.x, margin)
  const anchorY = finiteNumber(anchor.y, margin)

  const minLeft = margin
  const minTop = margin
  const maxLeft = Math.max(minLeft, viewportWidth - margin - menuWidth)
  const maxTop = Math.max(minTop, viewportHeight - margin - menuHeight)

  return {
    left: Math.min(Math.max(anchorX, minLeft), maxLeft),
    top: Math.min(Math.max(anchorY, minTop), maxTop),
  }
}

function samePosition(a: MenuPosition, b: MenuPosition): boolean {
  return a.left === b.left && a.top === b.top
}

export function useViewportMenuPosition<T extends HTMLElement>(
  anchor: MenuAnchorPoint | null | undefined,
  options: UseViewportMenuPositionOptions = {},
): { ref: RefObject<T>; style: CSSProperties } {
  const ref = useRef<T>(null)
  const estimatedWidth = nonNegative(options.estimatedSize?.width)
  const estimatedHeight = nonNegative(options.estimatedSize?.height)
  const margin = getMargin(options.margin)
  const viewportWidthOverride = options.viewportWidth
  const viewportHeightOverride = options.viewportHeight
  const anchorX = anchor?.x
  const anchorY = anchor?.y
  const [position, setPosition] = useState<MenuPosition>(() => (
    anchor
      ? positionMenuInViewport(
        anchor,
        { width: estimatedWidth, height: estimatedHeight },
        { viewportWidth: viewportWidthOverride, viewportHeight: viewportHeightOverride, margin },
      )
      : { left: 0, top: 0 }
  ))

  useLayoutEffect(() => {
    if (anchorX === undefined || anchorY === undefined) return undefined
    const anchorPoint = { x: anchorX, y: anchorY }

    const updatePosition = () => {
      const rect = ref.current?.getBoundingClientRect()
      const measuredWidth = rect && rect.width > 0 ? rect.width : estimatedWidth
      const measuredHeight = rect && rect.height > 0 ? rect.height : estimatedHeight
      const nextPosition = positionMenuInViewport(
        anchorPoint,
        { width: measuredWidth, height: measuredHeight },
        { viewportWidth: viewportWidthOverride, viewportHeight: viewportHeightOverride, margin },
      )

      setPosition(previous => (samePosition(previous, nextPosition) ? previous : nextPosition))
    }

    updatePosition()
    window.addEventListener('resize', updatePosition)
    return () => window.removeEventListener('resize', updatePosition)
  }, [anchorX, anchorY, estimatedWidth, estimatedHeight, margin, viewportWidthOverride, viewportHeightOverride])

  return {
    ref,
    style: {
      position: 'fixed',
      left: position.left,
      top: position.top,
      boxSizing: 'border-box',
    },
  }
}
