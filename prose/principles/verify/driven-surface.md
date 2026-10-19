# Driven surface

A driven surface is an interface the verifier operates from outside, the way a user or client does: HTTP endpoints, a browser UI, a terminal UI.

## Evidence

The verifier performs the scenario's steps and observes what the surface shows after each: a response, a rendered page, a screen. It proves the behavior end to end through every layer behind the surface. A scripted driver yields repeatable evidence; agent-driven steps yield evidence as good as the observations recorded at each step.

## Catalog

A `journey` row per scenario that has no script, with its file under the journeys directory: preconditions and setup, ordered steps with what to observe after each, a sanity predicate, cleanup and traps. A scripted driver suite is an `executed` row. A route or screen listing that the repository can print is a `generated` row the journeys depend on.

## Traps

- A step that waits a fixed time races the system; wait for the observable state the next step needs.
- Data left by one journey changes the next; each journey sets up and cleans up its own.
- An observation of "it looked right" carries no evidence; record the exact value, text or element seen.
