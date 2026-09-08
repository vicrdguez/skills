# Fix embedded resource delegation Plan

## Approach
Follow ADR 0001: author Markdown, embed it in `skl`, discover thin stubs, retrieve supporting resources by name. The runtime already supports this. Correct the pointers at their existing locations rather than adding a generalized interpretation rule or changing packet rendering.

## Implementation decisions
- Keep edits surgical: replace transport wording only, preserving leading conditions, ordering, mandatory/optional distinctions, responsibilities, and unrelated prose.
- Use concrete commands with flags before the skill name. Resource names are exact and relative to the owning skill, not to the current resource. For example: `skl skill --resource reference/DEEPENING.md design`.
- A parent definition is retrieved with `skl skill <name>`, only when not already present. `--resource SKILL.md` is not a supported substitute.
- Do not add shared packet boilerplate, a Markdown parser, resource aliases, filesystem fallback, or production Go changes to make old pointers work.
- Preserve consumer-file references, including `CONTEXT.md`, ADR/capability output locations, standards files, `.changes/`, `.worktrees/`, and `.thinking/` inputs. They have a different lifetime and purpose from embedded resources.
- Leave already-correct resource delegation and skill frontmatter unchanged.
- Keep personal cutover settings outside this Work Item and outside its implementation commits.

### Pointer inventory
The accepted baseline contains these actionable locations. Correct repeated mentions within the indicated passages without expanding their surrounding behavior:

| File | Pointer-sized correction |
| --- | --- |
| `skills/misc/writing-for-agents/SKILL.md` | Both mechanics pointers retrieve root-level `SKILL-MECHANICS.md` from `writing-for-agents`. |
| `skills/misc/writing-for-agents/SKILL-MECHANICS.md` | Identify the parent Writing for Agents definition through CLI retrieval when needed; remaining parent mentions refer to that definition rather than a local `SKILL.md`. |
| `skills/dev/domain/SKILL.md` | Retrieve the three format resources through their exact `reference/` names owned by `domain`. |
| `skills/dev/design/SKILL.md` | Retrieve `reference/DEEPENING.md` and `reference/DESIGN-IT-TWICE.md` through `design`. |
| `skills/dev/design/reference/DEEPENING.md` | Replace the parent `../SKILL.md` assumption with the Design definition, fetched only if needed. |
| `skills/dev/design/reference/DESIGN-IT-TWICE.md` | Correct parent-definition and sibling DEEPENING pointers, including those handed to subagents. |
| `skills/dev/propose/SKILL.md` | Retrieve the four existing artifact templates through `propose`, retaining mandatory versus conditional use. |
| `skills/dev/propose/reference/tasks.md` | Name the existing capability format retrieval command owned by `domain`. |
| `skills/dev/tdd/SKILL.md` | Retrieve tests and mocking resources through `tdd`. |
| `skills/dev/audit/SKILL.md` | Retrieve `reference/smells.md` through `audit`; supply that command in the Standards brief instead of an absolute resource path. |

### README
Extend the existing installation/skill-retrieval guidance, rather than adding a separate guide. Explain that `skills/` is authoring input, installed `SKILL.md` files are discovery stubs, and the binary owns runtime definitions/resources. Document exact owner-relative resource names, including a root-level example, and the `go install ./cmd/skl` followed by `skl install` refresh sequence. Note that raw source-tree registrations bypass this arrangement; OpenCode can consume the installed Claude-compatible stubs. Keep personal machine paths out of the tracked README.

### Module shapes and seams

#### [MODIFIED] Instruction Catalog content
The public interface remains `skl skill [--format json] <name>` and `skl skill --resource <resource> <name>`. Definitions and resources change only their retrieval pointers. No new seam or runtime implementation is required.

#### [MODIFIED] CLI verification
Use the existing command seam, `newApp` / `newAppWithSkillHome` and `Run`, in `cmd/skl/main_test.go`. Reuse temporary home/working-directory and buffer patterns. One bounded table-driven smoke check covers installation plus corrected source-free retrieval; update existing assertions that pin obsolete pointer syntax.

Use literal headings, delegation commands, and relevant content fragments for meaningful expectations. For each covered pointer, verify both the instruction naming the command and the content retrieved by it: testing resource retrieval alone would already pass before this change. Include standalone, nested, cross-skill, and bundled cases. Do not use `readRepositoryFile` or other source-file reads to construct smoke expectations. Do not add a generic command/link scanner, duplicate per-function suites, a binary-build framework, or a live harness integration harness.

### Verification
- Materialize the behavior scenarios at the existing CLI seam, preserving the no-real-home/no-backend-side-effect constraint.
- Check that restoring a representative broken pointer fails the relevant smoke assertion; retain production behavior outside pointer transport.
- Run `go test -count=1 ./...`, `go vet ./...`, `go build ./...`, `node --test prompts/queue-next.test.mjs`, `gofmt -l cmd setup workflow catalog.go distribution.go`, and `git diff --check`.
- Review the instruction diff for semantic drift and README for consistency with the existing commands. This does not require live harness sessions or checking human-owned merge behavior.
