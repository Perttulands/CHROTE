/**
 * Reading one file inside the Files panel.
 *
 * Opening a file replaces the tree rather than floating a window over the
 * work: the operator is reading beside a terminal, and a window that covers
 * the terminal defeats the reason the panel is there. Back returns to the
 * tree, and the header carries the whole of what can be done with the file —
 * Edit, Diff, Send — as words.
 *
 * The viewer suits itself to the file. Markdown is rendered in the theme, an
 * image is shown, JSON is pretty-printed, and everything else is monospace
 * text with line numbers, capped at the first 2000 lines and saying so.
 *
 * Diff is offered only when the file is inside a git repository, which the
 * panel learns once when the file opens: the same request carries the diff, so
 * pressing Diff costs nothing more.
 */

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useStatus } from '../context/StatusContext'
import { useConfirmInPlace } from './confirmInPlace'
import Editor from './Editor'
import Markdown from './Markdown'
import PanelPath from './PanelPath'
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
  onBack: () => void
  /** Following a Markdown link to another file, without leaving the panel. */
  onOpenPath: (path: string) => void
  /** Null when no terminal has the focus and there is nobody to send to. */
  onSend: ((path: string) => void) | null
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

function FilePanelViewer({ path, onBack, onOpenPath, onSend }: FilePanelViewerProps) {
  const { announce } = useStatus()
  const [mode, setMode] = useState<ViewerMode>('view')
  const [content, setContent] = useState<string | null>(null)
  const [draft, setDraft] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [diff, setDiff] = useState<FileDiffResult | null>(null)
  const [saving, setSaving] = useState(false)

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
      <div className="files-panel-viewer-head">
        <button type="button" className="files-panel-action" onClick={onBack}>Back</button>
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
            </>
          )}
        </div>
      </div>
      <div className="files-panel-viewer-body">
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
          <div className="files-panel-image"><img src={getDownloadUrl(path)} alt={name} /></div>
        ) : content === null ? (
          <p className="files-panel-note">
            No inline view for this file. <a href={getDownloadUrl(path)} download>Download</a>
          </p>
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
