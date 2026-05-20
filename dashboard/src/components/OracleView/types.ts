// Oracle TypeScript types

export type AgentStatus = 'idle' | 'working' | 'complete' | 'error'

export interface OracleAgent {
  name: string
  status: AgentStatus
  contextPct: number
  beadId?: string
  lastLines: string[]
  startedAt?: string
  group: string
  attached: boolean
}

export interface OracleStatusData {
  totalAgents: number
  workingAgents: number
  idleAgents: number
  beadsActive: number
  sseClients: number
}

export interface OracleEvent {
  type: 'agent_new' | 'agent_status' | 'agent_removed' | 'heartbeat' | 'connected'
  data: Record<string, unknown>
  timestamp: string
}

export interface ActivityEntry {
  id: string
  type: string
  agentName: string
  message: string
  timestamp: string
}

export interface RalphProject {
  project: string
  path: string
  status: Record<string, unknown>
}

export interface ApiResponse<T> {
  success: boolean
  timestamp: string
  data?: T
  error?: {
    code: string
    message: string
  }
}
