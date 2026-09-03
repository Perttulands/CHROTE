/**
 * One shelf's pages, listed: the step between "what is on this shelf" and
 * "read this".
 */

import { libraryWhen, type LibraryPage } from '../../library/libraryApi'

interface ShelfProps {
  shelf: string
  pages: LibraryPage[]
  onOpenPage: (path: string) => void
}

export default function Shelf({ shelf, pages, onOpenPage }: ShelfProps) {
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
