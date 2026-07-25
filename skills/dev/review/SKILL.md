---
name: review
description: Review the changes for the specific since a fixed point (commit, branch, tag or merge-base) along two axes - Standards (does the code follow this repo documented coding standards) and change Artifacts (does the code match what the originating change asked for?) Runs both reviews in parallel subagents and reports them side by side. Use when the user wants to review a branch, a PR, work-in-progress changes or asks to "review since X"
---

Two-axis review of the diff between `HEAD` and a fixed point the user supplies:

- **Standards** — does the code conform to this repo's documented coding standards? This include any documented standard in the repo but also make use of project/repo specific skills to aid here. E.g. the repo has a specific skill to review a module or a layer present in the project
- **Artifacs** — does the code faithfully implement the originating intent, behaviors, plan and tasks?
  1. Every `intent.md` item in "Definition of Done" is demostrably met
  2. Every `behavior.md` scenario has materialized as a test (or for prose changes, encoded)
  3. The full suite is green
  4. Read `plan.md` against the diff. If the implementation diverged, note it.

Both axes run as **parallel sub-agents** so they don't pollute each other's context, then this skill aggregates their findings.

Use  `docs/github.md` to know how to do board operations.


## Process

### 1. Pin the fixed point

Look for the first commit the implementor for this change did. The goal is to review the work done for the single claimed unit of work.


If the user provides the fixed point — a commit SHA, branch name, tag, `main`, `HEAD~5`, etc. then use that instead, and specially if there is no unit of work done and this skill has been invoked independently, ask for it if they didn't specify one.

Capture the diff command once: `git diff <fixed-point>...HEAD` (three-dot, so the comparison is against the merge-base). Also note the list of commits via `git log <fixed-point>..HEAD --oneline`.

Before going further, confirm the fixed point resolves (`git rev-parse <fixed-point>`) and the diff is non-empty. A bad ref or empty diff should fail here — not inside two parallel sub-agents.

### 2. Identify the artifacts source

Look for the originating artifacts, in this order:

1. Issue references in the commit messages (`#123`, `Closes #45`, etc.) — fetch via the workflow in `docs/github.md`.
2. A path the user passed as an argument.
3. Artifacts in `.changes/<slug>` for the in-flight unit of work matching the branch name or feature
4. If nothing is found, ask the user where the artifacts are is. If they say there isn't one, the **Artifacts** sub-agent will skip and report "no Artifacts available".

### 3. Identify the standards sources

Anything in the repo that documents how code should be written, such as `CODING_STANDARDS.md` or `CONTRIBUTING.md`.

On top of whatever the repo documents, the Standards axis always carries the **smell baseline** below — a fixed set of Fowler code smells (_Refactoring_, ch.3) that applies even when a repo documents nothing. Two rules bind it:

- **The repo overrides.** A documented repo standard always wins; where it endorses something the baseline would flag, suppress the smell.
- **Always a judgement call.** Each smell is a labelled heuristic ("possible Feature Envy"), never a hard violation — and, like any standard here, skip anything tooling already enforces.

Each smell reads *what it is* → *how to fix*; match it against the diff:

- **Mysterious Name** — a function, variable, or type whose name doesn't reveal what it does or holds. → rename it; if no honest name comes, the design's murky.
- **Duplicated Code** — the same logic shape appears in more than one hunk or file in the change. → extract the shared shape, call it from both.
- **Feature Envy** — a method that reaches into another object's data more than its own. → move the method onto the data it envies.
- **Data Clumps** — the same few fields or params keep travelling together (a type wanting to be born). → bundle them into one type, pass that.
- **Primitive Obsession** — a primitive or string standing in for a domain concept that deserves its own type. → give the concept its own small type.
- **Repeated Switches** — the same `switch`/`if`-cascade on the same type recurs across the change. → replace with polymorphism, or one map both sites share.
- **Shotgun Surgery** — one logical change forces scattered edits across many files in the diff. → gather what changes together into one module.
- **Divergent Change** — one file or module is edited for several unrelated reasons. → split so each module changes for one reason.
- **Speculative Generality** — abstraction, parameters, or hooks added for needs the artifacts don't have. → delete it; inline back until a real need shows.
- **Message Chains** — long `a.b().c().d()` navigation the caller shouldn't depend on. → hide the walk behind one method on the first object.
- **Middle Man** — a class or function that mostly just delegates onward. → cut it, call the real target direct.
- **Refused Bequest** — a subclass or implementer that ignores or overrides most of what it inherits. → drop the inheritance, use composition.

### 4. Spawn both sub-agents in parallel
%% This line in particular, should be adapted to also work with pi-subagents%%
Send a single message with two `Agent` tool calls. Use the `general-purpose` subagent for both.

%% Unsure if the rest of this section fits the current reviewer workflow that uses github comments. We need to reconcile this%%
**Standards sub-agent prompt** — include:

- The full diff command and commit list.
- The list of standards-source files you found in step 3, **plus the smell baseline from step 3** pasted in full — the sub-agent has no other access to it.
- The brief: "Report — per file/hunk where relevant — (a) every place the diff violates a documented standard: cite the standard (file + the rule); and (b) any baseline smell you spot: name it and quote the hunk. Distinguish hard violations from judgement calls — documented-standard breaches can be hard, but baseline smells are always judgement calls, and a documented repo standard overrides the baseline. Skip anything tooling enforces. Under 500 words."

**Artifacts sub-agent prompt** — include:

- The diff command and commit list.
- The path or fetched contents of the Artifacts.
- The brief: "Report: (a) requirements, intent and behaviors the artifacts asked for that are missing or partial; (b) behaviour in the diff that wasn't asked for (scope creep); (c) requirements that look implemented but where the implementation looks wrong. Quote the spec line for each finding. Under 500 words."

If the Artifacts is missing, skip the Artifacts sub-agent and note this in the final report.

### 5. Aggregate

Present the two reports under `## Standards` and `## Artifacts` headings, verbatim or lightly cleaned. Do **not** merge or rerank findings — the two axes are deliberately separate (see _Why two axes_).

End with a one-line summary: total findings per axis, and the worst issue _within each axis_ (if any). Don't pick a single winner across axes — that's the reranking the separation exists to prevent.

## Why two axes

A change can pass one axis and fail the other:

- Code that follows every standard but implements the wrong thing → **Standards pass, Artifacts fail.**
- Code that does exactly what the issue asked but breaks the project's conventions → **Artifacts pass, Standards fail.**

Reporting them separately stops one axis from masking the other.
