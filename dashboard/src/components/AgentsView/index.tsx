/**
 * The Agents tab: what an agent would see, for any folder on the host.
 *
 * The panel answers the question for the session in front of the operator; this
 * answers it for a workspace he has not started an agent in yet, and for the
 * other harness, so the same instruction can be compared where it is supposed
 * to hold. Nothing here writes except the shared editor in a row, and the
 * curation itself belongs to the tender, whose column sits at the far right of
 * the tab exactly as the Librarian's does in the Library.
 */

import { useCallback, useEffect, useMemo, useState } from 'react'
import AgentStack from '../AgentStack'
import MenuTarget from '../MenuTarget'
import type { MenuGroup } from '../Menu'
import ResidentColumn from '../ResidentColumn'
import TableColumn from '../TableColumn'
import Rail, { RailScroll, RailSection } from '../Rail'
import { HarnessMark } from '../harnessMarks'
import { useSession } from '../../context/SessionContext'
import { useStatus } from '../../context/StatusContext'
import { getTerminalUserInitial, resolveLaunchUser } from '../../types'
import { identityColorFor } from '../../theme/theme'
import { useTheme } from '../../theme/ThemeContext'
import {
  AGENT_HARNESSES,
  fetchAgentContext,
  shortWorkspacePath,
  type AgentContext,
  type AgentHarness,
} from '../../agents/agentContextApi'
import { fetchWorkspaces, isRunning, workspaceName, type Workspace } from '../../workspaces/workspacesApi'
import './AgentsView.css'

function tally(count: number, singular: string, plural: string): string {
  return `${count} ${count === 1 ? singular : plural}`
}

interface AgentsViewProps {
  active?: boolean
  /**
   * Puts a folder in front of the Files tab. The tab is not this view's to
   * switch, so the app that owns both hands the request down.
   */
  onOpenInFiles?: (path: string) => void
}

export default function AgentsView({ active = true, onOpenInFiles }: AgentsViewProps = {}) {
  const { settings, terminalUsers, openSendToSession, updateSettings } = useSession()
  const { announce } = useStatus()
  const theme = useTheme()
  const [harness, setHarness] = useState<AgentHarness>('claude-code')
  const [workspaces, setWorkspaces] = useState<Workspace[]>([])
  const [folder, setFolder] = useState<string>('')
  const [context, setContext] = useState<AgentContext | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [query, setQuery] = useState('')

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

  const harnessLabel = useMemo(
    () => AGENT_HARNESSES.find(entry => entry.id === harness)?.label ?? harness,
    [harness],
  )

  const running = useMemo(() => workspaces.filter(isRunning), [workspaces])
  const projects = useMemo(() => workspaces.filter(workspace => !isRunning(workspace)), [workspaces])
  const commitRailWidth = useCallback((agents: number) => {
    updateSettings({ railWidth: { ...settings.railWidth, agents } })
  }, [settings.railWidth, updateSettings])
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
        <Rail
          className="agents-rail"
          data-ui="agents.rail"
          label="Agents"
          width={settings.railWidth.agents}
          onWidthCommit={commitRailWidth}
        >
          <input
            type="text"
            className="agents-filter"
            aria-label="Filter skills and memories"
            placeholder="Filter skills and memories…"
            value={query}
            onChange={event => setQuery(event.target.value)}
          />
          <RailSection className="agents-group" title="Harness">
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
          </RailSection>
          <RailScroll className="agents-workspaces">
            {running.length > 0 && (
              <RailSection className="agents-group" title="Running">
                {running.map(workspaceRow)}
              </RailSection>
            )}
            <RailSection className="agents-group" title="Projects">
              {workspaces.length === 0 && <span className="agent-note">No workspace found under the roots.</span>}
              {projects.map(workspaceRow)}
            </RailSection>
          </RailScroll>
        </Rail>

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

        <TableColumn />
        <ResidentColumn active={active} tab="agents" reference={`agents ${folder} ${harness}`} />
      </div>
    </div>
  )
}
