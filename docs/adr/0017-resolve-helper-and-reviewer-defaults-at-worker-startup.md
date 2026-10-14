# Resolve helper and reviewer defaults at Worker Session startup

The merged harness-launch-settings work resolves helper and reviewer defaults at dispatch and carries per-session choices through a private launch-resolved marker. Resolve those defaults instead at Worker Session startup, remove the marker and persistent launch-snapshot machinery, and retain the choices supplied to a running session. This lets a newly starting or recovering worker use current configuration without changing an already-running worker's launch choices or accidentally preserving resolved defaults as explicit overrides. Deliver this correction in a new launch-startup-correction Slice of cli-interface-consolidation, then make delivery-interface depend on it; the correction remains outside the delivery worker's implementation scope.

## Consequences

- Dispatch resolves the worker model and thinking when it tells the Supervisor which worker to launch. Helper and reviewer defaults resolve when that Worker Session starts, from current configuration and explicit invocation overrides.
- Configuration changes after dispatch but before worker startup affect helper and reviewer defaults. Changes after startup do not alter the choices already supplied to that session; prepare and inspect do not establish another launch.
- A fresh recovery Worker Session resolves current defaults again. Explicit overrides retain precedence; missing settings inherit the harness's normal behavior rather than a compiled fallback.
- Subsequent dispatches refresh configuration and continuation carries only explicit overrides, not resolved defaults. Existing acquisition-time refresh after idle waiting remains intact.
- Remove acceptance and emission of --launch-resolved without introducing another persistent snapshot or hidden marker. The representation of session-local launch choices is delegated to implementation within these behavioral bounds.
- The correction Slice owns launch behavior, affected generated commands, guidance and verification. Delivery retains its partial implementation and integrates the merged correction before completing its canonical command journey and verification. Historical harness Contracts and delivery's frozen Contract are not rewritten; the amendment records the new prerequisite and corrects the earlier ownership assumption.

This refines the launch timing and recovery consequences of [ADR 0015](0015-resolve-agent-launch-defaults-from-harness-scoped-config.md) and replaces the correction-ownership assumption in [ADR 0016](0016-consolidate-command-syntax-without-compatibility-aliases.md). Configuration schema, harness selection, explicit field-by-field precedence, tool scope, Workflow Mechanics and the coordinated binary/adapter upgrade boundary remain unchanged.
