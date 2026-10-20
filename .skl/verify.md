---
ranks: [cli, tests, build, inspection]
---

# skl verification

## Launch

The repository is one Go module; `mise.toml` pins Go. Every check runs from the repository root with `go` on the path and needs `git` for its fixtures. Nothing is started: the tests build `skl` in process and drive it against local Git fixtures, with no network and no forge.

## Doctor

- `go version` prints the Go version `mise.toml` installs.
- `git --version` prints a version; the fixtures run `git` hundreds of times.
- `go build ./...` exits 0 before any other row is worth running.

## Channels

- `oracle`: the agent-prose goldens in `testdata/prose/`, one Markdown file per rendering a worker receives, compared byte for byte by `TestAgentProseGoldens`.
- `process`: the `cmd/skl` tests, which run the CLI app with arguments against Git ledger and source fixtures and assert its status, outcome text and the records it commits.
- `tests`: the package tests of the root package, `ledger`, `setup`, `workflow`, `tools/prosemetrics` and `scripts`.
- `static`: the Go build, `go vet` and `gofmt`.

## Catalog

| Name | Channel | Kind | Run | Expectations | Rank |
| --- | --- | --- | --- | --- | --- |
| goldens | oracle | executed | `go test ./cmd/skl -run TestAgentProseGoldens -count=1` | `ok`; every rendering matches its golden and every resource a golden names has one | cli |
| cli-tests | process | executed | `go test -timeout 20m -count=1 ./cmd/skl` | `ok`, the cited test among those run | cli |
| go-tests | tests | executed | `go test -count=1 . ./ledger ./setup ./workflow ./tools/... ./scripts` | `ok` for every package, the cited test among those run | tests |
| build | static | executed | `go build ./... && go vet ./... && test -z "$(gofmt -l $(go list -f '{{.Dir}}' ./...))"` | exits 0 with no output | build |

The Full Gate is `go test -timeout 20m ./...` plus the `build` row.

## Cleanup

The tests write only to temporary directories. After regenerating goldens, `git status` shows only the goldens you meant to change.

## Extend

- A behavior rule of a command: add a test in `cmd/skl/*_test.go` that runs the command and asserts its outcome, and cite it as `cli-tests <TestName>`.
- A change to agent-visible prose: regenerate with `go test ./cmd/skl -run TestAgentProseGoldens -update-prose`, read the diff of `testdata/prose/`, and cite `goldens <golden-name>`. Report `mise run prose-metrics` before and after.
- A rule inside one package: add a test beside it and cite `go-tests <TestName>`.
- A commitment checked by reading the code: free text in the Check cell at rank `inspection`.

## Traps

- `cmd/skl` runs serially and spawns `git` for every fixture; the whole package takes over ten minutes, so run it with `-timeout 20m`, and run one test with `-run` while iterating.
- `-update-prose` rewrites the goldens to whatever renders: a golden diff is a change to review, never evidence by itself.
- A golden that no rendering produces, or a resource a golden names without a golden of its own, fails `TestAgentProseGoldens`; add or remove the golden with the rendering.
- `gofmt -l .` also walks `.worktrees/`, which holds other branches' checkouts; the `build` row passes the module's package directories instead.
- `go test` caches passing results; `-count=1` reruns them when the result is evidence.
- The tests set `GIT_SSH_COMMAND=false` and their own `TMPDIR`; a real remote or forge is never contacted, so a failure that names one is a fixture fault.
