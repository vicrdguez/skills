# Agent prose

This document is for maintainers. It describes how the prose an Agent Worker reads is composed, and how that prose is measured. ADR 0008 records the decision; the terms are defined in `CONTEXT.md`.

## The model

An Execution Skill is one **Procedure** plus the **Craft** it needs.

- The **Procedure** is the invocation's script: its ordered steps, completion checks and commands. The Workflow Engine binds its facts (the Claim, paths, commands) and selects its conditional steps.
- **Craft** is judgment guidance, such as testing, review or design vocabulary. It is independent of any harness and any invocation, and it is written once.
- **Workflow policy** is why the workflow is shaped the way it is. It is never agent-visible. It lives in ADRs, `CONTEXT.md`, the README and `docs/`.

Every sentence of agent-visible prose sorts into one of these kinds. A sentence that sorts into policy is moved out of agent-visible prose.

| Sentence | Kind | Why |
| --- | --- | --- |
| "Prepare or safely reuse the exact planned worktree with `skl implement prepare …`." | Procedure | A step this invocation performs, with its bound command. |
| "Test through an interface that exposes the promised consequence without reaching through it into incidental implementation details." | Craft | Judgment that holds for any harness and any invocation. |
| "Public PR bodies are deliberate human-facing summaries; detailed worker exchanges and findings stay private." | Policy | It explains why the engine keeps reports private. The worker acts on the engine's commands, not on the reason. |
| "A `fix_required` result retains this Claim: repair the reported precondition and retry the same command." | Procedure | What the worker does next after one outcome. |

## Kinds of authored prose

| Kind | What it is | Where it lives today |
| --- | --- | --- |
| Procedures | One script per workflow operation or user-invoked activity. | `skills/dev/*/SKILL.md` and `skills/dev/*/modules/` |
| Craft | Judgment guidance composed into the Procedures that need it. | `skills/dev/testing`, `design`, `domain`, and the review guidance in `audit` |
| Outcomes | Outcome Instructions in Workflow Engine responses: what happened and the worker's next step. | Go rendering in `cmd/skl/`, refusal reasons in the engine packages, and the Decision Inbox templates |
| Documents | Templates and instructions for the documents a worker writes: Contract files, phase reports, public prose. | `skills/dev/propose/reference/`, `skills/dev/implement/reference/`, `skills/dev/watchdog/reference/` |
| Adapters | Harness Adapters: the installed stubs and harness-specific entry points. | `stubs/common.md`, `agents/`, `prompts/` |

The reorganization slice sets the final paths. Until then, the table records where each kind lives now.

## Composition rules

- **Craft carries no invocation facts.** A Craft file names no command, Claim, path, phase or Work Item. When Craft needs a fact, the Procedure that includes it supplies the fact.
- **A Procedure inlines what every run needs and points to what only some runs reach.** A step every invocation performs is written in the Procedure. A step only some runs reach, such as a blocker path or report instructions, is a Skill Resource that the Procedure names at that step.

## Metrics

`tools/prosemetrics` measures each golden in `testdata/prose/`. Counts exclude the supplied fixture documents in `testdata/prose/fixtures/`: the Contract, the reports and the decision are the invocation's data, not authored prose.

| Metric | What it counts | Why it matters |
| --- | --- | --- |
| Rendered words per operation | Words in each golden. | Every word is context load for the worker, and growth here is how prose sprawl shows up. |
| Journey total | Words a worker reads during one initial Implement and one first Watchdog review: the renderings and the Skill Resources those Procedures always retrieve. | It is the cost of one Work Item's delivery. A cut that moves words into a resource the worker always retrieves does not lower it. |
| Negations per 1,000 words | *no*, *not*, *never*, *without*, *cannot*, other negation words and every *n't* contraction. | Naming a forbidden behavior makes that behavior more available to the model. A Procedure that states what to do needs few negations. |
| Abstract-term density | Terms per 1,000 words that name a policy property instead of an action or object, such as *authority*, *obligation*, *invariant*, *provenance*. | Prose written for humans in a decision record is dense in these terms. A high density shows policy that leaked into agent-visible prose. |

The journey counts the Skill Resources every such run retrieves: Audit's smells and acceptance criteria and the Implement report instructions, then the acceptance criteria again and the Watchdog report instructions. Resources a Procedure only points to, such as the testing references, are left out. The journey members and both word lists live in `tools/prosemetrics/main.go`.

## Goldens

`TestAgentProseGoldens` in `cmd/skl/prose_golden_test.go` drives the `skl` CLI through one scripted journey against local Git fixtures. It records every ledger-path rendering a worker receives:

- Implement and Watchdog renderings, with their prepare and inspect continuations;
- standalone skills, the Decision Inbox and its results;
- every Skill Resource a rendering tells the worker to retrieve;
- every CLI outcome on the ledger path;
- the installed stubs and runner files;
- the `AGENTS.md` block that setup writes.

The CLI renders a Watchdog review before inspection resolves its scope, so its incremental and full sections never reach a worker through `skl`. `watchdog-repeat-incremental` and `watchdog-repeat-full` render those sections from the same Claim's facts with the scope inspection reported; every other golden is CLI output.

The test fails if a golden has no rendering, or if a rendering names a Skill Resource without a golden. Run-specific values are replaced with stable placeholders: temporary paths, commit identities (numbered by first appearance), the forge URL and timestamps.

After an intended rendering change, regenerate the goldens and review the diff:

```sh
go test ./cmd/skl -run TestAgentProseGoldens -update-prose
```

## Reporting

`mise run prose-metrics` prints each golden's current values beside the committed baseline, `testdata/prose/baseline.tsv`. The baseline was recorded before the prose sweep.

Every slice that changes agent-visible prose reports the table before and after the change, for each golden it affects and for the journey row. Run the task on the target branch for *before*, and on the slice branch for *after*.

`go run ./tools/prosemetrics -record` rewrites the baseline from the current goldens. Use it only when a change deliberately sets a new reference point, and say so in the slice report.
