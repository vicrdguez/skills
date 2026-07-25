# {change title} Plan

<!--
The purpose is to keep the implementer from making important architectural decisions at code time Reference ADRs rather than restating them.

Write this file knowing that a different agent with a completely fresh context will implement it. Making
sure that no context resolution is lost that could drive the implementer to drift from the plan is _crucial_. 

When you include code-snippets anywhere on this document, **trim to the decision-rich parts** - a working demo is not needed, just the important bits
-->

## Approach
<!--
The implementation strategy and how all the pieces fit together. 

Add detail and clarity as needed, use code snippets as examples if it encodes the approach precisely and directly than prose.
-->

## Implementation decisions
<!--
A list of the pinned implementation decisions made in the session. These are the decisions the implementer MUST NOT relitigate.

Including (but not limited to):

- Technical clarifications from the developer
- Architectural decisions
- API contracts
- Specific interactions
-->

### Module shapes & seams

<!--
The deep-module / seam decisions made in the sessions. This should include:
- Sketches of the seams at which this change will be tested
- The modules that will be built/modified
- The interfaces of those modules that will be modified.
- If there is another *existing Module* that can act as a reference implementation, reference it.

Modules are scale-agnostic: capture PUBLIC modules and (if needed) its critical INTERNAL modules. 
-->

#### [{NEW/MODIFIED}] {Module} 
<!--
For each module state the interface, its dependencies (with its categories) and the invariants that it must uphold. Internal correctness is the whole point so if there is more information that significantly helps during implementation, pin it.

Use snippets that show the interface functions/methods in scope.

Finally, include the Test strategy for the module.
-->

## Sequence
<!--
Numbered, Ordered high-level steps, if order matters. Be concise but precise.
-->


