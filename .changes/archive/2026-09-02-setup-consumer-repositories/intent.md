# Set up Consumer Repositories

## Why
The current `dev-setup` skill asks an Agent Worker to interpret and execute deterministic repository and GitHub preparation. That repeats protocol prose, varies by harness, and maintains a copied `docs/github.md` that the new Workflow Engine makes obsolete.

## What
Introduce the Go `skl` executable and an idempotent `skl setup` command that prepares a Consumer Repository for the existing Workflow while preserving user-owned repository guidance.

## Scope
- Build the initial `skl` command with `urfave/cli`
- Resolve a Consumer Repository from the current directory, with an explicit repository-path override
- Use the installed `git` executable for repository facts and local operations
- Infer GitHub from `origin` when it is a GitHub remote, otherwise from the sole GitHub remote
- Require an explicit remote when GitHub remote inference is ambiguous
- Discover the target branch from GitHub
- Authenticate direct GitHub API requests through `GH_TOKEN`, then `GITHUB_TOKEN`, then `gh auth token`
- Validate repository, GitHub access, authentication, and owned-file preconditions before making changes
- Create or refresh the existing workflow labels with their current names, colors, and meanings
- Ensure `.worktrees/` appears in `.gitignore` without changing unrelated entries
- Create or replace only the exact `<!-- dev-pipeline:start -->` / `<!-- dev-pipeline:end -->` block in `AGENTS.md`
- Keep the owned `AGENTS.md` block to a minimal workflow entrypoint, `skl`-over-manual-projection rule, and human merge boundary
- Stop without rewriting `AGENTS.md` when markers are malformed or duplicated
- Offer a relative `CLAUDE.md` symlink when the path is absent or contains only the exact legacy `@AGENTS.md` import
- Preserve substantive `CLAUDE.md` content unless the user explicitly authorizes replacement
- Remove the legacy `docs/github.md`
- Write no per-repository workflow configuration
- Remain idempotent when repeated

## Out of Scope
- Installing Skill Stubs or Agent Harness adapters
- Implementing proposal, implementation, or review transitions
- Removing the repository's legacy setup skill or packaging before every workflow stage has an equivalent CLI path
- Running Consumer Repository tests, linters, formatters, or typecheckers
- Migrating or rewriting existing Work Items
- Implementing a Local Backend
- Storing credentials

## Definition of Done
- [x] `skl setup` resolves the intended Consumer Repository and GitHub backend without configuration, or refuses before mutation when resolution is ambiguous or invalid.
- [x] Setup prepares the existing workflow labels through authenticated direct GitHub API calls and preserves unrelated labels.
- [x] Setup creates or refreshes only its minimal owned `AGENTS.md` block and refuses malformed or duplicate marker layouts without changing the file.
- [x] Setup safely offers the `CLAUDE.md` symlink migration while preserving substantive existing content unless replacement is explicitly authorized.
- [x] Setup ensures the worktree ignore, removes legacy `docs/github.md`, and preserves unrelated repository files and ignore entries.
- [x] Repeating Setup against an already prepared Consumer Repository produces the same owned repository and GitHub state and no workflow configuration file.

## Manual verification
None.
