/**
 * The Brain tab's search: the question, its scope, and the brain's ranked
 * answer, which arrives as the operator types and is replaced only by the
 * answer to the question still being asked.
 */

import { useCallback, useEffect, useRef, useState } from 'react'
import { searchLibrary, type LibrarySearchResult, type LibrarySearchScope } from '../../library/libraryApi'

/** How long typing rests before the brain is asked. */
const SETTLE_MS = 200

export function useBrainSearch(announce: (message: string, level: 'error') => void, open: (path: string) => void) {
  const [query, setQuery] = useState('')
  const [scope, setScope] = useState<LibrarySearchScope>('')
  // Null while there is no query, so the map is the room.
  const [results, setResults] = useState<LibrarySearchResult[] | null>(null)
  // The result the keyboard is on.
  const [selected, setSelected] = useState(0)
  const listRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const asked = query.trim()
    if (!asked) {
      setResults(null)
      return
    }
    let current = true
    const timer = window.setTimeout(() => {
      searchLibrary(asked, scope)
        .then(found => {
          if (!current) return
          setResults(found)
          setSelected(0)
        })
        .catch((cause: unknown) => {
          if (current) announce(cause instanceof Error ? cause.message : 'The search failed', 'error')
        })
    }, SETTLE_MS)
    return () => { current = false; window.clearTimeout(timer) }
  }, [announce, query, scope])

  // An emptied field puts the map back: with the results standing in the
  // room, there is otherwise no way out of them.
  const ask = useCallback((text: string) => {
    setQuery(text)
    if (!text.trim()) setResults(null)
  }, [])

  // Up and Down move through the results and open what they land on; Enter
  // opens the one the keyboard is on.
  const key = useCallback((pressed: string) => {
    if (!results || results.length === 0) return false
    const at = pressed === 'ArrowDown' ? Math.min(selected + 1, results.length - 1)
      : pressed === 'ArrowUp' ? Math.max(selected - 1, 0)
      : pressed === 'Enter' ? selected : -1
    if (at < 0) return false
    setSelected(at)
    open(results[at].path)
    listRef.current?.querySelector(`[data-result="${at}"]`)?.scrollIntoView?.({ block: 'nearest' })
    return true
  }, [open, results, selected])

  const choose = useCallback((index: number, path: string) => {
    setSelected(index)
    open(path)
  }, [open])

  return { query, ask, scope, setScope, results, selected, listRef, key, choose }
}
