import { cloneElement, useEffect } from 'react'
import type { CSSProperties, ReactElement } from 'react'

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

  return (
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
}

export default DismissiblePanel
