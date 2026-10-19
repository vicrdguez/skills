# Bind a repo-owned verification skill

Nothing in the Workflow knows which check in a Consumer Repository proves what, how strong it is, or how to run it from a fresh context. Let the repository own one thin verification skill at `.skl/verify.md`: an ordered evidence ladder, a catalog with one row per check the repository can run, and the facts a fresh verifier needs. skl owns the craft for each observation channel and the Procedures that create and maintain the skill. The engine reads the ladder and the catalog, binds the cited checks into implement and Watchdog, and bounds a verdict's rank by the weakest cited row. This is the first repo-owned file rendered into agent-visible prose, and with the journeys it names, the only one besides project principles (ADR 0020).

## Status

Accepted after the proposal checkpoint of the design-workflow Proposal; implementation is pending.

## Consequences

- The skill has one shape: frontmatter `ranks`, then Launch, Doctor, Channels, Catalog, Cleanup, Extend and Traps. The catalog's columns are name, channel, kind, run, expectations and rank. Kinds are `generated` (an index command; no rank), `executed` (a runnable check with its expectations in the repository), `journey` (steps a verifier performs, in `.skl/journeys/<name>.md`, with a sanity predicate independent of any slice) and `human` (a review gate, cited by Manual Verification only).
- The catalog rule replaces any feature map: a row is generated or executed, never hand-maintained; a row that cannot run is invalid, not stale. The maintain Procedure runs every row and reports clean, changed or blocked.
- Channels are a fixed set with skl-owned craft: executable oracle, process, live runtime introspection, driven surface, tests, static and human-observed. The channel decides which rules are verifiable and at what rank. The create Procedure may note an unnamed channel; promotion to craft is manual.
- The project ladder replaces the built-in one wholesale. Contracts cite catalog rows by name; lint refuses unknown names and rows that cannot prove a rule, and accepts a row the slice declares new. Implement runs the cited rows before Audit; Watchdog runs them from the reviewed source and records the rank.
- The engine never runs a check. Discovery goes through installed stubs; the repository file is a Skill Resource, never a harness-discoverable skill.
- For a greenfield repository the verification strategy is a seams-level node of the design and the first slice creates the skill from it.

## Evidence

Minos `proposal/native-engine` keeps its verification in repo-native form (a gate with a checkout guard, one task per package, a conformance corpus with a shrinking unsupported list); it does not drift because it runs on every gate. The capability documents skl once kept drifted because nothing executed them.
