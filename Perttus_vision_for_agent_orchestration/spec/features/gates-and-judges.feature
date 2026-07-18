# Captures the prototype's gate + judge model (03-formations.js: makeGateNode, GATE_KINDS,
# attachJudge / setJudgeReturn / judgeEntry / syncJudgeKind, startJudgeWire, openJudgePicker,
# evalGate, followBranch). The headline (D7): a judge can be ONE formation (a loop) or SEVERAL
# wired in sequence, and the gate actually RUNS that judge chain to decide.
# Prototype `gate.verdict` is a mock value only. Real gate definitions do not store verdicts or route
# from remembered/default verdict state.

Feature: Gates and judges — checkpoints that route work and can be judged by formations
  As an agent wiring quality checkpoints into a mission
  I need gates that combine code/human/formation checks and route pass vs fail
  So that work only proceeds when it should, and loops back when it shouldn't

  Background:
    Given a board "session-search"
    And a gate "gate" with input "in", outputs "pass" and "fail", and a "judge" socket
    And "judge" is reserved evaluation control with at most one send and one return, never work routing
    And every "run_failed" exact-names one prior unique "run_failure_reconciliation_started" through "failureReconciliationSeq"
    And that start projects non-final "failing", freezes the failure header and complete open-resource snapshots, and permits reconciliation only
    And the final failure byte-matches that header and exactly disposes those snapshots

  # ── Creating gates ──────────────────────────────────────────────────────────

  @ui @file
  Scenario Outline: A gate can be created from any of the prototype's affordances
    When I create a gate via "<gesture>"
    Then a gate node is persisted with default kind "code" and no stored verdict
    And fail behavior is determined by the gate's fail-port wiring
    Examples:
      | gesture                                   |
      | dragging the topbar gate token to canvas  |
      | right-clicking the board and choosing Gate |
      | the New menu's Gate item                   |

  @cli @file
  Scenario: A gate is created from the CLI with explicit checks
    When I run "archon gate create session-search --kinds code,human --criterion 'research is sound and safe to build'"
    Then the gate has kinds "code" and "human"
    And its criterion is "research is sound and safe to build"

  # ── Kinds combine ───────────────────────────────────────────────────────────

  @ui @file
  Scenario: Check kinds are additive — code, human, and formation can be combined
    When I enable "code" and "human" on the gate
    Then the gate's label reads "Code · Human"
    And at least one kind is always present (removing the last one is refused)

  @file @security
  Scenario Outline: Invalid persisted Gate kind arrays fail before execution
    Given a hand-edited Gate has kinds <kinds>
    When run preflight validates the board
    Then the board is rejected before "run_started"
    And no Gate evaluation event is written
    Examples:
      | kinds                    |
      | "[]"                     |
      | "[code,code]"            |
      | "[code,unknown-check]"   |

  @file @security
  Scenario: A schema-2 code Gate uses a frozen pure evaluator profile
    Given a code Gate references a host-owned evaluator profile and non-secret parameters
    When run preflight resolves the Gate
    Then one "RunGateBinding" freezes its profile version/content, evaluator-bundle, parameter, policy, and determinism-policy hashes
    And it freezes positive input-byte, result-byte, and deterministic operation limits plus "resultEncoding=decision-result-jcs-v1"
    And the certified evaluator is in-process and admits only declared input plus frozen bundle/parameters/policy
    And network, secrets, undeclared host reads, spawn, and host writes are denied
    And locale/timezone are normalized, clock/entropy are frozen or denied, and repeat vectors match expected result hashes
    And a profile requiring an OS process is rejected before "run_started"
    And Gate-owned argv/shell process evaluation is retired

  @file @security
  Scenario Outline: Every authored legacy Gate command field is inspection-only
    Given a schema-1 Gate contains authored field <field> even when its value is empty
    When the board is read and validated
    Then source inspection preserves that field without executing or rewriting it
    And validation reports "legacy_script_gate_requires_fenced_migration" for that Gate
    And an inert legacy string or cwd-only Gate receives the same migration finding
    And "gate_not_routable" is reported separately when the Gate has no human or formation-judge route
    And no executable, cwd, environment, argv, or profile is resolved or inferred
    Examples:
      | field          |
      | command        |
      | commandArgv    |
      | commandShell   |
      | commandCwd     |

  @file @security
  Scenario: A reachable legacy command Gate fences one selected Mission root
    Given a selected Mission can reach a Gate with a legacy command field on a possible pass or fail branch
    When that Mission starts or its frozen snapshot resumes
    Then it fails with "legacy_script_gate_requires_fenced_migration" before "run_started" or "run_resumed"
    And no snapshot, binding, ledger, Gate evaluation, process, verdict, or route is produced
    And reachable judge chains are included in the selected-root check

  @file @security
  Scenario: Legacy command findings outside the selected root do not broaden execution preflight
    Given board validation reports a legacy command Gate
    And that Gate is unreachable from the selected Mission
    When the selected Mission starts
    Then the whole-board validation finding remains visible
    But that unreachable Gate does not block the selected Mission

  @file @security
  Scenario: An isolated Formation root does not traverse legacy command Gates
    Given board validation reports a legacy command Gate outside an isolated Formation root
    When I run that isolated Formation
    Then downstream board edges are not traversed
    And the legacy Gate does not block the isolated Formation root

  @cli @api @file @security
  Scenario: Legacy Gate migration is a non-mutating plan before Tool profiles exist
    Given a legacy command Gate remains in the source board
    When API inspection or "archon board validate --json" describes migration
    Then the plan binds board id, revision, ETag, Gate id, source mode, source field names, and affected edge ids
    And it records "code=legacy_script_gate_requires_fenced_migration"
    And it records "targetKind=tool_plus_pure_gate", "ready=false", and "applySupported=false"
    And it lists the host Tool profile, pure Gate profile, explicit mapping, media compatibility, and atomic CAS requirements
    And it contains no raw command value, resolved executable or cwd, environment, generated Tool id or port, parameters, or suggested profile
    And no migration mutation verb or endpoint is available

  @file @security
  Scenario: A later explicit Tool-to-pure-Gate migration never infers legacy command semantics
    Given ctx-ug7.8.1 has supplied non-executing Tool definitions and registry descriptors
    And ctx-ug7.8 has supplied certified host-private implementations and runtime execution
    And ctx-ug7.30 has supplied certified pure code-Gate profiles and the explicit apply
    When a future caller explicitly selects Tool and pure-Gate profiles, parameters, and port mapping
    Then one atomic compare-and-swap validates every affected edge and downstream media contract
    And the existing Gate id, title, criterion, judge relationships, pass/fail edges, and layout are preserved
    And only the inserted Tool receives new-node placement
    And the Gate evaluates and forwards the exact Tool output rather than the pre-Tool payload
    But an unprovable mapping leaves the source bytes unchanged and fails loud

  @file @security
  Scenario Outline: A code Gate evaluator cannot wedge the coordinator
    Given the admitted evaluator is total under its frozen host-metered operation budget
    When evaluation ends by "<boundary>"
    Then one Gate-scoped "error" records "code=gate_evaluator_error"
    And independent work settles before a non-resumable Gate block
    And no "gate_kind_result", aggregate "gate_verdict", or route is recorded
    And immutable run wall-clock finality can still make progress
    Examples:
      | boundary          |
      | fuel exhaustion   |
      | input byte limit  |
      | result byte limit |
      | contained panic   |

  @file @security
  Scenario: Code Gate replay cannot substitute an upgraded evaluator
    Given a code Gate evaluation crashed before its "gate_kind_result"
    And the CHROTE binary or evaluator registry now resolves different implementation bytes
    When recovery considers repeating the evaluation
    Then it requires the exact frozen evaluator-bundle and determinism-policy hashes
    And it fails loud without evaluation, result, verdict, or route when they are unavailable or mismatched
    But a completed matching "gate_kind_result" is replayed without evaluator execution

  @file @security
  Scenario: A code Gate hashes one canonical strict result
    Given a certified code evaluator returns one strict verdict, reason, and ordered evidence array
    When the code kind result is accepted
    Then exactly "{verdict,reason,evidence}" is encoded as RFC 8785 canonical UTF-8 JSON with no unknown keys or trailing newline
    And "gate_kind_result" records "resultEncoding=decision-result-jcs-v1"
    And "resultSha256" hashes those exact bytes with evidence order preserved
    And replay cannot substitute another serializer or result encoding

  @file @security
  Scenario Outline: A code Gate cannot recover discarded redacted input
    Given a code Gate has frozen "inputSha256" and has not recorded "gate_kind_result"
    When an "<phase>" evaluation finds Redact=true exact bytes are no longer live
    Then bounded Gate-scoped context names the gate, attempt, and input id
    And terminal "run_failed" records "code=redacted_input_unavailable"
    And "relatedSeq" names the exact source input only as provenance
    And "failureCause" is "kind=none"
    And exact dispositions revoke every open node attempt, slot dispatch, and Tool lease
    And no hash, marker, summary, or drifted artifact is substituted
    And no "gate_kind_result", aggregate "gate_verdict", or route is recorded
    Examples:
      | phase    |
      | initial  |
      | recovery |

  @file @security
  Scenario Outline: A code Gate fails the exact attempt when durable input drifts
    Given a non-redacted code Gate has frozen "inputSha256" and has not recorded "gate_kind_result"
    When an "<phase>" evaluation finds artifact bytes that mismatch that hash
    Then one Gate-scoped "error" records "code=gate_input_integrity_failed"
    And terminal "run_failed" records the same code with that error as "failureCause"
    And the exact Gate attempt is failed while other open attempts are abandoned
    And exact dispositions revoke every open slot dispatch and Tool lease
    And no drifted artifact is substituted
    And no "gate_kind_result", aggregate "gate_verdict", or route is recorded
    Examples:
      | phase    |
      | initial  |
      | recovery |

  @ui @file
  Scenario: Mixed Gate kinds aggregate once in deterministic all-of order
    Given a Gate declares kinds in stored order "human,formation,code"
    When its code check passes and its formation judge passes
    Then code runs before formation and each fsyncs one unique "gate_kind_result"
    And one "human_input_requested" records those exact result sequences and projects "waiting_human"
    And restart reuses both completed results without rerunning code or the judge chain
    When the matching human decision passes
    Then "human_verdict_recorded" completes the human kind and returns the same Gate attempt to evaluation
    And exactly one "gate_verdict" records code, formation, and human as "pass"
    And its kind-result sequence map points to each exact completion event
    And only that aggregate verdict routes the pass edge

  @ui @file
  Scenario: The first mixed-kind failure short-circuits later checks
    Given a Gate declares kinds "code,formation,human"
    When the code kind records a strict fail
    Then formation and human are not run or requested
    And exactly one aggregate "gate_verdict" records code "fail" and both later kinds "not_run"
    And only the fail route is considered

  @file
  Scenario: A human Gate wait has no implicit default or request timeout
    Given prior declared kinds passed and one "human_input_requested" is outstanding
    When no matching human decision has arrived
    Then the Gate remains "waiting_human" and the request has no "timeoutSeconds"
    And its prompt is the closed fixed-system "gate-human-verdict-v1" projection
    And "choiceProjections" is the exact closed fixed-system object keyed by "pass" and "fail" using "gate-human-pass-v1" and "gate-human-fail-v1"
    And those immutable template ids require a new version for any text change, while an unknown id is rejected
    And they never interpolate runtime input, output, evidence, capture, or secrets
    And only a matching decision, cancellation, or terminal run wall-clock exhaustion can end the wait
    And wall-clock exhaustion records the ordinary terminal run-limit failure, never an invented human verdict

  @file @security
  Scenario Outline: Authored-configuration source roles cannot be relabeled
    Given a writer attempts to persist a typed configuration projection with "<substitution>"
    When schema-2 event validation runs
    Then the append is rejected before any public projection
    Examples:
      | substitution                                      |
      | mission_objective used as gate_criterion          |
      | gate_criterion used as a human_choice             |
      | root-authored bytes in generic PayloadProjection  |
      | copied config with missing or extra manifest entry|
      | config field and manifest hash mismatch           |
      | unknown sourceKind, encoding, or media combination|

  @file
  Scenario: A code evaluator boundary error is not a fail verdict
    Given a schema-2 code Gate evaluator cannot return a strict result because of a boundary error, frozen limit, or contained panic
    When the Gate evaluates that kind
    Then "error" records "code=gate_evaluator_error", "boundary=evaluator", and "errorScope=gate" with the Gate id
    And after independent work settles a non-resumable "run_blocked" uses "blockScope=gate" with only the Gate id
    And it has empty "openDispatches" and "retryTargets", "resumeAllowed=false", "resumePolicy=new_run_required", and no "nextEpoch"
    And no "gate_kind_result", aggregate "gate_verdict", or route is recorded

  @ui @file
  Scenario: Selecting the "formation" kind prompts for a judge
    When I enable the "formation" kind on a gate that has no judge
    Then the judge picker opens
    And cancelling leaves the gate without the "formation" kind
    # syncJudgeKind keeps the "formation" kind in lockstep with whether a judge is wired.

  # ── Routing: pass / fail / block / pushback ─────────────────────────────────

  @ui @file
  Scenario: Pass routes forward, fail routes to a fallback
    Given "gate:pass -> ship:in[0]" and "gate:fail -> research:feedback"
    When the gate evaluates to PASS at run time
    Then the durable ref/projection and exact authorized live value flow unchanged to "ship"
    And redaction evidence is never substituted for that value
    And only the pass-edge traversal is new
    But not down the fail wire

  @ui
  Scenario: An unwired fail output blocks the run
    Given "gate:fail" has no outgoing connection
    When the gate evaluates to FAIL
    Then one aggregate "gate_verdict" records "routePort=fail" and "routedEdges=[]"
    And one stable typed feedback object is recorded with zero deliveries
    And after quiescence "run_blocked" uses "reason=unwired_gate_fail" and "blockScope=gate" with only the Gate id
    And it has empty "openDispatches" and "retryTargets", "resumeAllowed=false", "resumePolicy=new_run_required", and no "nextEpoch"
    And the block overlays the Gate attempt already closed by "gate_verdict" and does not close it again
    And the Gate remains visibly FAIL/blocked while the upstream Formation remains completed

  @ui @file
  Scenario: A fail wire back to an earlier formation is a pushback/revise loop
    Given the gate evaluated "research" output
    And "research"'s entire connected workflow-output frontier is the direct edge into the gate
    And "gate:fail -> research:feedback" (a backward wire)
    And "research:feedback" is an optional "gate_feedback" port with role "retry_control"
    And that edge is the gate's entire fail frontier
    When the gate evaluates to FAIL
    Then pushback is the fail-route action, not another verdict
    And the feedback's identity-only input pointer resolves to "research"'s exact evaluated source attempt
    And the feedback embeds no work ref, payload projection, payload text, or artifact
    And the frozen authoritative inputs must remain live or durably exact
    And a bounded next attempt reuses that attempt's frozen work refs
    And its revised output opens the next gate attempt linked by cycle id, feedback id, prior gate seq, and source attempt
    And no brief or work payload is annotated or replaced
    And no side output, fail fan-out, downstream replay, or non-source pushback occurs

  @ui @file
  Scenario: Non-pushback fail fan-out shares one feedback identity
    Given two explicit fail edges leave the same gate
    And neither edge targets a "retry_control" port
    When the gate evaluates to FAIL
    Then exactly one "gate_feedback" object is recorded for the gate sequence
    And both delivery traversals reference that same feedback id

  # ── The judge: single formation (the classic loop) ──────────────────────────

  @ui @file
  Scenario: Attach a single-formation judge by dragging the gate's judge socket onto a formation
    When I drag the gate's "judge" socket onto formation "review"
    Then two judge connections exist: "gate:judge -> review:in[0]" and "review:out[0] -> gate:judge"
    And both persist "channel=judge" and reserve those Formation endpoints from workflow use
    And the gate gains the "formation" kind
    And the gate card shows it has a judge

  @ui @file
  Scenario: Dropping the judge wire on empty canvas spawns a judge formation in place
    When I drag the gate's "judge" socket onto empty canvas
    Then a new solo formation "Judge" is created at the drop point
    And it is wired as a linear "channel=judge" loop with exclusive endpoints
    # "Just works": the missing piece is created rather than failing.

  @ui @file
  Scenario: Dropping a formation's output onto the judge socket sets the judge return
    Given the gate has no judge yet
    When I drag from "review:out[0]" and release on "gate:judge"
    Then a judge send "gate:judge -> review:in[0]" is auto-created
    And a judge return "review:out[0] -> gate:judge" is created
    And both persist "channel=judge" and never route PortPayload
    And together they form the loop

  @ui
  Scenario: Clicking the judge socket opens the judge picker
    When I click the gate's "judge" socket without dragging
    Then I can pick a NEW judge (solo / peer / orchestrated), an EXISTING formation, or detach

  # ── The judge: a CHAIN of multiple formations (the headline) ────────────────

  @ui @file
  Scenario: A judge can be several formations wired in sequence
    Given a gate with "channel=judge" send "gate:judge -> j1:in[0]"
    When I extend the judge channel with "j1:out[0] -> j2:in[0]" and "j2:out[0] -> j3:in[0]"
    And I set the judge return from "j3:out[0]" onto "gate:judge"
    Then the judge is the chain "j1 → j2 → j3" with "j3" returning the verdict
    And every chain edge persists "channel=judge"
    And all addressed Formation endpoints are exclusive to the linear judge chain
    And the gate still shows a single "formation" check whose entry is "j1"

  @ui @file
  Scenario: Re-pointing the judge return moves the chain's exit without breaking the entry
    Given a judge chain "gate:judge -> j1 -> j2 -> gate:judge"
    When I drag from "j3:out[0]" onto "gate:judge"
    Then the return becomes "j3:out[0] -> gate:judge"
    And the entry "gate:judge -> j1:in[0]" is preserved
    And the resulting judge channel remains linear and endpoint-exclusive

  @cli @file
  Scenario: The CLI expresses single and chained judges explicitly
    When I run "archon gate judge session-search gate --chain j1,j2,j3"
    Then the judge send targets "j1", every edge persists "channel=judge", and "j3" returns to the gate
    And "archon gate judge session-search gate --detach" removes all judge connections and the "formation" kind

  # ── The judge runs to decide ────────────────────────────────────────────────

  @ui
  Scenario: A formation-only gate runs its staffed judge chain before routing
    Given the gate declares only kind "formation"
    And the judge chain's formations have agents assigned
    When the run reaches the gate
    Then the gate enters the "evaluating" state
    And the judge chain executes from its entry
    And each judge "node_started" records "contextEncoding=judge-context-jcs-v1" and the SHA-256 of RFC 8785 canonical "{gateId,gateAttempt,criterion,kinds,evaluatedInput,durableEvaluatedInput,priorResults}" bytes
    And those context bytes have fixed Gate-kind order, judge-chain prior-result order, preserved nested evidence order, no unknown keys, and no trailing newline
    And every strict member result is fsynced as "judge_result" before the next member dispatch
    And each "judge_result" records "resultEncoding=decision-result-jcs-v1" and hashes the exact canonical "{verdict,reason,evidence}" bytes
    And replay rebuilds prior results without rerunning completed judge capture
    And the next member dispatches only while the exact evaluated input remains live or durably exact
    And otherwise recovery records "run_failed" with code "redacted_input_unavailable"
    And when the exit formation finishes, its parsed verdict and evidence become Gate metadata
    And one "gate_kind_result" durably completes the formation kind
    And judge-authored content never becomes downstream work implicitly
    And only the aggregate "gate_verdict" routes pass/fail

  @file
  Scenario: A judge channel cannot branch, join, cross into workflow, repeat a node, or include a Tool
    Given a valid linear judge channel
    When an edit adds any forbidden judge-channel shape or endpoint cross-use
    Then structural validation rejects cross-use and extra producers or consumers
    And executable preflight rejects a non-linear or incomplete chain

  @file
  Scenario: Formation gate kind and complete judge channel agree at execution
    Given a draft may temporarily contain one half or a kind/channel mismatch
    When the board is validated for execution
    Then a complete judge channel exists if and only if the gate kinds include "formation"
    And a hand-edited mismatch is rejected before dispatch

  @ui @file
  Scenario: A judge return must be one strict metadata result
    Given the judge exit returns a missing, malformed, or multiple result
    When the Gate parses the reserved judge return
    Then it appends and fsyncs "judge_attempt_failed" with code "invalid_judge_result"
    And that event completes the judge attempt as failed without ordinary "node_output"
    And exactly one result or failed-attempt completion is accepted for the judge key
    And the Gate is blocked
    And no new Gate or dependent work is dispatched
    And already in-flight and independent work settles and records evidence
    And absent an execution-final event, it then records "run_blocked" with "blockScope=gate", the judge and Gate ids, and no open dispatches or retry targets
    And that block has "resumeAllowed=false" and "resumePolicy=new_run_required"
    And that non-resumable block omits "nextEpoch"
    And resume is rejected; corrected configuration or staffing requires a new run
    And neither pass nor fail is routed

  @ui
  Scenario: An unstaffed formation judge fails preflight, never routes from a stored verdict
    Given the gate has the "formation" kind but its judge has no agents
    When run preflight resolves every declared judge slot
    Then the unassigned slot is "unresolved/agent_unassigned"
    And no "run_started" or Gate evaluation event is appended
    And no pass or fail wire is routed from a stored or default verdict

  # ── Gate lifecycle ──────────────────────────────────────────────────────────

  @ui @file
  Scenario: Duplicate a gate carries its kinds and criterion but no run verdict
    When I duplicate the gate
    Then the copy has the same kinds and criterion, with a fresh id and offset position
    And it has no stored verdict

  @ui @file
  Scenario: Deleting a gate removes its wires too
    When I delete the gate
    Then the gate and all connections touching it (including judge connections) are gone
