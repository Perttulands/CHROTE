# Gas Town Formulas: Operator Guide

Formulas are TOML-based workflow templates that define multi-step processes in Gas Town. They coordinate work across AI agents, enabling complex tasks like builds, deployments, code reviews, and releases.

## The Formula Lifecycle

```
Formula (TOML source)
    │
    ├── bd cook <formula>
    ▼
Protomolecule (frozen template)
    │
    ├── bd mol pour <proto>  →  Molecule (persistent, synced to git)
    └── bd mol wisp <proto>  →  Wisp (ephemeral, for patrol cycles)
    │
    ▼
Step Beads Created (one per workflow step)
    │
    ▼
Agent Executes (bd ready → work → bd close)
    │
    ▼
Molecule Completes
    │
    ├── bd mol squash  →  Digest (permanent record)
    └── bd mol burn    →  (deleted, no trace)
```

## Formula Types

### 1. Workflow (Sequential + Dependencies)

Traditional step-by-step execution with explicit dependencies.

```toml
formula = "shiny"
type = "workflow"
version = 1

[[steps]]
id = "design"
title = "Design {{feature}}"

[[steps]]
id = "implement"
needs = ["design"]
title = "Implement {{feature}}"

[[steps]]
id = "test"
needs = ["implement"]
title = "Test {{feature}}"
```

**Use case**: Build pipelines, release workflows, sequential tasks.

### 2. Convoy (Parallel Analysis)

Spawn multiple parallel workers, each analyzing a different dimension, then synthesize.

```toml
formula = "code-review"
type = "convoy"

[[legs]]
id = "correctness"
focus = "Logic errors and edge cases"

[[legs]]
id = "security"
focus = "Security vulnerabilities"

[[legs]]
id = "performance"
focus = "Performance bottlenecks"

[synthesis]
depends_on = ["correctness", "security", "performance"]
```

**Use case**: Code reviews, design reviews, multi-dimensional analysis.

### 3. Parallel Containers (Nested Parallelism)

Workflow steps that contain parallel children.

```toml
[[steps]]
id = "start-services"
type = "parallel"

[[steps.children]]
id = "start-api"
title = "Start API server"

[[steps.children]]
id = "start-worker"
title = "Start background worker"
```

**Use case**: Parallel service startup, independent subtasks.

## TOML Structure

### Required Fields

```toml
formula = "my-workflow"       # Must match filename prefix
type = "workflow"             # workflow | convoy | expansion
version = 1                   # Increment on breaking changes

description = """
Explain what this formula does and when to use it.
"""
```

### Variables

```toml
[vars]
[vars.issue]
description = "The issue ID to work on"
required = true

[vars.region]
description = "Deployment region"
default = "us-east-1"
```

Usage in steps: `{{issue}}`, `{{region}}`

### Step Definition

```toml
[[steps]]
id = "run-tests"              # Unique identifier
title = "Run test suite"      # Human-readable title
needs = ["compile"]           # Dependencies (array of step IDs)
description = """
Run the full test suite:

```bash
go test ./...
```

**If tests PASS:** Continue to next step.
**If tests FAIL:** Fix issues and re-run.
"""
```

### Parallel Container

```toml
[[steps]]
id = "parallel-tasks"
type = "parallel"

[[steps.children]]
id = "task-a"
title = "Task A"

[[steps.children]]
id = "task-b"
title = "Task B"
```

## Three-Tier Resolution

Formulas are resolved in priority order:

| Tier | Location | Use Case |
|------|----------|----------|
| 1. Project | `<project>/.beads/formulas/` | Project-specific workflows |
| 2. Town | `~/gt/.beads/formulas/` | User customizations |
| 3. System | Embedded in `gt` binary | Factory defaults |

First match wins. Override by placing a formula at a higher-priority tier.

```bash
# See which tier a formula comes from
bd formula show mol-polecat-work --resolve
```

## Operator Commands

### Working with Formulas

```bash
bd formula list              # List all available formulas
bd formula show <name>       # View formula details
bd cook <formula>            # Cook formula → protomolecule
```

### Creating Molecules

```bash
# Persistent molecule (synced to git)
bd mol pour shiny --var feature="dark mode"

# Ephemeral wisp (for patrol loops)
bd mol wisp mol-witness-patrol
```

### Dispatching to Agents

```bash
# Sling a formula to a polecat
gt sling polecat1 --formula mol-polecat-work --var issue=gt-123

# Sling to a rig (auto-spawns polecat)
gt sling gastown --formula code-review --var pr=456

# Dry run (preview what would happen)
gt sling polecat1 --formula mol-polecat-work --dry-run
```

### Agent-Side Commands

```bash
gt hook                      # What's on my hook?
gt mol current               # What step should I work on?
bd ready                     # Get next ready step
bd close <step-id>           # Mark step complete
bd close <step-id> --continue  # Complete + auto-advance
```

### Cleanup

```bash
bd mol squash <mol-id>       # Completed → Digest (permanent record)
bd mol burn <wisp-id>        # Delete wisp (no trace)
```

## Available Formulas

### Core Agent Patrol

| Formula | Purpose |
|---------|---------|
| `mol-deacon-patrol` | Mayor's daemon - evaluates gates, dispatches work |
| `mol-witness-patrol` | Per-rig worker monitor |
| `mol-refinery-patrol` | Merge queue processor |

### Polecat Work

| Formula | Purpose |
|---------|---------|
| `mol-polecat-work` | Full 8-step lifecycle: context → branch → implement → test → submit |
| `mol-polecat-code-review` | Code review workflow |
| `mol-polecat-conflict-resolve` | Merge conflict resolution |

### Infrastructure

| Formula | Purpose |
|---------|---------|
| `mol-gastown-boot` | Bootstrap Gas Town (daemons, witnesses, refineries) |
| `mol-town-shutdown` | Graceful shutdown |
| `mol-session-gc` | Garbage collect orphaned sessions |

### General Purpose

| Formula | Purpose |
|---------|---------|
| `shiny` | Design → Implement → Review → Test → Submit |
| `code-review` | 7-dimension parallel code review |
| `design` | 6-dimension parallel design review |
| `beads-release` | Release workflow for beads |
| `gastown-release` | Release workflow for Gas Town |

## Creating Custom Formulas

### 1. Create the File

```bash
# Project-level (committed to repo)
vim <project>/.beads/formulas/my-workflow.formula.toml

# Town-level (personal customization)
vim ~/gt/.beads/formulas/my-workflow.formula.toml
```

### 2. Define the Formula

```toml
description = """
My custom workflow for doing X.

**When to use**: Describe the use case.
**Pre-requisites**: List any requirements.
"""

formula = "my-workflow"
type = "workflow"
version = 1

[vars]
[vars.target]
description = "The target to operate on"
required = true

[[steps]]
id = "preflight"
title = "Preflight checks"
description = """
Verify preconditions before starting.

```bash
git status
```

**If dirty**: Commit or stash changes first.
"""

[[steps]]
id = "main-work"
needs = ["preflight"]
title = "Do the work on {{target}}"
description = """
Perform the main task.

**Exit criteria:**
- Work is complete
- Changes are committed
"""

[[steps]]
id = "verify"
needs = ["main-work"]
title = "Verify results"
description = """
Confirm the work was successful.
"""
```

### 3. Test the Formula

```bash
# Cook it (validates TOML, checks for cycles)
bd cook my-workflow

# Create a test molecule
bd mol pour my-workflow --var target=test-123

# Walk through manually
bd show <mol-id>
bd ready
```

### 4. Use in Production

```bash
gt sling polecat1 --formula my-workflow --var target=real-thing
```

## Best Practices

1. **Start simple, extend incrementally** - Don't try to cover every case upfront

2. **Embed decision trees in descriptions** - Tell agents what to do when things go wrong

3. **Use variables for user input** - Don't hardcode values that change per-invocation

4. **Document dependencies** - Explain *why* a step needs its predecessors

5. **Leverage parallelism** - Use parallel containers or convoy legs for independent work

6. **Version your formulas** - Increment version on breaking changes

7. **Test before widespread use** - Cook and walk through manually first

8. **Write for agents** - Descriptions should be clear instructions agents can follow

## Troubleshooting

### "Formula not found"

```bash
bd formula list              # Check available formulas
ls ~/.beads/formulas/        # Check town-level
ls <project>/.beads/formulas/  # Check project-level
```

### "Cycle detected"

Check your `needs` dependencies for circular references:

```toml
# BAD: Creates a cycle
[[steps]]
id = "a"
needs = ["b"]

[[steps]]
id = "b"
needs = ["a"]
```

### "Variable not set"

Provide all required variables at sling time:

```bash
gt sling polecat1 --formula mol-polecat-work --var issue=gt-123
```

Check required vars:

```bash
bd formula show mol-polecat-work  # Look for [vars] with required = true
```
