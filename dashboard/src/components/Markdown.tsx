/**
 * Markdown rendered in CHROTE's theme.
 *
 * One renderer serves every surface that shows written text: the Files
 * panel's viewer, the Bead card, and the Library. It is react-markdown with
 * GitHub flavour, so tables, task lists and strikethrough behave the way the
 * operator writes them, and the theme is CSS — `.chrote-markdown` in
 * Markdown.css — not a component per element.
 *
 * Two behaviours are not styling. A link to another file opens in the surface
 * the reader is already in: a relative or absolute filesystem link is resolved
 * against the document's own path and handed to `onOpenPath`, so following a
 * citation does not throw the reader out of the panel. And bare tokens the
 * host recognises — a Bead id inside a Bead's own text — become controls the
 * host handles, without the writer having had to mark them up; those links
 * carry a fragment rather than a scheme of their own, so react-markdown's URL
 * sanitising stays in force for everything an agent wrote. Everything else is
 * an ordinary external link, and a link CHROTE will not follow (`javascript:`
 * and its relatives) is drawn as plain text.
 */

import { useMemo } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import type { AnchorHTMLAttributes, ImgHTMLAttributes, MouseEvent as ReactMouseEvent, ReactNode } from 'react'
import { getDownloadUrl } from './FilesView/fileService'

export interface MarkdownProps {
  /** The Markdown source. */
  content: string
  /** The document's own absolute path, so relative links can be resolved. */
  basePath?: string
  /** Where a link to another file goes. Without it every file link is drawn as text. */
  onOpenPath?: (path: string) => void
  /** Bare tokens matching this become controls the host opens. */
  tokenPattern?: RegExp
  onToken?: (token: string) => void
  className?: string
}

/** Schemes CHROTE follows. Anything else is drawn as text, never as a link. */
const SAFE_SCHEME = /^(https?:|mailto:)/i
const HAS_SCHEME = /^[a-z][a-z0-9+.-]*:/i
const TOKEN_HREF_PREFIX = '#token-'

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

interface MdastNode {
  type: string
  value?: string
  url?: string
  children?: MdastNode[]
}

/** Split the text nodes of a tree on a pattern, linking what matched. */
function linkifyTokens(pattern: RegExp) {
  const global = new RegExp(pattern.source, pattern.flags.includes('g') ? pattern.flags : `${pattern.flags}g`)
  const split = (node: MdastNode): MdastNode[] => {
    const text = node.value ?? ''
    global.lastIndex = 0
    const parts: MdastNode[] = []
    let cursor = 0
    for (const match of text.matchAll(global)) {
      if (match.index === undefined) continue
      if (match.index > cursor) parts.push({ type: 'text', value: text.slice(cursor, match.index) })
      parts.push({
        type: 'link',
        url: `${TOKEN_HREF_PREFIX}${match[0]}`,
        children: [{ type: 'text', value: match[0] }],
      })
      cursor = match.index + match[0].length
    }
    if (parts.length === 0) return [node]
    if (cursor < text.length) parts.push({ type: 'text', value: text.slice(cursor) })
    return parts
  }
  const walk = (node: MdastNode): void => {
    if (!node.children) return
    // A token inside a link is already a link, and one inside code is text the
    // writer asked to be left alone.
    if (node.type === 'link' || node.type === 'linkReference') return
    node.children = node.children.flatMap(child => (child.type === 'text' ? split(child) : [child]))
    node.children.forEach(walk)
  }
  return () => (tree: MdastNode) => { walk(tree) }
}

function MarkdownLink(
  { href, children, basePath, onOpenPath, onToken, ...rest }: AnchorHTMLAttributes<HTMLAnchorElement> & {
    basePath: string
    onOpenPath?: (path: string) => void
    onToken?: (token: string) => void
  },
) {
  const target = (href || '').trim()
  if (target.startsWith(TOKEN_HREF_PREFIX)) {
    const token = target.slice(TOKEN_HREF_PREFIX.length)
    return (
      <button type="button" className="chrote-markdown-token" onClick={() => onToken?.(token)}>
        {children as ReactNode}
      </button>
    )
  }
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

function Markdown({ content, basePath = '/', onOpenPath, tokenPattern, onToken, className }: MarkdownProps) {
  const plugins = useMemo(
    () => (tokenPattern ? [remarkGfm, linkifyTokens(tokenPattern)] : [remarkGfm]),
    [tokenPattern],
  )
  return (
    <div className={className ? `chrote-markdown ${className}` : 'chrote-markdown'}>
      <ReactMarkdown
        remarkPlugins={plugins}
        components={{
          a: props => <MarkdownLink {...props} basePath={basePath} onOpenPath={onOpenPath} onToken={onToken} />,
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
