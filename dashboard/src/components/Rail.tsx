/**
 * The shared left rail used by the Beads, Library, and Agents tabs.
 *
 * `Rail` owns the column shell and its right-edge resize handle. `RailSection`
 * supplies the common heading treatment, and `RailScroll` supplies the one
 * bounded scrolling region inside a section. Callers own their rows, current
 * width, and persistence key. The shell keeps 480px for the content beside it.
 */

import { useCallback, useRef } from 'react'
import type {
  ComponentPropsWithoutRef,
  ReactNode,
} from 'react'
import { useResizableWidth } from '../hooks/useResizableWidth'
import './Rail.css'

export interface RailProps extends Omit<ComponentPropsWithoutRef<'aside'>, 'children'> {
  label: string
  width: number
  minWidth?: number
  contentMinWidth?: number
  onWidthCommit: (width: number) => void
  children: ReactNode
}

export interface RailSectionProps extends Omit<ComponentPropsWithoutRef<'section'>, 'title'> {
  title?: ReactNode
  fill?: boolean
}

export type RailScrollProps = ComponentPropsWithoutRef<'div'>

export default function Rail({
  label,
  width,
  minWidth = 120,
  contentMinWidth = 480,
  onWidthCommit,
  className,
  style,
  children,
  ...asideProps
}: RailProps) {
  const railRef = useRef<HTMLElement>(null)
  const widest = useCallback(() => {
    const room = railRef.current?.parentElement?.clientWidth || Number.POSITIVE_INFINITY
    return Math.max(minWidth, room - contentMinWidth)
  }, [contentMinWidth, minWidth])
  const { width: renderedWidth, resizing, handleProps } = useResizableWidth({
    elementRef: railRef,
    width,
    minWidth,
    maxWidth: widest,
    edge: 'right',
    onCommit: onWidthCommit,
  })

  const maximum = widest()

  return (
    <aside
      {...asideProps}
      ref={railRef}
      className={['rail', className].filter(Boolean).join(' ')}
      aria-label={asideProps['aria-label'] ?? label}
      style={{ ...style, width: renderedWidth }}
    >
      {children}
      <div
        {...handleProps}
        className={`rail-resize-handle${resizing ? ' dragging' : ''}`}
        role="separator"
        aria-orientation="vertical"
        aria-label={`Resize ${label}`}
        aria-valuenow={Math.round(renderedWidth)}
        aria-valuemin={Math.round(minWidth)}
        aria-valuemax={Number.isFinite(maximum) ? Math.round(maximum) : undefined}
        tabIndex={0}
      />
    </aside>
  )
}

export function RailSection({ title, fill = false, className, style, children, ...sectionProps }: RailSectionProps) {
  return (
    <section
      {...sectionProps}
      className={['rail-section', fill ? 'fill' : '', className].filter(Boolean).join(' ')}
      style={style}
    >
      {title !== undefined && <h3 className="rail-section-heading">{title}</h3>}
      {children}
    </section>
  )
}

export function RailScroll({ className, style, ...scrollProps }: RailScrollProps) {
  return (
    <div
      {...scrollProps}
      className={['rail-scroll', className].filter(Boolean).join(' ')}
      style={style}
    />
  )
}
