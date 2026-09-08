# Install the repository-wide simplicity standard

## Why

Ponytail supplies useful simplicity pressure but also carries a persona, line-count objectives, and testing prescriptions that this artifact-driven Workflow does not need. Most scope, design, testing, and review safeguards already exist. A concise but actionable repository standard should guide implementation choices and review without rewriting those safeguards.

## What

Normal `skl setup` installs and maintains the exact Simplicity section accepted in `plan.md` within its managed `AGENTS.md` block. It applies to coding and review both inside and outside the Workflow. Audit no longer discovers or invokes `ponytail-review`; it considers the installed standard through its existing repository-standards behavior.

This is one Work Item, `install-simplicity-standard`, blocked by #8 (`review-and-complete-work-items`) reaching Merged. The chain #4 -> #5 -> #6 -> #7 -> #8 places implementation after all of Coordination Item #3. Ready for Merge or issue closure alone does not satisfy this Dependency.

## Scope

- Install the approved wording by default on fresh Setup and refresh it during subsequent Setup, using the existing marker-owned block.
- Preserve surrounding Consumer Repository guidance, existing Workflow bootstrap instructions, and Setup's established ownership and safety behavior.
- Keep one authoritative runtime source for the installed text in the existing Setup guidance mechanism; do not duplicate the standard in Instruction Packets or add a Skill Definition or Skill Resource.
- Remove only the enumerated Ponytail activation, dispatch, reporting, and aggregation instructions from Audit.
- Preserve all other Skill Definitions and resources, including overlapping simplicity instructions in implement, design, and TDD.
- Verify the delivered guidance through existing public CLI seams and update the Setup capability documentation.

## Out of Scope

- Any of the rejected optional edits to implement, design, or TDD, or general consolidation of overlapping instructions.
- Changing Audit beyond removal of its Ponytail integration, including its ordinary finding classifications, smell baseline, two axes, or verification requirements.
- Altering Watchdog Review, Full Gates, TDD sequencing, agreed test seams, or scenario-to-test obligations.
- PR size limits, reviewability policy, Proposal slicing changes, or line-count targets.
- New skills, configuration, opt-ins, CLI commands, harness adapters, evaluators, or model-compliance benchmarks.
- Changing personal Ponytail installations, arbitrary guidance outside the owned block, or unsupported Consumer Repository overrides.
- Changing Workflow Mechanics or fixing the publisher's support for Dependencies outside a Proposal.

## Definition of Done

- [x] Fresh Setup installs the exact approved Simplicity section once without requiring a simplicity-specific choice.
- [x] Setup refreshes an existing owned block with that section while preserving surrounding user-authored guidance and existing Workflow entrypoint instructions.
- [x] Repeating Setup with the same choices leaves the resulting guidance unchanged and does not duplicate the section.
- [x] Direct Audit instruction retrieval in Markdown and JSON omits the complete Ponytail integration and does not inject the Simplicity section; the ordinary Audit instructions remain intact.
- [x] Implementation instruction retrieval in Markdown and JSON includes the same Ponytail-free Audit definition, without injecting the Simplicity section or introducing a new supporting skill or resource.
- [x] The implementation diff changes no other Skill Definition or resource and changes Audit only as enumerated in `plan.md`, compared with the post-#8 Target Snapshot at Work Start.
- [x] Existing relevant regression checks and the Consumer Repository Full Gate pass; the capability documentation describes the delivered behavior rather than a planned extension.

## Manual verification

None. Verification establishes installed and retrieved instructions, not deterministic compliance by an Agent Worker.
