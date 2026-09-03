/**
 * The status line: a 28px footer that is always there.
 *
 * It runs the full width of the window, under both the Sessions panel and the
 * workspace, so a sheet docked at an edge can cover a panel without truncating
 * the line. It carries one thing — the last event, with the time it happened —
 * and a failure is the only thing on it that takes colour.
 */

import { useStatus } from '../context/StatusContext'
import './StatusLine.css'

// One clock for every device: hours and minutes, 24 hours, no seconds. A
// locale that would write "02:13 PM" here is a locale, not information.
function timeOf(at: number): string {
  return new Date(at).toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit', hour12: false })
}

export default function StatusLine() {
  const { status } = useStatus()

  return (
    <div className="status-line" data-ui="status.line" role="status" aria-live="polite" aria-label="Status">
      {status !== null && (
        <>
          <span className="status-line-time">{timeOf(status.at)}</span>
          <span
            className={status.severity === 'error' ? 'status-line-message status-line-failure' : 'status-line-message'}
          >
            {status.message}
          </span>
        </>
      )}
    </div>
  )
}
