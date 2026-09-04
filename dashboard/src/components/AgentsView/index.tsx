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
import MenuTarget from '../MenuTarget'
import type { MenuGroup } from '../Menu'
import TableColumn from '../TableColumn'
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
  fetchAgentTender,
  shortWorkspacePath,
  type AgentContext,
  type AgentHarness,
  type AgentTender,
} from '../../agents/agentContextApi'
import { fetchWorkspaces, isRunning, workspaceName, type Workspace } from '../../workspaces/workspacesApi'
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

interface AgentsViewProps {
  /**
   * Puts a folder in front of the Files tab. The tab is not this view's to
   * switch, so the app that owns both hands the request down.
   */
  onOpenInFiles?: (path: string) => void
}

export default function AgentsView({ onOpenInFiles }: AgentsViewProps = {}) {
  const { settings, terminalUsers, openSendToSession } = useSession()
  const { announce } = useStatus()
  const theme = useTheme()
  const [harness, setHarness] = useState<AgentHarness>('claude-code')
  const [workspaces, setWorkspaces] = useState<Workspace[]>([])
  const [tender, setTender] = useState<AgentTender>({ session: '', beads: '', folder: '' })
  const [folder, setFolder] = useState<string>('')
  const [context, setContext] = useState<AgentContext | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [query, setQuery] = useState('')
  const [proposals, setProposals] = useState<BeadRow[]>([])

  const user = resolveLaunchUser(settings, 'terminal1', terminalUsers)

  // The list is the host's, not the user's: the most recently active folder
  // comes first, and that is the one the tab opens on.
  useEffect(() => {
    let current = true
    fetchWorkspaces()
      .then(found => {
        if (!current) return
        setWorkspaces(found)
        setFolder(previous => previous || found[0]?.path || '')
      })
      .catch((cause: unknown) => {
        if (!current) return
        setError(cause instanceof Error ? cause.message : 'Could not list the workspaces')
      })
    return () => { current = false }
  }, [])

  useEffect(() => {
    let current = true
    fetchAgentTender()
      .then(found => { if (current) setTender(found) })
      .catch(() => { /* an unconfigured tender is what the empty fields already say */ })
    return () => { current = false }
  }, [])

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

  const running = useMemo(() => workspaces.filter(isRunning), [workspaces])
  const projects = useMemo(() => workspaces.filter(workspace => !isRunning(workspace)), [workspaces])

  // What a folder offers besides being looked at: an agent started in it, the
  // folder itself in the Files tab, and the stack this tab is showing handed
  // to a session. Launch goes through the drawer's launcher, which is the one
  // place a session is created, with this folder and this harness already in it.
  const workspaceMenu = (workspace: Workspace) => (): MenuGroup[] => [
    {
      id: 'workspace',
      rows: [
        {
          id: 'launch',
          label: 'Launch here',
          onSelect: () => openSendToSession({
            reference: `agents ${workspace.path} ${harness}`,
            launch: { label: `Launch in ${workspaceName(workspace.path)}`, folder: workspace.path, harness },
          }),
        },
        {
          id: 'files',
          label: 'Open in Files',
          disabled: !onOpenInFiles,
          onSelect: () => onOpenInFiles?.(workspace.path),
        },
        {
          id: 'send',
          label: 'Send',
          onSelect: () => openSendToSession({ reference: `agents ${workspace.path} ${harness}` }),
        },
      ],
    },
  ]

  const workspaceRow = (workspace: Workspace) => (
    <MenuTarget key={workspace.path} label={`Actions for ${workspace.path}`} groups={workspaceMenu(workspace)}>
      <button
        type="button"
        className={`agents-rail-row ${workspace.path === folder ? 'active' : ''}`}
        aria-pressed={workspace.path === folder}
        title={workspace.sessions.length > 0 ? `${workspace.path} — ${workspace.sessions.join(', ')}` : workspace.path}
        onClick={() => setFolder(workspace.path)}
      >
        <span className="agents-workspace-path">{shortWorkspacePath(workspace.path)}</span>
        <span className="agents-count">{workspace.instructions}</span>
      </button>
    </MenuTarget>
  )

  return (
    <div className="agents-view">
      <div className="agents-columns">
        <aside className="agents-rail" data-ui="agents.rail">
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
            {running.length > 0 && <h3>Running</h3>}
            {running.map(workspaceRow)}
            <h3>Projects</h3>
            {workspaces.length === 0 && <span className="agent-note">No workspace found under the roots.</span>}
            {projects.map(workspaceRow)}
          </div>
        </aside>

        <div className="agents-main" data-ui="agents.stack">
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

        <aside className="agents-proposals" data-ui="agents.proposals">
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

        <TableColumn />
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
