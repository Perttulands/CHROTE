import { useMemo } from 'react'
import { useSession } from '../context/SessionContext'
import { useToast } from '../context/ToastContext'
import { copyTextToClipboard } from '../utils/clipboard'

type SessionBankSectionProps = {
  collapsed: boolean
  onCollapsedChange: (collapsed: boolean) => void
  searchTerm?: string
  showEmpty?: boolean
  className?: string
}

function SessionBankSection({
  collapsed,
  onCollapsedChange,
  searchTerm = '',
  showEmpty = true,
  className = '',
}: SessionBankSectionProps) {
  const { sessionBank, refreshSessions, createSession, settings } = useSession()
  const { addToast } = useToast()

  const bankedSessions = useMemo(() => {
    const needle = searchTerm.trim().toLowerCase()
    return sessionBank
      .filter(session => !session.live)
      .filter(session => {
        if (!needle) return true
        return [
          session.name,
          session.unixUser ?? '',
          session.resumeCommand ?? '',
          session.agentKind ?? '',
          session.agentSessionId ?? '',
          session.cwd ?? '',
        ].some(value => value.toLowerCase().includes(needle))
      })
  }, [sessionBank, searchTerm])

  if (bankedSessions.length === 0 && !showEmpty) return null

  const recoverableLabel = `${bankedSessions.length} ${bankedSessions.length === 1 ? 'recoverable' : 'recoverable'}`
  const listId = 'settings-session-bank-list'

  const copyResumeCommand = async (resumeCommand: string) => {
    const copied = await copyTextToClipboard(resumeCommand)
    addToast(copied ? 'Resume command copied' : 'Failed to copy resume command', copied ? 'success' : 'error')
  }

  const recreateBankedSession = async (name: string, unixUser?: string) => {
    await createSession({ workspaceId: 'terminal1', ...(unixUser ? { unixUser } : {}), name })
  }

  const isRecoverableAgent = (session: typeof sessionBank[number]) => (
    session.recoveryKind === 'agent' && Boolean(session.agentKind && session.agentSessionId && session.resumeCommand?.trim())
  )

  const resumeBankedAgent = async (session: typeof sessionBank[number]) => {
    try {
      const query = new URLSearchParams()
      if (session.unixUser) query.set('unixUser', session.unixUser)
      const response = await fetch(`/api/tmux/session-bank/${encodeURIComponent(session.name)}/recover${query.toString() ? `?${query.toString()}` : ''}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ mouseScroll: settings.mouseScroll }),
        signal: AbortSignal.timeout(10000),
      })
      if (!response.ok) {
        console.error('Failed to resume banked agent:', await response.text())
        addToast('Failed to resume agent', 'error')
        return
      }
      addToast(`Resumed agent ${session.name}`, 'success')
      await refreshSessions()
    } catch (e) {
      console.error('Failed to resume banked agent:', e)
      addToast('Failed to resume agent', 'error')
    }
  }

  const removeBankedSession = async (name: string, unixUser?: string) => {
    try {
      const query = new URLSearchParams()
      if (unixUser) query.set('unixUser', unixUser)
      const response = await fetch(`/api/tmux/session-bank/${encodeURIComponent(name)}${query.toString() ? `?${query.toString()}` : ''}`, {
        method: 'DELETE',
        signal: AbortSignal.timeout(10000),
      })
      if (!response.ok) {
        console.error('Failed to remove banked session:', await response.text())
        addToast('Failed to remove session bank entry', 'error')
        return
      }
      addToast(`Removed ${name} from session bank`, 'success')
      await refreshSessions()
    } catch (e) {
      console.error('Failed to remove banked session:', e)
      addToast('Failed to remove session bank entry', 'error')
    }
  }

  return (
    <section className={`session-bank ${collapsed ? 'session-bank-collapsed' : ''} ${className}`.trim()} aria-label="Session Bank">
      <div className="session-bank-header">
        <h2>Session Bank</h2>
        <span className="session-bank-count">{recoverableLabel}</span>
        <button
          type="button"
          className="session-bank-toggle"
          aria-label={collapsed ? 'Expand session bank' : 'Collapse session bank'}
          aria-expanded={!collapsed}
          aria-controls={listId}
          onClick={() => onCollapsedChange(!collapsed)}
        >
          {collapsed ? '▸' : '▾'}
        </button>
      </div>
      {!collapsed && (
        <div id={listId} className="session-bank-list">
          {bankedSessions.length > 0 ? (
            <>
              <p>Offline sessions seen before restart. Resume saved agents directly, or recreate shell-only entries.</p>
              {bankedSessions.map(session => {
                const recoverableAgent = isRecoverableAgent(session)
                const resumeCommand = session.resumeCommand?.trim() || `/resume ${session.name}`
                return (
                  <div key={`${session.unixUser || 'default'}:${session.name}`} className={`session-bank-item ${recoverableAgent ? 'session-bank-item-recoverable' : 'session-bank-item-shell'}`}>
                    <div className="session-bank-main">
                      <div className="session-bank-title-row">
                        <strong>{session.name}</strong>
                        <span className={`session-bank-badge ${recoverableAgent ? 'session-bank-badge-agent' : 'session-bank-badge-shell'}`}>
                          {recoverableAgent ? 'Recoverable agent' : 'Shell only'}
                        </span>
                      </div>
                      <span>{[session.id ? `id ${session.id}` : '', session.unixUser || 'default', `last seen ${new Date(session.lastSeen).toLocaleString()}`].filter(Boolean).join(' · ')}</span>
                      {recoverableAgent ? (
                        <>
                          <span>{[session.agentKind, session.agentSessionId].filter(Boolean).join(' · ')}</span>
                          <code>{resumeCommand}</code>
                        </>
                      ) : (
                        <>
                          <span>No agent resume metadata saved.</span>
                          <code>{resumeCommand}</code>
                        </>
                      )}
                    </div>
                    <div className="session-bank-actions">
                      {recoverableAgent && (
                        <button
                          type="button"
                          className="session-bank-resume"
                          onClick={() => void resumeBankedAgent(session)}
                          aria-label={`Resume agent for ${session.name}`}
                        >
                          Resume agent
                        </button>
                      )}
                      <button
                        type="button"
                        onClick={() => void copyResumeCommand(resumeCommand)}
                        aria-label={`Copy resume command for ${session.name}`}
                      >
                        Copy
                      </button>
                      <button
                        type="button"
                        onClick={() => void recreateBankedSession(session.name, session.unixUser)}
                        aria-label={`Recreate shell for ${session.name}`}
                      >
                        Recreate shell
                      </button>
                      <button
                        type="button"
                        className="session-bank-remove"
                        onClick={() => void removeBankedSession(session.name, session.unixUser)}
                        aria-label={`Remove ${session.name} from session bank`}
                      >
                        Remove
                      </button>
                    </div>
                  </div>
                )
              })}
            </>
          ) : (
            <p>No recoverable sessions in the bank.</p>
          )}
        </div>
      )}
    </section>
  )
}

export default SessionBankSection
