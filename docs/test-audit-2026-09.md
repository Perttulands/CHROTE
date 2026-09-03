# Test and CI audit, September 2026

A read-only audit of every test file and every gate. It measures what each costs,
says what each protects, and gives each a verdict. It deletes nothing: the cuts
are a separate piece of work.

## The rule

A test earns its place if it would catch a regression the operator would notice,
or a contract another component relies on.

- Tests of absence are deleted once the removal has shipped.
- Tests that assert CSS text, class names or markup shape without behaviour go.
- Playwright covers journeys, not widgets; a widget is a unit test.
- One test per behaviour; duplicates across unit and Playwright keep the cheaper one.
- A gate that takes longer than the work it protects is restructured or moved to CI only.

Every verdict below is judged against that rule and names which clause it invokes.

## How this was measured

Timings are measured, not estimated, unless a row says otherwise.

| What | Command | Result |
| --- | --- | --- |
| Dashboard unit | `npx vitest run --reporter=verbose` | 5.7s wall, 650 cases, 57 files, 27.1s CPU |
| Dashboard lint | `npm run lint` | 5s |
| Mocked Playwright | `npm test -- --reporter=list` on an alternate Vite port | 76s wall, 98 cases, 25 specs, 282.0s CPU, 4 workers |
| Go | `go test -race -v ./...` | 9s wall cold, 1s fully cached |
| Go build | `go build -trimpath ./cmd/server` | 1s |
| Embedded dashboard build | `scripts/build-embedded-dashboard.sh` | 7s |
| Source contracts | doc-lint, host-neutrality, embedded parity | under 1s each |
| Built-server contract | `scripts/test-built-server-contract.sh` | 4s wall, 2.2s of test |
| Public installer smoke | read, not run; timed from the CI log | 0.4s on the runner |
| CI | `gh run view --log` on the last successful run of the default branch | 346.4s total |

The audit machine has 16 cores. The hosted runner's core count is not recorded in
the CI log, so every projected CI number below is labelled estimated.

Go package times: api 8.657s, proxy 3.202s, scheduled 1.166s, cmd/server 1.115s,
core 1.020s, jsonstrict 1.010s.

## Headline findings

**The tests are not what makes CI slow; the Playwright worker cap is.**
`dashboard/playwright.config.ts` sets `workers: process.env.CI ? 1 : 4`. CI runs
all 98 mocked cases on one worker, serialising 282s of CPU. The same suite takes
76s locally on four workers. Deleting tests helps, but raising the worker count is
the larger and cheaper win. Any report that leads with deletions would be selling
the wrong fix.

**The dashboard step is 69% of CI.** 238.9s of a 346.4s job.

**The `t.Skip`-free Go suite is already cheap** at 9s, and three quarters of what
it does spend is fixed waiting, not work:

- `TestScheduledTmuxRunnerDeliversThroughGuardedPasteAndSubmits` (2.41s) does not
  stub `tmuxSendSleep`, so it really sleeps the 1200ms production settle delay twice.
  The neighbouring send-contract harness already stubs it.
- `TestTerminal_HonoursClientFlowControl` (2.01s) waits a fixed 2s absence window
  for output that never comes.
- The four permission tests that each cost exactly 1.01s re-exec the test binary as
  an unprivileged subprocess so a `chmod 0` directory really denies access. The
  second is not process startup: the child is race-instrumented and Go's race runtime
  sleeps `GORACE atexit_sleep_ms` (default 1000ms) before exiting. The same 1s floor
  is visible on otherwise trivial packages (jsonstrict 1.010s, core 1.020s). Setting
  `GORACE=atexit_sleep_ms=0` in the child environment drops each to about 50ms.

**Two gate faults are local-environment artifacts, not repo bugs.**

- The public installer smoke passes on the runner in 0.4s and the job is green. The
  fault recorded in the epic — exit 1 after PASS while deleting a read-only Go module
  cache — does not reproduce there. The mechanism is the final `rm -rf` of the
  temporary tree that holds the smoke's fake `HOME`; it only fails where something has
  populated a Go module cache under that fake home. The runner uses a prebuilt binary,
  so nothing does.
- `scripts/test-built-server-contract.sh` passes its test in 2.2s but exits 1 on the
  audit machine, because a host-level tmux guard refuses `kill-session` even on the
  script's own private socket. Green in CI in 3.5s.

**`scripts/test-systemd-restart-preserves-tmux.sh` is orphaned.** No workflow, no
document and no validation list references it, yet it is the gate that defends the
golden invariant that a service restart preserves tmux sessions. It is invisible.

**The strategy document is stale.** It claims 57 mocked journeys against an actual
98, and calls the job a five-minute one against a measured 5m46s. The epic's own
estimate of 574 unit cases is really 650, and its grep-derived "absence assertions in
60 files" badly overcounts: most `without …` titles describe behaviour, not removal.

## Go suite

31 files, 259 test functions, 9s wall. Verdicts: 27 delete, 55 merge, 177 keep.

| path | scope | runtime | verdict | reason |
| --- | --- | --- | --- | --- |
| cmd/server/main_test.go | file | pkg 1.115s | keep | Four live runtime contracts. |
| cmd/server/no_access_token_test.go | file | pkg 1.115s | merge | Two tests paying a file of their own; fold into main_test.go. |
| internal/api/api_envelope_contract_test.go | file | 0.02s | merge | Dissolve into the files that own each endpoint. |
| internal/api/beads_test.go | file | 3.1s | keep | Workspace resolution and permission reporting; needs the GORACE fix. |
| internal/api/files_test.go | file | 1.2s | keep | Root confinement and resource shapes. |
| internal/api/health_test.go | file | 0.01s | keep | Collapse to one table. |
| internal/api/integration_test.go | file | 0.16s | delete | Every subtest duplicates a cheaper one or accepts both outcomes. |
| internal/api/launch_test.go | file | 0.01s | keep | Launch cwd resolution. |
| internal/api/scheduled_test.go | file | 2.7s | keep | Stub the settle sleep. |
| internal/api/services_test.go | file | 0.5s | keep | Proxy timeouts and token redaction. |
| internal/api/system_test.go | file | 0.15s | keep | Sampler and history shapes. |
| internal/api/terminal_user_env_test.go | file | 0.01s | merge | Five tests that are one table. |
| internal/api/theme_test.go | file | 0.03s | keep | Theme serving and traversal refusal. |
| internal/api/tmux_launch_test.go | file | 0.05s | keep | Real tmux argv contracts. |
| internal/api/tmux_multiuser_socket_error_test.go | file | 0.02s | keep | Partial-inventory contract. |
| internal/api/tmux_send_contract_test.go | file | 0.05s | keep | Send refusals and pane pinning. |
| internal/api/tmux_send_real_test.go | file | build-tagged | keep | Live tag plus env gates; never compiled by the default run. |
| internal/api/tmux_session_facts_test.go | file | 0.03s | keep | Session fact parsing. |
| internal/api/tmux_session_size_test.go | file | 0.02s | keep | Sizing without pinning. |
| internal/api/tmux_test.go | file | 0.3s | keep | Consolidate the fake installers. |
| internal/core/pathutil_test.go | file | 1.020s pkg | keep | Path confinement. |
| internal/core/response_test.go | file | pkg | keep | Wire shape is the contract. |
| internal/core/session_test.go | file | pkg | keep | Sorting and grouping. |
| internal/core/tmux_bin_test.go | file | pkg | merge | Same package as session_test.go, no isolation earned. |
| internal/jsonstrict/unicode_test.go | file | 1.010s pkg | delete | The package it tests is imported by nothing in production. |
| internal/proxy/origin_test.go | file | pkg | keep | 13 origin edge cases. |
| internal/proxy/sizing_test.go | file | pkg | keep | list-clients parsing decides the sizing seat. |
| internal/proxy/terminal_test.go | file | 3.0s | keep | Attach lifecycle; fix the flow-control window. |
| internal/scheduled/scheduler_test.go | file | 1.166s pkg | keep | Fan-out bounding and delivery. |
| internal/scheduled/schedule_test.go | file | pkg | keep | Same package, no overhead. |
| internal/scheduled/store_test.go | file | pkg | keep | 0770/0660 is the agent-user socket contract. |

### Go: every non-keep test

| path | test | verdict | reason |
| --- | --- | --- | --- |
| cmd/server/main_test.go | TestCORSMiddlewareDefaultDoesNotSetAllowOrigin | merge | Negative row of the exact-origin test. |
| cmd/server/no_access_token_test.go | TestProductionServerHasNoAccessTokenAuthentication | delete | Greps source for removed identifiers; shipped removal. |
| internal/api/api_envelope_contract_test.go | TestAPIEnvelopeContract_FlatHealthEndpointsDoNotUseDataEnvelope | merge | Into health_test.go; /api/version has no consumer. |
| internal/api/api_envelope_contract_test.go | TestAPIEnvelopeContract_BeadsHealthUsesSuccessDataEnvelope | merge | Into beads_test.go. |
| internal/api/beads_test.go | TestBeadsHandler_ListProjectsAllowsConfiguredWorkspaceOutsideRoots | merge | Row of the workspace-inclusion test. |
| internal/api/beads_test.go | TestBeadsHandler_IssuesRejectsInvalidWorkspaceBeforeRunningBd | merge | Pair with the issue-detail twin; neither installs a fake. |
| internal/api/beads_test.go | TestBeadsHandler_IssueDetailRejectsInvalidWorkspaceBeforeRunningBd | merge | Same. |
| internal/api/beads_test.go | TestBeadsHandler_IssuesSupportsExplicitStatusAndLimit | merge | Identical body to the insights twin. |
| internal/api/beads_test.go | TestBeadsHandler_InsightsSupportsExplicitStatusAndLimit | merge | Same. |
| internal/api/beads_test.go | TestBeadsHandler_CheckBeadsDirectoryUnreadableReportsPermissionError | merge | Same fixture as the list-projects permission test; one less re-exec. |
| internal/api/files_test.go | TestFilesHandler_NewFilesHandler | delete | Tautology. |
| internal/api/files_test.go | TestFilesHandler_GetResource_EmptyPath | merge | Row of ListRoot. |
| internal/api/files_test.go | TestFilesHandler_RenameResource_InvalidPath | merge | Pair with the not-allowed twin. |
| internal/api/files_test.go | TestFilesHandler_RenameResource_NotAllowedPath | merge | Same. |
| internal/api/files_test.go | TestFilesHandler_CreateResource_AtRoot | merge | One at-root table with delete and download. |
| internal/api/files_test.go | TestFilesHandler_DeleteResource_AtRoot | merge | Same. |
| internal/api/files_test.go | TestFilesHandler_DownloadFile_AtRoot | merge | Same. |
| internal/api/files_test.go | TestFilesHandler_RegisterRoutes | delete | Asserts only that registration does not panic. |
| internal/api/files_test.go | TestFilesHandlerDownloadRejectsParentReplacedByOutboundSymlink | delete | A same-UID TOCTOU the threat model explicitly disclaims. |
| internal/api/files_test.go | TestFilesHandlerMutationsRejectParentReplacedByOutboundSymlink | delete | Same; retiring both also retires a test-only seam in production code. |
| internal/api/files_test.go | TestFilesHandler_SuccessResponse | delete | JSON round-trip of a struct. |
| internal/api/files_test.go | TestFilesHandler_DirectoryResponse | delete | Same. |
| internal/api/files_test.go | TestFilesHandler_FileInfoResponse | delete | Same. |
| internal/api/health_test.go | TestHealthHandler_Health_ReportsBuildCommit | merge | Row of the health test. |
| internal/api/health_test.go | TestHealthHandler_RegisterRoutes | merge | Drive the health test through the mux instead. |
| internal/api/integration_test.go | TestIntegration_FullAPIRouting | delete | Accepts 200 or 503; reads ambient env; the install gate already covers routing. |
| internal/api/integration_test.go | TestIntegration_APIResponseFormat | delete | Duplicate of core TestWriteJSON. |
| internal/api/integration_test.go | TestIntegration_ErrorHandling | delete | Duplicate of the invalid-name and nuke-header tests. |
| internal/api/scheduled_test.go | TestScheduledTasksAPIRejectsEmptyTargetList | merge | Row of the validation table. |
| internal/api/scheduled_test.go | TestScheduledTasksAPIRejectsLegacySingleTargetWithoutMutation | delete | Absence test of a retired request field. |
| internal/api/scheduled_test.go | TestProductionScheduledServiceUsesEightConcurrentWorkers | delete | Pins a tuning constant; bounding is proven by the fan-out test. |
| internal/api/services_test.go | TestLoadServiceConfigFromEnvDefaults | merge | Pair with the overrides twin. |
| internal/api/services_test.go | TestLoadServiceConfigFromEnvOverridesAndTrims | merge | Same. |
| internal/api/services_test.go | TestServicesHandlerCatalogRedactsTokenAndShowsDegradedContext | merge | Pair with the missing-token twin. |
| internal/api/services_test.go | TestServicesHandlerCatalogShowsMissingTokenAsDegraded | merge | Same. |
| internal/api/services_test.go | TestServicesHandlerTTSFeedStreamsServerSentEvents | merge | Subset of the flush-and-clear test. |
| internal/api/services_test.go | TestServicesHandlerContextIntegrationProxyRoutesUseServerToken | merge | Into the context proxy test. |
| internal/api/services_test.go | TestServicesHandlerContextProxyEscapesNestedPathsWithSpaces | merge | Pair with the browser-encoded twin. |
| internal/api/services_test.go | TestServicesHandlerContextProxyEscapesBrowserEncodedNestedPathsWithSpaces | merge | Same. |
| internal/api/terminal_user_env_test.go | TestValidateTerminalUserEnv_RejectsDuplicateWorkdirKey | merge | Row of one validation table. |
| internal/api/terminal_user_env_test.go | TestValidateTerminalUserEnv_AcceptsOneEntryPerUser | merge | Same. |
| internal/api/terminal_user_env_test.go | TestValidateTerminalUserEnv_RejectsNonAbsoluteSocket | merge | Same. |
| internal/api/terminal_user_env_test.go | TestParseUserValueMap_ResolvesTheSingleEntryPerUser | delete | Trimming already proven by the ordered-users list test. |
| internal/api/theme_test.go | TestThemeHandler_ServesEmbeddedDefaultWhenTheThemeFileIsAbsent | merge | Row of the no-directory test. |
| internal/api/theme_test.go | TestThemeHandler_ArtIsAbsentWithoutAThemeDirectory | merge | Same table. |
| internal/api/theme_test.go | TestThemeHandler_ArtRefusesEverythingOutsideTheArtDirectory | merge | Drive the routed traversal test through the mux. |
| internal/api/tmux_launch_test.go | TestCreateSessionTypesTheRequestedFlagsLine | merge | Into the harness-start test. |
| internal/api/tmux_multiuser_socket_error_test.go | TestTmuxHandler_ListSessionsNamesTheUnixUserWhoseSocketFailed | merge | Identical fixture to the healthy-users test. |
| internal/api/tmux_send_contract_test.go | TestSendToSessionRejectsConflictingQueryAndBodyUnixUsers | merge | Into the explicit-user requirement test. |
| internal/api/tmux_send_contract_test.go | TestSendToSessionRejectsEmptyPayloadAsBadRequest | merge | One refusal table with the multi-pane and stale-pane cases. |
| internal/api/tmux_session_facts_test.go | TestParseSessionsOutputCountsEveryViewerIncludingCHROTEsOwn | merge | Row of the contradicting-appearances table. |
| internal/api/tmux_session_facts_test.go | TestParseSessionsOutputRaisesNoClaimForAnOrdinarySession | merge | Same. |
| internal/api/tmux_session_facts_test.go | TestParseSessionsOutputTreatsAnUnattributableClientAsSilent | merge | Same. |
| internal/api/tmux_session_facts_test.go | TestParseSessionsOutputKeepsOlderShapesClaimFree | delete | The short field shapes exist only in fixtures; production emits the full form. |
| internal/api/tmux_session_facts_test.go | TestSessionInventoryFormatCarriesEveryBadgeFact | merge | Keep only the field-count check. |
| internal/api/tmux_session_facts_test.go | TestOwnedPTYsOwnsNothingWhenProcIsUnreadable | merge | Into the descendants test. |
| internal/api/tmux_session_facts_test.go | TestOwnedPTYsSurvivesACycleInTheProcessTree | delete | The process tree cannot contain a cycle. |
| internal/api/tmux_session_size_test.go | TestCreateSessionHonoursConfiguredCanonicalSize | merge | Row of the sizes-once test. |
| internal/api/tmux_session_size_test.go | TestCanonicalWindowSizeIgnoresUnusableConfiguration | merge | Same. |
| internal/api/tmux_test.go | TestMain | delete | Restates default behaviour. |
| internal/api/tmux_test.go | TestTmuxHandler_NewTmuxHandler | delete | Non-nil check. |
| internal/api/tmux_test.go | TestTmuxHandler_CreateSession_InvalidJSON | merge | One malformed-request table with the fake installed. |
| internal/api/tmux_test.go | TestTmuxHandler_CreateSession_InvalidName | merge | Same. |
| internal/api/tmux_test.go | TestTmuxHandler_DeleteSession_InvalidName | merge | Same. |
| internal/api/tmux_test.go | TestTmuxHandler_RenameSession_InvalidNewName | merge | Same. |
| internal/api/tmux_test.go | TestTmuxHandler_SetMouseModeRejectsInvalidJSON | merge | Same. |
| internal/api/tmux_test.go | TestTmuxHandler_SetMouseModeRejectsMissingEnabled | merge | Same. |
| internal/api/tmux_test.go | TestTmuxHandler_DeleteAllSessions_NoConfirmHeader | delete | Row of the exact-header table. |
| internal/api/tmux_test.go | TestTmuxHandler_RegisterRoutes | delete | No assertion. |
| internal/api/tmux_test.go | TestTmuxHandler_ListSessions_ReturnsValidJSON | merge | Keep only the empty-array-not-null check. |
| internal/api/tmux_test.go | TestTmuxHandler_SoleExplicitSocketUsesConfiguredWorkDir | merge | Into one creation table. |
| internal/api/tmux_test.go | TestTmuxHandler_CreateSessionUsesSelectedUnixUserTarget | merge | Same. |
| internal/api/tmux_test.go | TestTmuxHandler_ListSessionsReturnsOrderedTrimmedTerminalUsers | merge | Into the aggregation test. |
| internal/api/tmux_test.go | TestTmuxHandler_RegisterRoutesWiresMouseMode | merge | Identical argv to the mouse-mode test; drive that through the mux. |
| internal/api/tmux_test.go | TestTmuxHandler_SendToSessionStoresDropAndPastesViaBuffer | merge | Belongs in the send-contract file. |
| internal/core/pathutil_test.go | TestFileExists | delete | Wrapper tautology. |
| internal/core/response_test.go | TestNewSuccessResponse | merge | Into the write test. |
| internal/core/response_test.go | TestNewErrorResponse | merge | Into the write test. |
| internal/core/session_test.go | TestGetGroupPriority | merge | Into the sort test. |
| internal/core/session_test.go | TestGroupSessions | merge | Into the sort test. |
| internal/proxy/terminal_test.go | TestTerminal_ConfiguredOriginIsServed | delete | Already a row in origin_test.go. |
| internal/proxy/terminal_test.go | TestTerminal_AbsentOriginIsServed | delete | Same. |
| internal/proxy/terminal_test.go | TestTerminal_PlainHTTPIsNotServed | delete | Retired transport-era path. |
| internal/proxy/terminal_test.go | TestTerminal_RefusesAnUnknownViewingMode | merge | One refusal table with the next four. |
| internal/proxy/terminal_test.go | TestTerminal_RefusesAMissingSessionName | merge | Same. |
| internal/proxy/terminal_test.go | TestTerminal_RefusesASessionMissingFromTheConfiguredSocket | merge | Same; keep the no-fallback assertion as a row. |
| internal/proxy/terminal_test.go | TestTerminal_ReportsAResolutionFailure | merge | Same. |
| internal/proxy/terminal_test.go | TestTerminal_RefusalClosesWithACloseFrame | merge | Same. |
| internal/proxy/terminal_test.go | TestTerminal_ClosesWhenTheAttachExits | delete | Subsumed by the end-of-attach close-frame test. |
| internal/proxy/terminal_test.go | TestAttachEnv_FallsBackWhenTheInheritedLocaleIsNotUTF8 | merge | Into the terminal-type test. |
| internal/scheduled/scheduler_test.go | TestTargetRunJSONReportsSubmitKeyDispatchWithoutSubmittedClaim | delete | Absence test of a shipped rename. |
| internal/scheduled/scheduler_test.go | TestTargetRunJSONMigratesLegacySubmissionClaim | merge | Persisted-file migration belongs in store_test.go. |
| internal/scheduled/scheduler_test.go | TestStoreReadsLegacySingleTargetDocument | merge | Same. |
| internal/scheduled/scheduler_test.go | TestTaskJSONWritesCurrentTargetsSchema | merge | Same. |
| internal/scheduled/store_test.go | TestStoreTryLockKeepsFreshLock | merge | Pair with the stale-lock twin. |
| internal/scheduled/store_test.go | TestStoreTryLockReclaimsStaleLock | merge | Same. |

### Go: looks stupid, protects something real

These stay untouched whatever their shape, because they defend the invariant that
tmux owns live sessions and must never be terminated implicitly.

- `TestRuntimeRoutesCanDisableSystemHistorySampler` — proves the runtime routes make
  zero background tmux calls.
- `TestTmuxHandler_DeleteAllSessionsRequiresExactNukeConfirmationHeader` — no tmux
  call at all unless the confirmation header is byte-exact.
- `TestTmuxHandler_CreateSessionReportsDuplicateNameClearly` — a duplicate create
  never kills the session that already holds the name.
- `TestTmuxHandler_CreateOwnedTmuxSessionCleansAmbiguousCreationByMarker`,
  `…CleansMalformedIDByOwnedName`, `…RefusesToCleanUnownedName`,
  `TestTmuxHandler_CleanupOwnedTmuxSessionJoinsKillFailure` — every cleanup kill is
  guarded by an ownership token, and a mismatch means no kill.
- `TestCreateSessionKeepsTheSessionWhenTheCommandCannotBeSent` — a harness that fails
  to start never takes the session with it.
- `TestCreateSessionSizesOnceWithoutPinningTheWindow` and `…CleansUpWhenSizingFails`.
- `TestSendToSessionRejectsTmuxPrefixMatchBeforePersisting` — tmux target prefixes
  match longer names; without this, a send lands in the wrong session.
- `TestListSessionsRunsOneTmuxCommandPerSocket` — one list per socket, no sweeps.
- `TestTerminal_ViewingModeSelectsTheAttachFlags` and
  `TestTerminal_ClientDisconnectEndsTheAttach` — never detach other viewers, never
  leak a tmux client.
- `TestCleanupPrivateTmuxSessionsRetainsRootOnAmbiguousClientFailure` — the live
  fixture refuses to remove a root it cannot prove it owns.
- `TestResponseWriterAllowsResponseControllerWriteDeadline` — without the unwrap, the
  server write timeout silently kills the long-lived event feed.

Only the build-tagged live file touches a real tmux, on a private socket with
exact-name cleanup. Ten tests run handlers with no fake installed and read ambient
socket configuration; validation short-circuits today, but the merged tables should
install the fake and clear the environment.

## Dashboard unit suite

57 files, 650 cases, 5.7s wall. Verdicts: 53 cases delete, 80 merge, 517 keep.
Runtimes are per file, since per-case times are mostly under 5ms.

| path | runtime | cases | verdict |
| --- | --- | --- | --- |
| src/components/FilesView.test.tsx | 3846ms | 23 | keep |
| src/terminal/terminalSession.test.ts | 1934ms | 22 | keep |
| src/components/TerminalFilesPanel.test.tsx | 1774ms | 10 | keep |
| src/components/TabBar.test.tsx | 1738ms | 19 | keep |
| src/components/ScheduledTasksView.test.tsx | 1652ms | 9 | keep |
| src/components/TerminalWindow.test.tsx | 1560ms | 39 | keep |
| src/components/Launcher.test.tsx | 1367ms | 18 | keep |
| src/components/ServicesView/ServicesView.test.tsx | 1306ms | 7 | keep |
| src/components/SystemStatusView/SystemStatusView.test.tsx | 1106ms | 19 | keep |
| src/components/SendDrawer.test.tsx | 1014ms | 13 | keep |
| src/components/FloatingModal.test.tsx | 976ms | 5 | keep |
| src/components/SettingsView.test.tsx | 812ms | 11 | keep |
| src/components/BeadsView.test.tsx | 737ms | 5 | keep |
| src/components/TerminalWorkspaceDock.test.tsx | 637ms | 9 | keep |
| src/components/SessionItem.test.tsx | 618ms | 17 | keep |
| src/components/FileViewer.test.tsx | 582ms | 37 | keep |
| src/context/useSendToSession.test.ts | 578ms | 11 | keep |
| src/components/SessionPanel.test.tsx | 536ms | 6 | keep |
| src/components/FlagPanel.test.tsx | 475ms | 9 | keep |
| src/context/SessionContext.test.ts | 435ms | 15 | keep |
| src/components/TerminalArea.test.tsx | 428ms | 5 | keep |
| src/components/Menu.test.tsx | 425ms | 5 | keep |
| src/components/FileTree.test.tsx | 378ms | 3 | keep |
| src/context/useSessionsPoll.test.ts | 271ms | 18 | keep |
| src/App.test.tsx | 268ms | 34 | keep |
| src/components/EmptyWindow.test.tsx | 236ms | 2 | keep |
| src/components/FolderPickerModal.test.tsx | 205ms | 1 | keep |
| src/context/useWorkspaceLayouts.test.ts | 188ms | 56 | keep |
| src/components/StatusLine.test.tsx | 143ms | 3 | keep |
| src/components/DockPanelToggle.test.tsx | 109ms | 2 | delete |
| src/hooks/useKeyboardShortcuts.test.ts | 104ms | 5 | keep |
| src/keys/KeysPanel.test.tsx | 93ms | 4 | keep |
| src/components/FilesView/pinnedPaths.test.tsx | 63ms | 4 | keep |
| src/components/Sheet.test.tsx | 43ms | 4 | keep |
| src/keys/KeyEcho.test.tsx | 42ms | 3 | keep |
| src/components/TerminalSurface.test.tsx | 39ms | 4 | keep |
| src/utils/clipboard.test.ts | 39ms | 3 | keep |
| src/keys/chords.test.ts | 37ms | 24 | keep |
| src/components/TerminalPool.test.tsx | 35ms | 7 | keep |
| src/main.test.tsx | 32ms | 1 | delete |
| src/components/DismissiblePanel.test.tsx | 24ms | 2 | keep |
| src/components/FilesView/fileService.test.ts | 22ms | 7 | keep |
| src/components/harnessMarks.test.tsx | 20ms | 4 | keep |
| src/services/servicesClient.test.ts | 18ms | 4 | keep |
| src/components/scheduledSchedule.test.ts | 17ms | 6 | keep |
| src/liveSessionCleanup.test.ts | 16ms | 4 | keep |
| src/chunkReloadRecovery.test.ts | 11ms | 6 | keep |
| src/components/FilesView/openFilesModel.test.ts | 10ms | 10 | keep |
| src/theme/theme.test.ts | 10ms | 16 | keep |
| src/components/workspaceFilesState.test.ts | 9ms | 10 | keep |
| src/components/launchFlags.test.ts | 9ms | 13 | keep |
| src/terminal/tileState.test.ts | 8ms | 31 | keep |
| src/terminal/ttydProtocol.test.ts | 8ms | 13 | keep |
| src/types.test.ts | 5ms | 25 | keep |
| src/components/FilesView/filesWorkbenchState.test.ts | 4ms | 3 | keep |
| src/featureFlags.test.ts | 2ms | 2 | keep |
| src/hooks/useViewportMenuPosition.test.ts | 1ms | 2 | keep |

### Dashboard unit: every deletion

| path | test name | verdict | reason |
| --- | --- | --- | --- |
| src/components/DockPanelToggle.test.tsx | file (both it.each rows) | delete | Class names, a data attribute and chevron text; and the component it tests has no consumer anywhere in the source. |
| src/main.test.tsx | mounts CHROTE directly without checking or requesting an access token | delete | Absence test of the retired token gate; the mount is exercised by every browser spec. |
| src/components/BeadsView.test.tsx | hides legacy Patrols UI from the visible Beads status strip | delete | Absence test of a shipped removal; the control exists in no source file. |
| src/components/FilesView.test.tsx | copies current folder path through the browser fallback when Clipboard API is unavailable | delete | Duplicate of the cheaper clipboard util test that owns the behaviour. |
| src/components/FilesView.test.tsx | clamps the background context menu inside the viewport near the bottom-right edge | delete | Duplicate of the viewport-menu hook test; its target element is rendered only by an orphaned component. |
| src/components/FileViewer.test.tsx | renders editable Markdown source as an unwrapped full-viewport source surface | delete | Attribute and class only; the regression it names is caught by the browser spec that measures real boxes. |
| src/components/SessionItem.test.tsx | opens the row menu without a location chip in front of the name | delete | Absence test of a shipped removal; neither the class nor the label exists in source. |
| src/components/SessionItem.test.tsx | treats legacy persistence metadata as an ordinary session | delete | Absence test of a retired persistence feature; the positive assertions are covered elsewhere. |
| src/components/SessionItem.test.tsx | clears the pending long-press timer on unmount before its state callback can run | delete | Asserts a timer count; the operator cannot observe the difference. |
| src/components/SessionPanel.test.tsx | keeps bulk destruction out of the primary Session panel | delete | Absence test of a shipped move; the positive halves live in Settings and the nuke journey. |
| src/components/SessionPanel.test.tsx | carries only the title, the launcher and close in its header | delete | Absence test of a shipped removal; neither label exists in source. |
| src/components/TerminalWindow.test.tsx | marks the active tag and offers no cycle controls beside it | delete | Absence of controls cut in September; the active-mark half is a class check covered elsewhere. |
| src/components/TerminalWindow.test.tsx | does not hide a Send action behind ctrl-click on an attached session tag | delete | Absence of a retired modifier-click affordance. |
| src/components/TerminalWindow.test.tsx | opens the live session working directory | delete | Exact duplicate of the clicked-tag actions test. |
| src/components/TerminalWindow.test.tsx | shares terminal header width equally between every tag without crowding controls | delete | Reads two stylesheets as text and regexes flex and overflow rules. |
| src/components/TerminalWindow.test.tsx | paints every tile on the same surface, whatever its stored window index | delete | Absence of retired custom properties plus CSS text matching. |
| src/components/TerminalWindow.test.tsx | states a detached tile in the middle of the frame, with plain outline controls | delete | Retired class names plus CSS text regexes; the controls are asserted by the ended-binding test. |
| src/components/TerminalWindow.test.tsx | dials again for a session another client is attached to, because that no longer evicts them | delete | Same path and assertions as the lost-connection test; the extra input changes nothing the tile reads. |
| src/context/useWorkspaceLayouts.test.ts | removes the viewport listener on unmount | delete | Tautological; a leaked listener would not throw either. |
| src/context/useWorkspaceLayouts.test.ts | removing the last session sets activeSession to null | delete | Exact duplicate of the bound-list removal test. |
| src/context/useWorkspaceLayouts.test.ts | preserves existing window data when increasing count | delete | Subsumed by the shrink, re-expand and reload test. |
| src/context/useWorkspaceLayouts.test.ts | defaults to three visible workspaces | delete | Duplicate of the canonical-count test in the types suite. |
| src/context/useWorkspaceLayouts.test.ts | leaves visibility and reveal state unchanged for malformed or noncanonical targets | delete | Validates identifiers only the store itself can generate; no operator path reaches it. |
| src/App.test.tsx | registers only pointer dragging with an 8px activation threshold | delete | Asserts a configuration literal captured from a mocked provider: a test of the mock. |
| src/App.test.tsx | rejects malformed drag starts (9 it.each cases) | delete | Validates payloads only the product's own draggables emit. |
| src/App.test.tsx | does not mutate for malformed drag ends (12 it.each cases) | delete | Same. |
| src/App.test.tsx | ignores a seam drop naming a workspace that is not a terminal one | delete | The seam zone can only name its own terminal workspace. |
| src/terminal/terminalSession.test.ts | answers xterm with false for the leader and true for the keys the shell owns | delete | Calls the handler by hand through a prototype spy; the real path is proven by the neighbouring test. |
| src/terminal/tileState.test.ts | is Lost even when another client is attached, because dialling no longer evicts anyone | delete | Byte-identical input and output to the plain lost-connection case; absence of a retired rule. |
| src/terminal/tileState.test.ts | never holds a verdict against the tile own open connection | delete | The helper compares by identity, so this restates the neighbouring test. |
| src/theme/theme.test.ts | leaves no property behind that a theme no longer defines | delete | Names three retired custom properties, and the function never removes properties at all, so it can fail only if the names are re-added. |

Note on the last row: read on its own this looks like a valuable guard against a theme
switch leaving ghost variables. It is not, because the implementation never removes a
property; the assertion is true by construction. That is worth stating because the
same title in a codebase that did clear properties would be a keep.

### Dashboard unit: merges

80 cases fold into a neighbour rather than disappearing. The largest clusters:

- `src/components/TerminalWindow.test.tsx` — 10 cases fold into sibling tests
  (drag feedback, tag menu, harness marks, tag press, cwd routing).
- `src/context/useWorkspaceLayouts.test.ts` — 15 cases fold, including five
  one-line count-clamping tests that are one table.
- `src/components/TabBar.test.tsx` — 6 navigation and menu cases fold into two.
- `src/types.test.ts` — 11 cases fold into existing tables.
- `src/context/useSessionsPoll.test.ts` — 5 cases fold into the coalescing and
  authoritative-state tests.
- `src/keys/chords.test.ts` — 4 twins fold.
- `src/terminal/tileState.test.ts` — 4 cases fold.
- The remainder are pairs across FilesView, SessionContext, workspaceFilesState,
  TerminalWorkspaceDock, ttydProtocol, servicesClient, featureFlags, KeyEcho,
  chunkReloadRecovery, TerminalPool and TerminalArea.

Alongside these, roughly 40 individual assertions inside otherwise-keep tests are
class-name, CSS-text or retired-label checks and should be cut without removing the
test: the audit lists them per test in the working notes and they are cheap to strip.

### Dashboard unit: looks stupid, protects something real

- `src/liveSessionCleanup.test.ts` tests a file that lives under the browser-test
  directory, which reads as testing the harness. Its first two cases are a keep: the
  ledger they prove is what guarantees a live smoke deletes exactly the tmux sessions
  it created and never drops an unconfirmed deletion. That is the golden invariant.
  Its last two cases read the live spec files as *text* and assert source substrings,
  including one pinning a shipped removal. Those are lint rules wearing a test's
  clothes; if the intent is worth keeping they belong in a source contract script,
  not in the unit suite.
- `src/components/DismissiblePanel.test.tsx` looks like nesting and z-order trivia.
  It protects a panel being swallowed by its own dismiss layer, which the test
  environment cannot hit-test, so stacking order is the only available proxy.
- `src/components/EmptyWindow.test.tsx` asserts a background image. That assertion is
  the behaviour: the slot index wraps deterministically so a tile shows the same
  picture on every device.
- `src/App.test.tsx` "uses one reset path for drag cancel" asserts a class. That class
  lifts pointer events off the terminal; if it sticks, every terminal goes dead to the
  mouse.
- `src/components/TerminalArea.test.tsx` "draws no seam between tiles while nothing is
  being dragged" reads as an absence test. It is not: the seam is a live conditional,
  and the positive half is a browser journey. Keep.
- `src/components/SessionItem.test.tsx` "cancels the pending touch pointer sensor when
  long-press opens session actions" looks like drag-library plumbing; it protects the
  phone bug where the menu opens and the row starts dragging at once.
- `src/terminal/terminalSession.test.ts` "never fits a terminal that is off screen"
  and "measures an emoji the two columns tmux lays out for it" both look fussy. The
  first prevents a zero-size fit resizing a shared tmux window for every viewer; the
  second keeps the width table in agreement with tmux, without which every character
  after an emoji sits one cell off.
- `src/components/TabBar.test.tsx` "claims only the sessions the visible windows are
  showing" matters because a claim resizes the tmux window for everyone watching.
- `src/types.test.ts` is 25 runtime parsers and guards, not type-level assertions;
  it stays.

## Mocked Playwright suite

25 specs, 98 cases, 76s wall on four workers, 282.0s CPU. Retries are 0 and there
are two projects: desktop Chrome (93 cases) and one phone-emulation project (5).
Verdicts across all 104 browser cases including the contract and live specs:
26 keep, 39 delete, 24 merge, 14 move.

| path | runtime | cases | verdict |
| --- | --- | --- | --- |
| tests/tile-states.spec.ts | 28.2s | 10 | keep, 10 to 3 |
| tests/dashboard.spec.ts | 26.7s | 7 | merge into terminal-sidecar |
| tests/filebrowser.spec.ts | 22.2s | 10 | keep, 10 to 1 |
| tests/dual-workspace.spec.ts | 17.9s | 4 | delete |
| tests/send-drawer.spec.ts | 17.4s | 5 | keep, 5 to 1 |
| tests/drag-lifecycle.spec.ts | 17.1s | 4 | keep, 4 to 2 |
| tests/presets.spec.ts | 14.5s | 3 | keep, 3 to 1 |
| tests/terminal-selection.spec.ts | 14.1s | 5 | keep, 5 to 2 |
| tests/keys.spec.ts | 11.8s | 5 | keep, 5 to 1 |
| tests/mobile.spec.ts | 11.2s | 5 | keep, 5 to 1 |
| tests/terminal-sidecar.spec.ts | 10.9s | 4 | keep, 4 to 1 |
| tests/session-context-menu.spec.ts | 10.4s | 4 | keep, 4 to 1 |
| tests/settings.spec.ts | 9.8s | 4 | delete after one merge |
| tests/beads.spec.ts | 9.5s | 4 | delete |
| tests/launcher.spec.ts | 8.5s | 4 | keep, 4 to 2 |
| tests/core-views.spec.ts | 7.9s | 3 | delete |
| tests/terminal-retention.spec.ts | 7.8s | 2 | keep |
| tests/floating-modal.spec.ts | 7.0s | 3 | keep, 3 to 2 |
| tests/nuke.spec.ts | 6.6s | 2 | delete after moves |
| tests/session-tag-click.spec.ts | 5.1s | 3 | keep, 3 to 1 |
| tests/terminal-fit.spec.ts | 4.9s | 2 | keep, 2 to 1 |
| tests/system-status.spec.ts | 4.4s | 2 | delete after one move |
| tests/rename-propagation.spec.ts | 3.8s | 1 | merge into session-context-menu |
| tests/terminal-links.spec.ts | 2.4s | 1 | keep |
| tests/error-states.spec.ts | 1.9s | 1 | delete |
| tests/contract/built-server.spec.ts | 2.2s | 1 | keep |
| tests/integration/real-backend.spec.ts | opt-in | 1 | delete |
| tests/integration/terminal-interactions-live.spec.ts | opt-in | 1 | move to the mocked suite |
| tests/integration/terminal-pool.spec.ts | opt-in | 2 | keep, 2 to 1 |
| tests/integration/terminal-sizing.spec.ts | opt-in | 1 | keep |

### Playwright: every deletion

| path | test name | verdict | reason |
| --- | --- | --- | --- |
| tests/beads.spec.ts | should refresh data when clicking refresh | delete | Asserts the view is still visible after the mock returns identical data: a test of the mock. |
| tests/core-views.spec.ts | opens Scheduled and runs the selected task now | delete | Duplicate of the cheaper Scheduled view unit test. |
| tests/core-views.spec.ts | opens Services and enqueues a TTS message | delete | Duplicate of the Services view and services client unit tests. |
| tests/dashboard.spec.ts | offers the cut controls nowhere and the moved ones in the tab menu | delete | Pure shipped-removal absence: five selectors with no hits in source. |
| tests/dashboard.spec.ts | should switch active tag on click | delete | Duplicate of the session-tag-click spec, which also proves the surface swap. |
| tests/dashboard.spec.ts | opens as a left sheet on an unassigned session and closes from its header | delete | Duplicate of the floating-modal spec. |
| tests/dashboard.spec.ts | should filter sessions by name | delete | Widget behaviour owned by the session panel unit test. |
| tests/drag-lifecycle.spec.ts | sub-8px movement stays inactive and a real touch pointer drag activates from the whole row | delete | Threshold is a unit test; the touch half uses synthesized events and asserts classes. |
| tests/dual-workspace.spec.ts | clicking Terminal 2 tab shows terminal2 and hides terminal1 | delete | Trivial visibility toggle proven by every spec that clicks the tab. |
| tests/dual-workspace.spec.ts | binding same session cross-workspace moves it (dedup) | delete | Duplicate of the workspace-layout reducer test. |
| tests/dual-workspace.spec.ts | switching between Terminal 1 and 2 preserves each workspace state | delete | The retention spec proves the stronger property, that the connection survives. |
| tests/dual-workspace.spec.ts | both workspaces persist independently after reload | delete | Duplicate of the reducer storage round-trip. |
| tests/error-states.spec.ts | states the failure on the status line when session creation returns 500 | delete | Both halves are unit-owned, and one selector is a shipped removal. |
| tests/filebrowser.spec.ts | should show context menu on right-click | delete | A z-index comparison; stacking is owned by the dismissible-panel unit test. |
| tests/filebrowser.spec.ts | should sort by column headers | delete | Asserts an active class and never checks the resulting order. |
| tests/filebrowser.spec.ts | should open a text file in the editor pane | delete | Duplicate of the Files view editor cases. |
| tests/filebrowser.spec.ts | should switch to Files tab and back to Terminal | delete | Tab toggle; the closed-by-default state is a unit test. |
| tests/keys.spec.ts | a chord that fires echoes its key caps, and an unregistered one does not | delete | Duplicate of two key-echo unit tests. |
| tests/keys.spec.ts | the keys panel is the registry, searched on either column and run from its rows | delete | Duplicate of three keys-panel unit tests. |
| tests/launcher.spec.ts | the Sessions plus opens the same launcher | delete | Duplicate of the session panel and launcher unit tests. |
| tests/launcher.spec.ts | the flags catalogue writes the line the session is launched with | delete | Duplicate of two launcher unit tests. |
| tests/mobile.spec.ts | at 768px wide, mobile layout shows | delete | Asserts a class set from a JavaScript flag; the breakpoint is a reducer test. |
| tests/send-drawer.spec.ts | retargets through the picker and pastes without submitting on Shift+Enter | delete | Duplicate of two send-drawer unit tests. |
| tests/send-drawer.spec.ts | keeps a refused send in the drawer, with the host's own words | delete | Duplicate of the send-drawer unit test. |
| tests/send-drawer.spec.ts | the session row and the tile open the same drawer | delete | Duplicate of the session-item and send-drawer unit tests. |
| tests/session-context-menu.spec.ts | right-click on session item opens context menu | delete | Duplicate of the session-item context menu test. |
| tests/session-context-menu.spec.ts | mobile long-press (500ms) opens context menu | delete | Synthesized touch events plus a fixed sleep, not a real gesture; three unit cases cover it. |
| tests/session-tag-click.spec.ts | leaves the remove cross removing the binding rather than switching to it | delete | Duplicate of two terminal-window unit tests. |
| tests/settings.spec.ts | should switch to Settings view | delete | Tab toggle. |
| tests/settings.spec.ts | offers no theme, tmux appearance or badge colour controls | delete | Shipped-removal absence: four selectors with no hits in source. |
| tests/settings.spec.ts | paints the document in the palette the host served | delete | Asserts the mock theme payload round-trips; the theme unit test owns the function. |
| tests/system-status.spec.ts | scrubs all rows to one hovered moment | delete | Duplicate of the system status view unit test. |
| tests/tile-states.spec.ts | a poll that fails does not take back an Ended verdict or offer Claim on a session that is gone | delete | Duplicate of a tile-state and a session-context unit test. |
| tests/tile-states.spec.ts | a partial outage does not take back an Ended verdict under the user that failed | delete | Duplicate of a tile-state unit test. |
| tests/tile-states.spec.ts | a detached tile states itself in the middle of the frame it is preserving, with plain outline controls | delete | Centring is CSS geometry, one selector is a shipped removal, and a unit test carries the same title. |
| tests/tile-states.spec.ts | a partial outage ends only the bindings it can speak for | delete | Duplicate of a tile-state and a poll unit test. |
| tests/tile-states.spec.ts | claiming a session that is already on screen takes the size without redialling | delete | Duplicate of a terminal-session unit test; the rest is a tag-menu widget. |
| tests/terminal-sidecar.spec.ts | opens Sessions and focuses its filter when slash is pressed while closed | delete | Same title and behaviour as the keyboard-shortcut unit test. |
| tests/terminal-sidecar.spec.ts | peeks an attached session from the row and marks the row the focused tile shows | delete | Every assertion is a class or chip check owned by session-item unit tests; the peek itself is the floating-modal spec. |
| tests/integration/real-backend.spec.ts | GET sessions connection and contract | delete | Never runs in CI; the built-server contract already exercises a real session, and the Go handler owns the shape. |

### Playwright: moves (widget specs that belong in the unit suite)

| path | test name | destination |
| --- | --- | --- |
| tests/beads.spec.ts | should open with the first discovered project selected | BeadsView unit test |
| tests/beads.spec.ts | should switch to Kanban view when clicking Kanban tab | BeadsView unit test |
| tests/beads.spec.ts | should display error message when API fails | BeadsView unit test |
| tests/core-views.spec.ts | opens Dashboard Help from the help menu | a new Help view unit test |
| tests/filebrowser.spec.ts | should retry loading on retry button click | FilesView unit test |
| tests/filebrowser.spec.ts | should navigate into folder on double-click | FilesView unit test (its overflow tail stays in the browser) |
| tests/filebrowser.spec.ts | should switch between list and grid view | FilesView unit test (no unit coverage today) |
| tests/filebrowser.spec.ts | should filter files by search | FilesView unit test |
| tests/filebrowser.spec.ts | should upload files to the current folder | FilesView unit test |
| tests/mobile.spec.ts | clicking hamburger opens mobile-nav-dropdown with all tabs | TabBar unit test |
| tests/nuke.spec.ts | the Nuke All button confirms in place and names what is preserved | SettingsView unit test |
| tests/nuke.spec.ts | the second press sends DELETE with the confirmation header | SettingsView unit test |
| tests/system-status.spec.ts | keeps status history warm before the Server tab is active | App unit test |
| tests/integration/terminal-interactions-live.spec.ts | desktop keeps terminal input native while exposing visible assignment controls | the mocked selection spec; it needs no live backend |

### Playwright: merge map

- `terminal-sidecar.spec.ts` absorbs the dashboard spec's Sessions-across-tabs journey
  and its Files peek, and folds its own narrow-screen case in via a viewport resize.
- `terminal-retention.spec.ts` absorbs the dashboard spec's chord grow and shrink.
- `session-context-menu.spec.ts` absorbs `rename-propagation.spec.ts` and its own kill
  case into one rename-then-kill journey.
- `terminal-selection.spec.ts` collapses to two and absorbs the native context-menu
  check from the live spec.
- `tile-states.spec.ts` collapses to three: die-while-viewed, reload-then-restart, and
  one page holding both a lost tile and a taken-over tile.
- keys, terminal-fit, presets, mobile, send-drawer, session-tag-click and
  floating-modal each collapse to one or two cases sharing a page.
- `contract/built-server.spec.ts` absorbs the served-from-host font check.

### Playwright: looks stupid, protects something real

Browser-only behaviour is the strongest reason to keep a browser test. These cannot
move down a layer:

- Real font metrics deciding the terminal grid (terminal-fit).
- Link hit-testing on real cell geometry (terminal-links) — the only coverage of the
  link addon anywhere.
- Granted clipboard permission and the fallback copy path in a real document
  (terminal-selection) — the plain-HTTP case is the one that matters on a private
  network origin.
- Real socket close codes, redial on visibility change, and connection counts across
  DOM reparenting (tile-states, terminal-retention).
- Real pointer drags and drifts through the drag library onto real tile and seam
  geometry (drag-lifecycle, session-tag-click). A 12px drift swallowing a click is
  exactly the reported regression.
- Container queries and grid layout measured by bounding boxes (launcher, send-drawer,
  floating-modal, terminal-sidecar, and the Markdown source viewport case).
- Focus surviving mount (the rename journey).
- Key routing between the document listener and the terminal's hidden textarea, read
  back on the pty side (keys).

### Harness findings

- The browser fixture fails any test that leaves an API route unmocked or logs a
  console error. That is a good guard and should stay.
- Two files mock the same three routes; one silently wins.
- The shared helper module exports nine symbols and only two are imported. Seven are
  dead, and five specs each carry a private copy of the same drag helper.
- Fifteen fixed sleeps remain, four of them 1.6s in one spec. Those are the cheapest
  seconds in the suite to reclaim.
- Three specs are effectively testing the mock rather than the product; they are in
  the deletion table above.

## Gates

| gate | runtime | in CI | verdict | reason |
| --- | --- | --- | --- | --- |
| gofmt and go vet | 12.7s in CI | yes | keep | Cheap, catches what tests do not. |
| go test -race | 9s local, 35.1s in CI | yes | keep | Restructure the three fixed waits, not the gate. |
| vitest | 5.7s | yes | keep | The cheapest layer; most content should live here. |
| eslint | 5s | yes | keep | Cheap. |
| mocked Playwright | 76s local, about 215s in CI | yes | keep, restructured | Raise the CI worker count and cut to journeys. |
| scripts/check-embedded-dashboard.py | under 1s | yes | keep | Proves the embedded bundle matches tracked sources. |
| scripts/doc-lint.py | under 1s | yes | keep | Frontmatter, shipped views and link integrity. |
| scripts/host-neutrality.py | under 1s | yes | keep | Enforces the boundary that keeps this repository publishable. |
| scripts/test-built-server-contract.sh | 4s local, 3.5s in CI | yes | keep | The only gate that exercises the built binary end to end. |
| scripts/test-public-install.sh | 0.4s in CI | yes | keep | Covers the installed product; unconditional for good reason. |
| scripts/test-systemd-restart-preserves-tmux.sh | not measured | no | keep, but wire it up | Defends the golden invariant and is referenced by nothing; needs a home in the strategy document and an operator-run entry. |
| live Playwright (5 cases) | not measured | no | keep, reduced to 2 | Opt-in and real-backend; one of the four files should move to the mocked suite and one should go. |
| weekly dependency scans | not measured | scheduled only | keep | Correctly excluded from push runs. |

## CI

Measured from the last successful run on the default branch. One job, `quality`,
running everything serially with an 8-minute timeout.

| step | measured | proposed job | note |
| --- | --- | --- | --- |
| Set up job | 1.2s | all | Per-job fixed cost, paid once per job after a split. |
| Checkout | 1.2s | all | |
| Set up Go | 10.8s | build, go | |
| Set up Node | 6.8s | all | |
| Install dashboard and Chromium | 25.8s | browser, contracts | The unit and Go jobs do not need a browser and should not install one. |
| Build embedded dashboard and server | 8.8s | build | Publishes the bundle and binary as artifacts. |
| Go format and vet | 12.7s | go | |
| Go race tests | 35.1s | go | Estimated 28s after the three fixed waits are removed. |
| Dashboard unit, lint, and browser tests | 238.9s | split three ways | 69% of the job. Unit and lint are seconds; the browser suite is the rest. |
| Source contracts | 0.5s | contracts | |
| Built server browser and API contract | 3.5s | contracts | |
| Public installer smoke | 0.4s | contracts | Passes; the documented cleanup fault does not reproduce here. |
| Post steps and completion | 0.7s | all | |
| **Total** | **346.4s** | | |

### Redundancy with local gates

None of the CI steps is redundant with a local gate in the sense of being safely
droppable: the local gates and the CI steps are the same commands, and the point of
CI is that they run on a clean machine. The genuine redundancy is *inside* the
suites, and it is catalogued above.

### Proposed parallel jobs

One hard constraint shapes any split: the dashboard bundle is embedded into the Go
binary with a compile-time directive, and the built bundle is not tracked. Every Go
job therefore needs the dashboard build first, either as an artifact or by rebuilding.

| job | depends on | contents | estimated |
| --- | --- | --- | --- |
| build | — | Node and Go setup, dashboard install, embedded bundle, server binary; uploads both | 55s |
| go | build | gofmt, vet, race tests against the downloaded bundle | 60s |
| unit | — | dashboard install without a browser, vitest, eslint | 50s |
| browser | — | dashboard install with a browser, mocked Playwright at the runner's real worker count | 105s |
| contracts | build | source contracts, built-server contract, installer smoke | 45s |

Estimated critical path about 115s against a measured 346.4s.

All five estimates are extrapolations from local timings; the hosted runner's core
count is not recorded in the log, so the browser job's figure is the least certain.

### The cheapest change of all

If only one thing is done, remove the CI worker cap in the browser configuration.
That single line serialises 282s of measured CPU. It requires no test to be deleted
and no job to be split.

## Summary

| suite | files | cases | keep | delete | merge | move |
| --- | --- | --- | --- | --- | --- | --- |
| Go | 31 | 259 | 177 | 27 | 55 | 0 |
| Dashboard unit | 57 | 650 | 517 | 53 | 80 | 0 |
| Playwright (all modes) | 29 | 104 | 26 | 39 | 24 | 14 |
| Gates | 13 | — | 13 | 0 | 0 | 0 |
| **Total** | **117** | **1013** | **733** | **119** | **159** | **14** |

Deleting 119 cases and folding 159 more removes about 27% of the test population.

### Estimated time saved

| change | measured basis | estimated saving |
| --- | --- | --- |
| Remove the CI browser worker cap | 282s CPU serialised at 1 worker; 76s at 4 locally | about 150s per CI run |
| Cut the browser suite from 98 to about 24 cases | 2.88s measured mean per case | about 155s per CI run at 1 worker, about 55s once parallel |
| Three fixed waits in the Go suite | 2.41s, 2.01s and 4 x 1.01s measured | about 7s per run |
| Unit deletions and merges | 5.7s wall for the whole suite | under 2s; not worth doing for speed |
| Split into five parallel jobs | 346.4s measured serial total | about 230s per CI run |

Taken together, an estimated 200s per push on a single job, or a critical path of
roughly 115s if the job is split: from 5m46s to under 2m.

The honest ordering is worth stating plainly. The unit suite costs 5.7 seconds and
deleting a third of it saves nothing measurable; the case for those deletions is
maintenance clarity, not speed. The time is in the browser suite and in the CI
configuration, and the configuration is the larger half of it.

### Contracts that must land before anything is deleted

- The nuke confirmation header must be pinned in a dashboard unit test before the
  browser case that currently pins it is removed.
- The preset ceiling announcement needs a reducer test.
- The served-from-host font check must move into the built-server contract before the
  settings spec goes.
- The four Go permission tests must keep at least one unprivileged re-exec; they are
  vacuous if run as root.

### Findings for separate work

These were found during the audit and are outside its scope:

- Two dashboard components have no consumers anywhere in the source.
- One Go package is imported by nothing in production.
- A test-only seam exists in production file-handling code purely to support two
  tests this audit recommends deleting.
- A tolerance for a legacy session-listing format is exercised only by its own
  fixtures; production never emits that shape.
- The strategy document's counts and job duration are stale.

## Execution: before and after

The cuts, the merges, the moves and the CI split landed under chrote-64pz.2.
Every figure below is measured on the same 16-core host, `-count=1` so nothing
is served from a cache, and the "after" column is the merged tree with the
Library and Agents tabs on it.

| gate | before | after | note |
| --- | --- | --- | --- |
| Go, `go test -race -count=1 ./...` | 9.132s | 3.936s | api 8.82s to 2.18s, proxy 3.19s to 1.18s |
| gofmt and go vet | clean | clean | unchanged |
| Dashboard unit, `npm run test:unit` | 5.783s | 5.791s | no change, as predicted |
| ESLint | 2.461s | 2.436s | unchanged |
| Mocked Playwright, 4 workers | 44.670s | 17.675s | 101 cases to 36 |
| doc-lint | 0.28s | 0.27s | unchanged |
| host-neutrality | 0.161s | 0.161s | unchanged |
| embedded parity | n/a | 0.30s | bundle rebuilt, 260 tracked files |
| embedded dashboard build | n/a | 3.758s | unchanged work, measured for the record |

Local wall time across the suites falls from about 62s to about 30s. The CI
saving is larger and is not measured here: the worker cap that serialised the
browser suite is gone, and the single job is now five that start together.

### Corrections to this audit

The counts above were measured against an older main and were stale by the time
the cuts began. The real baseline was 64 unit files and 716 cases (not 57 and
650), 101 browser cases (not 98), and 33 Go test files with 278 test functions.
`beads.spec.ts` had been rewritten entirely by another lane, so none of its
verdict rows applied and it was left alone.

"Two dashboard components have no consumers" was one component, DockPanelToggle,
plus three dead exports of LoadingSkeleton whose default export is live.

### What was deliberately not done

`tests/integration/terminal-pool.spec.ts` keeps both cases rather than
collapsing to one. It is an opt-in live spec, and verifying a collapse needs a
real tmux substrate that must not be touched on this host. An unverified edit to
the gate that defends the golden invariant is worse than a second case that
never runs in CI.

Nothing now pins the live specs to the cleanup ledger's retry counts. The two
unit cases that did it read the spec files as text, which is a lint rule wearing
a test's clothes, and they were deleted. If that intent is worth keeping it
belongs in a source contract script.

### Measured after the cut (2026-09-03, first run of the parallel layout)

| | before | after |
| --- | --- | --- |
| CI wall time | 5m46s, one serial job | 1m43s, five parallel jobs (build 47s, go 34s, unit 68s, browser 99s, contracts 38s) |
| Go suite | 9.1s | 3.9s |
| Browser suite, 4 workers | 44.7s | 17.7s |
| Unit suite | 5.8s | 5.8s |
