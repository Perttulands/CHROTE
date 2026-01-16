// Beads Tab - Main container component with sub-navigation
// This is the entry point for the beads_module in the dashboard

import { useState, useCallback, useEffect, useRef } from 'react';
import { BeadsProvider, useBeads } from '../context';
import { BeadsGraphView } from './BeadsGraphView';
import { BeadsKanbanView } from './BeadsKanbanView';
import { BeadsTriageView } from './BeadsTriageView';
import { BeadsInsightsView } from './BeadsInsightsView';
import type { BeadsSubTab } from '../types';

// Import module styles
import '../beads.css';

// ============================================================================
// FILE BROWSER TYPES
// ============================================================================

interface FolderItem {
  name: string;
  path: string;
  isDir: boolean;
}

// ============================================================================
// FILE BROWSER API
// ============================================================================

const FILES_API_BASE = '/files/api';

async function fetchFolders(path: string): Promise<FolderItem[]> {
  const cleanPath = path.startsWith('/') ? path : '/' + path;
  const response = await fetch(`${FILES_API_BASE}/resources${cleanPath}`, {
    headers: { 'Accept': 'application/json' },
  });

  if (!response.ok) {
    throw new Error(`Failed to fetch directory: ${response.status}`);
  }

  const data = await response.json();

  if (data.isDir && data.items) {
    return data.items
      .filter((item: { isDir: boolean }) => item.isDir)
      .map((item: { name: string; isDir: boolean }) => ({
        name: item.name,
        path: cleanPath === '/' ? `/${item.name}` : `${cleanPath}/${item.name}`,
        isDir: item.isDir,
      }));
  }

  return [];
}

async function checkBeadsExists(path: string): Promise<boolean> {
  try {
    const beadsPath = path.endsWith('/') ? `${path}.beads` : `${path}/.beads`;
    const response = await fetch(`${FILES_API_BASE}/resources${beadsPath}`, {
      headers: { 'Accept': 'application/json' },
    });
    return response.ok;
  } catch {
    return false;
  }
}

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
// PATH SELECTOR MODAL COMPONENT
// ============================================================================

interface PathSelectorModalProps {
  isOpen: boolean;
  currentPath: string;
  onClose: () => void;
  onSelect: (path: string) => void;
}

function PathSelectorModal({ isOpen, currentPath, onClose, onSelect }: PathSelectorModalProps) {
  const [mode, setMode] = useState<'input' | 'browse'>('input');
  const [inputPath, setInputPath] = useState(currentPath);
  const [browsePath, setBrowsePath] = useState('/code');
  const [folders, setFolders] = useState<FolderItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [hasBeads, setHasBeads] = useState<boolean | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  // Load folders when browse path changes
  useEffect(() => {
    if (mode === 'browse' && isOpen) {
      setLoading(true);
      setError(null);
      fetchFolders(browsePath)
        .then(setFolders)
        .catch((e) => setError(e.message))
        .finally(() => setLoading(false));

      // Check if current browse path has .beads
      checkBeadsExists(browsePath).then(setHasBeads);
    }
  }, [browsePath, mode, isOpen]);

  // Reset state when modal opens
  useEffect(() => {
    if (isOpen) {
      setInputPath(currentPath);
      setMode('input');
      setError(null);
      setHasBeads(null);
      setTimeout(() => inputRef.current?.focus(), 100);
    }
  }, [isOpen, currentPath]);

  // Check if input path has beads when it changes
  useEffect(() => {
    if (mode === 'input' && inputPath) {
      const timer = setTimeout(() => {
        checkBeadsExists(inputPath).then(setHasBeads);
      }, 300);
      return () => clearTimeout(timer);
    }
  }, [inputPath, mode]);

  const handleInputSubmit = useCallback(() => {
    if (inputPath.trim()) {
      onSelect(inputPath.trim());
    }
  }, [inputPath, onSelect]);

  const handleBrowseSelect = useCallback(() => {
    onSelect(browsePath);
  }, [browsePath, onSelect]);

  const handleNavigate = useCallback((path: string) => {
    setBrowsePath(path);
  }, []);

  const handleGoUp = useCallback(() => {
    const parts = browsePath.split('/').filter(Boolean);
    if (parts.length > 1) {
      parts.pop();
      setBrowsePath('/' + parts.join('/'));
    } else {
      setBrowsePath('/');
    }
  }, [browsePath]);

  if (!isOpen) return null;

  const breadcrumbs = browsePath.split('/').filter(Boolean);

  return (
    <>
      <div className="beads-modal-backdrop" onClick={onClose} />
      <div className="beads-path-modal">
        <div className="beads-modal-header">
          <h3>Select Beads Project Location</h3>
          <button className="beads-modal-close" onClick={onClose}>x</button>
        </div>

        <div className="beads-modal-tabs">
          <button
            className={`beads-modal-tab ${mode === 'input' ? 'active' : ''}`}
            onClick={() => setMode('input')}
          >
            Enter Path
          </button>
          <button
            className={`beads-modal-tab ${mode === 'browse' ? 'active' : ''}`}
            onClick={() => setMode('browse')}
          >
            Browse Folders
          </button>
        </div>

        <div className="beads-modal-body">
          {mode === 'input' ? (
            <div className="beads-path-input-section">
              <p className="beads-modal-hint">
                Enter the path to a directory containing a <code>.beads/</code> folder.
                Available mount points: <code>/code</code>
              </p>
              <div className="beads-path-input-row">
                <input
                  ref={inputRef}
                  type="text"
                  className="beads-path-input"
                  value={inputPath}
                  onChange={(e) => setInputPath(e.target.value)}
                  onKeyDown={(e) => e.key === 'Enter' && handleInputSubmit()}
                  placeholder="/code/my-project"
                />
                <button
                  className="beads-btn beads-btn-primary"
                  onClick={handleInputSubmit}
                  disabled={!inputPath.trim()}
                >
                  Select
                </button>
              </div>
              {hasBeads !== null && (
                <div className={`beads-path-status ${hasBeads ? 'valid' : 'invalid'}`}>
                  {hasBeads ? (
                    <><span className="beads-status-icon">O</span> .beads/ directory found</>
                  ) : (
                    <><span className="beads-status-icon">!</span> No .beads/ directory at this path</>
                  )}
                </div>
              )}
            </div>
          ) : (
            <div className="beads-browse-section">
              <div className="beads-browse-breadcrumbs">
                <button
                  className="beads-breadcrumb-item"
                  onClick={() => setBrowsePath('/')}
                >
                  /
                </button>
                {breadcrumbs.map((part, idx) => (
                  <span key={idx}>
                    <span className="beads-breadcrumb-sep">/</span>
                    <button
                      className="beads-breadcrumb-item"
                      onClick={() =>
                        setBrowsePath('/' + breadcrumbs.slice(0, idx + 1).join('/'))
                      }
                    >
                      {part}
                    </button>
                  </span>
                ))}
              </div>

              <div className="beads-browse-toolbar">
                <button
                  className="beads-btn beads-btn-icon"
                  onClick={handleGoUp}
                  disabled={browsePath === '/'}
                  title="Go up"
                >
                  ..
                </button>
                {hasBeads && (
                  <span className="beads-has-beads-badge">
                    .beads/ found here
                  </span>
                )}
              </div>

              <div className="beads-folder-list">
                {loading ? (
                  <div className="beads-loading">
                    <div className="beads-spinner" />
                    <span>Loading folders...</span>
                  </div>
                ) : error ? (
                  <div className="beads-browse-error">{error}</div>
                ) : folders.length === 0 ? (
                  <div className="beads-browse-empty">No subdirectories</div>
                ) : (
                  folders.map((folder) => (
                    <button
                      key={folder.path}
                      className="beads-folder-item"
                      onClick={() => handleNavigate(folder.path)}
                    >
                      <span className="beads-folder-icon">folder</span>
                      <span className="beads-folder-name">{folder.name}</span>
                    </button>
                  ))
                )}
              </div>

              <div className="beads-browse-actions">
                <button
                  className="beads-btn beads-btn-primary"
                  onClick={handleBrowseSelect}
                >
                  Select This Folder
                </button>
              </div>
            </div>
          )}
        </div>

        <div className="beads-modal-footer">
          <div className="beads-modal-info">
            <strong>Tip:</strong> Your project should have a <code>.beads/issues.jsonl</code> file.
            Install <a href="https://github.com/Dicklesworthstone/beads_viewer" target="_blank" rel="noopener noreferrer">bv</a> for full features.
          </div>
        </div>
      </div>
    </>
  );
}

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
  const [isPathModalOpen, setIsPathModalOpen] = useState(false);

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

  const handlePathModalSelect = useCallback(
    (path: string) => {
      setProjectPath(path);
      setIsPathModalOpen(false);
      // Auto-refresh after selecting new path
      refreshAll(path);
    },
    [setProjectPath, refreshAll]
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
          {/* Path selector button */}
          <button
            className="beads-btn beads-btn-icon"
            onClick={() => setIsPathModalOpen(true)}
            title="Select Project Location"
          >
            ...
          </button>

          {/* Project selector dropdown */}
          <div className="beads-project-selector">
            <button
              className="beads-project-selector-btn"
              onClick={() => setIsProjectSelectorOpen(!isProjectSelectorOpen)}
            >
              <span className="beads-project-selector-icon">folder</span>
              <span className="beads-project-selector-path">
                {viewState.projectPath}
              </span>
              <span className="beads-project-selector-arrow">v</span>
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
                  <button
                    className="beads-project-selector-item beads-project-selector-browse"
                    onClick={() => {
                      setIsProjectSelectorOpen(false);
                      setIsPathModalOpen(true);
                    }}
                  >
                    + Browse for project...
                  </button>
                </div>
              </>
            )}
          </div>

          <button
            className="beads-btn beads-btn-icon"
            onClick={handleRefresh}
            disabled={isLoading}
            title="Refresh All Data"
          >
            {isLoading ? '...' : 'R'}
          </button>
        </div>
      </div>

      {/* Content area */}
      <div className="beads-tab-content">{renderActiveView()}</div>

      {/* Credits */}
      <div className="beads-credits">
        Beads viewer inspired by{' '}
        <a
          href="https://github.com/Dicklesworthstone/beads_viewer"
          target="_blank"
          rel="noopener noreferrer"
        >
          beads_viewer
        </a>
        {' '}by Jeff Emanuel
      </div>

      {/* Path Selector Modal */}
      <PathSelectorModal
        isOpen={isPathModalOpen}
        currentPath={viewState.projectPath}
        onClose={() => setIsPathModalOpen(false)}
        onSelect={handlePathModalSelect}
      />
    </div>
  );
}

// ============================================================================
// MAIN EXPORTED COMPONENT (with provider)
// ============================================================================

interface BeadsTabProps {
  initialProjectPath?: string;
}

export function BeadsTab({ initialProjectPath = '/code' }: BeadsTabProps) {
  return (
    <BeadsProvider initialProjectPath={initialProjectPath}>
      <BeadsTabContent />
    </BeadsProvider>
  );
}

export default BeadsTab;
