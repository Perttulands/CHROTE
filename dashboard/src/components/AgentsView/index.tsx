/**
 * The Agents tab: what an agent would see, for any folder on the host.
 *
 * The panel answers the question for the session in front of the operator; this
 * answers it for a workspace he has not started an agent in yet, and for the
 * other harness, so the same instruction can be compared where it is supposed
 * to hold. Nothing here writes except the shared editor in a row, and the
 * curation itself belongs to the tender, whose desk sits at the foot of the tab
 * exactly as the Librarian's does in the Library.
 */

import { useCallback, useEffect, useMemo, useState } from 'react'
import AgentStack from '../AgentStack'
import Desk from '../Desk'
import { HarnessMark } from '../harnessMarks'
import { useSession } from '../../context/SessionContext'
import { useStatus } from '../../context/StatusContext'
import { openBeadCard } from '../../beads/beadCard'
import { fetchBeadWork, type BeadRow } from '../../beads/beadsApi'
import { isBeadClosed, beadGlyph } from '../../beads/beadStatus'
import { getTerminalUserInitial, resolveLaunchUser } from '../../types'
import { identityColorFor } from '../../theme/theme'
import { useTheme } from '../../theme/ThemeContext'
import {
  AGENT_HARNESSES,
  fetchAgentContext,
  fetchAgentWorkspaces,
  shortWorkspacePath,
  type AgentContext,
  type AgentHarness,
  type AgentTender,
  type AgentWorkspace,
} from '../../agents/agentContextApi'
import './AgentsView.css'

/** How many proposals the right column shows. */
const PROPOSAL_LIMIT = 12

function tally(count: number, singular: string, plural: string): string {
  return `${count} ${count === 1 ? singular : plural}`
}

function ageOf(updated: string | undefined): string {
  if (!updated) return ''
  const when = new Date(updated)
  if (Number.isNaN(when.getTime())) return ''
  const hours = Math.floor((Date.now() - when.getTime()) / 3600000)
  if (hours < 1) return 'now'
  if (hours < 24) return `${hours}h`
  return `${Math.floor(hours / 24)}d`
}

export default function AgentsView() {
  const { settings, terminalUsers } = useSession()
  const { announce } = useStatus()
  const theme = useTheme()
  const [harness, setHarness] = useState<AgentHarness>('claude-code')
  const [workspaces, setWorkspaces] = useState<AgentWorkspace[]>([])
  const [tender, setTender] = useState<AgentTender>({ session: '', beads: '', folder: '' })
  const [folder, setFolder] = useState<string>('')
  const [context, setContext] = useState<AgentContext | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [query, setQuery] = useState('')
  const [proposals, setProposals] = useState<BeadRow[]>([])

  const user = resolveLaunchUser(settings, 'terminal1', terminalUsers)

  useEffect(() => {
    let current = true
    fetchAgentWorkspaces(user)
      .then(found => {
        if (!current) return
        setWorkspaces(found.workspaces)
        setTender(found.tender)
        setFolder(previous => previous || found.workspaces[0]?.path || '')
      })
      .catch((cause: unknown) => {
        if (!current) return
        setError(cause instanceof Error ? cause.message : 'Could not list the workspaces')
      })
    return () => { current = false }
  }, [user])

  useEffect(() => {
    if (!folder) return
    let current = true
    setContext(null)
    setError(null)
    fetchAgentContext(folder, harness, user)
      .then(resolved => {
        if (!current) return
        setContext(resolved)
        announce(
          `${folder} under ${harness === 'codex' ? 'Codex' : 'Claude Code'}: ` +
          `${tally(resolved.instructions.length, 'instruction file', 'instruction files')}, ` +
          `${tally(resolved.skills.length, 'skill', 'skills')}, ` +
          `${tally(resolved.memories.length, 'memory', 'memories')}`,
          'info',
        )
      })
      .catch((cause: unknown) => {
        if (!current) return
        setError(cause instanceof Error ? cause.message : 'Could not resolve the stack')
      })
    return () => { current = false }
  }, [announce, folder, harness, user])

  useEffect(() => {
    if (!tender.beads) {
      setProposals([])
      return
    }
    let current = true
    fetchBeadWork(tender.beads)
      .then(work => {
        if (!current) return
        setProposals(work.beads.filter(bead => !isBeadClosed(bead.status)).slice(0, PROPOSAL_LIMIT))
      })
      .catch(() => { if (current) setProposals([]) })
    return () => { current = false }
  }, [tender.beads])

  const openProposal = useCallback((bead: BeadRow) => {
    openBeadCard(bead.id, tender.beads)
  }, [tender.beads])

  const harnessLabel = useMemo(
    () => AGENT_HARNESSES.find(entry => entry.id === harness)?.label ?? harness,
    [harness],
  )

  return (
    <div className="agents-view">
      <div className="agents-columns">
        <aside className="agents-rail">
          <input
            type="text"
            className="agents-filter"
            aria-label="Filter skills and memories"
            placeholder="Filter skills and memories…"
            value={query}
            onChange={event => setQuery(event.target.value)}
          />
          <div className="agents-group">
            <h3>Harness</h3>
            {AGENT_HARNESSES.map(entry => (
              <button
                type="button"
                key={entry.id}
                className={`agents-rail-row ${entry.id === harness ? 'active' : ''}`}
                aria-pressed={entry.id === harness}
                onClick={() => setHarness(entry.id)}
              >
                <HarnessMark id={entry.id} />
                <span>{entry.label}</span>
              </button>
            ))}
          </div>
          <div className="agents-group agents-workspaces">
            <h3>Workspaces</h3>
            {workspaces.length === 0 && <span className="agent-note">No workspace holds instructions.</span>}
            {workspaces.map(workspace => (
              <button
                type="button"
                key={workspace.path}
                className={`agents-rail-row ${workspace.path === folder ? 'active' : ''}`}
                aria-pressed={workspace.path === folder}
                title={workspace.path}
                onClick={() => setFolder(workspace.path)}
              >
                <span className="agents-workspace-path">{shortWorkspacePath(workspace.path)}</span>
                <span className="agents-count">{workspace.instructions}</span>
              </button>
            ))}
          </div>
        </aside>

        <div className="agents-main">
          <div className="agents-subject">
            <span className="agents-folder">{folder || 'no workspace'}</span>
            <HarnessMark id={harness} />
            <span>{harnessLabel}</span>
            {user && (
              <>
                <span
                  className="unix-user-badge"
                  style={{ backgroundColor: identityColorFor(user, terminalUsers, theme) }}
                  aria-label={`Unix user ${user}`}
                >
                  {getTerminalUserInitial(user)}
                </span>
                <span className="agents-user">{user}</span>
              </>
            )}
          </div>
          <p className="agents-purpose">
            Everything an agent started here loads before it reads a word from you.
          </p>
          <div className="agents-stack-scroll">
            {error !== null && <div className="agent-note">{error}</div>}
            {error === null && context === null && folder && <div className="agent-note">Resolving…</div>}
            {context !== null && <AgentStack context={context} query={query} />}
          </div>
        </div>

        <aside className="agents-proposals">
          <h3>Proposals</h3>
          {!tender.beads && <span className="agent-note">No tender Beads project is configured.</span>}
          {tender.beads && proposals.length === 0 && <span className="agent-note">Nothing is in flight.</span>}
          {proposals.map(bead => (
            <button type="button" className="agents-proposal" key={bead.id} onClick={() => openProposal(bead)}>
              <span className="agents-proposal-head">
                <span className="agents-proposal-glyph">{beadGlyph(bead.status, bead.blocked)}</span>
                <span className="agents-proposal-id">{bead.id}</span>
                <span className="agents-proposal-age">{ageOf(bead.updated)}</span>
              </span>
              <span className="agents-proposal-title">{bead.title}</span>
            </button>
          ))}
          {tender.beads && <span className="agents-proposals-foot">From the tender&apos;s Beads project.</span>}
        </aside>
      </div>

      <Desk
        label="Tender"
        sessionName={tender.session || undefined}
        reference={`agents ${folder} ${harness}`}
        placeholder="Ask the tender…"
        launchFolder={tender.folder || undefined}
      />
    </div>
  )
}
