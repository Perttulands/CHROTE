/**
 * Stale: open work nobody has touched in a while, the most neglected first.
 *
 * The row's Send says what the Bead needs — a close or a revival — so the
 * hand-off is one click and the agent gets the question already asked.
 */

import BeadRow from './BeadRow'
import { useSession } from '../../context/SessionContext'
import { daysSince } from '../../beads/beadStatus'
import { beadRowKey, type WorkRow } from '../../beads/beadsTree'

interface StaleViewProps {
  rows: WorkRow[]
  now?: number
}

export function staleReference(id: string): string {
  return `bead ${id} looks stale: close it or revive it`
}

export default function StaleView({ rows, now = Date.now() }: StaleViewProps) {
  const { openSendToSession } = useSession()

  if (rows.length === 0) return <p className="beads-empty">Nothing has gone stale.</p>

  return (
    <div className="bead-stale">
      {rows.map(row => (
        <BeadRow
          key={beadRowKey(row)}
          row={row}
          trailing={
            <>
              <span className="bead-row-age">{daysSince(row.updated, now)} days</span>
              <button
                type="button"
                className="bead-row-send"
                onClick={() => openSendToSession({ reference: staleReference(row.id) })}
              >
                Send
              </button>
            </>
          }
        />
      ))}
    </div>
  )
}
