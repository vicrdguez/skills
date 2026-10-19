# Verification skill

Write the repository's verification skill at `.skl/verify.md`. A verifier reads it from a fresh context, so every command runs from the repository root exactly as written.

Start with YAML frontmatter that lists the ranks, strongest first, named for this repository:

```yaml
---
ranks: [tests, build]
---
```

Then these `##` sections, in this order:

## Launch

How to bring the system to an observable state: install, build and start, with the commands, and what refuses to run and how to tell.

## Doctor

How to confirm the environment is ready: each check, its command and its expected result.

## Channels

One list item per channel the repository has, starting with the channel's name in backticks, one of `oracle`, `process`, `introspection`, `driven-surface`, `tests`, `static` and `human-observed`, followed by how this repository exposes it. A channel outside those names gets its own word, followed by what it observes.

## Catalog

One table, with one row per check the repository can run:

| Name | Channel | Kind | Run | Expectations | Rank |
| --- | --- | --- | --- | --- | --- |

- **Name**: one word, unique in the table. A Contract's Verification strategy cites it.
- **Channel**: the channel the check observes through.
- **Kind**: `executed` for a runnable check whose expectations live in the repository; `journey` for steps a verifier performs from a journey file; `generated` for a command that prints an index the other rows must cover; `human` for a review gate a person closes.
- **Run**: the command, or for a journey the path `.skl/journeys/<name>.md`.
- **Expectations**: what a pass shows.
- **Rank**: one of the ranks; empty for a `generated` row.

## Cleanup

How to return the repository and any running system to where Launch started.

## Extend

How to add evidence when a change adds a behavior rule: where a new case or test goes, and how to add a catalog row for a new kind of check.

## Traps

What makes a check here fail, or pass, for the wrong reason.
