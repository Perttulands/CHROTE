/**
 * What CHROTE just took from the keyboard, shown for as long as it takes to
 * read: key caps at the foot of the workspace, and gone.
 *
 * Only a registered chord echoes. Ordinary typing and every unregistered key
 * belong to the program in the terminal, and a badge for them would be a lie
 * about who answered.
 */

import { useEffect, useState } from 'react'
import { useLeader } from './chords'
import './KeyEcho.css'

/** How long the badge stays up after a chord fires. */
export const ECHO_MS = 800

export default function KeyEcho() {
  const { echo } = useLeader()
  const [shown, setShown] = useState<typeof echo>(null)

  useEffect(() => {
    if (echo === null) return
    setShown(echo)
    const timer = setTimeout(() => setShown(null), ECHO_MS)
    return () => clearTimeout(timer)
  }, [echo?.nonce, echo])

  if (shown === null) return null

  return (
    <div className="key-echo" aria-hidden="true">
      {shown.caps.map((cap, index) => (
        <span key={`${cap.label}-${index}`} className="key-echo-part">
          {index > 0 && <span className="key-echo-plus">+</span>}
          <span className={cap.modifier ? 'key-echo-cap key-echo-modifier' : 'key-echo-cap'}>{cap.label}</span>
        </span>
      ))}
    </div>
  )
}
