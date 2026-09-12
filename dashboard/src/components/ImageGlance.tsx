/**
 * The image glance: a look at a picture an agent named, in Peek's manner.
 *
 * A centred floating window over the workspace with a one-line header — the
 * path, the pixel size, and Open in Files, Copy path and Close as words — and
 * the image beneath on the terminal background. Its default size is the
 * image's: never larger than the picture is, never more than 90% of the
 * workspace, and never upscaled. The operator resizes it from any edge or
 * corner through the shared floating-window frame, and that size is
 * remembered for every later picture until Reset size gives the picture the
 * say again. It is a glance, so a press outside closes it and is consumed,
 * and Escape closes it from anywhere. It loads the raw route the file viewers
 * already load; there is no route of its own.
 */

import { useEffect, useRef, useState } from 'react'
import PanelPath from './PanelPath'
import FloatingFrameHandles from './FloatingFrameHandles'
import { describeReadFailure, getDownloadUrl } from './FilesView/fileService'
import { getFileBaseName } from './FileViewer'
import { closeImageGlance, useImageGlanceRequest } from './imageGlance'
import { openInFiles } from '../terminal/openInFiles'
import { useSurface } from '../keys/dismiss'
import { useFloatingFrame } from '../hooks/useFloatingFrame'
import type { FrameSize } from '../hooks/floatingWindowSize'
import { useStatus } from '../context/StatusContext'
import { copyTextToClipboard } from '../utils/clipboard'
import './ImageGlance.css'

/** The most of the workspace the glance takes, in each direction. */
export const IMAGE_GLANCE_MAX_SHARE = 0.9
/** The header's fixed height, as ImageGlance.css draws it. */
export const IMAGE_GLANCE_HEADER_PX = 30
/** The hairline around the window. */
const IMAGE_GLANCE_BORDER_PX = 2
/** What the window shows while the image is on its way, or when it never comes. */
const IMAGE_GLANCE_EMPTY = { width: 480, height: 160 }

export interface ImageGlanceSize {
  width: number
  height: number
  /** The image as drawn, at 1:1 or scaled down to fit. */
  image: { width: number; height: number }
}

/** The room the picture has inside a window of the given size. */
function imageRoom(frame: FrameSize): FrameSize {
  return {
    width: frame.width - IMAGE_GLANCE_BORDER_PX,
    height: frame.height - IMAGE_GLANCE_BORDER_PX - IMAGE_GLANCE_HEADER_PX,
  }
}

/** The picture at 1:1 when it fits the room, scaled down at its own ratio when
 * it does not, and never up. */
export function fitImage(natural: FrameSize, room: FrameSize): FrameSize {
  const scale = Math.min(1, room.width / natural.width, room.height / natural.height)
  return {
    width: Math.max(1, Math.floor(natural.width * scale)),
    height: Math.max(1, Math.floor(natural.height * scale)),
  }
}

/**
 * The window's default size for an image of the given pixels: the image at
 * 1:1 when it fits inside 90% of the workspace, scaled down at its own ratio
 * when it does not, and the header and hairline around it.
 */
export function imageGlanceSize(
  natural: { width: number; height: number },
  workspace: { width: number; height: number },
): ImageGlanceSize {
  const image = fitImage(natural, imageRoom({
    width: Math.floor(workspace.width * IMAGE_GLANCE_MAX_SHARE),
    height: Math.floor(workspace.height * IMAGE_GLANCE_MAX_SHARE),
  }))
  return {
    width: image.width + IMAGE_GLANCE_BORDER_PX,
    height: image.height + IMAGE_GLANCE_BORDER_PX + IMAGE_GLANCE_HEADER_PX,
    image,
  }
}

type Picture =
  | { state: 'loading' }
  | { state: 'shown'; natural: { width: number; height: number } }
  | { state: 'failed'; reason: string | null }

function ImageGlance() {
  const request = useImageGlanceRequest()
  const { announce } = useStatus()
  const glanceRef = useRef<HTMLDivElement>(null)
  const [picture, setPicture] = useState<Picture>({ state: 'loading' })
  const nonceRef = useRef(request?.nonce)
  nonceRef.current = request?.nonce

  useSurface({ open: request !== null, kind: 'glance', onClose: closeImageGlance, ref: glanceRef })

  // The picture asks for its own size; a size the operator dragged overrides
  // it, for this picture and every later one.
  const frame = useFloatingFrame({
    kind: 'image',
    elementRef: glanceRef,
    open: request !== null,
    label: 'image',
    contentSize: workspace => (
      picture.state === 'shown' ? imageGlanceSize(picture.natural, workspace) : null
    ),
  })

  // A new request is a new picture: what the last one measured says nothing
  // about this one.
  useEffect(() => {
    setPicture({ state: 'loading' })
  }, [request?.nonce])

  if (!request) return null

  const { path } = request
  const name = getFileBaseName(path)
  const windowSize = frame.size ?? IMAGE_GLANCE_EMPTY
  const drawn = frame.size && picture.state === 'shown'
    ? fitImage(picture.natural, imageRoom(frame.size))
    : null

  return (
    <div
      ref={glanceRef}
      className="image-glance"
      data-ui="image.glance"
      role="dialog"
      aria-label={`Image ${name}`}
      style={{ width: windowSize.width, height: windowSize.height }}
    >
      <FloatingFrameHandles frame={frame} />
      <div className="image-glance-header">
        <PanelPath path={path} className="image-glance-path" />
        <span className="image-glance-size">
          {picture.state === 'shown' ? `${picture.natural.width} × ${picture.natural.height}` : picture.state === 'loading' ? 'Loading…' : ''}
        </span>
        <button
          type="button"
          className="image-glance-word"
          onClick={() => {
            closeImageGlance()
            openInFiles(path)
          }}
        >
          Open in Files
        </button>
        <button
          type="button"
          className="image-glance-word"
          onClick={() => {
            void copyTextToClipboard(path)
            announce(`Copied ${path}`, 'success')
          }}
        >
          Copy path
        </button>
        {frame.remembered && (
          <button type="button" className="image-glance-word" onClick={frame.resetSize}>Reset size</button>
        )}
        <button type="button" className="image-glance-word" onClick={closeImageGlance}>Close</button>
      </div>
      <div className="image-glance-body">
        {picture.state === 'failed' ? (
          <p className="image-glance-note">Could not load {path}{picture.reason ? `: ${picture.reason}` : ''}.</p>
        ) : (
          <img
            key={request.nonce}
            src={getDownloadUrl(path)}
            alt={name}
            style={drawn ? { width: drawn.width, height: drawn.height } : undefined}
            onLoad={event => {
              const { naturalWidth, naturalHeight } = event.currentTarget
              setPicture(naturalWidth > 0 && naturalHeight > 0
                ? { state: 'shown', natural: { width: naturalWidth, height: naturalHeight } }
                : { state: 'failed', reason: null })
            }}
            onError={() => {
              // The <img> never says why. Ask the route once, in words.
              const { nonce } = request
              setPicture({ state: 'failed', reason: null })
              void describeReadFailure(path).then(reason => {
                if (reason && nonceRef.current === nonce) setPicture({ state: 'failed', reason })
              })
            }}
          />
        )}
      </div>
    </div>
  )
}

export default ImageGlance
