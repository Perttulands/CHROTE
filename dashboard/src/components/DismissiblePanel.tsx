import { cloneElement, useEffect } from 'react'
import type { CSSProperties, ReactElement } from 'react'
import { createPortal } from 'react-dom'

interface DismissiblePanelProps {
  children: ReactElement<{ style?: CSSProperties }>
  onDismiss: () => void
  panelZIndex?: number
  panelPosition: CSSProperties['position']
}

/**
 * Renders one floating menu above a full-viewport sibling layer.
 * The first outside pointer is intentionally consumed so iframe boundaries cannot bypass dismissal.
 */
function DismissiblePanel({
  children,
  onDismiss,
  panelZIndex = 2000,
  panelPosition,
}: DismissiblePanelProps) {
  useEffect(() => {
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onDismiss()
    }
    document.addEventListener('keydown', closeOnEscape)
    return () => document.removeEventListener('keydown', closeOnEscape)
  }, [onDismiss])

  const panel = cloneElement(children, {
    style: {
      ...children.props.style,
      position: children.props.style?.position ?? panelPosition,
      zIndex: panelZIndex,
    },
  })

  const content = (
    <>
      <div
        className="floating-panel-dismiss-layer"
        aria-hidden="true"
        style={{ zIndex: panelZIndex - 1 }}
        onPointerDown={event => {
          event.preventDefault()
          event.stopPropagation()
          onDismiss()
        }}
      />
      {panel}
    </>
  )

  // Viewport-positioned panels must live at the document overlay root. Keeping
  // them under a pane traps even a large z-index inside that pane's stacking
  // context. Absolute dropdowns intentionally remain locally anchored.
  return panelPosition === 'fixed' && typeof document !== 'undefined'
    ? createPortal(content, document.body)
    : content
}

export default DismissiblePanel
