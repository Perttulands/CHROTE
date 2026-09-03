/**
 * Markdown in the theme.
 *
 * One renderer for every surface that shows written text — a Bead's
 * description, a file in the Files panel, a Library page — so that a heading, a
 * table and a code block look the same wherever the operator meets them.
 *
 * It also linkifies bare tokens: a Bead id inside a Bead's own text is a link
 * to that Bead, without the writer having had to mark it up. Those links carry
 * a fragment rather than a scheme of their own, so react-markdown's URL
 * sanitising stays in force for everything an agent wrote.
 */

import { useMemo } from 'react'
import type { ReactNode } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import './Markdown.css'

const TOKEN_HREF_PREFIX = '#token-'

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

export interface MarkdownProps {
  text: string
  className?: string
  /** Bare tokens matching this become links the host opens. */
  tokenPattern?: RegExp
  onToken?: (token: string) => void
}

export default function Markdown({ text, className, tokenPattern, onToken }: MarkdownProps) {
  const plugins = useMemo(
    () => (tokenPattern ? [remarkGfm, linkifyTokens(tokenPattern)] : [remarkGfm]),
    [tokenPattern],
  )

  return (
    <div className={className ? `markdown ${className}` : 'markdown'}>
      <ReactMarkdown
        remarkPlugins={plugins}
        components={{
          a: ({ href, children }) => {
            const token = href?.startsWith(TOKEN_HREF_PREFIX) ? href.slice(TOKEN_HREF_PREFIX.length) : null
            if (token) {
              return (
                <button type="button" className="markdown-token" onClick={() => onToken?.(token)}>
                  {children as ReactNode}
                </button>
              )
            }
            return <a href={href} target="_blank" rel="noreferrer noopener">{children as ReactNode}</a>
          },
        }}
      >
        {text}
      </ReactMarkdown>
    </div>
  )
}
