# Compose Execution Skills from Procedures and Craft

Execution Skills were meant to shrink what an Agent Worker reads by moving deterministic complexity into the Workflow Engine. Instead, specialization accreted: rendered skills carry workflow policy, rejected alternatives, prohibitions and a retired GitHub mode alongside the instructions. Ledger Implement renders about 4,500 words, against about 900 before `skl`. We will compose every Execution Skill from one Procedure (the invocation's script) plus the Craft it needs (judgment guidance written once). Workflow policy stays in ADRs and documentation, and the result is measured on rendered goldens. This deliberately rewrites agent judgment prose once to establish a new baseline, superseding ADR 0004's surgical-edit consequence for that re-baseline. Afterwards, prose changes are surgical again.

## Status

Accepted after final Explore confirmation; implementation is pending.

## Consequences

- Only the ledger path is tuned. The foundation deletes the GitHub-mode templates, resources and goldens, plus the code that only tests reach. A separate Proposal retires the fallbacks for repositories that have not adopted the ledger.
- Craft carries no invocation facts. A Procedure inlines what every run needs and points to what only some runs reach. Implement no longer bundles design or domain, and Propose points to design and testing instead of bundling them.
- Execution Skills carry no frontmatter, packet header or included-skill framing.
- Workflow Engine responses are Outcome Instructions: each states what happened and the worker's next step, so Procedures no longer explain status codes. JSON output remains available to programmatic callers.
- Subagent guidance is harness-agnostic, and the execution-capability specialization is removed.
- The Implement and Watchdog entry points are Harness Adapters, with one Implement adapter per mode (standard and team). Adapters pass mode, subagent model and thinking level, and the Engine places those choices at the step they govern. Only the pi adapters ship model defaults. The pi runners and the disabled loop prompts are removed.
- The Watchdog Procedure checks its own fresh-context boundary: a session that built the change stops and asks for a new session.
- Standalone Audit and the Audit step inside Implement are separate Procedures that share Audit Craft.
- Rendered words per operation, the implement-to-watchdog journey total, negation density and abstract-term density are measured from goldens against a recorded baseline. Every Execution Skill golden shrinks and the journey total drops. `docs/agent-prose.md` documents the architecture and the metrics.
- In this repository only, `AGENTS.md` carries the writing rules for agent-visible prose, including that ADR and Contract wording is never transcribed into it.

This refines ADR 0001's composition model and leaves ADR 0005's verification decisions unchanged.
