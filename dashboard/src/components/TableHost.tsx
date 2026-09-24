import { createContext, useCallback, useContext, useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import TableColumn from './TableColumn'
import ErrorBoundary from './ErrorBoundary'

type PlaceTable = (slot: HTMLElement) => () => void

const TablePlacementContext = createContext<PlaceTable | null>(null)

/**
 * The selected object's content lives once, for as long as the dashboard does.
 * A view contributes only a slot. Moving the portal's one container preserves
 * its readers, drafts and scroll; changing the portal target would remount them.
 */
export function TableHost({ children }: { children: ReactNode }) {
  const [container] = useState(() => {
    const element = document.createElement('div')
    element.style.display = 'contents'
    return element
  })
  const parkingRef = useRef<HTMLDivElement>(null)
  const [layoutParent, setLayoutParent] = useState<HTMLElement | null>(null)
  const scrollPositions = useRef(new WeakMap<Element, { top: number; left: number }>())

  const place = useCallback<PlaceTable>(slot => {
    slot.appendChild(container)
    // The old view may already be display:none during this layout effect, so
    // reading its scroll offsets now is too late. Remember actual scrolls and
    // restore them after the same nodes enter their visible destination.
    for (const element of container.querySelectorAll('*')) {
      const position = scrollPositions.current.get(element)
      if (position) {
        element.scrollTop = position.top
        element.scrollLeft = position.left
      }
    }
    setLayoutParent(slot.parentElement)
    return () => {
      // An older slot may retire after a newer one has already claimed it.
      if (container.parentElement !== slot) return
      parkingRef.current?.appendChild(container)
      setLayoutParent(null)
    }
  }, [container])

  useLayoutEffect(() => {
    const rememberScroll = (event: Event) => {
      if (!(event.target instanceof Element) || event.target.getClientRects().length === 0) return
      scrollPositions.current.set(event.target, { top: event.target.scrollTop, left: event.target.scrollLeft })
    }
    container.addEventListener('scroll', rememberScroll, true)
    if (!container.parentElement) parkingRef.current?.appendChild(container)
    return () => {
      container.removeEventListener('scroll', rememberScroll, true)
      container.remove()
    }
  }, [container])

  return (
    <TablePlacementContext.Provider value={place}>
      {children}
      <div ref={parkingRef} hidden />
      {createPortal(
        <ErrorBoundary>
          <TableColumn layoutParent={layoutParent} active={layoutParent !== null} />
        </ErrorBoundary>,
        container,
      )}
    </TablePlacementContext.Provider>
  )
}

/** A view's place for the table; an inactive slot owns no content or requests. */
export function TableSlot({ active = true }: { active?: boolean }) {
  const slotRef = useRef<HTMLDivElement>(null)
  const place = useContext(TablePlacementContext)

  useLayoutEffect(() => {
    if (active && slotRef.current && place) return place(slotRef.current)
  }, [active, place])

  return <div ref={slotRef} style={{ display: 'contents' }} />
}
