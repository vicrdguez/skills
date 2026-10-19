# Route work by task shape through playbooks

Every change enters the Workflow through the same gate, an interview and a four-document Contract, whatever its size. Make `skl work "<prompt>"` the entry point: the engine binds the prompt, the agent classifies the task shape and confirms it with the human, and the engine composes the shape's playbook with the principles and templates a composition table names. Shapes are bug, perf, investigation, change, design and experiment. Bug, perf and change end in an accepted Proposal through the existing acceptance; investigation ends in an escalation or a written finding, never a Slice of its own; design runs through checkpoints and experiment is the standalone form of its experiment phase (ADR 0022). Procedures are authored as playbooks, one per shape, phase or user-invoked activity.

## Status

Accepted after the proposal checkpoint of the design-workflow Proposal; implementation is pending.

## Consequences

- The agent classifies and confirms; the human may name the shape. The engine records the shape path on the Proposal with the prompt and composes from it; it never classifies.
- Templates are shape-sized: a bug renders intent as a paragraph with the reproduction scenario as its one rule, perf with the measurement; the Contract grammar and lint of ADR 0018 apply unchanged.
- A running session may escalate to another shape, carrying what it learned; the engine composes the next playbook. An amendment to an accepted Proposal follows ADR 0014.
- Every shape ends in the one acceptance path; `skl work` renders and records nothing of its own before acceptance.
- The change playbook carries the amendment path of ADR 0014 and the cleanup call of ADR 0006 that propose performed.
- Until the design playbook exists, the design and experiment shapes are refused with an explicit outcome, never a silent fallback.
