/**
 * The shared left rail used by the Beads, Library, and Agents tabs.
 *
 * `Rail` owns the column shell and its right-edge resize handle. `RailSection`
 * supplies the common heading treatment, and `RailScroll` supplies the one
 * bounded scrolling region inside a section. Callers own their rows, current
 * width, and persistence key. The shell keeps 480px for the content beside it.
 */

import { useCallback, useRef, useState } from 'react'
import type {
  ComponentPropsWithoutRef,
  CSSProperties,
  ReactNode,
} from 'react'
import { useResizableWidth } from '../hooks/useResizableWidth'

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

const railBase: CSSProperties = {
  position: 'relative',
  display: 'flex',
  flex: 'none',
  flexDirection: 'column',
  gap: 12,
  minWidth: 0,
  minHeight: 0,
  padding: 8,
  overflow: 'hidden',
  borderRight: '1px solid var(--divider)',
  backgroundColor: 'var(--surface-primary)',
  color: 'var(--text-primary)',
  fontSize: 13,
}

const handleBase: CSSProperties = {
  position: 'absolute',
  zIndex: 1,
  top: 0,
  right: 0,
  bottom: 0,
  width: 4,
  cursor: 'col-resize',
  touchAction: 'none',
}

const sectionBase: CSSProperties = {
  display: 'flex',
  flex: 'none',
  flexDirection: 'column',
  minWidth: 0,
  minHeight: 0,
  overflow: 'hidden',
}

const headingStyle: CSSProperties = {
  flex: 'none',
  margin: '0 0 6px',
  padding: '0 6px',
  color: 'var(--text-dim)',
  fontSize: 12,
  fontWeight: 400,
  letterSpacing: '0.08em',
  lineHeight: 1.4,
  textTransform: 'uppercase',
}

const scrollBase: CSSProperties = {
  display: 'flex',
  flex: '1 1 auto',
  flexDirection: 'column',
  minWidth: 0,
  minHeight: 0,
  overflowY: 'auto',
}

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
  const [handleHovered, setHandleHovered] = useState(false)
  const [handleFocused, setHandleFocused] = useState(false)
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
  const showHover = useCallback(() => setHandleHovered(true), [])
  const hideHover = useCallback(() => setHandleHovered(false), [])
  const showFocus = useCallback(() => setHandleFocused(true), [])
  const hideFocus = useCallback(() => setHandleFocused(false), [])

  return (
    <aside
      {...asideProps}
      ref={railRef}
      className={['rail', className].filter(Boolean).join(' ')}
      aria-label={asideProps['aria-label'] ?? label}
      style={{ ...railBase, ...style, width: renderedWidth }}
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
        onPointerEnter={showHover}
        onPointerLeave={hideHover}
        onFocus={showFocus}
        onBlur={hideFocus}
        style={{
          ...handleBase,
          backgroundColor: resizing || handleFocused
            ? 'var(--accent)'
            : handleHovered ? 'var(--text-dim)' : undefined,
        }}
      />
    </aside>
  )
}

export function RailSection({ title, fill = false, className, style, children, ...sectionProps }: RailSectionProps) {
  return (
    <section
      {...sectionProps}
      className={['rail-section', className].filter(Boolean).join(' ')}
      style={{ ...sectionBase, ...(fill ? { flex: '1 1 0' } : null), ...style }}
    >
      {title !== undefined && <h3 style={headingStyle}>{title}</h3>}
      {children}
    </section>
  )
}

export function RailScroll({ className, style, ...scrollProps }: RailScrollProps) {
  return (
    <div
      {...scrollProps}
      className={['rail-scroll', className].filter(Boolean).join(' ')}
      style={{ ...scrollBase, ...style }}
    />
  )
}
