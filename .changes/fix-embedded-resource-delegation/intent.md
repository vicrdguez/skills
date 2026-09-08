# Fix embedded resource delegation

## Why
Skill Definitions and Skill Resources are embedded in `skl`, while Agent Harnesses discover only thin Skill Stubs. Several instructions still direct workers to relative source files or ask for an absolute path to an embedded resource. Those pointers work accidentally in the Workflow Definition Repository but fail to tell workers how to retrieve the same material in a Consumer Repository.

## What
Surgically replace incorrect source-file pointers with the existing CLI retrieval commands, document the source-versus-runtime distinction in README, and prove the path through installed stubs and embedded retrieval with a source-independent smoke check.

## Scope
- Correct resource delegation in the affected Audit, Design, Domain, Propose, TDD, and Writing for Agents definitions and their linked resources.
- Use exact resource names relative to the owning skill, including nested and cross-skill pointers.
- Retrieve a parent Skill Definition with `skl skill <name>` when it is not already supplied; do not treat `SKILL.md` as a resource.
- Replace Audit's requirement for an absolute embedded smell-baseline path with a concrete CLI retrieval command in the Standards brief.
- Preserve each pointer's existing activation condition and the surrounding instruction semantics.
- Update README's installation guidance to distinguish authoring sources, installed stubs, and the running binary's embedded definitions/resources; document resource ownership and rebuild/reinstall steps.
- Add one bounded, table-driven smoke check through the existing CLI command seam from a temporary working directory and temporary skill home. Check installed delegation, corrected instruction pointers, and meaningful retrieved resource content without consulting source files for expectations.
- Update existing assertions that intentionally mention the replaced pointer syntax.

## Out of Scope
- Rewriting skill bodies, frontmatter, review rules, activation policy, or Workflow Mechanics.
- Changing the packet schema, dependency bundling, resource lookup, CLI flags, or installer behavior.
- Adding a Markdown/link parser, resource-copying layer, fallback path resolution, or new test framework.
- Rewriting Consumer Repository paths, artifact paths, code examples, or user-provided `.thinking/` paths as resource requests.
- Changing already-correct Implement and Watchdog resource commands.
- Native OpenCode installation, queue changes, release automation, automatic updates, or uninstall/migration machinery.
- Editing personal harness settings or performing the user's installation cutover as part of this tracked Work Item. That separately authorized local operation follows proposal publication and installs existing merged V1.

## Definition of Done
- [x] Every identified incorrect embedded-resource pointer uses an exact existing CLI command with its owning skill; parent definitions use definition retrieval rather than resource retrieval.
- [x] Nested, cross-skill, and bundled-definition instructions retain correct resource ownership without depending on source-tree paths or rereading already-supplied parent definitions.
- [x] Audit's Standards brief can obtain the smell baseline through `skl` while retaining ordinary filesystem paths for Consumer Repository standards files.
- [x] A runnable source-independent smoke check verifies installed stubs, corrected delegation instructions, and retrieved contents at the CLI seam. Restoring a covered broken pointer makes the check fail.
- [x] README explains the authoring/runtime distinction, exact resource retrieval, and the rebuild/install lifecycle without introducing a second distribution mechanism.
- [x] The diff is limited to pointer-sized instruction edits, README guidance, and necessary tests; the complete documented Go/Node gate remains green.

## Manual verification
None. This slice changes instruction transport pointers and documentation, not live harness session behavior.
