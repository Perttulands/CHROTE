export function InfoPanel() {
  return (
    <div className="fb-info-panel">
      <div className="fb-info-card">
        <h3>Available Directories</h3>
        <div className="fb-info-item">
          <span className="fb-info-label">/code</span>
          <span className="fb-info-value">Project workspace</span>
        </div>
        <div className="fb-info-item">
          <span className="fb-info-label">/vault</span>
          <span className="fb-info-value">Read-only storage</span>
        </div>
      </div>

      <div className="fb-info-card">
        <h3>Keyboard Shortcuts</h3>
        <div className="fb-info-item">
          <span className="fb-info-label">Open/Enter</span>
          <kbd className="fb-kbd">Enter</kbd>
        </div>
        <div className="fb-info-item">
          <span className="fb-info-label">Go Back</span>
          <kbd className="fb-kbd">Backspace</kbd>
        </div>
        <div className="fb-info-item">
          <span className="fb-info-label">Rename</span>
          <kbd className="fb-kbd">F2</kbd>
        </div>
        <div className="fb-info-item">
          <span className="fb-info-label">Delete</span>
          <kbd className="fb-kbd">Del</kbd>
        </div>
        <div className="fb-info-item">
          <span className="fb-info-label">Select All</span>
          <kbd className="fb-kbd">Ctrl+A</kbd>
        </div>
        <div className="fb-info-item">
          <span className="fb-info-label">Refresh</span>
          <kbd className="fb-kbd">F5</kbd>
        </div>
      </div>

      <div className="fb-info-card">
        <h3>Tips</h3>
        <ul className="fb-info-tips">
          <li>Double-click folders to open them</li>
          <li>Double-click files to download</li>
          <li>Right-click for more options</li>
          <li>Drag and drop files to upload</li>
        </ul>
      </div>
    </div>
  )
}
