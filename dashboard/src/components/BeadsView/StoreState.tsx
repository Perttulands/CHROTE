/**
 * The Beads rail's lower half: what the selected store is, in numbers.
 *
 * Two blocks under a hairline. The state block draws one short bar per status,
 * the word at the left and the number at the right; the facts list names the
 * store, when it was last touched, and what it holds by type. Every number is
 * the server's manifest-keyed projection, never a count of the work payload.
 */

import type { ReactNode } from 'react'
import BeadTypeLabel from '../BeadTypeLabel'
import { formatBeadTime } from '../../beads/beadStatus'
import { statusRows, storeWarnings, totalBeads, typeRows, type CountRow } from '../../beads/storeState'
import type { BeadProject } from '../../beads/beadsApi'

function Bar({ row, children }: { row: CountRow; children: ReactNode }) {
  return (
    <li className="beads-store-row">
      <span className="beads-store-row-label">{children}</span>
      <span className="beads-store-track" aria-hidden="true">
        <span className="beads-store-fill" style={{ width: `${Math.round(row.share * 100)}%` }} />
      </span>
      <span className="beads-store-count">{row.count}</span>
    </li>
  )
}

export default function StoreState({ store }: { store: BeadProject | null }) {
  if (!store) {
    return (
      <p className="beads-store-note">Select a store to read its state.</p>
    )
  }

  const warnings = storeWarnings(store)
  const counts = store.error ? undefined : store.counts

  return (
    <div className="beads-store">
      <div className="beads-store-head">
        <span className="beads-store-prefix">{store.prefix || store.name}</span>
        {counts && <span className="beads-store-total">{totalBeads(counts)}</span>}
      </div>
      <p className="beads-store-path" title={store.path}>{store.path}</p>

      {warnings.length > 0 && (
        <ul className="beads-store-warnings">
          {warnings.map(warning => (
            <li key={warning.kind} className={`beads-store-warning beads-store-warning-${warning.kind}`}>
              {warning.text}
            </li>
          ))}
        </ul>
      )}

      {counts && (
        <>
          <ul className="beads-store-block">
            {statusRows(counts).map(row => (
              <Bar key={row.key} row={row}>{row.label}</Bar>
            ))}
          </ul>
          {typeRows(counts).length > 0 && (
            <ul className="beads-store-block beads-store-block-types">
              {typeRows(counts).map(row => (
                <Bar key={row.key} row={row}><BeadTypeLabel type={row.key} /></Bar>
              ))}
            </ul>
          )}
          <dl className="beads-store-facts">
            <dt>Newest update</dt>
            <dd>{formatBeadTime(store.newestUpdate) || 'unknown'}</dd>
          </dl>
        </>
      )}
    </div>
  )
}
