# Embed skill definitions behind common filesystem stubs

Author harness-independent skill definitions and their named resources as Markdown in the Workflow Definition Repository, embed them in the versioned Workflow Engine binary, and install only thin filesystem stubs for discovery. At invocation, the engine deterministically specializes a definition from structured facts and can return equivalent rendered Markdown or typed JSON; it does not infer, summarize, or ask a model to tailor instructions. This keeps one authoritative behavior across harnesses while preserving their static discovery mechanisms without depending on plugin packaging formats.

## Consequences

- Every invoked skill incurs one CLI call after harness activation.
- Pure reasoning skills gain centralized distribution but usually receive unchanged instructions and no context reduction.
- Stub installation and binary versions must remain compatible.
- Supporting resources are embedded and retrieved by name instead of being injected into every instruction packet.
- Skill Definitions guaranteed for an invocation are bundled once and listed in the packet manifest; conditional dependencies remain deferred, and adapters do not activate stubs already listed in the packet.
- Harness-specific execution capabilities remain outside the shared skill definition unless selected through deterministic adapter metadata.
- Pi, Codex, and Claude Code receive equivalent skill discovery and one-item workflow operations in V1; automated queue draining may remain Pi-only.
- V1 installs the binary from this repository with Go. The binary then writes its embedded common Skill Stubs into the supported Pi, Codex, and Claude Code user skill directories; prebuilt releases, plugin packages, automatic updates, and uninstall machinery are deferred.
- Installation may also write harness-specific queue adapters where supported; deterministic Setup is a CLI command and has no Skill Stub.
- Skill templates use Go `text/template` with typed inputs and a deliberately small helper set; Consumer Repositories cannot override them in V1.
- Agent-authored handoff results use template-guided Markdown files in private temporary directories rather than command-line prose or tracked files. The CLI passes their contents through without parsing or validating the Markdown.
- A semantic command or typed flag carries each agent-decided outcome independently of its opaque Result Document. The engine validates and applies the requested transition but does not cross-check the decision against the prose.
- Packets and stubs carry protocol versions; compatibility guarantees begin at V1.
- The binary is named `skl` and exposes stage-oriented commands matching the existing propose, implement, and watchdog roles rather than generic state-machine or GitHub operations.
- Expected workflow outcomes are structured successful statuses; nonzero exits are reserved for invalid command inputs, invalid setup, unavailable dependencies, and unexpected failures.
- Commands discover the Consumer Repository from its Git root with an explicit adapter override.
- Successful publication removes its private temporary result directory; a failed handoff retains it for same-session repair and explicit later cleanup.
- A repairable deterministic precondition refusal returns `fix_required`, retains the Claim and current Workflow State, and supplies exact repair instructions to the current or next resumed Agent Worker. Result Document prose is not such a precondition; only contradictions without a safe deterministic repair enter Needs Human.
- Submission publication appends the machine-owned `Closes #<issue>` footer to the otherwise opaque agent-authored body so GitHub closes the source issue when the Merge Authority merges it.
