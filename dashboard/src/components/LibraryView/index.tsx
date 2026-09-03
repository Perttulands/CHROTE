/**
 * The Library: a reading room over the operator's own context corpus.
 *
 * Three columns and a desk. The shelves, what arrived lately and the proposals
 * in flight down the left; the page itself in the middle at a reading measure;
 * its history and the rest of its shelf at the right; the Librarian on duty at
 * the foot. The corpus is a Markdown tree under git, so every fact here — a
 * title, a date, an author, a history — is read out of that tree rather than
 * kept anywhere in CHROTE. The one thing written back is the operator's own
 * correction, and it is a commit signed as him.
 */

import { useCallback, useEffect, useMemo, useState } from 'react'
import Desk from '../Desk'
import Editor from '../Editor'
import Markdown from '../Markdown'
import ReadingRoom from './ReadingRoom'
import { useSession } from '../../context/SessionContext'
import { useStatus } from '../../context/StatusContext'
import { openBeadCard } from '../../beads/beadCard'
import { fetchBeadWork, type BeadRow } from '../../beads/beadsApi'
import { isBeadClosed } from '../../beads/beadStatus'
import { registerChords } from '../../keys/chords'
import TableColumn from '../TableColumn'
import {
  fetchChanges,
  fetchPage,
  fetchShelfPages,
  fetchShelves,
  libraryProse,
  libraryWhen,
  savePage,
  searchLibrary,
  shelfOf,
  type LibraryChange,
  type LibraryPage,
  type LibraryPageContent,
  type LibrarySearchResult,
  type LibraryShelves,
} from '../../library/libraryApi'
import './LibraryView.css'

/** How many commits the New arrivals column reads. */
const ARRIVALS = 30

/** What a corpus calls its own front matter, in the order the room tries. */
const LIBRARY_FRONT_MATTER = ['README.md', 'CLAUDE.md']

/** The one line the desk and the drawer carry: which page is on the table. */
export function libraryReference(path: string | null): string {
  return path ? `library ${path}` : 'library'
}

export default function LibraryView() {
  const { openSendToSession } = useSession()
  const { announce } = useStatus()

  const [shelves, setShelves] = useState<LibraryShelves | null>(null)
  const [changes, setChanges] = useState<LibraryChange[]>([])
  const [proposals, setProposals] = useState<BeadRow[]>([])
  const [error, setError] = useState<string | null>(null)
  // What git said when it would not read the corpus. Without it a refused
  // repository looks exactly like a library nobody has written in.
  const [gitError, setGitError] = useState('')

  const [query, setQuery] = useState('')
  const [results, setResults] = useState<LibrarySearchResult[] | null>(null)
  const [shelf, setShelf] = useState<string | null>(null)
  const [shelfPages, setShelfPages] = useState<LibraryPage[]>([])
  const [page, setPage] = useState<LibraryPageContent | null>(null)

  const [draft, setDraft] = useState<string | null>(null)
  const [summary, setSummary] = useState('')
  const [discarding, setDiscarding] = useState(false)

  useEffect(() => {
    let current = true
    fetchShelves()
      .then(found => {
        if (!current) return
        setShelves(found)
        if (!found.root) return
        const pages = found.shelves.reduce((total, entry) => total + entry.pages, 0)
        announce(`Library loaded · ${pages} pages on ${found.shelves.length} shelves`, 'info')
      })
      .catch((cause: unknown) => {
        if (!current) return
        setError(cause instanceof Error ? cause.message : 'The library did not answer')
      })
    return () => { current = false }
  }, [announce])

  const root = shelves?.root ?? ''

  useEffect(() => {
    if (!root) return
    let current = true
    fetchChanges(ARRIVALS)
      .then(found => {
        if (!current) return
        setChanges(found.changes)
        setGitError(found.error ?? '')
      })
      .catch(() => { if (current) setChanges([]) })
    return () => { current = false }
  }, [root])

  // Proposals are open Beads of the configured store. Without one the shelf
  // does not exist, rather than standing empty.
  const beadsProject = shelves?.beadsProject ?? ''
  useEffect(() => {
    if (!beadsProject) return
    let current = true
    fetchBeadWork(beadsProject)
      .then(work => { if (current) setProposals(work.beads.filter(bead => !isBeadClosed(bead.status))) })
      .catch(() => { if (current) setProposals([]) })
    return () => { current = false }
  }, [beadsProject])

  const openPage = useCallback((path: string) => {
    setResults(null)
    setDraft(null)
    fetchPage(path)
      .then(found => {
        setPage(found)
        setGitError(found.error ?? '')
        setShelf(shelfOf(found.path) || null)
      })
      .catch((cause: unknown) => {
        announce(cause instanceof Error ? cause.message : `Could not open ${path}`, 'error')
      })
  }, [announce])

  const openShelf = useCallback((name: string) => {
    setResults(null)
    setPage(null)
    setDraft(null)
    setShelf(name)
  }, [])

  // A corpus that introduces itself is read rather than described: its own
  // README or CLAUDE.md is the page the room opens on. A corpus with neither
  // gets the shelves as cards, and the miss is not worth a status line.
  useEffect(() => {
    if (!root) return
    let current = true
    const introduce = async () => {
      for (const candidate of LIBRARY_FRONT_MATTER) {
        try {
          const found = await fetchPage(candidate)
          if (!current) return
          setPage(found)
          return
        } catch {
          // The next candidate, then the cards.
        }
      }
    }
    void introduce()
    return () => { current = false }
  }, [root])

  // The right column is always the shelf the reader is on, whether he chose it
  // or arrived through a page.
  useEffect(() => {
    if (!shelf) {
      setShelfPages([])
      return
    }
    let current = true
    fetchShelfPages(shelf)
      .then(found => {
        if (!current) return
        setShelfPages(found.pages)
        setGitError(found.error ?? '')
      })
      .catch(() => { if (current) setShelfPages([]) })
    return () => { current = false }
  }, [shelf])

  const runSearch = useCallback(() => {
    const asked = query.trim()
    if (!asked) {
      setResults(null)
      return
    }
    searchLibrary(asked)
      .then(found => {
        setResults(found)
        announce(`${found.length} ${found.length === 1 ? 'page mentions' : 'pages mention'} "${asked}"`, 'info')
      })
      .catch((cause: unknown) => {
        announce(cause instanceof Error ? cause.message : 'The search failed', 'error')
      })
  }, [announce, query])

  const send = useCallback(() => {
    openSendToSession({ reference: libraryReference(page?.path ?? shelf) })
  }, [openSendToSession, page, shelf])

  // Alt+S sends what is on the table. It is registered only while the tab is
  // mounted, so it never shadows the tile chord it borrows the key from.
  useEffect(() => registerChords([{
    id: 'library.send',
    key: 's',
    direct: { alt: true, shift: false, key: 's' },
    label: 'Send this page',
    scope: 'global',
    run: send,
  }]), [send])

  const save = useCallback(async () => {
    if (!page || draft === null) return
    const message = summary.trim() || `Edit ${page.title}`
    try {
      const commit = await savePage(page.path, draft, message)
      setDraft(null)
      setPage({ ...page, content: draft, updated: commit.time, author: commit.author, history: [commit, ...page.history] })
      announce(`Shelved ${page.path} · ${commit.message}`, 'success')
    } catch (cause: unknown) {
      announce(cause instanceof Error ? cause.message : 'The library refused the save', 'error')
    }
  }, [announce, draft, page, summary])

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

  const arrivalsByShelf = useMemo(() => {
    const last = new Map<string, LibraryChange>()
    changes.forEach(change => {
      change.files.forEach(file => {
        const name = shelfOf(file)
        if (name && !last.has(name)) last.set(name, change)
      })
    })
    return last
  }, [changes])

  if (error) return <div className="library-view"><p className="library-error">{error}</p></div>
  if (!shelves) return <div className="library-view"><p className="library-empty">Opening the library…</p></div>
  if (!shelves.root) return <div className="library-view"><p className="library-empty">No library is configured</p></div>

  const totalPages = shelves.shelves.reduce((total, entry) => total + entry.pages, 0)

  return (
    <div className="library-view">
      <div className="library-columns">
        <div className="library-left" data-ui="library.shelves">
          <input
            className="library-search"
            type="search"
            value={query}
            placeholder="Search the library…"
            aria-label="Search the library"
            onChange={event => setQuery(event.target.value)}
            onKeyDown={event => { if (event.key === 'Enter') runSearch() }}
          />

          <section className="library-section">
            <h3>Shelves</h3>
            {shelves.shelves.map(entry => (
              <button
                key={entry.path}
                type="button"
                className={`library-shelf ${shelf === entry.name ? 'active' : ''}`}
                onClick={() => openShelf(entry.name)}
              >
                <span className="library-shelf-name">{entry.name}</span>
                <span className="library-shelf-count">{entry.pages}</span>
              </button>
            ))}
          </section>

          <section className="library-section library-yields">
            <h3>New arrivals</h3>
            <div className="library-scroll">
              {gitError && <p className="library-git-error">{gitError}</p>}
              {changes.length === 0 && <p className="library-empty">Nothing has arrived yet.</p>}
              {changes.map(change => (
                <button
                  key={change.hash}
                  type="button"
                  className="library-arrival"
                  disabled={change.files.length === 0}
                  onClick={() => { if (change.files[0]) openPage(change.files[0]) }}
                >
                  <span className="library-arrival-when">{libraryWhen(change.time)} · {change.author}</span>
                  <span className="library-arrival-message">{change.message}</span>
                </button>
              ))}
            </div>
          </section>

          {shelves.beadsProject && (
            <section className="library-section">
              <h3>Proposals</h3>
              <div className="library-scroll">
                {proposals.length === 0 && <p className="library-empty">Nothing is in flight.</p>}
                {proposals.map(bead => (
                  <button
                    key={bead.id}
                    type="button"
                    className="library-proposal"
                    onClick={() => openBeadCard(bead.id, shelves.beadsProject)}
                  >
                    <span className="library-proposal-id">{bead.id}</span>
                    <span className="library-proposal-title">{bead.title}</span>
                  </button>
                ))}
              </div>
            </section>
          )}
        </div>

        <div className="library-room" data-ui="library.room">
          {results !== null ? (
            <div className="library-page">
              <div className="library-page-head">
                <div className="library-page-title-row">
                  <h1>Search</h1>
                </div>
                <p className="library-page-meta">
                  {results.length} {results.length === 1 ? 'page mentions' : 'pages mention'} “{query.trim()}”
                </p>
                <div className="library-rule" />
              </div>
              <div className="library-results">
                {results.map(result => (
                  <button key={result.path} type="button" className="library-result" onClick={() => openPage(result.path)}>
                    <span className="library-result-title">{result.title}</span>
                    <span className="library-result-path">{result.path}{result.line > 0 ? `:${result.line}` : ''}</span>
                    {result.snippet && <span className="library-result-snippet">{result.snippet}</span>}
                  </button>
                ))}
              </div>
            </div>
          ) : page ? (
            <div className="library-page">
              <div className="library-page-head">
                <div className="library-page-title-row">
                  <h1>{page.title}</h1>
                  <div className="library-page-actions">
                    {draft === null ? (
                      <button type="button" className="library-action" onClick={startEdit}>Edit</button>
                    ) : (
                      <>
                        <button type="button" className="library-action library-action-primary" onClick={() => void save()}>Save</button>
                        <button type="button" className="library-action" onClick={discard}>
                          {discarding ? 'Confirm' : 'Discard'}
                        </button>
                      </>
                    )}
                    <button type="button" className="library-action" onClick={send}>
                      Send<span className="library-chord">Alt+S</span>
                    </button>
                  </div>
                </div>
                <p className="library-page-meta">
                  {page.path}
                  {page.updated ? ` · changed ${libraryWhen(page.updated)} by ${page.author}` : ' · never committed'}
                </p>
                <div className="library-rule" />
              </div>
              {draft === null ? (
                <div className="library-body">
                  <Markdown
                    content={libraryProse(page.content, page.title)}
                    basePath={`/${page.path}`}
                    onOpenPath={path => openPage(path.replace(/^\//, ''))}
                  />
                </div>
              ) : (
                <div className="library-editor">
                  <input
                    className="library-summary"
                    type="text"
                    value={summary}
                    aria-label="What this edit changes"
                    onChange={event => setSummary(event.target.value)}
                  />
                  <Editor
                    value={draft}
                    onChange={setDraft}
                    onSave={() => void save()}
                    onCancel={discard}
                    label={`Editing ${page.path}`}
                    autoFocus
                  />
                </div>
              )}
            </div>
          ) : (
            <ReadingRoom
              shelves={shelves.shelves}
              shelf={shelf}
              pages={shelfPages}
              totalPages={totalPages}
              lastChange={changes[0] ?? null}
              arrivalsByShelf={arrivalsByShelf}
              onOpenShelf={openShelf}
              onOpenPage={openPage}
            />
          )}
        </div>

        <div className="library-right" data-ui="library.history">
          <section className="library-section">
            <h3>History</h3>
            {!page && <p className="library-empty">No page is open.</p>}
            {page?.history.length === 0 && <p className="library-empty">This page is not committed yet.</p>}
            {page?.history.map(entry => (
              <div key={entry.hash} className="library-history">
                <span className="library-history-head">
                  <span className="library-history-hash">{entry.hash.slice(0, 7)}</span>
                  <span className="library-history-when">{libraryWhen(entry.time)}</span>
                  <span className="library-history-author">{entry.author}</span>
                </span>
                <span className="library-history-message">{entry.message}</span>
              </div>
            ))}
          </section>

          <section className="library-section library-yields">
            <h3>{shelf ? `On ${shelf}` : 'On this shelf'}</h3>
            <div className="library-scroll">
              {!shelf && <p className="library-empty">Pick a shelf or a card.</p>}
              {shelfPages.map(sibling => (
                <button
                  key={sibling.path}
                  type="button"
                  className={`library-sibling ${page?.path === sibling.path ? 'active' : ''}`}
                  onClick={() => openPage(sibling.path)}
                >
                  <span className="library-sibling-title">{sibling.title}</span>
                  <span className="library-sibling-when">{libraryWhen(sibling.updated)}</span>
                </button>
              ))}
            </div>
          </section>
        </div>

        <TableColumn />
      </div>

      <Desk
        label="Front desk"
        sessionName={shelves.librarianSession || undefined}
        reference={libraryReference(page?.path ?? shelf)}
        placeholder={page ? 'Ask the Librarian about this page…' : 'Ask the Librarian…'}
        launchFolder={shelves.root}
      />
    </div>
  )
}
