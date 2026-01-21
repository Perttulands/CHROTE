// Beads TypeScript types - requires bv CLI, no fallbacks

export type IssueStatus =
  | 'open'
  | 'in_progress'
  | 'blocked'
  | 'closed'
  | 'ready'
  | 'wont_fix'
  | 'duplicate'
  | 'deferred'
  | 'hooked'

export type IssueType = 'bug' | 'feature' | 'task' | 'chore' | 'epic'

export type ImpactLevel = 'high' | 'medium' | 'low'

export interface BeadsIssue {
  id: string
  title: string
  status: IssueStatus
  priority?: number // 1 = highest
  type?: IssueType
  dependencies?: string[] // issue IDs this blocks
  labels?: string[]
  assignee?: string
  created?: string
  updated?: string
  description?: string
}

export interface BeadsProject {
  name: string
  path: string
  beadsPath: string
}

export interface TriageRecommendation {
  issueId: string
  rank: number
  reasoning: string
  estimatedImpact: ImpactLevel
}

export interface TriageResponse {
  recommendations: TriageRecommendation[]
  quickWins: string[]
  blockers: string[]
  data_hash?: string
}

export interface HealthInfo {
  score: number // 0-100
  risks: string[]
  warnings: string[]
}

export interface InsightsResponse {
  issueCount: number
  openCount: number
  blockedCount: number
  closedCount?: number
  byStatus?: Record<string, number>
  byType?: Record<string, number>
  health: HealthInfo
  metrics?: {
    pageRank?: Record<string, number>
    betweenness?: Record<string, number>
    degree?: Record<string, number>
    cycles?: string[][]
    density?: number
    criticalPath?: string[]
  }
  data_hash?: string
}

export interface GraphNode {
  id: string
  title: string
  status: IssueStatus
  priority?: number
  type?: IssueType
}

export interface GraphEdge {
  source: string
  target: string
}

export interface GraphResponse {
  nodes: GraphNode[]
  edges: GraphEdge[]
}

// API response wrapper
export interface ApiResponse<T> {
  success: boolean
  timestamp: string
  data?: T
  error?: {
    code: string
    message: string
  }
}

// Sub-tab types
export type BeadsSubTab = 'kanban' | 'triage' | 'insights'
