/**
 * The resize handles of a floating window: four edges and four corners, drawn
 * as the docked columns draw theirs — nothing at all until the pointer is on
 * one or the keyboard has focused it.
 *
 * A window adopts the frame by rendering this inside itself with the frame
 * `useFloatingFrame` returned.
 */

import type { FloatingFrame } from '../hooks/useFloatingFrame'
import './FloatingFrameHandles.css'

interface FloatingFrameHandlesProps {
  frame: FloatingFrame
}

function FloatingFrameHandles({ frame }: FloatingFrameHandlesProps) {
  return (
    <>
      {frame.handles.map(handle => (
        <div
          key={handle.id}
          {...handle.props}
          className={`floating-frame-handle${frame.activeHandle === handle.id ? ' dragging' : ''}`}
          data-handle={handle.id}
        />
      ))}
    </>
  )
}

export default FloatingFrameHandles
