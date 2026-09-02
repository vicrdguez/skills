# Set up Consumer Repositories Plan

## Approach
Create the smallest executable vertical path: an `urfave/cli` command delegates one Setup request to a deep Setup module. Setup gathers all repository and GitHub facts first, builds the intended mutations, then applies only the owned changes. Real Git and filesystem behavior are exercised in temporary repositories; the true external GitHub dependency sits behind the Backend seam.

Do not turn Setup into a generic configuration or file-management framework. Its constants are this repository's one Workflow: fixed marker names, label definitions, durable-document paths, and worktree convention.

## Implementation decisions
- Use `github.com/urfave/cli` for the CLI surface; choose the compatible major version during implementation.
- Expose `skl setup`, with current-directory Git-root discovery, `--repo` for an explicit path, and `--remote` only to resolve ambiguous GitHub remotes.
- Use Go's standard library for embedding-free Setup logic, filesystem operations, HTTP, JSON, and process execution.
- Invoke the installed `git` binary rather than implementing Git plumbing or wrapping ordinary Agent Worker operations.
- Authenticate in order through `GH_TOKEN`, `GITHUB_TOKEN`, then `gh auth token`; never persist the token.
- Implement a narrow backend-neutral port because GitHub is a true external dependency and tests need an in-memory adapter. Add only the Setup operations V1 needs; do not design the later Local Backend now.
- Validate every precondition before the first local or remote mutation.
- Replace only one exact marker-owned AGENTS block. A half-pair, duplicate pair, nested pair, or reversed markers is an error.
- The minimal block names the Workflow entrypoint, tells agents to use `skl` instead of manually mutating Workflow Projections, and states that only the human merges. It does not restate the state machine, GitHub commands, or project checks.
- Ask before replacing an eligible `CLAUDE.md`; use a relative symlink. Preserve substantive files by default.
- Remove `docs/github.md` when present even though the legacy file has no ownership marker; this destructive migration is explicitly accepted.
- Preserve unrelated dirty worktree content and unrelated labels.
- Produce structured expected outcomes; reserve nonzero exits for invalid invocation, setup precondition, dependency, or unexpected operational errors.

### Module shapes & seams

#### [NEW] CLI
**Interface:** `skl setup [--repo <path>] [--remote <name>]`, its exit status, stdout outcome, stderr diagnostics, and interactive confirmation input.

The CLI translates arguments and presentation only. It contains no setup rules.

**Test strategy:** invoke the command through the same application interface used by `main`, using temporary repositories and controlled stdin/stdout/stderr. Assert behavior, not command-handler calls.

#### [NEW] Setup
**Interface:** a typed Setup request containing repository location, optional remote, and confirmation decisions; a typed outcome describing applied or refused preparation.

Setup owns resolution, preflight ordering, the exact owned-file rules, and idempotence. Callers do not coordinate individual writes or label mutations.

Dependencies: the concrete local Repository module; the Backend port; a confirmation input. Filesystem and Git are local-substitutable through real temporary repositories. GitHub is true external and uses the Backend port.

**Test strategy:** test Setup through its request/outcome interface with a temporary Git repository and in-memory Backend adapter. Verify final observable files and normalized labels.

#### [NEW] Repository
**Interface:** focused operations needed by Setup to locate a Git root, inspect remotes, and safely apply its fixed owned-file mutations.

Keep this concrete. It shells out to `git` and uses the standard filesystem; do not add a speculative Git interface or mock internal calls.

**Test strategy:** exercise it only through Setup in temporary real Git repositories unless an invariant cannot be observed there.

#### [NEW] Backend port and GitHub adapter
**Interface:** resolve repository identity and target branch, validate access, and ensure the fixed Workflow Projection labels.

The port speaks Workflow terms rather than HTTP endpoints. The GitHub adapter uses direct API requests and the supported authentication chain; an in-memory adapter supports behavior tests.

**Test strategy:** Setup tests use the in-memory adapter. A small adapter test uses an HTTP test server to verify request/response mapping and authentication without calling GitHub.

## Sequence
1. Add the `urfave/cli` executable and command-level behavior test seam.
2. Implement Git-root, remote, and authentication resolution with refusal-before-mutation tests.
3. Add the minimal Backend port, in-memory adapter, and direct GitHub adapter.
4. Implement owned AGENTS, CLAUDE, gitignore, and legacy-file mutations through temporary-repository scenarios.
5. Wire label preparation and repeatability.
6. Add Setup-facing documentation while retaining the legacy setup skill/reference as a migration fallback until the completed lifecycle retires it.
