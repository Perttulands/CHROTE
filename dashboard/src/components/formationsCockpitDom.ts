/* Pure DOM/geometry helpers for the Formations cockpit: hit-testing wire ports
   under the pointer, classifying connections, parsing hand-routed lanes, and
   small input utilities. Extracted from FormationsCockpit. */
import type { BoardConnection } from './formationsTypes'

export function splitList(value: string): string[] {
  return value.split(',').map(item => item.trim()).filter(Boolean)
}

/** Collapse a produced output payload to a single-line card-sized preview; full text lives in the title. */
export function truncatePayload(value: string, max = 120): string {
  const single = value.replace(/\s+/g, ' ').trim()
  return single.length > max ? `${single.slice(0, max - 1)}…` : single
}

export function isTextEditingTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  return target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable
}

function findPortAt(clientX: number, clientY: number, selector: string): HTMLElement | null {
  const direct = (document.elementFromPoint(clientX, clientY) as HTMLElement | null)?.closest<HTMLElement>(selector)
  if (direct) return direct
  const ports = Array.from(document.querySelectorAll<HTMLElement>(selector.split(',').map(part => `.fmx ${part.trim()}`).join(',')))
  let best: { element: HTMLElement; distance: number } | null = null
  for (const port of ports) {
    const rect = port.getBoundingClientRect()
    const margin = 18
    if (clientX < rect.left - margin || clientX > rect.right + margin || clientY < rect.top - margin || clientY > rect.bottom + margin) continue
    const cx = rect.left + rect.width / 2
    const cy = rect.top + rect.height / 2
    const distance = Math.hypot(clientX - cx, clientY - cy)
    if (!best || distance < best.distance) best = { element: port, distance }
  }
  return best?.element || null
}

/** New wires and target reconnects land on inputs or a gate's judge socket (reference startWire). */
export function findInputPortAt(clientX: number, clientY: number): HTMLElement | null {
  return findPortAt(clientX, clientY, '[data-port-in],[data-gate-judge-socket]')
}

export function findOutputPortAt(clientX: number, clientY: number): HTMLElement | null {
  return findPortAt(clientX, clientY, '[data-port-out]')
}

export function connectionKind(connection: BoardConnection): 'wire' | 'pass' | 'fail' | 'judge' {
  const fromPort = connection.from.split(':')[1]
  const toPort = connection.to.split(':')[1]
  if (fromPort === 'judge' || toPort === 'judge') return 'judge'
  if (fromPort === 'pass') return 'pass'
  if (fromPort === 'fail') return 'fail'
  return 'wire'
}

/** Parse a hand-routed lane persisted as `y:<worldY>` (legacy `auto`/`manual` → none). */
export function laneYFrom(lane: string | undefined): number | null {
  if (!lane || !lane.startsWith('y:')) return null
  const value = Number(lane.slice(2))
  return Number.isFinite(value) ? value : null
}
