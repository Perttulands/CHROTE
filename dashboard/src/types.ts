// Arena Dashboard Types

// User settings for persistent configuration
export interface UserSettings {
  terminalMode: 'tmux' | 'shell'    // Default mode for new terminals
  fontSize: number                   // Terminal font size (12-20)
  theme: 'matrix' | 'dark' | 'gastown' // Color theme
  autoRefreshInterval: number        // Session refresh interval in ms (1000-30000)
  defaultSessionPrefix: string       // Prefix for new sessions (e.g., 'shell')
  musicVolume: number                // Music volume (0-1)
  musicEnabled: boolean              // Whether music is playing
}

export const DEFAULT_SETTINGS: UserSettings = {
  terminalMode: 'tmux',
  fontSize: 14,
  theme: 'matrix',
  autoRefreshInterval: 5000,
  defaultSessionPrefix: 'shell',
  musicVolume: 0.5,
  musicEnabled: false,
}

export interface TmuxSession {
  name: string
  agentName: string
  windows: number
  attached: boolean
  group: string
}

export interface SessionsResponse {
  sessions: TmuxSession[]
  grouped: Record<string, TmuxSession[]>
  timestamp: string
  error?: string
}

export interface TerminalWindow {
  id: string
  boundSessions: string[] // Session names bound to this window
  activeSession: string | null // Currently displayed session
  colorIndex: number // 0-3 for window color theme
}

export interface DashboardState {
  // Session data from API
  sessions: TmuxSession[]
  groupedSessions: Record<string, TmuxSession[]>
  loading: boolean
  error: string | null

  // Window configuration
  windows: TerminalWindow[]
  windowCount: number // 1-4
  focusedWindowIndex: number // 0-based index of focused window

  // UI state
  sidebarCollapsed: boolean
  floatingSession: string | null // Session shown in floating modal
  isDragging: boolean // True when a session is being dragged

  // Computed: which sessions are assigned to any window
  assignedSessions: Map<string, { windowId: string; colorIndex: number; windowIndex: number }>

  // User settings
  settings: UserSettings
}

export interface DashboardActions {
  // Window management
  setWindowCount: (count: number) => void
  addSessionToWindow: (windowId: string, sessionName: string) => void
  removeSessionFromWindow: (windowId: string, sessionName: string) => void
  setActiveSession: (windowId: string, sessionName: string) => void
  cycleSession: (windowId: string, direction: 'prev' | 'next') => void
  cycleWindow: (direction: 'prev' | 'next') => void
  setFocusedWindowIndex: (index: number) => void

  // UI actions
  toggleSidebar: () => void
  openFloatingModal: (sessionName: string) => void
  closeFloatingModal: () => void

  // Session click handler
  handleSessionClick: (sessionName: string) => void

  // Refresh sessions from API
  refreshSessions: () => Promise<void>

  // Drag state
  setIsDragging: (dragging: boolean) => void

  // Settings
  updateSettings: (settings: Partial<UserSettings>) => void
}

export type DashboardContextType = DashboardState & DashboardActions

// Window color themes
export const WINDOW_COLORS = [
  { name: 'blue', bg: '#0a0a1a', border: '#4a9eff', accent: '#4a9eff' },
  { name: 'purple', bg: '#0f0a1a', border: '#9966ff', accent: '#9966ff' },
  { name: 'green', bg: '#0a1a0a', border: '#00ff41', accent: '#00ff41' },
  { name: 'orange', bg: '#1a140a', border: '#ff9933', accent: '#ff9933' },
] as const

// Group display names and priorities
export const GROUP_CONFIG: Record<string, { displayName: string; priority: number }> = {
  'hq': { displayName: 'HQ', priority: 0 },
  'main': { displayName: 'Main', priority: 1 },
  'other': { displayName: 'Other', priority: 100 },
}

export function getGroupDisplayName(group: string): string {
  if (GROUP_CONFIG[group]) return GROUP_CONFIG[group].displayName
  if (group.startsWith('gt-')) {
    // gt-gastown → Gastown
    const rigName = group.slice(3)
    return rigName.charAt(0).toUpperCase() + rigName.slice(1)
  }
  return group
}

export function getGroupPriority(group: string): number {
  if (GROUP_CONFIG[group]) return GROUP_CONFIG[group].priority
  if (group.startsWith('gt-')) return 2 // Rigs after main
  return 99
}
