import { useState } from 'react'

type ManualSection = 'overview' | 'agents' | 'molecules' | 'beads' | 'philosophy' | 'links'

interface SectionConfig {
  id: ManualSection
  label: string
  icon: string
}

const sections: SectionConfig[] = [
  { id: 'overview', label: 'Overview', icon: '00' },
  { id: 'agents', label: 'Agents', icon: '01' },
  { id: 'molecules', label: 'Molecules', icon: '02' },
  { id: 'beads', label: 'Beads', icon: '03' },
  { id: 'philosophy', label: 'Philosophy', icon: '04' },
  { id: 'links', label: 'Links', icon: '05' },
]

function ManualView() {
  const [activeSection, setActiveSection] = useState<ManualSection>('overview')

  const renderContent = () => {
    switch (activeSection) {
      case 'overview':
        return <OverviewSection />
      case 'agents':
        return <AgentsSection />
      case 'molecules':
        return <MoleculesSection />
      case 'beads':
        return <BeadsSection />
      case 'philosophy':
        return <PhilosophySection />
      case 'links':
        return <LinksSection />
      default:
        return <OverviewSection />
    }
  }

  return (
    <div className="help-view">
      <div className="help-header">
        <h1 className="help-title">Operator's Manual</h1>
        <p className="help-subtitle">You don't run commands. You tell agents what to do.</p>
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
        <div className="help-hero-icon">&#x1F3ED;</div>
        <h2>Welcome to Gas Town</h2>
        <p>A multi-agent orchestrator for Claude Code.</p>
      </div>

      <div className="help-card help-card-accent">
        <h3>What Is This?</h3>
        <p>
          Gas Town presents itself as a single-agent interface, but in the background it spawns
          and manages a series of specialized agents to get many threads of work done in parallel.
        </p>
        <p style={{ marginTop: '0.75rem' }}>
          It's more like a <strong>coding agent factory</strong> than a coding agent.
          You talk to the factory foreman—the Mayor—and they coordinate as many workers as needed
          to get your tasks done.
        </p>
      </div>

      <div className="help-card">
        <h3>The Mad Max Theme</h3>
        <p>
          "Gas Town" is a reference to the oil refinery citadel in Mad Max.
          The theme continues throughout:
        </p>
        <ul className="help-list">
          <li><strong>Rigs</strong> - Project workspaces</li>
          <li><strong>Refinery</strong> - Merge queue processor</li>
          <li><strong>Convoy</strong> - Parallel work swarms</li>
          <li><strong>Guzzoline</strong> - The fuel that powers the work</li>
        </ul>
        <p style={{ marginTop: '0.75rem', opacity: 0.8 }}>
          The Deacon is named for a Dennis Hopper character from Waterworld,
          inspired by Lord Humungus from Mad Max. It's also short for "daemon beacon."
        </p>
      </div>

      <div className="help-card">
        <h3>Running with Just the Mayor</h3>
        <p>
          You technically only need the Mayor. Running with just the Mayor is a good tutorial.
          The Mayor can file and fix issues itself, or you can ask it to sling work to polecats.
        </p>
        <p style={{ marginTop: '0.75rem', opacity: 0.8 }}>
          Even alone, it still gets all the benefits of GUPP, MEOW, and the other protocols.
        </p>
      </div>

      <div className="help-card">
        <h3>Entry Points</h3>
        <div className="help-shortcuts-table">
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <code>gt may at</code>
            </div>
            <span>Attach to the Mayor (main interface)</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <code>gt crew at</code>
            </div>
            <span>Attach to a Crew workspace</span>
          </div>
        </div>
        <p style={{ marginTop: '1rem', opacity: 0.8 }}>
          These are your two main entry points to Gas Town.
        </p>
      </div>
    </div>
  )
}

function AgentsSection() {
  return (
    <div className="help-section-content">
      <h2>The Seven Roles</h2>
      <p className="help-intro">Plus you—the Overseer. That's eight.</p>

      <div className="help-card help-card-accent">
        <h3>Who Do You Talk To?</h3>
        <p>
          The Mayor is the main agent you talk to most of the time.
          It's your concierge and chief-of-staff.
        </p>
        <p style={{ marginTop: '0.75rem' }}>
          But if the Mayor is busy, all the other workers are also Claude Code—
          so they're all very smart and helpful.
        </p>
      </div>

      <div className="help-card">
        <h3>Town-Level Roles</h3>
        <div className="help-control-list">
          <div className="help-control-item">
            <span className="help-control-icon">&#x1F3A9;</span>
            <div>
              <strong>Mayor</strong>
              <p>Your main point of contact. Kicks off most work convoys and receives notifications when they finish.</p>
            </div>
          </div>
          <div className="help-control-item">
            <span className="help-control-icon">&#x1F43A;</span>
            <div>
              <strong>Deacon</strong>
              <p>Runs town-level plugins and handles <code>gt handoff</code>, agent recycling, and worker cleanup. Background role.</p>
            </div>
          </div>
          <div className="help-control-item">
            <span className="help-control-icon">&#x1F415;</span>
            <div>
              <strong>Dogs</strong>
              <p>The Deacon's personal crew. Handle complex work and investigations so patrol steps don't interfere with core eventing.</p>
            </div>
          </div>
        </div>
        <p style={{ marginTop: '1rem', opacity: 0.8 }}>
          Dogs are named for the MI5 internal security characters in Mick Herron's Slow Horses novels.
        </p>
      </div>

      <div className="help-card">
        <h3>Per-Rig Roles</h3>
        <div className="help-control-list">
          <div className="help-control-item">
            <span className="help-control-icon">&#x1F431;</span>
            <div>
              <strong>Polecats</strong>
              <p>Ephemeral, unsupervised workers. Pick up slung work, complete it, submit MRs without human intervention. Best for well-defined, well-specified work.</p>
            </div>
          </div>
          <div className="help-control-item">
            <span className="help-control-icon">&#x1F989;</span>
            <div>
              <strong>Witness</strong>
              <p>Checks on polecat and refinery wellbeing. Peeks in on the Deacon. Runs rig-level plugins.</p>
            </div>
          </div>
          <div className="help-control-item">
            <span className="help-control-icon">&#x1F3ED;</span>
            <div>
              <strong>Refinery</strong>
              <p>Manages merges. Most useful when you've done a LOT of up-front specification work and have huge piles of beads to churn through.</p>
            </div>
          </div>
          <div className="help-control-item">
            <span className="help-control-icon">&#x1F477;</span>
            <div>
              <strong>Crew</strong>
              <p>Named, long-lived workers on each project rig. Your design team. They create guzzoline for the swarms.</p>
            </div>
          </div>
        </div>
      </div>

      <div className="help-card">
        <h3>The Overseer (You)</h3>
        <p>
          That's you, Human. The eighth role. As the Overseer, you have an identity in the system,
          your own inbox, and you can send and receive town mail.
        </p>
        <p style={{ marginTop: '0.75rem', fontWeight: 'bold' }}>
          You're the boss, the head honcho, the big cheese.
        </p>
      </div>
    </div>
  )
}

function MoleculesSection() {
  return (
    <div className="help-section-content">
      <h2>Formulas & Molecules</h2>
      <p className="help-intro">Durable workflows that survive crashes.</p>

      <div className="help-card help-card-accent">
        <h3>The Core Idea</h3>
        <p>
          Molecules are workflows chained with Beads. They can have arbitrary shapes—
          unlike epics—and they can be stitched together at runtime.
        </p>
        <p style={{ marginTop: '0.75rem' }}>
          The key benefit: <strong>workflows survive agent crashes, compactions, restarts, and interruptions.</strong>
          {' '}Just start the agent up in the same sandbox, have it find its place in the molecule,
          and pick up where it left off.
        </p>
      </div>

      <div className="help-card">
        <h3>The Lifecycle</h3>
        <ol className="help-steps">
          <li>
            <span className="help-step-number">1</span>
            <div className="help-step-content">
              <strong>Formula (TOML)</strong>
              <p>Source form for workflows. Stored in <code>.beads/formulas/</code>.</p>
            </div>
          </li>
          <li>
            <span className="help-step-number">2</span>
            <div className="help-step-content">
              <strong>Protomolecule</strong>
              <p>Like a class or template. An entire graph of template issues with instructions and dependencies, ready to instantiate.</p>
            </div>
          </li>
          <li>
            <span className="help-step-number">3</span>
            <div className="help-step-content">
              <strong>Molecule</strong>
              <p>Instantiated from a protomolecule via variable substitution. A real workflow with actual beads.</p>
            </div>
          </li>
        </ol>
      </div>

      <div className="help-card">
        <h3>Hooks</h3>
        <p>
          Every Gas Town worker has its own <strong>hook</strong> 🪝.
          It's a special pinned bead, just for that agent, where you hang molecules.
        </p>
        <p style={{ marginTop: '0.75rem' }}>
          You <strong>sling</strong> work to workers, and it goes on their hook.
          The hook is persistent—a Bead backed by Git.
        </p>
      </div>

      <div className="help-card">
        <h3>Why Molecules Are Durable</h3>
        <p>If a molecule is on an agent's hook:</p>
        <ul className="help-list">
          <li><strong>The agent is persistent</strong> - A Bead backed by Git. Sessions come and go; agents stay.</li>
          <li><strong>The hook is persistent</strong> - Also a Bead backed by Git.</li>
          <li><strong>The molecule is persistent</strong> - A chain of Beads, also in Git.</li>
        </ul>
        <p style={{ marginTop: '1rem', opacity: 0.8 }}>
          So it doesn't matter if Claude Code crashes or runs out of context.
        </p>
      </div>

      <div className="help-card">
        <h3>Common Formulas</h3>
        <div className="help-shortcuts-table">
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <code>shiny</code>
            </div>
            <span>Design → Plan → Implement → Review → Test</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <code>release</code>
            </div>
            <span>Bump version → Run tests → Build → Tag → Publish</span>
          </div>
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <code>code-review</code>
            </div>
            <span>Multi-dimension parallel review</span>
          </div>
        </div>
      </div>
    </div>
  )
}

function BeadsSection() {
  return (
    <div className="help-section-content">
      <h2>Beads</h2>
      <p className="help-intro">Git-backed issue tracking that agents understand.</p>

      <div className="help-card help-card-accent">
        <h3>What Are Beads?</h3>
        <p>
          Beads are atomic units of work stored as JSON files in <code>.beads/</code>.
          They're Git-backed, so they survive across sessions and machines.
        </p>
        <p style={{ marginTop: '0.75rem' }}>
          Each bead has a short ID like <code>a7x</code>.
          States flow: <strong>open</strong> → <strong>in-progress</strong> → <strong>done</strong> → <strong>closed</strong>.
        </p>
      </div>

      <div className="help-card">
        <h3>Why Beads?</h3>
        <p>
          With molecules, the idea was: make 20 beads for the release steps, chain them together
          in the right order, and make the agent walk the chain one issue at a time.
        </p>
        <ul className="help-list">
          <li><strong>Activity feed</strong> - Automatically produced as agents claim and close issues</li>
          <li><strong>Crash recovery</strong> - Workflow survives interruptions</li>
          <li><strong>Audit trail</strong> - Everything committed to Git</li>
        </ul>
      </div>

      <div className="help-card">
        <h3>Bead Flow</h3>
        <ol className="help-steps">
          <li>
            <span className="help-step-number">1</span>
            <div className="help-step-content">
              <strong>You ask for something</strong>
              <p>"Hey Mayor, implement dark mode."</p>
            </div>
          </li>
          <li>
            <span className="help-step-number">2</span>
            <div className="help-step-content">
              <strong>Beads get created</strong>
              <p>Design bead, implement bead, test bead. Chained in order.</p>
            </div>
          </li>
          <li>
            <span className="help-step-number">3</span>
            <div className="help-step-content">
              <strong>Polecats claim beads</strong>
              <p>Work is slung. Polecats grab beads from their hook.</p>
            </div>
          </li>
          <li>
            <span className="help-step-number">4</span>
            <div className="help-step-content">
              <strong>Work completes</strong>
              <p>MRs submitted. Refinery merges. You approve.</p>
            </div>
          </li>
        </ol>
      </div>

      <div className="help-card">
        <h3>Beads Tab Views</h3>
        <div className="help-control-list">
          <div className="help-control-item">
            <span className="help-control-icon">&#x1F4CB;</span>
            <div>
              <strong>Kanban</strong>
              <p>Columns by state. See everything at a glance.</p>
            </div>
          </div>
          <div className="help-control-item">
            <span className="help-control-icon">&#x1F4E5;</span>
            <div>
              <strong>Triage</strong>
              <p>Inbox of new beads. Prioritize if needed.</p>
            </div>
          </div>
          <div className="help-control-item">
            <span className="help-control-icon">&#x1F4CA;</span>
            <div>
              <strong>Insights</strong>
              <p>Stats and metrics. How fast is work flowing?</p>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

function PhilosophySection() {
  return (
    <div className="help-section-content">
      <h2>Operating Philosophy</h2>
      <p className="help-intro">Principles that make this work.</p>

      <div className="help-card help-card-accent">
        <h3>You Ask, They Do</h3>
        <p>
          Your job is <strong>intent</strong>. Their job is <strong>execution</strong>.
        </p>
        <p style={{ marginTop: '0.75rem' }}>
          Don't tell agents how to do things. Tell them what you want done.
          They know how to write code, run tests, fix bugs.
          You provide direction, not instructions.
        </p>
      </div>

      <div className="help-card">
        <h3>Sessions Are Cattle</h3>
        <p>
          Tmux sessions come and go. Agents die and respawn. This is normal.
          Don't get attached. Don't mourn them.
        </p>
        <p style={{ marginTop: '0.75rem', opacity: 0.8 }}>
          Your work lives in Git. Beads are committed. Code is committed.
          If a session dies, the work survives.
        </p>
      </div>

      <div className="help-card">
        <h3>Start Simple</h3>
        <p>
          You can use Polecats without the Refinery, even without the Witness or Deacon.
          Just tell the Mayor to shut down the rig and sling work to polecats with the message
          that they should merge to main directly.
        </p>
        <p style={{ marginTop: '0.75rem', opacity: 0.8 }}>
          Or polecats can submit MRs and the Mayor merges them manually.
        </p>
      </div>

      <div className="help-card">
        <h3>When Refineries Shine</h3>
        <p>
          Refineries are most useful when you've done a LOT of up-front specification work
          and have huge piles of Beads to churn through with long convoys.
        </p>
        <p style={{ marginTop: '0.75rem', opacity: 0.8 }}>
          For smaller work, the Mayor alone is often enough.
        </p>
      </div>

      <div className="help-card">
        <h3>When to Intervene</h3>
        <ul className="help-list">
          <li><strong>Agent asks a question</strong> - Answer it</li>
          <li><strong>Polecat escalates</strong> - Work needs human attention</li>
          <li><strong>PR ready</strong> - Review and approve</li>
          <li><strong>Total chaos</strong> - Settings → Nuke All Sessions</li>
        </ul>
      </div>
    </div>
  )
}

function LinksSection() {
  return (
    <div className="help-section-content">
      <h2>Resources</h2>
      <p className="help-intro">Go deeper. The real documentation lives here.</p>

      <div className="help-card help-card-accent">
        <h3>Steve Yegge's Articles</h3>
        <ul className="help-list">
          <li>
            <a href="https://steve-yegge.medium.com/gas-town-emergency-user-manual-cf0e4556d74b" target="_blank" rel="noopener noreferrer">
              Gas Town Emergency User Manual
            </a>
            {' '}- The essential guide after 12 days and 100+ PRs
          </li>
          <li>
            <a href="https://steve-yegge.medium.com/welcome-to-gas-town-4f25ee16dd04" target="_blank" rel="noopener noreferrer">
              Welcome to Gas Town
            </a>
            {' '}- The original launch announcement
          </li>
          <li>
            <a href="https://steve-yegge.medium.com/the-future-of-coding-agents-e9451a84207c" target="_blank" rel="noopener noreferrer">
              The Future of Coding Agents
            </a>
            {' '}- Three days after launch, the vision
          </li>
        </ul>
      </div>

      <div className="help-card">
        <h3>Community Guides</h3>
        <ul className="help-list">
          <li>
            <a href="https://gist.github.com/Xexr/3a1439038e4ce34b5e9de020f6cbdc4b" target="_blank" rel="noopener noreferrer">
              Gastown User Manual (Xexr)
            </a>
            {' '}- Comprehensive community-maintained guide
          </li>
        </ul>
      </div>

      <div className="help-card">
        <h3>Source Code</h3>
        <ul className="help-list">
          <li>
            <a href="https://github.com/steveyegge/gastown" target="_blank" rel="noopener noreferrer">
              Gastown on GitHub
            </a>
            {' '}- The multi-agent orchestrator
          </li>
          <li>
            <a href="https://github.com/Perttulands/CHROTE" target="_blank" rel="noopener noreferrer">
              CHROTE on GitHub
            </a>
            {' '}- This dashboard
          </li>
          <li>
            <a href="https://github.com/anthropics/claude-code" target="_blank" rel="noopener noreferrer">
              Claude Code on GitHub
            </a>
            {' '}- The AI coding agent underneath
          </li>
        </ul>
      </div>

      <div className="help-card">
        <h3>Access</h3>
        <div className="help-shortcuts-table">
          <div className="help-shortcut-row">
            <div className="help-shortcut-keys">
              <code>http://chrote:8080</code>
            </div>
            <span>This dashboard (via Tailscale)</span>
          </div>
        </div>
        <p style={{ marginTop: '1rem', opacity: 0.8 }}>
          Works from any device on your Tailscale network.
        </p>
      </div>
    </div>
  )
}

export default ManualView
