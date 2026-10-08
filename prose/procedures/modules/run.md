{{define "run-metadata"}}Write `{{.ResultDirectory}}/run.json` last, just before you submit or pause, with what your harness shows about this session. Put in only the values you read from the harness and drop the rest, empty objects included. The file is optional.

```json
{
  "harness": {"name": "…", "version": "…"},{{with .Mode}}
  "mode": "{{.}}",{{end}}
  "elapsed_ms": 0,
  "worker": {"provider": "…", "model": "…", "reasoning": "…"},
  "usage": {
    "coverage": "worker_only | partial | complete",
    "input_tokens": 0,
    "output_tokens": 0,
    "cache_read_tokens": 0,
    "cache_write_tokens": 0,
    "input_tokens_include_cache": true,
    "tool_calls": 0
  },
  "children": [
    {"role": "…", "provider": "…", "model": "…", "reasoning": "…", "elapsed_ms": 0, "usage": {"coverage": "…"}}
  ]
}
```

- `harness` and `worker` describe this session as it actually runs: the provider, model and reasoning level your harness reports, rather than the ones you were asked to use.
- `elapsed_ms` is the milliseconds since this session started, counting tools, tests and time spent waiting on subagents.
- `usage` holds your harness's latest counters. `coverage` says whose usage they count: `worker_only` for this session alone, `partial` when they include some of your subagents, `complete` when they include all of them. Set `input_tokens_include_cache` only when your harness says whether input tokens include cache reads. Copy `tool_calls` from the harness rather than counting.
- `children` lists each subagent you have numbers for, with its `role`, such as `implementation` or `standards-review`, and its own `elapsed_ms` and `usage`. Keep their numbers separate from yours.

Check: `run.json` holds one JSON object with only the values your harness showed, or you skipped it for lack of any.{{end}}
