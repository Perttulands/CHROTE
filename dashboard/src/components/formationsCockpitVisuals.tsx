/* Pure visual helpers for the Formations cockpit: type taglines, inline SVG
   glyphs, and deterministic agent-sphere colors/initials/role/state. Extracted
   from FormationsCockpit so the component focuses on stateful canvas logic. */
import type { AgentProjection, FormationNode } from './formationsTypes'

export const TYPE_TAG: Record<FormationNode['type'], string> = {
  solo: 'Do the thing.',
  peer: 'Work together · challenge · synthesize.',
  flow: 'A, then B, then C.',
  orchestrated: 'One controller decides what happens next.',
}

export const GATE_SVG = (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7">
    <path d="M4 21V10a8 8 0 0116 0v11" />
    <path d="M3 21h18M8 21V9M12 21V8M16 21V9" />
  </svg>
)
export const PLAY_SVG = (
  <svg viewBox="0 0 24 24" fill="currentColor"><path d="M6 4l14 8-14 8z" /></svg>
)

function hashHue(id: string): number {
  let h = 0
  for (let i = 0; i < id.length; i += 1) h = (h * 31 + id.charCodeAt(i)) % 360
  return h
}
export function agentColor(id: string): string {
  return `radial-gradient(hsl(${hashHue(id)} 60% 62%), hsl(${hashHue(id)} 55% 34%))`
}
export function initials(id: string): string {
  const cleaned = id.replace(/[^a-zA-Z0-9]/g, '')
  return (cleaned.slice(0, 2) || '?').toUpperCase()
}
export function agentRole(agent: AgentProjection): string {
  return agent.harnessDefault || (agent.unbound ? 'unbound' : 'agent')
}
export function agentState(agent: AgentProjection): 'on' | 'idle' {
  return agent.liveness === 'live' || agent.assignable ? 'on' : 'idle'
}
