/* Pure visual helpers for the Formations cockpit: type taglines, inline SVG
   glyphs, and agent initials/role/state. Extracted from FormationsCockpit so
   the component focuses on stateful canvas logic. */
import type { AgentProjection, FormationNode } from './formationsTypes'
import { harnessIcon } from './harnessIcons'

export const TYPE_TAG: Record<FormationNode['type'], string> = {
  solo: 'Do the thing.',
  peer: 'Work together · challenge · synthesize.',
  flow: 'A, then B, then C.',
  orchestrated: 'One controller decides what happens next.',
}

/* Castle wall with an opening: gates are checkpoints work must pass through. */
export const GATE_SVG = (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" aria-hidden="true">
    <path d="M4 20V6h3.5v2.5h3V6h3v2.5h3V6H20v14" />
    <path d="M9.5 20v-4a2.5 2.5 0 015 0v4" />
  </svg>
)

/* Harness product marks live in the shared library; this wrapper keeps the
   cockpit's call sites stable. */
export function harnessGlyph(harness: string | undefined | null): JSX.Element | null {
  return harnessIcon(harness)
}
export const PLAY_SVG = (
  <svg viewBox="0 0 24 24" fill="currentColor"><path d="M6 4l14 8-14 8z" /></svg>
)

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
