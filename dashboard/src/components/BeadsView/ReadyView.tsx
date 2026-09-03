/**
 * Ready and in progress, side by side: what can be started, and what is
 * already claimed. Both are the newest first, because the question this view
 * answers is what to do next.
 */

import BeadRow from './BeadRow'
import { beadRowKey, type WorkRow } from '../../beads/beadsTree'

interface ReadyViewProps {
  ready: WorkRow[]
  inProgress: WorkRow[]
}

function Column({ title, rows }: { title: string; rows: WorkRow[] }) {
  return (
    <section className="beads-column">
      <h2>{title}</h2>
      {rows.length === 0
        ? <p className="beads-empty">Nothing here.</p>
        : rows.map(row => <BeadRow key={beadRowKey(row)} row={row} />)}
    </section>
  )
}

export default function ReadyView({ ready, inProgress }: ReadyViewProps) {
  return (
    <div className="beads-columns">
      <Column title="Ready" rows={ready} />
      <Column title="In progress" rows={inProgress} />
    </div>
  )
}
