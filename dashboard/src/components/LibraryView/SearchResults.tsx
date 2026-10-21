/**
 * The brain's ranked answer to a search, as a list the operator reads down:
 * each page by title and kind, its summary, the words that matched and where
 * it lives. The list stands while a page is read beside it.
 */

import { Fragment, type RefObject } from 'react'
import type { MenuGroup } from '../Menu'
import MenuTarget from '../MenuTarget'
import type { LibrarySearchResult, LibrarySearchScope } from '../../library/libraryApi'

/** What the scope buttons offer; the brain's default leaves Self out. */
const SCOPES: { scope: LibrarySearchScope; label: string; title: string }[] = [
  { scope: '', label: 'Brain', title: 'Knowledge, evidence and projects' },
  { scope: 'self', label: 'Self', title: 'Only what is about the operator' },
  { scope: 'all', label: 'All', title: 'Every collection' },
]

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
  results: LibrarySearchResult[]
  selected: number
  openPath: string | null
  scope: LibrarySearchScope
  listRef: RefObject<HTMLDivElement>
  menu: (path: string) => () => MenuGroup[]
  onScope: (scope: LibrarySearchScope) => void
  onOpen: (index: number, path: string) => void
  /** Show the answer as lit stars on the map. */
  onMap: () => void
}

export default function SearchResults({ results, selected, openPath, scope, listRef, menu, onScope, onOpen, onMap }: SearchResultsProps) {
  return (
    <div className="library-search-results" data-ui="library.results">
      <div className="library-search-head">
        <span className="library-search-count">
          {results.length === 0 ? 'Nothing found' : `${results.length} ${results.length === 1 ? 'page' : 'pages'}`}
          {results.length >= BRAIN_LIMIT ? ', best first' : ''}
        </span>
        {results.length > 0 && <button type="button" className="library-scope" onClick={onMap}>On the map</button>}
        <div className="library-search-scope" role="group" aria-label="Collections searched">
          {SCOPES.map(entry => (
            <button
              key={entry.scope}
              type="button"
              className="library-scope"
              aria-pressed={scope === entry.scope}
              title={entry.title}
              onClick={() => onScope(entry.scope)}
            >
              {entry.label}
            </button>
          ))}
        </div>
      </div>
      <div className="library-results" ref={listRef}>
        {results.map((result, index) => (
          <MenuTarget key={result.path} label={`Actions for ${result.path}`} groups={menu(result.path)}>
            <button
              type="button"
              data-result={index}
              className={`library-result${index === selected ? ' selected' : ''}${openPath === result.path ? ' open' : ''}`}
              onClick={() => onOpen(index, result.path)}
            >
              <span className="library-result-head">
                <span className="library-result-title">{result.title}</span>
                <span className={`library-result-kind kind-${result.kind}`}>{result.kind}</span>
              </span>
              {result.summary && <span className="library-result-summary">{result.summary}</span>}
              {result.snippet && <span className="library-result-snippet">{marked(result.snippet)}</span>}
              <span className="library-result-path">{result.path}</span>
            </button>
          </MenuTarget>
        ))}
      </div>
    </div>
  )
}
