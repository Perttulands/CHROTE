import { useState } from 'react'

type ManualSection = 'overview' | 'getting-started' | 'roles' | 'commands' | 'workflows'

interface SectionConfig {
  id: ManualSection
  label: string
  icon: string
}

const sections: SectionConfig[] = [
  { id: 'overview', label: 'Overview', icon: '01' },
  { id: 'getting-started', label: 'Getting Started', icon: '02' },
  { id: 'roles', label: 'Roles', icon: '03' },
  { id: 'commands', label: 'Commands', icon: '04' },
  { id: 'workflows', label: 'Workflows', icon: '05' },
]

function ManualView() {
  const [activeSection, setActiveSection] = useState<ManualSection>('overview')

  const renderContent = () => {
    switch (activeSection) {
      case 'overview':
        return <OverviewSection />
      case 'getting-started':
        return <GettingStartedSection />
      case 'roles':
        return <RolesSection />
      case 'commands':
        return <CommandsSection />
      case 'workflows':
        return <WorkflowsSection />
      default:
        return <OverviewSection />
    }
  }

  return (
    <div className="help-view">
      <div className="help-header">
        <h1 className="help-title">Operator's Manual</h1>
        <p className="help-subtitle">Gas Town documentation and reference</p>
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
        <div className="help-hero-icon">&#x2699;</div>
        <h2>Gas Town</h2>
        <p>A multi-agent orchestration system for AI-powered software development.</p>
      </div>

      <div className="help-features-grid">
        <div className="help-feature-card">
          <div className="help-feature-icon">&#x1F3ED;</div>
          <h3>Rigs</h3>
          <p>Isolated workspaces for different projects or repositories.</p>
        </div>

        <div className="help-feature-card">
          <div className="help-feature-icon">&#x1F431;</div>
          <h3>Polecats</h3>
          <p>Ephemeral worker agents that handle individual tasks.</p>
        </div>

        <div className="help-feature-card">
          <div className="help-feature-icon">&#x1F4BF;</div>
          <h3>Beads</h3>
          <p>Work items that track tasks, issues, and progress.</p>
        </div>

        <div className="help-feature-card">
          <div className="help-feature-icon">&#x1F4EC;</div>
          <h3>Mail</h3>
          <p>Messaging system for agent communication and coordination.</p>
        </div>
      </div>

      <div className="help-card">
        <h3>Architecture</h3>
        <p>Gas Town organizes work through a hierarchy of components:</p>
        <ul className="help-list">
          <li><strong>Town</strong> - The top-level workspace containing all rigs</li>
          <li><strong>Rigs</strong> - Individual project workspaces with their own agents</li>
          <li><strong>Agents</strong> - Workers (polecats, crew, witness, etc.) that execute tasks</li>
          <li><strong>Beads</strong> - Work items that flow through the system</li>
        </ul>
      </div>
    </div>
  )
}

function GettingStartedSection() {
  return (
    <div className="help-section-content">
      <h2>Getting Started</h2>
      <p className="help-intro">Quick guide to start working with Gas Town.</p>

      <div className="help-card">
        <h3>Essential Commands</h3>
        <div className="help-shortcuts-table">
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <code>gt status</code>
            </div>
            <span>Show overall town status</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <code>gt hook</code>
            </div>
            <span>Check your current hooked work</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <code>gt mail inbox</code>
            </div>
            <span>Check your inbox for messages</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <code>gt ready</code>
            </div>
            <span>Show work ready across town</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <code>gt done</code>
            </div>
            <span>Signal work is ready for merge</span>
          </div>
        </div>
      </div>

      <div className="help-card">
        <h3>Workflow</h3>
        <ol className="help-steps">
          <li>
            <span className="help-step-number">1</span>
            <div className="help-step-content">
              <strong>Check your hook</strong>
              <p>Run <code>gt hook</code> to see if work has been assigned to you.</p>
            </div>
          </li>
          <li>
            <span className="help-step-number">2</span>
            <div className="help-step-content">
              <strong>Complete the task</strong>
              <p>Work on the assigned bead, making commits as needed.</p>
            </div>
          </li>
          <li>
            <span className="help-step-number">3</span>
            <div className="help-step-content">
              <strong>Signal completion</strong>
              <p>Run <code>gt done</code> when ready for review and merge.</p>
            </div>
          </li>
        </ol>
      </div>
    </div>
  )
}

function RolesSection() {
  return (
    <div className="help-section-content">
      <h2>Agent Roles</h2>
      <p className="help-intro">Gas Town uses specialized agents for different responsibilities.</p>

      <div className="help-card">
        <h3>Town-Level Agents</h3>
        <div className="help-control-list">
          <div className="help-control-item">
            <span className="help-control-icon">&#x1F3A9;</span>
            <div>
              <strong>Mayor</strong>
              <p>Chief of Staff for cross-rig coordination and high-level decisions.</p>
            </div>
          </div>
          <div className="help-control-item">
            <span className="help-control-icon">&#x1F43A;</span>
            <div>
              <strong>Deacon</strong>
              <p>Town-level watchdog for system health and escalations.</p>
            </div>
          </div>
        </div>
      </div>

      <div className="help-card">
        <h3>Rig-Level Agents</h3>
        <div className="help-control-list">
          <div className="help-control-item">
            <span className="help-control-icon">&#x1F989;</span>
            <div>
              <strong>Witness</strong>
              <p>Per-rig polecat health monitor that watches over worker agents.</p>
            </div>
          </div>
          <div className="help-control-item">
            <span className="help-control-icon">&#x1F3ED;</span>
            <div>
              <strong>Refinery</strong>
              <p>Merge queue processor that handles completed work.</p>
            </div>
          </div>
          <div className="help-control-item">
            <span className="help-control-icon">&#x1F431;</span>
            <div>
              <strong>Polecat</strong>
              <p>Ephemeral workers that handle individual tasks then terminate.</p>
            </div>
          </div>
          <div className="help-control-item">
            <span className="help-control-icon">&#x1F477;</span>
            <div>
              <strong>Crew</strong>
              <p>Persistent workspaces for human operators.</p>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

function CommandsSection() {
  return (
    <div className="help-section-content">
      <h2>Command Reference</h2>
      <p className="help-intro">Common gt commands organized by category.</p>

      <div className="help-card">
        <h3>Work Management</h3>
        <div className="help-shortcuts-table">
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <code>gt hook</code>
            </div>
            <span>Show or attach work on your hook</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <code>gt sling</code>
            </div>
            <span>Assign work to an agent</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <code>gt done</code>
            </div>
            <span>Signal work ready for merge queue</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <code>gt show &lt;bead&gt;</code>
            </div>
            <span>Show details of a bead</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <code>gt close &lt;bead&gt;</code>
            </div>
            <span>Close one or more beads</span>
          </div>
        </div>
      </div>

      <div className="help-card">
        <h3>Communication</h3>
        <div className="help-shortcuts-table">
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <code>gt mail inbox</code>
            </div>
            <span>Check your inbox</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <code>gt nudge &lt;agent&gt;</code>
            </div>
            <span>Send a message to an agent</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <code>gt broadcast</code>
            </div>
            <span>Send a message to all workers</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <code>gt escalate</code>
            </div>
            <span>Escalate critical issues</span>
          </div>
        </div>
      </div>

      <div className="help-card">
        <h3>Monitoring</h3>
        <div className="help-shortcuts-table">
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <code>gt status</code>
            </div>
            <span>Show overall town status</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <code>gt ready</code>
            </div>
            <span>Show work ready across town</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <code>gt trail</code>
            </div>
            <span>Show recent agent activity</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <code>gt peek &lt;agent&gt;</code>
            </div>
            <span>View recent output from an agent</span>
          </div>
        </div>
      </div>
    </div>
  )
}

function WorkflowsSection() {
  return (
    <div className="help-section-content">
      <h2>Workflows</h2>
      <p className="help-intro">Common patterns for working with Gas Town.</p>

      <div className="help-card">
        <h3>Polecat Work Cycle</h3>
        <ol className="help-steps">
          <li>
            <span className="help-step-number">1</span>
            <div className="help-step-content">
              <strong>Work is slung</strong>
              <p>Mayor or Crew assigns a bead to a polecat via <code>gt sling</code>.</p>
            </div>
          </li>
          <li>
            <span className="help-step-number">2</span>
            <div className="help-step-content">
              <strong>Polecat executes</strong>
              <p>The polecat works on the task, checking <code>gt hook</code> for details.</p>
            </div>
          </li>
          <li>
            <span className="help-step-number">3</span>
            <div className="help-step-content">
              <strong>Work completes</strong>
              <p>Polecat runs <code>gt done</code> to signal completion.</p>
            </div>
          </li>
          <li>
            <span className="help-step-number">4</span>
            <div className="help-step-content">
              <strong>Refinery processes</strong>
              <p>The Refinery merges approved work into the main branch.</p>
            </div>
          </li>
        </ol>
      </div>

      <div className="help-card">
        <h3>Convoy Pattern</h3>
        <p>Convoys batch related work across multiple rigs:</p>
        <ul className="help-list">
          <li>A convoy groups beads that should be completed together</li>
          <li>Progress is tracked across all included beads</li>
          <li>Use <code>gt convoy</code> to manage batch operations</li>
        </ul>
      </div>

      <div className="help-card help-card-accent">
        <h3>Best Practices</h3>
        <ul className="help-list">
          <li>Always check <code>gt hook</code> at the start of a session</li>
          <li>Use <code>gt commit</code> instead of <code>git commit</code> for proper attribution</li>
          <li>Signal completion promptly with <code>gt done</code></li>
          <li>Check <code>gt mail inbox</code> for important messages</li>
        </ul>
      </div>
    </div>
  )
}

export default ManualView
