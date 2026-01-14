import { useState } from 'react'

type ViewMode = 'browser' | 'info'

function FilesView() {
  const [viewMode, setViewMode] = useState<ViewMode>('browser')
  const [iframeKey, setIframeKey] = useState(0)

  const handleRefresh = () => {
    setIframeKey(prev => prev + 1)
  }

  const openInNewTab = () => {
    window.open('/files/', '_blank')
  }

  return (
    <div className="files-view">
      {/* Header Controls */}
      <div className="files-header">
        <div className="files-header-left">
          <h2 className="files-title">File Browser</h2>
          <div className="files-tabs">
            <button
              className={`files-tab ${viewMode === 'browser' ? 'active' : ''}`}
              onClick={() => setViewMode('browser')}
            >
              Browser
            </button>
            <button
              className={`files-tab ${viewMode === 'info' ? 'active' : ''}`}
              onClick={() => setViewMode('info')}
            >
              Info
            </button>
          </div>
        </div>
        <div className="files-header-right">
          <button className="files-btn" onClick={handleRefresh} title="Refresh">
            ↻
          </button>
          <button className="files-btn" onClick={openInNewTab} title="Open in new tab">
            ↗
          </button>
        </div>
      </div>

      {/* Content Area */}
      <div className="files-content">
        {viewMode === 'browser' ? (
          <div className="files-iframe-container">
            <iframe
              key={iframeKey}
              src="/files/"
              title="File Browser"
              className="files-iframe"
            />
          </div>
        ) : (
          <div className="files-info">
            <div className="files-card">
              <h3>Mounted Volumes</h3>
              <div className="files-item">
                <span className="files-label">/srv/code</span>
                <span className="files-value">E:/Code</span>
              </div>
              <div className="files-item">
                <span className="files-label">/srv/vault</span>
                <span className="files-value">E:/Vault</span>
              </div>
            </div>

            <div className="files-card">
              <h3>Quick Actions</h3>
              <p className="files-description">
                The file browser provides access to your code and vault directories.
                You can upload, download, create, edit, and organize files directly.
              </p>
              <div className="files-actions">
                <button className="files-action-btn" onClick={() => setViewMode('browser')}>
                  Open Browser
                </button>
                <button className="files-action-btn" onClick={openInNewTab}>
                  Open in New Tab
                </button>
              </div>
            </div>

            <div className="files-card">
              <h3>Keyboard Shortcuts</h3>
              <div className="files-item">
                <span className="files-label">Select All</span>
                <kbd className="files-kbd">Ctrl + A</kbd>
              </div>
              <div className="files-item">
                <span className="files-label">Copy</span>
                <kbd className="files-kbd">Ctrl + C</kbd>
              </div>
              <div className="files-item">
                <span className="files-label">Paste</span>
                <kbd className="files-kbd">Ctrl + V</kbd>
              </div>
              <div className="files-item">
                <span className="files-label">Delete</span>
                <kbd className="files-kbd">Del</kbd>
              </div>
              <div className="files-item">
                <span className="files-label">Rename</span>
                <kbd className="files-kbd">F2</kbd>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

export default FilesView
