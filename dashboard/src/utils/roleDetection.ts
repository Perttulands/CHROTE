// Gastown Agent Role Detection
// Detects agent roles from session name patterns

export type AgentRole = 'mayor' | 'deacon' | 'witness' | 'polecat' | 'refinery' | 'crew'

export interface RoleInfo {
  role: AgentRole
  emoji: string
  name: string
  color: string
  glowColor: string
}

// Role configuration with display info and theme colors
export const ROLE_CONFIG: Record<AgentRole, Omit<RoleInfo, 'role'>> = {
  mayor: {
    emoji: '👑',
    name: 'Mayor',
    color: '#ffd700',      // Gold
    glowColor: 'rgba(255, 215, 0, 0.5)',
  },
  deacon: {
    emoji: '📿',
    name: 'Deacon',
    color: '#9966ff',      // Purple
    glowColor: 'rgba(153, 102, 255, 0.5)',
  },
  witness: {
    emoji: '👁',
    name: 'Witness',
    color: '#00bfff',      // Deep sky blue
    glowColor: 'rgba(0, 191, 255, 0.5)',
  },
  polecat: {
    emoji: '🦨',
    name: 'Polecat',
    color: '#ff6b9d',      // Pink
    glowColor: 'rgba(255, 107, 157, 0.5)',
  },
  refinery: {
    emoji: '🏭',
    name: 'Refinery',
    color: '#ff9933',      // Orange
    glowColor: 'rgba(255, 153, 51, 0.5)',
  },
  crew: {
    emoji: '🔧',
    name: 'Crew',
    color: '#4ade80',      // Green
    glowColor: 'rgba(74, 222, 128, 0.5)',
  },
}

/**
 * Detect the Gastown agent role from a session name
 * @param sessionName The tmux session name
 * @returns The detected role or null if no role pattern matches
 */
export function detectAgentRole(sessionName: string): AgentRole | null {
  const name = sessionName.toLowerCase()

  // Mayor - HQ coordination role
  if (name.startsWith('hq-mayor') || name === 'mayor' || name.includes('-mayor')) {
    return 'mayor'
  }

  // Deacon - HQ assistant role
  if (name.startsWith('hq-deacon') || name === 'deacon' || name.includes('-deacon')) {
    return 'deacon'
  }

  // Witness - observer role
  if (name.includes('-witness') || name === 'witness' || name.startsWith('witness')) {
    return 'witness'
  }

  // Polecat - worker role (matches gt-*-pc-* pattern or polecat keyword)
  if (name.includes('-polecat') || name === 'polecat' || /-pc-/.test(name) || name.endsWith('-pc')) {
    return 'polecat'
  }

  // Refinery - processing role
  if (name.includes('-refinery') || name === 'refinery' || name.startsWith('refinery')) {
    return 'refinery'
  }

  // Crew - general worker role
  if (name.includes('-crew-') || name.includes('-crew') || name === 'crew' || name.startsWith('crew-')) {
    return 'crew'
  }

  return null
}

/**
 * Get full role information for a session name
 * @param sessionName The tmux session name
 * @returns Full role info or null if no role matches
 */
export function getRoleInfo(sessionName: string): RoleInfo | null {
  const role = detectAgentRole(sessionName)
  if (!role) return null

  return {
    role,
    ...ROLE_CONFIG[role],
  }
}
