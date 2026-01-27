// BeadsViewerTab - Project selector, BV launcher, and full-width Kanban

import { useState, useEffect, useCallback } from 'react'

// BV Help Modal content
const BV_HELP_CONTENT = `
## Beads Viewer (bv) - Quick Reference

**bv** is a high-performance Terminal UI for browsing and managing tasks in projects using the Beads issue tracking system.

### Navigation
| Key | Action |
|-----|--------|
| \`j/k\` | Move down/up |
| \`/\` | Fuzzy search |
| \`Enter\` | Select/expand |
| \`q\` | Quit |

### Views
| Key | View |
|-----|------|
| \`b\` | Kanban board |
| \`g\` | Dependency graph |
| \`i\` | Insights & metrics |
| \`h\` | History timeline |

### Filters
| Key | Filter |
|-----|--------|
| \`o\` | Open issues |
| \`c\` | Closed issues |
| \`r\` | Ready (unblocked) |

### Actions
| Key | Action |
|-----|--------|
| \`E\` | Export to Markdown |
| \`C\` | Copy issue as Markdown |
| \`O\` | Open in editor |
| \`t\` | Time-travel (pick revision) |
| \`T\` | Compare with HEAD~5 |

### Features
- **Live Reload**: Auto-refreshes when beads.jsonl changes
- **Split View**: List + details on wide screens
- **Markdown**: Beautiful rendering with syntax highlighting
- **AI-Ready**: Structured insights for coding agents
`
import { useProjects, useIssues } from '../BeadsView/hooks'
import KanbanView from '../BeadsView/KanbanView'
import './beads-viewer.css'

export default function BeadsViewerTab() {
  const { projects, loading: projectsLoading } = useProjects()
  const [selectedPath, setSelectedPath] = useState<string | null>(null)
  const [showHelp, setShowHelp] = useState(false)

  // Fetch issues for Kanban view when project is selected
  const { issues, loading: issuesLoading, error: issuesError } = useIssues(selectedPath)

  // Auto-select first project if none selected, and start BV terminal
  useEffect(() => {
    if (!selectedPath && projects.length > 0) {
      const firstPath = projects[0].path
      setSelectedPath(firstPath)
      // Start BV terminal for auto-selected project
      fetch(`/api/bv-terminal/restart?path=${encodeURIComponent(firstPath)}`, {
        method: 'POST'
      }).catch(e => console.error('Failed to start bv terminal:', e))
    }
  }, [projects, selectedPath])

  // Handle project selection - also restart bv terminal for when user opens it
  const handleSelectProject = useCallback(async (path: string) => {
    if (!path) return
    setSelectedPath(path)

    // Restart bv terminal with new path so it's ready when opened
    try {
      await fetch(`/api/bv-terminal/restart?path=${encodeURIComponent(path)}`, {
        method: 'POST'
      })
    } catch (e) {
      console.error('Failed to restart bv terminal:', e)
    }
  }, [])

  // Open bv terminal in new tab - restart with current project first
  const openBeadsViewer = useCallback(async () => {
    if (selectedPath) {
      // Ensure BV terminal is running with current project
      await fetch(`/api/bv-terminal/restart?path=${encodeURIComponent(selectedPath)}`, {
        method: 'POST'
      }).catch(e => console.error('Failed to restart bv terminal:', e))
    }
    window.open('/bv-terminal/', '_blank')
  }, [selectedPath])

  return (
    <div className="bv-container">
      {/* Toolbar: Project selector + Open BV button */}
      <div className="bv-toolbar">
        <div className="bv-toolbar-left">
          <label htmlFor="bv-project-select">Project:</label>
          <select
            id="bv-project-select"
            value={selectedPath || ''}
            onChange={(e) => handleSelectProject(e.target.value)}
            disabled={projectsLoading}
          >
            {projectsLoading ? (
              <option>Loading projects...</option>
            ) : projects.length === 0 ? (
              <option>No projects found - add paths in Settings</option>
            ) : (
              <>
                <option value="">Select a project</option>
                {projects.map(project => (
                  <option key={project.path} value={project.path}>
                    {project.path}
                  </option>
                ))}
              </>
            )}
          </select>

          <button
          className="bv-btn bv-btn-primary"
          onClick={openBeadsViewer}
          disabled={!selectedPath}
          title="Open interactive beads_viewer in new tab"
        >
          Open Beads Viewer
        </button>
          <button
            className="bv-btn"
            onClick={() => setShowHelp(true)}
            title="BV keyboard shortcuts and help"
          >
            BV Help
          </button>
        </div>
        {/* Hint note for adding custom paths */}
        <span className="bv-hint">
          Missing a project? Add paths in <strong>Settings → Beads Projects</strong>
        </span>
      </div>

      {/* Help Modal */}
      {showHelp && (
        <div className="bv-help-overlay" onClick={() => setShowHelp(false)}>
          <div className="bv-help-modal" onClick={e => e.stopPropagation()}>
            <div className="bv-help-header">
              <h2>Beads Viewer Help</h2>
              <button className="bv-help-close" onClick={() => setShowHelp(false)}>×</button>
            </div>
            <div className="bv-help-content">
              <pre>{BV_HELP_CONTENT}</pre>
            </div>
          </div>
        </div>
      )}

      {/* Full-width Kanban */}
      <div className="bv-kanban-full">
        <KanbanView
          issues={issues}
          loading={issuesLoading}
          error={issuesError}
        />
      </div>
    </div>
  )
}
