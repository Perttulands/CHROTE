/**
 * The page being read beside the sky: a one-line head, the prose at a reading
 * measure, and where to go from it. The trail, the history and the edit are
 * there when asked for.
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
    open, close, save, startEdit, discard, setDraft, setSummary, toggleHistory: () => setHistoryOpen(current => !current),
  }
}

export type PageReaderState = ReturnType<typeof usePageReader>

interface PageReaderProps {
  reader: PageReaderState & { page: LibraryPageContent }
  diveRef: RefObject<HTMLElement>
  titleOf: (path: string) => string
  summaryOf: (path: string) => string
  /** Where a page sits, in words: its subject. */
  whereOf: (path: string) => string
  /** Every page one hop away, by title. */
  related: { path: string; title: string }[]
  resolveWikiLink: (target: string) => string | undefined
  onSend: () => void
}

export default function PageReader({ reader, diveRef, titleOf, summaryOf, whereOf, related, resolveWikiLink, onSend }: PageReaderProps) {
  const { page, trail, draft, historyOpen, discarding, summary } = reader
  const where = whereOf(page.path)

  return (
    <aside className="library-dive" data-ui="library.dive" ref={diveRef} aria-label={`Reading ${page.title}`}>
      <header className="library-dive-head">
        <div className="library-page-title-row">
          <h1 title={page.title}>{page.title}</h1>
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
            <button type="button" className="library-action" onClick={onSend}>Send</button>
            <button type="button" className="library-action library-dive-close" onClick={reader.close}>
              Close<span className="library-chord">Esc</span>
            </button>
          </div>
        </div>
        <p className="library-page-meta">
          {where && <span>{where}</span>}
          <span>{page.updated ? `changed ${libraryWhen(page.updated)} by ${page.author}` : 'never committed'}</span>
          {page.history.length > 0 && (
            <button type="button" className="library-history-more" aria-expanded={historyOpen} onClick={reader.toggleHistory}>
              History
            </button>
          )}
        </p>
        {/* Where this dive has been, once it has been anywhere. The last step
            is the page in hand; every earlier one is the way back to it. */}
        {trail.length > 1 && (
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
          </nav>
        )}
      </header>
      {historyOpen && (
        <div className="library-history-strip">
          {page.history.map(entry => (
            <span key={entry.hash} className="library-history">
              <span className="library-history-when">{libraryWhen(entry.time)}</span>
              <span className="library-history-message">{entry.message}</span>
              <span className="library-history-hash">{entry.hash.slice(0, 7)}</span>
            </span>
          ))}
        </div>
      )}
      {draft === null ? (
        <div className="library-page">
          <div className="library-body">
            {summaryOf(page.path) && <p className="library-page-summary">{summaryOf(page.path)}</p>}
            <Markdown
              content={libraryProse(page.content, page.title)}
              basePath={`/${page.path}`}
              onOpenPath={path => reader.open(path.replace(/^\//, ''))}
              resolveWikiLink={resolveWikiLink}
            />
            {related.length > 0 && (
              <div className="library-links">
                <h2>Related</h2>
                <ul>
                  {related.map(link => (
                    <li key={link.path}>
                      <button type="button" className="library-link" onClick={() => reader.open(link.path)}>{link.title}</button>
                    </li>
                  ))}
                </ul>
              </div>
            )}
          </div>
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
    </aside>
  )
}
