/**
 * The selected Bead's own words, beside its work graph: the title, what it is
 * and who holds it, then its description, design, acceptance criteria, notes
 * and close reason. The text comes from the server's projection of the store,
 * the same read the table is drawn from, so it costs no bd run and is there
 * after a restart; a Bead read before is drawn at once. A Bead id in the text
 * selects that Bead here.
 */

import { memo, useEffect, useMemo, useRef, useState } from 'react'
import Markdown from '../Markdown'
import BeadTypeLabel from '../BeadTypeLabel'
import { beadGlyph, formatBeadTime } from '../../beads/beadStatus'
import { fetchHostBeadText, heldHostBeadText, type HostBeadText } from '../../beads/hostBeads'
import { SLICE_STATUSES, STATUS_LABELS, type SliceIndex } from '../../beads/beadsSlice'

interface BeadTextProps {
  index: SliceIndex
  /** The selected row, or -1. */
  row: number
  /** The generation each store path was read at. */
  generations: ReadonlyMap<string, string | undefined>
  /** Bead ids in the text become links when they match. */
  idPattern: RegExp
  onToken: (id: string) => void
}

interface Read {
  path: string
  id: string
  generation?: string
  bead?: HostBeadText
  error?: string
}

const SECTIONS: { key: keyof HostBeadText; label: string }[] = [
  { key: 'description', label: 'Description' },
  { key: 'design', label: 'Design' },
  { key: 'acceptance', label: 'Acceptance criteria' },
  { key: 'notes', label: 'Notes' },
  { key: 'closeReason', label: 'Close reason' },
]

function BeadText({ index, row, generations, idPattern, onToken }: BeadTextProps) {
  const bead = row >= 0 ? index.rows[row] : null
  const project = row >= 0 ? index.projects[index.project[row]] : null
  const path = project?.path ?? ''
  const id = bead?.id ?? ''
  const generation = generations.get(path)
  const [read, setRead] = useState<Read | null>(null)
  const scroller = useRef<HTMLElement>(null)

  useEffect(() => {
    if (!id || !path || heldHostBeadText(path, id, generation)) return
    const controller = new AbortController()
    fetchHostBeadText(path, id, generation, controller.signal)
      .then(text => setRead({ path, id, generation, bead: text }))
      .catch((cause: unknown) => {
        if (controller.signal.aborted) return
        setRead({ path, id, generation, error: cause instanceof Error ? cause.message : 'Could not read the text' })
      })
    return () => controller.abort()
  }, [generation, id, path])

  // A new Bead is read from its top.
  useEffect(() => { if (scroller.current) scroller.current.scrollTop = 0 }, [id, path])

  // The text held for this Bead now; while a newer read of its store is on
  // its way, the last text of the same Bead stands rather than blanking.
  const held = id ? heldHostBeadText(path, id, generation) : undefined
  const same = read && read.path === path && read.id === id ? read : null
  const text = held ?? same?.bead
  const error = !text && same?.generation === generation ? same?.error : undefined

  const sections = useMemo(() => SECTIONS
    .map(section => ({ ...section, body: text?.[section.key] }))
    .filter((section): section is { key: keyof HostBeadText; label: string; body: string } => typeof section.body === 'string' && section.body.trim() !== ''),
  [text])

  if (!bead || !project) {
    return (
      <article className="beads-text" data-ui="beads.text">
        <p className="beads-text-empty">Select a Bead to read it here.</p>
      </article>
    )
  }

  const status = SLICE_STATUSES[index.status[row]]
  const updated = bead.updated ? formatBeadTime(new Date(bead.updated * 1000).toISOString()) : ''
  return (
    <article ref={scroller} className="beads-text" data-ui="beads.text" aria-label={`${bead.id}: ${bead.title}`}>
      <header className="beads-text-head">
        <p className="beads-text-line">
          <span className="beads-text-id">{bead.id}</span>
          <span>{`${beadGlyph(status === 'closed' ? 'closed' : bead.status, status === 'blocked')} ${STATUS_LABELS[status]}`}</span>
          <BeadTypeLabel type={bead.type} />
          <span>{`P${bead.priority}`}</span>
          <span title={project.path}>{project.label}</span>
          {text?.assignee && <span>{text.assignee}</span>}
          {updated && <span>{`updated ${updated}`}</span>}
        </p>
        <h2 className="beads-text-title">{bead.title}</h2>
        {text?.labels && text.labels.length > 0 && (
          <p className="beads-text-labels">{text.labels.join(' · ')}</p>
        )}
      </header>
      {sections.map(section => (
        <section key={section.key} className="beads-text-section">
          <h3>{section.label}</h3>
          <Markdown content={section.body} tokenPattern={idPattern} onToken={onToken} />
        </section>
      ))}
      {text && sections.length === 0 && <p className="beads-text-empty">No written text.</p>}
      {error && <p className="beads-text-error" role="status">{error}</p>}
    </article>
  )
}

export default memo(BeadText)
