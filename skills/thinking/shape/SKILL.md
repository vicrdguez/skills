---
name: shape
description: Develop an idea into a faithful, implementation-independent design.
disable-model-invocation: true
---

# Shape

Think with the user until the idea becomes coherent and bounded. Keep the conversation fluid: reflect emerging understanding, surface tensions, contribute alternatives, and follow the threads the user considers important. Favor coherence over exhaustive coverage; the user decides when the idea is sufficiently shaped.

Work from the current conversation or an exact source path supplied by the user. Never search or enumerate `.thinking/` to find input.

## Artifact

When the user chooses to preserve the shape, write it to `<project-root>/.thinking/<topic>/shape.md`. If the source is a brainstorm artifact, place the shape beside it. If the destination exists, ask before replacing it. Report the exact output path.

Produce a faithful, implementation-independent design rather than a transcript. Choose headings that suit the idea. Make its intent, shaped concept, boundaries, important rationale, and remaining uncertainty clear without forcing a fixed template. Retain significant alternatives and reasoning; compress repetition, not substance. A protocol may naturally organize itself around roles, interactions, guarantees, and failure behavior.

Keep the result portable across codebases and technologies. Work from the conversation and sources supplied by the user. Leave repository investigation, exhaustive decision trees, implementation choices, and durable project documentation to `explore`.

In a Git repository, add `/.thinking/` to the repository-local exclude returned by `git rev-parse --git-path info/exclude` before writing, if absent. Keep tracked ignore files untouched.
