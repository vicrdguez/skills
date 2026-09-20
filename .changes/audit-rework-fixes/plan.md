# Focus Audit on finding-driven Rework Plan

## Approach
Specialize the already bundled Audit from the engine-established `Implementation.Procedure`. Keep the existing initial/resumed and standalone branches intact. For `rework`, instruct the worker to identify the latest applicable review that caused the current Rework from the supplied labeled evidence and use that review's `Commit` as the fixed point. Missing or ambiguous evidence is a stop condition, not a guessed comparison.

The Rework Audit reads only the resulting delta while retaining the frozen Contracts as its correctness oracle. It runs the Full Gate but relies on Implement's existing Inspect steps for immutable endpoint and retirement checks. Its output joins the existing cumulative Audit ledger with monotonic `F<n>` identities. Applying findings never recursively invokes Audit.

## Implementation decisions
- `Implementation.Procedure == "rework"` is the sole procedure selector; templates must not reconstruct Rework from branch names, Submission presence, or incidental comments.
- The Rework fixed point is the `Commit` on the latest applicable supplied review whose verdict caused the current Rework. Do not use `FinalHead`; do not add an invocation fact. Missing or ambiguous applicable evidence requires stopping rather than guessing.
- Audit is limited once per Implement execution, not once per Submission lifetime.
- The Rework review surface is `<reviewed-commit>...HEAD`. Reviewers may read surrounding code and callers to understand consequences, but findings must be caused by the delta.
- Keep both independent axes. Standards checks delta-caused conformance and smell problems. Contracts checks supplied-finding resolution and Contract safety caused by the delta; it does not repeat complete initial conformance review.
- Supplied Watchdog findings are evidence and resolution targets, never new frozen Contract Items.
- Audit owns one Rework Full Gate run. It does not run endpoint inspection; Implement owns Inspect before editing and after all edits.
- After Audit, apply required dispositions without another Audit. If a disposition changes code, run its affected checks and a final Full Gate.
- Audit Findings follow ADR 0006 in every procedure. Assign new findings monotonically from `F1` when no `F<n>` exists, otherwise continue after the greatest existing `F<n>`; preserve historical identifiers in other formats without retroactive renumbering.
- Keep one cumulative Audit ledger. Advance its audited head after a clean or finding-bearing Rework Audit; add no round-specific provenance section.
- Keep the change in authored templates, resources, rendered goldens, and direct rendering tests. Add no workflow state or backend behavior.

### Module shapes & seams

#### [MODIFIED] Implement Execution Skill
**Interface:** Rendered Markdown returned by `skl implement next` or `skl implement resume --item <number>` for the engine-selected procedure.

**Dependencies:** Typed `ImplementationFacts`, bundled Audit rendering, supplied labeled review evidence, and deferred Submission resource retrieval.

**Invariants:** One selected procedure; no discarded procedure residue; one Audit maximum per execution; Implement remains owner of before-and-after inspection and handoff.

**Test strategy:** Exercise the existing Implement command/rendering seam with finding-driven Rework fixtures. Assert the focused Audit, fixed-point selection rule, Full Gate ownership, no repeated endpoint check in Audit, and no second Audit. Preserve representative initial/resumed golden outputs except where shared `F<n>` vocabulary intentionally applies.

#### [MODIFIED] Bundled Audit Skill
**Interface:** Audit Markdown bundled into an Implement Execution Skill, specialized by `Implementation.Procedure`; standalone `skl skill audit` remains its existing interface.

**Dependencies:** Implement procedure, rendered review evidence, repository standards, frozen Contracts, and the Full Gate.

**Invariants:** Initial behavior remains exhaustive; Rework findings are delta-caused; both axes remain independent; Watchdog evidence never becomes a Contract; deterministic checks run once at their responsible stage; every new Audit Finding has a monotonic `F<n>` identity.

**Test strategy:** Verify the complete rendered Rework packet at the public command seam and retain the standalone Audit golden as a regression oracle.

#### [MODIFIED] Submission Result Document resource
**Interface:** Deferred Markdown returned by `skl skill --resource reference/submission.md --input ... implement`.

**Dependencies:** Existing opaque Submission body and Audit ledger.

**Invariants:** Historical identities are preserved; new Audit Findings start at or continue monotonic `F<n>` identities; a clean Audit adds no synthetic finding; the single ledger records the current audited head.

**Test strategy:** Render the Rework resource through the existing resource command and assert identity and cumulative-ledger guidance.

## Sequence
1. Add direct rendered-Rework expectations for one focused Audit, delta-scoped axes, responsible deterministic checks, convergence, and `F<n>` ledger behavior.
2. Specialize the Audit template for finding-driven Rework while leaving initial/resumed and standalone behavior unchanged.
3. Update Implement's Rework procedure and deferred Submission guidance to invoke, disposition, verify, and record that Audit once.
4. Refresh representative goldens and run focused rendering tests followed by the repository Full Gate.
