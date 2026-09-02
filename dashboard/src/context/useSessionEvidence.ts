import { useSession } from './SessionContext'
import type { SessionEvidence } from '../terminal/tileState'

/**
 * The one join from the session poll: what the last response is entitled to say
 * about a binding. Tiles, peek and the Send target all ask the same question of
 * the same answer, so they read it here rather than each re-deriving it — and
 * so a peek opened during a poll outage inherits the verdict its tile holds
 * instead of starting over with none.
 */
export function useSessionEvidence(): SessionEvidence {
  return useSession().sessionEvidence
}
