/**
 * The moment the map is read at.
 *
 * The map used to offer four buttons — day, week, month, all — which answered
 * one question, how lately has this moved, and answered it four ways. What the
 * operator actually wants to see is the corpus at a moment: which pages existed
 * then, and which of them were current. So the four buttons become one scrubber
 * along the bottom of the map, and the four windows become stops on it.
 *
 * A moment decides two things about a page, and no more:
 *
 * - A page committed after the moment did not exist then, and is not drawn.
 * - A page whose last change is at or before the moment is current, and is
 *   drawn at full strength; one changed since is drawn at the stale level,
 *   because what it says now is not what it said then.
 *
 * A page git has never seen has no arrival and no change, so no moment can
 * place it: it is drawn, at every moment, at its own strength. That is the
 * honest answer for a page the corpus cannot date, and it is the same answer
 * the recency window gave.
 */

/** The rightmost stop: the corpus as it stands. */
export const NOW = Number.POSITIVE_INFINITY

const DAY = 86_400_000

/** The stops the scrubber offers, oldest first, as offsets back from now. */
export const MOMENT_STOPS: readonly { id: string; label: string; back: number }[] = [
  { id: 'month', label: 'A month ago', back: 30 * DAY },
  { id: 'week', label: 'A week ago', back: 7 * DAY },
  { id: 'day', label: 'A day ago', back: DAY },
  { id: 'now', label: 'Now', back: 0 },
]

/** When a page arrived, or 0 for a page git has never seen. */
export function momentOf(iso: string): number {
  if (!iso) return 0
  const at = Date.parse(iso)
  return Number.isNaN(at) ? 0 : at
}

/** Whether a page existed at this moment. */
export function existedAt(created: number, moment: number): boolean {
  return created === 0 || created <= moment
}

/** Whether what a page says now is what it said at this moment. */
export function currentAt(updated: number, moment: number): boolean {
  return updated === 0 || updated <= moment
}

/** How many places the scrubber has between the corpus's first page and now. */
export const SCRUB_STEPS = 1000

/**
 * How far back one place on the scrubber is.
 *
 * Not evenly: a reader asks about last week far more often than about the
 * month the corpus started, so the recent past takes most of the track and the
 * far past is compressed into its left end. Cubed is enough to put a day, a
 * week and a month at four fifths, three fifths and a third of the way along a
 * corpus four months old, which is a scrubber the operator can land on rather
 * than one where every stop he wants is in the last percent.
 */
export function backAt(position: number, span: number): number {
  const left = Math.max(0, Math.min(1, 1 - position / SCRUB_STEPS))
  return span * left * left * left
}

/** Where on the scrubber a moment that far back sits. */
export function positionOf(back: number, span: number): number {
  if (span <= 0) return SCRUB_STEPS
  const left = Math.cbrt(Math.max(0, Math.min(1, back / span)))
  return Math.round((1 - left) * SCRUB_STEPS)
}

/** The stops the scrubber carries, as places along it. */
export function stopsOn(span: number): { id: string; label: string; back: number; position: number }[] {
  return MOMENT_STOPS
    .map(stop => ({ ...stop, position: positionOf(stop.back, span) }))
    .filter(stop => stop.position > 0)
}

/**
 * How far back the scrubber is asking about.
 *
 * A place that is one of the stops is that stop exactly. The track is a
 * thousand whole steps over months, so the place nearest a week ago is a few
 * hours off it, and a stop that reads "6 days ago" is a stop that does not
 * work. Everywhere else the curve answers.
 */
export function backFrom(position: number, span: number): number {
  const stop = stopsOn(span).find(entry => entry.position === position)
  return stop ? stop.back : backAt(position, span)
}

/** How close to a stop the scrubber comes before it takes it. */
const SNAP = 10

/**
 * The place the scrubber settles on: a stop, if it came near one. The four
 * windows the map used to offer as buttons are these stops, so the reading they
 * gave is still one movement away.
 */
export function snapTo(position: number, span: number): number {
  let best = position
  let nearest = SNAP
  stopsOn(span).forEach(stop => {
    const distance = Math.abs(stop.position - position)
    if (distance < nearest) {
      nearest = distance
      best = stop.position
    }
  })
  return best
}
