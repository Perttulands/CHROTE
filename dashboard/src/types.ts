// Arena Dashboard Types

// tmux appearance customization (hot-reloadable)
export interface TmuxAppearance {
  statusBg: string           // Status bar background color (e.g., "#000000", "black")
  statusFg: string           // Status bar foreground color (e.g., "#00ff41", "green")
  paneBorderActive: string   // Active pane border color
  paneBorderInactive: string // Inactive pane border color
  modeStyleBg: string        // Copy-mode/selection background
  modeStyleFg: string        // Copy-mode/selection foreground
}

export const DEFAULT_TMUX_APPEARANCE: TmuxAppearance = {
  statusBg: 'default',
  statusFg: '#00ff41',
  paneBorderActive: '#00ff41',
  paneBorderInactive: '#333333',
  modeStyleBg: '#00ff41',
  modeStyleFg: '#000000',
}

// tmux appearance presets matching dashboard themes
export const TMUX_PRESETS: Record<string, TmuxAppearance> = {
  matrix: {
    statusBg: 'default',
    statusFg: '#00ff41',
    paneBorderActive: '#00ff41',
    paneBorderInactive: '#333333',
    modeStyleBg: '#00ff41',
    modeStyleFg: '#000000',
  },
  dark: {
    statusBg: 'default',
    statusFg: '#6b9fff',
    paneBorderActive: '#6b9fff',
    paneBorderInactive: '#3a3a3a',
    modeStyleBg: '#6b9fff',
    modeStyleFg: '#0f0f0f',
  },
  gastown: {
    statusBg: 'default',
    statusFg: '#f6cd54',
    paneBorderActive: '#f6cd54',
    paneBorderInactive: '#4a2518',
    modeStyleBg: '#f6cd54',
    modeStyleFg: '#32160f',
  },
}

// User settings for persistent configuration
export interface UserSettings {
  terminalMode: 'tmux'              // Terminal mode (tmux only)
  fontSize: number                   // Terminal font size (12-20)
  theme: 'matrix' | 'dark' | 'gastown' // Color theme
  autoRefreshInterval: number        // Session refresh interval in ms (1000-30000)
  defaultSessionPrefix: string       // Prefix for new sessions (e.g., 'shell')
  musicVolume: number                // Music volume (0-1)
  musicEnabled: boolean              // Whether music is playing
  tmuxAppearance: TmuxAppearance     // tmux color customization
  beadsProjectPaths?: string[]       // Manually added beads project paths
}

export const DEFAULT_SETTINGS: UserSettings = {
  terminalMode: 'tmux',
  fontSize: 14,
  theme: 'matrix',
  autoRefreshInterval: 5000,
  defaultSessionPrefix: 'shell',
  musicVolume: 0.5,
  musicEnabled: false,
  tmuxAppearance: DEFAULT_TMUX_APPEARANCE,
}

export interface TmuxSession {
  name: string
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

export type WorkspaceId = 'terminal1' | 'terminal2'

export interface TerminalWorkspace {
  windows: TerminalWindow[]
  windowCount: number // 1-4
}

// Layout preset for saving/restoring window configurations
export interface LayoutPreset {
  id: string
  name: string
  createdAt: number
  workspaces: Record<WorkspaceId, TerminalWorkspace>
}

export const MAX_PRESETS = 10

export interface DashboardState {
  // Session data from API
  sessions: TmuxSession[]
  groupedSessions: Record<string, TmuxSession[]>
  loading: boolean
  error: string | null

  // Window configuration
  workspaces: Record<WorkspaceId, TerminalWorkspace>

  // UI state
  sidebarCollapsed: boolean
  floatingSession: string | null // Session shown in floating modal
  isDragging: boolean // True when a session is being dragged

  // Computed: which sessions are assigned to any window
  assignedSessions: Map<string, { workspaceId: WorkspaceId; windowId: string; colorIndex: number; windowIndex: number }>

  // User settings
  settings: UserSettings

  // Focused window for keyboard navigation (workspaceId-windowId format)
  focusedWindowKey: string | null

  // Layout presets
  layoutPresets: LayoutPreset[]
}

export interface DashboardActions {
  // Window management
  setWindowCount: (workspaceId: WorkspaceId, count: number) => void
  addSessionToWindow: (workspaceId: WorkspaceId, windowId: string, sessionName: string) => void
  removeSessionFromWindow: (workspaceId: WorkspaceId, windowId: string, sessionName: string) => void
  setActiveSession: (workspaceId: WorkspaceId, windowId: string, sessionName: string) => void
  cycleSession: (workspaceId: WorkspaceId, windowId: string, direction: 'prev' | 'next') => void

  // UI actions
  toggleSidebar: () => void
  openFloatingModal: (sessionName: string) => void
  closeFloatingModal: () => void

  // Session click handler
  handleSessionClick: (sessionName: string) => void

  // Refresh sessions from API
  refreshSessions: () => Promise<void>

  // Delete a session
  deleteSession: (sessionName: string) => Promise<void>

  // Rename a session
  renameSession: (oldName: string, newName: string) => Promise<boolean>

  // Drag state
  setIsDragging: (dragging: boolean) => void

  // Settings
  updateSettings: (settings: Partial<UserSettings>) => void

  // Focus tracking for keyboard navigation
  setFocusedWindowKey: (key: string | null) => void

  // Layout presets
  saveCurrentLayout: (name: string) => boolean
  loadPreset: (presetId: string) => void
  deletePreset: (presetId: string) => void
  renamePreset: (presetId: string, newName: string) => void
}

export type DashboardContextType = DashboardState & DashboardActions

// Window color themes
export const WINDOW_COLORS = [
  { name: 'blue', bg: 'rgba(10, 10, 26, 0.85)', border: '#4a9eff', accent: '#4a9eff' },
  { name: 'purple', bg: 'rgba(15, 10, 26, 0.85)', border: '#9966ff', accent: '#9966ff' },
  { name: 'green', bg: 'rgba(10, 26, 10, 0.85)', border: '#00ff41', accent: '#00ff41' },
  { name: 'orange', bg: 'rgba(26, 20, 10, 0.85)', border: '#ff9933', accent: '#ff9933' },
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
  if (group.startsWith('gt-')) return 3 // Rigs after main
  return 99
}
