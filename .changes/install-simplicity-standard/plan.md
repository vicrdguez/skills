# Install the repository-wide simplicity standard Plan

## Approach

Extend Setup's existing managed `AGENTS.md` block with the accepted standing standard. Remove the optional Ponytail wiring from the authoritative Audit definition so existing embedding and instruction retrieval deliver the replacement without new plumbing.

Implementation starts only after #8 is Merged. References below describe the existing seams at proposal time; follow their post-#8 equivalents without reverting or reimplementing #3. ADR 0001 governs embedded instruction delivery; ADR 0004 preserves existing agent behavior. This Proposal explicitly authorizes only the expanded Setup guidance and Audit removals listed here.

## Implementation decisions

- The standard is a default repository-wide principle, not a Workflow stage, independent skill, configuration choice, or special review pass.
- Setup owns and maintains the installed section inside its existing workflow markers. Preserve every existing bootstrap instruction and all guidance outside that block. Do not deduplicate or rewrite user-authored text outside the block.
- Use the existing Setup text source (`setup.AgentsBlock` at proposal time); no new runtime resource, loader, or indirection is required. An independent expected-text fixture in tests is permitted and is not a second runtime authority.
- Instruction Packets do not repeat the new section. Existing overlapping meanings in Skill Definitions and resources remain untouched; they are not prohibited duplication for this change.
- No changes to implement, design, TDD, Watchdog, their resources, or the Audit smell baseline are authorized. This expressly excludes the three optional pointer/deduplication edits discussed and rejected during exploration.
- Preserve Audit's existing precedence and finding-classification behavior. The installed standard participates as repository guidance through that existing behavior, not through a new lens or special severity rule.
- Evaluate preservation against the Target Snapshot observed after #8 at Work Start, not against pre-#3 skill text in this Artifact Baseline. Subsequent required target synchronization must not be mistaken for changes authorized by this Work Item.

### Exact installed section

Install the following Markdown within the managed block, in addition to its existing Workflow entrypoint guidance. Preserve the wording and headings; the fence itself is not installed.

```markdown
## Simplicity

Choose the least complexity that satisfies the accepted scope, repository
standards, and required verification. Preserve relevant invariants, error
handling, security, and accessibility. When a simpler approach would change
the agreed behavior, raise that trade-off rather than silently reducing scope.

### Understand and Reuse

Trace the affected behavior and relevant callers before choosing a solution.
Fix causes at the boundary responsible for them, rather than patching symptoms
in individual callers.

Look for adequate existing code, standard-library features, native platform
capabilities, and installed dependencies before adding a mechanism. Check
that a candidate actually satisfies the required semantics and failure modes;
availability alone does not make it suitable.

### Justify Structure

Introduce abstractions, configuration, dependencies, and test infrastructure
for concrete current needs. Prefer the approach that leaves less for callers
and maintainers to understand, rather than the fewest lines or files.

Apply the deletion test while preserving behavior: if removing an abstraction
eliminates complexity, simplify it; if it spreads responsibilities into callers,
keep those responsibilities together. A boundary can earn its keep through
encapsulation or testability without multiple production implementations.

### Keep Tests Direct

Test observable behavior at the agreed seams, using the project's existing
test tools. Keep setup direct and expected results independent of the
implementation.

Ask what meaningful regression would lose protection if a test disappeared.
Keep distinct regression protection; simplify repeated setup and incidental
implementation coupling. Remove redundant or implementation-coupled tests
only when their behavioral and failure-mode protection remains covered at
the appropriate interface.

### Review Concrete Alternatives

Judge simplifications by the burden they remove, not lines saved.

For a proposed simplification, identify the simpler alternative, the burden
it removes, and why required behavior and verification remain intact.
A named code smell is not sufficient justification.

Keep cleanup within the change's scope; raise unrelated opportunities
separately.
```

### Exhaustive permitted Audit edits

In `skills/dev/audit/SKILL.md`, or its authoritative post-#8 location, remove only the following integration. Line references are navigational references to the proposal-time file, not a requirement to preserve obsolete #3 mechanics.

1. Delete the `Optional Ponytail review` subsection: discovery, Standards-only activation, and Ponytail-specific classification and separation (currently lines 55-59).
2. Remove the sentence passing `ponytail-review` to the Standards task from the Pi dispatch instruction (currently line 73). Preserve the rest of dispatch behavior.
3. Delete the Standards brief bullet requiring the additional lens, `### Ponytail review` subsection, concise findings, and net-lines estimate (currently line 82).
4. In the ordinary Standards brief, remove the Ponytail parenthetical, conditional 750-word allowance, and Ponytail one-line-format requirement (currently line 83). Retain the normal 500-word allowance and all other instructions.
5. Remove the sentence retaining the Ponytail subsection during aggregation and the phrase counting Ponytail findings in Standards (currently lines 98 and 100). Preserve the remaining aggregation instructions.

No other Audit wording or behavior is to be consolidated, reclassified, reordered, or rewritten. If #3 has already removed a listed fragment, leave it absent rather than recreating it.

### Module shapes and seams

#### Modified: Setup

- Public interface: existing `skl setup`, including its existing `--repo` override.
- Responsibilities: install/refresh owned repository guidance while retaining existing validation, ownership, and idempotence behavior.
- Dependencies: existing local Git/filesystem fixtures and existing GitHub test adapter; no new seam or live-forge requirement.
- Tests: extend the existing public CLI Setup tests in `cmd/skl/main_test.go` or their post-#8 equivalents. Retain the existing ownership-marker, surrounding-guidance, and repeat-run checks rather than duplicating their suites.
- Check the actual installed section against independent expected text transcribed from this accepted section, not against the production text constant alone.

#### Modified: delivered Audit instructions

- Public interface: existing `skl skill audit`, `skl skill --format json audit`, and equivalent `implement` retrieval for bundled Audit.
- Responsibilities: deliver the same updated Audit definition directly and as a supporting definition, with equivalent Markdown/JSON content and unchanged non-Audit definitions/resources.
- Dependencies: existing embedded catalog and CLI test infrastructure. No new runtime supporting definition or resource.
- Tests: reuse the existing rendered/typed retrieval and guaranteed-supporting-definition checks. Table-driven format cases are appropriate. Inspect the delivered content and manifest, not internal helper calls.
- Review the source diff against the post-#8 Target Snapshot to establish that every non-Audit definition/resource and every non-Ponytail part of Audit is unchanged. Avoid introducing a permanent snapshot suite for every unrelated skill.

The tests establish instruction delivery, not model obedience or a reduction in generated line counts. Simplicity remains Agent Worker judgment. Use the existing Full Gate without adding an evaluator or new test framework.

## Sequence

1. Confirm #8 is Merged and use the normal Work Start Target Snapshot and Git preparation.
2. Materialize each `behavior.md` scenario through the agreed CLI seams with existing red-to-green practices.
3. Complete the permitted Audit removals and verify source-diff preservation without editing other skills or resources.
4. Update the Setup capability doc from planned to delivered behavior. If the planned section is no longer present after #3, add the delivered behavior without altering the settled scope.
5. Run the existing implementation-phase Audit and Full Gate, and hand off under the post-#3 Workflow, including its Implementation Ledger retirement rules.
