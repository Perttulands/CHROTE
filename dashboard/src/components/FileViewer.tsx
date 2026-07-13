import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import type { CSSProperties, MouseEvent as ReactMouseEvent, PointerEvent as ReactPointerEvent, ReactNode } from 'react'
import { getDownloadUrl, getErrorMessage, readTextFile } from './FilesView/fileService'
import type { FileItem } from './FilesView/types'
import type { FileViewState, MarkdownMode } from './workspaceFilesState'

export type PreviewKind = 'text' | 'image' | 'audio' | 'video' | 'pdf' | 'download'

const TEXT_EXTENSIONS = new Set([
  'bash', 'c', 'conf', 'cpp', 'css', 'csv', 'dockerfile', 'env', 'go', 'h', 'html', 'ini',
  'java', 'js', 'json', 'jsx', 'log', 'md', 'markdown', 'py', 'rb', 'rs', 'sh', 'sql',
  'toml', 'ts', 'tsx', 'txt', 'xml', 'yaml', 'yml',
])
const IMAGE_EXTENSIONS = new Set(['bmp', 'gif', 'jpeg', 'jpg', 'png', 'svg', 'webp'])
const AUDIO_EXTENSIONS = new Set(['flac', 'm4a', 'mp3', 'ogg', 'wav'])
const VIDEO_EXTENSIONS = new Set(['avi', 'mkv', 'mov', 'mp4', 'webm'])
export const MAX_TEXT_PREVIEW_BYTES = 1024 * 1024

export function normalizeFilePath(path: string): string {
  const trimmed = path.trim()
  if (!trimmed || trimmed === '.') return '/'
  const withRoot = trimmed.startsWith('/') ? trimmed : `/${trimmed}`
  const compact = withRoot.replace(/\/+/g, '/')
  return compact.length > 1 ? compact.replace(/\/$/, '') : compact
}

export function getFileBaseName(path: string): string {
  const parts = normalizeFilePath(path).split('/').filter(Boolean)
  return parts[parts.length - 1] || '/'
}

export function getFileExtension(name: string): string {
  const lower = name.toLowerCase()
  if (lower === 'dockerfile' || lower.startsWith('dockerfile.')) return 'dockerfile'
  if (lower.startsWith('.env')) return 'env'
  const parts = lower.split('.')
  return parts.length > 1 ? parts.pop() || '' : ''
}

export function isMarkdownFileName(name: string): boolean {
  const extension = getFileExtension(name)
  return extension === 'md' || extension === 'markdown'
}

export function getPreviewKind(item: FileItem): PreviewKind {
  const extension = getFileExtension(item.name)
  const lower = item.name.toLowerCase()
  if (TEXT_EXTENSIONS.has(extension) || lower === 'readme' || lower === 'license' || lower === 'makefile') return 'text'
  if (IMAGE_EXTENSIONS.has(extension)) return 'image'
  if (AUDIO_EXTENSIONS.has(extension)) return 'audio'
  if (VIDEO_EXTENSIONS.has(extension)) return 'video'
  if (extension === 'pdf') return 'pdf'
  return 'download'
}

export function getFileBadge(item: FileItem): string {
  if (item.isDir) return 'DIR'
  const extension = getFileExtension(item.name)
  return extension ? extension.slice(0, 4).toUpperCase() : 'TXT'
}

export function makeFileItemFromPath(path: string): FileItem {
  const name = getFileBaseName(path)
  return {
    name,
    path: normalizeFilePath(path),
    isDir: false,
    size: 0,
    modified: new Date().toISOString(),
    type: getFileExtension(name),
  }
}

function getSafeMarkdownHref(rawHref: string): string | null {
  const href = rawHref.trim()
  if (!href) return null
  const normalized = href.replace(/[\u0000-\u001F\u007F\s]+/g, '').toLowerCase()
  if (/^(javascript|data|vbscript):/.test(normalized)) return null
  if (/^[a-z][a-z0-9+.-]*:/i.test(href) && !/^(https?:|mailto:)/i.test(href)) return null
  return href
}

function plainMarkdownText(text: string): string {
  return text.replace(/\[([^\]\n]+)\]\(([^)\n]+)\)/g, '$1').replace(/[*_`~]/g, '').trim()
}

function renderMarkdownInline(text: string, keyPrefix: string): ReactNode[] {
  const nodes: ReactNode[] = []
  const linkPattern = /\[([^\]\n]+)\]\(([^)\n]+)\)/g
  let cursor = 0
  let linkIndex = 0
  let match: RegExpExecArray | null
  while ((match = linkPattern.exec(text)) !== null) {
    if (match.index > cursor) nodes.push(text.slice(cursor, match.index))
    const [, label, href] = match
    const safeHref = getSafeMarkdownHref(href)
    const key = `${keyPrefix}-link-${linkIndex}`
    nodes.push(safeHref ? (
      <a
        key={key}
        href={safeHref}
        target="_blank"
        rel="noopener noreferrer"
        onClick={(event: ReactMouseEvent<HTMLAnchorElement>) => event.preventDefault()}
      >
        {label}
      </a>
    ) : <span key={key} className="fb-markdown-unsafe-link">{label}</span>)
    cursor = linkPattern.lastIndex
    linkIndex += 1
  }
  if (cursor < text.length) nodes.push(text.slice(cursor))
  return nodes
}

function isMarkdownBlockStart(line: string): boolean {
  return /^(#{1,6})\s+/.test(line)
    || /^(```|~~~)\s*([A-Za-z0-9_-]+)?\s*$/.test(line)
    || /^>\s?/.test(line)
    || /^\s*(?:[-*+]\s+|\d+[.)]\s+)/.test(line)
}

export function renderMarkdownPreview(content: string): ReactNode[] {
  const lines = content.replace(/\r\n?/g, '\n').split('\n')
  const nodes: ReactNode[] = []
  let index = 0
  while (index < lines.length) {
    const line = lines[index]
    if (line.trim() === '') {
      index += 1
      continue
    }
    const fenceMatch = line.match(/^(```|~~~)\s*([A-Za-z0-9_-]+)?\s*$/)
    if (fenceMatch) {
      const marker = fenceMatch[1]
      const language = fenceMatch[2]
      const codeLines: string[] = []
      index += 1
      while (index < lines.length && !lines[index].startsWith(marker)) {
        codeLines.push(lines[index])
        index += 1
      }
      if (index < lines.length) index += 1
      nodes.push(
        <pre key={`md-code-${nodes.length}`} className="fb-markdown-code">
          <code className={language ? `language-${language}` : undefined}>{codeLines.join('\n')}</code>
        </pre>,
      )
      continue
    }
    const headingMatch = line.match(/^(#{1,6})\s+(.+)$/)
    if (headingMatch) {
      const HeadingTag = `h${headingMatch[1].length}` as 'h1' | 'h2' | 'h3' | 'h4' | 'h5' | 'h6'
      nodes.push(<HeadingTag key={`md-heading-${nodes.length}`}>{renderMarkdownInline(headingMatch[2], `heading-${nodes.length}`)}</HeadingTag>)
      index += 1
      continue
    }
    if (/^>\s?/.test(line)) {
      const quoteLines: string[] = []
      while (index < lines.length) {
        const quoteMatch = lines[index].match(/^>\s?(.*)$/)
        if (!quoteMatch) break
        quoteLines.push(quoteMatch[1])
        index += 1
      }
      nodes.push(
        <blockquote key={`md-quote-${nodes.length}`}>
          {quoteLines.map((quoteLine, quoteIndex) => (
            <p key={`quote-line-${quoteIndex}`}>{renderMarkdownInline(quoteLine, `quote-${nodes.length}-${quoteIndex}`)}</p>
          ))}
        </blockquote>,
      )
      continue
    }
    const listMatch = line.match(/^\s*((?:[-*+])|(?:\d+[.)]))\s+(.+)$/)
    if (listMatch) {
      const ordered = /^\d/.test(listMatch[1])
      const items: Array<{ text: string; checked: boolean | null }> = []
      while (index < lines.length) {
        const itemMatch = lines[index].match(/^\s*((?:[-*+])|(?:\d+[.)]))\s+(.+)$/)
        if (!itemMatch || /^\d/.test(itemMatch[1]) !== ordered) break
        const taskMatch = itemMatch[2].match(/^\[([ xX])\]\s+(.+)$/)
        items.push({
          text: taskMatch ? taskMatch[2] : itemMatch[2],
          checked: taskMatch ? taskMatch[1].toLowerCase() === 'x' : null,
        })
        index += 1
      }
      const ListTag = ordered ? 'ol' : 'ul'
      nodes.push(
        <ListTag key={`md-list-${nodes.length}`} className={items.some(item => item.checked !== null) ? 'fb-markdown-task-list' : undefined}>
          {items.map((item, itemIndex) => (
            <li key={`item-${itemIndex}`} className={item.checked !== null ? 'fb-markdown-task-item' : undefined}>
              {item.checked !== null && (
                <input
                  type="checkbox"
                  aria-label={`Task: ${plainMarkdownText(item.text)}`}
                  checked={item.checked}
                  disabled
                  readOnly
                />
              )}
              <span>{renderMarkdownInline(item.text, `list-${nodes.length}-${itemIndex}`)}</span>
            </li>
          ))}
        </ListTag>,
      )
      continue
    }
    const paragraphLines: string[] = []
    while (index < lines.length && lines[index].trim() !== '' && !isMarkdownBlockStart(lines[index])) {
      paragraphLines.push(lines[index])
      index += 1
    }
    nodes.push(<p key={`md-paragraph-${nodes.length}`}>{renderMarkdownInline(paragraphLines.join(' '), `paragraph-${nodes.length}`)}</p>)
  }
  return nodes
}

interface FileViewerProps {
  item: FileItem
  viewState: FileViewState
  onViewStateChange: (state: FileViewState) => void
  content?: string
  editable?: boolean
  onContentChange?: (content: string) => void
  onPrevious?: (() => void) | null
  onNext?: (() => void) | null
}

function FileViewer({
  item,
  viewState,
  onViewStateChange,
  content: controlledContent,
  editable = false,
  onContentChange,
  onPrevious,
  onNext,
}: FileViewerProps) {
  const scrollRef = useRef<HTMLDivElement>(null)
  const [loadedContent, setLoadedContent] = useState(controlledContent || '')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const kind = getPreviewKind(item)
  const content = controlledContent === undefined ? loadedContent : controlledContent

  const patchViewState = useCallback((patch: Partial<FileViewState>) => {
    onViewStateChange({ ...viewState, ...patch })
  }, [onViewStateChange, viewState])

  useEffect(() => {
    if (controlledContent !== undefined || kind !== 'text') return
    if (item.size > MAX_TEXT_PREVIEW_BYTES) {
      setError('File is too large for inline viewing')
      return
    }
    let cancelled = false
    setLoading(true)
    setError(null)
    void readTextFile(item.path)
      .then(next => {
        if (!cancelled) setLoadedContent(next)
      })
      .catch(readError => {
        if (!cancelled) setError(getErrorMessage(readError, 'read'))
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => { cancelled = true }
  }, [controlledContent, item.path, item.size, kind])

  useLayoutEffect(() => {
    if (!loading && scrollRef.current) scrollRef.current.scrollTop = viewState.scrollTop
  }, [item.path, loading, viewState.markdownMode, viewState.scrollTop])

  const setMarkdownMode = (mode: MarkdownMode) => patchViewState({ markdownMode: mode })
  const viewerStyle = {
    '--fb-viewer-font-size': `${viewState.fontSize}px`,
    '--fb-markdown-source-width': `${viewState.markdownSplitPercent}%`,
  } as CSSProperties

  const resizeMarkdownSplit = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (!event.currentTarget.hasPointerCapture(event.pointerId)) return
    const container = event.currentTarget.parentElement
    if (!container) return
    const bounds = container.getBoundingClientRect()
    if (bounds.width <= 0) return
    const percent = Math.min(80, Math.max(20, ((event.clientX - bounds.left) / bounds.width) * 100))
    patchViewState({ markdownSplitPercent: percent })
  }

  if (loading) return <div className="fb-editor-empty">Loading file...</div>
  if (error) return <div className="fb-editor-empty">{error}</div>

  return (
    <div className="fb-file-viewer" style={viewerStyle}>
      {(kind === 'text' && isMarkdownFileName(item.name)) && (
        <div className="fb-viewer-controls" aria-label="Markdown view controls">
          <button type="button" aria-label="Preview Markdown" aria-pressed={viewState.markdownMode === 'preview'} onClick={() => setMarkdownMode('preview')}>Preview</button>
          <button type="button" aria-label="Show Markdown source" aria-pressed={viewState.markdownMode === 'source'} onClick={() => setMarkdownMode('source')}>Source</button>
          <button type="button" aria-label="Split Markdown view" aria-pressed={viewState.markdownMode === 'split'} onClick={() => setMarkdownMode('split')}>Split</button>
          <span className="fb-viewer-controls-spacer" />
          <button type="button" aria-label="Decrease viewer font size" onClick={() => patchViewState({ fontSize: Math.max(11, viewState.fontSize - 1) })}>A−</button>
          <span>{viewState.fontSize}px</span>
          <button type="button" aria-label="Increase viewer font size" onClick={() => patchViewState({ fontSize: Math.min(28, viewState.fontSize + 1) })}>A+</button>
        </div>
      )}
      {kind === 'image' && (
        <div className="fb-viewer-controls" aria-label="Image view controls">
          <button type="button" disabled={!onPrevious} onClick={() => onPrevious?.()}>Previous</button>
          <button type="button" onClick={() => patchViewState({ imageFit: !viewState.imageFit })}>{viewState.imageFit ? 'Actual size' : 'Fit'}</button>
          <button type="button" aria-label="Zoom out" onClick={() => patchViewState({ imageZoom: Math.max(0.1, viewState.imageZoom - 0.1), imageFit: false })}>−</button>
          <span>{Math.round(viewState.imageZoom * 100)}%</span>
          <button type="button" aria-label="Zoom in" onClick={() => patchViewState({ imageZoom: Math.min(8, viewState.imageZoom + 0.1), imageFit: false })}>+</button>
          <button type="button" disabled={!onNext} onClick={() => onNext?.()}>Next</button>
        </div>
      )}
      <div
        ref={scrollRef}
        className={`fb-file-viewer-scroll fb-file-viewer-${kind}`}
        data-testid="file-viewer-scroll"
        onScroll={event => patchViewState({ scrollTop: event.currentTarget.scrollTop })}
      >
        {kind === 'text' && isMarkdownFileName(item.name) ? (
          <div className={`fb-markdown-editor mode-${viewState.markdownMode}`} aria-label={`Markdown viewer for ${item.name}`}>
            {(viewState.markdownMode === 'source' || viewState.markdownMode === 'split') && (
              editable ? (
                <textarea
                  className="fb-markdown-source"
                  aria-label={`Markdown source for ${item.name}`}
                  value={content}
                  spellCheck={false}
                  onChange={event => onContentChange?.(event.target.value)}
                />
              ) : (
                <pre className="fb-markdown-source fb-markdown-source-readonly" aria-label={`Markdown source for ${item.name}`}>{content}</pre>
              )
            )}
            {viewState.markdownMode === 'split' && (
              <div
                className="fb-markdown-split-resizer"
                role="separator"
                aria-label="Resize Markdown split"
                aria-orientation="vertical"
                tabIndex={0}
                onPointerDown={event => event.currentTarget.setPointerCapture(event.pointerId)}
                onPointerMove={resizeMarkdownSplit}
                onPointerUp={event => event.currentTarget.releasePointerCapture(event.pointerId)}
                onKeyDown={event => {
                  if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return
                  event.preventDefault()
                  patchViewState({
                    markdownSplitPercent: Math.min(80, Math.max(20, viewState.markdownSplitPercent + (event.key === 'ArrowRight' ? 5 : -5))),
                  })
                }}
              />
            )}
            {(viewState.markdownMode === 'preview' || viewState.markdownMode === 'split') && (
              <article className="fb-markdown-preview" aria-label={`Markdown preview for ${item.name}`}>{renderMarkdownPreview(content)}</article>
            )}
          </div>
        ) : kind === 'text' ? (
          editable ? (
            <textarea className="fb-editor-textarea" value={content} spellCheck={false} onChange={event => onContentChange?.(event.target.value)} />
          ) : <pre className="fb-plain-text-preview">{content}</pre>
        ) : kind === 'image' ? (
          <div className={`fb-media-preview ${viewState.imageFit ? 'is-fit' : 'is-actual'}`}>
            <img
              src={getDownloadUrl(item.path)}
              alt={item.name}
              style={viewState.imageFit ? undefined : { transform: `scale(${viewState.imageZoom})` }}
            />
          </div>
        ) : kind === 'audio' ? (
          <div className="fb-media-preview"><audio src={getDownloadUrl(item.path)} controls /></div>
        ) : kind === 'video' ? (
          <div className="fb-media-preview"><video src={getDownloadUrl(item.path)} controls /></div>
        ) : kind === 'pdf' ? (
          <iframe className="fb-pdf-preview" src={getDownloadUrl(item.path)} title={item.name} />
        ) : (
          <div className="fb-editor-empty">
            <p>No inline preview is available for this file type.</p>
            <a className="fb-btn" href={getDownloadUrl(item.path)} download>Download</a>
          </div>
        )}
      </div>
    </div>
  )
}

export default FileViewer
