/**
 * One shelf's pages, listed: the step between "what is on this shelf" and
 * "read this".
 */

import { libraryWhen, type LibraryPage } from '../../library/libraryApi'
import type { MenuGroup } from '../Menu'
import MenuTarget from '../MenuTarget'

interface ShelfProps {
  shelf: string
  pages: LibraryPage[]
  onOpenPage: (path: string) => void
  /** The rows a page's menu offers, the same ones wherever the page is listed. */
  pageMenu: (path: string) => () => MenuGroup[]
}

export default function Shelf({ shelf, pages, onOpenPage, pageMenu }: ShelfProps) {
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
          <MenuTarget key={page.path} label={`Actions for ${page.path}`} groups={pageMenu(page.path)}>
            <button type="button" className="library-result" onClick={() => onOpenPage(page.path)}>
              <span className="library-result-title">{page.title}</span>
              <span className="library-result-path">
                {page.path}{page.updated ? ` · changed ${libraryWhen(page.updated)} by ${page.author}` : ''}
              </span>
            </button>
          </MenuTarget>
        ))}
        {pages.length === 0 && <p className="library-empty">This shelf is empty.</p>}
      </div>
    </div>
  )
}
