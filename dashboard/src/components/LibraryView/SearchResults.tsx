/**
 * The brain's ranked answer to a search, as a narrow list down the sky's
 * left edge: each page by title and kind, its summary and the words that
 * matched. The sky beside it lights what was found, and the page pointed at.
 */

import { Fragment, type RefObject } from 'react'
import type { MenuGroup } from '../Menu'
import MenuTarget from '../MenuTarget'
import type { LibrarySearchResult } from '../../library/libraryApi'

/** The brain returns at most this many; a full list is its best, not all. */
const BRAIN_LIMIT = 50

/** A snippet with the brain's »matched« words marked. */
function marked(snippet: string) {
  return snippet.split(/(»[^«]*«)/).map((part, index) => (
    part.startsWith('»') && part.endsWith('«')
      ? <mark key={index}>{part.slice(1, -1)}</mark>
      : <Fragment key={index}>{part}</Fragment>
  ))
}

interface SearchResultsProps {
  /** Null while the brain has not answered yet. */
  results: LibrarySearchResult[] | null
  selected: number
  openPath: string | null
  listRef: RefObject<HTMLDivElement>
  menu: (path: string) => () => MenuGroup[]
  onOpen: (index: number, path: string) => void
  /** The result pointed at, which the sky lights and brings into view. */
  onHover: (path: string | null) => void
}

export default function SearchResults({ results, selected, openPath, listRef, menu, onOpen, onHover }: SearchResultsProps) {
  return (
    <div className="library-search-results" data-ui="library.results">
      <div className="library-search-head">
        <span className="library-search-count">
          {results === null ? 'Searching…'
            : results.length === 0 ? 'Nothing found'
            : `${results.length} ${results.length === 1 ? 'page' : 'pages'}${results.length >= BRAIN_LIMIT ? ', best first' : ''}`}
        </span>
      </div>
      <div className="library-results" ref={listRef} onMouseLeave={() => onHover(null)}>
        {results?.map((result, index) => (
          <MenuTarget key={result.path} label={`Actions for ${result.path}`} groups={menu(result.path)}>
            <button
              type="button"
              data-result={index}
              className={`library-result${index === selected ? ' selected' : ''}${openPath === result.path ? ' open' : ''}`}
              onClick={() => onOpen(index, result.path)}
              onMouseEnter={() => onHover(result.path)}
              onFocus={() => onHover(result.path)}
              onBlur={() => onHover(null)}
            >
              <span className="library-result-head">
                <span className="library-result-title">{result.title}</span>
                <span className={`library-result-kind kind-${result.kind}`}>{result.kind}</span>
              </span>
              {result.summary && <span className="library-result-summary">{result.summary}</span>}
              {result.snippet && <span className="library-result-snippet">{marked(result.snippet)}</span>}
            </button>
          </MenuTarget>
        ))}
      </div>
    </div>
  )
}
