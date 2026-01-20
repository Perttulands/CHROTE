import { useEffect, useCallback } from 'react'
import { useSession } from '../context/SessionContext'
import type { WorkspaceId } from '../types'

interface KeyboardShortcutsConfig {
  activeTab: 'terminal1' | 'terminal2' | 'files' | 'beads' | 'mail' | 'settings' | 'help'
  onTabChange: (tab: 'terminal1' | 'terminal2' | 'files' | 'beads' | 'mail' | 'settings' | 'help') => void
  onShowHelp: () => void
  isHelpOpen: boolean
}

/**
 * Hook to manage global keyboard shortcuts for the dashboard.
 * Shortcuts are disabled when typing in input fields or when help overlay is open.
 */
export function useKeyboardShortcuts({
  activeTab,
  onTabChange,
  onShowHelp,
  isHelpOpen,
}: KeyboardShortcutsConfig) {
  const {
    workspaces,
    toggleSidebar,
    closeFloatingModal,
    floatingSession,
    refreshSessions,
    settings,
    sessions,
    loadPreset,
    layoutPresets,
  } = useSession()

  // Create a new session
  const createSession = useCallback(async () => {
    try {
      const prefix = settings.defaultSessionPrefix || 'tmux'
      const escapedPrefix = prefix.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
      const regex = new RegExp(`^${escapedPrefix}(\\d+)$`)

      const existingNumbers = sessions
        .map(s => s.name.match(regex))
        .filter(Boolean)
        .map(m => parseInt(m![1], 10))

      const nextNum = existingNumbers.length > 0
        ? Math.max(...existingNumbers) + 1
        : 1
      const sessionName = `${prefix}${nextNum}`

      const response = await fetch('/api/tmux/sessions', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: sessionName })
      })
      if (response.ok) {
        refreshSessions()
      }
    } catch (e) {
      console.error('Failed to create session:', e)
    }
  }, [settings.defaultSessionPrefix, sessions, refreshSessions])

  // Focus search box
  const focusSearchBox = useCallback(() => {
    const searchInput = document.querySelector('.session-search-input') as HTMLInputElement
    if (searchInput) {
      searchInput.focus()
      searchInput.select()
    }
  }, [])

  // Get current workspace ID based on active tab
  const getCurrentWorkspaceId = useCallback((): WorkspaceId | null => {
    if (activeTab === 'terminal1' || activeTab === 'terminal2') {
      return activeTab
    }
    return null
  }, [activeTab])

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // Check if user is typing in an input field
      const target = e.target as HTMLElement
      const isTyping =
        target.tagName === 'INPUT' ||
        target.tagName === 'TEXTAREA' ||
        target.isContentEditable

      // Allow escape to close modals even when typing
      if (e.key === 'Escape') {
        if (isHelpOpen) {
          // Help overlay handles its own escape
          return
        }
        if (floatingSession) {
          e.preventDefault()
          closeFloatingModal()
          return
        }
      }

      // Don't process other shortcuts when typing or help is open
      if (isTyping || isHelpOpen) return

      // ? - Show help overlay
      if (e.key === '?' || (e.shiftKey && e.key === '/')) {
        e.preventDefault()
        onShowHelp()
        return
      }

      // / - Focus search box
      if (e.key === '/' && !e.ctrlKey && !e.metaKey) {
        e.preventDefault()
        focusSearchBox()
        return
      }

      // Tab - Toggle between Terminal 1 and Terminal 2
      if (e.key === 'Tab' && !e.ctrlKey && !e.shiftKey && !e.altKey) {
        // Only toggle if we're on a terminal tab
        if (activeTab === 'terminal1' || activeTab === 'terminal2') {
          e.preventDefault()
          onTabChange(activeTab === 'terminal1' ? 'terminal2' : 'terminal1')
          return
        }
      }

      // Number keys 1-4 without Ctrl - Switch to window
      if (!e.ctrlKey && !e.metaKey && !e.altKey && /^[1-4]$/.test(e.key)) {
        const workspaceId = getCurrentWorkspaceId()
        if (workspaceId) {
          const windowIndex = parseInt(e.key, 10)
          const workspace = workspaces[workspaceId]
          if (windowIndex <= workspace.windowCount) {
            // Focus the window (this would need integration with TerminalWindow focus)
            e.preventDefault()
            return
          }
        }
      }

      // Ctrl + S - Toggle sidebar
      if (e.ctrlKey && e.key === 's') {
        e.preventDefault()
        toggleSidebar()
        return
      }

      // Ctrl + N - Create new session
      if (e.ctrlKey && e.key === 'n') {
        e.preventDefault()
        createSession()
        return
      }

      // Ctrl + 1-9 - Load layout preset
      if (e.ctrlKey && /^[1-9]$/.test(e.key)) {
        const presetIndex = parseInt(e.key, 10) - 1
        if (presetIndex < layoutPresets.length) {
          e.preventDefault()
          loadPreset(layoutPresets[presetIndex].id)
          return
        }
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [
    activeTab,
    onTabChange,
    onShowHelp,
    isHelpOpen,
    floatingSession,
    closeFloatingModal,
    focusSearchBox,
    toggleSidebar,
    createSession,
    getCurrentWorkspaceId,
    workspaces,
    loadPreset,
    layoutPresets,
  ])
}
