// Main BeadsView component with sub-tab navigation

import { useState, useCallback, useEffect, useMemo } from 'react'
import type { MouseEvent as ReactMouseEvent } from 'react'
import type { BeadsIssue, BeadsSubTab } from './types'
import { useProjects, useIssues, useTriage, useInsights } from './hooks'
import { isFeatureEnabled } from '../../featureFlags'
import ProjectSelector from './ProjectSelector'
import KanbanView from './KanbanView'
import TriageView from './TriageView'
import InsightsView from './InsightsView'
import IssueDetailModal from './IssueDetailModal'

const SUB_TABS: { id: BeadsSubTab; label: string }[] = [
  { id: 'kanban', label: 'Kanban' },
  { id: 'triage', label: 'Triage' },
  { id: 'insights', label: 'Insights' },
]

type BeadsContextMenu =
  | { type: 'issue'; issue: BeadsIssue; x: number; y: number }
  | { type: 'surface'; x: number; y: number }

interface BeadsViewProps {
  onOpenProjectInFiles?: (path: string) => void
}

function copyText(text: string): void {
  void navigator.clipboard?.writeText(text)
}

function issueReference(issue: BeadsIssue): string {
  return `${issue.id} — ${issue.title}`
}

export default function BeadsView({ onOpenProjectInFiles }: BeadsViewProps = {}) {
  const [activeSubTab, setActiveSubTab] = useState<BeadsSubTab>('kanban')
  const [selectedProjectPath, setSelectedProjectPath] = useState<string | null>(null)
  const [contextMenu, setContextMenu] = useState<BeadsContextMenu | null>(null)
  const [selectedIssue, setSelectedIssue] = useState<BeadsIssue | null>(null)
  const includeAllStatuses = isFeatureEnabled('beadsAllStatuses')
  const enableDetailModal = isFeatureEnabled('beadsDetailModal')

  const { projects, loading: projectsLoading } = useProjects()
  const { issues, loading: issuesLoading, error: issuesError, refresh: refreshIssues } = useIssues(
    selectedProjectPath,
    false,
    { includeAllStatuses }
  )
  const { triage, loading: triageLoading, error: triageError, refresh: refreshTriage } = useTriage(selectedProjectPath)
  const { insights, loading: insightsLoading, error: insightsError, refresh: refreshInsights } = useInsights(
    selectedProjectPath,
    { includeAllStatuses }
  )
  const selectedProject = useMemo(
    () => projects.find(project => project.path === selectedProjectPath) || null,
    [projects, selectedProjectPath]
  )

  const handleProjectSelect = useCallback((path: string) => {
    setSelectedProjectPath(path || null)
  }, [])

  const handleRefresh = useCallback(() => {
    refreshIssues()
    refreshTriage()
    refreshInsights()
    setContextMenu(null)
  }, [refreshIssues, refreshTriage, refreshInsights])

  const openIssueDetails = useCallback((issue: BeadsIssue) => {
    if (!enableDetailModal) return
    setSelectedIssue(issue)
    setContextMenu(null)
  }, [enableDetailModal])

  const handleIssueContextMenu = useCallback((issue: BeadsIssue, event: ReactMouseEvent) => {
    event.preventDefault()
    setContextMenu({ type: 'issue', issue, x: event.clientX, y: event.clientY })
  }, [])

  const handleSurfaceContextMenu = useCallback((event: ReactMouseEvent) => {
    if (!selectedProjectPath) return
    event.preventDefault()
    setContextMenu({ type: 'surface', x: event.clientX, y: event.clientY })
  }, [selectedProjectPath])

  const handleOpenProjectInFiles = useCallback(() => {
    if (!selectedProjectPath || !onOpenProjectInFiles) return
    onOpenProjectInFiles(selectedProjectPath)
    setContextMenu(null)
  }, [onOpenProjectInFiles, selectedProjectPath])

  const isLoading = issuesLoading || triageLoading || insightsLoading

  useEffect(() => {
    if (!contextMenu) return
    const handlePointerDown = (event: MouseEvent) => {
      const target = event.target as Element
      if (!target.closest('.beads-context-menu')) setContextMenu(null)
    }
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setContextMenu(null)
    }
    document.addEventListener('mousedown', handlePointerDown)
    document.addEventListener('keydown', handleKeyDown)
    return () => {
      document.removeEventListener('mousedown', handlePointerDown)
      document.removeEventListener('keydown', handleKeyDown)
    }
  }, [contextMenu])

  useEffect(() => {
    if (projectsLoading) return
    if (projects.length === 0) {
      if (selectedProjectPath !== null) setSelectedProjectPath(null)
      return
    }
    if (!selectedProjectPath || !projects.some(project => project.path === selectedProjectPath)) {
      setSelectedProjectPath(projects[0].path)
    }
  }, [projects, projectsLoading, selectedProjectPath])

  return (
    <div className="beads-view">
      <div className="beads-status-strip" onContextMenu={handleSurfaceContextMenu}>
        <ProjectSelector
          projects={projects}
          selectedPath={selectedProjectPath}
          onSelect={handleProjectSelect}
          loading={projectsLoading}
        />
        <span className="beads-status-pill">
          <span>projects</span>
          <strong>{projectsLoading ? '--' : projects.length}</strong>
        </span>
        {selectedProject && (
          <span className="beads-status-pill">
            <span>active</span>
            <strong>{selectedProject.name}</strong>
          </span>
        )}
        {selectedProjectPath && (
          <button
            className="beads-refresh-btn"
            onClick={handleRefresh}
            disabled={isLoading}
            title="Refresh data"
          >
            {isLoading ? 'Loading' : 'Refresh'}
          </button>
        )}
      </div>

      {selectedProjectPath ? (
        <>
          <div className="beads-subtabs">
            {SUB_TABS.map(tab => (
              <button
                key={tab.id}
                className={`beads-subtab ${activeSubTab === tab.id ? 'active' : ''}`}
                onClick={() => setActiveSubTab(tab.id)}
              >
                {tab.label}
              </button>
            ))}
          </div>

          <div className="beads-content">
            {activeSubTab === 'kanban' && (
              <KanbanView
                issues={issues}
                loading={issuesLoading}
                error={issuesError}
                projectPath={selectedProjectPath}
                enableDetailModal={enableDetailModal}
                onIssueUpdated={refreshIssues}
                onIssueOpen={openIssueDetails}
                onIssueContextMenu={handleIssueContextMenu}
              />
            )}
            {activeSubTab === 'triage' && (
              <TriageView
                triage={triage}
                issues={issues}
                loading={triageLoading}
                error={triageError}
                onIssueOpen={openIssueDetails}
                onIssueContextMenu={handleIssueContextMenu}
              />
            )}
            {activeSubTab === 'insights' && (
              <InsightsView insights={insights} loading={insightsLoading} error={insightsError} />
            )}
          </div>
        </>
      ) : (
        <div className="beads-empty-state">
          <div className="empty-icon">BD</div>
          <h2>{projectsLoading ? 'Loading Projects' : projects.length === 0 ? 'No Beads Projects' : 'Select a Project'}</h2>
          <p>{projectsLoading ? 'Scanning configured workspaces.' : 'No modern .beads workspace is available to display.'}</p>
          {projects.length === 0 && !projectsLoading && (
            <p className="empty-hint">
              Check CHROTE_BEADS_WORKSPACES or add a project path in Settings.
            </p>
          )}
        </div>
      )}

      {selectedIssue && selectedProjectPath && enableDetailModal && (
        <IssueDetailModal
          projectPath={selectedProjectPath}
          issue={selectedIssue}
          onClose={() => setSelectedIssue(null)}
          onIssueUpdated={refreshIssues}
        />
      )}

      {contextMenu && (
        <div className="beads-context-menu" style={{ left: contextMenu.x, top: contextMenu.y }}>
          {contextMenu.type === 'issue' ? (
            <>
              {enableDetailModal && (
                <button className="beads-context-item" type="button" onClick={() => openIssueDetails(contextMenu.issue)}>Open Details</button>
              )}
              <button className="beads-context-item" type="button" onClick={() => { copyText(contextMenu.issue.id); setContextMenu(null) }}>Copy Bead ID</button>
              <button className="beads-context-item" type="button" onClick={() => { copyText(issueReference(contextMenu.issue)); setContextMenu(null) }}>Copy Bead Reference</button>
              <button className="beads-context-item" type="button" onClick={() => { copyText(`bd show ${contextMenu.issue.id}`); setContextMenu(null) }}>Copy bd show Command</button>
              {selectedProjectPath && (
                <button className="beads-context-item" type="button" onClick={() => { copyText(selectedProjectPath); setContextMenu(null) }}>Copy Active Project Path</button>
              )}
              <button className="beads-context-item" type="button" onClick={handleRefresh}>Refresh</button>
            </>
          ) : (
            <>
              {selectedProjectPath && (
                <button className="beads-context-item" type="button" onClick={() => { copyText(selectedProjectPath); setContextMenu(null) }}>Copy Active Project Path</button>
              )}
              {selectedProjectPath && onOpenProjectInFiles && (
                <button className="beads-context-item" type="button" onClick={handleOpenProjectInFiles}>Open Project in Files</button>
              )}
              <button className="beads-context-item" type="button" onClick={handleRefresh}>Refresh</button>
            </>
          )}
        </div>
      )}
    </div>
  )
}
