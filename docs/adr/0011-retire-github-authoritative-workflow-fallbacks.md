# Retire GitHub-authoritative workflow fallbacks

The private Workflow Ledger is the only supported authority for Workflow State. Retire the remaining GitHub-authoritative paths rather than maintain and test a second workflow for repositories without ledger adoption. GitHub remains a publication surface and a source of merge observations for ledger-backed work; shared transport and those supported responsibilities remain.

## Status

Retirement decisions accepted during Explore; implementation is pending. This completes the authority transition in ADR 0006 and the fallback retirement deferred by ADR 0008. It does not change running commands until implemented.

## Consequences

- Remove `skl propose publish`, its legacy flags and exclusive machinery, without a compatibility command. `ledger accept` and `ledger publish` remain the supported publication path.
- Without a configured ledger or an accepted Project, `status` and `propose cleanup` stop with actionable configuration/adoption guidance. They do not infer Workflow State or cleanup authority from GitHub. Preserve broken-configuration safeguards and ledger-backed behavior.
- Setup prepares local repository guidance: the owned `AGENTS.md` block, the `.worktrees/` ignore entry and the optional `CLAUDE.md` link. It works in a local Git repository without a GitHub remote or ledger configuration. Keep `--repo`; remove Setup's `--remote` flag, GitHub/default-branch validation and lifecycle-label creation. Preserve local file-safety checks and report local preparation rather than readiness for ledger execution. Setup does not initialize a ledger or implicitly adopt a Project.
- Preserve ledger-backed issue and PR publication, merge observation, and source-cleanup safety. Remove legacy-only production code, tests and fake capabilities; shared code survives only for supported responsibilities.
- Leave historical GitHub labels, comments and issues untouched. This retirement adds no importer, remote cleanup mechanism or migration workflow.
