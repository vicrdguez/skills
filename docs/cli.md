# skl command reference

This is the operator's view of the CLI: what each command does, when it runs, and what it returns. Most commands are run by an agent from an installed stub or an Execution Skill, so their flags are usually already bound when a human sees them. Every command defaults to Markdown output; `--format json` gives the same facts as typed transport. Refusals name the invariant that stopped the command and how to repair it.

## Configuration

`skl` reads one file, `$XDG_CONFIG_HOME/skl/config.json` (falling back to `~/.config/skl/config.json`), with one key:

```json
{"ledger": "/absolute/path/to/ledger-clone"}
```

The path must be an existing local Git clone with at least one commit, separate from any source repository. The clone's own remote and upstream decide where the ledger replicates; `skl` provisions no hosting. A missing or broken configuration is a repairable refusal, never a fallback to forge-authoritative delivery.

## Setup and install

`skl setup [--repo <path>] [--remote <name>]` binds a consumer repository to the workflow: it writes the managed Workflow and Simplicity block into `AGENTS.md`, offers to link `CLAUDE.md` to it, updates `.gitignore`, and creates the GitHub labels used for the public view. Remote inference prefers a GitHub `origin`, otherwise the sole GitHub remote. Authentication comes from `GH_TOKEN`, then `GITHUB_TOKEN`, then `gh auth token`; nothing is stored.

`skl install` refreshes the owned skill stubs in `~/.pi/agent/skills`, `~/.codex/skills`, `~/.claude/skills` and `~/.config/opencode/skills`, and installs Implement and Watchdog as Harness Adapters in each harness's native entry point: `/implement` and `/watchdog` prompt templates in `~/.pi/agent/prompts/`, user-invoked skills in `~/.claude/skills/`, and commands in `~/.config/opencode/commands/`. Codex has no entry point that takes arguments, so it keeps plain stubs for those two. Every adapter runs its `next` command. The installer touches only files it owns and retires the ones it used to own: the `tdd` stub and the earlier Pi runners, loop prompts and queue helper. Stubs and adapters read the running binary, so rebuild after prose changes and run `skl install` again.

## Retrieving skills

```sh
skl skill <name>                                  # a Craft or user-invoked Procedure
skl skill --resource DEEPENING.md design          # one named Skill Resource
skl skill --resource ledger-submission.md --describe-inputs implement
skl skill --resource ledger-submission.md --input result_directory=/tmp/r --input procedure=initial implement
skl skill --resource ledger-review.md --input result_directory=/tmp/r --input round=2 --input reviewed_head=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa watchdog
```

Resource names are exact and relative to the skill's resource directory. Parameterized resources take repeated `--input name=value`; `--describe-inputs` lists accepted names, types and choices without rendering. Plain `skl skill implement` and `skl skill watchdog` refuse, because those Procedures only make sense for a selected and claimed Slice; use `skl implement next` and `skl watchdog next`.

## Accepting a proposal

Propose prepares an intake directory with `proposal.md`, `proposal.json` (name, optional parent title, slice titles, planned branches, dependencies) and per-slice Contract files. Public issue bodies are separate, deliberately authored files.

```sh
skl ledger accept --repo <path> --proposal-dir <dir> \
  --issue <slice>=<public-body.md> [--parent-body <parent.md>]
skl ledger publish --repo <path> --proposal <proposal> --issue <slice>=<fresh-body.md>
skl ledger show --repo <path> --item <proposal>/<slice>
```

`accept` freezes the Contracts under `projects/<repository>/` and commits locally, then attempts replication and issue publication. Unchanged acceptance is idempotent; changed obligations need a renewed proposal. A Project names exactly one source repository, and a different repository with the same name is refused. `publish` updates or creates the issues for an accepted proposal at any later time from freshly written prose. `show` returns the frozen Contract and the current report references; add `--phase implement` or `--phase watchdog` for a report body, or `--commit <sha> --path <ledger-path>` for any exact historical document.

## Delivery lanes

`skl implement` and `skl watchdog` share one command set. Every command takes `--repo` and `--remote`; the Execution Skill binds the rest.

| Command | What it does |
|---|---|
| `next` (alias `start`) | Selects one eligible Slice in this repository's Project, records a Claim, and returns the Execution Skill. Rework precedes new work; older acceptance precedes newer. Claimed Slices and Slices with unmerged blockers are skipped. |
| `resume --item --claim` | Continues exactly that reservation. It cannot take over or clear a later one. |
| `prepare` | Creates or safely reuses the planned branch and `.worktrees/<branch>`; never resets, rebases or stashes. |
| `inspect` | Read-only look at the prepared workspace; returns a narrow continuation. |
| `submit --head --target --body [--public-body]` | Records the phase report, moves Workflow State and releases the Claim in one ledger commit, then attempts push and public presentation. |
| `needs-human --body [--head --target]` | Implement only. Records a blocker question for a human and releases the Claim. |
| `release --item --claim` | Gives the reservation back, preserving any source progress. |

A Claim is a local ledger commit protected by a short lock. It never expires, so an interrupted worker is resumed or released explicitly rather than timed out. Implement submit records `awaiting_review` or `needs_human`. Watchdog submit takes `--outcome pass|rework|needs-human`: `pass` records `ready_for_merge`; the first `rework` returns the Slice to implementation, a second records `needs_human`. Success is recorded locally even when the push, replication or public presentation is still pending.

Reports are Markdown bodies the worker writes from a template the engine hands over at the right step; the engine adds the metadata and exact input references described in [report-schema.md](report-schema.md) and never infers a verdict from prose.

`next` checks once by default. `--wait` (bare: 15 minutes, or `--wait 2m`) keeps polling every `--poll` (default 30 seconds) and ends with work, a refusal, or `idle_timeout`, which means the queue was quiet, not that everything is done.

## Supervisors

```sh
skl implement next --dispatch --wait --repo <path> [--worker-model <m>] [--worker-thinking <t>]
```

A Dispatch claims a Slice like `next` but answers with two bound commands instead of the Execution Skill: a worker command (`skl <phase> resume … --dispatched`) that a fresh subagent runs, and a continue command that repeats the Dispatch with `--after <claim>`. Continuation first reads how that Claim ended and proceeds only after its phase handoff. A Claim still held or released without a handoff stops the Supervisor with the Claim untouched. Model and thinking values are passed through opaquely to the harness.

## Presenting a result

```sh
skl ledger present --repo <path> --item <proposal>/<slice> [--public-body <fresh-prose.md>]
```

Without `--public-body` it returns the current committed result, its private evidence references and authoring guidance for the PR body. With one it pushes the recorded source revision (never forcing), opens or updates the pull request as the latest view of that result, and marks it ready only when the PR shows the reviewed final revision. It reruns no phase and changes no state; a pull request is a view of the local result, not a queue of updates.

## Human decisions

```sh
skl decision inbox [--project <name>]
skl decision apply --project <p> --item <proposal>/<slice> \
  --request-commit <sha> --request-path <ledger-path> \
  --route implement|watchdog|supersede --answer <answer.md>
skl decision retire --project <p> --proposal <proposal>
```

`inbox` reads every current Needs Human request from the ledger, so it works from any directory; an unreadable ledger is reported as unavailable, never as empty. Each request comes with the worker's question, evidence, options and recommendation, and a bound `apply` command. The route is part of the answer: `implement` requeues, `watchdog` returns to review at the same revision, `supersede` abandons the unmerged work. `retire` archives a proposal parent once at least one slice is superseded and nothing active remains; merged slices and frozen records are preserved. Changed obligations go back through `explore` and `propose`, not through a decision.

## Browsing

`skl browse` opens a terminal browser over the ledger: Projects, their Proposals, and each Slice's state, Claim, dependencies, branch and attachments. It starts at the current checkout's Project when there is exactly one. Keys: `s` switches Project, `a` includes archived Proposals, `i` and `p` open the recorded issue or pull request.

To find Slices, `f` picks a lifecycle or Claim from the counted facts of the current Project (every Project from the overview), and `/` searches Project, Proposal and Slice names and Slice titles. The criteria narrow the same results together. In the results, `w` switches between the current Project and every Project, `g` groups by Proposal or lifecycle, and `enter` opens the Slice. Slices whose unreadable records leave a criterion undecided are listed apart, never counted as matches.

```sh
skl browse projects [--include-archived]
skl browse project --project <name> [--include-archived]
skl browse proposal --project <name> --proposal <proposal>
skl browse slice --project <name> --item <proposal>/<slice>
skl browse slices [--project <name>] [--lifecycle <state>]... [--claim implement|watchdog|none]... [--search <text>] [--group proposal|lifecycle] [--include-archived]
```

Every view reads one committed ledger revision, never fetches or contacts a forge, and changes nothing. A Claim is shown as a reservation, not as a running worker.

## Legacy

`skl propose publish` is the forge-era, source-artifact proposal flow. It remains for repositories that have not adopted the ledger and is refused for a ledger Project. `skl propose cleanup` and `skl status` follow the Project: for a ledger Project, cleanup archives terminal proposals and removes only safely merged local source work, and status reads the ledger; otherwise both use the forge.
