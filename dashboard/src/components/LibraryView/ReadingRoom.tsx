/**
 * The reading room with nothing on the table yet.
 *
 * Before a page is chosen the middle column is the library itself: its shelves
 * as cards, each with what it holds and what last happened on it. Choose one
 * and the cards give way to that shelf's pages, which is the step between "what
 * is in here" and "read this".
 */

import { libraryWhen, type LibraryChange, type LibraryPage, type LibraryShelf } from '../../library/libraryApi'

interface ReadingRoomProps {
  shelves: LibraryShelf[]
  /** The shelf the reader picked, or null while he is still looking. */
  shelf: string | null
  pages: LibraryPage[]
  totalPages: number
  lastChange: LibraryChange | null
  /** What last touched each shelf, taken from the same log New arrivals reads. */
  arrivalsByShelf: Map<string, LibraryChange>
  onOpenShelf: (name: string) => void
  onOpenPage: (path: string) => void
}

export default function ReadingRoom({
  shelves,
  shelf,
  pages,
  totalPages,
  lastChange,
  arrivalsByShelf,
  onOpenShelf,
  onOpenPage,
}: ReadingRoomProps) {
  if (shelf) {
    return (
      <div className="library-page">
        <div className="library-page-head">
          <div className="library-page-title-row">
            <h1>{shelf}</h1>
          </div>
          <p className="library-page-meta">
            {pages.length} {pages.length === 1 ? 'page' : 'pages'} on this shelf
          </p>
          <div className="library-rule" />
        </div>
        <div className="library-results">
          {pages.map(page => (
            <button key={page.path} type="button" className="library-result" onClick={() => onOpenPage(page.path)}>
              <span className="library-result-title">{page.title}</span>
              <span className="library-result-path">
                {page.path}{page.updated ? ` · changed ${libraryWhen(page.updated)} by ${page.author}` : ''}
              </span>
            </button>
          ))}
          {pages.length === 0 && <p className="library-empty">This shelf is empty.</p>}
        </div>
      </div>
    )
  }

  return (
    <div className="library-page">
      <div className="library-page-head">
        <div className="library-page-title-row">
          <h1>Reading room</h1>
        </div>
        <p className="library-page-meta">
          {totalPages} pages on {shelves.length} {shelves.length === 1 ? 'shelf' : 'shelves'}
          {lastChange ? ` · last change ${libraryWhen(lastChange.time)}` : ''}
        </p>
        <div className="library-rule" />
      </div>
      <div className="library-cards">
        {shelves.map(entry => {
          const change = arrivalsByShelf.get(entry.name)
          return (
            <button key={entry.path} type="button" className="library-card" onClick={() => onOpenShelf(entry.name)}>
              <span className="library-card-name">{entry.name}</span>
              <span className="library-card-count">
                {entry.pages} {entry.pages === 1 ? 'page' : 'pages'}
                {change ? ` · changed ${libraryWhen(change.time)}` : ''}
              </span>
              {change && <span className="library-card-last">{change.message}</span>}
            </button>
          )
        })}
      </div>
    </div>
  )
}
