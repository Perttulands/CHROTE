/**
 * What the leader window offers, while it is open and never otherwise.
 *
 * It is chrome, not terminal text: it sits along the bottom of the workspace
 * in the dashboard's own type, and it says only two things — the keys pressed
 * so far, and the "key  action" pairs the current scope can reach.
 */

import { useLeader } from './chords'
import './LeaderStrip.css'

export default function LeaderStrip() {
  const { leaderOpen, pressed, scopeChords } = useLeader()
  if (!leaderOpen) return null

  return (
    <div className="leader-strip" role="status" aria-label="Leader chords">
      <span className="leader-strip-echo">{pressed.join(' ')}</span>
      <ul className="leader-strip-chords">
        {scopeChords.map(chord => (
          <li key={chord.id} className="leader-strip-chord">
            <span className="leader-strip-key">{chord.key}</span>
            <span className="leader-strip-label">{chord.label}</span>
          </li>
        ))}
      </ul>
    </div>
  )
}
