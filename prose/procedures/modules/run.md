{{define "run-metadata"}}Write `{{.ResultDirectory}}/run.json` last, just before you submit or pause, with what your harness shows about this session. Put in only the values you read from the harness and drop the rest, empty objects included. Replace any `run.json` already there, since it describes an earlier session; delete it when you have no values.

```json
{
  "harness": {"name": "…", "version": "…"},{{with .Mode}}
  "mode": "{{.}}",{{end}}
  "elapsed_ms": <ms>,
  "worker": {"provider": "…", "model": "…", "reasoning": "…"},
  "usage": {
    "coverage": "worker_only | partial | complete",
    "input_tokens": <count>,
    "output_tokens": <count>,
    "cache_read_tokens": <count>,
    "cache_write_tokens": <count>,
    "input_tokens_include_cache": <true|false>,
    "tool_calls": <count>
  },
  "children": [
    {"role": "…", "provider": "…", "model": "…", "reasoning": "…", "elapsed_ms": <ms>, "usage": {"coverage": "…"}}
  ]
}
```

- `harness` and `worker` describe this session as it actually runs: the provider, model and reasoning level your harness reports, rather than the ones you were asked to use.
- `elapsed_ms` is the milliseconds since this session started, counting tools, tests and time spent waiting on subagents.
- `usage` holds your harness's latest counters. `coverage` says whose usage they count: `worker_only` for this session alone, `partial` when they include some of your subagents, `complete` when they include all of them. Set `input_tokens_include_cache` only when your harness says whether input tokens include cache reads. Copy `tool_calls` from the harness rather than counting.
- `children` lists each subagent you have numbers for, with its `role`, such as `implementation` or `standards-review`, and its own `elapsed_ms` and `usage`. Keep their numbers separate from yours.

Check: `run.json` holds one JSON object with only the values your harness showed for this session, or it is absent.{{end}}
