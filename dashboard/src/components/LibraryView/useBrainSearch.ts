/**
 * The Brain tab's search: the question and the brain's ranked answer over
 * every collection, which arrives as the operator types and is replaced only by the
 * answer to the question still being asked.
 */

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { searchLibrary, type LibrarySearchResult } from '../../library/libraryApi'

/** How long typing rests before the brain is asked. */
const SETTLE_MS = 200

/** A collection hidden from the sky is left out of the answer too. */
export function useBrainSearch(
  announce: (message: string, level: 'error') => void,
  open: (path: string) => void,
  hidden: ReadonlySet<string>,
) {
  const [query, setQuery] = useState('')
  // Null while there is no query, so the map is the room.
  const [found, setFound] = useState<LibrarySearchResult[] | null>(null)
  const results = useMemo(() => found?.filter(result => !hidden.has(result.collection)) ?? null, [found, hidden])
  // The result the keyboard is on.
  const [selected, setSelected] = useState(0)
  const listRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const asked = query.trim()
    if (!asked) {
      setFound(null)
      return
    }
    let current = true
    const timer = window.setTimeout(() => {
      searchLibrary(asked)
        .then(answer => {
          if (!current) return
          setFound(answer)
          setSelected(0)
        })
        .catch((cause: unknown) => {
          if (current) announce(cause instanceof Error ? cause.message : 'The search failed', 'error')
        })
    }, SETTLE_MS)
    return () => { current = false; window.clearTimeout(timer) }
  }, [announce, query])

  // An emptied field takes the answer off the sky.
  const ask = useCallback((text: string) => {
    setQuery(text)
    if (!text.trim()) setFound(null)
  }, [])

  // Up and Down move through the results and open what they land on; Enter
  // opens the one the keyboard is on; Escape clears the search.
  const key = useCallback((pressed: string) => {
    if (pressed === 'Escape' && query) { ask(''); return true }
    if (!results || results.length === 0) return false
    const at = pressed === 'ArrowDown' ? Math.min(selected + 1, results.length - 1)
      : pressed === 'ArrowUp' ? Math.max(selected - 1, 0)
      : pressed === 'Enter' ? selected : -1
    if (at < 0) return false
    setSelected(at)
    open(results[at].path)
    listRef.current?.querySelector(`[data-result="${at}"]`)?.scrollIntoView?.({ block: 'nearest' })
    return true
  }, [ask, open, query, results, selected])

  const choose = useCallback((index: number, path: string) => {
    setSelected(index)
    open(path)
  }, [open])

  return { query, ask, results, selected, listRef, key, choose }
}
