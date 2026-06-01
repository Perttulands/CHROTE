// Main BeadsView component with sub-tab navigation

import { useState, useCallback, useEffect, useMemo } from 'react'
import type { BeadsSubTab } from './types'
import { useProjects, useIssues, useTriage, useInsights } from './hooks'
import { isFeatureEnabled } from '../../featureFlags'
import ProjectSelector from './ProjectSelector'
import KanbanView from './KanbanView'
import TriageView from './TriageView'
import InsightsView from './InsightsView'

const SUB_TABS: { id: BeadsSubTab; label: string }[] = [
  { id: 'kanban', label: 'Kanban' },
  { id: 'triage', label: 'Triage' },
  { id: 'insights', label: 'Insights' },
]

export default function BeadsView() {
  const [activeSubTab, setActiveSubTab] = useState<BeadsSubTab>('kanban')
  const [selectedProjectPath, setSelectedProjectPath] = useState<string | null>(null)
  const [showPatrols, setShowPatrols] = useState(false)
  const includeAllStatuses = isFeatureEnabled('beadsAllStatuses')
  const enableDetailModal = isFeatureEnabled('beadsDetailModal')

  const { projects, loading: projectsLoading } = useProjects()
  const { issues, loading: issuesLoading, error: issuesError, refresh: refreshIssues } = useIssues(
    selectedProjectPath,
    showPatrols,
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
  }, [refreshIssues, refreshTriage, refreshInsights])

  const isLoading = issuesLoading || triageLoading || insightsLoading

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
      <div className="beads-status-strip">
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
          <>
            <label className="beads-patrol-toggle" title="Show/hide patrol digest beads">
              <input
                type="checkbox"
                checked={showPatrols}
                onChange={(e) => setShowPatrols(e.target.checked)}
              />
              <span>Show patrols</span>
            </label>
            <button
              className="beads-refresh-btn"
              onClick={handleRefresh}
              disabled={isLoading}
              title="Refresh data"
            >
              {isLoading ? 'Loading' : 'Refresh'}
            </button>
          </>
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
              />
            )}
            {activeSubTab === 'triage' && (
              <TriageView triage={triage} issues={issues} loading={triageLoading} error={triageError} />
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
    </div>
  )
}
