/**
 * Open work, side by side: what can be started, and what is
 * already claimed. Both are the newest first, because the question this view
 * answers is what to do next.
 */

import BeadRow from './BeadRow'
import { beadRowKey, type WorkRow } from '../../beads/beadsTree'

interface ReadyViewProps {
  ready: WorkRow[]
  incomplete?: boolean
  inProgress: WorkRow[]
}

function Column({ title, rows, incomplete }: { title: string; rows: WorkRow[]; incomplete?: boolean }) {
  return (
    <section className="beads-column">
      <h2>{title}</h2>
      {rows.length === 0
        ? !incomplete && <p className="beads-empty">Nothing here.</p>
        : rows.map(row => <BeadRow key={beadRowKey(row)} row={row} />)}
    </section>
  )
}

export default function ReadyView({ ready, inProgress, incomplete }: ReadyViewProps) {
  return (
    <div className="beads-columns">
      <Column title="Ready to start" rows={ready} incomplete={incomplete} />
      <Column title="In progress" rows={inProgress} incomplete={incomplete} />
    </div>
  )
}
