# Drive Supervisors through Dispatch outcomes

A Supervisor drains one phase's queue by following Dispatch outcomes from the Workflow Engine, not by interpreting a loop Procedure or harness-native worker roles. Each outcome is a complete Outcome Instruction. It either says to start a fresh Worker Session with a bound command, and then run a bound continuation command, or it says to stop. Continuation names the dispatched Claim, and passes only when that Claim ended in a phase handoff recorded in the Workflow Ledger. Everything else stops the Supervisor with the Claim left as it is. This keeps loop decisions in `skl` and reduces each harness's loop adapter to one line, so adding a harness means adding an adapter rather than porting loop behavior.

## Status

Accepted after Explore confirmation; implementation is pending. It replaces the per-harness worker roles, installed role defaults and repository role overrides that forge-era parent #24 planned, and refines ADR 0001's allowance that queue draining may remain Pi-only.

## Considered Options

- **Harness-native worker roles with installed model defaults** (#24): every harness then carries its own loop configuration. ADR 0008 already moved model choice into opaque adapter arguments.
- **Marking dispatched Claims in the ledger**, so dispatch refuses while this phase still holds one: the Supervisor carries nothing between rounds, but it adds a ledger field and depends on one Supervisor per phase per Project. Naming the Claim needs no new state.
- **A loop Procedure the Supervisor interprets**: it duplicates what outcomes already state, and puts continuation judgment back into agent prose.

## Consequences

- The dispatched Claim's ending decides continuation, not the Slice's current Workflow State. This lets the other phase advance the Slice freely, and an earlier round cannot vouch for a later one.
- A Claim that is still held, or was released without a handoff, stops the Supervisor. Nothing is replayed automatically.
- Starting a Worker Session uses plain harness-agnostic wording: a subagent, a model, a thinking level. It uses no extension protocol.
