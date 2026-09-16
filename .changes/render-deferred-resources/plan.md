# Render Typed Deferred Resources Plan

## Approach

Implement the first #44 vertical slice described by the planned refinement in `docs/adr/0001-embed-skill-definitions-behind-harness-stubs.md`, using `CONTEXT.md` terminology. Carry a real parent invocation through a deferred command into a complete Implement or Watchdog resource. Parsing, rendering, converted content, valid callers, and embedded distribution belong in this slice because a renderer alone is not usable and mandatory inputs would otherwise break existing callers.

The approved test seams are the existing in-process CLI and embedded distribution. Keep the current Propose formats and mandatory scenario-by-scenario red-green construction. ADR 0005 is durable future knowledge, explicitly scheduled after all #44 slices; do not adopt its artifact conventions, optional test-first ordering, or pre-Audit integration policy.

### Prerequisite

Publication now is approved with the advisory issue prefix `Blocked by: #40`. This is not an engine-native external Dependency and does not itself enforce eligibility. Do not begin implementation until #40 is Merged and its required code has been incorporated into this branch from the existing `c691fdf` starting point. Preserve the accepted ledger and reachable history. This ordinary incorporation of a known prerequisite neither restores target-sync mechanics nor requires recurring target merges before Audit.

## Implementation decisions

- Public flags are fixed: repeated `--input name=value` and `--describe-inputs` on `skl skill --resource <owner-relative-name> ... <owner>`, with flags before the owner. They are resource-only flags, not standalone Skill Definition variables. Resource-specific input names, small helpers, and exact authored file layout are delegated implementation details; earlier illustrative names are not an API contract.
- Declare each resource's accepted scalars using existing `urfave/cli/v2` flag types, typed destinations, `Required`, and `Usage`. Apply those declarations to a fresh local stdlib `flag.FlagSet`. Small shared glue checks duplicates and required presence and performs resource-local semantic validation before rendering. Explicit boolean `false` satisfies presence. Descriptions come from these same declarations and their usage/choice information, not a second schema.
- Split raw assignments only at the first `=`. Do not let `StringSliceFlag` comma splitting or trimming alter values. Keep any raw-input collector's behavior local to this flag; do not change application-wide separator behavior or unrelated flags. All mutable destinations and presence tracking are per call.
- Validate syntax, declared types, and applicable semantics, including accepted procedure choices and positive review rounds, before producing resource instructions. Fail with the offending name and a correction or discovery route. Do not fall back to generic prose or emit partial procedural output. Validation does not query the Backend, inspect Git, or require referenced output files to exist.
- Use the shared Go `text/template` renderer for Skill Definitions, included definitions, and parameterized resources. Feed ordinary typed data to rendering, never a CLI context or arbitrary input-name map exposed as template variables. Parse only embedded authored templates. Values and external evidence are execution data, never parsed again as template source.
- Organize authored Skill Modules around coherent procedures and use named templates where useful, not one file per sentence. Keep ownership consistent between definitions, includes, and resource lookup. Ensure private authored modules are embedded but excluded from both enumeration and lookup; `SKILL.md` is not a resource. Do not build speculative Propose variants or a configurable composition framework.
- Parameterize the three real resources only where procedure differences or bound references justify it. Submission instructions distinguish first implementation from finding-driven Rework; decision instructions distinguish whether work needs preservation; review instructions bind the round, original reviewed head, and relevant references. Do not require Audit prose, finding text, a verdict, future resolution commits, or every eventual document field as rendering inputs.
- The Workflow Engine establishes procedure facts from authoritative state already available to the invocation. If the existing facts need an explicit procedure field, populate it where that decision is made, without additional reads. Templates must not infer finding-driven Rework from comment presence, branch naming, or merely the existence of a draft Submission. Preserve #40's metadata-only startup boundary; a known reference does not prove its contents or ancestry have been checked.
- Parent resource commands bind every relevant argument already established by their invocation, including concrete paths and the original review identity. Do not leave settled values as placeholders or ask the worker to reconstruct them. Use shell quoting that preserves exact argument values. For preservation that becomes known only after work, leave only that value to be filled at the decision step and explain its boolean meaning. Bind it too if already established for that invocation.
- Keep submission retrieval at result writing, decision retrieval at the Needs Human step, and Watchdog review retrieval before dispositions. Preserve applicable future-dependent branches inside the review resource because its rules are needed to decide the verdict. Existing finding IDs survive a lost Review Checkpoint even when the supplied round restarts at one; round number is not evidence that no findings exist.
- Preserve human-directive authorization and precedence, the separation of semantic command flags from opaque Result Document prose, optional anchor transport, current Audit and Full Gate obligations, and human-owned Manual Verification and merge. This slice changes instruction delivery, not those rules.
- Convert all active callers and documentation together. A no-facts generic Implement/Watchdog definition may direct the worker to the concrete workflow invocation's resource command or `--describe-inputs`; it must not advertise a bare call that now lacks required inputs. Standalone skill disabling, full parent-procedure specialization, lifecycle presentation changes, and Pi retirement remain later slices.

## Module shapes & seams

### Modified: CLI resource retrieval

Interface: the existing `skill` command in `cmd/skl/main.go`, extended with the two resource flags and description mode. Dependencies are the already-installed CLI library, stdlib flag parsing, and the local embedded catalog. Resource requests must not construct a Backend or enter Workflow operations.

Keep typed declarations and validation behind this interface, passing only validated ordinary data into rendering. The existing `newApp` test seam exercises successful variants, descriptions, errors, explicit false, statelessness, and literal input handling. No new framework or public test-only API is needed.

### Modified: Embedded skill rendering and resource ownership

Interface: the catalog's existing packet-building and named-resource retrieval paths in `catalog.go`, backed by the embedded assets in `distribution.go`. Generalize the existing definition rendering path rather than adding a parallel resource-only rendering mechanism. Keep public ownership lookup and private module composition coherent inside this module.

Dependencies are local authored assets and stdlib `text/template`; no runtime external service is needed. Validate public behavior through CLI retrieval and the existing embedded-distribution checks, including once-only inclusion, unchanged context-free references, unavailable private modules, and inert template-looking input data. Do not use equality with the authored template source to prove rendered behavior.

### Modified: Deferred resource callers and authored procedures

Interface: resource commands in the concrete Implement/Watchdog instructions and the three existing public resource paths under `skills/dev/implement/reference/` and `skills/dev/watchdog/reference/`. Their inputs come from existing invocation facts and genuinely later worker-established values. Extend fact plumbing only for explicit procedure decisions already available without new inspection.

Use the existing startup/resume command fixtures when exercising the parent-to-resource path. Capture the emitted command, fill only its documented late value when needed, and exercise its shell argument semantics before passing the resulting arguments through the in-process resource CLI. Compare against independently specified literal arguments and expected procedural obligations. `strings.Fields` is not a shell parser and must not be the oracle for quoted commands. Reuse the repository's process tools rather than adding a shell-parsing dependency or test framework.

Update `cmd/skl/main_test.go` retrieval and documented-command coverage, especially `TestDocumentedImplementResourceCommands` and bundled-definition source-equality assumptions affected by templating. Retain useful context-free regression checks, but replace affected source-equality assertions with independent rendered outcomes and applicable/absent procedure assertions. Resource-only tests must fail if Backend access is attempted. Parent fixtures may use the existing in-memory Backend without adding production queries.

## Sequence

1. Wait for #40 to be Merged, incorporate its required code, and trace its resulting invocation facts and metadata-only startup behavior. Do not apply ADR 0005.
2. Start with one real Implement submission variant through the public resource CLI. In its red-green cycle, connect typed parsing, the shared renderer, and the real resource; do not stop at synthetic renderer coverage.
3. Complete the remaining named scenarios one red-green cycle at a time, using focused tables for the listed variants and error classes rather than a Cartesian product or per-field quota.
4. Update every active converted caller and README example with the chosen input names, discovery guidance, and a runnable deferred flow. Retain no-input examples for context-free resources.
5. Run focused retrieval/parent-flow checks, then follow the existing Audit and artifact-completion process, with the repository Full Gate including `go test ./...` at Audit. Review the diff for extra startup reads, accidental public modules, changed flag behavior, or later-slice work. The automated parent-to-resource demonstration is sufficient; no genuinely human-only verification is required.
