# Audit

Review the change along two independent axes, each by its own reviewer:

- **Standards**: does the code follow this repository's documented standards, required tooling, project quality skills and the smell baseline?
- **Contracts**: does the code deliver the complete accepted behavior, architecture and Definition of Done, with evidence behind each?

Code inherited unchanged from a merged target is outside the review. The effects of the merge and its conflict resolutions are inside it.

When sources disagree, the higher one wins: the accepted Contract, then required tooling and CI, then the project's `AGENTS.md`, standards documents and quality skills, then language and framework correctness, security and accessibility, then the smell baseline.

## Standards brief

Read the diff, the callers it affects, the standards sources and the smell baseline. Report, per file and hunk, every place the diff breaks a documented standard, citing the file and rule, and every baseline smell you spot, naming it and quoting the hunk. For a simplification, name the simpler alternative, the burden it removes, and why behavior and verification stay intact. Skip anything tooling enforces.

## Contracts brief

Judge the complete final implementation against every accepted behavior, scenario, architecture commitment and Definition of Done item, even when the diff is only the latest increment. Apply the acceptance criteria. Report behavior that is missing, partial, wrong or unasked for; plan commitments the code breaks; and specific evidence gaps, naming the obligation and the plausible violation the existing evidence cannot distinguish. Quote the Contract line for each finding. Reports and notes written after the Contract was accepted are evidence, not requirements. Note where the implementation diverges from the plan.

## Tag every finding

Start each finding with `HARD` or `JUDGEMENT` and anchor it to `file:line`. Contract violations, material risks, specific evidence gaps and a red gate are `HARD`. A breach of an explicit mandatory standard can be `HARD`. Generic smells are always `JUDGEMENT`, and so is a plan divergence that breaks no accepted obligation. Keep each report under 500 words by compressing findings rather than dropping any.

## Aggregate without reranking

Number each new finding `F<n>`, continuing after the greatest existing `F<n>`. Present the reports under `## Standards` and `## Contracts`, each finding keeping the tag its axis gave it: the axis that found it saw the evidence, and you did not. Merge nothing across axes. End with the `HARD` and `JUDGEMENT` counts and the worst issue within each axis, beside the gate result.

## Why two axes

A change can pass one axis and fail the other:

- Code that follows every standard but implements the wrong thing: **Standards pass, Contracts fail.**
- Code that does exactly what the Contract asked but breaks the project's conventions: **Contracts pass, Standards fail.**

Reporting them separately stops one axis from masking the other.

## Pin the comparison

Use the fixed point the user gave: a commit, branch, tag or merge-base. When none was given, ask for one. Resolve it with `git rev-parse <fixed-point>` before going further. The diff command is `git diff <fixed-point>...HEAD`, and the commit list is `git log <fixed-point>..HEAD --oneline`. An empty diff is legal: judge the complete implementation.

## Find the Contract

When the branch belongs to a ledger Work Item, read its Contract with `skl ledger show --item <proposal>/<slice>`. Otherwise use the issue or path the user supplies. When there is no Contract, skip the Contracts axis and say so in the report.

## Run the gate once

Run the project's full test, typecheck and lint suite once and record the commands, head and results.

Check: you hold the diff command, the commit list, the gate result and the Contract, or the note that there is none.

## Dispatch the reviewers

Give each reviewer its brief and everything the check above names, the Contract included. Both reviewers use the recorded gate result. The Standards reviewer retrieves the smell baseline with `skl skill --resource smells.md audit`; both reviewers retrieve the acceptance criteria with `skl skill --resource acceptance.md audit`. Reviewers read and report.

Run the axes at once as fresh-context subagents when you can spawn them. Otherwise run them yourself in sequence, Standards first, finishing its report before starting Contracts.

Aggregate the reports as above. Check: the report carries every axis that ran, its tagged `F<n>` findings and the gate result.

