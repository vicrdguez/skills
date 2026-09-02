# Extract workflow mechanics without redesigning agent behavior

Build the CLI as a compatibility-first extraction of this repository's existing workflow. Move deterministic mechanics, invariants, state transitions, and instruction specialization into the Workflow Engine, but preserve the established agent roles, stage ordering, Audit, Watchdog Review, one-bounce policy, and human merge boundary unless existing behavior is ambiguous, unsafe to make deterministic, or explicitly changed by a recorded decision. This gains efficiency and consistency without using the implementation project as an excuse to redesign the working judgment process.

## Consequences

- V1 uses [`urfave/cli`](https://github.com/urfave/cli) for command definition, argument parsing, and CLI composition; version selection is deferred to implementation.
- Audit remains a required implementation-phase Skill Definition rather than becoming a workflow state.
- Existing Skill Definition prose is edited surgically: remove or replace only deterministic mechanics now owned by the CLI, preserving judgment instructions, order, and behavior.
- The deterministic `setup` command replaces `dev-setup`; it no longer copies `docs/github.md` because workflow mechanics live in the engine.
- Setup infers the GitHub backend from repository remotes, prepares labels, adds the worktree ignore, and maintains only a minimal marker-owned `AGENTS.md` bootstrap plus an optional `CLAUDE.md` symlink. It writes no workflow configuration file.
- Setup unconditionally removes the legacy `docs/github.md`, replaces only the owned `AGENTS.md` block, preserves substantive `CLAUDE.md` files unless explicitly replaced, and retains existing GitHub label names as state projections.
- Existing in-flight Work Items are adopted lazily from their current projections; only ambiguous or contradictory items require human resolution.
- Agent Workers retain repository-content edits and ordinary Git operations such as commits, target-branch synchronization, pushes, and implementation-ledger retirement. `skl` reads Git to enforce workflow preconditions and owns backend state transitions; it does not wrap Git merely to reduce tool calls. Deterministically identified workflow worktree and branch cleanup is the narrow exception: `skl` performs it as part of the transition that makes the cleanup valid.
- Proposal publication accepts agent-authored, template-guided issue bodies as opaque Markdown. The CLI handles only deterministic publication facts and mutations: the slice slug, branch and Artifact Baseline, plus ephemeral parent prose, child slugs, and Dependency edges for a multi-slice Proposal. No duplicate durable proposal bundle, metadata file, Markdown parser, or prose generator is introduced.
- Before proposal publication, `skl` removes only clean conventional worktrees for already Merged Work Items and their matching local branches. It preserves and reports dirty, unexpected, remote, and unrelated Git state without blocking otherwise valid publication.
- Proposal publication refuses with repair instructions while fixed durable-document paths contain uncommitted changes or an Artifact Baseline does not descend from the target commit observed for publication. The Agent Worker owns the corrective Git operations.
- Implementers and Watchdog Review continue running each project's Full Gate and post-marker checks from preserved prose; `skl` does not configure or duplicate project commands.
- Watchdog remains independently invoked in a fresh context; V1 adds no identity, attestation, or Claim Token system beyond that operating convention.
- Watchdog may add linked `DEBT(...)` source comments for nonblocking findings. The CLI records the final head and runs only the existing formatter/parser check plus `git diff --check`; it does not parse markers, rerun Audit, or rerun the full gate.
- Deliberate changes such as retired implementation ledgers and merge-time issue closure must be recorded rather than smuggled in as cleanup.
