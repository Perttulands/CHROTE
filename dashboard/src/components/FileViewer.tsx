import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import type { CSSProperties, MouseEvent as ReactMouseEvent, ReactNode } from 'react'
import { MAX_TEXT_PREVIEW_BYTES, getDownloadUrl, getErrorMessage, probeTextFile, readTextFile } from './FilesView/fileService'
import type { FileItem } from './FilesView/types'
import type { FileViewState, MarkdownMode } from './workspaceFilesState'
import { openImageGlance } from './imageGlance'
import { useResizableWidth } from '../hooks/useResizableWidth'

export type PreviewKind = 'text' | 'image' | 'audio' | 'video' | 'pdf' | 'download'
export { MAX_TEXT_PREVIEW_BYTES } from './FilesView/fileService'

const TEXT_EXTENSIONS = new Set([
  'adoc', 'asm', 'bash', 'bat', 'c', 'cc', 'cfg', 'clj', 'cljs', 'cljc', 'cmake', 'cnf',
  'conf', 'cpp', 'cs', 'css', 'csv', 'cts', 'cue', 'dart', 'desktop', 'diff',
  'dockerfile', 'dockerignore', 'editorconfig', 'eml', 'env', 'erl', 'ex', 'exs', 'fish',
  'fs', 'fsx', 'geojson', 'gitattributes', 'gitconfig', 'gitignore', 'gitmodules', 'go',
  'gql', 'gradle', 'graphql', 'groovy', 'h', 'har', 'hcl', 'hpp', 'hs', 'htm', 'html',
  'ics', 'ignore', 'ini', 'ipynb', 'java', 'jl', 'js', 'json', 'json5', 'jsonc', 'jsonl',
  'jsx', 'kt', 'kts', 'less',
  'lock', 'log', 'lua', 'm', 'markdown', 'md', 'mjs', 'mount',
  'ndjson', 'nim', 'nix', 'npmrc', 'patch', 'path', 'pem', 'php', 'pl',
  'properties', 'proto', 'ps1', 'psv', 'py', 'r', 'rb', 'rego', 'rs', 'rst', 's', 'sass',
  'scala', 'scss', 'service', 'sh', 'socket', 'sql', 'svelte',
  'swift', 'target', 'tex', 'tf', 'tfstate', 'tfvars', 'timer', 'toml', 'ts',
  'tsbuildinfo', 'tsv', 'tsx', 'txt', 'vcf', 'vue', 'webmanifest', 'xml',
  'yaml', 'yml', 'zsh',
])
const TEXT_FILE_NAMES = new Set([
  'brewfile', 'gemfile', 'go.mod', 'go.sum', 'go.work', 'jenkinsfile', 'justfile', 'license',
  'makefile', 'pipfile', 'procfile', 'rakefile', 'readme', 'vagrantfile',
])
const IMAGE_EXTENSIONS = new Set(['avif', 'bmp', 'gif', 'ico', 'jpeg', 'jpg', 'png', 'svg', 'webp'])
const AUDIO_EXTENSIONS = new Set(['aac', 'flac', 'm4a', 'mp3', 'oga', 'ogg', 'opus', 'wav'])
const VIDEO_EXTENSIONS = new Set(['avi', 'm4v', 'mkv', 'mov', 'mp4', 'mpeg', 'mpg', 'ogv', 'webm'])
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
  if (TEXT_EXTENSIONS.has(extension) || TEXT_FILE_NAMES.has(lower)) return 'text'
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
  const splitContainerRef = useRef<HTMLDivElement>(null)
  const splitSourceRef = useRef<HTMLElement | null>(null)
  const [loadedContent, setLoadedContent] = useState(controlledContent || '')
  const [probedFile, setProbedFile] = useState<{ path: string; content: string } | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const declaredKind = getPreviewKind(item)
  const probedContent = probedFile?.path === item.path ? probedFile.content : null
  const kind = declaredKind === 'download' && probedContent !== null ? 'text' : declaredKind
  const content = controlledContent === undefined ? probedContent ?? loadedContent : controlledContent

  const patchViewState = useCallback((patch: Partial<FileViewState>) => {
    onViewStateChange({ ...viewState, ...patch })
  }, [onViewStateChange, viewState])
  const splitPixelsPerPercent = useCallback(() => {
    const width = splitContainerRef.current?.getBoundingClientRect().width ?? 0
    return width > 0 ? width / 100 : 1
  }, [])
  const widestSplit = useCallback(() => 80, [])
  const commitSplitWidth = useCallback((markdownSplitPercent: number) => {
    patchViewState({ markdownSplitPercent })
  }, [patchViewState])
  const splitResize = useResizableWidth({
    elementRef: splitSourceRef,
    width: viewState.markdownSplitPercent,
    minWidth: 20,
    maxWidth: widestSplit,
    edge: 'right',
    onCommit: commitSplitWidth,
    pixelsPerUnit: splitPixelsPerPercent,
  })
  const captureSplitSource = useCallback((element: HTMLElement | null) => {
    splitSourceRef.current = element
  }, [])

  useEffect(() => {
    let cancelled = false
    setLoading(false)
    setError(null)
    setProbedFile(null)
    if (controlledContent !== undefined || (declaredKind !== 'text' && declaredKind !== 'download')) return
    if (item.size > MAX_TEXT_PREVIEW_BYTES) {
      if (declaredKind === 'text') setError('File is too large for inline viewing')
      return
    }
    setLoading(true)
    const read = declaredKind === 'text'
      ? readTextFile(item.path, MAX_TEXT_PREVIEW_BYTES)
      : probeTextFile(item.path, MAX_TEXT_PREVIEW_BYTES)
    void read
      .then(next => {
        if (cancelled || next === null) return
        if (declaredKind === 'text') setLoadedContent(next)
        else setProbedFile({ path: item.path, content: next })
      })
      .catch(readError => {
        if (!cancelled) setError(getErrorMessage(readError, 'read'))
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => { cancelled = true }
  }, [controlledContent, declaredKind, item.path, item.size])

  useLayoutEffect(() => {
    if (!loading && scrollRef.current) scrollRef.current.scrollTop = viewState.scrollTop
  }, [item.path, loading, viewState.markdownMode, viewState.scrollTop])

  const setMarkdownMode = (mode: MarkdownMode) => patchViewState({ markdownMode: mode })
  const viewerStyle = {
    '--fb-viewer-font-size': `${viewState.fontSize}px`,
    '--fb-markdown-source-width': `${splitResize.width}%`,
  } as CSSProperties

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
          <div ref={splitContainerRef} className={`fb-markdown-editor mode-${viewState.markdownMode}`} aria-label={`Markdown viewer for ${item.name}`}>
            {(viewState.markdownMode === 'source' || viewState.markdownMode === 'split') && (
              editable ? (
                <textarea
                  ref={captureSplitSource}
                  className="fb-markdown-source"
                  aria-label={`Markdown source for ${item.name}`}
                  value={content}
                  wrap="off"
                  spellCheck={false}
                  onChange={event => onContentChange?.(event.target.value)}
                />
              ) : (
                <pre ref={captureSplitSource} className="fb-markdown-source fb-markdown-source-readonly" aria-label={`Markdown source for ${item.name}`}>{content}</pre>
              )
            )}
            {viewState.markdownMode === 'split' && (
              <div
                {...splitResize.handleProps}
                className={`fb-markdown-split-resizer${splitResize.resizing ? ' dragging' : ''}`}
                role="separator"
                aria-label="Resize Markdown split"
                aria-orientation="vertical"
                aria-valuenow={Math.round(splitResize.width)}
                aria-valuemin={20}
                aria-valuemax={80}
                tabIndex={0}
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
          // A click on the picture opens the glance for a look at it full size.
          <div className={`fb-media-preview ${viewState.imageFit ? 'is-fit' : 'is-actual'}`}>
            <button type="button" className="fb-image-look" onClick={() => openImageGlance(item.path)}>
              <img
                src={getDownloadUrl(item.path)}
                alt={item.name}
                style={viewState.imageFit ? undefined : { transform: `scale(${viewState.imageZoom})` }}
              />
            </button>
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
