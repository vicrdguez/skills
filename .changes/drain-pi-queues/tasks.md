# Tasks - Drain Pi Queues Through Shared Loop Skills

Blocked by `continue-verified-dispatches`, transitively `wait-for-claimable-work`. Each B task is one red-green cycle at the existing CLI retrieval/install seams; its scenario validates emitted contracts or installed files, not LLM runtime behavior.

## Behavioral

- [ ] B1 Retrieve shared loops separately from one-item skills -> behavior.md B1; intent.md A1. Register/embed shared names, install stubs, and extend existing Markdown/JSON retrieval and discovery coverage.
- [ ] B2 Emit CLI-authorized dispatch and continuation ordering -> behavior.md B2; intent.md A2. Encode predecessor-returned startup/continuation ordering, CLI-only action authority, and per-conversation progress reports in both shared loops.
- [ ] B3 Emit bounded waiting without a queue attempt cap -> behavior.md B3; intent.md A3. Encode 15-minute per-request windows, predecessor wait/poll behavior, and lane-local timeout semantics without a new timer or retry policy.
- [ ] B4 Emit independent cold-context lanes -> behavior.md B4; intent.md A4. Encode lane/project limits, fresh worker contexts, nested Audit distinction, and unchanged bounce/merge authority.
- [ ] B5 Emit safe stopping and explicit recovery instructions -> behavior.md B5; intent.md A5. Cover failed workers with known continuation, unknown dispatch/actions, operational errors, and explicit recovery without automatic reselection or Claim mutation.
- [ ] B6 Emit extension-based workers and two independent Audit axes -> behavior.md B6; intent.md A6. Install thin dedicated roles and update the Pi-specific shared Audit delegation while preserving its existing judgment contract.
- [ ] B7 Install role-specific global defaults without pinning dispatch models -> behavior.md B7; intent.md A7. Verify exact Astra frontmatter and thinking levels, no fallback chain, thin role bodies, and native override precedence unobstructed by dispatch pins.
- [ ] B8 Preserve user configuration while refreshing managed assets -> behavior.md B8; intent.md A8. Extend temporary-home install coverage for idempotence, ownership collisions, unchanged global/project settings, and persona-preserving native override guidance.
- [ ] B9 Emit actionable prerequisite failures without fallback execution -> behavior.md B9; intent.md A9. Encode required Pi/extension/model/thinking/nested delegation capabilities and actionable halts without parent work or silent substitution.
- [ ] B10 Retire old installation targets but preserve legacy copies -> behavior.md B10; intent.md A10. Test both fresh and legacy-seeded homes, repeated preservation, current entrypoint references, and manual-removal guidance.

## Chores

- [ ] C1 Retire PR #19 prompt/runner/helper source, `queue-outcomes.json`, `queue-next.test.mjs`, and obsolete embedding/install/helper-dependent test wiring listed in plan.md; retain shared Workflow code and the extension. Correct directly affected old-role references only. Add no installed-file cleanup or migration. Completes A10 alongside B10.
- [ ] C2 Run `go test ./...`, `git diff --check`, and any existing documented project gate after the red-green cycles. Keep tests in the existing catalog/retrieval/install suite where practical; do not add live harness smoke tests or model-quality tests. Verification for A1-A10, not a claim of LLM runtime compliance.

## Docs

- [ ] D1 Update README installation and Pi operation guidance with explicit shared-skill invocation, independent lanes, predecessor-owned wait/continuation behavior, exact role defaults, native project model-only/model-plus-thinking overrides, prerequisites, effective execution-limit caveats, and the five manual legacy-removal paths. Distinguish unavailable configured models from permissible explicit user overrides; make no silent-fallback, universal-availability, or measured-performance claim. Completes A11 and supports B8-B10.
- [ ] D2 Update `docs/adr/0001-embed-skill-definitions-behind-harness-stubs.md` to replace the obsolete Pi-only queue decision with authoritative shared loop skills and thin Pi adapters using the retained extension. Record frontmatter defaults/native overrides and no installed-file migration. Leave ADR 0002's provider-neutral Backend project outside this slice. Completes A11.
- [ ] D3 Update `docs/capabilities/skill-distribution.md` and `docs/capabilities/work-item-lifecycle.md` using `skills/dev/domain/reference/CAPABILITIES-FORMAT.md`: replace retired Pi-only installer/helper behavior with delivered shared/Pi behavior, preserve later OpenCode/Codex adapters as planned, and retain single-operator/human-merge constraints. Document configurable roles and optional reviewer model diversity without claiming diversity for the shipped same-model profile. Completes A11; final doc task.
