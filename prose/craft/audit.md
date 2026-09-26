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

