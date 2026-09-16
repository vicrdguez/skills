# Render Typed Deferred Resources

Slug: `render-deferred-resources`

Blocked by: #40

## Why

Deferred Skill Resources currently return static instructions even when an invocation already establishes the applicable procedure and references. Workers must reconstruct those facts, and the current bare retrieval commands cannot support required resource inputs. Deliver real specialized resources and valid callers together, rather than a renderer-only foundation.

## What

Extend `skl skill --resource <owner-relative-name> [--input name=value ...] <owner>` with stateless typed rendering and `--describe-inputs`. Convert Implement's submission and decision resources and Watchdog's review resource, binding settled inputs in their parent instructions while keeping genuinely later decisions deferred. Reuse one authored-template renderer for definitions, included skills, and resources.

## Scope

- Implement `--input` and `--describe-inputs` for named resources, with resource-specific typed declarations, actionable validation, and lossless values.
- Convert `reference/submission.md` and `reference/decision.md` under Implement and `reference/review.md` under Watchdog into complete applicable instructions, not demonstration templates.
- Update converted callers and retrieval documentation together. Bind every relevant value already established by the invocation; explain only genuinely later-known inputs.
- Preserve deferred retrieval, Result Document opacity, finding identities, human-directive rules, and existing Workflow Mechanics.
- Share Go `text/template` rendering across definitions, includes, and parameterized resources. Keep context-free resources available and private Skill Modules outside the public resource surface.
- Verify through the existing in-process CLI and embedded distribution, with independent expected outcomes and no Workflow Backend access during resource rendering.
- Publish with the approved advisory issue prefix `Blocked by: #40`, not an engine-native external Dependency. Implementation must wait until #40 is Merged and incorporate its required code. The branch starts at `c691fdf`, including #37/#39 and the durable #44 decisions; incorporating #40 is ordinary prerequisite incorporation, not a general target-sync policy.

## Out of Scope

- Later #44 slices for complete Implement/Watchdog Execution Skill specialization, lifecycle output defaults, standalone skill disabling, and Pi loop retirement or redesign.
- Eliminating unrelated generic main-skill branches, inventing Propose variants, or building a general composition framework.
- New Backend queries, Git inspection, or early evidence/resource loading solely to specialize instructions; saved invocation sessions or prerendered resource bundles.
- Arbitrary template-variable injection, a new input schema language, Consumer Repository template overrides, or making future Result Document prose into CLI inputs.
- Changes to Claims, eligibility, transitions, judgment obligations, human merge authority, or general target synchronization.
- ADR 0005 implementation. Its new artifact format, optional TDD, and pre-Audit synchronization follow all #44 slices and do not apply here.

## Definition of Done

- [ ] DOD1: `--describe-inputs` describes accepted names, meanings, types and any allowed choices, and required status for each requested resource without requiring values or rendering procedural content.
- [ ] DOD2: Implement submission retrieval renders the applicable first-implementation or finding-driven Rework instructions with bound references, current Audit evidence requirements, and unchanged opaque Result Document semantics.
- [ ] DOD3: Implement decision retrieval specializes preservation instructions from an explicitly supplied boolean, accepting `false` as present and retaining the existing Needs Human and draft-preservation obligations.
- [ ] DOD4: Watchdog review retrieval binds the review round and original reviewed head before dispositions, preserves existing finding IDs even after checkpoint loss, and retains authorization, precedence, and opaque result transport rules without requiring a verdict or finding choices.
- [ ] DOD5: Malformed, unknown, duplicate, missing-required, invalid-type, and semantically invalid inputs fail actionably before rendering, without generic fallback, partial procedural output, or mutation.
- [ ] DOD6: Repeated inputs preserve string spaces, commas, embedded equals, quotes, and literal template-looking data across independent calls; generated commands preserve those values through shell quoting, and other commands' flag behavior is unchanged.
- [ ] DOD7: A worker can take each converted resource command from a concrete parent invocation, supply only explained later-established values, and retrieve complete applicable instructions through the public CLI at the existing procedural step. Callers and docs advertise no invalid bare retrieval of converted resources.
- [ ] DOD8: The shared renderer preserves context-free no-input retrieval, owner-relative references, and once-only bundled definitions without eagerly disclosing deferred resource content or changing existing output defaults.
- [ ] DOD9: Embedded private Skill Modules are neither listed as public resources nor retrievable or describable as resources; public resource ownership remains exact.

## Manual verification

None.
