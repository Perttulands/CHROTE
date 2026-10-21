/**
 * The page being read beside the map: the dive's trail, the page with its
 * history, and the operator's own edit of it.
 *
 * Diving into a page adds it to the trail; a page the dive has already passed
 * through is not visited twice: the trail is cut back to it, which is what a
 * step back means. A dive ends where it was opened, and the map stays exactly
 * where the operator left it.
 */

import { Fragment, useCallback, useState } from 'react'
import type { RefObject } from 'react'
import Editor from '../Editor'
import Markdown from '../Markdown'
import { fetchPage, libraryProse, libraryWhen, savePage, type LibraryPageContent } from '../../library/libraryApi'

/** How many commits the history strip shows before "… n more". */
const HISTORY_STRIP = 3

/** What a page is opened for: reading, editing, or its history unfolded. */
export type OpenIntent = 'read' | 'edit' | 'history'

type Announce = (message: string, level: 'error' | 'success') => void

export function usePageReader(announce: Announce, onSaved: () => void, onGitError: (error: string) => void) {
  const [page, setPage] = useState<LibraryPageContent | null>(null)
  // Every page this dive has passed through, oldest first; the last of them is
  // the page being read. Empty means no dive is open.
  const [trail, setTrail] = useState<string[]>([])
  const [historyOpen, setHistoryOpen] = useState(false)
  const [draft, setDraft] = useState<string | null>(null)
  const [summary, setSummary] = useState('')
  const [discarding, setDiscarding] = useState(false)

  // A row's menu can ask for the page already in the editor or with its whole
  // history unfolded; the page arrives in that state rather than a step short.
  const open = useCallback((path: string, intent: OpenIntent = 'read') => {
    setDraft(null)
    fetchPage(path)
      .then(found => {
        setPage(found)
        setTrail(current => {
          const at = current.indexOf(found.path)
          return at === -1 ? [...current, found.path] : current.slice(0, at + 1)
        })
        setHistoryOpen(intent === 'history')
        onGitError(found.error ?? '')
        if (intent === 'edit') {
          setDraft(found.content)
          setSummary(`Edit ${found.title}`)
          setDiscarding(false)
        }
      })
      .catch((cause: unknown) => {
        announce(cause instanceof Error ? cause.message : `Could not open ${path}`, 'error')
      })
  }, [announce, onGitError])

  const close = useCallback(() => {
    setPage(null)
    setTrail([])
    setDraft(null)
    setHistoryOpen(false)
    setDiscarding(false)
  }, [])

  const save = useCallback(async () => {
    if (!page || draft === null) return
    const message = summary.trim() || `Edit ${page.title}`
    try {
      const commit = await savePage(page.path, draft, message)
      setDraft(null)
      setPage({ ...page, content: draft, updated: commit.time, author: commit.author, history: [commit, ...page.history] })
      announce(`Shelved ${page.path} · ${commit.message}`, 'success')
      onSaved()
    } catch (cause: unknown) {
      announce(cause instanceof Error ? cause.message : 'The library refused the save', 'error')
    }
  }, [announce, draft, onSaved, page, summary])

  const startEdit = useCallback(() => {
    if (!page) return
    setDraft(page.content)
    setSummary(`Edit ${page.title}`)
    setDiscarding(false)
  }, [page])

  // Discarding an edit asks in the control that does it: a second press within
  // the operator's own attention span throws the draft away.
  const discard = useCallback(() => {
    if (!discarding) {
      setDiscarding(true)
      window.setTimeout(() => setDiscarding(false), 3000)
      return
    }
    setDraft(null)
    setDiscarding(false)
  }, [discarding])

  return {
    page, trail, historyOpen, draft, summary, discarding,
    open, close, save, startEdit, discard, setDraft, setSummary, unfoldHistory: () => setHistoryOpen(true),
  }
}

export type PageReaderState = ReturnType<typeof usePageReader>

interface PageReaderProps {
  reader: PageReaderState & { page: LibraryPageContent }
  diveRef: RefObject<HTMLElement>
  titleOf: (path: string) => string
  summaryOf: (path: string) => string
  neighbours: { path: string; title: string }[]
  backlinks: { path: string; title: string }[]
  resolveWikiLink: (target: string) => string | undefined
  onSend: () => void
}

export default function PageReader({ reader, diveRef, titleOf, summaryOf, neighbours, backlinks, resolveWikiLink, onSend }: PageReaderProps) {
  const { page, trail, draft, historyOpen, discarding, summary } = reader

  /** A list of pages as links that travel, the way both link lists read. */
  const linkList = (heading: string, links: { path: string; title: string }[]) => (
    <div className="library-links">
      <h2>{heading}</h2>
      <p>
        {links.map((link, index) => (
          <Fragment key={link.path}>
            {index > 0 && <span className="library-linked-sep"> · </span>}
            <button type="button" className="library-link" onClick={() => reader.open(link.path)}>{link.title}</button>
          </Fragment>
        ))}
      </p>
    </div>
  )

  return (
    <aside className="library-dive" data-ui="library.dive" ref={diveRef} aria-label={`Reading ${page.title}`}>
      {/* Where this dive has been. The last step is the page in hand;
          every earlier one is the way back to it. */}
      <nav className="library-trail" aria-label="This dive">
        {trail.map((path, index) => (
          <Fragment key={path}>
            {index > 0 && <span className="library-trail-sep">›</span>}
            <button
              type="button"
              className="library-trail-step"
              aria-current={path === page.path ? 'page' : undefined}
              onClick={() => reader.open(path)}
            >
              {titleOf(path)}
            </button>
          </Fragment>
        ))}
        <button type="button" className="library-action library-dive-close" onClick={reader.close}>
          Close<span className="library-chord">Esc</span>
        </button>
      </nav>
      <div className="library-page">
        <div className="library-page-head">
          <div className="library-page-title-row">
            <h1>{page.title}</h1>
            <div className="library-page-actions">
              {draft === null ? (
                <button type="button" className="library-action" onClick={reader.startEdit}>Edit</button>
              ) : (
                <>
                  <button type="button" className="library-action library-action-primary" onClick={() => void reader.save()}>Save</button>
                  <button type="button" className="library-action" onClick={reader.discard}>
                    {discarding ? 'Confirm' : 'Discard'}
                  </button>
                </>
              )}
              <button type="button" className="library-action" onClick={onSend}>
                Send
              </button>
            </div>
          </div>
          {summaryOf(page.path) && <p className="library-page-summary">{summaryOf(page.path)}</p>}
          <p className="library-page-meta">
            {page.path}
            {page.updated ? ` · changed ${libraryWhen(page.updated)} by ${page.author}` : ' · never committed'}
          </p>
          <div className="library-history-strip">
            {page.history.length === 0 && <span className="library-history-none">This page is not committed yet.</span>}
            {(historyOpen ? page.history : page.history.slice(0, HISTORY_STRIP)).map(entry => (
              <span key={entry.hash} className="library-history">
                <span className="library-history-hash">{entry.hash.slice(0, 7)}</span>
                <span className="library-history-when">{libraryWhen(entry.time)}</span>
                <span className="library-history-message">{entry.message}</span>
              </span>
            ))}
            {!historyOpen && page.history.length > HISTORY_STRIP && (
              <button type="button" className="library-history-more" onClick={reader.unfoldHistory}>
                … {page.history.length - HISTORY_STRIP} more
              </button>
            )}
          </div>
          <div className="library-rule" />
        </div>
        {draft === null ? (
          <div className="library-body">
            <Markdown
              content={libraryProse(page.content, page.title)}
              basePath={`/${page.path}`}
              onOpenPath={path => reader.open(path.replace(/^\//, ''))}
              resolveWikiLink={resolveWikiLink}
            />
            {neighbours.length > 0 && linkList('Neighbours', neighbours)}
            {backlinks.length > 0 && linkList('Linked from', backlinks)}
          </div>
        ) : (
          <div className="library-editor">
            <input
              className="library-summary"
              type="text"
              value={summary}
              aria-label="What this edit changes"
              onChange={event => reader.setSummary(event.target.value)}
            />
            <Editor
              value={draft}
              onChange={reader.setDraft}
              onSave={() => void reader.save()}
              onCancel={reader.discard}
              label={`Editing ${page.path}`}
              autoFocus
            />
          </div>
        )}
      </div>
    </aside>
  )
}
