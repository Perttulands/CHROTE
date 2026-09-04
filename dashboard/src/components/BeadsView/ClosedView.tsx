/** Finished work, requested only while this view is selected. */

import BeadRow from './BeadRow'
import { beadRowKey, type WorkRow } from '../../beads/beadsTree'

export interface ClosedFailure {
  projectName: string
  message: string
}

interface ClosedViewProps {
  rows: WorkRow[]
  failures: ClosedFailure[]
  query: string
}

export default function ClosedView({ rows, failures, query }: ClosedViewProps) {
  const groups = new Map<string, WorkRow[]>()
  rows.forEach(row => {
    const group = groups.get(row.projectPath) ?? []
    group.push(row)
    groups.set(row.projectPath, group)
  })

  return (
    <div className="beads-closed">
      {failures.length > 0 && (
        <div className="beads-partial-errors" role="status">
          {failures.map(failure => (
            <p key={`${failure.projectName}\u0000${failure.message}`}>
              {failure.projectName}: {failure.message}
            </p>
          ))}
        </div>
      )}
      {rows.length === 0 && (
        <p className="beads-empty">
          {query.trim() === '' ? 'No closed Beads in this scope.' : `No closed Beads match "${query.trim()}".`}
        </p>
      )}
      {[...groups.entries()].map(([projectPath, projectRows]) => (
        <section key={projectPath} className="beads-closed-project">
          <h2>{projectRows[0].projectName}</h2>
          {projectRows.map(row => <BeadRow key={beadRowKey(row)} row={row} />)}
        </section>
      ))}
    </div>
  )
}
