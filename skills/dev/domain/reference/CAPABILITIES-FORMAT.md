# Capability Doc Format

Capability docs live in `docs/capabilities/` — one capability per file, kebab-case (`order-cancellation.md`). They answer "what can the system do?" at the level of observable behavior, complementing `CONTEXT.md` (what words mean) and ADRs (why it is this way).

Create the directory and files lazily — only when there is a capability worth recording. Update a capability doc in the same change that alters, extends, or retires its behavior (`/propose` reads these to see what a change touches; `tasks.md` carries the update as the final doc task).

## Template

```md
# {Capability name}

{1-3 sentences: what the system does, in CONTEXT.md vocabulary, from the caller's
or user's perspective.}

## Behaviors
- {One line per observable behavior currently supported}

## Out of scope
- {Adjacent things this capability deliberately does not do}
```

## Rules

- **Observable behavior only.** No implementation details, no module names, no seams — those live in the code, `plan.md`, and ADRs.
- **Use `CONTEXT.md` vocabulary.** If a capability doc needs a term the glossary lacks, resolve the term in `CONTEXT.md` first.
- **Keep it current or delete it.** A stale capability doc is worse than none; if a change retires a capability, delete or edit the file in that change.
