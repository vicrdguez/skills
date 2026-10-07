# Keep run metadata observational

Record optional Run Metadata alongside implementation and Watchdog Phase Reports for future evaluation of workflow effectiveness and efficiency. Unlike the report's authoritative outcome and provenance, these observations are not submission requirements or workflow evidence: preserve available values even when they are implausible, and leave their validity and interpretation to future evaluation. Metadata-specific collection, decoding, or value problems must not block a handoff or subsequent workflow operations. Existing report fields retain their meanings, requiredness, and validation.

Accepted after Explore confirmation; implementation is pending. This record does not change existing Contracts or running workflow behavior.

Collect portable observations from the reporting Worker Session without harness-specific integrations or estimated usage. Include available child-agent attribution, but exclude the loop Supervisor. Only reported runs are in scope; do not add attempt tracking, slice-lifecycle timing, or a separate execution ledger. Missing observations are absent, not zero.

Measure elapsed session time and sample available usage immediately before submission, excluding earlier interrupted sessions and post-submission activity. Standard mode may also use children, so usage coverage is independent of mode; do not automatically sum potentially overlapping usage counters. Record available model/provider, reasoning setting, harness identity, execution mode, elapsed duration, token/cache counters and tool-use counts, with participant and usage-coverage information where available. These are reporting conventions, not value-validation requirements.

The CLI supplies the submitting binary's available version or installed build revision; missing identity must not prevent submission. No start-time build tracking, instruction fingerprinting, pricing, complexity scores, or evaluation harness is introduced. Existing ledger history and Contract references remain the source material for future analysis.

Introduce report schema 2 while retaining schema-1 reading and preserving historical reports unchanged, following ADR 0006. Existing binaries must be upgraded before consuming schema-2 reports. New readers tolerate unfamiliar or invalid observational values within the Run Metadata block, while existing authoritative fields retain their meanings, requiredness, and strict validation. Unknown future report schemas still receive the existing explicit refusal; this is not general forward-schema compatibility.

Reuse the existing completed-handoff replay behavior without adding metadata comparison or retry tracking. An otherwise identical retry returns the already committed report; differing Run Metadata does not affect replay recognition. The original metadata remains because the existing operation does not rewrite that report, not because of a new metadata-freezing mechanism.

## Agreed field vocabulary

Use the same optional `run` block in both phase reports. Field spelling, nesting, and meanings are approved design choices, not delegated to implementation. This example illustrates the complete vocabulary; its values are illustrative:

```yaml
run:
  skl:
    version: "v1.2.3"
    revision: "4f3c8a1d5b6e7092c4d8e0f1a2b3c4d5e6f70819"
  harness:
    name: "pi"
    version: "1.0.3"
  mode: "team"
  elapsed_ms: 420000
  worker:
    provider: "example-provider"
    model: "example-model"
    reasoning: "high"
  usage:
    coverage: "worker_only"
    input_tokens: 85000
    output_tokens: 9000
    cache_read_tokens: 60000
    cache_write_tokens: 5000
    input_tokens_include_cache: true
    tool_calls: 42
  children:
    - role: "implementation"
      provider: "example-provider"
      model: "example-helper"
      reasoning: "medium"
      elapsed_ms: 120000
      usage:
        coverage: "worker_only"
        input_tokens: 22000
        output_tokens: 3000
        cache_read_tokens: 10000
        cache_write_tokens: 2000
        input_tokens_include_cache: true
        tool_calls: 12
```

Every field is optional; omit unavailable fields and empty containers. The illustrated types are reporting conventions, not submission-validation requirements: unexpected values or types remain recordable. Unknown fields within `run` are observational too.

- `skl.version` and `skl.revision` identify the submitting binary where available. The revision is its build revision, not a Consumer Repository revision.
- `harness` identifies the reporting worker's harness. `mode` is `standard` or `team` where applicable, not standalone versus loop execution.
- `elapsed_ms` is the reporting worker's elapsed session time in milliseconds through sampling immediately before submission. A child's `elapsed_ms` is its observed execution duration, not an amount to add to the reporting worker's duration.
- `worker` identifies the actual reporting worker's provider, model, and reasoning setting where known, not merely requested settings.
- `usage.coverage` is relative to the associated worker or child: `worker_only` includes only that agent, `partial` includes some but not all descendant usage, and `complete` includes that agent and all its descendants. Absence means coverage is unknown.
- `input_tokens_include_cache` says whether reported input tokens already include cached-input tokens. Omit it when the accounting convention is unknown.
- `tool_calls` is the harness-reported tool-call count for that usage coverage. Do not reconstruct or normalize hidden tool activity.
- `children` contains available participating child-agent observations, not a required complete inventory. `role` is free-form, for example `implementation`, `standards-review`, or `contracts-review`.
- Root usage and child usage are never automatically added together. No timestamps, monetary cost, run IDs, or slice-level totals are introduced.
