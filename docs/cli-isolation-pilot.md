# CLI process-isolation pilot

## Status

The small self-reexec approach improves runtime, but does not preserve native
Go test caching. It is **not enabled in the default suite**. This pilot stops
before broad conversion; it is evidence for the human decision about the
app-local configuration-lookup alternative described in the optimization plan.
No production inputs, existing scenario bodies or golden files are changed.

## Reproduce

With Go, Git and Python 3 installed, from the repository root:

```sh
python3 scripts/cli-isolation-pilot.py --output /tmp/skl-isolation-pilot
```

The output directory must not exist. `--cache-only` runs only the cache probe;
`--repetitions N` changes the default three timing repetitions. Logs and
`result.json` are written to that directory. Temporary overlay files and the
probe fixture are removed on normal exit, including a failed probe. Forced
termination can leave a `testdata/cli-isolation-*` temporary directory.

The script uses a Go build overlay to add the same self-reexec helper to six
existing scenarios: ledger configuration selection/refusals, private delivery,
the delivery lifecycle, terminal-proposal cleanup and the agent-prose journey.
It changes no assertions. Each isolated parent joins the parallel queue with
`t.Parallel`, directly executes the already compiled test executable and
returns without running its scenario body. The child runs that body serially.
There are no Bash background jobs or shared writable Git fixtures.

Timing uses one compiled executable in both modes, with uncached test results,
eight available parallel slots and alternating native/isolated runs. Child
startup is inside the measurement. Run this without competing benchmarks.
These are pilot timings, not a delivered default-suite speedup.

## Observation at 480179c

Go 1.27.2, darwin/arm64, three repetitions, seconds:

| Mode | Run 1 | Run 2 | Run 3 | Median |
| --- | ---: | ---: | ---: | ---: |
| Native serial | 78.653 | 78.923 | 78.453 | 78.653 |
| Isolated | 51.223 | 50.902 | 51.515 | 51.223 |

The pilot is 1.54 times faster (35% lower median wall time). This does not
establish the whole-suite target or the semantics of a completed helper.

## Cache blocker

The probe reads an external expected fixture inside the module but outside Go
build inputs. It first establishes a normal cache hit, then changes the fixture
to a value that must fail. It uses old, distinct mtimes so Go's refusal to cache
very recently modified files does not obscure the result.

| Execution | Normal run after fixture change | Uncached run |
| --- | --- | --- |
| Native | Assertion fails; cache invalidated | Not needed |
| Isolated | Stale cached success | Child assertion fails with diagnostics |

The child reads do not enter the parent's dependency log. In the inspected Go
1.27.2 sources, `testing/testing.go` declares `test.testlogfile` for use only by
`cmd/go`, opens it with `os.Create`, and buffers logging through
`testing/internal/testdeps`. `cmd/go/internal/test/test.go` computes cache keys
from that one process's `getenv`, `open`, `stat` and `chdir` records. Forwarding
the same log filename would truncate and race the parent's log, not combine
independent child dependencies.

A dependency relay would need separate child logs, cwd-aware interpretation,
parent dependency accounting, completeness/error checks and its own cache
regressions. Hard-coding today's fixture/environment list is not equivalent;
disabling caching or always forcing `-count=1` is not an accepted substitute.
This goes beyond a direct-exec wrapper and adds reliance on Go's internal log
protocol. Combined with native matcher/flag forwarding, profile/coverage
fallback and process-tree lifetime handling, it needs a separately justified
runner design, not an assumed extension of this pilot.

The experimental helper does **not** claim general matcher, profile/coverage,
golden-update, interrupt or orphan-cleanup support. Those paths remain native
in the default suite. The plan's narrower alternative is to make configuration
lookup app-local while retaining per-invocation reload/default/refusal behavior,
then parallelize only scenarios whose remaining mutable state is local. That
would change production seams and requires human review before implementation.
