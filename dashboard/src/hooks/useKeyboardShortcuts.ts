import { useCallback, useEffect } from 'react'
import { useSession } from '../context/SessionContext'

interface KeyboardShortcutsConfig {
  onShowHelp: () => void
  isHelpOpen: boolean
}

// The terminal is rendered in this document now (ADR-0018), so its keystrokes
// bubble here. Anything typed into a terminal belongs to the shell.
function isTerminal(target: HTMLElement): boolean {
  return Boolean(target.closest('.terminal-window-body, .floating-modal-body'))
}

function isDashboardChrome(target: HTMLElement): boolean {
  if (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable) return false
  return !isTerminal(target)
}

export function useKeyboardShortcuts({ onShowHelp, isHelpOpen }: KeyboardShortcutsConfig) {
  const { closeFloatingModal, floatingSession } = useSession()

  const focusSearchBox = useCallback((): boolean => {
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
  }, [])

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      const target = event.target instanceof HTMLElement
        ? event.target
        : (document.activeElement instanceof HTMLElement ? document.activeElement : document.body)

      if (event.key === 'Escape' && floatingSession && !isTerminal(target)) {
        event.preventDefault()
        closeFloatingModal()
        return
      }

      if (isHelpOpen || !isDashboardChrome(target)) return

      if (event.key === '?' || (event.shiftKey && event.key === '/')) {
        event.preventDefault()
        onShowHelp()
        return
      }

      if (event.key === '/' && !event.ctrlKey && !event.metaKey && !event.altKey && focusSearchBox()) {
        event.preventDefault()
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [closeFloatingModal, floatingSession, focusSearchBox, isHelpOpen, onShowHelp])
}
