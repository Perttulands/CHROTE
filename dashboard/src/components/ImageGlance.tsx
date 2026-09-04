/**
 * The image glance: a look at a picture an agent named, in Peek's manner.
 *
 * A centred floating window over the workspace with a one-line header — the
 * path, the pixel size, and Open in Files, Copy path and Close as words — and
 * the image beneath on the terminal background. It is sized to the image:
 * never larger than the picture is, never more than 90% of the workspace,
 * and never upscaled. It is a glance, so a press outside closes it and is
 * consumed, and Escape closes it from anywhere. It loads the raw route the
 * file viewers already load; there is no route of its own.
 */

import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import PanelPath from './PanelPath'
import { getDownloadUrl } from './FilesView/fileService'
import { getFileBaseName } from './FileViewer'
import { closeImageGlance, useImageGlanceRequest } from './imageGlance'
import { openInFiles } from '../terminal/openInFiles'
import { useSurface } from '../keys/dismiss'
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

/**
 * The window's size for an image of the given pixels: the image at 1:1 when
 * it fits inside 90% of the workspace, scaled down at its own ratio when it
 * does not, and the header and hairline around it.
 */
export function imageGlanceSize(
  natural: { width: number; height: number },
  workspace: { width: number; height: number },
): ImageGlanceSize {
  const roomWidth = Math.floor(workspace.width * IMAGE_GLANCE_MAX_SHARE) - IMAGE_GLANCE_BORDER_PX
  const roomHeight = Math.floor(workspace.height * IMAGE_GLANCE_MAX_SHARE) - IMAGE_GLANCE_BORDER_PX - IMAGE_GLANCE_HEADER_PX
  const scale = Math.min(1, roomWidth / natural.width, roomHeight / natural.height)
  const image = {
    width: Math.max(1, Math.floor(natural.width * scale)),
    height: Math.max(1, Math.floor(natural.height * scale)),
  }
  return {
    width: image.width + IMAGE_GLANCE_BORDER_PX,
    height: image.height + IMAGE_GLANCE_BORDER_PX + IMAGE_GLANCE_HEADER_PX,
    image,
  }
}

type Picture =
  | { state: 'loading' }
  | { state: 'shown'; natural: { width: number; height: number } }
  | { state: 'failed' }

function ImageGlance() {
  const request = useImageGlanceRequest()
  const { announce } = useStatus()
  const glanceRef = useRef<HTMLDivElement>(null)
  const [picture, setPicture] = useState<Picture>({ state: 'loading' })
  const [size, setSize] = useState<ImageGlanceSize | null>(null)

  useSurface({ open: request !== null, kind: 'glance', onClose: closeImageGlance, ref: glanceRef })

  // A new request is a new picture: what the last one measured says nothing
  // about this one.
  useEffect(() => {
    setPicture({ state: 'loading' })
    setSize(null)
  }, [request?.nonce])

  // The window is sized once the image's pixels are known, and again only
  // when the operator resizes the window.
  useLayoutEffect(() => {
    if (!request || picture.state !== 'shown') return
    const measure = () => {
      const workspace = glanceRef.current?.parentElement
      if (!workspace) return
      setSize(imageGlanceSize(picture.natural, { width: workspace.clientWidth, height: workspace.clientHeight }))
    }
    measure()
    window.addEventListener('resize', measure)
    return () => window.removeEventListener('resize', measure)
  }, [request, picture])

  if (!request) return null

  const { path } = request
  const name = getFileBaseName(path)
  const frame = size ?? IMAGE_GLANCE_EMPTY

  return (
    <div
      ref={glanceRef}
      className="image-glance"
      data-ui="image.glance"
      role="dialog"
      aria-label={`Image ${name}`}
      style={{ width: frame.width, height: frame.height }}
    >
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
        <button type="button" className="image-glance-word" onClick={closeImageGlance}>Close</button>
      </div>
      <div className="image-glance-body">
        {picture.state === 'failed' ? (
          <p className="image-glance-note">Could not load {path}.</p>
        ) : (
          <img
            key={request.nonce}
            src={getDownloadUrl(path)}
            alt={name}
            style={size ? { width: size.image.width, height: size.image.height } : undefined}
            onLoad={event => {
              const { naturalWidth, naturalHeight } = event.currentTarget
              setPicture(naturalWidth > 0 && naturalHeight > 0
                ? { state: 'shown', natural: { width: naturalWidth, height: naturalHeight } }
                : { state: 'failed' })
            }}
            onError={() => setPicture({ state: 'failed' })}
          />
        )}
      </div>
    </div>
  )
}

export default ImageGlance
