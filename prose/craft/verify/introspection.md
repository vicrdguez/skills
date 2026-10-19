# Live runtime introspection

Introspection attaches to a running system and reads its internal state: a remote shell into the runtime, process and supervisor trees, in-memory tables, telemetry, a sandboxed database connection.

## Evidence

A verifier drives the system, then reads the state the rule is about directly instead of inferring it from output. It is the strongest evidence for rules about state, concurrency and recovery, and it holds only for the build that is running: the journey starts from a known build and a known state.

## Catalog

A `journey` row per scenario: the journey file lists the launch and attach commands, each step with the state to read after it, and a sanity predicate that proves the attach works on any slice. Expectations come from the rule the Contract cites. A scripted introspection with asserted state can be an `executed` row.

## Traps

- Reading state can change it: a call that starts a process, warms a cache or takes a lock alters what the next step sees.
- A runtime left from an earlier run carries its state; start from the journey's setup every time.
- Runtimes differ in what they expose; a weaker runtime gets a lower rank, named in the ladder, rather than an inflated one.
