# Journey

Write each journey at `.skl/journeys/<name>.md`, where `<name>` is its catalog row's name, with these sections:

## Preconditions

What must hold before the first step: the build, the running services, the accounts or fixtures, each with the command that sets it up or checks it.

## Steps

Numbered steps, each one action followed by what to observe after it: the exact command, request or interaction, then the output, state or screen that shows the step worked.

## Sanity predicate

One observable end state that proves the journey itself works on any slice, independent of the rule a Contract cites it for, and the command or observation that checks it.

## Cleanup

The commands that return the system to the preconditions, so the next run starts clean.

## Traps

What makes this journey fail or pass for the wrong reason.

A Contract that cites the journey supplies the expected behavior; the journey supplies how to reach and observe it.
