/**
 * Reading one file: the contents and the words for acting on them.
 *
 * The file is read beside the tree, not instead of it. In the terminal
 * workspace this is what the Files pop-out carries — a window hung off the
 * panel's right edge, so walking a directory is one click each and the tree
 * never leaves; on the table it is the column's own contents. Either way the
 * header carries the whole of what can be done with the file — Edit, Diff,
 * Send, Copy path — and Close puts the file away and leaves everything else
 * where it was.
 *
 * The viewer suits itself to the file. Markdown is rendered in the theme, an
 * image is shown, a video or a sound plays with the browser's own controls,
 * JSON is pretty-printed, and everything else is monospace text with line
 * numbers, capped at the first 2000 lines and saying so.
 *
 * A picture or a video at fit is shown whole inside the room the viewer has,
 * in both dimensions, at its own ratio and never above its own size: the room
 * is measured, not assumed, so a drag of the panel or the window refits it.
 *
 * Diff is offered only when the file is inside a git repository, which the
 * panel learns once when the file opens: the same request carries the diff, so
 * pressing Diff costs nothing more.
 */

import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { useStatus } from '../context/StatusContext'
import { copyAndAnnounce } from '../utils/clipboard'
import { useConfirmInPlace } from './confirmInPlace'
import Editor from './Editor'
import Markdown from './Markdown'
import PanelPath from './PanelPath'
import { openImageGlance } from './imageGlance'
import { fitImage, useImageZoom, zoomedPixels, type PixelSize } from './imageZoom'
import { useMeasuredSize } from '../hooks/useMeasuredSize'
import {
  getPreviewKind,
  getFileBaseName,
  getFileExtension,
  isMarkdownFileName,
  makeFileItemFromPath,
} from './FileViewer'
import {
  MAX_TEXT_PREVIEW_BYTES,
  fetchFileDiff,
  getDownloadUrl,
  getErrorMessage,
  probeTextFile,
  readTextFile,
  writeTextFile,
  type FileDiffResult,
} from './FilesView/fileService'

/** How much of a long file the viewer draws before it says it stopped. */
export const MAX_VIEWER_LINES = 2000

type ViewerMode = 'view' | 'diff' | 'edit'

export interface FilePanelViewerProps {
  path: string
  onClose: () => void
  /** Following a Markdown link to another file, without leaving the panel. */
  onOpenPath: (path: string) => void
  /** Null when no terminal has the focus and there is nobody to send to. */
  onSend: ((path: string) => void) | null
  /**
   * A picture is a way to the centred glance where there is nowhere better to
   * look at it. The pop-out is already the look, so it turns this off.
   */
  pictureOpensGlance?: boolean
  /** Offered by a window with a remembered size, next to Close. */
  onResetSize?: (() => void) | null
}

export type DiffLineKind = 'add' | 'del' | 'hunk' | 'context'

export interface DiffLine {
  kind: DiffLineKind
  gutter: string
  text: string
}

/**
 * Read a unified diff into rows.
 *
 * Everything before the first hunk header is dropped: the panel already says
 * which file this is, so `diff --git` and its index line are noise. The gutter
 * carries the sign, so the colour is never the only thing that says what a line
 * is.
 */
export function parseUnifiedDiff(diff: string): DiffLine[] {
  const lines = diff.replace(/\r\n?/g, '\n').split('\n')
  const rows: DiffLine[] = []
  let started = false
  for (const line of lines) {
    if (!started) {
      if (!line.startsWith('@@')) continue
      started = true
    }
    if (line.startsWith('@@')) {
      rows.push({ kind: 'hunk', gutter: '', text: line })
      continue
    }
    if (line.startsWith('+')) {
      rows.push({ kind: 'add', gutter: '+', text: line.slice(1) })
      continue
    }
    if (line.startsWith('-')) {
      rows.push({ kind: 'del', gutter: '-', text: line.slice(1) })
      continue
    }
    if (line.startsWith('\\')) {
      rows.push({ kind: 'context', gutter: '', text: line })
      continue
    }
    rows.push({ kind: 'context', gutter: '', text: line.startsWith(' ') ? line.slice(1) : line })
  }
  while (rows.length > 0 && rows[rows.length - 1].text === '' && rows[rows.length - 1].kind === 'context') rows.pop()
  return rows
}

/** JSON reads as JSON when it parses, and as the bytes on disk when it does not. */
export function prettyJson(content: string): string {
  try {
    return JSON.stringify(JSON.parse(content), null, 2)
  } catch {
    return content
  }
}

function TextLines({ content, label }: { content: string; label: string }) {
  const gutterRef = useRef<HTMLPreElement>(null)
  const all = content.split('\n')
  const capped = all.length > MAX_VIEWER_LINES
  const shown = capped ? all.slice(0, MAX_VIEWER_LINES) : all
  const numbers = shown.map((_, index) => String(index + 1)).join('\n')

  return (
    <>
      <div className="files-panel-lines">
        <pre className="files-panel-lines-gutter" ref={gutterRef} aria-hidden="true">{numbers}</pre>
        <pre
          className="files-panel-lines-text"
          aria-label={label}
          onScroll={event => {
            if (gutterRef.current) gutterRef.current.scrollTop = event.currentTarget.scrollTop
          }}
        >{shown.join('\n')}</pre>
      </div>
      {capped && (
        <p className="files-panel-note">
          First {MAX_VIEWER_LINES} of {all.length} lines. Open the file in a terminal to read the rest.
        </p>
      )}
    </>
  )
}

/**
 * The room a picture or a video is drawn in, and the size it is drawn at.
 *
 * The stage is measured, so fit is the whole thing inside it whatever the
 * window's size; a size the operator asked for (`drawn`) is obeyed literally
 * and the stage scrolls to it.
 */
function MediaStage({
  natural,
  drawn,
  children,
}: {
  natural: PixelSize | null
  drawn: PixelSize | null
  children: (size: PixelSize | null) => ReactNode
}) {
  const room = useMeasuredSize()
  const size = drawn ?? (natural ? fitImage(natural, { width: room.width, height: room.height }) : null)
  return (
    <div className={drawn ? 'files-panel-media is-zoomed' : 'files-panel-media'}>
      <div className="files-panel-media-stage" ref={room.ref}>
        {children(size)}
      </div>
    </div>
  )
}

/** What the viewer says when the browser has nothing to show the file with. */
function NoInlineView({ path }: { path: string }) {
  return (
    <p className="files-panel-note">
      No inline view for this file. <a href={getDownloadUrl(path)} download>Download</a>
    </p>
  )
}

function FilePanelViewer({
  path,
  onClose,
  onOpenPath,
  onSend,
  pictureOpensGlance = true,
  onResetSize = null,
}: FilePanelViewerProps) {
  const { announce } = useStatus()
  const [mode, setMode] = useState<ViewerMode>('view')
  const [content, setContent] = useState<string | null>(null)
  const [draft, setDraft] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [diff, setDiff] = useState<FileDiffResult | null>(null)
  const [saving, setSaving] = useState(false)
  const [pixels, setPixels] = useState<PixelSize | null>(null)
  // A video or sound the browser cannot decode is offered as a download.
  const [mediaFailed, setMediaFailed] = useState(false)
  // One zoom level governs every picture, here as in the glance.
  const zoomLevel = useImageZoom()
  const zoomed = zoomedPixels(pixels, zoomLevel)

  const name = getFileBaseName(path)
  const item = useMemo(() => makeFileItemFromPath(path), [path])
  const kind = getPreviewKind(item)
  const readable = kind === 'text' || kind === 'download'

  useEffect(() => {
    let cancelled = false
    setMode('view')
    setContent(null)
    setError(null)
    setDiff(null)
    setPixels(null)
    setMediaFailed(false)
    setLoading(readable)
    if (readable) {
      const read = kind === 'text'
        ? readTextFile(path, MAX_TEXT_PREVIEW_BYTES)
        : probeTextFile(path, MAX_TEXT_PREVIEW_BYTES)
      void read
        .then(next => {
          if (cancelled) return
          if (next === null) setError('No inline view for this file')
          else setContent(next)
        })
        .catch(readError => {
          if (!cancelled) setError(getErrorMessage(readError, 'read'))
        })
        .finally(() => {
          if (!cancelled) setLoading(false)
        })
    }
    void fetchFileDiff(path)
      .then(next => {
        if (!cancelled) setDiff(next)
      })
      .catch(() => {
        if (!cancelled) setDiff(null)
      })
    return () => { cancelled = true }
  }, [kind, path, readable])

  const startEdit = () => {
    setDraft(content ?? '')
    setMode('edit')
  }

  const discard = useCallback(() => {
    setDraft('')
    setMode('view')
  }, [])

  const { armed, press } = useConfirmInPlace(discard)

  const save = useCallback(() => {
    if (saving) return
    setSaving(true)
    void writeTextFile(path, draft)
      .then(() => {
        setContent(draft)
        setMode('view')
        announce(`Saved ${name}`, 'success')
        return fetchFileDiff(path).then(setDiff).catch(() => undefined)
      })
      .catch(saveError => {
        announce(`Could not save ${name}: ${getErrorMessage(saveError, 'write')}`, 'error')
      })
      .finally(() => setSaving(false))
  }, [announce, draft, name, path, saving])

  const inRepository = Boolean(diff && diff.repository)
  const editable = mode !== 'edit' && content !== null

  return (
    <>
      <div className="files-panel-viewer-head" data-ui="files.header">
        <PanelPath path={path} className="files-panel-viewer-path" />
        <div className="files-panel-actions">
          {mode === 'edit' ? (
            <>
              <button type="button" className="files-panel-action is-current" disabled={saving} onClick={save}>Save</button>
              <button type="button" className="files-panel-action" onClick={press}>{armed ? 'Confirm' : 'Discard'}</button>
            </>
          ) : (
            <>
              {editable && <button type="button" className="files-panel-action" onClick={startEdit}>Edit</button>}
              {inRepository && (
                <button
                  type="button"
                  className={`files-panel-action ${mode === 'diff' ? 'is-current' : ''}`}
                  aria-pressed={mode === 'diff'}
                  onClick={() => setMode(mode === 'diff' ? 'view' : 'diff')}
                >
                  Diff
                </button>
              )}
              <button
                type="button"
                className="files-panel-action"
                disabled={!onSend}
                title={onSend ? 'Send this path to the focused session' : 'Focus a terminal session first'}
                onClick={() => onSend?.(path)}
              >
                Send
              </button>
              <button
                type="button"
                className="files-panel-action"
                onClick={() => void copyAndAnnounce(path, path, announce)}
              >
                Copy path
              </button>
              {onResetSize && (
                <button type="button" className="files-panel-action" onClick={onResetSize}>Reset size</button>
              )}
              <button type="button" className="files-panel-action" onClick={onClose}>Close</button>
            </>
          )}
        </div>
      </div>
      <div className="files-panel-viewer-body" data-ui="files.viewer">
        {mode === 'edit' ? (
          <Editor
            value={draft}
            onChange={setDraft}
            onSave={save}
            onCancel={press}
            label={`Edit ${name}`}
            autoFocus
          />
        ) : mode === 'diff' ? (
          <DiffView diff={diff} />
        ) : loading ? (
          <p className="files-panel-note">Reading {name}…</p>
        ) : error ? (
          <p className="files-panel-note">{error}</p>
        ) : kind === 'image' ? (
          // The picture at the zoom level, whole in the room while that is
          // fit, with its pixels beneath it. Where the picture is not already
          // the look, a press on it opens the centred glance.
          <>
            <MediaStage natural={pixels} drawn={zoomed}>
              {size => {
                const picture = (
                  <img
                    src={getDownloadUrl(path)}
                    alt={name}
                    style={size ? { width: size.width, height: size.height } : undefined}
                    onLoad={event => setPixels({ width: event.currentTarget.naturalWidth, height: event.currentTarget.naturalHeight })}
                  />
                )
                return pictureOpensGlance ? (
                  <button type="button" className="files-panel-image" onClick={() => openImageGlance(path)}>{picture}</button>
                ) : (
                  <div className="files-panel-image is-still">{picture}</div>
                )
              }}
            </MediaStage>
            <p className="files-panel-note">{pixels ? `${pixels.width} × ${pixels.height}` : ''}</p>
          </>
        ) : (kind === 'video' || kind === 'audio') && mediaFailed ? (
          <NoInlineView path={path} />
        ) : kind === 'video' ? (
          // Always fit: a video has no zoom levels.
          <>
            <MediaStage natural={pixels} drawn={null}>
              {size => (
                <video
                  className="files-panel-video"
                  src={getDownloadUrl(path)}
                  aria-label={name}
                  controls
                  preload="metadata"
                  style={size ? { width: size.width, height: size.height } : undefined}
                  onLoadedMetadata={event => {
                    const { videoWidth: width, videoHeight: height } = event.currentTarget
                    if (width > 0 && height > 0) setPixels({ width, height })
                  }}
                  onError={() => setMediaFailed(true)}
                />
              )}
            </MediaStage>
            <p className="files-panel-note">{pixels ? `${pixels.width} × ${pixels.height}` : ''}</p>
          </>
        ) : kind === 'audio' ? (
          <div className="files-panel-audio">
            <audio
              src={getDownloadUrl(path)}
              aria-label={name}
              controls
              preload="metadata"
              onError={() => setMediaFailed(true)}
            />
          </div>
        ) : content === null ? (
          <NoInlineView path={path} />
        ) : isMarkdownFileName(name) ? (
          <div className="files-panel-markdown">
            <Markdown content={content} basePath={path} onOpenPath={onOpenPath} />
          </div>
        ) : getFileExtension(name) === 'json' ? (
          <TextLines content={prettyJson(content)} label={`${name} contents`} />
        ) : (
          <TextLines content={content} label={`${name} contents`} />
        )}
      </div>
    </>
  )
}

function DiffView({ diff }: { diff: FileDiffResult | null }) {
  const rows = useMemo(() => parseUnifiedDiff(diff?.diff ?? ''), [diff])
  if (!diff) return <p className="files-panel-note">Reading the diff…</p>
  if (!diff.repository) return <p className="files-panel-note">Not inside a git repository.</p>
  if (rows.length === 0) return <p className="files-panel-note">No changes against HEAD.</p>
  return (
    <div className="files-panel-diff" aria-label="Diff against HEAD">
      {rows.map((row, index) => (
        <div key={index} className={`files-panel-diff-line is-${row.kind}`}>
          <span className="files-panel-diff-gutter" aria-hidden="true">{row.gutter}</span>
          <span className="files-panel-diff-text">{row.text || ' '}</span>
        </div>
      ))}
      {diff.truncated && <p className="files-panel-note">The diff is longer than the panel will show.</p>}
    </div>
  )
}

export default FilePanelViewer
