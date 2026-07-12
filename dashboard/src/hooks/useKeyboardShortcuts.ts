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

  const focusSearchBox = useCallback(() => {
    const searchInput = document.querySelector('.session-search-input') as HTMLInputElement | null
    searchInput?.focus()
    searchInput?.select()
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

      if (event.key === '/' && !event.ctrlKey && !event.metaKey && !event.altKey) {
        event.preventDefault()
        focusSearchBox()
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [closeFloatingModal, floatingSession, focusSearchBox, isHelpOpen, onShowHelp])
}
