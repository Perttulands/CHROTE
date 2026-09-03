/**
 * Markdown rendered in CHROTE's theme.
 *
 * One renderer serves every surface that shows Markdown: the Files panel's
 * viewer, the Bead card, and the Library. It is react-markdown with GitHub
 * flavour, so tables, task lists and strikethrough behave the way the operator
 * writes them, and the theme is CSS — `.chrote-markdown` in Markdown.css — not
 * a component per element.
 *
 * The one behaviour that is not styling: a link to another file opens in the
 * surface the reader is already in. A document that cites its neighbours is
 * only useful if following the citation does not throw the reader out of the
 * panel, so a relative or absolute filesystem link is resolved against the
 * document's own path and handed to `onOpenPath`. Everything else is an
 * ordinary external link, and a link CHROTE will not follow (`javascript:` and
 * its relatives) is drawn as plain text.
 */

import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import type { AnchorHTMLAttributes, ImgHTMLAttributes, MouseEvent as ReactMouseEvent } from 'react'
import { getDownloadUrl } from './FilesView/fileService'

export interface MarkdownProps {
  /** The Markdown source. */
  content: string
  /** The document's own absolute path, so relative links can be resolved. */
  basePath?: string
  /** Where a link to another file goes. Without it every link is external. */
  onOpenPath?: (path: string) => void
}

/** Schemes CHROTE follows. Anything else is drawn as text, never as a link. */
const SAFE_SCHEME = /^(https?:|mailto:)/i
const HAS_SCHEME = /^[a-z][a-z0-9+.-]*:/i

/** Resolve `href` against the directory holding `basePath`, POSIX style. */
export function resolveMarkdownPath(basePath: string, href: string): string {
  const target = href.replace(/[?#].*$/, '')
  if (!target) return ''
  const base = target.startsWith('/')
    ? []
    : basePath.split('/').slice(0, -1).filter(Boolean)
  const parts = target.startsWith('/') ? target.split('/').filter(Boolean) : base.concat(target.split('/'))
  const stack: string[] = []
  for (const part of parts) {
    if (part === '' || part === '.') continue
    if (part === '..') {
      stack.pop()
      continue
    }
    stack.push(part)
  }
  return `/${stack.join('/')}`
}

/**
 * A file link is anything without a scheme: `../PRD.md`, `docs/journeys.md`,
 * `/srv/chrote/README.md`. A bare fragment (`#section`) stays on the page.
 */
export function isFileLink(href: string): boolean {
  return href !== '' && !href.startsWith('#') && !HAS_SCHEME.test(href)
}

function MarkdownLink(
  { href, children, basePath, onOpenPath, ...rest }: AnchorHTMLAttributes<HTMLAnchorElement> & {
    basePath: string
    onOpenPath?: (path: string) => void
  },
) {
  const target = (href || '').trim()
  if (onOpenPath && isFileLink(target)) {
    const path = resolveMarkdownPath(basePath, target)
    return (
      <a
        {...rest}
        href={path}
        className="chrote-markdown-file-link"
        onClick={(event: ReactMouseEvent<HTMLAnchorElement>) => {
          event.preventDefault()
          onOpenPath(path)
        }}
      >
        {children}
      </a>
    )
  }
  if (!SAFE_SCHEME.test(target)) return <span {...rest}>{children}</span>
  return <a {...rest} href={target} target="_blank" rel="noopener noreferrer">{children}</a>
}

/** An image beside the document is a file on the host, so it is read as one. */
function MarkdownImage({ src, basePath, ...rest }: ImgHTMLAttributes<HTMLImageElement> & { basePath: string }) {
  const target = typeof src === 'string' ? src.trim() : ''
  const resolved = isFileLink(target) ? getDownloadUrl(resolveMarkdownPath(basePath, target)) : target
  return <img {...rest} src={resolved} />
}

function Markdown({ content, basePath = '/', onOpenPath }: MarkdownProps) {
  return (
    <div className="chrote-markdown">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{
          a: props => <MarkdownLink {...props} basePath={basePath} onOpenPath={onOpenPath} />,
          img: props => <MarkdownImage {...props} basePath={basePath} />,
        }}
      >
        {content}
      </ReactMarkdown>
    </div>
  )
}

export default Markdown
import './Markdown.css'
