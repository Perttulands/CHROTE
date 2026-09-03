import { cloneElement, useRef } from 'react'
import type { CSSProperties, ReactElement } from 'react'
import { createPortal } from 'react-dom'
import { useSurface, type SurfaceKind } from '../keys/dismiss'

interface DismissiblePanelProps {
  children: ReactElement<{ style?: CSSProperties }>
  onDismiss: () => void
  panelZIndex?: number
  panelPosition: CSSProperties['position']
  /** A glance unless said otherwise: a press outside closes it and is consumed. */
  kind?: SurfaceKind
}

/**
 * Renders one floating panel and hands its dismissal to the owner in
 * keys/dismiss: Escape closes it, and, as a glance, so does a press outside,
 * which is consumed so that a nested surface cannot bypass the dismissal.
 */
function DismissiblePanel({
  children,
  onDismiss,
  panelZIndex = 2000,
  panelPosition,
  kind = 'glance',
}: DismissiblePanelProps) {
  // The holder generates no box of its own, so the panel keeps whatever
  // containing block it had; it only gives the owner an element to hit-test.
  const holderRef = useRef<HTMLDivElement>(null)
  useSurface({ open: true, kind, onClose: onDismiss, ref: holderRef })

  const panel = cloneElement(children, {
    style: {
      ...children.props.style,
      position: children.props.style?.position ?? panelPosition,
      zIndex: panelZIndex,
    },
  })

  const content = <div ref={holderRef} style={{ display: 'contents' }}>{panel}</div>

  // Viewport-positioned panels must live at the document overlay root. Keeping
  // them under a pane traps even a large z-index inside that pane's stacking
  // context. Absolute dropdowns intentionally remain locally anchored.
  return panelPosition === 'fixed' && typeof document !== 'undefined'
    ? createPortal(content, document.body)
    : content
}

export default DismissiblePanel
