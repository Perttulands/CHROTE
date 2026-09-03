/**
 * Confirming in place: the control that does the thing is the control that
 * asks. A destructive button reads its own name, and a first press arms it —
 * the label becomes the confirmation, a second press within three seconds runs
 * it, and walking away disarms it. Nothing is torn out of the operator's way to
 * ask a question he can answer where he is standing.
 */

import { useCallback, useEffect, useState } from 'react'

/** How long an armed control waits for its second press. */
export const CONFIRM_WINDOW_MS = 3000

export function useConfirmInPlace(run: () => void, windowMs: number = CONFIRM_WINDOW_MS) {
  const [armed, setArmed] = useState(false)

  useEffect(() => {
    if (!armed) return
    const timer = setTimeout(() => setArmed(false), windowMs)
    return () => clearTimeout(timer)
  }, [armed, windowMs])

  const press = useCallback(() => {
    if (armed) {
      setArmed(false)
      run()
      return
    }
    setArmed(true)
  }, [armed, run])

  return { armed, press }
}
