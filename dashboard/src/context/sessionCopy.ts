import type { TmuxSession } from '../types'
import { harnessIdForCommand } from '../components/harnessMarks'

export function copySessionUnavailableReason(session: TmuxSession): string | undefined {
  if (!session.cwd?.trim()) return 'Working directory unavailable'
  if (!harnessIdForCommand(session.currentCommand)) return 'Running command is not a recognized harness'
  return undefined
}

/** Copies share one family even when the operator copies an existing copy. */
export function nextCopiedSessionName(source: string, taken: ReadonlySet<string>): string {
  const match = source.match(/^(.*)-(\d+)$/)
  // A numeric project name is not automatically a copy. Only fold the suffix
  // into a family when its original session is also in the inventory.
  const base = match && Number(match[2]) >= 2 && taken.has(match[1]) ? match[1] : source
  for (let suffix = 2; ; suffix++) {
    const candidate = `${base}-${suffix}`
    if (!taken.has(candidate)) return candidate
  }
}
