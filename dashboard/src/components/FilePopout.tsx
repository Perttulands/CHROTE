/**
 * The Files pop-out: the open file, hung off the panel's right edge.
 *
 * It is open exactly while the panel has an open path, so the tree stays in
 * view and the next click in the tree retargets this window rather than
 * replacing anything. Its left edge is `--terminal-files-width`, inherited
 * from the panel it is drawn inside and the one variable that draws the
 * panel's own width, so the edge follows a drag of the panel by construction:
 * nothing here measures, observes or polls.
 *
 * It is a window of the shared floating frame's `file` kind, and the only
 * handles it offers are the ones that can move — right, bottom, and the
 * corner between them — because the left edge belongs to the panel. Until the
 * operator drags it, it is a readable column beside the tree; after that the
 * remembered size decides for every later file, and Reset size gives the
 * default back.
 *
 * Dismissal follows the panel it hangs off. Pinned, it is a work surface:
 * Escape closes the file and leaves the tree. Overlaid over the terminals it
 * is part of the glance the panel already is, so a press on a terminal takes
 * the file and the sidecar away together, and a press anywhere in the panel
 * or the window is an ordinary press.
 */

import { useEffect, useRef } from 'react'
import type { RefObject } from 'react'
import FilePanelViewer from './FilePanelViewer'
import FloatingFrameHandles from './FloatingFrameHandles'
import { getFileBaseName } from './FileViewer'
import { useFloatingFrame } from '../hooks/useFloatingFrame'
import { registerSurface } from '../keys/dismiss'

/** A readable column of text, until the operator says otherwise. */
export const FILE_POPOUT_DEFAULT_WIDTH = 560
/** The workspace header the overlaid panel already starts below. */
export const FILE_POPOUT_TOP_PX = 45

interface FilePopoutProps {
  path: string
  /** Pinned into the rail, rather than overlaid over the terminals. */
  pinned: boolean
  /** The panel this window hangs off: what a press outside is measured from. */
  panelRef: RefObject<HTMLElement | null>
  /** Put the file away and leave the tree where it is. */
  onClose: () => void
  /** Put the file and the overlaid sidecar away together. */
  onDismiss: () => void
  onOpenPath: (path: string) => void
  onSend: ((path: string) => void) | null
}

function FilePopout({ path, pinned, panelRef, onClose, onDismiss, onOpenPath, onSend }: FilePopoutProps) {
  const popoutRef = useRef<HTMLDivElement>(null)

  const frame = useFloatingFrame({
    kind: 'file',
    elementRef: popoutRef,
    // Drawn inside the panel, held by the workspace the panel sits in.
    boundsElement: () => panelRef.current?.parentElement ?? null,
    open: true,
    label: 'file window',
    anchor: 'edge',
    handleIds: ['e', 's', 'se'],
    contentSize: bounds => ({
      width: FILE_POPOUT_DEFAULT_WIDTH,
      height: Math.max(1, bounds.height - FILE_POPOUT_TOP_PX),
    }),
  })

  // Registered once per opening, with the kind the panel has: the same rule
  // the panel's own dismissal follows.
  const closeRef = useRef(onClose)
  closeRef.current = onClose
  const dismissRef = useRef(onDismiss)
  dismissRef.current = onDismiss
  const pinnedRef = useRef(pinned)
  pinnedRef.current = pinned
  useEffect(() => registerSurface({
    kind: pinned ? 'work' : 'glance',
    close: () => (pinnedRef.current ? closeRef.current() : dismissRef.current()),
    contains: target => Boolean(panelRef.current?.contains(target)),
  }), [panelRef, pinned])

  return (
    <div
      ref={popoutRef}
      className="terminal-file-popout"
      data-ui="files.popout"
      role="dialog"
      aria-label={`File ${getFileBaseName(path)}`}
      style={frame.size ? { width: frame.size.width, height: frame.size.height } : undefined}
    >
      <FloatingFrameHandles frame={frame} />
      <FilePanelViewer
        path={path}
        onClose={onClose}
        onOpenPath={onOpenPath}
        onSend={onSend}
        pictureOpensGlance={false}
        onResetSize={frame.remembered ? frame.resetSize : null}
      />
    </div>
  )
}

export default FilePopout
