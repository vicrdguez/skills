# Record Private Proposals Plan

## Accepted Commitments

ADR 0006 defines the target authority, layout, references, and publication boundary; ADR 0005 defines contract-grounded verification and delegated implementation choices. `CONTEXT.md` supplies the target vocabulary. Implement after #53, not by preserving contradictory terminology or mechanics merely because they remain at the proposal's starting revision.

### A1: Machine Configuration and Repository Identity

Use standard-library JSON for the single `ledger` absolute local path at the XDG/home location in B1. The clone already exists; its ordinary Git configuration owns replication destinations. No credentials, hosting choices, project aliases, or per-repository backend modes belong in machine configuration. Resolve source identity using existing remote-selection semantics, store the source association in `project.json`, and enforce B2 across checkouts and worktrees without a checkout registry. Ledger storage is distinct from source storage.

### A2: Git Owns Durable Acceptance

Use this ADR 0006 layout exactly, creating optional files only when warranted:

```text
projects/<repository-name>/
  project.json
  proposals/<proposal>/
    proposal.json
    proposal.md
    <slice>/
      state.json
      intent.md
      behavior.md
      plan.md                 # when warranted
      tasks.md                # when warranted
```

`proposal.md` is the durable approved proposal description, distinct from temporary public issue prose. `proposal.json` holds proposal metadata, not a duplicate child inventory; directory membership defines slices. `state.json` owns initial lifecycle, dependencies, planned source attachment, forge attachments, and pending-publication information applicable to this slice. Phase reports, decisions, and whole-proposal `archive/` operations arrive in their owning slices; do not scaffold them here.

Validate the declaration as a whole and commit its accepted records together. Keep Contract bytes frozen; subsequent publication/state bookkeeping must not rewrite them. Resolve repeated acceptance from recorded identities, not title matching or history searches. Serialize only brief local ledger mutations, not network publication; preserve unrelated edits and successful local work. Git is replication and readable history under an append-only operating convention, not enforced archival or distributed ownership. Do not add a database, event stream, source mirror, or permanent source-retention mechanism.

### A3: The Public CLI Is the Intake and Readback Boundary

Supply usable local acceptance and exact Contract retrieval, not just an internal storage adapter. Use explicit full commit/path references; the referenced revision need only contain the document, not introduce it. Preserve paths valid at referenced revisions. Keep source revisions separate from ledger revisions. No marker discovery, commit-message identity search, or worker responsibility for ledger navigation/bookkeeping.

Follow #44's specialized Markdown defaults and explicit JSON option for these operations, including refusal, pending, and deferred-resource output. Bind known facts and concrete readback commands; leave unknown implementation choices to the worker only when appropriate. Update Propose's authored definition/resources and their distributed retrieval coherently, preserving #53's bounded fidelity review and accepted-scope checks. Item numbering belongs to authors, not a CLI numbering service or prose parser. Exact new command names, JSON field spellings beyond the agreed configuration, and internal helper boundaries remain implementation choices.

### A4: Forge Publication Is a Best-Effort Human Surface

Keep local acceptance authoritative before publication. Attempt ledger push after authoritative ledger writes using existing Git remote/upstream configuration, and issue publication through the existing forge adapter. Record successful attachments and sufficient pending information for later catch-up without persisting public bodies. Keep replication and issue publication outcomes distinguishable; ordinary remote/authentication failures are pending, whereas competing history is an explicit reconciliation refusal before grants. Preserve ambiguous publication results rather than knowingly duplicating issues.

The agent supplies descriptive temporary Markdown; the CLI transports it and does not author prose or mine labels/comments for authority. A multi-slice parent groups work but owns no branch or Submission. Catch-up commands, replay/reconstruction policy, inline findings, and PR publication belong to later slices. Do not add YAML or report-schema infrastructure here; ADR 0006's YAML dependency becomes relevant only when phase reports are implemented by `run-ledger-delivery`.

### A5: A Safe Intermediate Release Has One Authority

Only intake, readback, and their initial publication are delivered here. Gate unsupported mutation/worker entrypoints before legacy selection, Claims, packets, or destructive cleanup can occur. Explain that delivery awaits `run-ledger-delivery`; do not direct a worker into a half-converted path or treat a missing ledger as permission to use forge state. Do not retain unsupported cleanup as a mandatory Propose intake prerequisite; its integration belongs to slice 6. This is an interim capability boundary, not a permanent backend selector.

Leave non-adopted records, accepted obligations, source progress, and public attachments intact. An operator may arrange the ADR's human-directed, ad-hoc adoption with normal workers stopped, but this slice ships no importer, migration command, compatibility mode, or invented historical evidence. Do not implement the later slices' execution, decisions, catch-up, merge/closure observation, dependency release, or archive/cleanup behaviors to bridge the gap.

### A6: Verify Contracts at Existing Seams

Use the public CLI with real temporary source and ledger Git repositories, including a local bare upstream and competing clone. Control publication responses through the existing forge HTTP adapter using Go's `httptest`/current HTTP test facilities, not live credentials. Assert observable CLI content/outcomes, committed ledger bytes and paths, source refs/worktrees remaining unchanged, and HTTP effects. Use independent expected content, not values recomputed by private helpers.

Preserve the Full Gate, Audit, and independent Watchdog judgment. Provide many-to-many evidence covering every rule, scenario, and A-item; grouped evidence is valid. Construction order, test grouping, and test count are delegated. Do not require one test per scenario, a private-helper mock scaffold, a new framework, or agent-behavior evaluation. Instruction retrieval checks and focused inspection verify supplied guidance, not autonomous agent judgment.

## Modules and Seams

These are existing orientation points, not frozen function signatures or a requirement to retain their forge-authoritative interfaces.

| Module | Interface Responsibility | Agreed Seam / Existing Location |
| --- | --- | --- |
| CLI composition | Accept/read Contracts, present outcomes, gate unsupported operations | Public `skl`; `cmd/skl/main.go`, `cmd/skl/implement.go`, `cmd/skl/watchdog.go`, `cmd/skl/status.go` |
| Setup and source binding | Load machine configuration; identify the source repository and Project without requiring forge availability | `setup/binding.go`, `setup/setup.go`, `github/remote.go`; CLI with local Git remotes |
| Proposal mechanics and persistence | Own acceptance, immutable Contract references, dependencies, and safe local writes | `workflow/proposal.go`; real ledger Git state. `workflow/ledger.go` currently means source endpoint validation and is not the target private-ledger contract |
| Forge adapter | Publish descriptions and return attachments/failures, without authoritative state selection | `setup/github.go`; existing configurable HTTP adapter |
| Propose distribution | Supply coherent intake instructions and warranted artifact resources | `skills/dev/propose/SKILL.md`, `skills/dev/propose/reference/`, `catalog.go`, `resource.go`; public instruction/resource retrieval |
| Existing verification | Exercise CLI, Git, HTTP, and distribution without new infrastructure | `cmd/skl/main_test.go`, `cmd/skl/watchdog_test.go`, `setup/github_test.go`, `catalog_test.go` |

## Errors and Evidence

| Boundary | Required Result / Verification Focus |
| --- | --- |
| Config, identity, declaration | Repair/refusal before acceptance or issue writes; cover XDG/home selection, collision, invalid paths/files/dependencies, and unchanged existing work (B1-B3, A1-A2) |
| Local write or repeat | No false success, partial accepted proposal, overwritten Contract, or absorbed unrelated edits; successful prior acceptance remains discoverable (B3/B6, A2) |
| Exact read | Byte-preserving accepted content and commit/path references, including Manual Verification; no substitute on missing input, no forge/history dependency (B4, A3) |
| Initial publication | Acceptance survives outages; retain successes and pending/uncertain outcomes, distinguish competing history, avoid guessed repair (B5-B6, A4) |
| Guidance and intermediate cutover | Coherent distributed intake instructions, no unsupported grants or fallback, preservation of non-adopted state (B7-B8, A3/A5) |

Run focused existing Go checks while implementing and the repository Full Gate, including `go test ./...` and `git diff --check`, on the final result. Record any material limitation rather than substituting a human-only checklist for behavior that local CLI/Git/HTTP checks can exercise. No `tasks.md` is warranted solely to restate these scenarios.
