# skl command reference

This is the operator's view of the CLI: what each command does, when it runs, and what it returns. Most commands are run by an agent from an installed stub or an Execution Skill, so their flags are usually already bound when a human sees them. Every command defaults to Markdown output; `--format json` gives the same facts as typed transport. Refusals name the invariant that stopped the command and how to repair it.

## Configuration

`skl` reads one file, `$XDG_CONFIG_HOME/skl/config.json` (falling back to `~/.config/skl/config.json`), with a required ledger path and optional harness-scoped launch choices:

```json
{
  "ledger": "/absolute/path/to/ledger-clone",
  "harnesses": {
    "pi": {
      "implement": {"model": "openai/gpt-6.1-sol", "thinking": "high"},
      "watchdog": {"model": "openai/gpt-6-astra", "thinking": "high"},
      "helper": {"model": "openai/gpt-6-luna", "thinking": "xhigh"},
      "reviewer": {"model": "openai/gpt-6-astra", "thinking": "low"}
    },
    "claude": {}
  }
}
```

The path must be an existing local Git clone with at least one commit, separate from any source repository. The clone's own remote and upstream decide where the ledger replicates; `skl` provisions no hosting. A missing or broken configuration is a repairable refusal, never a fallback to forge-authoritative delivery.

The supported harness keys are `pi`, `opencode`, `claude` and `codex`. Each supports optional `implement`, `watchdog`, `helper` and `reviewer` roles, each with optional string `model` and `thinking` fields. Empty sections and values are equivalent to omission; ledger-only files remain valid. Unknown fields and non-string launch values are refused. Values are opaque choices interpreted by the harness, not a provider catalog maintained by `skl`.

Delivery `next`, `resume`, `prepare` and `inspect` take `--harness`. Installed adapters supply it, including Codex's direct stubs. Resolve each field independently: a nonempty explicit launch flag, then the selected harness's role setting, then harness inheritance. Without `--harness`, only explicit launch flags are used; environment variables never select a harness, and another harness's defaults never apply. `implement` and `watchdog` choose dispatched workers; `helper` applies only to team implementation; `reviewer` supplies the same implementation-review defaults in standard and team modes.

Every new dispatch reads the current file. Its continuation carries the selector and explicit overrides, not resolved defaults. The worker command freezes that session's helper/reviewer values (including inherited empty choices), and its prepare/inspect/resume commands keep them. Editing the file therefore affects later dispatches, not existing sessions. Bound commands include an internal `--launch-resolved` marker for these session choices.

## Setup and install

`skl setup [--repo <path>]` prepares local guidance in a Git repository: it writes the managed Workflow and Simplicity block into `AGENTS.md`, offers to link `CLAUDE.md` to it, and adds `.worktrees/` to `.gitignore`. It needs no GitHub remote, authentication or ledger configuration. Setup does not configure a ledger or adopt a Project; those are separate operations.

`skl install` refreshes the owned skill stubs in `~/.pi/agent/skills`, `~/.codex/skills`, `~/.claude/skills` and `~/.config/opencode/skills`, and installs `implement`, `implement-team` and `watchdog` as Harness Adapters in each harness's native entry point: prompt templates in `~/.pi/agent/prompts/`, user-invoked skills in `~/.claude/skills/`, and commands in `~/.config/opencode/commands/`. Codex has no entry point that takes arguments, so it keeps plain stubs for those three. Every adapter runs its `next` command; `implement-team` adds `--mode team`. The Implement adapters take model and thinking arguments in order: `implement` takes the reviewer model and thinking level, and `implement-team` takes the helper's and then the reviewer's. An omitted argument passes an empty value, which `skl` treats as omitted. Adapters supply their harness selector and invocation overrides, with no compiled model or thinking defaults; Codex's `implement-team` stub passes the mode and selector. The installer touches only files it owns and retires the ones it used to own: the `tdd` stub and the earlier Pi runners, loop prompts and queue helper. Stubs and adapters read the running binary, so deliver binary or prose changes with `go install ./cmd/skl` and run `skl install` again. This refreshes owned files without changing loaded sessions or running workers. After this one-time update, editing launch choices in the configuration requires no rebuild, adapter reinstall or Supervisor restart.

Before changing files, `skl install` previews each create, update and removal and asks `Install these changes? [y/N]`. Enter `y` or `yes` to proceed; Enter, no answer or any other response cancels without changes. There is no confirmation-bypass flag. An unchanged installation says “Already up to date” without prompting, and user-owned collisions are identified separately. After acceptance, the command reports completed changes and confirms success. On failure it reports the completed changes and failed action, leaves completed changes in place, and exits with an error rather than rolling back.

## Retrieving skills

```sh
skl skill <name>                                  # a Craft or user-invoked Procedure
skl skill --resource DEEPENING.md design          # one named Skill Resource
skl skill --resource ledger-submission.md --describe-inputs implement
skl skill --resource ledger-submission.md --input result_directory=/tmp/r --input procedure=initial implement
skl skill --resource ledger-review.md --input result_directory=/tmp/r --input round=2 --input reviewed_head=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa watchdog
```

Resource names are exact and relative to the skill's resource directory. Parameterized resources take repeated `--input name=value`; `--describe-inputs` lists accepted names, types and choices without rendering. Plain `skl skill implement` and `skl skill watchdog` refuse, because those Procedures only make sense for a selected and claimed Slice; use `skl implement next` and `skl watchdog next`. Invoke `walkthrough <proposal>` or `walkthrough <proposal>/<slice>` through its installed Skill Stub to tour recorded commitments, delivered code, findings and open Manual Verification one stop at a time. It uses read-only `skl browse`, `skl ledger show` and git inspections, and makes no changes.

## Proposal operations

Propose prepares an intake directory with `proposal.md`, `proposal.json` (name, optional top-level `branch` for the Proposal Branch, optional parent title, slice titles, planned Slice branches, dependencies), and per-slice Contract files. When omitted, the Proposal Branch defaults to `proposal/<proposal-name>`; acceptance validates it with Git and reserves it against all Project branches, including archived records. Public issue bodies are separate, deliberately authored files.

```sh
skl proposal accept --repo <path> --proposal-dir <dir> \
  --issue <slice>=<public-body.md> [--parent-body <parent.md>]
skl proposal publish --repo <path> --proposal <proposal> --issue <slice>=<fresh-body.md>
skl ledger show --repo <path> --item <proposal>/<slice>
skl proposal export --repo <path> --proposal <proposal> --to <dir>
skl proposal amend --repo <path> --proposal-dir <dir> \
  [--issue <slice>=<public-body.md>]... [--parent-body <parent.md>]
```

All `proposal` operations support `--format markdown|json`, defaulting to Markdown. Acceptance, export, amendment and publication resolve `--repo` and `--remote` as before; retirement requires an explicit `--project` and never infers it from the working directory. These replace `ledger accept/export/amend/publish` and `decision retire`, without aliases. Coordinate the binary and installed adapter upgrade between runs with `skl install`; existing Claims and historical records remain intact.

`accept` freezes the Contracts under `projects/<repository>/` and commits locally, then attempts replication and issue publication. Unchanged acceptance is idempotent; a changed intake of an accepted Proposal is refused, and `amend` records it instead. A Project names exactly one source repository, and a different repository with the same name is refused. `publish` updates or creates the issues for an accepted proposal at any later time from freshly written prose. `show` returns the frozen Contract and the current report references; add `--phase implement` or `--phase watchdog` for a report body, or `--commit <sha> --path <ledger-path>` for any exact historical document. `export` writes one active accepted Proposal back out as an intake directory: `proposal.json` with the recorded Proposal Branch, parent title and every Slice's title, planned branch and Dependencies, `proposal.md`, and each Slice's frozen Contract files byte for byte. A Superseded Slice is declared with `"superseded": true`; acceptance refuses that flag on a new Proposal and compares it on re-acceptance, so accepting an unchanged export reports `existing`. Export reads the committed ledger head, names that commit in its outcome, changes no ledger record and contacts no forge. It refuses an unknown or archived Proposal (read archived records with `skl browse`) and a destination that exists and is not empty.

`amend` takes an edited export of an accepted Proposal and records every difference from its committed record as one ledger commit whose message summarises the changes, or refuses and writes nothing. It may add Slices, recorded as acceptance records them and taking the Proposal's place in work selection; replace the Contract files, title or planned branch of a Slice that was never claimed (Ready for Implementation with no Claim, report, Submission or Human Decision; a Claim released without a handoff leaves none); change the Dependencies of any Slice that holds no Claim and is not Merged or Superseded; replace `proposal.md` or the parent title, so a single-Slice Proposal can grow into a multi-Slice one; and supersede, through `"superseded": true`, any Slice that holds no Claim and is not Merged, whatever its lifecycle. A superseded Slice keeps its Contract, reports, Submission and issue attachments, and only its `state.json` changes; no decision record is written and its issue and pull request are not contacted. Superseding a Needs Human Slice removes its request from the Decision Inbox, and a later answer to it is refused.

Every added or changed Dependency must name a recorded Slice of the Project; no Slice of the amended Proposal that is neither Merged nor Superseded may depend on a Superseded Slice, including edges the amendment left untouched, so its dependents are repointed or superseded in the same intake; and the resulting graph, including other Proposals' recorded edges, must be acyclic. Active Slices of other Proposals left depending on a Slice superseded here are listed in the outcome, and an amendment of their own Proposal removes the edge. It refuses a Proposal that is unrecorded, archived or finished (every Slice Merged or Superseded), a change to the Proposal Branch, and a recorded Slice missing from the declaration. It refuses the `superseded` flag on a claimed, Merged or added Slice, a Slice superseded together with any change to its Contract, title, planned branch or Dependencies, an issue body supplied for a Superseded Slice, and a recorded Superseded Slice declared without the flag: a Superseded Slice keeps its name, and its replacement is a new Slice. Once every Slice is Superseded or Merged and none is claimed, `skl proposal retire` retires the Proposal. An unchanged intake reports `existing`. After the commit it presents, through the publication path, every added or changed Slice other than one superseded, and every Slice a supplied body names, plus the parent; when it creates the parent issue, the issue of every Slice that is not Superseded is grouped under it. A listed Slice without a body is named with a bound `publish` command, and the outcome lists the active Slices that depend on each changed Slice for the human to confirm.

## Delivery lanes

`skl implement` and `skl watchdog` share one command set. Every command takes `--repo` and `--remote`; the Execution Skill binds the rest.

| Command | What it does |
|---|---|
| `next` (alias `start`) | Selects one eligible Slice in this repository's Project, records a Claim, and returns the Execution Skill. Rework precedes new work; older acceptance precedes newer. Claimed Slices and Slices with unmerged blockers are skipped. |
| `resume --item --claim` | Continues exactly that reservation. It cannot take over or clear a later one. |
| `prepare` | Creates or safely reuses the planned branch and `.worktrees/<branch>`; never resets, rebases or stashes. Watchdog preparation of a Slice targeting a Proposal Branch also performs the Integration Merge: it fetches the branch and, when its head is not already an ancestor, commits one ordinary merge of it into the work branch. A conflict aborts the merge, leaves the worktree clean and refuses; the review then ends in `rework`. |
| `inspect` | Read-only look at the prepared workspace; returns a narrow continuation. |
| `submit --head --target --body [--public-body]` | Records the phase report, moves Workflow State and releases the Claim in one ledger commit, then attempts push and public presentation. |
| `needs-human --body [--head --target]` | Implement only. Records a blocker question for a human and releases the Claim. |
| `release --item --claim` | Gives the reservation back, preserving any source progress. |

Only `next` takes `--auto` (also with `--dispatch`). An implement lane's first Claim on a closed Proposal records the Proposal Branch as open; source preparation creates that remote branch from fetched `main` if absent. Later Claims take the open branch even without `--auto`. A Slice keeps its Claim-time Integration Target across rework and review. Auto Mode is meant for one implement lane and one watchdog lane per Project; this is guidance, not an enforced limit. Without `--auto` and without an open Proposal Branch, Slices target `main`.

`watchdog next --auto` records Auto Mode on its Claim. When that Claim's `pass` targets an open Proposal Branch, submit squash-merges the presented pull request through the forge, pinned to the final head, and only when the branch's current remote head is an ancestor of that head. Before selecting, every `watchdog next --auto` poll retries the merge of each Ready-for-Merge Slice with an attached pull request on an open Proposal Branch, including Slices passed without the flag. A refusal (ancestry, pending presentation, branch protection or another forge error) is reported in the outcome and leaves the Slice `ready_for_merge`; the handoff still succeeds. A merge writes no Workflow State: the Slice becomes `merged` when completion observation reads the merged pull request, which the same poll does before selecting. Nothing targeting `main` is merged.

Implement `next`, `resume`, `prepare` and `inspect` also take `--mode standard|team` (default `standard`), and the opaque values `--helper-model`, `--helper-thinking`, `--reviewer-model` and `--reviewer-thinking`. Team mode replaces the Execution Skill's delegation guidance with the team strategy. A reviewer value renders in the Audit dispatch step and a helper value in the team delegation step; helper values need team mode. An unknown mode is refused before anything is claimed, and the bound prepare, inspect and resume commands repeat the supplied values.

A Claim is a local ledger commit protected by a short lock. It never expires, so an interrupted worker is resumed or released explicitly rather than timed out. Implement submit records `awaiting_review` or `needs_human`. Watchdog submit takes `--outcome pass|rework|needs-human`: `pass` records `ready_for_merge`; the first `rework` returns the Slice to implementation, a second records `needs_human`. Success is recorded locally even when the push, replication or public presentation is still pending.

Reports are Markdown bodies the worker writes from a template the engine hands over at the right step; the engine adds the metadata and exact input references described in [report-schema.md](report-schema.md) and never infers a verdict from prose.

`next` checks once by default. `--wait` (bare: 15 minutes, or `--wait 2m`) keeps polling every `--poll` (default 30 seconds) and ends with work, a refusal, or `idle_timeout`, which means the queue was quiet, not that everything is done.

## Supervisors

```sh
skl implement next --dispatch --wait --repo <path> [--worker-model <m>] [--worker-thinking <t>]
```

A Dispatch claims a Slice like `next` but answers with two bound commands instead of the Execution Skill: a worker command (`skl <phase> resume … --dispatched`) that a fresh subagent runs, and a continue command that repeats the Dispatch with `--after <claim>`. Continuation first reads how that Claim ended and proceeds only after its phase handoff. A Claim still held or released without a handoff stops the Supervisor with the Claim untouched. Model and thinking values are passed through opaquely to the harness.

Pi, OpenCode and Claude Code install both entry-point families, independently of local harness configuration:

| Ordinary loop | Auto Mode loop | Optional arguments, in order |
| --- | --- | --- |
| `implement-loop` | `implement-auto` | worker-model, worker-thinking, reviewer-model, reviewer-thinking |
| `implement-team-loop` | `implement-team-auto` | worker-model, worker-thinking, helper-model, helper-thinking, reviewer-model, reviewer-thinking |
| `watchdog-loop` | `watchdog-auto` | worker-model, worker-thinking |

Ordinary loops omit `--auto`; Auto Mode entries include it without needing any model/thinking arguments. Neither family has a positional auto toggle. Both start the same dispatch and waiting commands with their harness selector, using the runtime configuration and field-by-field precedence described above. Pi uses positional prompt arguments, OpenCode uses positional command arguments, and Claude Code uses named skill arguments (for example, `worker_model`). Omitted values remain configured or inherited. Direct CLI `next --auto` remains available. Codex has no loop/auto adapters.

Pi workers retain ordinary extension-enabled subagent tool loading and scope: codemode is available when enabled under the normal Pi session configuration. Availability does not require the model to call it.

## Presenting a result

```sh
skl ledger present --repo <path> --item <proposal>/<slice> [--public-body <fresh-prose.md>]
```

Without `--public-body` it returns the current committed result with the Work Item's completed Review Count, private evidence references to its report and every ledger input that report consumed, and authoring guidance for a self-contained PR body. With one it pushes the recorded source revision (never forcing), opens or updates the pull request as the latest view of that result, and marks it ready only when the PR shows the reviewed final revision. It reruns no phase and changes no state; a pull request is a view of the local result, not a queue of updates.

## Human decisions

```sh
skl decision inbox [--project <name>]
skl decision apply --project <p> --slice <proposal>/<slice> \
  --request-commit <sha> --request-path <ledger-path> \
  --route implement|watchdog|supersede --answer <answer.md>
skl proposal retire --project <p> --proposal <proposal>
```

`inbox` reads every current Needs Human request from the ledger, so it works from any directory; an unreadable ledger is reported as unavailable, never as empty. Each request comes with the worker's question, evidence, options and recommendation, and a bound `apply` command. The route is part of the answer: `implement` requeues, `watchdog` returns to review at the same revision, `supersede` abandons the unmerged work. `proposal retire` records retirement of a proposal parent once at least one slice is superseded and nothing active remains; merged slices and frozen records are preserved. Retirement leaves ledger records in place and preserves source work; `skl cleanup` separately handles archival and safe local source cleanup. A Proposal with an open Proposal Branch and no recorded completion can be retired once nothing active remains, even when every Slice merged; retirement spends the branch and leaves the remote branch alone. A recorded Proposal Submission is observed first: retirement is refused while the forge is unreachable, and refused when the pull request has merged, so the next `skl status` records the Proposal's completion instead. Changed obligations go back through `explore` and `propose`, not through a decision.

## Browsing

`skl browse` opens a terminal browser over the ledger: Projects, their Proposals (including the Proposal Branch as open or spent, or its absence for legacy records, and the Proposal Submission), and each Slice's state, Claim, dependencies, branch, attachments and the branch it merged into. It starts at the current checkout's Project when there is exactly one.

To find Slices, `f` picks a lifecycle or Claim from the counted facts of the current Project (every Project from the overview), and `/` searches Project, Proposal and Slice names and Slice titles. The criteria narrow the same results together. In the results, `w` switches between the current Project and every Project, `g` groups by Proposal or lifecycle, `d` opens scrollable membership diagnostics (`esc` returns to results), and `enter` opens the Slice. Slices whose unreadable records leave a criterion undecided are listed apart, never counted as matches.

A Slice shows both Depends on and Blocks relationships, including targets in other or archived Proposals. `tab` / `shift+tab` select a relationship, `enter` opens it without changing archive visibility, and `esc` restores the previous browsing context.

```sh
skl browse projects [--include-archived]
skl browse project --project <name> [--include-archived]
skl browse proposal --project <name> --proposal <proposal> [--archived]
skl browse slice --project <name> --item <proposal>/<slice> [--archived]
skl browse slices [--project <name>] [--lifecycle <state>]... [--claim implement|watchdog|none]... [--search <text>] [--group proposal|lifecycle] [--include-archived]
skl browse documents --project <name> --proposal <proposal> [--archived]
skl browse documents --project <name> --item <proposal>/<slice> [--archived]
skl browse document --commit <full-ledger-commit> --path <ledger-path>
```

Detail queries prefer active records when active and archived Proposals share a name; `--archived` explicitly selects the archived one. `browse slice` exposes both relationship directions. A Dependency is satisfied only when its blocker is Merged into `main`, the dependent's own Proposal Branch, or another Proposal Branch whose Proposal records completion into `main`. A blocker Merged only into another open Proposal Branch does not unblock it. Missing or unreadable targets remain explicitly unresolved; reverse-dependency results disclose when unreadable records make them incomplete.

`browse documents` reads the Proposal description with every Slice's documents, or one selected Slice's documents. A Slice's documents are its accepted Contract, latest committed implementation and Watchdog reports, and current Human Decision. It also returns structured metadata for reports and decisions alongside their Markdown. An absent optional report is marked not yet available; unreadable or malformed records carry diagnostics, while readable document content remains available. Its `--archived` flag selects the archived Proposal when an active one shares its name.

`browse document` follows one exact ledger reference, including a historical commit and the path valid there. It never substitutes the latest version. Output identifies the ledger snapshot and document revisions separately; source-repository revisions in report metadata are labeled separately from ledger references. An unavailable exact reference is reported as unavailable.

Every view reads one committed ledger revision, never fetches or contacts a forge, and changes nothing. A Claim is shown as a reservation, not as a running worker. Markdown is the default transport; add `--format json` for the same facts and document content as structured JSON. Existing `skl ledger show` remains the byte-preserving raw exact-readback command.

## Status and cleanup

`skl status` requires ledger configuration and an accepted Project. It reads the Project's Workflow State and observes attached submissions for merge or closure; without configuration or adoption it reports how to configure or accept work, rather than inferring state from GitHub. `--item <proposal>/<slice>` limits the status read to a fixed Work Item.

Status and every `implement next` poll also keep the Proposal Submission current. Once a Slice is observed Merged into an open Proposal Branch, they open a draft pull request from that branch into `main`, or attach the one already open, and record it on the Proposal. Each later Slice merge re-renders its body from the ledger, and it is marked ready once every Slice is Merged; a ready pull request is never returned to draft. When it merges, the Proposal records its completion and the branch is spent; a pull request closed without merge is reported and nothing is recorded. Status shows the Proposal Branch as open or spent, the Proposal Submission and its completion.

```sh
skl cleanup [--repo <path>] [--remote <name>] [--format markdown|json]
```

`skl cleanup` replaces the removed `skl propose cleanup` command and its now-empty command group; the standalone `skl skill propose` remains available. Coordinate this syntax upgrade between runs: finish or stop active workers and Supervisors, then update the binary and refresh installed adapters together with `skl install`. Existing Claims and ledger history remain intact.

`skl cleanup` defaults to the current repository and the `origin` remote, with Markdown output. It also requires ledger configuration and an accepted Project. It archives terminal proposals and removes only safely merged local source work. Without that authority, cleanup reports how to configure the ledger and accept a Project, without consulting GitHub. Accept and publish Proposals through `skl proposal accept` and `skl proposal publish`.
