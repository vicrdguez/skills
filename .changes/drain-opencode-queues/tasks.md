# Tasks - Drain OpenCode Queues

Blocked by `drain-pi-queues` being Merged, including its transitive CLI waiting/dispatch dependencies.

## Behavioral
One task maps to one Gherkin scenario and its Definition of Done. Runnable distribution/configuration changes use red-green cycles at the approved seams. Instruction-only behavior is encoded and reviewed through delivered instruction retrieval; it does not require fabricated runtime tests or blanket phrase assertions.

- [ ] B1 Install native OpenCode discovery assets -> behavior.md B1; intent.md DoD1. Extend existing temporary-home `skl install` checks, including native config-home resolution and parsed asset validity.
- [ ] B2 Preserve user ownership on reinstall -> behavior.md B2; intent.md DoD2. Check refresh/idempotence and byte-preservation of collisions, unrelated settings, and project overrides; report blocking collisions.
- [ ] B3 Retrieve shared loop instructions without changing one-item entry points -> behavior.md B3; intent.md DoD3. Extend existing Markdown/JSON retrieval equivalence checks and review stub/manifest routing.
- [ ] B4 Dispatch a fresh worker for the fixed Claim -> behavior.md B4; intent.md DoD4. Encode and review separate-supervisor native Task instructions, omitted `task_id`, supplied startup command, and one-item worker exit.
- [ ] B5 Follow authoritative continuation and waiting decisions -> behavior.md B5; intent.md DoD5. Encode and review predecessor continuation/wait argument forwarding, uncapped draining, timeout meaning, error stops, and preserved Claims.
- [ ] B6 Delegate both Audit axes independently -> behavior.md B6; intent.md DoD6. Add and review OpenCode's parallel fresh-axis binding while preserving shared Audit judgment, deterministic facts, and Watchdog separation.
- [ ] B7 Install role-local defaults without changing the supervisor -> behavior.md B7; intent.md DoD7. Parse role and command documents for native types, defaults, scoped Task permissions, and unchanged supervisor context/model.
- [ ] B8 Override project role settings without replacing prompts -> behavior.md B8; intent.md DoD8. Parse the documented JSON and native role documents; review prompt-retention and precedence guidance against the verified native contract, not a fake resolver.
- [ ] B9 Halt on unavailable native prerequisites -> behavior.md B9; intent.md DoD9. Encode and review actionable depth, Task permission, role/model availability, and post-selection recovery guidance without automatic config edits.
- [ ] B10 Document the supported operating path and evidence limits -> behavior.md B10; intent.md DoD10. Review README/capabilities for installation, separate lanes, waiting overrides, prerequisites, recovery, role configuration, and honest verification scope.

## Chores
- [ ] C1 Read the merged predecessor's concrete CLI startup/continuation and shared loop interfaces before adapter wiring; reuse them without adding Workflow Mechanics. Supports B3-B6 and B9.
- [ ] C2 Coordinate final role names across embedded assets, native Task references, scoped permissions, and examples; preserve ownership protocols and existing non-OpenCode distribution. Supports B1-B3 and B6-B8.
- [ ] C3 Run focused existing install/retrieval and parser checks plus the repository's existing Full Gate and `git diff --check`. Report evidence limits explicitly; add no live harness smoke tests, benchmarks, fake delegation runtime, or new test seam. Supports B1-B10.

## Docs
- [ ] D1 Update `README.md` with explicit OpenCode support, global discovery/install ownership, separate native supervisor invocations, default and overridden waiting, model profile, user-applied depth/permissions/provider prerequisites, nested JSON role overrides, and recovery guidance. Supports B8-B10.
- [ ] D2 Update `docs/capabilities/skill-distribution.md` and the relevant OpenCode portion of `docs/capabilities/work-item-lifecycle.md` to reflect this delivered slice without claiming Codex implementation or changing predecessor scope. Keep the existing capability format and distinguish parser/instruction checks from untested live execution. Supports B1-B10.
