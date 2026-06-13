import { FormEvent, useCallback, useEffect, useState } from 'react'

interface ApiResponse<T> {
  success: boolean
  data?: T
  error?: { code: string; message: string }
}

interface HarnessVariant {
  id: string
  sessionStem?: string
  launch?: string
  source?: string
}

interface AgentProjection {
  id: string
  displayName?: string
  kind?: string
  tags?: string[]
  harnessDefault?: string
  harnessVariants?: HarnessVariant[]
  liveness: string
  sessionId?: string
  assignable: boolean
  unbound?: boolean
}

interface PersonaCard {
  id: string
  displayName?: string
  kind: string
  tags: string[]
  harnessDefault: string
  harnessVariants: HarnessVariant[]
}

async function fetchApi<T>(endpoint: string, init?: RequestInit): Promise<T> {
  const response = await fetch(endpoint, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...(init?.headers || {}),
    },
    signal: AbortSignal.timeout(10000),
  })
  const result = await response.json() as ApiResponse<T>
  if (!response.ok || !result.success || !result.data) {
    throw new Error(result.error?.message || `Request failed: ${response.status}`)
  }
  return result.data
}

export default function AgentsView() {
  const [agents, setAgents] = useState<AgentProjection[]>([])
  const [selected, setSelected] = useState<PersonaCard | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [id, setId] = useState('')
  const [kind, setKind] = useState('specialist')
  const [capabilities, setCapabilities] = useState('')

  const refresh = useCallback(async () => {
    try {
      const data = await fetchApi<{ agents: AgentProjection[]; count: number }>('/api/agents')
      setAgents(data.agents)
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Agent roster request failed')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    refresh()
  }, [refresh])

  const inspect = useCallback(async (agentId: string) => {
    try {
      const card = await fetchApi<PersonaCard>(`/api/agents/${encodeURIComponent(agentId)}`)
      setSelected(card)
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Agent inspect request failed')
    }
  }, [])

  const createAgent = useCallback(async (event: FormEvent) => {
    event.preventDefault()
    const capabilityList = capabilities
      .split(',')
      .map(value => value.trim())
      .filter(Boolean)
    try {
      const card = await fetchApi<PersonaCard>('/api/agents', {
        method: 'POST',
        body: JSON.stringify({
          id: id.trim(),
          kind: kind.trim(),
          harness: 'claude-code',
          capabilities: capabilityList,
        }),
      })
      setSelected(card)
      setId('')
      setKind('specialist')
      setCapabilities('')
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Agent create request failed')
    }
  }, [capabilities, id, kind, refresh])

  return (
    <div className="agents-view" data-testid="agents-view">
      <div className="oracle-status-bar">
        <div className="oracle-status-items">
          <span className="oracle-stat">Agents: {loading ? '--' : agents.length}</span>
        </div>
        <div className="oracle-status-right">
          <button className="oracle-refresh-btn" type="button" onClick={refresh} disabled={loading}>
            {loading ? 'Loading...' : 'Refresh'}
          </button>
        </div>
      </div>

      {error && <div className="oracle-empty" role="alert">{error}</div>}

      <div className="oracle-content">
        <div className="oracle-agents-section">
          <form className="agent-create-form" onSubmit={createAgent} aria-label="Create agent">
            <input
              aria-label="Agent id"
              value={id}
              onChange={event => setId(event.target.value)}
              placeholder="agent-id"
            />
            <input
              aria-label="Kind"
              value={kind}
              onChange={event => setKind(event.target.value)}
              placeholder="specialist"
            />
            <input
              aria-label="Capabilities"
              value={capabilities}
              onChange={event => setCapabilities(event.target.value)}
              placeholder="writing, voice"
            />
            <button type="submit" className="oracle-refresh-btn">Create</button>
          </form>

          <div className="oracle-agents-grid">
            {agents.map(agent => (
              <button
                key={agent.id}
                type="button"
                className={`oracle-agent-card oracle-status-${agent.liveness === 'live' ? 'idle' : 'complete'}`}
                onClick={() => !agent.unbound && inspect(agent.id)}
              >
                <div className="oracle-agent-header">
                  <span className="oracle-agent-name">{agent.displayName || agent.id}</span>
                  <span className="oracle-status-badge oracle-badge-idle">{agent.liveness}</span>
                </div>
                <div className="oracle-agent-meta">
                  <span>{agent.kind || (agent.unbound ? 'unbound' : 'agent')}</span>
                  {agent.sessionId && <span>{agent.sessionId}</span>}
                </div>
                <div className="oracle-agent-output">
                  {(agent.tags || []).map(tag => (
                    <span key={tag} className="oracle-output-line">{tag}</span>
                  ))}
                </div>
              </button>
            ))}
          </div>
        </div>

        <div className="oracle-sidebar">
          {selected && (
            <div className="oracle-sidebar-section">
              <h3 className="oracle-section-title">{selected.displayName || selected.id}</h3>
              <div className="oracle-agent-output">
                <div className="oracle-output-line">{selected.kind}</div>
                <div className="oracle-output-line">{selected.harnessDefault}</div>
                {selected.harnessVariants.map(variant => (
                  <div className="oracle-output-line" key={variant.id}>
                    {variant.id} {variant.sessionStem || selected.id} {variant.source || ''}
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
