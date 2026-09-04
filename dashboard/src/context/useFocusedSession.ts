import { useMemo } from 'react'
import { useSession } from './SessionContext'

/**
 * The session the focused tile is showing, or null when no tile is focused and
 * when the focused tile is empty. The focus key and the workspaces are both
 * held by the session context, so the question they answer together is asked
 * here rather than in each reader — four of them had copied the walk, and a
 * copy is what lets the Send target, a row's focus mark and the seen-on-focus
 * post disagree about which terminal the operator is looking at.
 */
export function useFocusedSession(): string | null {
  const { focusedWindowKey, workspaces } = useSession()
  return useMemo(() => {
    if (!focusedWindowKey) return null
    for (const [workspaceId, workspace] of Object.entries(workspaces)) {
      const focused = workspace.windows.find(candidate => `${workspaceId}-${candidate.id}` === focusedWindowKey)
      if (focused) return focused.activeSession ?? null
    }
    return null
  }, [focusedWindowKey, workspaces])
}
