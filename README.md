# Vic's skills repository

My collection of personal skills. These are heavily influenced by [OpenSpec](https://github.com/Fission-AI/OpenSpec) and [Matt Pocock skills](https://github.com/mattpocock/skills), mixing ideas from both.

I started customizing my OpenSpec workflow a lot, and eventually trying to incorporate Matt's ideas in it. It reached a point where I wanted to build the workflow on my own to make it easily installable in any of my projects.

In that sense, this collection is not inventing anything new but remixing mulitple ideas in a single bundle I can take anywhere. In fact, some Skill files are essentially verbatim from Matt's with maybe minor modifications to fit the pipeline

The only thing I'm bringing here is the way these ideas are remixed to work for me. So all props both projects.

## Optional thinking tools

`brainstorm` and `shape` are manually invoked thinking tools outside the development pipeline. `brainstorm` preserves an open-ended conversation; `shape` turns selected thinking into a faithful, implementation-independent design. Their artifacts stay local under `.thinking/`, excluded through Git's repository-local exclude, and are read only when their exact path is supplied.

## Why I'm not using Matt's skills directly

I think his skill repository is full of great ideas and skill implementations that I've benefited multiple times from. However I wanted mi pipeline to use different artifacts with different goals to aid Agent implementations, all while keeping a set of durable docs that persist across changes, even when those artifacts are deleted.

`grill-with-docs` is pretty much that idea. My overall pipeline is also different so everything adapts to fit.

## The staged workflow

Each stage is a skill, entered cold and left behind at handoff. `propose` is the exception: it runs in the same session as `explore`.

```mermaid
flowchart LR
    explore[explore] --> propose[propose] --> implement[implement] --> watchdog[watchdog]
    watchdog -->|pass| merge([human merge])
    watchdog -->|first failed review| implement
    implement -->|blocked decision| human[needs-human]
    watchdog -->|blocked decision / later failed review| human
    human -->|renewed proposal| explore
```

| Stage | What it hands off |
|---|---|
| Explore | Shared understanding; durable knowledge in `CONTEXT.md` and ADRs through `domain` |
| Propose | Frozen, privately accepted tracer-bullet Contracts and explicit Dependencies |
| Implement | A claimed Work Item implemented, verified, audited, and recorded as `awaiting_review` or `needs_human` |
| Watchdog | An independent private report and `ready_for_merge`, `rework`, or `needs_human` |
| Human merge | Final integration, Manual Verification, and merge authority |

Supporting skills are `design` (deep modules and seams), `domain` (glossary and ADRs), `testing` (behavioral and regression evidence), and `audit` (independent Standards and Contracts axes). `writing-for-agents` covers skill and agent-facing document authoring.

- **Fresh contexts.** Handoff uses recorded evidence, not conversation history. Watchdog never runs in the context that built the change.
- **Private authority.** The Workflow Ledger owns Work Items, nonexpiring Claims, reports, and Workflow State. Issues and PRs are human-facing attachments, never delivery authority.
- **Frozen Contracts, current reports.** Accepted documents remain unchanged in the ledger. Reports carry completion declarations and evidence. Delivery neither requires nor ticks, deletes, or reconstructs source `.changes` or `.watchdog` records.
- **Tracer bullets.** Each slice cuts a demoable path through its layers. A dependent item becomes eligible only after every blocker is Merged, not merely Ready for Merge.

Compared with OpenSpec, implementation is separated from independent Watchdog Review and human merge. Compared with a collection of composable skills, this is an ordered workflow with explicit entry conditions and recorded handoffs.

## Installation

### Prepare a Consumer Repository

```sh
go install ./cmd/skl
skl setup
```

Use `skl setup --repo <path>` for another checkout. Remote inference prefers a GitHub `origin`, otherwise the sole GitHub remote; use `--remote <name>` when ambiguous or overriding it. Authentication comes from `GH_TOKEN`, then `GITHUB_TOKEN`, then `gh auth token`; Setup stores no credentials.

### Configure the private Workflow Ledger

Configure one existing private local Git clone for all Projects on the machine. `skl` provisions no hosting and invents no default location:

```sh
git clone <ledger-remote> ~/workflow-ledger
mkdir -p "${XDG_CONFIG_HOME:-$HOME/.config}/skl"
printf '{"ledger": "%s/workflow-ledger"}\n' "$HOME" > "${XDG_CONFIG_HOME:-$HOME/.config}/skl/config.json"
```

`ledger` must be an absolute path to a usable local Git clone, separate from the source repository. Missing or broken configuration is a repairable refusal, never permission to fall back to forge-authoritative delivery. The ledger clone's own remote/upstream owns replication destinations.

### Record a Proposal

Propose prepares an intake directory containing `proposal.md`, `proposal.json` (proposal name, optional parent title, slice titles, planned branches, and dependencies), and per-slice frozen Contract files. Deliberately public issue bodies are separate temporary files:

```sh
skl ledger accept --repo <path> --proposal-dir <dir> \
  --issue add-foundation=/tmp/add-foundation.md \
  --issue add-feature=/tmp/add-feature.md \
  --parent-body /tmp/proposal.md
skl ledger show --repo <path> --item add-foundation/add-foundation
```

Parent flags are for multi-slice proposals. `accepted` and `existing` freeze Contracts under `projects/<repository>/` in the ADR 0006 layout. A Project names exactly one source repository; a different repository with the same name is refused. Unchanged acceptance is idempotent; changed obligations require a renewed proposal.

Acceptance commits locally before attempting replication and publication of supplied public bodies. Network failure leaves local acceptance authoritative; a pending ledger push stays recorded, while issue publication failures, uncertain creates, and missing prose are reported for that invocation only. Publish the current issue and parent view at any later time with freshly authored prose, without repeating acceptance:

```sh
skl ledger publish --repo <path> --proposal add-foundation \
  --issue add-foundation=/tmp/add-foundation.md
```

Established issues are updated in place and missing ones created; only established attachments are recorded. Reads and updates retry briefly, but a create whose outcome is unknown is never retried, so a later publication may duplicate it. Without prose, the outcome names the private readback and guidance from `skl skill --resource issue-publication.md --input proposal=add-foundation --input repo=/abs/path --input remote=origin propose` to author it. Known competing upstream history requires explicit reconciliation, never automatic merge, rebase, or force-push. Public bodies are not retained as private workflow history.

Read exact evidence with `skl ledger show --commit <sha> --path <ledger-path>`; a missing reference is refused, not substituted. Commands default to Markdown; `--format json` gives equivalent typed transport.

### Inspect private phase reports

Optional local inspection starts with the Work Item, independent of its lifecycle or Claim and without forge access:

```sh
skl ledger show --repo <source-repository> --item <proposal>/<slice>
skl ledger show --repo <source-repository> --item <proposal>/<slice> --phase watchdog
```

The first command returns the committed current report references and explicitly marks absent phases. Add `--phase implement` or `--phase watchdog` to retrieve that current report's original document; readback remains available even at Ready for Merge or while a later Claim exists. The report frontmatter retains exact consumed-input references. Follow its ledger references to earlier rounds with `skl ledger show --commit <ledger-commit> --path <ledger-path>`; source SHAs in `source` metadata identify source revisions, not ledger documents. Exact references are never replaced with newer content. Inspection is read-only and is not an approval gate.

### Browse the ledger

`skl browse` opens a terminal browser over the configured ledger: Projects, their Proposals, and each Slice's lifecycle, Claim, dependencies, branch, and attachments. It starts at the Project of the current checkout when exactly one Project records its repository, otherwise at the overview; `--project <name>` selects the starting Project explicitly. Press `s` to switch Projects, `a` to include archived Proposals, and `i` or `p` to open a Slice's recorded issue or pull request in your browser (`i` on a Proposal opens its parent issue).

The same facts are available without a terminal UI:

```sh
skl browse projects [--include-archived]
skl browse project --project <name> [--include-archived]
skl browse proposal --project <name> --proposal <proposal>
skl browse slice --project <name> --item <proposal>/<slice>
```

Every view reads one committed ledger revision and never fetches, contacts a forge, or changes Workflow State, so it can lag a merge that `skl status` has not yet observed. A Claim is shown as a reservation, not as a running worker. Unreadable records are diagnosed where they occur and mark the affected summaries incomplete; the rest stays browsable.

## Implement a Work Item

```sh
skl implement next --repo <path> --capability pi-subagents
# Continue exactly the reservation returned above:
skl implement resume --repo <path> --item <proposal>/<slice> --claim <acquisition-commit>
```

`next` (alias `start`) selects one eligible item within the source repository's Project. Rework precedes new implementation; within each lane, older proposal acceptance precedes newer work, with item identity breaking ties. Claimed items and unsatisfied Dependencies are skipped. Claim acquisition is a local ledger commit protected by a short mutation lock; known competing upstream history refuses new work, while unavailable replication alone does not prevent local delivery. Claims never expire automatically. `resume` and `release` require the exact acquisition commit and cannot take over or clear a later reservation.

The complete Execution Skill binds the item, branch, conventional `.worktrees/<branch>` path, remote, private Result Document directory, exact Contract/report references, and commands. Selection prepares no source workspace. Run its `prepare` command to create or safely reuse the planned branch/worktree, then the returned `inspect` command. Preparation preserves commits, dirty files, and the index; it never resets, rebases, stashes, or forces progress aside. Fetch failures are reported honestly, and required source revisions must remain available. Inspection is read-only and returns a narrow continuation, not another full skill.

The worker implements the full accepted Contract, chooses suitable existing verification seams and construction order, and runs focused checks. Tests may be grouped, reused, strengthened, consolidated, or removed only while required behavior and failure-mode protection remain. Every explicitly frozen obligation still binds.

Immediately before the submission's single Audit, fetch the selected remote's `main`, record its full observed SHA, and merge that exact revision normally. Preparation-time fetching does not satisfy this late step. If fetching is unavailable, an available last observed target may be used with honest local-only evidence; a failed fetch is never proof of freshness. Resolve conflicts before Audit. Consequential unresolved choices require human direction, not guessed Contract meaning.

Audit uses that fixed integrated comparison, runs the Full Gate, and dispatches independent Standards and Contracts reviews. Apply every `HARD`; fix, decline with a reason, or carry as debt every `JUDGEMENT`, recording all `F<n>` dispositions under `## Audit ledger`. Functional edits afterward require affected checks and a Full Gate over the final functional state, not another Audit. Later target movement alone does not restart the integration or review.

Retrieve the bound deferred report resource when verification and dispositions are settled:

```sh
skl skill --resource ledger-submission.md --input result_directory=/tmp/skl-result --input procedure=initial implement
```

Write the private Markdown body at the returned location. It contains the full current completion-and-evidence table for `B<n>`, `A<n>`, and warranted `T<n>` items, explicit `complete`/`incomplete` declarations, grouped evidence, separate human-owned `M<n>` checks, and the Audit ledger. Omitted items are not complete. The engine adds schema-1 YAML metadata and exact input references; it never infers a verdict from prose. The format is documented in `docs/report-schema.md`.

Commit source changes and keep the planned worktree clean, then use the bound command:

```sh
skl implement submit --repo <path> --item <proposal>/<slice> --claim <acquisition-commit> \
  --head <full-final-source-sha> --target <full-integrated-target-sha> \
  --body /tmp/skl-result/implement-report.md --public-body /tmp/skl-result/public.md
```

`--public-body` is optional and always separately authored. The private report is never a public fallback. Git identity/ancestry checks precede the handoff; the engine does not run the project's Full Gate or judge completion prose. Report, Workflow State, and Claim release commit atomically. Success is `awaiting_review` even if normal source push, ledger replication, or public presentation remains pending. Preserve Result Documents and the exact command for retry; a recognized completed result does not record another transition or overwrite later work.

Use `skl implement needs-human` with the same item, Claim, and private body for a permitted blocker decision. Include the question, evidence, options, recommendation, and incomplete obligations. If source progress exists, also supply its clean `--head` and `--target`; source-less pause is allowed only before the planned branch/worktree exists. This records `needs_human` and releases the Claim atomically. `fix_required` retains the Claim and source progress for repair.

All delivery commands accept the selected `--remote`. `--capability claude-agents`, `pi-subagents`, or `sequential` binds the available helper recipe; omission leaves a bounded runtime capability check. Unsupported format/capability values fail before Claim or result-directory creation. `--result-directory` retains a supplied absolute Result Document location through prepare, inspect, and resume.

## Review and human completion

In a fresh session:

```sh
skl watchdog next --repo <path>
skl watchdog resume --repo <path> --item <proposal>/<slice> --claim <acquisition-commit>
```

Watchdog selects unclaimed Awaiting Review items from the private ledger. Its Execution Skill supplies the frozen Contract, consumed implementation report, prior review and recorded human direction when applicable, fixed implementation head and target, completed-review count, and bound preparation/inspection commands. Neither public discussions nor source markers supply review identity.

Run the Full Gate independently and verify the implementation's complete current declarations and every Audit disposition against the frozen Contract. Do not rerun Audit. An available ancestral previously reviewed revision permits a bounded incremental comparison; otherwise review the full comparison without resetting counts or finding identities. Repeat review verifies active findings, regressions, integration effects, false claims, and critical discoveries, not settled noncritical preferences.

Retrieve the bound deferred report instructions:

```sh
skl skill --resource ledger-review.md --input result_directory=/tmp/skl-result --input round=2 --input reviewed_head=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa watchdog
```

Record stable Work-Item-local `W<n>` identities, evidence, required outcomes, and `BLOCK`, `HUMAN`, or `NOTE` dispositions. Distinguish resolved historical findings from still-active ones. Submit the private body with the bound `skl watchdog submit` command and `--outcome pass`, `rework`, or `needs-human`:

- `pass` is legal at any round when no active `BLOCK` or `HUMAN` remains; it records Ready for Merge.
- First completed-review `rework` returns to implementation; second or later `rework` records Needs Human.
- Explicit `needs-human` also counts as a completed review.

Interruption or same-result retry records no extra round. Worktree recreation, an unchanged source SHA, and recorded human direction do not reset the count. Continued paused work needs separately recorded human authorization within the frozen Contract; public comments alone never authorize it.

On pass, Watchdog may add only permitted non-functional Debt Marker comments. Run the touched-file formatter/parser and `git diff --check`, inspect the final diff, commit, and append `--head <final-source-sha>` to the handoff. The engine retains the fixed reviewed revision separately; it validates Git identity, not comment prose. Publication then attempts a normal push and verifies the remote head before public readiness. Other outcomes may not advance beyond the fixed reviewed head.

Public PR bodies are deliberate human-facing summaries; detailed worker exchanges and findings stay private, with no automatic inline review comments. Public material must acknowledge that remaining human verification obligations remain privately accessible through `skl`. Approval never means Merged or promises conflict-free integration. Only a human merges.

A pull request is a latest view of the current local result, not a queue of updates to deliver. Each handoff commits locally first and then attempts one bounded presentation; a failed or interrupted attempt leaves no reservation, pending record, or source receipt, and never blocks later work. Present the current result at any time, whether or not earlier attempts failed:

```sh
skl ledger present --repo <path> --item <proposal>/<slice>
skl ledger present --repo <path> --item <proposal>/<slice> --public-body <fresh-public-prose.md>
```

Without `--public-body` it returns the current result, its exact private evidence references for `skl ledger show`, the phase's `pull-presentation.md` authoring guidance, and the bound continuation; author fresh prose rather than restoring an earlier body. With prose it pushes the recorded source revision normally (never forcing), presents it as draft, and marks an approved result ready only while the pull request shows its reviewed final revision. It reruns no phase and changes no lifecycle, report, Claim, or review count; only a newly established pull request association is recorded. Reads and repeatable updates retry immediately within a small bound, further updates stop once a later local result supersedes the selected one, and a creation whose response was lost is reported as `uncertain` rather than retried. Temporarily stale public content is possible and is corrected by presenting the then-current result.

Ledger-backed terminal observation/archival and final-review packaging are separate work, not implied by these delivery commands. Existing `skl status` and legacy proposal operations do not replace those ledger operations. In-flight forge-authoritative work must finish with its former binary or undergo an explicit administrative cutover with normal workers stopped; current delivery commands never silently adopt it.

## Resolve a Human Decision

A Needs Human pause waits for an explicit human answer, never an inferred one. Read the ledger-wide inbox of current requests:

```sh
skl decision inbox
skl decision inbox --project beacon
```

The inbox is resolved from the configured Workflow Ledger, so it works without a source checkout and from inside any Project's repository; a `--project` filter is always explicit, never taken from the working directory. A readable ledger with no current requests is an empty inbox; an unresolvable or unreadable ledger is reported as unavailable, never as an empty inbox. Reading claims no work and changes no Workflow State. Plain `skl skill decision` carries no facts and directs you to `skl decision inbox`.

The returned Execution Skill presents every current request with its Work Item identity, exact blocking Phase Report reference, accepted Contract documents, source context, and the worker's question, evidence, options, consequences, and recommendation. It may group related questions, but each request's identity, references, and differences stay separate; missing detail is clarified, not invented. Only an explicitly scoped human answer authorizes a change; questions, discussion, agent recommendations, and public comments, labels, or PR bodies are not authorization. A clear answer naming the affected request needs no second confirmation, while ambiguous, stale, or coupled direction is clarified before any write.

A request's bound command binds every known argument and leaves only the continuation route and the answer file:

```sh
skl decision apply --project <project> --item <proposal>/<slice> \
  --request-commit <full-sha> --request-path <ledger-path> \
  --route <implement|watchdog|supersede> --answer <human-answer-file>
```

The route is part of the answer: `implement` requeues for implementation, `watchdog` returns to review at the same code revision, and `supersede` abandons unmerged work. The engine records the answer and route together and atomically, reports every selected item as `applied`, `already_applied`, `refused`, or `unresolved`, and never claims a mixed group succeeded. Several named requests may instead be submitted later from one explicitly scoped `skl decision apply --input <json-file>`.

Explicitly retire the old parent of abandoned work only when at least one slice is Superseded and no active work or Claim remains. All-Merged proposals belong to completion observation, not this operation:

```sh
skl decision retire --project <project> --proposal <proposal>
```

Retirement preserves frozen Contracts, reports, exact references, and Merged slices, reports partial delivery rather than all-delivered completion, keeps dependents blocked until their blockers are Merged, and never moves archives, remaps dependencies, observes forge completion, or deletes source. Changed obligations require renewed proposal and re-slicing through `explore` and `propose`, not a recorded decision.

The next selected Implement or Watchdog worker receives the exact recorded answer and its full ledger reference through `skl`, within the frozen Contract. It records the consumed reference in its schema-1 handoff, keeps independent judgment, and never resets the Review Count or the two-review automatic-rework limit. A recorded decision is never reconstructed from ledger files or GitHub comments as directive authority, and a recorded answer never manufactures a pass, waives an obligation, or authorizes new work.

## Wait for claimable work

Both lanes check once by default. Bounded waiting is optional:

```sh
skl implement next --wait --repo <path> --remote upstream
skl watchdog next --wait=2m --poll 5s --repo <path>
skl implement next --wait 2m --poll=5s
```

Bare `--wait` allows 15 minutes; `--poll` defaults to 30 seconds. Durations must be positive Go durations. A valid `--poll` alone does not enable waiting. Selection runs immediately, then waits between empty observations for the smaller of the poll interval and remaining idle window. There is one final outcome: work, a refusal, or `idle_timeout`. Idle timeout means queue-local inactivity, not global completion; it creates no Claim or persistent run record and launches no worker.

The idle deadline prevents new polls, not completion of an in-flight Claim. Errors and refusals stop waiting. SIGINT/SIGTERM or caller cancellation interrupts waiting with a nonzero diagnostic, unless in-flight selection already returns its Claim. No Claim is automatically released or retried; inspect and explicitly resume an uncertain reservation instead of blindly selecting again.

## Drain a queue with a Supervisor

A Supervisor drains one phase's queue through Dispatches (ADR 0010):

```sh
skl implement next --dispatch --wait --repo <path> --worker-model <model> --worker-thinking <level>
```

A Dispatch claims one Slice like `next`, but answers with two bound commands instead of the Execution Skill: a worker command (`skl <phase> resume … --dispatched`) that a fresh subagent runs to receive the Execution Skill `next` would have returned, and a continue command that repeats the Dispatch with `--after <claim>`. Worker model and thinking values are opaque and optional; every other option carries through, and the waiting rules above apply unchanged.

The continue command first reads how the named Claim ended in the ledger. It continues only after that Claim's own phase handoff (Implement: submitted or needs-human; Watchdog: a recorded pass, rework or needs-human review), whatever the Slice's current state. A Claim still held, or released without a handoff, stops the Supervisor with the Claim left as it is; an invalid reference is refused before anything is selected or awaited.

## Install skills

```sh
go install ./cmd/skl
skl install
```

`skl install` refreshes owned Skill Stubs in Pi, Codex, Claude Code, and OpenCode, plus Pi-only prompts, runners, and the retained queue helper, without touching unrelated user files. It removes only marker-owned legacy `tdd/SKILL.md` stubs now that `testing` is canonical. OpenCode receives independent stubs at `~/.config/opencode/skills/<name>/SKILL.md`, not links to another harness or the authoring tree.

Direct Implement and Watchdog stubs run their respective `next` commands. Plain `skl skill implement` and `skl skill watchdog` refuse read-only retrieval because no Work Item would be selected or claimed. Named resources remain retrievable. Independent skills use `skl skill <name>` or `skl skill --format json <name>`; flags precede the skill name.

For a one-time OpenCode cutover, install the new binary and stubs before removing obsolete Pi skill-directory or raw-source entries from OpenCode's `skills.paths`. Preserve unrelated settings and other skills. The installer does not edit discovery settings. Restart OpenCode and confirm the native stubs load without claiming work.

The `prose/` tree is authoring input. Installed stubs retrieve embedded definitions from the running binary, not the checkout or neighboring files. Rebuild the binary after Markdown edits, then refresh owned adapters. Raw source-tree registration bypasses this distribution arrangement.

Resource names are exact and owner-relative, including inside nested resources:

```sh
skl skill --resource DEEPENING.md design
skl skill --resource SKILL-MECHANICS.md writing-for-agents
skl skill --resource ledger-submission.md --describe-inputs implement
```

Parameterized resources use repeated `--input name=value` flags, split at the first `=`. `--describe-inputs` reports accepted names, types, choices, and required status without rendering a procedure. An invocation binds every already-known input and defers resource bodies until needed. Retrieve a parent skill only when it was not already supplied.

## Legacy proposal publication

The source-artifact proposal commands remain available for non-adopted repositories, not as an alternative delivery authority. Previously prepared/pushed baseline branches can be published with:

```sh
skl propose cleanup --repo <path>
skl propose publish --repo <path> --target main \
  --slice add-foundation=/tmp/add-foundation.md \
  --slice add-feature=/tmp/add-feature.md \
  --depends add-feature:add-foundation \
  --parent-title "Build the feature" --parent-body /tmp/proposal.md
```

Omit parent and repeated slice/dependency flags for a single slice. For ledger-adopted projects, `skl propose cleanup` instead archives whole terminal, unclaimed Proposals and removes only safe merged local source work from recorded ledger facts; current Propose runs it before `skl ledger accept`. New delivery requires ledger acceptance rather than falling back to source artifacts.

## Pi entrypoints

Installed Pi adapters use [pi-subagents](https://github.com/nicobailon/pi-subagents). Install it separately with `pi install npm:pi-subagents` to launch one-item runners.

Both marker-owned queue prompts remain disabled. `skl install` replaces older owned copies while preserving user-owned prompts. `/implement-loop` directs the worker to `skl implement next` or explicit resume; `/watchdog-loop` directs independent review to `skl watchdog next`, explicit resume, or a fresh one-item runner. Runners report verified outcomes or unresolved failures in normal Markdown.

| Agent | Model | Thinking | Fallback |
|---|---|---|---|
| `implement-runner` | `openai-codex/gpt-5.6-sol` | medium | `openai-codex/gpt-5.6-terra:high` |
| `watchdog-runner` | `openai-codex/gpt-5.6-sol` | high | `openai-codex/gpt-5.6-terra:high` |

A one-item invocation may choose its model explicitly:

```text
/run watchdog-runner[model=openai-codex/gpt-5.6-sol:xhigh] "Follow the Watchdog Execution Skill for one Work Item and report the verified result."
```
