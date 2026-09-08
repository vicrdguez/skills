# Install native OpenCode Skill Stubs Plan

## Approach
Extend the existing destination list in `distribution.go`. OpenCode uses the same common stub template and embedded catalog as the existing harnesses. There is no new module or runtime adapter to design.

## Implementation decisions
- Use `.config/opencode/skills` relative to the existing installer home argument, matching OpenCode's documented default global discovery directory. Custom configuration roots and new environment/flag handling are not part of this slice.
- Reuse the current `skl.stub/v1` ownership logic and frontmatter/delegation generation. Do not add an OpenCode-specific template or special-case skill instructions.
- Preserve user-owned collisions rather than silently taking them over. Native installation removes the need for Pi-directory discovery; it does not authorize deleting Pi files or rewriting personal settings.
- Keep queue-asset installation unchanged. This slice is basic skill discovery, independent of queue draining and resource-pointer proposal #21; shared files alone do not create a Dependency.
- Make only the small supported-harness/path changes needed in README, `docs/capabilities/skill-distribution.md`, and `docs/adr/0001-embed-skill-definitions-behind-harness-stubs.md`. Leave the independently planned queue extension and other workflow guidance untouched.

### Module and test seam
The existing distribution module exposes `Install(home)` through `skl install`. Test through `newAppWithSkillHome(...).Run` in `cmd/skl/main_test.go`, as the current installation tests do. Extend their harness cases to OpenCode and exercise the existing owned/unowned refresh checks at the new destination; reuse helpers rather than adding a parallel suite or framework.

Assertions must cover actual output files, preserved content, delegation, and the absence of a cross-harness link or generated OpenCode configuration. Existing Pi installation tests continue to establish the Pi-only queue-asset behavior. The normal gate does not launch OpenCode, require its installation, or touch a real home directory.

### Local cutover guidance
Document this order: build/install the new binary, run `skl install`, remove only obsolete Pi or raw-source entries from OpenCode's skill discovery settings, then restart and verify native discovery. Retain unrelated settings and any intentionally configured other skills. Never replace the Pi override with a dependency on Claude or Codex directories. The current workaround is kept until the native stubs are available; removing it is a separate explicit local operation, not an installer side effect.

OpenCode's documented default directory is confirmed by https://opencode.ai/docs/skills/#place-files. Live discovery after restart is the human-owned checklist item in `intent.md`; CLI output and file layout are agent-verifiable checks.

## Verification
- Demonstrate native installation fails its expectation before adding the new destination, then passes with the minimal implementation.
- Verify owned refresh, byte-stable repeat installation, unowned skill/configuration preservation, and retained existing harness output at the CLI seam.
- Run `go test -count=1 ./...`, `go vet ./...`, `go build ./...`, `node --test prompts/queue-next.test.mjs`, `gofmt -l cmd setup workflow catalog.go distribution.go`, and `git diff --check`.
- Keep all normal tests isolated from real personal configuration and live workflow state.
