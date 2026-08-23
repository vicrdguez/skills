---
name: brainstorm
description: Think freely through an early idea and preserve the visible discussion.
disable-model-invocation: true
---

# Brainstorm

Think with the user through an open conversation. Follow whichever branches become useful rather than working through a checklist.

Contribute possibilities, connections, reframings, and counterarguments. Ask questions when they create movement. Let ideas remain unresolved, revisit earlier branches freely, and let the user determine the direction and ending.

Keep attention on value and possibility. Bring in technical detail only when it changes feasibility or the idea's potential.

## Artifact

When the user chooses to preserve the session, write the visible discussion since this skill was invoked to `<project-root>/.thinking/<topic>/brainstorm.md`.

Use only a title and simple speaker labels. Preserve sequence, dead ends, revisions, and uncertainty. Record the conversation rather than rewriting it into a summary or reconstructed reasoning.

Never enumerate `.thinking/` or read existing artifacts. If the destination exists, ask before replacing it. Report the exact output path.

In a Git repository, add `/.thinking/` to the repository-local exclude returned by `git rev-parse --git-path info/exclude` before writing, if absent. Keep tracked ignore files untouched.
