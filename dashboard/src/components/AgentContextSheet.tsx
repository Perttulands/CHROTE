/**
 * What this agent sees, for one session.
 *
 * The way in is the session's own menu, in the Sessions panel or on its tile,
 * because the question is asked about the agent in front of the operator. The
 * answer docks as a sheet over the right of the workspace: the tiles beside it
 * stay readable, so the stack can be read against what the agent is doing with
 * it.
 *
 * A session running a shell has no stack of its own. The panel says so and
 * shows the folder's stack anyway, because a folder is what the next agent
 * started here will load.
 */

import { useCallback, useEffect, useState } from 'react'
import Sheet from './Sheet'
import AgentStack from './AgentStack'
import { SessionCommandMark } from './sessionLabel'
import { useSession } from '../context/SessionContext'
import { getSessionKey, getSessionNameFromKey } from '../types'
import {
  closeAgentContext,
  useAgentContextRequest,
} from '../agents/agentContextPanel'
import { fetchAgentContext, type AgentContext } from '../agents/agentContextApi'
import './AgentContextSheet.css'

/** The share of the workspace the panel takes, as the contract fixes it. */
export const AGENT_CONTEXT_EXTENT = '60%'

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

  const close = useCallback(() => closeAgentContext(), [])

  if (request === null) return null

  const name = getSessionNameFromKey(request.sessionKey)
  const session = sessions.find(candidate => getSessionKey(candidate.name, candidate.unixUser) === request.sessionKey)

  const header = (
    <>
      <span className="agent-sheet-name">{name}</span>
      <SessionCommandMark command={session?.currentCommand} />
      <span className="agent-sheet-folder">{folder || 'no working directory'}</span>
      {user && <span className="agent-sheet-user">{user}</span>}
      <span className="agent-sheet-actions">
        <button
          type="button"
          className="agent-word"
          onClick={() => openSendToSession({
            targetSessionKey: request.sessionKey,
            reference: `agents ${folder} ${harness}`,
          })}
        >
          Send
        </button>
        <button type="button" className="agent-word" aria-label="Close" onClick={close}>×</button>
      </span>
    </>
  )

  return (
    <Sheet
      open
      edge="right"
      extent={AGENT_CONTEXT_EXTENT}
      label={`What ${name} sees`}
      onClose={close}
      header={header}
    >
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
    </Sheet>
  )
}
