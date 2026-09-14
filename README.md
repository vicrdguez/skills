# Vic's skills repository

My collection of personal skills. These are heavily influenced by [OpenSpec](https://github.com/Fission-AI/OpenSpec) and [Matt Pocock skills](https://github.com/mattpocock/skills), mixing ideas from both.

I started customizing my OpenSpec workflow a lot, and eventually trying to incorporate Matt's ideas in it. It reached a point where I wanted to modify the skill files created by OpenSpec, so I decided to build the workflow on my own to make it easily installable in any of my projects.

In that sense, this collection is not inventing anything new but remixing mulitple ideas in a single bundle I can take anywhere. In fact, some Skill files are essentially verbatim from Matt's with maybe minor modifications to fit the pipeline

The only thing I'm bringing here is the way these ideas are remixed to work for me. So all props both projects.

## Optional thinking tools

`brainstorm` and `shape` are manually invoked thinking tools outside the development pipeline. `brainstorm` preserves an open-ended conversation; `shape` turns selected thinking into a faithful, implementation-independent design. Their artifacts stay local under `.thinking/`, excluded through Git's repository-local exclude, and are read only when their exact path is supplied.

## Why I'm not using Matt's skills directly

I think his skill repository is full of great ideas and skill implementations that I've benefited multiple times from. However I wanted mi pipeline to use different artifacts with different goals to aid Agent implementations, all while keeping a set of durable docs that persist across changes, even when those artifacts are deleted.

`grill-with-docs` is pretty much that idea, I'm just adding the concept of *Capabilities*, which is similar to what OpenSpec does with delta specs. My overall pipeline is also different so everything adapts to fit.


## The staged workflow

The collection is a pipeline of five stages. Each stage is a skill, entered cold and left behind when it hands off. The only exception is `propose`, that is run directly in the same session as `explore`:

```mermaid
flowchart LR
    explore[explore] --> propose[propose] --> implement[implement] --> watchdog[watchdog]
    watchdog -->|pass| merge([human merge])
    watchdog -->|rework| implement
    implement -->|blocked decision| human[needs-human]
    watchdog -->|blocked decision / bounce cap| human
    human -->|supersede| explore
```

| Stage | Skill | What it hands off |
|---|---|---|
| Explore | `explore` | Shared understanding through frontier rounds; durable docs written inline via `domain` |
| Propose | `propose` | Agent-authored tracer-bullet slices prepared in Git, then deterministically published by `skl propose` |
| Implement | `implement` | One claimed slice, TDD'd at pinned seams, refactored via `audit`, PR labeled `review` |
| Watchdog | `watchdog` | A fresh-context verdict published through `skl watchdog`: Ready for Merge, Rework, or Needs Human. Only non-functional Debt Marker comments may change |
| Merge | human | The acceptance gate |

The rest are reference skills the stages pull in rather than stages of their own: `design` (deep modules and seams), `domain` (glossary, ADRs, capabilities), `tdd` (the red → green loop), `audit` (the two-axis review engine). `writing-for-agents` is a standalone reference for authoring skills and agent-facing docs.

Four rules hold it together:

- **A fresh context per stage.** Handoff happens through the board and the filesystem, never through conversation history. The watchdog is the strict case: it never runs in the context that built the change, so a green suite it did not run itself does not count.
- **The backend projects the queue.** GitHub issues and PRs project Workflow State through `ready`, `review`, `rework`, `needs-human`, and `done`; `wip` is an additive Claim, not a lifecycle state. `done` means Ready for Merge, not Merged. Each slice has one branch and one conventional worktree under `.worktrees/<slug>`.
- **Two document lifetimes.** Implementation Ledgers (`intent.md`, `behavior.md`, and optional `plan.md` / `tasks.md`) live in `.changes/<slug>/` on the slice branch. The pushed publication head is marked `[baseline] <slug>`; after only existing non-manual boxes change to lowercase `[x]`, the still-present ledger is marked `[completion] <slug>`. A later commit removes it before review. Review and Rework compare those exact historical endpoints and ledger absence, not intermediate edits. Durable docs (`CONTEXT.md`, `docs/adr/`, `docs/capabilities/`) outlive the change.
- **Slices are tracer bullets.** Each one cuts a complete path through every layer, is demoable on its own, declares its blocking edges, and is sized to fit a single fresh context window.

### How this differs

**From OpenSpec** (`explore` → `propose` → `apply` → `archive`): the first two stage names are borrowed outright. The divergence is that `apply` splits into `implement` plus an adversarial `watchdog` stage and then the human merge — review becomes a stage with its own trust boundary instead of a step inside implementation. There is also no spec store: durable knowledge is `CONTEXT.md`, ADRs and capability docs on `main`, and in-flight state lives on the issue board rather than in a change folder.

**From Matt's skills**: his repo is a composable collection, you reach for whichever skill fits the moment. This is an ordered pipeline where each stage has entry and exit conditions, which is what makes the cold handoff between stages possible at all. Several skill files here are near-verbatim from his; the pipeline wrapped around them is the part I added.

## Installation

### Prepare a Consumer Repository

From this checkout, install the workflow executable and run Setup anywhere inside the target Git repository:

```sh
go install ./cmd/skl
skl setup
```

Use `skl setup --repo <path>` to target another checkout. If that repository has multiple GitHub remotes and no GitHub `origin`, select one with `--remote <name>`. Authentication is read from `GH_TOKEN`, then `GITHUB_TOKEN`, then `gh auth token`; Setup stores no credentials.

### Publish a Proposal

After Propose has prepared and pushed each slice branch at its complete Artifact Baseline with exact subject prefix `[baseline] <slice-slug>`, remove only safe Merged local state and publish the agent-authored issue bodies:

```sh
skl propose cleanup --repo <path>
skl propose publish --repo <path> --target main \
  --slice add-foundation=/tmp/add-foundation.md \
  --slice add-feature=/tmp/add-feature.md \
  --depends add-feature:add-foundation \
  --parent-title "Build the feature" --parent-body /tmp/proposal.md
```

Omit the parent flags and repeated slice/dependency flags for a single-slice Proposal. `fix_required` identifies a Git preparation problem to repair before retrying; `needs_human` identifies ambiguous backend state that the engine will not guess through. Issue Markdown remains opaque and is not copied into the repository.

### Implement a Work Item

Run `skl implement next` for one claimed Work Item and its bundled Instruction Packet, or `skl implement resume --item <number>` for interrupted work. Inside its conventional worktree, `skl implement resume` resolves the Claim by location. Selection prefers eligible Rework and skips blocked, claimed, and paused items. Markerless existing work may supply full `--artifact-baseline` and `--artifact-completion` SHAs on relevant Implement and Watchdog commands; preserve those exact flags through every command for that invocation and Work Item.

Implement uses Setup's remote inference: GitHub `origin`, otherwise the sole GitHub remote. Select explicitly with `--remote <name>` when ambiguous or overriding `origin`; all Implement operations accept it, and packet retry commands retain it. Use that same remote for ordinary Git fetch/push.

The worker merges the pinned Target Snapshot, writes code and scenario tests in red-green commits, and runs focused checks. At the Audit gate the worker runs the Full Gate and both independent review axes against a provisional endpoint, which may still have unfinished boxes and must not be reported ready for review. After dispositions, the worker ticks every automated box, commits the still-present ledger with `[completion] <slug>`, removes it in a later commit, and pushes. `skl` runs none of those project checks or Git mutations.

Write the Audit-bearing `submission.md` in the packet's private temporary directory, then run `skl implement submit --item <number> --body <absolute-file>` with any supplied artifact endpoint flags. A repairable refusal retains the Claim and prose. Successful publication reports `awaiting_review`, removes the temporary directory, and leaves the issue open until human merge. `skl implement inspect --item <number>` supplies current fixed Git and endpoint-only ledger evidence for Audit.

For a permitted human decision, use `skl implement needs-human --item <number> --reason <reason> --decision <absolute-decision.md>`; also supply `--body` and push when a draft Submission must preserve implementation work. Retrieve both Result Document templates through `skl skill --resource reference/submission.md implement` or `skl skill --resource reference/decision.md implement`.

Existing in-flight work stays on its former CLI unless an operator explicitly hands it to this endpoint contract with full SHAs. The handoff is per invocation: it writes no Adoption record, never overrides a unique marker, and never resolves ambiguous markers by choosing one.

### Wait for claimable work

Both `skl implement next` and `skl watchdog next` check once and return `work_available` with a claimed item and packet, or `no_work`, by default. Add bounded waiting when another lane or a human merge may make work eligible:

```sh
skl implement next --wait --repo <path> --remote upstream
skl watchdog next --wait=2m --poll 5s --repo <path> --remote upstream
skl implement next --wait 2m --poll=5s
```

Bare `--wait` means up to 15 minutes; `--wait=2m` and `--wait 2m` override that idle window. `--poll` defaults to 30 seconds. Durations must be positive Go durations with units; invalid values fail before Backend effects. A valid `--poll` without `--wait` is accepted but does not enable waiting.

Selection runs immediately, then waits between completed empty observations for the smaller of the poll interval and remaining idle window. Each invocation starts a new window, including Backend operation time. It emits only one final JSON outcome: a Claim and packet, an existing refusal, or successful `idle_timeout`. The timeout means only local queue inactivity, not global completion; it creates no Claim, private Result Document directory, or persistent run record. Waiting does not launch an Agent Worker or change eligibility, ordering, Dependencies, or handoffs.

The idle deadline prevents new polls but does not cancel an in-flight Claim: a late successful Claim, refusal, or operational error is returned as-is; a late empty observation becomes `idle_timeout`. Operational errors and refusals stop waiting without added retries. SIGINT/SIGTERM or caller cancellation interrupts waiting with a nonzero error, not `no_work` or `idle_timeout`, unless the in-flight selection successfully returns its Claim and packet. No Claim is automatically released or retried. If selection was interrupted and a Claim may have been acquired, inspect the Work Item and explicitly resume it rather than blindly running `next` again.

### Install skills

```sh
go install ./cmd/skl
skl install
```

`skl install` refreshes its owned Skill Stubs in Pi, Codex, Claude Code, and OpenCode, plus Pi-only queue prompts, runners, and their continuation check, without touching unrelated user files. OpenCode receives independent common stubs at `~/.config/opencode/skills/<name>/SKILL.md`, not links to another harness or the authoring tree. This replaces the former Pi package and Claude plugin distribution. Run `skl skill <name>` for rendered instructions, `skl skill --format json <name>` for the typed packet, or `skl skill --resource <path> <name>` for one named resource. Flags precede the skill name.

For a one-time OpenCode cutover, first install the new binary and run `skl install` as above, keeping any existing discovery workaround until the native stubs are available. Then manually remove only obsolete Pi skill-directory or raw-source entries from OpenCode's `skills.paths`; retain unrelated settings and intentionally configured other skills. Do not delete another harness's skills or replace the override with Claude or Codex paths. The installer does not edit discovery settings. Quit and restart OpenCode, then confirm the workflow skills load from `~/.config/opencode/skills/` as thin CLI stubs without claiming work.

The `skills/` tree is authoring input. Installed `SKILL.md` files are thin discovery stubs; the running `skl` binary supplies the embedded definitions and resources, not the source checkout or files beside a stub. After updating this checkout, run `go install ./cmd/skl` here to rebuild the binary, then `skl install` to refresh its owned stubs and adapters. Editing Markdown alone does not update an already-installed binary.

Resource names are exact and relative to their owning skill, even inside a nested resource or bundled definition:

```sh
skl skill --resource reference/DEEPENING.md design
skl skill --resource SKILL-MECHANICS.md writing-for-agents
```

Retrieve a parent definition with `skl skill <name>` only when it is not already supplied; `SKILL.md` is not a resource name. Raw source-tree registrations bypass this distribution arrangement. OpenCode can consume the installed Claude-compatible stubs rather than registering the authoring tree.

### Review and human completion

In a fresh session, run `skl watchdog next`, or resume the fixed Claim with `skl watchdog resume --item <number>`. The packet carries the reviewed head, historical Artifact Baseline and Completion files, the opaque Audit-bearing PR body, prior findings, and raw human comments. Watchdog runs the Full Gate and independently checks only the exact endpoint paths, modes, bytes, ticks, ancestry, and ledger absence; it neither audits intermediate artifact history nor reruns Audit.

Write the summary and optional anchored findings in the packet's private temporary directory. Submit its concrete command with `--verdict pass`, `--verdict rework`, or `--verdict needs-human`. A pass also takes `--body <absolute-submission.md>` containing the complete final PR body and unchecked Manual Verification checklist. Retrieve the transport template with `skl skill --resource reference/review.md watchdog`. If passing Notes need Debt Marker comments, the worker commits and pushes them, runs the formatter/parser and `git diff --check`, and supplies `--head <final-sha>` without replacing `--reviewed-head`.

`ready_for_merge` projects `done` without closing the source issue. Only a human merges; GitHub then closes the issue through the PR's `Closes #N` footer. `skl status` observes Merged, releases Dependencies, closes Coordination Items whose children are all Merged, and safely reconciles partial projections. Contradictions are reported as Needs Human without overwriting them. An unmerged closed Submission is Superseded, preserving its branch reference for later Explore.

A merge conflict routes a passing review to Synchronization Rework with a fresh Target Snapshot while retaining its completed Review Checkpoint. No review outcome restores or archives the retired ledger. Human comments are supplied verbatim; only explicit relabeling to Rework or Awaiting Review requeues paused work.

## Pi subagent loops

The installed Pi adapters use [pi-subagents](https://github.com/nicobailon/pi-subagents) to drain each queue sequentially. Install that extension separately with `pi install npm:pi-subagents`; Node runs the installed structured-outcome continuation check.

- `/implement-loop [max-items]` launches one fresh `implement-runner` using `skl implement next`. A verified `awaiting_review` handoff starts the next worker.
- `/watchdog-loop [max-items]` launches one fresh `watchdog-runner` using `skl watchdog next`. A verified `ready_for_merge` or `rework` handoff starts the next worker.

Review findings carry stable per-PR IDs (`W1`, `W2`, …) so a round can be compared with the last one. The first watchdog review is complete; later rounds verify the open findings and read only what changed since the previous `Reviewed head`, and a second failing review pauses at `needs-human` rather than bouncing again. A verified `needs-human` is a complete handoff — the loop moves on, and nothing reclaims that item until a person does.

The scheduler and every worker have separate contexts. Run the two schedulers in different Pi sessions so implementation and review history never mix:

```sh
pi --name implement-loop --model openai-codex/gpt-5.6-luna --thinking medium
pi --name watchdog-loop --model openai-codex/gpt-5.6-luna --thinking medium
```

The worker defaults are:

| Agent | Model | Thinking | Fallback |
|---|---|---|---|
| `implement-runner` | `openai-codex/gpt-5.6-sol` | medium | `openai-codex/gpt-5.6-terra:high` |
| `watchdog-runner` | `openai-codex/gpt-5.6-sol` | high | `openai-codex/gpt-5.6-terra:high` |

For a critical watchdog run, override only that loop invocation:

```text
/watchdog-loop 10 model=openai-codex/gpt-5.6-sol:xhigh
```

The loop passes this as the `model` override on every fresh `watchdog-runner` launch; it does not change the saved default. The direct pi-subagents equivalent for one item is:

```text
/run watchdog-runner[model=openai-codex/gpt-5.6-sol:xhigh] "Load and follow the watchdog skill exactly. Process one eligible work item and exit."
```

Use `xhigh` for security, authorization, billing, destructive migrations, irreversible data operations, public API compatibility, or the watchdog's opt-in independent test reimplementation. Keep `high` for normal reviews: extra reasoning can otherwise increase latency and speculative edge-case findings.

Both loops consume unchanged CLI JSON and stop on `no_work`, their item limit, incomplete Claims, ambiguous results, or adapter errors. A normal watchdog rejection that reaches verified `rework` is complete, so the loop continues. Queue draining remains Pi-only; Codex and Claude Code use the same one-item semantic commands.
