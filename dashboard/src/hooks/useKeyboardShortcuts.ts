import { useCallback, useEffect } from 'react'
import { useSession } from '../context/SessionContext'

interface KeyboardShortcutsConfig {
  onShowHelp: () => void
  isHelpOpen: boolean
}

function isDashboardChrome(target: HTMLElement): boolean {
  if (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable) return false
  return !target.closest('.terminal-window-body, .floating-modal-body')
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

      if (event.key === 'Escape' && floatingSession) {
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
