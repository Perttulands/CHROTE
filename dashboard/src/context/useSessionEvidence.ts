import { useMemo } from 'react'
import { useSession } from './SessionContext'
import { sessionEvidenceFrom, type SessionEvidence } from '../terminal/tileState'

/**
 * The one join from the session poll: what the last response is entitled to say
 * about a binding. Tiles, peek and the Send target all ask the same question of
 * the same answer, so they ask it here rather than each re-deriving it.
 */
export function useSessionEvidence(): SessionEvidence {
  const { sessions, loading, error, partialAnsweringUsers } = useSession()
  return useMemo(
    () => sessionEvidenceFrom({ sessions, loading, error, partialAnsweringUsers }),
    [sessions, loading, error, partialAnsweringUsers],
  )
}
