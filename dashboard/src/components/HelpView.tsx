import { useState } from 'react'

type HelpSection = 'overview' | 'terminal' | 'sessions' | 'files' | 'shortcuts' | 'tips'

interface SectionConfig {
  id: HelpSection
  label: string
  icon: string
}

const sections: SectionConfig[] = [
  { id: 'overview', label: 'Overview', icon: '01' },
  { id: 'terminal', label: 'Terminal', icon: '02' },
  { id: 'sessions', label: 'Sessions', icon: '03' },
  { id: 'files', label: 'Files', icon: '04' },
  { id: 'shortcuts', label: 'tmux', icon: '05' },
  { id: 'tips', label: 'Tips', icon: '06' },
]

function HelpView() {
  const [activeSection, setActiveSection] = useState<HelpSection>('overview')

  const renderContent = () => {
    switch (activeSection) {
      case 'overview':
        return <OverviewSection />
      case 'terminal':
        return <TerminalSection />
      case 'sessions':
        return <SessionsSection />
      case 'files':
        return <FilesSection />
      case 'shortcuts':
        return <ShortcutsSection />
      case 'tips':
        return <TipsSection />
      default:
        return <OverviewSection />
    }
  }

  return (
    <div className="help-view">
      <div className="help-header">
        <h1 className="help-title">Chrote Dashboard</h1>
        <p className="help-subtitle">Your command center for AI agent orchestration</p>
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

function OverviewSection() {
  return (
    <div className="help-section-content">
      <div className="help-hero">
        <div className="help-hero-icon">&#x2726;</div>
        <h2>Welcome to Chrote</h2>
        <p>A powerful dashboard for managing tmux sessions and AI agents in a unified interface.</p>
      </div>

      <div className="help-features-grid">
        <div className="help-feature-card">
          <div className="help-feature-icon">&#x25A0;</div>
          <h3>Multi-Terminal View</h3>
          <p>View up to 4 terminal sessions simultaneously in a responsive grid layout.</p>
        </div>

        <div className="help-feature-card">
          <div className="help-feature-icon">&#x21C4;</div>
          <h3>Drag & Drop</h3>
          <p>Organize sessions by dragging them between terminal windows or the sidebar.</p>
        </div>

        <div className="help-feature-card">
          <div className="help-feature-icon">&#x2630;</div>
          <h3>File Browser</h3>
          <p>Browse, upload, and manage files directly within the dashboard.</p>
        </div>

        <div className="help-feature-card">
          <div className="help-feature-icon">&#x2699;</div>
          <h3>Customizable</h3>
          <p>Choose from multiple themes and customize tmux colors to match your style.</p>
        </div>
      </div>

      <div className="help-card">
        <h3>Quick Start</h3>
        <ol className="help-steps">
          <li>
            <span className="help-step-number">1</span>
            <div className="help-step-content">
              <strong>View Sessions</strong>
              <p>Your tmux sessions appear in the left sidebar. Each session shows its agent name.</p>
            </div>
          </li>
          <li>
            <span className="help-step-number">2</span>
            <div className="help-step-content">
              <strong>Assign to Windows</strong>
              <p>Drag sessions from the sidebar onto any of the terminal windows to view them.</p>
            </div>
          </li>
          <li>
            <span className="help-step-number">3</span>
            <div className="help-step-content">
              <strong>Interact</strong>
              <p>Click on a terminal window to focus it, then type commands directly.</p>
            </div>
          </li>
        </ol>
      </div>
    </div>
  )
}

function TerminalSection() {
  return (
    <div className="help-section-content">
      <h2>Terminal Windows</h2>
      <p className="help-intro">The terminal view displays up to 4 tmux sessions in a responsive grid layout.</p>

      <div className="help-card">
        <h3>Window Layout</h3>
        <div className="help-layout-demo">
          <div className="help-layout-grid">
            <div className="help-layout-cell">Window 1</div>
            <div className="help-layout-cell">Window 2</div>
            <div className="help-layout-cell">Window 3</div>
            <div className="help-layout-cell">Window 4</div>
          </div>
        </div>
        <p className="help-hint">The grid automatically adjusts based on how many windows have sessions assigned.</p>
      </div>

      <div className="help-card">
        <h3>Window Controls</h3>
        <div className="help-control-list">
          <div className="help-control-item">
            <span className="help-control-icon">&#x25A1;</span>
            <div>
              <strong>Pop Out</strong>
              <p>Open a terminal in a floating window for better multitasking.</p>
            </div>
          </div>
          <div className="help-control-item">
            <span className="help-control-icon">&#x2715;</span>
            <div>
              <strong>Clear</strong>
              <p>Remove all sessions from a window without closing them.</p>
            </div>
          </div>
          <div className="help-control-item">
            <span className="help-control-icon">&#x21BB;</span>
            <div>
              <strong>Cycle Sessions</strong>
              <p>When multiple sessions are assigned, cycle through them with the arrow buttons.</p>
            </div>
          </div>
        </div>
      </div>

      <div className="help-card">
        <h3>Session Tags</h3>
        <p>Each window shows colored tags for assigned sessions. You can:</p>
        <ul className="help-list">
          <li>Click a tag to switch to that session</li>
          <li>Drag tags between windows to reorganize</li>
          <li>Drag tags outside to remove from the window</li>
        </ul>
      </div>
    </div>
  )
}

function SessionsSection() {
  return (
    <div className="help-section-content">
      <h2>Session Management</h2>
      <p className="help-intro">The sidebar shows all available tmux sessions and provides drag-and-drop organization.</p>

      <div className="help-card">
        <h3>Session Panel</h3>
        <p>The left sidebar displays all running tmux sessions. Sessions are identified by:</p>
        <ul className="help-list">
          <li><strong>Agent Name</strong> - Extracted from the session name (e.g., "claude" from "gastown-claude-abc123")</li>
          <li><strong>Color Indicator</strong> - Each session gets a unique color for easy identification</li>
          <li><strong>Assignment Status</strong> - Dimmed appearance if already assigned to a window</li>
        </ul>
      </div>

      <div className="help-card">
        <h3>Drag & Drop</h3>
        <div className="help-drag-demo">
          <div className="help-drag-source">
            <span>Session</span>
            <span className="help-drag-arrow">&#x2192;</span>
          </div>
          <div className="help-drag-target">
            <span>Window</span>
          </div>
        </div>
        <ul className="help-list">
          <li>Drag from sidebar to assign a session to a window</li>
          <li>Drag between windows to move sessions</li>
          <li>Drag outside any window to unassign</li>
        </ul>
      </div>

      <div className="help-card">
        <h3>Panel Controls</h3>
        <div className="help-control-list">
          <div className="help-control-item">
            <span className="help-control-icon">&#x276E;</span>
            <div>
              <strong>Collapse/Expand</strong>
              <p>Toggle the sidebar to maximize terminal space.</p>
            </div>
          </div>
          <div className="help-control-item">
            <span className="help-control-icon">&#x21BB;</span>
            <div>
              <strong>Auto-Refresh</strong>
              <p>Sessions automatically refresh. Adjust the interval in Settings.</p>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

function FilesSection() {
  return (
    <div className="help-section-content">
      <h2>File Browser</h2>
      <p className="help-intro">Browse and manage files on the server through an integrated file manager.</p>

      <div className="help-card">
        <h3>Features</h3>
        <div className="help-control-list">
          <div className="help-control-item">
            <span className="help-control-icon">&#x1F4C1;</span>
            <div>
              <strong>Browse Files</strong>
              <p>Navigate through directories with a familiar file explorer interface.</p>
            </div>
          </div>
          <div className="help-control-item">
            <span className="help-control-icon">&#x2191;</span>
            <div>
              <strong>Upload</strong>
              <p>Drag and drop files to upload them to the server.</p>
            </div>
          </div>
          <div className="help-control-item">
            <span className="help-control-icon">&#x2193;</span>
            <div>
              <strong>Download</strong>
              <p>Download files directly to your local machine.</p>
            </div>
          </div>
          <div className="help-control-item">
            <span className="help-control-icon">&#x270E;</span>
            <div>
              <strong>Edit</strong>
              <p>Open and edit text files with syntax highlighting.</p>
            </div>
          </div>
        </div>
      </div>

      <div className="help-card help-card-accent">
        <h3>Workspace Path</h3>
        <p>The file browser is rooted at the configured workspace directory. All agent operations happen within this sandboxed environment.</p>
      </div>
    </div>
  )
}

function ShortcutsSection() {
  return (
    <div className="help-section-content">
      <h2>tmux Commands</h2>
      <p className="help-intro">Useful tmux commands for working with terminal sessions. The prefix key is <kbd>Ctrl</kbd> + <kbd>B</kbd>.</p>

      <div className="help-card">
        <h3>Copy & Scroll Mode</h3>
        <div className="help-shortcuts-table">
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Prefix</kbd> <kbd>[</kbd>
            </div>
            <span>Enter copy/scroll mode</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>q</kbd>
            </div>
            <span>Exit copy mode</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Space</kbd>
            </div>
            <span>Start selection (in copy mode)</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Enter</kbd>
            </div>
            <span>Copy selection (in copy mode)</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Prefix</kbd> <kbd>]</kbd>
            </div>
            <span>Paste copied text</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>g</kbd>
            </div>
            <span>Go to top (in copy mode)</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>G</kbd>
            </div>
            <span>Go to bottom (in copy mode)</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>/</kbd>
            </div>
            <span>Search forward (in copy mode)</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>?</kbd>
            </div>
            <span>Search backward (in copy mode)</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>n</kbd> / <kbd>N</kbd>
            </div>
            <span>Next / previous search result</span>
          </div>
        </div>
      </div>

      <div className="help-card">
        <h3>Window Management</h3>
        <div className="help-shortcuts-table">
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Prefix</kbd> <kbd>c</kbd>
            </div>
            <span>Create new window</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Prefix</kbd> <kbd>n</kbd>
            </div>
            <span>Next window</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Prefix</kbd> <kbd>p</kbd>
            </div>
            <span>Previous window</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Prefix</kbd> <kbd>0-9</kbd>
            </div>
            <span>Switch to window by number</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Prefix</kbd> <kbd>w</kbd>
            </div>
            <span>List all windows</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Prefix</kbd> <kbd>&</kbd>
            </div>
            <span>Kill current window</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Prefix</kbd> <kbd>,</kbd>
            </div>
            <span>Rename current window</span>
          </div>
        </div>
      </div>

      <div className="help-card">
        <h3>Pane Management</h3>
        <div className="help-shortcuts-table">
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Prefix</kbd> <kbd>%</kbd>
            </div>
            <span>Split pane vertically</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Prefix</kbd> <kbd>"</kbd>
            </div>
            <span>Split pane horizontally</span>
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
            <span>Toggle pane zoom (fullscreen)</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Prefix</kbd> <kbd>x</kbd>
            </div>
            <span>Kill current pane</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Prefix</kbd> <kbd>o</kbd>
            </div>
            <span>Cycle through panes</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Prefix</kbd> <kbd>{'{'}</kbd> / <kbd>{'}'}</kbd>
            </div>
            <span>Swap pane left / right</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Prefix</kbd> <kbd>Ctrl+Arrow</kbd>
            </div>
            <span>Resize pane</span>
          </div>
        </div>
      </div>

      <div className="help-card">
        <h3>Session Commands</h3>
        <div className="help-shortcuts-table">
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Prefix</kbd> <kbd>d</kbd>
            </div>
            <span>Detach from session</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Prefix</kbd> <kbd>s</kbd>
            </div>
            <span>List all sessions</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Prefix</kbd> <kbd>$</kbd>
            </div>
            <span>Rename current session</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Prefix</kbd> <kbd>(</kbd> / <kbd>)</kbd>
            </div>
            <span>Previous / next session</span>
          </div>
        </div>
      </div>

      <div className="help-card">
        <h3>Miscellaneous</h3>
        <div className="help-shortcuts-table">
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Prefix</kbd> <kbd>:</kbd>
            </div>
            <span>Enter command mode</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Prefix</kbd> <kbd>?</kbd>
            </div>
            <span>List all key bindings</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Prefix</kbd> <kbd>t</kbd>
            </div>
            <span>Show clock</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <kbd>Prefix</kbd> <kbd>~</kbd>
            </div>
            <span>Show tmux messages</span>
          </div>
        </div>
      </div>
    </div>
  )
}

function TipsSection() {
  return (
    <div className="help-section-content">
      <h2>Tips & Tricks</h2>
      <p className="help-intro">Get the most out of Chrote with these pro tips.</p>

      <div className="help-tips-grid">
        <div className="help-tip-card">
          <div className="help-tip-number">01</div>
          <h3>Scroll History</h3>
          <p>Use <kbd>Ctrl+B</kbd> then <kbd>[</kbd> to enter copy mode and scroll through terminal history with arrow keys or Page Up/Down.</p>
        </div>

        <div className="help-tip-card">
          <div className="help-tip-number">02</div>
          <h3>Quick Copy Text</h3>
          <p>Hold <kbd>Shift</kbd> while selecting text with your mouse to bypass tmux and perform a native selection. Then use <kbd>Ctrl+C</kbd> to copy.</p>
        </div>

        <div className="help-tip-card">
          <div className="help-tip-number">03</div>
          <h3>Search in Terminal</h3>
          <p>In copy mode, press <kbd>/</kbd> to search forward or <kbd>?</kbd> to search backward through the terminal output.</p>
        </div>

        <div className="help-tip-card">
          <div className="help-tip-number">04</div>
          <h3>Theme Matching</h3>
          <p>Match your tmux theme with the dashboard theme in Settings for a cohesive experience.</p>
        </div>

        <div className="help-tip-card">
          <div className="help-tip-number">05</div>
          <h3>Collapse for Space</h3>
          <p>Collapse the session panel when you need maximum terminal real estate.</p>
        </div>

        <div className="help-tip-card">
          <div className="help-tip-number">06</div>
          <h3>CLI Commands</h3>
          <p>Use <code>gt peek</code> and <code>gt status</code> in any terminal for Gastown monitoring.</p>
        </div>
      </div>

      <div className="help-card help-card-accent">
        <h3>Need More Help?</h3>
        <p>Check the <strong>Status</strong> tab to verify system health, or visit the <strong>Settings</strong> tab to customize your experience.</p>
      </div>
    </div>
  )
}

export default HelpView
