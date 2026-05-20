import { useState } from 'react'

type HelpSection = 'shortcuts' | 'terminals' | 'sessions' | 'files' | 'tmux'

interface SectionConfig {
  id: HelpSection
  label: string
  icon: string
}

const sections: SectionConfig[] = [
  { id: 'shortcuts', label: 'Shortcuts', icon: '01' },
  { id: 'terminals', label: 'Terminals', icon: '02' },
  { id: 'sessions', label: 'Sessions', icon: '03' },
  { id: 'files', label: 'Files', icon: '04' },
  { id: 'tmux', label: 'tmux', icon: '05' },
]

function HelpView() {
  const [activeSection, setActiveSection] = useState<HelpSection>('shortcuts')

  const renderContent = () => {
    switch (activeSection) {
      case 'shortcuts':
        return <ShortcutsSection />
      case 'terminals':
        return <TerminalsSection />
      case 'sessions':
        return <SessionsSection />
      case 'files':
        return <FilesSection />
      case 'tmux':
        return <TmuxSection />
      default:
        return <ShortcutsSection />
    }
  }

  return (
    <div className="help-view">
      <div className="help-header">
        <h1 className="help-title">Dashboard Help</h1>
        <p className="help-subtitle">How to use this interface.</p>
      </div>

      <nav className="help-nav">
        {sections.map((section) => (
          <button
            key={section.id}
            className={`help-nav-item ${activeSection === section.id ? 'active' : ''}`}
            onClick={() => setActiveSection(section.id)}
          >
            <span className="help-nav-icon">{section.icon}</span>
            <span className="help-nav-label">{section.label}</span>
          </button>
        ))}
      </nav>

      <div className="help-content">
        {renderContent()}
      </div>
    </div>
  )
}

function ShortcutsSection() {
  return (
    <div className="help-section-content">
      <h2>Keyboard Shortcuts</h2>
      <p className="help-intro">Press <kbd>?</kbd> anywhere to see this overlay.</p>

      <div className="help-card">
        <h3>Navigation</h3>
        <div className="help-shortcuts-table">
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Tab</kbd>
            </div>
            <span>Toggle between Terminal 1 and Terminal 2</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>1</kbd> <kbd>2</kbd> <kbd>3</kbd> <kbd>4</kbd>
            </div>
            <span>Focus terminal pane by number</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>/</kbd>
            </div>
            <span>Focus the session search box</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Esc</kbd>
            </div>
            <span>Close modals and overlays</span>
          </div>
        </div>
      </div>

      <div className="help-card">
        <h3>Actions</h3>
        <div className="help-shortcuts-table">
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Ctrl</kbd> + <kbd>S</kbd>
            </div>
            <span>Toggle the sidebar</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Ctrl</kbd> + <kbd>N</kbd>
            </div>
            <span>Create a new session</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Ctrl</kbd> + <kbd>1-9</kbd>
            </div>
            <span>Load layout preset by number</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>?</kbd>
            </div>
            <span>Show keyboard shortcuts overlay</span>
          </div>
        </div>
      </div>

      <div className="help-card help-card-accent">
        <h3>Quick Tips</h3>
        <ul className="help-list">
          <li>Hold <kbd>Shift</kbd> while selecting text to bypass tmux and copy natively</li>
          <li>Collapse the sidebar with <kbd>Ctrl+S</kbd> for maximum terminal space</li>
          <li>Use <kbd>Tab</kbd> to quickly flip between your two terminal workspaces</li>
        </ul>
      </div>
    </div>
  )
}

function TerminalsSection() {
  return (
    <div className="help-section-content">
      <h2>Terminal Panes</h2>
      <p className="help-intro">Each workspace has up to 4 panes in a responsive grid.</p>

      <div className="help-card">
        <h3>Two Workspaces</h3>
        <p>
          <strong>Terminal 1</strong> and <strong>Terminal 2</strong> are independent.
          Each has its own set of 4 panes with different sessions bound.
          Use <kbd>Tab</kbd> to switch between them.
        </p>
      </div>

      <div className="help-card">
        <h3>Pane Controls</h3>
        <div className="help-control-list">
          <div className="help-control-item">
            <span className="help-control-icon">&#x25A1;</span>
            <div>
              <strong>Pop Out</strong>
              <p>Open terminal in a floating window.</p>
            </div>
          </div>
          <div className="help-control-item">
            <span className="help-control-icon">&#x2715;</span>
            <div>
              <strong>Clear</strong>
              <p>Unbind sessions from this pane (doesn't kill them).</p>
            </div>
          </div>
          <div className="help-control-item">
            <span className="help-control-icon">&#x21BB;</span>
            <div>
              <strong>Cycle</strong>
              <p>When multiple sessions are bound, cycle through them.</p>
            </div>
          </div>
        </div>
      </div>

      <div className="help-card">
        <h3>Session Tags</h3>
        <p>Each pane shows colored tags for bound sessions:</p>
        <ul className="help-list">
          <li><strong>Click a tag</strong> - Switch to that session</li>
          <li><strong>Drag a tag</strong> - Move to another pane</li>
          <li><strong>Drag outside</strong> - Unbind from pane</li>
        </ul>
      </div>

      <div className="help-card">
        <h3>Watch vs Control Mode</h3>
        <div className="help-control-list">
          <div className="help-control-item">
            <span className="help-control-icon">&#x1F441;</span>
            <div>
              <strong>Watch Mode (default)</strong>
              <p>See output, scroll freely. Keyboard input disabled.</p>
            </div>
          </div>
          <div className="help-control-item">
            <span className="help-control-icon">&#x2328;</span>
            <div>
              <strong>Control Mode</strong>
              <p>Click the pane to enable input. Type directly into tmux.</p>
            </div>
          </div>
        </div>
        <p style={{ marginTop: '1rem', opacity: 0.8 }}>
          Watch mode is default because agents don't like surprise keyboard input.
        </p>
      </div>
    </div>
  )
}

function SessionsSection() {
  return (
    <div className="help-section-content">
      <h2>Session Panel</h2>
      <p className="help-intro">The left sidebar lists all tmux sessions.</p>

      <div className="help-card">
        <h3>Click vs Drag</h3>
        <div className="help-control-list">
          <div className="help-control-item">
            <span className="help-control-icon">&#x1F441;</span>
            <div>
              <strong>Click = Peek</strong>
              <p>Opens a modal with read-only preview. Quick look without binding.</p>
            </div>
          </div>
          <div className="help-control-item">
            <span className="help-control-icon">&#x2725;</span>
            <div>
              <strong>Drag = Bind</strong>
              <p>Drag onto a pane to bind the session there. It stays until you remove it.</p>
            </div>
          </div>
        </div>
      </div>

      <div className="help-card">
        <h3>Session Groups</h3>
        <p>Sessions auto-organize by naming convention:</p>
        <div className="help-shortcuts-table">
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <code>hq-*</code>
            </div>
            <span>HQ group (top priority)</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <code>main</code> / <code>shell</code>
            </div>
            <span>Main group</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <code>gt-*</code>
            </div>
            <span>Gastown workers</span>
          </div>
        </div>
      </div>

      <div className="help-card">
        <h3>Search & Filter</h3>
        <p>
          Press <kbd>/</kbd> to focus the search box. Type to filter sessions by name.
          Clear the search to see all sessions again.
        </p>
      </div>

      <div className="help-card">
        <h3>Context Menu</h3>
        <p>Right-click a session for options:</p>
        <ul className="help-list">
          <li><strong>Rename</strong> - Change the session name</li>
          <li><strong>Kill</strong> - Terminate the session</li>
        </ul>
      </div>
    </div>
  )
}

function FilesSection() {
  return (
    <div className="help-section-content">
      <h2>File Browser</h2>
      <p className="help-intro">Browse files on the server. Read-only for /vault.</p>

      <div className="help-card">
        <h3>Available Paths</h3>
        <div className="help-shortcuts-table">
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <code>/code</code>
            </div>
            <span>Project files (read/write)</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <code>/vault</code>
            </div>
            <span>Reference files (read-only)</span>
          </div>
        </div>
        <p style={{ marginTop: '1rem', opacity: 0.8 }}>
          All other paths are restricted for security.
        </p>
      </div>

      <div className="help-card">
        <h3>Features</h3>
        <div className="help-control-list">
          <div className="help-control-item">
            <span className="help-control-icon">&#x1F4C1;</span>
            <div>
              <strong>Navigate</strong>
              <p>Click folders to enter, breadcrumbs to go back.</p>
            </div>
          </div>
          <div className="help-control-item">
            <span className="help-control-icon">&#x1F4C4;</span>
            <div>
              <strong>View</strong>
              <p>Click files to preview with syntax highlighting.</p>
            </div>
          </div>
          <div className="help-control-item">
            <span className="help-control-icon">&#x2193;</span>
            <div>
              <strong>Download</strong>
              <p>Download files to your local machine.</p>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

function TmuxSection() {
  return (
    <div className="help-section-content">
      <h2>tmux Reference</h2>
      <p className="help-intro">Useful when you're in control mode. Prefix is <kbd>Ctrl</kbd> + <kbd>B</kbd>.</p>

      <div className="help-card">
        <h3>Scrolling & Copy</h3>
        <div className="help-shortcuts-table">
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Prefix</kbd> <kbd>[</kbd>
            </div>
            <span>Enter scroll/copy mode</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>q</kbd>
            </div>
            <span>Exit copy mode</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>g</kbd> / <kbd>G</kbd>
            </div>
            <span>Go to top / bottom</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>/</kbd> / <kbd>?</kbd>
            </div>
            <span>Search forward / backward</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Space</kbd>
            </div>
            <span>Start selection</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Enter</kbd>
            </div>
            <span>Copy selection</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Prefix</kbd> <kbd>]</kbd>
            </div>
            <span>Paste</span>
          </div>
        </div>
      </div>

      <div className="help-card">
        <h3>Panes (within tmux)</h3>
        <div className="help-shortcuts-table">
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Prefix</kbd> <kbd>%</kbd>
            </div>
            <span>Split vertical</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Prefix</kbd> <kbd>"</kbd>
            </div>
            <span>Split horizontal</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Prefix</kbd> <kbd>Arrow</kbd>
            </div>
            <span>Move between panes</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Prefix</kbd> <kbd>z</kbd>
            </div>
            <span>Toggle pane zoom</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Prefix</kbd> <kbd>x</kbd>
            </div>
            <span>Kill pane</span>
          </div>
        </div>
      </div>

      <div className="help-card">
        <h3>Sessions</h3>
        <div className="help-shortcuts-table">
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Prefix</kbd> <kbd>d</kbd>
            </div>
            <span>Detach</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Prefix</kbd> <kbd>s</kbd>
            </div>
            <span>List sessions</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Prefix</kbd> <kbd>$</kbd>
            </div>
            <span>Rename session</span>
          </div>
        </div>
      </div>

      <div className="help-card help-card-accent">
        <h3>Pro Tip</h3>
        <p>
          Hold <kbd>Shift</kbd> while selecting text with your mouse to bypass tmux
          and use native browser copy. Works in watch mode too.
        </p>
      </div>
    </div>
  )
}

export default HelpView
