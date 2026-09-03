/**
 * The Bead card: one Bead, on the table.
 *
 * It opens from a Bead id clicked in a terminal, from a row of the Beads tab,
 * and from an id inside another Bead's text. The table's column at the right
 * of the tab shows it, so the session that printed the id, or the map that
 * listed it, stays readable next to what it means.
 *
 * The card only reads. Writing Beads stays with `bd` and the agents; what the
 * operator can do from here is hand the Bead to one of them.
 */

import { useCallback, useEffect, useMemo, useState } from 'react'
import Markdown from './Markdown'
import { useSession } from '../context/SessionContext'
import { useStatus } from '../context/StatusContext'
import { copyTextToClipboard } from '../utils/clipboard'
import { registerChords, type Chord } from '../keys/chords'
import { backInBeadCard, closeBeadCard, followBeadFromCard, useBeadCardRequest } from '../beads/beadCard'
import { beadIdPattern, beadProjectPath, ensureBeadProjects } from '../beads/beadIds'
import { beadReference } from '../beads/beadReference'
import { fetchBead, type BeadDetail, type BeadLink } from '../beads/beadsApi'
import { beadGlyph, beadStatusLabel, formatBeadTime } from '../beads/beadStatus'
import { nameBeadOnTable } from '../context/TableContext'
import { useResidentPresent } from '../residents/residentPresence'
import './BeadCard.css'

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
  // In a tab with a resident, Alt+S is the resident's: it pastes the Bead into
  // that prompt. The card's Send stays a word for every other session.
  const residentPresent = useResidentPresent()
  const [bead, setBead] = useState<BeadDetail | null>(null)
  const [projectPath, setProjectPath] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)

  const manualPaths = useMemo(() => settings.beadsProjectPaths || [], [settings.beadsProjectPaths])
  const id = request?.id ?? null
  const behind = request?.trail.length ?? 0

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
        if (!current) return
        setBead(detail)
        nameBeadOnTable(detail.id, detail.title)
      })
      .catch((cause: unknown) => {
        if (!current) return
        setBead(null)
        setError(cause instanceof Error ? cause.message : `Could not read ${id}`)
      })
      .finally(() => { if (current) setLoading(false) })
    return () => { current = false }
  }, [id, request?.nonce, request?.projectPath, manualPaths])

  // A link followed from the card extends the trail, so Back and Escape can
  // retrace it; the trail is the table's, and outlives this mount.
  const open = useCallback((next: string) => {
    followBeadFromCard(next, projectPath ?? undefined)
  }, [projectPath])

  const send = useCallback(() => {
    if (!bead) return
    openSendToSession({ reference: beadReference(bead) })
  }, [bead, openSendToSession])

  // Alt+S from the card sends the Bead. It is registered in both scopes the
  // operator can be in while the card is open — a focused tile takes tile
  // chords first — and retired with the card, so the tile's own Send comes
  // back the moment there is no Bead in hand.
  useEffect(() => {
    if (!bead || residentPresent) return
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
  }, [bead, openSendToSession, residentPresent])

  if (!request) return null

  const canOpenInBeads = Boolean(bead && projectPath && onOpenInBeads)

  return (
    <div className="bead-card" data-ui="beads.card">
      {/* The actions keep their places while the Bead loads: a control the
          operator is reaching for does not move because its data arrived. */}
      <div className="table-header">
        {onOpenInBeads && (
          <button
            type="button"
            className="table-action"
            disabled={!canOpenInBeads}
            onClick={() => { if (bead && projectPath) onOpenInBeads(projectPath, bead.id) }}
          >
            Open in Beads
          </button>
        )}
        <button
          type="button"
          className="table-action"
          aria-keyshortcuts={residentPresent ? undefined : 'Alt+S'}
          disabled={!bead}
          onClick={send}
        >
          Send{!residentPresent && <span className="table-chord" aria-hidden="true">Alt+S</span>}
        </button>
        <button
          type="button"
          className="table-action"
          onClick={() => {
            void copyTextToClipboard(request.id)
            announce(`Copied ${request.id}`, 'success')
          }}
        >
          Copy id
        </button>
        {behind > 0 && (
          <button type="button" className="table-action" onClick={backInBeadCard}>Back</button>
        )}
        <span className="table-header-spacer" />
        <button type="button" className="table-action" aria-keyshortcuts="Escape" onClick={closeBeadCard}>
          Close<span className="table-chord" aria-hidden="true">Esc</span>
        </button>
      </div>
      <div className="bead-card-body">
        <p className="bead-card-meta">
          <span className="bead-card-id">{request.id}</span>
          {bead && (
            <>
              {' · '}{bead.type || 'task'}
              {' · '}<span>{beadStatusLabel(bead.status, bead.blockedBy.some(link => link.status !== 'closed'))}</span>
              {' · '}P{bead.priority}
              {' · '}updated {formatBeadTime(bead.updated)}
            </>
          )}
        </p>
        {loading && !bead && <p className="bead-card-note">Reading {request.id}…</p>}
        {error && <p className="bead-card-error">{error}</p>}
        {bead && (
          <>
            <h2 className="bead-card-title">{bead.title}</h2>
            <dl className="bead-card-fields">
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
    </div>
  )
}
