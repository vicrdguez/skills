# Install portable skills

## Why
The repository currently depends on Pi package metadata and Claude plugin manifests to expose duplicated filesystem skills, while Codex has no declared installation path. That couples distribution to harness packaging and leaves deterministic instruction specialization inside long static prompts.

## What
Embed this repository's Skill Definitions, resources, templates, and common Skill Stubs in `skl`; let the Go-installed binary install the stubs for Pi, Codex, and Claude Code and return one authoritative Instruction Packet in rendered Markdown or typed JSON.

## Scope
- Embed every current Skill Definition and supporting resource except the retired `dev-setup`
- Keep repository Markdown files as the authoritative skill sources at build time
- Embed one common filesystem Skill Stub per installed skill
- Install user-level stubs directly into the supported Pi, Codex, and Claude Code skill directories
- Avoid Pi packages, Claude plugins, Codex plugins, marketplaces, and other harness packaging formats
- Make repeated stub installation idempotent and replace only files owned by `skl`
- Let each stub delegate to the appropriate `skl` instruction or workflow-stage command
- Return non-workflow Skill Definitions through a `skl skill` command
- Return Instruction Packets as rendered Markdown by default and equivalent typed JSON on request
- Embed named Skill Resources and disclose them only when explicitly requested or required
- Bundle guaranteed supporting Skill Definitions once in an Instruction Packet
- List bundled definitions in `included_skills` so adapters do not activate them again
- Use Go `text/template`, typed inputs, and a deliberately small helper set for deterministic specialization
- Carry protocol versions in packets and stubs
- Keep Consumer Repository overrides unsupported

## Out of Scope
- Prebuilt releases or package-manager distribution
- Automatic updates, uninstall management, or plugin marketplaces
- Starting, scheduling, or controlling Agent Harness sessions
- Implementing proposal, implementation, or watchdog state transitions
- A general skill-authoring or runtime plugin system
- Conditional supporting-skill dependency resolution beyond named resource retrieval

## Definition of Done
- [x] `skl install` writes owned common Skill Stubs for every supported current skill into Pi, Codex, and Claude Code user skill locations without using plugin packaging.
- [x] Repeating installation refreshes owned stubs without changing unrelated harness files.
- [x] Invoking a non-workflow skill through `skl` returns its authoritative embedded instructions as rendered Markdown.
- [x] Requesting JSON returns the same Instruction Packet facts and instructions in a typed machine-readable representation.
- [x] Named supporting resources remain absent from the initial packet and can be retrieved explicitly from the embedded catalog.
- [x] Guaranteed supporting Skill Definitions are bundled exactly once and declared in `included_skills`.
- [x] Stubs and packets expose compatible protocol versions and no Consumer Repository skill override is read.

## Manual verification
- [ ] Invoke one installed stub in each of Pi, Codex, and Claude Code and confirm that the harness discovers it and follows the delegated `skl` instruction.
