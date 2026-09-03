/**
 * What this agent sees, for one session.
 *
 * The way in is the session's own menu, in the Sessions panel or on its tile,
 * because the question is asked about the agent in front of the operator. The
 * answer goes on the table, in the column at the right of the tab: the tiles
 * beside it stay readable, so the stack can be read against what the agent is
 * doing with it.
 *
 * A session running a shell has no stack of its own. The panel says so and
 * shows the folder's stack anyway, because a folder is what the next agent
 * started here will load.
 */

import { useEffect, useState } from 'react'
import AgentStack from './AgentStack'
import { SessionCommandMark } from './sessionLabel'
import { useSession } from '../context/SessionContext'
import { getSessionKey, getSessionNameFromKey } from '../types'
import { closeAgentContext, useAgentContextRequest } from '../agents/agentContextPanel'
import { fetchAgentContext, type AgentContext } from '../agents/agentContextApi'
import './AgentContextSheet.css'

export default function AgentContextSheet() {
  const request = useAgentContextRequest()
  const { sessions, openSendToSession } = useSession()
  const [context, setContext] = useState<AgentContext | null>(null)
  const [error, setError] = useState<string | null>(null)

  const folder = request?.folder ?? ''
  const harness = request?.harness ?? 'claude-code'
  const user = request?.user ?? ''

  useEffect(() => {
    if (request === null || !folder) {
      setContext(null)
      setError(null)
      return
    }
    let current = true
    setContext(null)
    setError(null)
    fetchAgentContext(folder, harness, user)
      .then(resolved => { if (current) setContext(resolved) })
      .catch((cause: unknown) => {
        if (!current) return
        setError(cause instanceof Error ? cause.message : 'Could not resolve what this agent sees')
      })
    return () => { current = false }
  }, [request?.nonce, folder, harness, user])

  if (request === null) return null

  const name = getSessionNameFromKey(request.sessionKey)
  const session = sessions.find(candidate => getSessionKey(candidate.name, candidate.unixUser) === request.sessionKey)

  return (
    <div className="agent-sheet" data-ui="agents.sheet">
      <div className="table-header">
        <span className="agent-sheet-name">{name}</span>
        <SessionCommandMark command={session?.currentCommand} />
        <span className="agent-sheet-folder">{folder || 'no working directory'}</span>
        {user && <span className="agent-sheet-user">{user}</span>}
        <span className="table-header-spacer" />
        <button
          type="button"
          className="table-action"
          onClick={() => openSendToSession({
            targetSessionKey: request.sessionKey,
            reference: `agents ${folder} ${harness}`,
          })}
        >
          Send
        </button>
        <button type="button" className="table-action" aria-keyshortcuts="Escape" onClick={closeAgentContext}>
          Close<span className="table-chord" aria-hidden="true">Esc</span>
        </button>
      </div>
      <div className="agent-sheet-body">
        <p className="agent-sheet-purpose">
          {request.shell
            ? 'This session runs a shell. This is the stack an agent started here would load.'
            : 'Everything this agent loaded before it read a word from you.'}
        </p>
        {!folder && <div className="agent-note">This session reports no working directory.</div>}
        {error !== null && <div className="agent-note">{error}</div>}
        {folder && error === null && context === null && <div className="agent-note">Resolving…</div>}
        {context !== null && <AgentStack context={context} />}
      </div>
    </div>
  )
}
