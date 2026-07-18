# Captures the RUN MODEL (03-formations.js header + runMission/flowFrom/followBranch/evalGate/
# runFormation/inputsReady). The prototype mocks execution with setTimeout; this spec defines the
# real engine behavior: cascade along wires, JOIN readiness, gate routing, judge execution,
# verification, fail-loud limits, and the append-only ledger that makes runs explainable.

Feature: Run a mission — cascade work along the wires with gates, joins, and judges
  As the engine driving a formation graph
  I need to dispatch work to live agents, gather results, and route by verdict
  So that a mission produces real artifacts and the run is fully auditable

  Background:
    Given a board "session-search" with mission -> frame -> research -> gate -> ship
    And the formations are staffed with live agent sessions
    And each run writes an append-only NDJSON ledger that status is projected from
    And ledger events use the envelope defined in "spec/contracts.md"
    And one CHROTE server coordinator holds the current workspace lease and writer fence
    And it validated a supported immutable workspace bootstrap, mutable authority-schema high-water mark, and complete hash-matched current admission-policy chain to revision 1 before fence acquisition
    And no UI or Archon process has peer runtime-writer authority
    And every "run_failed" exact-names one prior unique "run_failure_reconciliation_started" through "failureReconciliationSeq"
    And that start projects non-final "failing", freezes the failure header and complete open-resource snapshots, and permits reconciliation only
    And the final failure byte-matches that header and exactly disposes those snapshots

  # ── The cascade ─────────────────────────────────────────────────────────────

  @ui @cli
  Scenario: Starting a mission cascades the whole reachable chain
    When I submit one stable start command id through "archon mission run session-search" (or press the mission's start)
    Then the engine resolves the sub-graph reachable from the mission
    And one "mission-objective-utf8-v1" "text/markdown" work payload is emitted from the mission's fixed "out" port
    And "run_started.rootInputProjection" is the exact closed "authored_config" shape with "sourceKind=mission_objective", "encoding=mission-objective-utf8-v1", "mediaType=text/markdown", exact SHA-256, and canonical "text"
    And Mission "node_output.outputs[out]" uses the classified root-derived projection whose source role, encoding, media, hash, and payload text exact-match that root input
    And both projections exact-match the Mission's private "authoredConfigManifest" entry
    And unchanged edge or Gate-pass deliveries preserve that classified projection, while a generic unclassified exact copy is rejected
    And successful Mission, Formation, and Tool outputs flow on their workflow edges
    And Gates route only the verdict-selected pass or fail frontier
    And judge-channel edges never carry PortPayload
    And the private command journal fsyncs the complete exact "run-command-jcs-v1" request plus hash and admitted/state fences before its effect
    And the ledger records "run_started" with run id, board id, board rev, mission id, actor, seq 1, authority schema, workspace/run authority ids, admission command id/hash, admission sequence/policy revision/hash, current writer fence, and exact graph/private-binding/projection hashes
    And the durable command receipt returns that same run id after "run_started" fsync without waiting for graph completion
    And "run_started" alone projects "queued" while unique fenced "run_activated" binds its activation-policy revision/hash and projects "running" before graph/dispatch events
    And admitted work survives client disconnect
    And output-producing workflow nodes record "node_started"/"node_output"
    And Gates and judge attempts record their dedicated evaluation/result events

  @api @cli @file
  Scenario Outline: Runtime command retries return one durable receipt
    Given canonical runtime command "<command>" has command id "cmd_00000000000000000000000000" and request hash H
    When the coordinator accepts that command
    Then it fsyncs the private command record before "<effect>"
    And "<effect>" exact-matches command id "cmd_00000000000000000000000000" and hash H
    And the applied receipt's outcome fence exact-matches that effect while a rejection names its decision fence
    And retrying "cmd_00000000000000000000000000" with hash H returns the original applied or rejected receipt without another effect
    And retrying "cmd_00000000000000000000000000" with another hash fails "command_id_conflict" without ledger mutation or dispatch
    Examples:
      | command | effect                     |
      | start   | run_started                |
      | resume  | run_resumed                |
      | cancel  | run_cancel_requested       |
      | verdict | human_verdict_recorded     |

  @api @file @security
  Scenario: Runtime command canonicalization has one value domain
    Given command ids match canonical uppercase ULID grammar "^cmd_[0-7][0-9A-HJKMNP-TV-Z]{25}$"
    And authority schema is integer 2, run-root kind is mission or formation, resume mode is reattach or retry-failed-producer, and verdict is pass or fail
    And revisions and sequences are positive JSON safe integers
    And each resolved limit is a positive 32-bit JSON integer while redact is boolean
    And an absent ETag is empty while a present ETag is 64 lowercase hexadecimal characters
    When a request has an unknown key, wrong JSON type, invalid enum, out-of-range value, or unsafe command id
    Then it is rejected before journaling or command-path construction

  @api @file
  Scenario: Workspace admission is a bounded durable FIFO
    Given one current immutable hash-verified "workspace-admission-policy-jcs-v1" generation sets JSON-integer "maxActiveRuns" in 1..2147483647 and "maxQueuedRuns" in 0..2147483647
    And every generation has a JSON-safe positive revision and the exact prior-generation hash, while revision 1 uses an empty prior hash
    And schema-2 admission has no implicit policy default while closed revision-1 "state=disabled" rejects starts and pauses activation
    And activated non-final runs retain active capacity through blocked, human-waiting, canceling, and failing states
    When valid start commands A and B enter the one workspace admission critical section
    Then active count is non-final ledgers with "run_activated"
    And queued count is ledgers whose latest projected status is exactly "queued"
    And an unactivated "canceling" or "failing" run is not queued and can never receive "run_activated"
    And each admitted "workspaceAdmissionSeq" counter is a JSON-safe positive integer advanced and fsynced before its event, allowing gaps but never reuse
    And "run_started" exact-matches the persisted admission-policy revision/hash used
    And every immediate or dequeued "run_activated" exact-matches the configured activation-policy revision/hash used
    And their sequences fix FIFO order across restart
    And queue wait begins at admission and counts against each run's wall-clock limit
    And cancellation, cleanup, and recovery reconciliation precede fresh dispatch
    When command C arrives while the queue is full
    Then its journal records stable rejection "run_queue_full" before any run directory or "run_started"
    And its terminal start record binds the exact decision-policy revision/hash
    And replaying C returns that same rejection while a later attempt requires a new command id

  @api @file @recovery
  Scenario: Admission policy generations remain historically resolvable
    Given the current WorkspaceAuthority references immutable policy generation P and its SHA-256
    When the owner publishes generation P+1 with an exact expected P revision/hash
    Then it stages and fsyncs the exact chained immutable generation before atomic no-replace install and current revision/hash ref replacement
    And every generation named by a terminal start, "run_started", or "run_activated" remains retained and hash-verifiable
    When a crash leaves P+1 durable before the current ref changes
    Then P+1 is non-authorizing, an exact retry may complete the ref change, and conflicting bytes fail loud
    And a stale expected current ref fails before creating another generation
    When the current ref already names the exact requested P+1 whose prior hash is P
    Then a lost-response retry returns the original success without another generation

  @api @file
  Scenario Outline: Admission policy limits reject fractional JSON numbers
    Given a proposed configured policy sets "<field>" to JSON number "<value>"
    When policy canonicalization validates its closed integer domain
    Then publication fails before immutable staging or workspace-ref mutation
    Examples:
      | field           | value |
      | maxActiveRuns   | 1.5   |
      | maxQueuedRuns   | 0.5   |

  @api @file
  Scenario: Policy changes preserve activation progress and FIFO
    Given older queued runs Q1 and Q2 precede fresh start C by workspace admission sequence
    And a configured generation lowers "maxQueuedRuns" below the current queued count while active capacity becomes available
    When admission and activation reconcile under the workspace lock
    Then "maxActiveRuns" alone governs activation and Q1 activates before Q2 or C
    And the reduced queue limit never blocks dequeue but blocks fresh queued admission while queued count is at or above that limit
    When "maxActiveRuns" is lowered below active count
    Then existing active runs continue and only new activation pauses until active count is below the active limit
    And a fresh start never bypasses an older queued run when capacity is released
    When a later configured generation raises "maxActiveRuns" above active count
    Then existing queued runs activate oldest-first up to the new capacity before any fresh start activates

  @api @file
  Scenario: Disabled admission pauses without canceling admitted work
    Given one active run and one queued run under a configured policy
    When the owner publishes the next immutable "state=disabled" generation
    Then the active run continues, the queued run remains queued with wall clock advancing, and no queued activation occurs
    And a fresh start records "admission_disabled" with the exact decision-policy revision/hash before any run directory or event
    When a later configured generation restores capacity
    Then the oldest non-expired queued run activates first under that generation

  @api @file
  Scenario: Idle capacity works with a zero queue limit under concurrent starts
    Given "maxActiveRuns=1" and "maxQueuedRuns=0" with no active run
    When two start commands concurrently enter admission
    Then one linearizes first and fsyncs "run_started" then "run_activated"
    And the other records "run_queue_full" without a run directory or event
    And restart derives one active run and zero queued runs from the same ledgers

  @api @security
  Scenario: Concurrent requests for one command id create one record and effect
    Given two requests carry the same valid command id and canonical payload hash
    When they race through the coordinator
    Then create-if-absent journal admission under the authority lock has one durable linearization point
    And both receive the same receipt with at most one semantic effect

  @api @file @security
  Scenario: One command file has closed request and terminal receipt variants
    Given one valid canonical command payload contains the sole actor, kind, workspace, run, reason, mode or verdict, and precondition authority
    When "commands/<commandId>.json" is pending
    Then it has no run id, effect sequence, rejection code, outcome fence, or decision-policy ref
    When that same record becomes applied
    Then it has exactly run id, effect sequence, immutable outcome fence, and closed decision-policy ref or null, and its API receipt derives from that record
    When it instead becomes rejected
    Then it has exactly rejection code, immutable outcome fence, and closed decision-policy ref or null, and its API receipt derives from that record
    And only terminal start records carry a non-null ref exact-matching the policy generation used
    And no second actor, receipt file, or contradictory state-field combination is valid
    And every duplicated command-effect event field exact-matches the stored hash-bound payload

  @file
  Scenario: Mission objective media incompatibility fails before admission
    Given the Mission objective is the fixed "text/markdown" root payload
    And a first destination work input does not accept "text/markdown"
    When run preflight validates the selected root
    Then it reports "mission_objective_media_incompatible"
    And no "run_started" event is appended

  @file @security
  Scenario: Canonical run authority is writer-private and hash verified
    Given the workspace is a configured generic Files read/write root
    When a run is durably admitted
    Then its opaque workspace authority exact-matches "workspace-root-identity-v1" for the configured/opened root
    And a path alias, changed symlink target, or same-named workspace cannot select or replace that authority
    And its canonical ledger, graph snapshot, private bindings, and private refs live under the CHROTE data root outside every Files root
    And the graph snapshot embeds a stable "authoredConfigManifest" covered by "graphSnapshotSha256"
    And each manifest entry classifies and hashes exactly one Mission objective, whole Formation brief, or Gate criterion by source kind and node id
    And the generic Files API cannot list, read, write, rename, or delete any canonical authority path
    And run detail exposes only safe hash-linked binding projections with opaque target ids
    When a client creates same-named workspace snapshots or later replaces an inspection export
    Then every execution and recovery read still exact-verifies the host-private hashes
    And altered or missing admitted authority records an error then "run_failed" with code "run_authority_integrity_failed"
    And it sends or evaluates nothing

  @file @security
  Scenario: Concurrent workspace registration cannot split authority
    Given no workspace authority is registered for one opened directory
    When two coordinators concurrently register different configured aliases for that directory
    Then the stable parent registry lock serializes the race before either authority-id lock is selected
    And the private registry enforces uniqueness for cleaned configured spelling and opened device/inode identity
    And exactly one fsynced authority mapping may exist
    And the alias/conflict requires explicit migration and cannot execute under a second owner lock

  @file @security
  Scenario: Workspace identity hashes uint64 device and inode without rounding
    Given the race-safe opened root has device or inode above the IEEE-754 exact integer range
    When "workspace-root-identity-v1" is canonicalized
    Then device and inode are unsigned base-10 JSON strings with no sign or leading zero
    And the same opened handle supplies resolved path, device, and inode

  @file
  Scenario: Registry ordering has one exact comparator
    Given registry entries include device strings "2" and "10" and valid non-ASCII configured paths
    When the closed registry is canonicalized
    Then device and inode compare as decoded unsigned integers so "2" sorts before "10"
    And configured paths with equal numeric identities compare by their valid UTF-8 bytes

  @file @security
  Scenario: Fence allocation and mutable authority publication are crash safe
    Given a supported coordinator holds the workspace owner lock
    When it acquires or takes over ownership
    Then it advances and fsyncs nextWriterFence before publishing owner.private.json
    And a crash may leave a gap but restart never reuses that fence
    And mutable registry generations publish under the parent registry lock
    And mutable workspace, owner, and command records have increasing record revisions and publish under the owner lock by generation-checked temp fsync, atomic rename, and parent fsync
    And immutable authority publishes only by same-directory staging fsync plus atomic no-replace install and parent fsync, never by writing its canonical path in place
    And every revision, fence, and admission sequence is in "1..9007199254740991" and exhaustion fails before mutation
    And a torn or conflicting published generation authorizes no projection or runtime effect

  @api @cli @security
  Scenario: Unknown event fields never bypass sanitized projection
    Given a private event contains an unknown key with unique fixture value "PRIVATE-X"
    When run detail, bounded events, SSE, CLI, and UI projections are produced
    Then each event type emits only its registered safe-field allowlist
    And no public projection contains the unknown key or "PRIVATE-X"
    And a Redact=true writer rejects an unregistered or unclassified extension before append
    And an extension that can change admission, identity, dispatch, result acceptance, routing, cleanup, cancellation, or finality requires an authority-schema bump
    And matching schema numbers alone cannot enable semantic projection or runtime adoption
    And an unsupported reader allocates no fence and performs no adoption, cleanup, quarantine, dispatch, result acceptance, execution mutation, or finality
    And only a registered redaction-classified projection-only extension that cannot change public status, actions, bindings, artifacts, or execution may remain ignorable

  @file @security
  Scenario: Authority-schema upgrade advances the workspace high-water mark first
    Given a supported current owner holds the owner lock and current fence
    When it enables a newer authority schema
    Then it atomically advances and fsyncs "workspace.private.json.authoritySchema" before any new-schema command, run, event, or private record
    And that high-water mark never decreases
    And a crash after advancement is read-only to an older reader rather than silently downgraded
    And the new schema starts a new run while older ledgers retain their recorded semantics

  @file @security
  Scenario: Pending redacted bytes are never a generic Files surface
    Given a Redact=true run has a fsynced private cleanup obligation
    And raw capture or Tool input bytes exist during its cleanup window
    When a client lists or fetches through generic Files or File Peek
    Then the pending registry, locator, and raw bytes are absent and inaccessible
    And only a later registered sanitized artifact may enter File Peek

  @ui @file
  Scenario: A successful reachable chain records run_succeeded
    Given every reachable node finishes and no gate blocks
    When the final downstream node finishes
    Then the ledger records "run_succeeded" as the terminal event
    And its optional "summaryArtifactId" and every "outputArtifactIds[]" value name prior durable registrations
    And public run, event, SSE, CLI, and UI views hydrate those ids through their latest authorized artifact projections
    And no further node or gate events are appended for that run

  @ui
  Scenario: Starting a mission with no outgoing wire fails loud
    Given the mission has no connection from its output
    When I start it
    Then it reports "wire the mission to a step" and does not start

  @cli
  Scenario: A single node can be run in isolation for testing
    Given a Redact=false run and "research" has a non-empty brief and exactly one required "data" input accepting "work" and "application/json"
    When I run "archon formation run session-search research"
    Then "runRoot" is the isolated Formation "research"
    And only that formation is bound and run
    And exactly "{goal,beadId,files,links}" becomes RFC 8785 canonical UTF-8 JSON with missing values normalized and no trailing newline
    And those bytes become one "mediaType=application/json" synthetic "sourceKind=run_seed" input for that port
    And the seed records "seedEncoding=formation-brief-jcs-v1", stable id, media type, and exact byte SHA-256 without inventing an edge or producer
    And its payload projection is the classified root-derived exact/available variant over those same hashed bytes
    And "run_started.rootInputProjection" is the exact closed "authored_config" shape with "sourceKind=formation_brief", "encoding=formation-brief-jcs-v1", "mediaType=application/json", that SHA-256, and the same canonical JSON in "text"
    And the seed projection exact-matches that root input's source role, encoding, media, hash, and payload text; a generic unclassified exact copy is rejected
    And both exact-match the isolated Formation's private "authoredConfigManifest" entry
    And optional inputs and "retry_control" remain absent
    And downstream edges are not traversed
    And its output is finalized in the ledger

  @cli @file
  Scenario: An isolated Formation with an ambiguous required-input shape fails preflight
    Given "research" has zero, multiple, or non-work required data inputs
    When I run "archon formation run session-search research"
    Then preflight reports "isolated_formation_input_invalid"
    And no "run_started" event is appended

  @cli @file
  Scenario: An isolated Formation seed must match the sole input media set
    Given "research" has exactly one required "data" work input that does not accept "application/json"
    When I run "archon formation run session-search research"
    Then preflight reports "isolated_formation_seed_media_incompatible"
    And no "run_started" event is appended

  @file @security
  Scenario: Redaction exempts only classified author configuration, never composed prompts
    Given a Redact=true run has unique authored objective, Formation brief, and Gate criterion fixtures
    And a runtime input fixture would be interpolated into a slot prompt
    When run preflight writes canonical private authority and dispatch composes the prompt
    Then the exact authored fixtures appear only in typed "authored_config" fields with closed source role, canonical encoding, media, and SHA-256
    And the private graph manifest classifies each exact fixture field/node with the same encoding, media, and hash
    And human prompt and PASS/FAIL choices use only their closed fixed-system templates
    And the private snapshot may retain those configuration values after later board edits
    And "slot_dispatch" contains only "promptSha256" and no prompt bytes, ref, path, or authority id
    And the adapter receives the exact already-hashed in-memory prompt slice at most once
    And no durable run-owned surface contains the runtime fixture or composed prompt
    And recovery never reconstructs or resends that prompt from its hash

  @file @security
  Scenario Outline: Formation dispatch consumes verified artifact bytes, never a mutable path
    Given a "Redact=<redact>" ordinary or judge Formation input contains an available "SafeArtifactRef"
    And the source file becomes "<state>" after registration
    When the coordinator prepares the Formation prompt
    Then it uses one authorized-root-relative no-follow handle to validate regular identity, media, size, and SHA-256
    And it reads prompt bytes only from that same handle without reopening or passing the path as input authority
    And the mismatch is detected before "slot_dispatch" and no prompt is sent
    And it fails "<code>"
    Examples:
      | redact | state             | code                              |
      | false  | replaced          | formation_input_integrity_failed  |
      | false  | changed in place  | formation_input_integrity_failed  |
      | false  | symlink swapped   | formation_input_integrity_failed  |
      | true   | missing           | redacted_input_unavailable        |

  @file @security
  Scenario: A pre-admission authority orphan never becomes a run
    Given private snapshots and a pending raw-redaction target are fsynced
    But no valid seq-1 "run_started" exists
    When a replacement coordinator validates supported bootstrap then acquires and fsyncs a newer workspace fence
    And it validates the orphan's workspace, command, and historical origin fence before claiming cleanup under its current state fence
    Then no prompt, Tool process, Gate evaluation, or run event is produced
    And the raw target is sanitized or removed and its obligation is fsynced first
    And the orphan tree is idempotently deleted with a parent-directory fsync
    But an unsupported reader remains strictly read-only, while a supported current owner quarantines conflicting or unprovable identity as non-authorizing with no public bytes or replay handle
    And a stale prior owner performs no cleanup or quarantine

  # ── Dispatch to live sessions (cross-harness is just dispatch — D4) ──────────

  @cli
  Scenario: Every reachable declared slot must resolve before exact tmux dispatch
    Given run preflight produced one "SlotResolution" per declared slot in the selected run root
    And every resolution is runnable
    And run start wrote one immutable "RunSlotBinding" per slot
    When the engine dispatches a node's work to slot agent "codex"
    Then it uses that slot's "bindingId" and opaque "sessionTargetId" for the exact pane
    And private authority freezes persona-card path/hash, server/socket/session/window/pane ids, resolved cwd/root, and target fingerprint
    And the API exposes only the hash-linked safe binding projection
    And the private canonical target key is one exact tmux server/pane incarnation
    And independent resolutions of that incarnation return the same opaque target id
    And it does not re-resolve the persona, session stem, or same-named session
    And it verifies and hashes the exact prompt bytes before target acquisition
    And it fsyncs exact "attachment-audit-registration-v1" before atomically acquiring and fsyncing the host-wide exclusive target lease
    And the first "fence_transition" interaction-journal record uses the registration's "startOffset" and exact registration hash as its predecessor and fsyncs before occupancy
    And every audit evidence range exact-matches that registration, validates a non-empty contiguous chain through its terminal record hash, and can never return from "audit_lost" to "none"
    And the certified boundary drains its chained interaction journal and installs "target-dispatch-input-barrier-v1" over that prompt hash
    And it obtains a fresh certified "target-ready-proof-v1" bound to that dispatch barrier under the target critical section before "slot_dispatch"
    And while continuously holding the workspace authority and target critical sections it rechecks the current fence plus frozen card, harness/process, cwd/root, and pane fingerprint then performs the bounded send
    And takeover cannot allocate a new fence between that check and send
    And it records binding, target, target-lease, fingerprint, exact dispatch barrier/hash, target-ready proof/hash, exact "tmux-pane-history-baseline-v1" token, and baseline hash in writer-private "slot_dispatch" before sending
    And only the exact hashed prompt may consume the one-send fence permit
    And the baseline binds the fingerprint, capture epoch, byte offset, and frozen terminal grid without pane bytes
    And sanitized projection exposes only baseline encoding, hash, and validation state
    And a Claude Code slot and a Codex slot are dispatched the same way (no special path)
    And a multi-slot formation may expose several exact session targets

  @cli @security
  Scenario: Stock owner-accessible tmux is unavailable without an enforceable input boundary
    Given raw attach, select, resize, send-keys, paste, or control clients can bypass the Formations adapter
    When preflight evaluates that stock target
    Then it reports "session_target_attachment_audit_unavailable"
    And no binding, occupancy, dispatch, prompt, or Peek capability is created

  @cli @security @recovery
  Scenario Outline: A mutation at the dispatch barrier cannot race prompt send
    Given the certified journal is drained and the one-send dispatch fence is installed
    When an unregistered "<mutation>" targets the pane before prompt permit consumption
    Then the mutation is synchronously rejected or durably latches the dispatch before send
    And no stale ready proof, baseline, slot_dispatch, or prompt is accepted
    Examples:
      | mutation        |
      | attach/select   |
      | resize/reflow   |
      | history clear   |
      | pane lifecycle/topology mutation |
      | raw input       |

  @cli @security
  Scenario Outline: Same-pane authority drift sends no prompt
    Given run start froze one exact slot binding and fingerprint
    And before dispatch the same pane has a changed "<changed>"
    When the engine acquires the target lease and performs its immediate pre-send check
    Then "slot_binding_observed" records the frozen binding stale
    And no "slot_dispatch" or prompt is produced
    And the engine does not re-resolve the persona or accept the changed pane as the old binding
    Examples:
      | changed                       |
      | persona card content          |
      | foreground harness process    |
      | resolved cwd or workspace root|

  @ui @file
  Scenario: Runtime binding health is durable inspection evidence, not rebinding
    Given a run froze one exact slot binding and target
    When the reconciler observes that target as stale
    Then it appends "slot_binding_observed" with binding id, target id, reason, time, and related seq
    And projection shows stale without consulting tmux or changing the frozen target
    And the observation cannot authorize a replacement dispatch

  @cli
  Scenario: Completion is detected by an exactly attributed sentinel
    When an agent finishes and emits "<<<CHROTE-DONE run-id=... dispatch-id=... target-lease-id=... status=ok artifact=...>>>"
    Then the engine records "slot_result" only after capture starts at the exact dispatch baseline, input is drained, no steering generation is open, capability revocation is durable, the sentinel is terminal, and the certified harness proves the same fingerprint returned to a closed turn
    And certified client/input audit continuously accounts for every mutation route since occupancy
    And the closure barrier drains that journal and rejects every later unregistered mutation through receipt fsync
    And the result repeats the exact run, dispatch, target-lease, binding, target, fingerprint, baseline dispatch/hash, capability-revocation sequence, latest closed steering generation, and turn-closure proof
    And the certified proof cannot be produced solely by user-writable pane bytes or an echoed sentinel
    And writer-private "slot_result" durably owns the exact audit proof and hash
    And only after that result is fsynced may occupancy be atomically replaced by a non-occupying "releaseKind=result_committed" receipt naming its result sequence, proof, exact audit proof, and "closure_barrier_held" releaseProof
    And the result is not consumable or routable before that receipt fsync
    And the receipt remains crash proof until execution-final fsync, then may be removed
    And a sentinel whose run, dispatch, or target-lease id does not match is ignored (prompt-injection safe)

  @cli @security @recovery
  Scenario Outline: Lost pane-history continuity cannot produce or release a result
    Given "slot_dispatch" durably records one exact pane-history baseline
    And before result capture the pane history boundary is "<change>"
    When the coordinator evaluates completion
    Then it records stable "capture_baseline_unavailable"
    And no "slot_result" or ordinary release receipt is appended
    And the target remains occupied, held, or quarantined for exact reconciliation
    Examples:
      | change                            |
      | trimmed past the baseline         |
      | cleared or reset                  |
      | resized or reflowed               |
      | replaced                          |
      | ambiguous after adapter restart   |

  @ui @cli @security
  Scenario: Peek input cannot race or forge turn closure
    Given exact target occupancy and "slot_dispatch" are durable
    And "slot_peek_capability_issued" is fsynced before the run-bound Peek attaches to the active dispatch
    When the user sends the first input bytes
    Then "slot_steering_started" is fsynced first and result closure is disabled
    When the input channel is drained and "slot_steering_ended" closes that generation
    Then capability issuance stops and "slot_peek_capability_revoked" is fsynced
    And only fresh certified proof bound to that revocation and generation can close the turn
    And certified client-attachment audit accounts for every event since occupancy
    And "operatorInfluenced" is true exactly when latest steering generation is non-zero
    And a sentinel typed through Peek cannot close by itself
    And input accepted after a proof invalidates it before result, otherwise revoked input is rejected

  @cli @security @recovery
  Scenario: Transient foreign attachment cannot hide outside steering generations
    Given an exact target occupancy has continuous certified client monitoring
    When an unregistered client attaches, types, and detaches before result validation
    Or a raw tmux command/control route targets the pane outside the steering-generation gate
    Then "session_target_foreign_attachment" or "session_target_foreign_input" latches the dispatch non-authorizing
    And capability input is revoked and no foreign bytes become a steering generation
    And no "slot_result" or ordinary release is appended
    And only exact pane-incarnation-gone proof may clear the foreign-latched target

  @cli @security @recovery
  Scenario Outline: A mutation at the closure barrier cannot race result release
    Given exact result capture, capability revocation, and the closure barrier are durable
    When an unregistered "<mutation>" targets the pane before result_committed receipt fsync
    Then the mutation is synchronously rejected or the result remains unconsumable in result-closed quarantine
    And no stale closure_barrier_held releaseProof is accepted
    And a lost barrier can release only after exact post-result pane-incarnation-gone proof
    Examples:
      | mutation        |
      | attach/select   |
      | resize/reflow   |
      | history clear   |
      | pane lifecycle/topology mutation |
      | raw input       |

  @cli @security
  Scenario: A matching sentinel followed by old-turn output cannot release the pane
    Given capture sees an exact completion sentinel for the active dispatch
    But later bytes from that same turn follow it and no certified harness-ready boundary exists
    When the coordinator evaluates completion
    Then no "slot_result" or "result_committed" receipt is appended
    And occupancy remains active or becomes a non-authorizing terminal hold at finality
    And another run cannot acquire the pane

  @cli @security
  Scenario: Final target release accepts only closed certified proof
    Given Peek capability revocation is durable for the exact active dispatch
    And "slot_reconciliation_interrupt" is fsynced against the cancel/failure authority before one Ctrl-C attempt
    And a crash after interrupt intent never resends and projects "send_uncertain" when outcome is absent
    When no exact cancel acknowledgement and harness-ready boundary is observed
    Then the target remains a non-authorizing terminal hold and is not reusable
    And its Peek capability is revoked and the terminal hold permits no run-bound input
    When an exact dispatch/lease/fingerprint cancel acknowledgement bound to that interrupt request, certified ready boundary, capability revocation, and continuous client/input audit barrier are durable
    Then occupancy may become a "final_quiescent" receipt with that proof
    But if the old pane incarnation dies, private pane-gone proof may release only that old key
    And a replacement pane has a new key/fingerprint and is never interrupted or released as the old target

  @cli @security
  Scenario: One exact pane cannot staff two selected slots in one run
    Given two selected slots independently resolve to the same private target key and opaque "sessionTargetId"
    When run preflight validates the bindings
    Then it rejects "duplicate_session_target" before "run_started"
    And no target lease, dispatch, or prompt is created

  @cli @security
  Scenario: Concurrent runs cannot interleave work in one exact pane
    Given run A holds the host-wide target lease for an exact pane
    When run B attempts to dispatch to that same "sessionTargetId"
    Then run B acquires no lease and sends no prompt
    And preflight returns the specific stable unavailable reason "session_target_leased" before start
    And after start "slot_binding_observed" preserves that reason before node error "session_target_busy" selects terminal failure
    And A's result or proven final quiescence must release the lease before a later run can use the pane
    And an unproven canceled or failed dispatch retains a non-authorizing terminal hold

  @cli @security
  Scenario: Unattached but active or unproven sessions are unavailable
    Given a candidate target has no tmux client and no CHROTE target lease
    When certified non-pane adapter evidence proves an open turn
    Then preflight reports "session_target_harness_busy" and creates no binding
    When exact closed/ready evidence is missing, stale, unsupported, or non-unique
    Then preflight reports "session_target_readiness_unknown" and creates no binding
    When complete client/input monitoring cannot be armed
    Then preflight reports "session_target_attachment_audit_unavailable" and creates no binding
    And final atomic acquisition repeats the certified-ready check before any dispatch or prompt

  @cli @file @security
  Scenario: Missing private lease state quarantines a possibly dispatched pane
    Given an unmatched ledger "slot_dispatch" exists but its exact private lease is missing or conflicting
    When recovery reconciles the frozen binding
    Then it atomically fsyncs a non-authorizing quarantine at the canonical target key before failure or finality
    And the quarantine preserves the expected dispatch/result and every conflicting occupant as separate exact candidates in stable run/dispatch/lease order
    And "target_lease_missing_or_mismatched" is recorded and no prompt/capture authority is recreated
    And a later run cannot acquire that pane until every candidate dispatch is proven quiescent
    And the final slot disposition records "targetLeaseState=quarantined"
    But a missing or corrupt canonical key quarantines all target dispatch host-wide

  @cli @file @security
  Scenario: Finality waits for a result-closed target record
    Given an exact "slot_result" is fsynced and its ledger dispatch is closed
    But the matching host target-registry record is not yet durably released
    When success, cancellation, failure, or crash recovery attempts to finalize the run
    Then no execution-final event is accepted
    And recovery atomically replaces occupancy with that exact result-closed "result_committed" receipt, certified turn-closure proof, durable audit proof, and closed releaseProof before finality
    And the record is not represented as an open slot disposition, terminal hold, or final quarantine
    But missing or conflicting private state first creates a temporary fail-closed quarantine
    And that candidate must prove the original barrier held or the exact post-result pane incarnation is gone before the exact receipt and finality
    And a crash reuses the receipt without another interrupt or quarantine
    And only then may the requested execution-final event append
    And the receipt may be removed only after that final event is fsynced

  @cli @file @security
  Scenario: A final-quiescent release survives a crash before cancellation or failure finality
    Given an unmatched dispatch has a certified cancel/ready acknowledgement or old-pane-gone proof for cancellation or failure
    And its occupancy is atomically replaced by an exact non-occupying "releaseKind=final_quiescent" receipt carrying that proof
    When the coordinator crashes before "run_canceled" or "run_failed" is fsynced
    Then a later run may acquire the now-unoccupied target without interleaving old work
    And recovery reuses the receipt for "targetLeaseState=released_quiescent"
    And it sends no second coordinator reconciliation interrupt and creates no quarantine for that old dispatch
    And the same final disposition is fsynced before the receipt is removed

  # ── JOIN readiness ──────────────────────────────────────────────────────────

  @ui
  Scenario: A node with distinct required inputs waits for all and freezes one attempt
    Given "ship:frame_input" and "ship:research_input" are distinct required ports
    When only "frame" has delivered
    Then "ship" shows "waiting · 1/2 inputs" and does not start
    When "research" also delivers
    Then "ship" becomes runnable and is dispatched
    And "node_started" freezes both refs as one immutable attempt input set

  @file
  Scenario: Optional data arriving after node_started is visible but cannot mutate the attempt
    Given "ship" has already started without its optional "context" data port
    When "context" receives a later delivery
    Then the ledger records "node_input_ignored" with reason "late_optional"
    And the current attempt is unchanged and no new attempt starts

  # ── Gate routing at run time ────────────────────────────────────────────────

  @ui
  Scenario: A gate evaluates then routes pass vs fail
    When the run reaches the gate
    Then the ledger records "gate_evaluating" then "gate_verdict"
    And a PASS preserves the durable ref/projection and exact authorized live value unchanged
    And it never substitutes redaction evidence for that value
    And a FAIL creates one stable typed "gate_feedback" object
    And its evaluated-input pointer contains only the input id and matching "gate_evaluating" sequence
    And it embeds no work ref, payload projection, payload text, or artifact
    And zero or more fail-edge traversals reference that same feedback object

  @ui
  Scenario: An unwired fail output blocks at the Gate without rewriting upstream work
    Given "gate:fail" is unwired and the verdict is FAIL
    Then one stable feedback object is recorded with zero delivery traversals
    And one aggregate "gate_verdict" records "routePort=fail" and "routedEdges=[]"
    And after quiescence the ledger records "run_blocked" with "reason=unwired_gate_fail"
    And the block has "blockScope=gate", only the Gate id, empty "openDispatches" and "retryTargets", "resumeAllowed=false", "resumePolicy=new_run_required", and no "nextEpoch"
    And it overlays the Gate attempt already closed by "gate_verdict" and does not close it again
    And the Gate retains visible FAIL/blocked state while the feeding Formation remains completed

  @ui
  Scenario: A fail wire to an earlier formation pushes back with revise feedback
    Given the gate evaluated "research" output
    And "research"'s entire connected workflow-output frontier is the direct edge into the gate
    And "gate:fail -> research:feedback"
    And "research:feedback" is an optional "gate_feedback" port with role "retry_control"
    And that pushback edge is the gate's entire fail frontier
    When the verdict is FAIL
    Then the feedback's evaluated source attempt must match "research" exactly
    And its frozen authoritative inputs must remain live or durably exact
    And a new bounded attempt reuses that attempt's frozen work refs and binds the feedback ref
    And its revised output opens gate attempt N+1 as the sole allowed late-required delivery
    And the new gate attempt records the stable revision-cycle id, feedback id, prior gate seq, and source attempt
    And the original brief and work payload are not annotated or replaced
    And no side output, fail fan-out, downstream replay, or non-source pushback occurs

  @file
  Scenario: Executable validation rejects an ambiguous pushback topology
    Given a pushback source has another connected workflow output or the gate fail port fans out
    When the board is validated for execution
    Then validation reports "pushback_topology_invalid"
    And no run starts

  @ui
  Scenario: A pushback loop stops loudly at the attempt limit
    Given "gate:fail -> research:feedback" and max attempts for "research" is 3
    When another feedback delivery would require attempt 4
    Then the ledger records node-scoped "error" with code "max_attempts_exhausted"
    And terminal "run_failed" records code "run_limit_exhausted" with that error as "failureCause"
    And its exact attempt, slot-dispatch, and Tool-lease dispositions revoke every open authority
    And "research" is not dispatched a fourth time

  # ── Judge execution ─────────────────────────────────────────────────────────

  @ui
  Scenario: A formation-only gate runs the judge chain and aggregates its result
    Given the gate declares only kind "formation"
    And the gate's judge is the chain "j1 -> j2 -> j3"
    When the run reaches the gate
    Then "gate_evaluating" is recorded and the judge chain executes from "j1"
    And each judge "node_started" freezes context hash and prior-result seqs instead of workflow inputs
    And each strict result is fsynced as one "judge_result" before the next judge dispatch
    And "judge_result" completes that judge attempt without ordinary "node_output"
    And replay reuses durable prior results without rerunning or reparsing completed judge capture
    And a next judge dispatch requires the exact evaluated input to remain live or durably exact
    And otherwise recovery records "run_failed" with code "redacted_input_unavailable"
    And when "j3" finishes, exactly one bounded verdict/reason/evidence result becomes Gate metadata
    And one "gate_kind_result" durably completes the formation kind
    And judge-authored content never becomes downstream work implicitly
    And only the aggregate "gate_verdict" routes pass/fail

  # ── Schema-1 inline verification compatibility ─────────────────────────────

  @file @security
  Scenario: Schema-2 execution rejects replay-ambiguous inline verification
    Given a schema-1 Formation contains an inline verification block
    When schema-2 validation or run preflight reads the board
    Then it fails "legacy_inline_verification_requires_migration" before "run_started"
    And no "verification_verdict", Formation output delivery, or revision dispatch is written
    And the schema-1 board and prior ledger remain inspectable without reinterpretation

  # ── Outputs are produced by the run, never authored ─────────────────────────

  @ui @file
  Scenario: A finished node has a produced Output with status, report, artifacts, and diffs
    When a formation finishes
    Then every contributing closed turn has one fsynced "slot_result" with matching turn key/phase and a hashed "slot-turn-result-jcs-v1" payload/output envelope
    And referenced non-Tool artifacts are registered before that slot result
    And the immutable graph snapshot's formation-type rule deterministically selects one bounded turn schedule and deciding turn
    And exactly one fsynced "formation_result" records the safe canonical "outputs", exact-key "outputHashes", result encoding/hash, already-registered artifact ids, and contributing slot-result sequences
    And "node_output" exact-matches that result's sequence/hash, status, outputs, report, artifacts, and diffs
    And recovery derives a missing Formation result from immutable turn envelopes or a missing node output from that result without reparsing capture or redispatching
    And its Output has a status in {done, needs-review, failed}; pre-result resource blocking has no Formation result
    And "node_output.outputs[portId]" contains typed durable payload projections for declared output ports
    And every artifact projection keeps one stable artifact id
    And an available safe artifact names root id, relative ref, media type, size, and SHA-256
    And unavailable, redacted, or expired artifacts keep that id but expose no readable ref
    And exactly one durable registration source establishes each artifact id
    And a slot, Gate, or system artifact uses fsynced "artifact_attached" before any later reference
    And a new Tool artifact is registered atomically by its lease-closing "tool_result"
    And registration identifies the producing slot dispatch, Tool lease, Gate attempt, or stable system source
    And File Peek revalidates content while durable availability changes only through "artifact_observed"
    And access validates and renders through one root-relative no-follow handle without reopening by path
    And an available observation supplies a validated descriptor whose artifact id matches the existing id
    And that descriptor exactly matches the first-established root, ref, media type, size, and SHA-256
    And changed content or location requires a new artifact id and registration
    And every non-available observation supplies an error code and no readable descriptor
    And an observation for an unknown artifact id is invalid
    And embedded artifact projections only mirror the latest registered or observed projection at their event sequence
    And unavailable or error output is explicit and never routed as successful work
    And "node_output.reportArtifactId", "artifactIds[]", and "diffArtifactIds[]" contain only already-registered ids
    And public rendering resolves each id through its latest authorized artifact projection
    And an available report is readable while an unavailable, redacted, or expired report exposes metadata only
    And the Output lives in run state, never in the board definition

  @file
  Scenario Outline: Formation type fixes its terminal result producer
    Given an ordinary "<type>" Formation has its persisted slot order frozen in the graph snapshot
    When all required "<turns>" slot results are durable
    Then "<terminal>" supplies Formation result status, outputs, output hashes, and report
    And only that terminal turn has a non-empty declared-port output map while non-terminal turns use their turn payload
    And artifact and diff ids are the stable first-seen union of contributing results in sequence order
    Examples:
      | type         | turns                                      | terminal                                      |
      | solo         | sole slot                                  | sole slot                                     |
      | flow         | ordered slots consuming the prior payload | last persisted slot                           |
      | peer         | every peer then facilitator synthesis      | first persisted peer's peer-facilitator turn |
      | orchestrated | controller plan, every worker, synthesis   | unique controller's leader-agentic turn      |

  @file @security
  Scenario Outline: Every Formation phase binds its complete ordered turn inputs
    Given a "<phase>" dispatch belongs to one frozen Formation attempt
    Then its closed "turnInputs.nodeStartedSeq" names that attempt's frozen ordered node inputs
    And its ordered "turnInputs.priorTurnResults" is exactly "<prior>"
    And every prior entry exact-matches one earlier slot-result sequence and turn-result hash
    And missing, extra, duplicate, reordered, or hash-mismatched entries reject before dispatch
    Examples:
      | phase             | prior                                           |
      | solo              | empty                                           |
      | first flow-step   | empty                                           |
      | later flow-step   | immediate predecessor                           |
      | peer-turn         | empty                                           |
      | peer-facilitator  | every peer-turn in persisted slot order         |
      | leader-plan       | empty                                           |
      | leader-worker     | leader-plan                                     |
      | leader-agentic    | leader-plan then every worker in persisted order |

  @file @security
  Scenario Outline: The first non-ok Formation turn closes a deterministic prefix
    Given an ordinary Formation has declared outputs A and B
    When a required turn records "<slotStatus>"
    Then no later required turn dispatches
    And one Formation result has status "<resultStatus>" and the exact completed slot-result prefix
    And every declared output repeats the same fixed non-routable "<payload>" projection
    And node output delivers no edge
    Examples:
      | slotStatus   | resultStatus | payload                    |
      | error        | failed       | normalized error turnPayload |
      | needs-review | needs-review | formation_needs_review      |

  @file @security
  Scenario: Formation needs-review uses one byte-exact non-routable projection
    When a required turn records "needs-review"
    Then every declared output is exactly {availability="available", exact=true, payload={kind="unavailable", code="formation_needs_review", message="Formation requires review", retryable=true}}
    And no locale, parser text, adapter text, or alternate message enters its output hashes

  @file @security
  Scenario: Closed invalid Formation output becomes a safe failed turn
    Given bounded capture ends with an exact terminal sentinel but its declared output map is invalid
    When the parser validates the closed capture once
    Then it records "slot_result.status=error" with exactly {availability="available", exact=true, payload={kind="error", code="invalid_formation_outputs", message="Formation outputs do not match the declared ports", retryable=true}}, outputs empty, and no raw parser text
    And the certified closed-turn proof permits ordinary result-committed release
    But a timeout or unclosed output block records no slot result and retains unmatched or terminal-hold authority

  @file @security
  Scenario: Orchestrated and peer turns never bypass coordinator authority
    Given a peer or orchestrated Formation uses its schema-2 bounded schedule
    Then every peer, plan, worker, facilitator, and synthesis turn has a fenced slot dispatch, target lease, result, and cancel/recovery identity
    And every dispatch binds its exact node-start sequence plus ordered prior slot-result sequences and hashes
    And no agent directly prompts, captures, interrupts, or writes another Formation-bound tmux target
    And no untracked shared chat or blackboard file is execution or replay authority

  @file @security
  Scenario: Artifact path swaps cannot race File Peek validation
    Given an available artifact descriptor resolves beneath its authorized root
    When a test swaps the path to a symlink or different file after open
    Then regular identity, size, media, and SHA-256 are checked on the no-follow opened handle
    And File Peek renders only those same verified bytes or returns unavailable
    And it never reopens the path after validation

  @ui @file @security
  Scenario: Artifact revocation removes readable authority from history
    Given artifact A was registered available and older ledger events reference its id
    When "artifact_observed" marks A redacted or expired
    Then run detail, bounded events, and SSE expose only A's latest metadata-only projection
    And Gate evidence and historical result events expose "artifactId=A" without an old root or ref
    And File Peek cannot open the formerly readable descriptor
    And the raw writer-only ledger is never returned as a browser authority surface

  @file
  Scenario: A redacted durable projection is never mistaken for routable input
    Given a "Redact=true" node produces an authoritative value that policy discards
    When fresh execution routes that value to the next node
    Then "node_output" and "node_started.inputRefs" persist only stable ids, provenance, and redacted payload projections
    And the exact value exists only in paired in-memory execution refs
    And it may survive safe slot-result, Formation-result, node-output, Gate, join, and dispatch fsyncs until every scheduled internal Formation consumer and every authorized taken-edge consumer sends once or becomes durably non-deliverable
    And it is erased when no Gate/retry path retains it and always on cancellation, finality, or process loss
    And no marker, hash, summary, or sanitized replacement is a graph input
    And sanitized non-exact evidence exists only outside port output maps in bounded display or artifact fields
    And recovery without that live value records "run_failed" with code "redacted_input_unavailable"

  @file
  Scenario: A pure Tool run freezes its executable profile contract
    Given a Tool references a host-owned profile id, version constraint, and non-secret parameters
    When the run starts
    Then it freezes the exact profile version/content hash, parameters, effective policy, determinism policy, and execution-bundle hash
    And the bundle covers executable, toolchain/script, argv, cwd, normalized non-secret environment values, supervisor/fence policy, and limits
    And the host-private binding authority stores them as one "RunToolBinding"
    And preflight rejects before "run_started" if the frozen supervisor/fence policy is unavailable
    And the certified sandbox denies network, secrets, undeclared environment/filesystem reads, and external writes
    And it normalizes locale/timezone, freezes or denies clock/entropy, and passes repeat vectors with expected output hashes
    And outputs are confined to one host-private run root outside generic Files roots
    And a redacted Tool fsyncs its private obligation as "pending" for generation 1 before public "tool_dispatch"
    And every exact input is validated into one sealed content-addressed read-only set before "tool_dispatch"
    And "tool_dispatch" is fsynced with the lease, input-manifest hash, and all execution hashes before process start
    And its lease id is unique in the run and identifies exactly this Tool node attempt
    And an obligation orphaned before dispatch is cleaned and deleted without Tool execution
    And the host reserves a private descendant-process scope and immutable deadline authority before fsyncing "tool_process_launch"
    And that authority derives one "startedAt" and "effectiveDeadlineAt" from the frozen timeout policy before spawn
    And the first launch generation is 1 with unique launch/scope/deadline-authority ids
    And its run, lease, launch, node, attempt, generation, scope, and deadline-authority tuple exactly matches the private records before the process spawns
    And for a redacted Tool the launch generation matches its "pending" obligation generation
    And the opaque scope id is not a reusable raw PID and retains private supervisor identity/start evidence
    And a private scope/deadline reservation orphaned before launch is fenced then reused as one exact pair or both records are deleted and directory-fsynced before replacement
    And no orphan path retains one half or spawns
    And the scope is sealed against new members and proven quiescent before final output promotion
    And one fsynced "tool_result" names the latest launch and closes that exact lease with the full durable canonical output map
    And its "artifactRegistrations" atomically registers every new Tool artifact under that lease
    And an open Tool lease has no public artifact registrations
    And a redacted Tool fsyncs its private obligation from "pending(N)" to "cleaned(N)" while retaining the locator before the result
    And the result's latest launch generation matches that "cleaned(N)" obligation
    And it deletes that private entry only after the policy-safe result is fsynced
    And "run_succeeded" requires no open node attempt, slot dispatch, Tool lease, host target lease, private obligation, or Tool-scope entry
    And each output is exact available payload or hash-only redacted evidence
    And sanitized non-exact Tool evidence never occupies that output map or authorizes routing
    And a completed Tool lease is never rerun

  @file @security
  Scenario: Tool execution never rereads a mutable source artifact
    Given an exact SafeArtifactRef is a Tool input
    When the host copies and fsyncs its validated bytes into the sealed input set
    And the source file changes before process spawn
    Then the Tool receives only the original content-addressed bytes
    And the lease and every launch exact-match the sealed manifest hash
    And recovery reuses that same sealed set without reopening the source
    But a missing or mismatched materialization fails loud before spawn and never reruns

  @file @security
  Scenario: A Tool result cannot commit while a descendant is still alive
    Given a Tool parent exits while a process in its recorded scope remains live
    And another independent Tool lease remains open
    And an independent slot dispatch remains open
    When the coordinator attempts final output promotion
    Then the ledger records terminal "run_failed" with code "tool_process_not_quiescent"
    And "failureCause" names the causative Tool lease, not "relatedSeq"
    And "nodeAttemptDispositions" fails that Tool attempt and abandons every other open attempt
    And "toolLeaseDispositions" covers both open leases exactly
    And the causative Tool is "failed_private_cleanup_owned" and the other is "abandoned_private_cleanup_owned"
    And "slotDispatchDispositions" abandons the open slot without authorizing a late result
    And no output is promoted and no "tool_result" is appended
    And restart projects neither Tool in flight or rerunnable

  @file @security
  Scenario: Recovery fences a surviving Tool child before cleanup or rerun
    Given a fsynced "tool_process_launch" whose process or descendant survives a coordinator crash
    When recovery reconciles the open Tool lease
    Then the supervisor terminates and seals the exact recorded scope and proves every descendant quiescent by the frozen deadline
    And no root is removed or promoted and no later launch is appended before that proof
    And after proof, cleanup leaves a durable sealed/quiescent old-scope tombstone until the next launch or terminal non-rerun event is fsynced
    And for a redacted Tool recovery fsyncs "cleaned(N)" to "pending(N+1)" before recording or spawning the rerun
    And a newly fsynced monotonically increasing launch generation precedes the rerun spawn
    And only after that launch commit is the old scope record deleted and directory-fsynced
    And the new launch consumes dispatch and wall-clock limits while retaining its logical node attempt

  @file @security
  Scenario: Recovery resumes a prepared redacted generation without losing cleanup ownership
    Given recovery fsynced "cleaned(N)" to "pending(N+1)" and crashed before launch N+1
    When recovery restarts with public generation N and no public launch N+1
    Then it treats the pending obligation as a safe prepared generation
    And it cleans the root idempotently and fences any orphan N+1 scope without spawning
    And it retains the pending locator and validates before reserving or reusing generation N+1

  @file @security
  Scenario: Recovery distinguishes a never-spawned Tool lease from a lost process fence
    Given "tool_dispatch" is fsynced but no "tool_process_launch" exists
    When recovery reconciles the open Tool lease
    Then it fences and deletes any orphan private scope without spawning
    And absence of a process scope is not "tool_process_not_quiescent"
    And generation 1 may launch only after the normal hash, input, redaction, and limit checks

  @file @security
  Scenario: Recovery refuses an unsafe Tool rerun when the process fence is unproven
    Given an open Tool lease with a recorded launch whose matching process scope is missing, ambiguous, or still live at the deadline
    When recovery attempts to fence that launch generation
    Then the ledger records terminal "run_failed" with code "tool_process_not_quiescent"
    And "failureCause" names that Tool lease and its node-attempt disposition is "failed_non_authorizing"
    And the root is not cleaned, promoted, or rerun
    And restart projects the Tool lease, launch, and node failed with private cleanup ownership
    And restart may continue only private fencing/cleanup, never the open-lease rerun branch

  @file
  Scenario Outline: Declared-output production failure has one explicit lifecycle
    Given one declared output ends as "<kind>" with engine-derived retryable "<retryable>"
    When in-flight and independent branches finish recording evidence
    Then the failed producer attempt has "deliveredEdges=[]" for every sibling output
    And no descendant on the failed dependency path is dispatched
    And the run records "<terminal>"
    And when "<terminal>" is "run_failed", "slotDispatchDispositions" exactly closes every still-open slot dispatch
    And when "<terminal>" is "run_failed", "toolLeaseDispositions" exactly closes every still-open Tool lease
    And when "<terminal>" is "run_failed", "nodeAttemptDispositions" exactly closes every still-open node attempt
    And when "<terminal>" is "run_failed", "failureCause" is present and "relatedSeq" is not used as cause identity
    And when "<terminal>" is "run_blocked", it has "blockScope=node", "resumePolicy=retry_failed_producer", "openDispatches=[]", exactly one whole-producer "retryTarget", and "nextEpoch"
    And no automatic retry occurs
    Examples:
      | kind        | retryable | terminal    |
      | unavailable | true      | run_blocked |
      | error       | false     | run_failed  |

  @file
  Scenario: Parallel retryable failures are selected one at a time deterministically
    Given independent producers A and B have latest retryable failed attempts with "deliveredEdges=[]"
    And B's minimum outcome sequence sorts before A's
    When the graph becomes quiescent
    Then "run_blocked" names only B's whole-producer retry target
    And A remains a closed durable unresolved failure without dispatch or abandonment
    And "run_succeeded" is forbidden
    When I explicitly resume B and its next attempt succeeds
    And the graph becomes quiescent again
    Then a new "run_blocked" names only A's whole-producer retry target
    And retrying A requires a separate explicit resume
    And completed B is not rerun

  @file
  Scenario: Parallel non-retryable output failures choose stable terminal provenance
    Given several latest declared-output failures are non-retryable and already closed
    When the graph becomes quiescent
    Then the first failure by minimum outcome sequence then node id selects "run_failed(code=declared_output_failed)"
    And "relatedSeq" is that selected outcome sequence as provenance only
    And "failureCause" is "kind=none" because no failed producer attempt remains open
    And every other closed failure remains inspectable evidence

  @file
  Scenario: One non-retryable output makes a mixed producer and run terminal
    Given producer A has a retryable failed output and "deliveredEdges=[]"
    And producer B's same closed attempt has one retryable and one non-retryable failed output
    And no later attempt exists for either producer
    When the graph becomes quiescent
    Then B is a non-retryable whole-producer candidate containing both failed port/outcome identities
    And B selects terminal "run_failed(code=declared_output_failed)" even if A sorts earlier
    And no "retry_failed_producer" block or automatic retry is appended
    And A and B remain inspectable closed evidence

  @file
  Scenario: Quiescence selects one stable semantic blocker before retryable work
    Given there is no cancel intent, execution-final condition, unmatched dispatch, or outstanding human request
    And an unsatisfied JOIN has causal sequence 20
    And an unwired Gate FAIL has causal sequence 30
    And an independent retryable producer failure has outcome sequence 10
    When the graph becomes quiescent
    Then non-resumable semantic candidates dominate the retryable candidate
    And exactly one "run_blocked" selects "unsatisfied_required_input" by causal sequence, reason rank, and ids
    And it has "resumeAllowed=false" and "resumePolicy=new_run_required"
    And the Gate FAIL and retryable producer remain inspectable durable evidence
    And no second block or retry dispatch is appended

  @file
  Scenario: An outstanding human request remains visible before semantic blockers
    Given there is no cancel intent, execution-final condition, or unmatched dispatch
    And a Gate attempt has an outstanding exact "human_input_requested"
    And independent branches leave an unsatisfied JOIN and a retryable producer failure
    When the graph becomes quiescent
    Then the Gate and run remain "waiting_human"
    And no "run_blocked" hides the human request
    When a journaled verdict command id/hash exact-matches the outstanding human-request sequence
    And the matching human decision contributes its kind result
    And the aggregate "gate_verdict" closes and routes that Gate attempt
    And the graph becomes quiescent again
    Then the stable semantic blocker is selected before the retryable producer

  @cli @file
  Scenario: Explicit operator resume retries one whole failed producer safely
    Given the latest "run_blocked" names an exact retryable producer attempt
    And that attempt delivered no edges and its frozen authoritative inputs remain available
    When I explicitly resume the run with one stable command id and the exact blocked sequence
    Then "run_resumed" binds that command id/hash and opens epoch N+1 with the exact recorded retry target
    And it records "resumeMode=retry-failed-producer" and "openDispatches=[]"
    And "node_started" records reason "resume" and producer attempt N+1
    And it reuses the frozen input refs and unchanged slot or Tool binding
    And it creates a new dispatch or Tool lease
    And completed sibling nodes are not rerun

  @cli @file
  Scenario: Resume cannot substitute for discarded redacted input
    Given a retryable blocked producer requires an authoritative input discarded by redaction
    When I explicitly resume the run
    Then resume is rejected and no new epoch or dispatch opens
    And the ledger records "run_failed" with code "redacted_input_unavailable"

  # ── Liveness, cancel, and fail-loud limits ──────────────────────────────────

  @ui @cli
  Scenario: A run streams live and can be watched from either surface
    When a run is in progress
    Then the UI and CLI "archon run logs --follow" receive the same sanitized projection
    And neither surface receives raw ledger bytes, private paths, or revoked artifact refs

  @cli
  Scenario: A run can be cancelled
    When I run "archon run cancel run_01J9" with a stable command id and expected last sequence
    Then "run_cancel_requested" binds that command id/hash and is fsynced first with the exact open node attempts, slot dispatches, and Tool leases
    And each attempt snapshot preserves node/kind/attempt/start sequence and its latest durable phase/sequence
    And each slot snapshot preserves dispatch/target-lease/node/attempt/slot/binding/target identity plus exact Peek capability and steering state
    And each lease snapshot preserves node/attempt/dispatch identity plus its optional latest launch/scope/deadline-authority identity
    And new dispatch/replay and Peek input stop and the writer rejects launches, results, outputs, routing, and new capability issuance
    And every Peek input channel drains, each open steering generation closes, and irreversible capability revocation fsyncs before final proof
    And active slots are soft-interrupted without killing tmux sessions
    And a soft interrupt is sent only to a target proven to host that exact unresolved dispatch and attempt
    And every snapshotted slot dispatch receives an exact canceled/non-authorizing disposition
    And its target occupancy becomes an exact durable "final_quiescent" receipt only with certified closed proof, remains a non-authorizing terminal hold, or enters exact quarantine
    And every never-launched Tool lease is cleaned without execution
    And every launched Tool scope is terminated, sealed, and proven quiescent before its root is cleaned
    And "run_canceled.cancelRequestSeq" names that unique request
    And "run_canceled" exactly reconciles the node attempts, slot dispatches, and Tool leases as the last accepted execution event
    And each attempt, slot, or Tool reconciliation entry preserves its snapshot identity and adds one typed disposition
    And a repeated cancel is idempotent and replaces none of the original snapshots
    And "abort" or "stop" compatibility spelling normalizes to cancel before hashing and creates no second snapshot or state
    And no further node, dispatch, steering, or capability events are appended
    But non-authorizing binding or artifact observations may still append for inspection

  @cli @security @recovery
  Scenario: Terminal failure freezes reconciliation before any slot interrupt
    Given a run will fail with a closed slot, Tool, error, or none failure cause
    When failure reconciliation begins
    Then "run_failure_reconciliation_started" fsyncs that exact cause/header and complete open attempt, slot, and Tool snapshots first
    And the run projects non-final status "failing"
    And if it was activated, it retains its active capacity
    And every failure-authorized "slot_reconciliation_interrupt" names that start sequence
    And a crash resumes the same snapshots without choosing a new cause or resending an existing interrupt request
    And "run_failed.failureReconciliationSeq" exact-names that start event
    And its code, reason, unrecoverable flag, related sequence, and failure cause byte-match the frozen header
    And its dispositions exactly cover all three frozen snapshots

  @cli @security @recovery
  Scenario: Cancel escalation to failure never sends a second coordinator interrupt
    Given cancel reconciliation durably requested or attempted a slot interrupt
    When Tool quiescence failure requires terminal "run_failed"
    Then "run_failure_reconciliation_started.originCancelRequestSeq" exact-names the unique cancel request
    And that start preserves the cancel-time slot identity and prior interrupt request/outcome
    And the run projects "failing" rather than returning to "canceling"
    And failure reconciliation reuses that request, including "send_uncertain"
    And it creates a failure-authorized interrupt only for an open slot with no prior request
    And no slot receives a second coordinator reconciliation interrupt request or send
    And terminal "run_failed.failureReconciliationSeq" exact-names that failure start
    And its header byte-matches the start and its dispositions exactly cover the frozen snapshots

  @cli @file
  Scenario: Success waits for the result-committed target release receipt
    Given an exact slot result is fsynced but its host target lease is still present
    When the coordinator considers the run complete
    Then it does not append "run_succeeded"
    And recovery may replace only the exact result-matched occupancy
    And success is allowed only after its durable "result_committed" receipt names the exact result sequence and certified turn-closure proof

  @cli @file
  Scenario: Canceling a Gate waiting for a human closes the same attempt
    Given a Gate attempt has an outstanding "human_input_requested" and projects "waiting_human"
    And that Gate has no open slot dispatch or Tool lease
    When I cancel the run
    Then "run_cancel_requested.openNodeAttempts" contains that exact Gate attempt with phase "waiting_human"
    And "run_canceled.nodeAttemptDispositions" preserves its identity and adds "canceled_non_authorizing"
    And the Gate and run no longer project "waiting_human"
    And a later decision for the disposed request is rejected

  @file
  Scenario Outline: Finality closes a downstream node that never started
    Given downstream node D recorded "node_waiting" but no "node_started"
    When the run records terminal "<terminal>"
    Then D projects "<node_state>" instead of active "waiting"
    And D's last readiness counts remain available as inspection evidence
    Examples:
      | terminal     | node_state |
      | run_canceled | canceled   |
      | run_failed   | abandoned  |

  @file
  Scenario: Valid success marks an untaken branch node not run
    Given downstream node D never started and received no input delivery
    And the ledger proves D lies only on the untaken Gate branch
    When the run records "run_succeeded"
    Then D projects "not_run"
    But "run_succeeded" is rejected if a never-started node received an input on a taken path

  @cli @security
  Scenario: Cancellation intent survives a coordinator crash
    Given "run_cancel_requested" is fsynced while a Tool lease remains open
    When the coordinator restarts before "run_canceled"
    Then the run projects "canceling" and no ordinary recovery or rerun occurs
    And recovery continues only cancellation reconciliation toward "run_canceled" or a cancel-origin "run_failure_reconciliation_started" followed by its matching "run_failed"
    And after "run_canceled" every snapshotted attempt and slot projects canceled
    And restart rejects a late slot result and never projects that dispatch in flight

  @cli @security
  Scenario: Cancellation fails closed when a Tool descendant cannot be fenced
    Given an open Tool launch has a descendant whose quiescence cannot be proven by the frozen deadline
    When I cancel the run
    Then "run_canceled" is not appended
    And terminal "run_failed" records code "tool_process_not_quiescent"
    And "failureCause" names the causative Tool lease
    And every open node attempt receives a failed or abandoned non-authorizing disposition
    And private reconciliation retains cleanup ownership after finality
    And redacted raw targets keep their obligation until sanitation or removal is fsynced
    And no promotion, result, rerun, or further execution event occurs

  @cli @security
  Scenario: A partial multi-Tool cancellation leaves no Tool logically in flight
    Given cancellation snapshots open Tool leases A and B
    And it snapshots an open slot dispatch C
    And A is cleaned but B cannot be proven quiescent by the deadline
    When terminal "run_failed" records B's fence failure
    Then "failureCause" names Tool lease B
    And "toolLeaseDispositions" exactly covers A and B
    And "slotDispatchDispositions" abandons C as non-authorizing
    And "nodeAttemptDispositions" fails B's attempt and abandons A's and C's attempts
    And B is "failed_private_cleanup_owned"
    And A is "abandoned_private_cleanup_owned"
    And restart projects neither lease or node in flight or rerunnable
    And each supervisor may continue only private terminate, seal, prove, and cleanup work

  @cli @security
  Scenario Outline: Multiple unquiescent Tool scopes choose one stable failure cause
    Given "<boundary>" freezes open launched Tool leases A and B
    And each launch has an already-fsynced private quiescence boundary with one derived "effectiveDeadlineAt"
    And neither scope can be proven quiescent by its persisted deadline
    And A sorts before B by "effectiveDeadlineAt", dispatch sequence, then Tool lease id
    When reconciliation derives the complete failed Tool candidate set
    Then "run_failed" records code "tool_process_not_quiescent"
    And "failureCause" names Tool lease A regardless of callback or completion arrival order
    And A is "failed_private_cleanup_owned"
    And B is "abandoned_private_cleanup_owned"
    And both supervisors retain private cleanup ownership
    Examples:
      | boundary          |
      | normal completion |
      | startup recovery  |
      | cancellation      |

  @cli @file @security
  Scenario: Restart reuses the same Tool quiescence boundaries and ordering
    Given launched Tool leases A and B have fsynced private quiescence boundaries
    And A sorts before B by their persisted "effectiveDeadlineAt", dispatch sequence, then Tool lease id
    When the coordinator crashes during fencing and restarts twice
    Then recovery exact-matches and reuses both boundary ids, causes, start times, timeout policies, and deadlines
    And no restart time grants either launch a new deadline
    And the reconstructed failed candidate set and order are byte-identical
    And A remains the sole "tool_process_not_quiescent" failure cause

  @cli @file @security
  Scenario: Crash after Tool spawn cannot create a fresh fence deadline
    Given Tool launches A and B have fsynced private scopes and immutable deadline authorities
    And both public "tool_process_launch" events exact-match those authorities before process spawn
    And the processes spawn but no quiescence-boundary record is yet durable
    When the coordinator crashes and startup recovery begins
    Then recovery creates each boundary only from its launch's persisted deadline authority
    And every start time, timeout policy, duration, and "effectiveDeadlineAt" remains unchanged
    And callback, map, and recovery-loop order cannot change the candidate ordering
    And restart grants no fresh timeout

  @cli @file
  Scenario: Terminal failure closes mixed active and waiting attempts
    Given Tool attempt T has open lease L
    And Gate attempt G is waiting on an exact human request
    And Formation attempt F has an open slot dispatch
    When L fails its process fence and "run_failed.failureCause" names L
    Then "nodeAttemptDispositions" exactly covers T, G, and F in start-sequence order
    And T is "failed_non_authorizing"
    And G and F are "abandoned_non_authorizing"
    And the Tool lease and slot dispatch receive matching failed or abandoned dispositions
    And restart projects no attempt active, evaluating, waiting, or rerunnable

  @cli
  Scenario: An unrecoverable engine error records run_failed
    Given the adapter returns an unrecoverable dispatch error before any slot receives work
    And no Tool lease is open
    And no slot dispatch is open
    And no node attempt is open
    When the engine cannot continue the run
    Then the ledger records "error" naming the failing adapter boundary
    And "run_failed" is recorded as the terminal event
    And "failureCause" names that run-scoped error sequence
    And it records "nodeAttemptDispositions=[]"
    And it records "toolLeaseDispositions=[]"
    And it records "slotDispatchDispositions=[]"

  @cli
  Scenario: Per-run fail-loud limits stop runaway loops without asking permission
    Given a per-run max-dispatch count, max-attempt count, and wall-clock timeout
    When any limit is exceeded
    Then the run records the exact stable limit code as scoped "error"
    And terminal "run_failed" records "code=run_limit_exhausted" and that error as "failureCause"
    And "nodeAttemptDispositions", "slotDispatchDispositions", and "toolLeaseDispositions" exactly revoke every open authority
    And finality rejects any issued Peek capability, undrained input channel, or open steering generation
    And a slot is soft-interrupted only on its proven exact target
    And Tool scopes retain private post-final fencing and cleanup ownership
    And late result, output, routing, replay, and resume are rejected
    And continuing requires a new run with a new frozen limit snapshot
    # Honors "no safeguards" (no gating) while preventing the runaway-cost failure mode.
