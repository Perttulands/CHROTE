// Beads Tab - Main container component with sub-navigation
// This is the entry point for the beads_module in the dashboard

import { useState, useCallback } from 'react';
import { BeadsProvider, useBeads } from '../context';
import { BeadsGraphView } from './BeadsGraphView';
import { BeadsKanbanView } from './BeadsKanbanView';
import { BeadsTriageView } from './BeadsTriageView';
import { BeadsInsightsView } from './BeadsInsightsView';
import type { BeadsSubTab } from '../types';

// Import module styles
import '../beads.css';

// ============================================================================
// SUB-TAB CONFIGURATION
// ============================================================================

interface SubTabConfig {
  id: BeadsSubTab;
  label: string;
  icon: string;
  description: string;
}

const SUB_TABS: SubTabConfig[] = [
  {
    id: 'graph',
    label: 'Graph',
    icon: '🕸️',
    description: 'Interactive dependency graph visualization',
  },
  {
    id: 'kanban',
    label: 'Kanban',
    icon: '📋',
    description: 'Kanban board for issue management',
  },
  {
    id: 'triage',
    label: 'Triage',
    icon: '🎯',
    description: 'AI-powered issue prioritization',
  },
  {
    id: 'insights',
    label: 'Insights',
    icon: '📈',
    description: 'Graph metrics and project health',
  },
];

// ============================================================================
// INNER CONTENT COMPONENT (uses context)
// ============================================================================

function BeadsTabContent() {
  const {
    viewState,
    setActiveSubTab,
    setProjectPath,
    availableProjects,
    refreshAll,
    isLoading,
  } = useBeads();

  const [isProjectSelectorOpen, setIsProjectSelectorOpen] = useState(false);

  const handleSubTabChange = useCallback(
    (tab: BeadsSubTab) => {
      setActiveSubTab(tab);
    },
    [setActiveSubTab]
  );

  const handleProjectChange = useCallback(
    (path: string) => {
      setProjectPath(path);
      setIsProjectSelectorOpen(false);
    },
    [setProjectPath]
  );
  
  const handleRefresh = useCallback(() => {
    refreshAll(viewState.projectPath);
  }, [refreshAll, viewState.projectPath]);

  // Render active view
  const renderActiveView = () => {
    const projectPath = viewState.projectPath;

    switch (viewState.activeSubTab) {
      case 'graph':
        return <BeadsGraphView projectPath={projectPath} />;
      case 'kanban':
        return <BeadsKanbanView projectPath={projectPath} />;
      case 'triage':
        return <BeadsTriageView projectPath={projectPath} />;
      case 'insights':
        return <BeadsInsightsView projectPath={projectPath} />;
      default:
        return <BeadsGraphView projectPath={projectPath} />;
    }
  };

  return (
    <div className="beads-tab">
      {/* Header with sub-tabs */}
      <div className="beads-tab-header">
        <div className="beads-sub-tabs">
          {SUB_TABS.map((tab) => (
            <button
              key={tab.id}
              className={`beads-sub-tab ${
                viewState.activeSubTab === tab.id ? 'active' : ''
              }`}
              onClick={() => handleSubTabChange(tab.id)}
              title={tab.description}
            >
              <span className="beads-sub-tab-icon">{tab.icon}</span>
              <span className="beads-sub-tab-label">{tab.label}</span>
            </button>
          ))}
        </div>

        <div className="beads-tab-actions">
          {/* Project selector */}
          <div className="beads-project-selector">
            <button
              className="beads-project-selector-btn"
              onClick={() => setIsProjectSelectorOpen(!isProjectSelectorOpen)}
            >
              <span className="beads-project-selector-icon">📁</span>
              <span className="beads-project-selector-path">
                {viewState.projectPath}
              </span>
              <span className="beads-project-selector-arrow">▼</span>
            </button>

            {isProjectSelectorOpen && (
              <>
                <div
                  className="beads-project-selector-backdrop"
                  onClick={() => setIsProjectSelectorOpen(false)}
                />
                <div className="beads-project-selector-dropdown">
                  {availableProjects.map((path) => (
                    <button
                      key={path}
                      className={`beads-project-selector-item ${
                        path === viewState.projectPath ? 'active' : ''
                      }`}
                      onClick={() => handleProjectChange(path)}
                    >
                      {path}
                    </button>
                  ))}
                  {availableProjects.length === 0 && (
                    <div className="beads-project-selector-empty">
                      No projects found
          
          <button 
            className="beads-btn beads-btn-icon" 
            onClick={handleRefresh}
            disabled={isLoading}
            title="Refresh All Data"
          >
            {isLoading ? '⏳' : '🔄'}
          </button>
                    </div>
                  )}
                </div>
              </>
            )}
          </div>
        </div>
      </div>

      {/* Content area */}
      <div className="beads-tab-content">{renderActiveView()}</div>
    </div>
  );
}

// ============================================================================
// MAIN EXPORTED COMPONENT (with provider)
// ============================================================================

interface BeadsTabProps {
  initialProjectPath?: string;
}

export function BeadsTab({ initialProjectPath = '/workspace' }: BeadsTabProps) {
  return (
    <BeadsProvider initialProjectPath={initialProjectPath}>
      <BeadsTabContent />
    </BeadsProvider>
  );
}

export default BeadsTab;
