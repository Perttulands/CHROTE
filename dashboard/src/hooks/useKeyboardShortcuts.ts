import { useEffect } from 'react'
import { useSession } from '../context/SessionContext'
import { focusSessionSearch } from '../keys/focusSessionSearch'

interface KeyboardShortcutsConfig {
  onShowKeys: () => void
  isKeysPanelOpen: boolean
}

// The terminal is rendered in this document now (ADR-0018), so its keystrokes
// bubble here. Anything typed into a terminal belongs to the shell.
function isTerminal(target: HTMLElement): boolean {
  return Boolean(target.closest('.terminal-window-body, .peek-body'))
}

function isDashboardChrome(target: HTMLElement): boolean {
  if (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable) return false
  return !isTerminal(target)
}

/**
 * The two plain keys that still work without the leader, outside a terminal:
 * `/` for the session search and `?` for the keys panel. Everything else the
 * dashboard answers to is a chord, registered in src/keys.
 */
export function useKeyboardShortcuts({ onShowKeys, isKeysPanelOpen }: KeyboardShortcutsConfig) {
  const { closeFloatingModal, floatingSession } = useSession()

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

      if (isKeysPanelOpen || !isDashboardChrome(target)) return

      if (event.key === '?' || (event.shiftKey && event.key === '/')) {
        event.preventDefault()
        onShowKeys()
        return
      }

      if (event.key === '/' && !event.ctrlKey && !event.metaKey && !event.altKey && focusSessionSearch()) {
        event.preventDefault()
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [closeFloatingModal, floatingSession, isKeysPanelOpen, onShowKeys])
}
