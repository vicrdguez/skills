# Consumer repository setup

`skl setup` prepares a Consumer Repository to use the repository-owned Workflow and its active Workflow Backend.

Run it from any directory inside the Consumer Repository, or pass `--repo <path>`. When GitHub remote inference is ambiguous, rerun with `--remote <name>`.

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

## Simplicity standard

- Installs and maintains a repository-owned simplicity standard in the managed `AGENTS.md` block as part of normal Setup, without an opt-in. It applies to coding and review inside and outside the Workflow.
- The standard preserves accepted behavior and required verification, favors adequate existing mechanisms and justified structure, and directs simplification toward maintenance burden rather than line counts. Test guidance preserves distinct regression protection while simplifying repeated setup and incidental implementation coupling.
- Setup refreshes the standard inside the existing workflow markers while preserving Workflow entrypoint instructions and all surrounding user-authored guidance byte-for-byte. Repeating Setup with the same choices leaves `AGENTS.md` unchanged and the section present once.
- The installed standard is the agent-facing source of truth; Instruction Packets do not repeat it, and it requires no separate Skill Definition. PR size and reviewability remain out of scope.
- Existing Skill Definitions and resources remain unchanged except for removal of Audit's optional `ponytail-review` integration. Overlapping guidance is retained; consolidation is out of scope.

## Out of scope

- Installing the user-level `skl` binary or Harness Adapters.
- Writing per-repository workflow configuration or customizing Workflow Mechanics and Skill Definitions.
- Making setup-time choices during normal worker commands.
