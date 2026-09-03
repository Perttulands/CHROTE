/**
 * The Bead card: one Bead, beside whatever named it.
 *
 * It opens from a Bead id clicked in a terminal, from a row of the Beads tab,
 * and from an id inside another Bead's text. It docks at the right as a sheet,
 * so the session that printed the id stays readable next to what it means.
 *
 * The card only reads. Writing Beads stays with `bd` and the agents; what the
 * operator can do from here is hand the Bead to one of them.
 */

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import Sheet from './Sheet'
import Markdown from './Markdown'
import { useSession } from '../context/SessionContext'
import { useStatus } from '../context/StatusContext'
import { copyAndAnnounce } from '../utils/clipboard'
import { registerChords, type Chord } from '../keys/chords'
import { closeBeadCard, openBeadCard, useBeadCardRequest } from '../beads/beadCard'
import { beadIdPattern, beadProjectPath, ensureBeadProjects } from '../beads/beadIds'
import { fetchBead, type BeadDetail, type BeadLink } from '../beads/beadsApi'
import { beadGlyph, beadStatusLabel, formatBeadTime } from '../beads/beadStatus'
import './BeadCard.css'

/** The card takes this much of the workspace, as the wave-2 contract states. */
export const BEAD_CARD_WIDTH = '480px'

export function beadReference(bead: { id: string; title: string }): string {
  return `bead ${bead.id}: ${bead.title}`
}

interface BeadCardProps {
  /** Show this Bead in the Beads tab, where its project and its map are. */
  onOpenInBeads?: (projectPath: string, id: string) => void
}

function BeadLinks({ links, onOpen }: { links: BeadLink[]; onOpen: (id: string) => void }) {
  if (links.length === 0) return <span className="bead-card-none">none</span>
  return (
    <>
      {links.map((link, index) => (
        <span key={link.id}>
          {index > 0 && ', '}
          <button type="button" className="bead-card-link" onClick={() => onOpen(link.id)} title={link.title}>
            <span className="bead-card-link-glyph">{beadGlyph(link.status)}</span>
            {link.id}
          </button>
        </span>
      ))}
    </>
  )
}

function BeadSection({ label, text, onToken }: { label: string; text?: string; onToken: (token: string) => void }) {
  if (!text || text.trim() === '') return null
  return (
    <section className="bead-card-section">
      <h3>{label}</h3>
      <Markdown content={text} tokenPattern={beadIdPattern()} onToken={onToken} />
    </section>
  )
}

export default function BeadCard({ onOpenInBeads }: BeadCardProps = {}) {
  const { settings, openSendToSession } = useSession()
  const { announce } = useStatus()
  const request = useBeadCardRequest()
  const [history, setHistory] = useState<string[]>([])
  const [bead, setBead] = useState<BeadDetail | null>(null)
  const [projectPath, setProjectPath] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)

  const manualPaths = useMemo(() => settings.beadsProjectPaths || [], [settings.beadsProjectPaths])
  const id = request?.id ?? null

  // A request from outside is a new card; one the card made for itself — a
  // link followed, a step back — carries the trail it just extended.
  const followed = useRef(false)
  useEffect(() => {
    if (followed.current) {
      followed.current = false
      return
    }
    setHistory([])
  }, [request?.nonce])

  useEffect(() => {
    if (!id) {
      setBead(null)
      setError(null)
      return
    }
    let current = true
    setLoading(true)
    setError(null)
    const resolve = async () => {
      const known = request?.projectPath ?? beadProjectPath(id)
      if (known) return known
      await ensureBeadProjects(manualPaths)
      return beadProjectPath(id)
    }
    resolve()
      .then(async path => {
        if (!path) throw new Error(`No configured Beads project owns ${id}`)
        if (current) setProjectPath(path)
        const detail = await fetchBead(path, id)
        if (current) setBead(detail)
      })
      .catch((cause: unknown) => {
        if (!current) return
        setBead(null)
        setError(cause instanceof Error ? cause.message : `Could not read ${id}`)
      })
      .finally(() => { if (current) setLoading(false) })
    return () => { current = false }
  }, [id, request?.nonce, request?.projectPath, manualPaths])

  const open = useCallback((next: string) => {
    followed.current = true
    setHistory(previous => (id ? [...previous, id] : previous))
    openBeadCard(next, projectPath ?? undefined)
  }, [id, projectPath])

  const back = useCallback(() => {
    const previous = history[history.length - 1]
    if (!previous) return
    followed.current = true
    setHistory(rest => rest.slice(0, -1))
    openBeadCard(previous, projectPath ?? undefined)
  }, [history, projectPath])

  const send = useCallback(() => {
    if (!bead) return
    openSendToSession({ reference: beadReference(bead) })
  }, [bead, openSendToSession])

  // Alt+S from the card sends the Bead. It is registered in both scopes the
  // operator can be in while the card is open — a focused tile takes tile
  // chords first — and retired with the card, so the tile's own Send comes
  // back the moment there is no Bead in hand.
  useEffect(() => {
    if (!bead) return
    const run = () => openSendToSession({ reference: beadReference(bead) })
    const chords: Chord[] = (['global', 'tile'] as const).map(scope => ({
      id: `beads.card.send.${scope}`,
      key: 's',
      direct: { alt: true, shift: false, key: 's' },
      label: `Send ${bead.id}`,
      scope,
      run,
    }))
    return registerChords(chords)
  }, [bead, openSendToSession])

  // The back step is the one thing Escape must not skip past: a card reached
  // from another card returns to it, and only then does Escape close.
  const dismiss = useCallback(() => {
    if (history.length > 0) {
      back()
      return
    }
    closeBeadCard()
  }, [back, history.length])

  if (!request) return null

  const header = (
    <>
      <span className="bead-card-id">{request.id}</span>
      {history.length > 0 && (
        <button type="button" className="bead-card-action" onClick={back}>Back</button>
      )}
      <span className="bead-card-header-spacer" />
      {bead && projectPath && onOpenInBeads && (
        <button type="button" className="bead-card-action" onClick={() => onOpenInBeads(projectPath, bead.id)}>
          Open in Beads
        </button>
      )}
      {bead && <button type="button" className="bead-card-action" onClick={send}>Send</button>}
      <button
        type="button"
        className="bead-card-action"
        onClick={() => { void copyAndAnnounce(request.id, request.id, announce) }}
      >
        Copy id
      </button>
      <button type="button" className="bead-card-close" onClick={closeBeadCard} aria-label="Close Bead card">×</button>
    </>
  )

  return (
    <Sheet open edge="right" extent={BEAD_CARD_WIDTH} label={`Bead ${request.id}`} onClose={dismiss} header={header}>
      <div className="bead-card-body" data-ui="beads.card">
        {loading && !bead && <p className="bead-card-note">Reading {request.id}…</p>}
        {error && <p className="bead-card-error">{error}</p>}
        {bead && (
          <>
            <h2 className="bead-card-title">{bead.title}</h2>
            <dl className="bead-card-fields">
              <dt>Status</dt>
              <dd>{beadStatusLabel(bead.status, bead.blockedBy.some(link => link.status !== 'closed'))}</dd>
              <dt>Type</dt>
              <dd>{bead.type || 'task'}</dd>
              <dt>Priority</dt>
              <dd>{bead.priority}</dd>
              <dt>Updated</dt>
              <dd>{formatBeadTime(bead.updated)}</dd>
              <dt>Parent</dt>
              <dd><BeadLinks links={bead.parents} onOpen={open} /></dd>
              <dt>Children</dt>
              <dd><BeadLinks links={bead.children} onOpen={open} /></dd>
              <dt>Blocked by</dt>
              <dd><BeadLinks links={bead.blockedBy} onOpen={open} /></dd>
              <dt>Blocks</dt>
              <dd><BeadLinks links={bead.blocks} onOpen={open} /></dd>
            </dl>
            <BeadSection label="Description" text={bead.description} onToken={open} />
            <BeadSection label="Design" text={bead.design} onToken={open} />
            <BeadSection label="Acceptance criteria" text={bead.acceptance} onToken={open} />
            <BeadSection label="Notes" text={bead.notes} onToken={open} />
          </>
        )}
      </div>
    </Sheet>
  )
}
