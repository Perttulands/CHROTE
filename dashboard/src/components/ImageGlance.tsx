/**
 * The image glance: a look at a picture an agent named, in Peek's manner.
 *
 * A centred floating window over the workspace with a one-line header — the
 * path, the pixel size, the zoom level as words, and Open in Files, Copy path
 * and Close — and the image beneath on the terminal background. It is a
 * glance, so a press outside closes it and is consumed, and Escape closes it
 * from anywhere. It loads the raw route the file viewers already load; there
 * is no route of its own.
 *
 * The zoom level and the remembered window size sit beside each other, and
 * they answer different questions. The level (`imageZoom.ts`) says how big
 * the picture is drawn, for every picture on this device, in the glance, the
 * Files panel and the file viewer alike. The frame's remembered size
 * (`floatingWindowSize.ts`) says how big this window is, once the operator has
 * dragged one. Where there is no remembered size the window is the picture at
 * the level, capped at 90% of the workspace — so one setting still governs
 * what the operator sees. They cannot disagree behind the operator's back,
 * because the one gesture that could make them disagree — dragging the frame —
 * writes both: the dragged size is remembered, and the percent is taken from
 * the width the picture was given. Reset size drops the window's memory and
 * the window goes back to following the picture at the level.
 *
 * A picture bigger than the window at the chosen level scrolls inside it. The
 * window itself never exceeds the workspace; the frame clamps it.
 */

import { useEffect, useMemo, useRef, useState } from 'react'
import PanelPath from './PanelPath'
import FloatingFrameHandles from './FloatingFrameHandles'
import { describeReadFailure, getDownloadUrl } from './FilesView/fileService'
import { getFileBaseName } from './FileViewer'
import { closeImageGlance, useImageGlanceRequest } from './imageGlance'
import {
  IMAGE_ZOOM_FIT,
  IMAGE_ZOOM_ONE_TO_ONE,
  drawImage,
  drawnZoomPercent,
  setImageZoom,
  stepImageZoom,
  useImageZoom,
  zoomPercentForWidth,
  zoomPercentLevel,
  zoomPercentWord,
  type ImageZoomLevel,
  type PixelSize,
} from './imageZoom'
import { openInFiles } from '../terminal/openInFiles'
import { useSurface } from '../keys/dismiss'
import { registerChords, type Chord } from '../keys/chords'
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
  /** The image as drawn at the level. */
  image: PixelSize
}

/** The room the picture has inside a window of the given size. */
function imageRoom(frame: FrameSize): FrameSize {
  return {
    width: frame.width - IMAGE_GLANCE_BORDER_PX,
    height: frame.height - IMAGE_GLANCE_BORDER_PX - IMAGE_GLANCE_HEADER_PX,
  }
}

/** The most of the workspace a glance may take, in pixels. */
function workspaceCap(workspace: PixelSize): FrameSize {
  return {
    width: Math.floor(workspace.width * IMAGE_GLANCE_MAX_SHARE),
    height: Math.floor(workspace.height * IMAGE_GLANCE_MAX_SHARE),
  }
}

/**
 * The window's size for an image of the given pixels at the given level: the
 * picture as the level draws it, with the header and hairline around it, and
 * never more than 90% of the workspace. At fit that is the picture at 1:1 when
 * it fits and scaled down at its own ratio when it does not; at a percent the
 * window stops at the cap and the picture scrolls inside it.
 */
export function imageGlanceSize(
  natural: PixelSize,
  workspace: PixelSize,
  level: ImageZoomLevel = IMAGE_ZOOM_FIT,
): ImageGlanceSize {
  const cap = workspaceCap(workspace)
  const image = drawImage(natural, imageRoom(cap), level)
  return {
    width: Math.min(cap.width, image.width + IMAGE_GLANCE_BORDER_PX),
    height: Math.min(cap.height, image.height + IMAGE_GLANCE_BORDER_PX + IMAGE_GLANCE_HEADER_PX),
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

  const level = useImageZoom()

  // The picture at the level asks for the window's size; a size the operator
  // dragged overrides it, for this picture and every later one.
  const frame = useFloatingFrame({
    kind: 'image',
    elementRef: glanceRef,
    open: request !== null,
    label: 'image',
    contentSize: workspace => (
      picture.state === 'shown' ? imageGlanceSize(picture.natural, workspace, level) : null
    ),
    // A drag is a zoom as well as a resize: the picture is given the width the
    // operator dragged to, and that width is the new percent for every picture.
    onResize: size => {
      if (picture.state !== 'shown') return
      setImageZoom(zoomPercentLevel(zoomPercentForWidth(picture.natural, imageRoom(size).width)))
    },
  })

  // A new request is a new picture: what the last one measured says nothing
  // about this one.
  useEffect(() => {
    setPicture({ state: 'loading' })
  }, [request?.nonce])

  // The keys the level answers to while the glance is open. They are ordinary
  // chords, and they are the terminal workspace's scope because that is where
  // a picture is looked at and because Plus and Minus already belong to the
  // window count there: same scope, later registration, so the glance takes
  // the key back while it is open and gives it up when it closes.
  const room = frame.size ? imageRoom(frame.size) : null
  const natural = picture.state === 'shown' ? picture.natural : null
  const zoomRef = useRef<{ level: ImageZoomLevel; natural: PixelSize | null; room: FrameSize | null }>({ level, natural, room })
  zoomRef.current = { level, natural, room }
  const open = request !== null
  const chords = useMemo<readonly Chord[]>(() => {
    const step = (direction: 1 | -1) => () => {
      const { level: at, natural: pixels, room: inside } = zoomRef.current
      if (!pixels || !inside) return
      setImageZoom(stepImageZoom(at, pixels, inside, direction))
    }
    return [
      { id: 'image.zoomIn', key: '=', direct: { alt: true, key: '+', layoutKeys: ['='] }, label: 'Zoom the picture in', scope: 'workspace', run: step(1) },
      { id: 'image.zoomOut', key: '-', direct: { alt: true, key: '-' }, label: 'Zoom the picture out', scope: 'workspace', run: step(-1) },
      { id: 'image.zoomFit', key: '0', direct: { alt: true, key: '0' }, label: 'Fit the picture to the window', scope: 'workspace', run: () => setImageZoom(IMAGE_ZOOM_FIT) },
      { id: 'image.zoomActual', key: '1', direct: { alt: true, key: '1' }, label: 'Show the picture at 1:1', scope: 'workspace', run: () => setImageZoom(IMAGE_ZOOM_ONE_TO_ONE) },
    ]
  }, [])
  useEffect(() => {
    if (!open) return
    return registerChords(chords)
  }, [chords, open])

  if (!request) return null

  const { path } = request
  const name = getFileBaseName(path)
  const windowSize = frame.size ?? IMAGE_GLANCE_EMPTY
  const drawn = room && natural ? drawImage(natural, room, level) : null
  const percent = room && natural ? drawnZoomPercent(natural, room, level) : null

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
          aria-pressed={level.kind === 'fit'}
          onClick={() => setImageZoom(IMAGE_ZOOM_FIT)}
        >
          Fit
        </button>
        <button
          type="button"
          className="image-glance-word"
          aria-pressed={level.kind === 'percent' && level.percent === 100}
          onClick={() => setImageZoom(IMAGE_ZOOM_ONE_TO_ONE)}
        >
          1:1
        </button>
        <span className="image-glance-zoom">{percent === null ? '' : zoomPercentWord(percent)}</span>
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
