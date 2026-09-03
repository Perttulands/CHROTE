/**
 * Put the cursor in the active workspace's session search, opening the
 * Sessions panel first if it is shut. Reached from plain `/` outside a
 * terminal and from the leader's `/` chord, which is why it lives beside the
 * registry rather than inside either caller.
 */
export function focusSessionSearch(): boolean {
  const activeDock = document.querySelector('.terminal-workspace-dock[data-active="true"]')
  if (!activeDock) return false

  const focusVisibleSearch = () => {
    const searchInput = activeDock.querySelector('.session-search-input') as HTMLInputElement | null
    searchInput?.focus()
    searchInput?.select()
    return searchInput !== null
  }

  if (focusVisibleSearch()) return true

  const sessionsTrigger = activeDock.querySelector('button[aria-label="Sessions sidecar"]') as HTMLButtonElement | null
  if (!sessionsTrigger) return false
  sessionsTrigger.click()
  queueMicrotask(focusVisibleSearch)
  return true
}
