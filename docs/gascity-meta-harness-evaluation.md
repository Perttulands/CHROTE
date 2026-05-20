# Gas City Meta-Harness Evaluation

This is a second-look assessment of Gas City through CHROTE's meta-harness desired state.

## User-Stated Lens

Perttu pointed to Gas City because it may be usable as-is, or as an SDK for orchestrators.

The relevant desired state is a CHROTE meta-harness that can coordinate interchangeable AI harnesses such as Claude Code, Codex, Pi, OpenCode, and others. Recipes/molecules are important because prebuilt bundles of Beads can represent repeatable workflows such as plan-review-synthesis or senate sessions.

## Freshness

Local checkout:

- path: `<workspace-root>/research/upstreams/gascity`
- local HEAD inspected and tested: `0e0b885 fix(packs): validate provider dolt state fallback (#2310)`
- fetched upstream `origin/main`: `37824e0 fix(cmd/gc/bd): surface bd silent on-disk fallback as loud error (#2080, #2079) (#2327)`

I did not reset the local checkout. Key upstream files were checked through `origin/main` where freshness mattered.

## Corrected Assessment

Gas City is closer to Perttu's desired meta-harness than my earlier framing allowed.

The README explicitly describes it as "composable orchestration infrastructure" and an orchestration-builder SDK for multi-agent systems. Its core primitives line up with CHROTE's desired direction:

- runtime providers for tmux, subprocess, exec, ACP, and Kubernetes
- Beads-backed work tracking
- mail as durable inter-agent messages
- formulas and molecules as reusable workflow bundles
- sling as work routing
- controller/supervisor reconciliation
- a typed HTTP/SSE control plane
- external messaging fabric with adapters, groups, transcripts, and participants

This means Gas City should be evaluated as a candidate sidecar/component for CHROTE's meta-harness, not only as inspiration.

## Direct Match: Review Quorum

Gas City already ships a core `mol-review-quorum` formula. This is highly relevant to Perttu's example workflow.

The formula:

- fans out two read-only reviewer lanes
- takes lane IDs, providers, models, and dispatch targets as variables
- routes a synthesis step after both reviewer lanes finish
- requires durable structured output from reviewer lanes
- preserves lane provenance in synthesis
- has retry behavior for reviewer lanes
- treats read-only enforcement as a mutation-baseline delta

This maps directly onto:

- one agent makes or owns the plan
- two other agents review from different perspectives or products
- a synthesis/chair role produces the combined result

The important nuance: this is a workflow scaffold, not a complete magic agent society. It gives us a durable Beads-backed graph and dispatch contract that CHROTE could drive or observe.

## Most Relevant Gas City Primitives

### Sessions

Gas City normalizes live agent processes behind a runtime `Provider` interface. The provider contract includes start, stop, attach, interrupt, nudge, peek, list-running, metadata, last activity, clear scrollback, copy-to-session, send-keys, live config, and capabilities.

This is directly relevant to CHROTE because CHROTE currently has tmux sessions but not a harness-neutral agent runtime contract.

### Exec Session Provider

The exec session provider is especially relevant. It delegates session operations to a script:

- `start`
- `stop`
- `interrupt`
- `is-running`
- `attach`
- `process-alive`
- `nudge`
- `set-meta`
- `get-meta`
- `remove-meta`
- `peek`
- `list-running`
- `get-last-activity`

That is a clean adapter point for CHROTE. It means CHROTE-specific harnesses or existing tmux sessions could be presented to Gas City without patching Gas City's Go code first.

### Mail

Gas City mail is not just terminal prompting. The default `beadmail` backend stores messages as Beads with type `message`. Messages have sender, recipient, subject, body, read/unread state, archive/delete, thread ID, reply-to, priority, CC, and rig metadata.

This is a strong fit for durable team communication because sessions can come and go while messages remain in the store.

### Sling

Sling is the routing mechanism. It routes a Bead or formula/wisp to an agent or pool. It can create an auto-convoy and optionally nudge the target.

This maps to CHROTE's need to assign work to interchangeable harness participants without hardcoding one product.

### Formulas and Molecules

Formulas are reusable workflow definitions. Molecules materialize every step as its own Bead, while wisps are lighter ephemeral molecule roots.

This is the Gas City primitive most aligned with CHROTE recipes:

- plan plus two reviewers
- senate session
- red-team review
- implementation plus review plus synthesis
- periodic patrols or status checks

### External Messaging Fabric

Gas City has an `extmsg` package for provider-neutral external conversation identity, bindings, delivery contexts, groups, participants, transcripts, and transport adapters.

This is notable because it resembles the generalization we want for Pi-style teams and outside chat surfaces. The adapter registry is currently in-memory and adapters must re-register, so it is not itself the durable source of truth, but the surrounding services are Beads-backed.

### Supervisor API

Gas City exposes a typed HTTP/SSE control plane. The docs say the CLI surface is not hidden; third-party clients can use the OpenAPI-described API.

This creates a plausible integration path where CHROTE stays the browser cockpit and uses Gas City's supervisor API for orchestration state, sessions, mail, formulas, convoys, and events.

## Current Host Fit

Available in host workspace:

- Go: `/usr/local/go/bin/go`, version `go1.26.2`
- tmux: available
- jq: available
- bd: version `1.0.3`

Not currently available on `PATH`:

- `gc`
- `dolt`

Gas City can run with a file-backed Beads store for a first spike, but production `bd`-backed behavior expects Dolt `1.86.2` or newer.

## Verification Performed

Targeted tests passed against the local checkout at `0e0b885`:

```text
go test ./internal/formula ./internal/mail/... ./internal/extmsg
go test ./internal/runtime ./internal/runtime/exec ./internal/runtime/subprocess ./internal/runtime/tmux
go run ./cmd/gc version
```

Results:

- formula, mail, beadmail, mail exec, and extmsg tests passed
- runtime, exec runtime, subprocess runtime, and tmux runtime tests passed
- `go run ./cmd/gc version` built and printed `dev`

These tests do not prove a full CHROTE integration. They do prove the relevant internal primitives compile and pass their local tests in this environment.

## Integration Options

### Option A: Borrow Concepts Only

CHROTE reimplements the relevant pieces:

- agent registry
- session provider interface
- Beads mailbox
- recipe/molecule model
- orchestration dashboard

Upside: maximum control, less dependency surface.

Downside: slow and likely duplicates exactly the hard parts Gas City already solved.

### Option B: Gas City As Sidecar Orchestration Runtime

CHROTE runs Gas City as a sidecar/controller and presents its state in CHROTE.

CHROTE would use Gas City's CLI/API for:

- sessions
- mail
- formulas
- molecules
- convoys
- events
- dashboard links

Upside: fastest path to real meta-harness behavior.

Downside: requires fitting Gas City's city/rig/config model into the existing `<workspace-root>` CHROTE workspace without letting it take over unrelated state.

### Option C: Gas City As Adapter SDK

CHROTE defines its own top-level product shape, but uses Gas City provider protocols and selected packages/contracts where possible:

- exec session provider protocol
- Beads-backed mail semantics
- formula/molecule schema
- review quorum formula structure
- external messaging model

Upside: lets CHROTE remain the cockpit while adopting proven seams.

Downside: may be awkward because many Go packages are internal and not exported as a stable public library.

## Current Recommendation

Treat Gas City as a first-class candidate component for the meta-harness.

The contained sidecar has now been set up at `<workspace-root>/gascity`.
See `docs/gascity-sidecar-spike-results.md` for the verification record.

Current recommendation after the spike: use Gas City as a sidecar
orchestration runtime for the next CHROTE slice, with CHROTE remaining the
browser cockpit. CHROTE should query the Gas City supervisor API first, then add
real harness adapters only after the read-only observer/control surface is
stable.

## Risks To Keep Visible

- Gas City has its own worldview: city root, rigs, packs, controller, supervisor, providers, and `.gc` runtime state.
- The production Beads path expects Dolt, which is not currently installed in host workspace.
- Some Go surfaces are internal packages rather than public SDK packages.
- The graph.v2 formula path is opt-in and depends on modern `bd --graph` behavior.
- The adapter registry for external messaging is in-memory; adapters need reconnect behavior.
- CHROTE already has a cockpit and tmux layer, so we must avoid creating two competing control planes.

## CHROTE Direction

CHROTE should remain the browser cockpit for the host-owned workspace.

Gas City may become:

- the orchestration sidecar behind the cockpit
- the source of recipe/molecule execution
- the session/mail/convoy/event layer that CHROTE visualizes
- or the reference model for a lighter CHROTE-native implementation

The next decision should be based on a local sidecar spike, not abstract judgment.
