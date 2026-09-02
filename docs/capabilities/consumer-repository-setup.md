# Consumer repository setup

`skl setup` prepares a Consumer Repository to use the repository-owned Workflow and its active Workflow Backend.

## Behaviors

- Validates the repository, backend access, and authentication before changing state.
- Infers GitHub from `origin` when it is a GitHub remote, otherwise from the sole GitHub remote; ambiguous repositories require an explicit remote for that invocation.
- Discovers the target branch through GitHub and accepts local-only work branches as resumable partial publication state.
- Prepares the Workflow Projection labels and the local worktree ignore.
- Uses existing GitHub environment or `gh` authentication and stores no credentials.
- Creates or replaces only the marker-owned workflow block in `AGENTS.md` and stops on malformed or duplicate markers.
- Offers a `CLAUDE.md` symlink when safe and preserves substantive existing guidance unless replacement is explicitly authorized.
- Removes the superseded `docs/github.md` protocol document.
- Produces the same owned guidance and backend preparation when repeated with the same choices.

## Out of scope

- Installing the user-level `skl` binary or Harness Adapters.
- Writing per-repository workflow configuration or customizing Workflow Mechanics and Skill Definitions.
- Making setup-time choices during normal worker commands.
