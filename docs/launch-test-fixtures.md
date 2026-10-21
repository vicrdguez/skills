# Adapter and launch test fixtures

`cmd/skl/launch_fixtures_test.go` arranges the committed preconditions needed by
adapter and launch tests. It does not implement acceptance or handoff behavior.
It uses the existing proposal specs, file writers, committed-state helper and
report codec; it creates no snapshots or shared writable seeds.

Each expensive row keeps its own ledger clone, bare upstream, source repository,
configuration selection, output buffer and Claims. The installed-entry matrix
shares only the completed, read-only installation. Native `t.Parallel` scheduling
and `ledgerFixture.app` retain the app-owned settings lookup supplied by the
native-parallel prerequisite. Source Git transport still uses the existing
protocol restrictions and, where pushes are required, the private local SSH
shim. No process environment is changed by these rows.

The ready fixture contains a committed Project, accepted Proposal, frozen
Contract and ready Slice(s). The review fixture additionally has a historical
implement Claim, pinned Contract references, a distinct source commit on the
planned branch, its real target ancestor, and a committed implementation report.
The wait case finishes its real acquired Claim rather than replacing its history.
No linked-worktree metadata is copied.

## Grouped preservation map

| Protection | Surviving checks |
| --- | --- |
| Every installed family, harness syntax, placeholders, opaque arguments and role slots | `TestInstallPassesHarnessModeAndOpaqueSlotsWithoutDefaults`, `TestInstallWritesLoopAndAutoFamiliesForNativeHarnesses`; installation/update tests remain unchanged, including Codex stubs |
| Installed dispatch to actual worker startup: harness, standard/team, loop/Auto, runtime defaults, explicit worker override, target, exact Claim; held-Claim continuation refusal | All 18 rows of `TestInstalledFamiliesDispatchWithRuntimeSettingsAndContinueSafely`; watchdog rows additionally check the implementation source/target and historical Claim reference |
| Installed prepare/inspect and recovery identity | `implement-team-auto` row for **each** native harness; non-Auto guidance/recovery stays in `TestStartupAndRecoveryRefreshDefaultsWithoutChangingRunningGuidance`, empty/missing-field tests and the unchanged delivery journeys |
| Harness and role isolation, partial overrides, direct startup and fresh recovery | `TestHarnessLaunchChoicesReachTheirRoles`, `TestDirectWorkerStartupAndRecoveryUseCurrentRoleDefaults`, `TestMissingRoleFieldsStayInheritedUntilFreshRecovery` |
| Defaults refresh at startup, recovery and continuation, while running prepare/inspect guidance stays stable | Both inherited and explicit rows of `TestStartupAndRecoveryRefreshDefaultsWithoutChangingRunningGuidance`, plus `TestEmptySessionChoicesStayInheritedAfterConfigurationEdit`; the live implement handoff, watchdog verdict and continuation remain |
| Waiting implementation, partial overrides, waiting watchdog, malformed config edit, changed-ledger refusal | All five rows of `TestWaitingDispatchRefreshesLaunchSettings`, still using `synctest` virtual waits and real acquisition/startup |
| Invalid configuration and selector refuse without acquiring work | `TestLaunchConfigurationRefusalsDoNotClaimWork`, now with eligible work; only this refusal test reuses its private fixture, checking unchanged refs/status after every invocation and stopping immediately on mutation |
| Real creation of the arranged lifecycle states | Unchanged `TestAcceptSingleSliceFreezesExactLayout`, `TestAcceptMultiSliceWithDependencies`, `TestDeliveryCLIEndToEnd`, dispatch and report/handoff tests |
| Real cancellation, OS signals, cleanup lock race | Unchanged existing tests; virtual wait changes do not replace these seams or shorten the lock-race wait |

## Measurements

Starting revision: `8556299841d81e83eb1f079780d3798e86a25847`, already containing
the native-parallel prerequisite. Machine: Apple M3 Pro, Go 1.27.2 darwin/arm64.
Warm build cache, uncached results, default Go parallelism; runs did not overlap
other benchmarks. These are Slice measurements, not a claim of the Proposal's
60-second full-suite target.

The affected selection is:

```sh
go test ./cmd/skl -count=1 -json -run 'Test(InstalledFamilies|HarnessLaunch|StartupAnd|EmptySession|LaunchConfiguration|WaitingDispatch|DirectWorker|LaunchResolved|MissingRole|StoppedDispatch)'
```

| Measurement | Before | After |
| --- | ---: | ---: |
| Affected package elapsed | 17.125s | 13.361s |
| Command wall time, including Go invocation/build overhead | 17.55s | 14.42s |
| Installed matrix scheduled span within that selection | 16.748s | 12.139s |
| Other launch/settings scheduled span within that selection | 14.571s | 10.869s |
| Git built-in invocations, including fixture construction | 11,711 | 8,249 |
| Private ledger clones / source repositories | 45 / 44 | 41 / 40 |
| Incidental acceptance CLI calls / forge servers | 39 | 0 |
| Source linked worktrees created | 23 | 7 |

Scheduled spans run from the group's first JSON `cont` to its last `pass` and
overlap each other; they are not isolated matrix benchmarks or additive costs.
Set `GIT_TRACE` to a fresh absolute file for each run and count lines containing
`built-in: git ` for the Git work measure. CLI setup/follow-up reduction is 77
invocations: 39 acceptances, six implement dispatch/prepare/submit journeys,
one wait-case prepare/submit, and nine redundant installed prepare/inspect pairs.
The five invalid-config/selector invocations remain distinct.

A separate sequential, three-repetition setup-only probe timed from before
`newPrivateLedgerFixture` through the completed fixture, without the installed
command under test. Both versions also created the source, local push shim and
initial `main` push. Ready setup used acceptance before and
`commitLaunchProposal(singleSlice(...))` after. Review setup additionally used
implement dispatch/handoff before and `commitLaunchReviewClaim` plus
`commitLaunchAwaitingReview` after. Medians: ready **0.934s → 0.821s**; review
**2.590s → 1.045s**. Total traced Git built-ins over three ready and three review
constructions fell **1,113 → 351**. The probe was temporary, not a new default test
or benchmark framework; fixture work is kept separate from startup timings.

Regression-sensitivity challenges temporarily changed the Pi installed team
command to standard and separately changed a fixture report's target to its
source head. Actual worker startup failed respectively on the team-mode and
review-evidence assertions. Neither mutation is retained.

Production inputs are unchanged. Controlled baseline/candidate builds used the
same toolchain, `go build -trimpath -buildvcs=false ./cmd/skl`, and compared equal
(SHA-256 `63420e4c32453f507ecc52a6c06a44d2eabaa053942cc25d059d9fcb93699bb4`).
VCS-stamped builds are not used for byte-equality claims.
