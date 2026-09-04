/**
 * A Bead's type, wherever a Bead is drawn: the word in the type's own colour.
 *
 * One component so the map tree, the Open view, Stale and the card's header
 * say a type the same way, and so the sixth colour rule has one place to live.
 */

import { beadTypeLabel, beadTypeName } from '../beads/beadType'
import './BeadTypeLabel.css'

export default function BeadTypeLabel({ type, className }: { type: string | undefined; className?: string }) {
  return (
    <span className={className ? `bead-type ${className}` : 'bead-type'} data-type={beadTypeName(type)}>
      {beadTypeLabel(type)}
    </span>
  )
}
