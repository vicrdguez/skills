# Install portable skills Plan

## Approach
Add an embedded Instruction Catalog behind a small retrieval interface and an Installer that copies embedded common stubs into the three harness user skill locations. The command surface is the highest seam: tests invoke installation and retrieval with an isolated user root and compare observable files and payloads. No harness plugin format or dynamic extension protocol is introduced.

Skill Markdown remains editable source in this repository. `go:embed` packages it into the binary. `text/template` performs deterministic specialization from typed facts; it never asks a model to rewrite instructions.

## Implementation decisions
- `skl install` installs stubs for Pi, Codex, and Claude Code at user scope. Test-only dependency injection redirects user directories; no persistent installation configuration is written.
- Common stubs are embedded assets, not generated from harness manifests. Harness-specific directory paths are installer facts; the stub behavior is shared.
- Omit `dev-setup`; its behavior is the direct `skl setup` command from the blocking slice.
- A stub contains discovery metadata, the exact `skl` command to run, and its protocol version. It does not duplicate its Skill Definition.
- Non-stage definitions are retrieved through the `skl skill` command. Propose, Implement, and Watchdog stubs delegate to their semantic command families as those slices add them.
- Markdown is the default human/agent payload. JSON carries explicit protocol version, primary skill, `included_skills`, structured invocation facts, available named resources, and the rendered instructions.
- Markdown and JSON are two projections of the same typed packet, not independently authored templates.
- Named resources remain individually retrievable. Do not inject every reference file into every packet.
- Bundle only unconditional dependencies: Explore includes Domain; Propose includes Design and TDD; Implement includes TDD, Audit, Design, and Domain; Watchdog has no supporting bundle. A bundled invocation renders each canonical definition once.
- Preserve optional dependency behavior such as Ponytail review without building conditional dependency resolution in V1.
- Use only a small reviewed helper set around Go `text/template`. Missing or invalid typed inputs fail rather than silently rendering placeholders.
- Repository-local overrides and runtime-downloaded definitions do not exist in V1.
- Retain checked-in Pi/Claude packaging and Pi queue adapters during the staged migration; the final lifecycle slice retires them only after every stage has equivalent CLI-backed installation.

### Module shapes & seams

#### [NEW] Instruction Catalog
**Interface:** resolve a Skill Definition by canonical name, build an Instruction Packet from typed invocation facts, and retrieve a named Skill Resource.

The Catalog owns embedded sources, dependency composition, deduplication, protocol metadata, and deterministic rendering. Callers never concatenate skills or inspect embed paths.

Dependencies: embedded files and Go `text/template`, both in-process.

**Test strategy:** test complete packets through catalog requests using fixed literals. Golden files are appropriate for stable rendered instructions and stub assets; JSON assertions use decoded typed values rather than textual field order.

#### [NEW] Stub Installer
**Interface:** install the embedded owned stubs into a supplied set of supported user skill roots and return the changed/unchanged outcome.

The Installer owns path mapping, directory creation, ownership boundaries, and idempotent replacement. It never interprets plugin manifests or scans Consumer Repositories.

Dependencies: local filesystem, exercised through isolated temporary directories.

**Test strategy:** invoke through `skl install`, inspect only resulting files, and prove unrelated files survive repeated installation.

#### [MODIFIED] CLI
**Interface:** add `skl install` and skill/resource retrieval with Markdown default and explicit JSON output.

Keep output deterministic: requested payload on stdout, diagnostics on stderr, nonzero only for invalid invocation or operational failure.

**Test strategy:** command-level scenarios use the same application interface as `main` and the real Instruction Catalog.

## Sequence
1. Embed authoritative skill sources, resources, and common stub assets.
2. Implement Instruction Packet composition, rendering, JSON projection, resource lookup, and once-only dependency bundling.
3. Add idempotent user-level stub installation for all three harnesses.
4. Wire install and retrieval commands through the existing CLI seam.
5. Keep legacy plugin/package and Pi queue installation available until the completed lifecycle can retire it atomically.
6. Update skill-distribution documentation and manual harness checks.
