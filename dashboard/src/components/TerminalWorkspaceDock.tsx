import { useCallback, useState } from 'react'
import type { WorkspaceId } from '../types'
import SessionPanel from './SessionPanel'
import TerminalArea from './TerminalArea'
import TerminalFilesPanel from './TerminalFilesPanel'
import {
  readWorkspaceDockState,
  writeWorkspaceDockState,
  type WorkspaceDockState,
} from './workspaceFilesState'

interface TerminalWorkspaceDockProps {
  workspaceId: WorkspaceId
  active: boolean
  onOpenSessionBankSettings: () => void
  onOpenInFiles: (path: string) => void
}

function TerminalWorkspaceDock({
  workspaceId,
  active,
  onOpenSessionBankSettings,
  onOpenInFiles,
}: TerminalWorkspaceDockProps) {
  const [dockState, setDockState] = useState<WorkspaceDockState>(() => readWorkspaceDockState(workspaceId))

  const updateDockState = useCallback((update: (previous: WorkspaceDockState) => WorkspaceDockState) => {
    setDockState(previous => {
      const next = update(previous)
      writeWorkspaceDockState(workspaceId, next)
      return next
    })
  }, [workspaceId])

  return (
    <div
      className="terminal-workspace-dock"
      data-workspace={workspaceId}
      data-active={active}
      data-sessions-collapsed={dockState.sessionsCollapsed}
      data-files-collapsed={dockState.filesCollapsed}
      style={{ display: active ? 'flex' : 'none' }}
    >
      {active && (
        <>
          <SessionPanel
            activeWorkspaceId={workspaceId}
            onOpenSessionBankSettings={onOpenSessionBankSettings}
            collapsed={dockState.sessionsCollapsed}
            width={dockState.sessionsWidth}
            onToggle={() => updateDockState(previous => ({ ...previous, sessionsCollapsed: !previous.sessionsCollapsed }))}
            onWidthChange={sessionsWidth => updateDockState(previous => ({ ...previous, sessionsWidth }))}
          />
          <TerminalFilesPanel
            workspaceId={workspaceId}
            collapsed={dockState.filesCollapsed}
            width={dockState.filesWidth}
            onToggle={() => updateDockState(previous => ({ ...previous, filesCollapsed: !previous.filesCollapsed }))}
            onWidthChange={filesWidth => updateDockState(previous => ({ ...previous, filesWidth }))}
            onOpenInFiles={onOpenInFiles}
          />
        </>
      )}
      <TerminalArea workspaceId={workspaceId} active={active} />
    </div>
  )
}

export default TerminalWorkspaceDock
