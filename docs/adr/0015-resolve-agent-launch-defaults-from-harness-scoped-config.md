# Resolve agent launch defaults from harness-scoped config

Model and thinking defaults currently live in installed Pi adapters, so changing them requires rebuilding or replacing those adapters. Move these defaults into the existing machine-local `skl/config.json`, with separate sections for Pi, OpenCode, Claude and Codex. Installed adapters identify their harness explicitly; the CLI resolves launch defaults without detecting the runtime or interpreting provider-specific model identifiers. This keeps launch preferences outside Workflow Mechanics while allowing edits to affect subsequent Worker Sessions.

## Status

Accepted after final Explore confirmation; implementation is pending.

## Consequences

- Configuration uses `harnesses` with optional `pi`, `opencode`, `claude` and `codex` sections. Each section has optional `implement`, `watchdog`, `helper` and `reviewer` roles, each with optional `model` and `thinking` fields. These respectively select implementation workers, watchdog workers, team helpers and implementation reviewers. Standard and team implementation share the `reviewer` default. Missing sections or settings inherit rather than falling back across harnesses; empty sections are equivalent to omission.
- Explicit CLI choices override configured defaults field by field. Missing choices inherit the harness's normal behavior; compiled model and thinking fallbacks are removed. Existing ledger-only configuration remains valid, and existing ledger validation is preserved.
- Adapters pass an explicit `--harness` selector. Dispatch, resume and continuation preserve it. Direct CLI calls without a selector use explicit launch flags only and otherwise inherit; there is no environment-based harness inference.
- Pi, OpenCode and Claude receive `implement-loop`, `implement-team-loop` and `watchdog-loop` plus corresponding `implement-auto`, `implement-team-auto` and `watchdog-auto` adapters. Loop adapters no longer expose an `auto` positional argument and start without Auto Mode; auto adapters run the same loops with `--auto` enabled. Both families retain optional model and thinking overrides. Direct CLI `--auto` remains supported, Codex entry-point support remains unchanged, and Auto Mode's Workflow Mechanics do not change.
- Every new dispatch reads the current configuration. Continuation preserves explicit overrides, not resolved defaults. Already-running workers retain their launch settings, including helper and reviewer choices supplied to that session.
- Model identifiers and thinking choices remain harness-interpreted values. Operators own choosing values supported by their selected harness; the engine does not acquire a provider catalog.
- This replaces ADR 0008's installed Pi-only model defaults and refines ADR 0010's opaque launch arguments without changing dispatch eligibility, Claim ownership or continuation rules.

Codemode availability remains an Agent Harness responsibility. Pi workers use ordinary extension-enabled subagent sessions so that codemode, when enabled for those sessions, is available under the harness's normal tool scope. No workflow prose mandates or encourages particular codemode usage; the model chooses whether it is useful. Other harnesses' tool policies are unchanged.
