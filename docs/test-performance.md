# Full-suite performance: remaining optimization

## Result and measurement conditions

**The 60-second target was not reached.** The ordinary complete suite still
requires several minutes. Neither the 90-second intermediate milestone nor
an isolated selection is substituted for that target.

Measured on 2026-10-21: Apple M3 Pro, 12 physical/logical cores, macOS/Darwin
arm64, Go 1.27.2, `/usr/bin/git` (Apple Git 2.54.0). `GOMAXPROCS` and `GOFLAGS`
were unset; Go's default package concurrency and test parallelism were 12.
There was no bespoke scheduler, short mode, build tag, removed package or
cached test-result timing. Build caches were warm. These runs were sequential,
not overlapping other verification or benchmarks launched for this change.
This is a developer machine, not an isolated benchmark host: system load and
per-process cost varied substantially. More concurrent execution reduced wall
time but increased combined user/system CPU; this is not a CPU-cost win.

Starting revision `4fa7c58c141ee06ce96fc91ada1f1d0fa4185b3f` includes all six
prerequisites. Final test-code revision is
`6dd7f705fcf87321441d8d2d7f8e8769ab6fcee6`. Changes after that revision
are this non-embedded maintainer document only.

Reproduce a run from the revision being measured:

```sh
# Explicitly warm compilation if needed; this is not the measurement.
go test -run '^$' ./...
/usr/bin/time -p go test -json -timeout 30m -count=1 ./... > full.json 2> full.time
python3 scripts/test-times.py full.json --top 25
# Diagnostics, run separately rather than concurrently:
/usr/bin/time -p go test -json -timeout 30m -count=1 -parallel=4 ./... > p4.json 2> p4.time
# Repeat with -parallel=8 and -parallel=12.
```

`test-times.py` uses only Python's standard library and is optional inspection
tooling, not a test runner or a new gate dependency. It counts completed
**top-level** results, including repetitions, and lists package times and the
slowest top-level durations. It never adds child durations to parents. Cached,
incomplete, empty and failed logs return failure rather than being presented
as successful uncached timing evidence. The logged invocation and revision,
not the summarizer alone, establish whether a selection was the full suite.

## Comparable complete-suite measurements

Every passing timing below contains **569 passing top-level tests**, all eight
tested packages, and the unchanged no-test-files `github` package. The earlier
historical count of 571 is not treated as the current inventory.

| Revision / run | Wall s | User s | System s |
| --- | ---: | ---: | ---: |
| Baseline, original worktree, 1 | 305.06 | 513.15 | 624.10 |
| Baseline, original worktree, 2 | 312.19 | 507.28 | 627.43 |
| Baseline, isolated checkout, 3 | 343.94 | 550.92 | 661.60 |
| Baseline, isolated checkout, 4 | 376.60 | 614.14 | 738.29 |
| Final test code, 4 | 206.16 | 621.05 | 783.75 |
| Final test code, 5 | 236.68 | 658.95 | 838.36 |
| Final test code, 6 | 240.04 | 666.57 | 851.88 |

Baseline median/range: **328.07s / 305.06–376.60s**. Final test-code
median/range: **236.68s / 206.16–240.04s**. User/system medians are respectively
532.04/644.52s before and 658.95/838.36s after. The median wall reduction is
about 28%, with material host variability; it is not evidence of 60-second
feasibility. Baseline 3 → candidate 5 → baseline 4 → candidate 6 were interleaved
to check that the benefit was not just the earlier, quieter host conditions.
The isolated baseline checkout was at the same starting SHA and was explicitly
compilation-warmed. Git selection and concurrency settings were unchanged.

Pre-final measurements at `5cc2b49` also passed: default wall times 218.25 and
240.27s. They are not pooled with the final-revision median. A subsequent
helper-rename intermediate revision failed to compile because one wait-test
caller still used the old name; that failed run is excluded, the caller was
fixed in `6dd7f70`, and the full gate and race/shuffle suite passed afterward.

Package elapsed medians (packages overlap, so these are **not additive**):

| Package | Baseline s | Final test code s |
| --- | ---: | ---: |
| cmd/skl | 327.53 | 236.12 |
| ledger | 43.35 | 56.21 |
| scripts | 38.48 | 50.00 |
| browse | 33.60 | 43.75 |
| workflow | 9.07 | 11.27 |
| setup | 1.90 | 1.96 |
| root | 0.54 | 0.38 |
| tools/prosemetrics | 0.55 | 0.46 |

The CLI remains the complete-suite critical path. Other packages did not get
faster in these runs; their increased times and the CPU totals expose the
contention/host-cost limitation rather than attributing all wall improvement
to eliminated work.

## Changes and retained protection

No top-level test or scenario assertion was removed. The only removed checks
were incidental acceptance/preparation success checks replaced by committed
fixture preconditions; real lifecycle owners remain below.

| Group | Removed cost / execution change | Surviving obligations |
| --- | --- | --- |
| Decision, completion/reconcile, browsing, publication, amendment and presentation | Remaining isolated CLI cases use the existing app-owned settings seam and native `t.Parallel`, rather than selecting their private ledger through process environment | Existing CLI outcomes, exact references, requests, refusal/repair text, no-effect snapshots, concurrent-writer hooks and report/history assertions remain; private apps also own their urfave help commands |
| Cleanup archive eligibility and failure cases | Nineteen incidental acceptance journeys become committed ready-Proposal fixtures | Whole-tree identity, terminal/claimed/unknown-state eligibility, exact older report reads, current archived reads, offline/reconciliation behavior, staged/dirty/interrupted/mode/collision recovery; the late real acceptance checks still reject archived identities and accept archived dependencies |
| Run Metadata | Accepted-work setup and incidental `implement prepare` become committed fixture state plus a real freshly created linked worktree | Real Claim acquisition and both phase handoffs/readback, exact opaque metadata values, malformed/missing file notices, authoritative source refusal, schema-1 bytes, exact retry identity; global build-info mutation cases stay serial |
| Readiness recovery | Five tests arrange real source and historical Claim/Contract/report objects and an already attached external PR instead of rerunning implementation/PR publication | Real watchdog handoff, fresh polling, body/title preservation, missing association refusal, uncertain readback retry, independent real-remote movement and final integration-head/tree checks; the native transport/SSH protections and real source worktree remain |
| Lifecycle owners | Unchanged real API journeys | `TestAcceptSingleSliceFreezesExactLayout`, `TestAcceptMultiSliceWithDependencies`, `TestDeliveryCLIEndToEnd`, dispatch continuation/handoff tests, `TestSlicePublishReauthorsTheCurrentResultAfterMissedPhases`, Auto Mode merge tests and source-preparation tests retain actual transitions |
| Global/OS boundaries | Not virtualized or parallelized unsafely | Environment/configuration reload and cwd adapters, concurrent acceptance helpers, real context cancellation and OS-signal tests stay distinct. The cleanup lock-race wait is unchanged. The real four-target release-artifact test is unchanged |

The existing committed-fixture helpers now retain explicit planned branches,
parent titles and both intra-/cross-Proposal dependencies for these current
callers. Each test still owns its mutable ledger clone, bare upstream, source,
Claims, config and output. No writable seed or copied linked-worktree metadata
is shared. Production files, dependencies, authored prose and goldens are
unchanged.

### Focused work counts and sensitivity

These are separate, traced **selections**, not whole-suite target measurements.
Before/after used the same selection, host, Git and default parallelism. Counts
are lines containing `built-in: git ` in a fresh absolute `GIT_TRACE` file.

| Selection | Git invocations before → after | Command wall before → after |
| --- | ---: | ---: |
| `TestCleanupArchive(sWholeTerminalProposals\|FailuresKeepOneCompleteProposal)` | 2,361 → 1,128 | 14.28 → 7.30s |
| `TestAutoReadiness(MissingAssociationRequiresExplicitRecovery\|UncertainReadbackWaitsForALaterFreshPoll\|UsesFinalIntegrationHeadNotReviewedImplementation)` | 2,586 → 1,761 | 11.08 → 8.32s |

Representative temporary production faults failed at the intended assertions:

- Omitting the real-remote source-head comparison let moved source merge;
  `TestAutoReadinessRefusesRealSourceMovementDespiteExactAPIHead` failed on
  `status: merged` rather than its required remote-source refusal.
- Passing nil metadata to the handoff made
  `TestRunMetadataIsRecordedWithBothPhaseReports` fail on absent recorded data.
- Classifying Superseded work as fully delivered made
  `TestCleanupArchivesWholeTerminalProposals` reject the mixed retirement flag.

All faults were restored before final verification; none is delivered.

## Remaining costs and limits

Complete-suite 4/8/12-way trials at `5cc2b49`, with default package concurrency:

| Per-package `-parallel` | Wall s | User s | System s | cmd/skl s |
| --- | ---: | ---: | ---: | ---: |
| 4 | 229.58 | 455.13 | 476.38 | 229.04 |
| 8 | 199.15 | 564.70 | 673.57 | 198.60 |
| 12 | 227.63 | 653.36 | 834.51 | 227.09 |

These single-run trials do not establish a universal best cap. Four reduces
CPU pressure but lengthens the CLI queue; eight was faster in this sample but
still nowhere near 60. No additional automatic concurrency policy is added:
the ordinary invocation benefits from native scheduling, and explicit Go
controls keep their normal meaning. Up to 12 packages can run concurrently,
each with its own parallel limit; `-parallel=12` does not mean a global
12-worker limit. The release package also starts actual Go cross-builds.

Final run 6's slowest top-level durations include:

| Owner / test | Seconds |
| --- | ---: |
| CLI `TestAgentProseGoldens` | 105.84 |
| ledger `TestReadinessRecoveryStopsObservableInvalidationBeforeMerge` | 55.07 |
| CLI `TestAutoPassRefusalsAreReportedWithoutFailingTheHandoff` | 47.08 |
| CLI `TestPresentationAuthoringCarriesTheCompletedReviewCount` | 41.68 |
| scripts `TestReleasePublishesCompleteTaggedArtifacts` | 40.86 |
| CLI `TestDeliveryCLIEndToEnd` | 36.45 |
| CLI `TestDispatchContinuesAfterEveryHandoff` | 34.82 |
| CLI `TestLedgerReconcileUnmergedClosureAndUnknownIdentityCannotSatisfyBlocker` | 34.46 |
| ledger `TestReadinessRecoveryRequiresKnownMatchingRepositoryAndVerifiableRemote` | 33.59 |
| CLI `TestSlicePublishReauthorsTheCurrentResultAfterMissedPhases` | 31.03 |
| browse `TestListKeepsTheSelectedRowVisibleAndItsPositionOnTheBorder` | 30.62 |

The golden parent's 105.84s includes waiting for child scheduling: its longest
capture, delivery, was 42.33s. Those numbers must not be added. Likewise,
ledger readiness table parents include their rows. Inspecting these owners
showed distinct invalidation/repository/remote refusal triggers and actual
handoff/rework/continuation consequences, not interchangeable assertions.
The browse test scrolls through real malformed rows at two widths; deleting
navigation or substituting a production reader cache would change its seam.
The release test still builds, packages and inspects four real target binaries.
These protections were retained rather than buying a reduced default gate.

For the **current execution plan**, the final CLI serial tests alone total
67.18–82.17s, before a further 137.75–159.39s CLI parallel tail. In final run 6,
the cwd-sensitive ledger-wide decision inbox costs 17.90s, malformed metadata
handoffs 9.83s, real signals 6.88s, environment selection 5.14s, and no-forge
fallback refusal 4.89s. Global build-info and cwd/environment isolation prevent
simply marking all of these parallel. The release/ledger/browser packages
also consume 40–56s concurrently. These are measured limits of this candidate,
**not a proof that every permitted future test-only design has a >60s floor**.
Further fixture/seam investigation may help; a production seam, Git algorithm,
platform-specific launcher or weakened regression guarantee needs human
agreement rather than being smuggled into this performance change.

## Integrated verification

The final Go/test code passes the complete ordinary test/build/vet/gofmt/diff
checks, and `go test -race -shuffle=77531 -timeout 30m -count=1 ./...` passes all
packages (CLI 307.809s under race; not an ordinary timing). Selection with
`-count=2`, cache hit followed by invalidation after a changed test-fixture
literal, coverage and CPU-profile output, repeated full golden capture and
serial `-update-prose` regeneration all pass. Regeneration changes no golden.
An injected unowned golden fails on the completeness assertion and was removed.
The optional timing tool was checked with repeated/nested, cached, incomplete,
empty and failed synthetic logs.

Controlled baseline/candidate builds use the same toolchain and
`go build -trimpath -buildvcs=false ./cmd/skl`. They are byte-identical:
SHA-256 `63420e4c32453f507ecc52a6c06a44d2eabaa053942cc25d059d9fcb93699bb4`.
Version, help, resource and error commands outside the checkout also match
stdout, stderr and exit status. VCS-stamped binaries are not used for the
identity claim. The delivered diff contains only test files, optional test
inspection tooling and non-embedded maintainer documentation.
