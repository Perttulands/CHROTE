import { getRoleInfo, type AgentRole } from '../utils/roleDetection'

interface RoleBadgeProps {
  sessionName: string
  compact?: boolean // For use in SessionTag (smaller size)
}

/**
 * Displays a role badge with emoji and tooltip for Gastown agent sessions
 * Shows nothing if no role is detected from the session name
 */
function RoleBadge({ sessionName, compact = false }: RoleBadgeProps) {
  const roleInfo = getRoleInfo(sessionName)

  if (!roleInfo) {
    return null
  }

  const badgeStyle = {
    '--role-color': roleInfo.color,
    '--role-glow': roleInfo.glowColor,
  } as React.CSSProperties

  return (
    <span
      className={`role-badge ${compact ? 'role-badge-compact' : ''} role-${roleInfo.role}`}
      style={badgeStyle}
      title={roleInfo.name}
      aria-label={roleInfo.name}
    >
      {roleInfo.emoji}
    </span>
  )
}

export default RoleBadge
export type { AgentRole }
