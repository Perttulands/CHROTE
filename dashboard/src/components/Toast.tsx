/**
 * The toast: what CHROTE just said, for as long as it takes to read.
 *
 * It sits in the bottom-centre slot with the key echo, fades in, holds, and
 * fades out, one at a time: a newer announcement replaces the one up and
 * restarts the hold. The status line keeps the same event as the record, so
 * nothing is lost when the toast is gone, and the toast is the only thing here
 * that moves.
 */

import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { useStatus, type StatusEvent } from '../context/StatusContext'
import './Toast.css'

export const TOAST_FADE_IN_MS = 120
export const TOAST_HOLD_MS = 1800
export const TOAST_FADE_OUT_MS = 200

export default function Toast() {
  const { status } = useStatus()
  const [shown, setShown] = useState<StatusEvent | null>(null)
  const [raised, setRaised] = useState(false)
  const node = useRef<HTMLDivElement>(null)

  // A new event goes up at once and starts the hold over, whether the slot is
  // empty, holding, or already on its way out. Information (a load, a count)
  // takes the status line only: the toast confirms the operator's own action
  // or reports a failure.
  useEffect(() => {
    if (status === null || status.severity === 'info') return
    setShown(status)
    const hold = setTimeout(() => setRaised(false), TOAST_FADE_IN_MS + TOAST_HOLD_MS)
    return () => clearTimeout(hold)
  }, [status])

  // The fade-in is a CSS transition, and a transition needs a start state the
  // browser has actually laid out. Reading offsetWidth forces that layout on
  // the hidden toast before the class that raises it lands.
  useLayoutEffect(() => {
    if (shown === null) return
    if (node.current) void node.current.offsetWidth
    setRaised(true)
  }, [shown])

  useEffect(() => {
    if (raised || shown === null) return
    const gone = setTimeout(() => setShown(null), TOAST_FADE_OUT_MS)
    return () => clearTimeout(gone)
  }, [raised, shown])

  if (shown === null) return null

  const failure = shown.severity === 'error'
  const className = ['toast', raised ? 'toast-raised' : '', failure ? 'toast-failure' : ''].filter(Boolean).join(' ')

  // The status line is the accessible record of the same event; a second live
  // region would read every announcement twice.
  return (
    <div ref={node} className={className} data-ui="toast" aria-hidden="true">
      {shown.message}
    </div>
  )
}
