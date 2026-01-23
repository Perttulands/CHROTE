// Main BeadsView component with sub-tab navigation

import { useState, useCallback } from 'react'
import type { BeadsSubTab } from './types'
import { useProjects, useIssues, useTriage, useInsights } from './hooks'
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

  const { projects, loading: projectsLoading } = useProjects()
  const { issues, loading: issuesLoading, error: issuesError, refresh: refreshIssues } = useIssues(selectedProjectPath, showPatrols)
  const { triage, loading: triageLoading, error: triageError, refresh: refreshTriage } = useTriage(selectedProjectPath)
  const { insights, loading: insightsLoading, error: insightsError, refresh: refreshInsights } = useInsights(selectedProjectPath)

  const handleProjectSelect = useCallback((path: string) => {
    setSelectedProjectPath(path || null)
  }, [])

  const handleRefresh = useCallback(() => {
    refreshIssues()
    refreshTriage()
    refreshInsights()
  }, [refreshIssues, refreshTriage, refreshInsights])

  const isLoading = issuesLoading || triageLoading || insightsLoading

  return (
    <div className="beads-view">
      <div className="beads-header">
        <ProjectSelector
          projects={projects}
          selectedPath={selectedProjectPath}
          onSelect={handleProjectSelect}
          loading={projectsLoading}
        />
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
              {isLoading ? 'Loading...' : 'Refresh'}
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
              <KanbanView issues={issues} loading={issuesLoading} error={issuesError} />
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
          <div className="empty-icon">&#x1F4CB;</div>
          <h2>Select a Project</h2>
          <p>Choose a project with a .beads directory to view issues and insights.</p>
          {projects.length === 0 && !projectsLoading && (
            <p className="empty-hint">
              No projects found. Create a .beads directory in /code or /workspace to get started.
            </p>
          )}
        </div>
      )}
    </div>
  )
}
