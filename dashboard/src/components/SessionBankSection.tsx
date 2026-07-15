import { useMemo } from 'react'
import { useSession } from '../context/SessionContext'
import { useToast } from '../context/ToastContext'
import { copyTextToClipboard } from '../utils/clipboard'
import { apiErrorMessage } from '../apiErrors'
import { getSessionBankRecoveryCapability, summarizeSessionBankCapabilities } from '../sessionBankRecovery'

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

  const capabilitySummary = useMemo(() => summarizeSessionBankCapabilities(bankedSessions), [bankedSessions])
  const countLabels = [
    `${capabilitySummary.total} banked`,
    capabilitySummary.workloadRecoverable > 0 ? `${capabilitySummary.workloadRecoverable} workload recoverable` : '',
    capabilitySummary.topologyOnly > 0 ? `${capabilitySummary.topologyOnly} topology only` : '',
    capabilitySummary.externallyManaged > 0 ? `${capabilitySummary.externallyManaged} managed` : '',
    capabilitySummary.unresolvedUnsafe > 0 ? `${capabilitySummary.unresolvedUnsafe} unresolved` : '',
  ].filter(Boolean)
  const listId = 'settings-session-bank-list'

  if (bankedSessions.length === 0 && !showEmpty) return null

  const copyResumeCommand = async (resumeCommand: string) => {
    const copied = await copyTextToClipboard(resumeCommand)
    addToast(copied ? 'Resume command copied' : 'Failed to copy resume command', copied ? 'success' : 'error')
  }

  const recreateBankedSession = async (name: string, unixUser?: string) => {
    await createSession({ workspaceId: 'terminal1', ...(unixUser ? { unixUser } : {}), name })
  }

  const recoverBankedSession = async (session: typeof sessionBank[number], topologyOnly = false) => {
    try {
      const query = new URLSearchParams()
      if (session.unixUser) query.set('unixUser', session.unixUser)
      const response = await fetch(`/api/tmux/session-bank/${encodeURIComponent(session.name)}/recover${query.toString() ? `?${query.toString()}` : ''}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(topologyOnly ? { topologyOnly: true } : { mouseScroll: settings.mouseScroll }),
        signal: AbortSignal.timeout(10000),
      })
      if (!response.ok) {
        const errorText = await response.text()
        console.error('Failed to recover banked session:', errorText)
        addToast(apiErrorMessage(errorText, topologyOnly ? 'Failed to restore topology' : 'Failed to recover workload'), 'error')
        return
      }
      addToast(topologyOnly
        ? `Restored topology for ${session.name} without launching workloads`
        : `Recovered workload ${session.name}`, 'success')
      await refreshSessions()
    } catch (e) {
      console.error('Failed to recover banked session:', e)
      addToast(topologyOnly ? 'Failed to restore topology' : 'Failed to recover workload', 'error')
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
        const errorText = await response.text()
        console.error('Failed to remove banked session:', errorText)
        addToast(apiErrorMessage(errorText, 'Failed to remove session bank entry'), 'error')
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
        {countLabels.map(label => (
          <span
            key={label}
            className={`session-bank-count ${label.includes('workload') ? 'session-bank-count-workload' : ''}`.trim()}
          >
            {label}
          </span>
        ))}
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
              <p>Offline sessions seen before restart. Typed plans can recover workloads; unsafe or unmanaged entries stay limited.</p>
              {bankedSessions.map(session => {
                const capability = getSessionBankRecoveryCapability(session)
                const resumeCommand = capability.kind === 'legacy-no-plan'
                  ? session.resumeCommand?.trim() || `/resume ${session.name}`
                  : ''
                const ownerLabel = capability.owner ? `Owner ${capability.owner.kind} · ${capability.owner.ref}` : ''
                const canRemove = capability.kind !== 'externally-managed' && capability.kind !== 'unresolved-unsafe'
                return (
                  <article
                    key={`${session.unixUser || 'default'}:${session.name}`}
                    className={`session-bank-item session-bank-item-${capability.kind}`}
                    aria-label={`Session Bank entry ${session.name}`}
                  >
                    <div className="session-bank-main">
                      <div className="session-bank-title-row">
                        <strong>{session.name}</strong>
                        <span className={`session-bank-badge session-bank-badge-${capability.kind}`}>
                          {capability.badgeLabel}
                        </span>
                      </div>
                      <span>{[session.id ? `id ${session.id}` : '', session.unixUser || 'default', `last seen ${new Date(session.lastSeen).toLocaleString()}`].filter(Boolean).join(' · ')}</span>
                      {ownerLabel && <span>{ownerLabel}</span>}
                      <span>{capability.description}</span>
                      {capability.unresolvedReasons.length > 0 && (
                        <span>{`Blocked reason: ${capability.unresolvedReasons.join(', ')}`}</span>
                      )}
                      {capability.canRestoreTopologyOnly && (
                        <span>Limited restore: creates tmux windows and panes only; no workloads launch.</span>
                      )}
                      {resumeCommand && (
                        <code>{resumeCommand}</code>
                      )}
                    </div>
                    <div className="session-bank-actions">
                      {capability.canRecoverWorkload && (
                        <button
                          type="button"
                          className="session-bank-resume"
                          onClick={() => void recoverBankedSession(session)}
                          aria-label={`Recover workload for ${session.name}`}
                        >
                          Recover workload
                        </button>
                      )}
                      {capability.canRestoreTopologyOnly && (
                        <button
                          type="button"
                          className="session-bank-topology"
                          onClick={() => void recoverBankedSession(session, true)}
                          aria-label={`Restore topology only for ${session.name}`}
                        >
                          Restore topology only
                        </button>
                      )}
                      {resumeCommand && (
                        <button
                          type="button"
                          onClick={() => void copyResumeCommand(resumeCommand)}
                          aria-label={`Copy resume command for ${session.name}`}
                        >
                          Copy
                        </button>
                      )}
                      {capability.kind === 'legacy-no-plan' && (
                        <button
                          type="button"
                          onClick={() => void recreateBankedSession(session.name, session.unixUser)}
                          aria-label={`Recreate shell for ${session.name}`}
                        >
                          Recreate shell
                        </button>
                      )}
                      {canRemove && (
                        <button
                          type="button"
                          className="session-bank-remove"
                          onClick={() => void removeBankedSession(session.name, session.unixUser)}
                          aria-label={`Remove ${session.name} from session bank`}
                        >
                          Remove
                        </button>
                      )}
                    </div>
                  </article>
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
