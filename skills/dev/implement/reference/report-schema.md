# Phase report schema 1

`implement-report.md` and `watchdog-report.md` are the durable phase results a
worker produces and the next phase consumes. Both use the same persisted
format: schema-1 YAML frontmatter followed by an opaque Markdown body. The
engine writes and reads the file in the private ledger; a worker submits its
judgment and evidence through the semantic command and never navigates or edits
the ledger record.

## Persisted shape

Implementation report:

```text
---
schema: 1
outcome: awaiting_review
source:
  head: 4f3c8a1d5b6e7092c4d8e0f1a2b3c4d5e6f70819
  target: 9a1b2c3d4e5f60718293a4b5c6d7e8f901234567
ledger:
  claim:
    commit: 2b7e6f5a4d3c2b1a09876543210fedcba9876543
    path: projects/payments/proposals/add-refunds/refund/state.json
  contract:
    - commit: 2b7e6f5a4d3c2b1a09876543210fedcba9876543
      path: projects/payments/proposals/add-refunds/refund/behavior.md
---
# Implementation
```

Watchdog report:

```text
---
schema: 1
outcome: pass
round: 2
source:
  head: 7c8d9e0f1a2b3c4d5e6f708192a3b4c5d6e7f809
  target: 9a1b2c3d4e5f60718293a4b5c6d7e8f901234567
  reviewed: 1b2c3d4e5f60718293a4b5c6d7e8f90123456789
ledger:
  claim:
    commit: 2b7e6f5a4d3c2b1a09876543210fedcba9876543
    path: projects/payments/proposals/add-refunds/refund/state.json
  contract:
    - commit: 2b7e6f5a4d3c2b1a09876543210fedcba9876543
      path: projects/payments/proposals/add-refunds/refund/behavior.md
  implement:
    commit: 5d4c3b2a109876543210fedcba9876543210abcd
    path: projects/payments/proposals/add-refunds/refund/implement-report.md
---
# Watchdog review
```

The first `---` line opens the frontmatter and the first later line that is
exactly `---` closes it. Everything after that closing line is the body, byte
for byte. A line with trailing whitespace is not the closing delimiter; within the
frontmatter it remains YAML, where a second document is refused. After the
closing delimiter, every later `---` belongs to the opaque body.

## Fields

A `Reference` is two strings: `commit` is the full lowercase 40-character
commit SHA at which `path` is valid, and `path` is a nonempty relative ledger
path with no `.` or `..` segment. A reference names the exact version read, not
the commit that first introduced the file, so an archived historical path keeps
its meaning at that revision.

| Field | Type | Required | Owner |
| --- | --- | --- | --- |
| `schema` | integer | always, exactly `1` | engine |
| `outcome` | string | always | worker, through the semantic command |
| `source.head` | commit SHA | see phase rules | worker evidence, engine-recorded |
| `source.target` | commit SHA | see phase rules | worker evidence, engine-recorded |
| `source.reviewed` | commit SHA | watchdog only | engine, from the consumed implementation |
| `ledger.claim` | Reference | always | engine |
| `ledger.contract` | Reference list | always, at least one | engine |
| `ledger.implement` | Reference | required for watchdog; optional prior input for implement | engine |
| `ledger.watchdog` | Reference | optional | engine |
| `ledger.decision` | Reference | optional | engine |
| `round` | integer | watchdog only, at least `1` | engine bookkeeping |

`round` counts completed independent reviews, not attempts and not
publications. It is omitted from an implementation report; a watchdog
report at round zero is refused. `ledger.watchdog` and `ledger.decision`
appear only when that prior review or recorded human direction was actually
consumed.

## Repository context

`source` and `ledger` name two different repositories. Every `source` value is
a revision in the Consumer Repository whose code the phase concerns. Every
`ledger` value is a commit and path in the private Workflow Ledger. A commit
SHA always retains its namespace's repository meaning, even if repositories
happen to contain the same object. Source revisions are not ledger references.

## Phase rules

An implementation report (`outcome` `awaiting_review` or `needs_human`):

- records round `0` and leaves `source.reviewed` empty;
- `awaiting_review` requires `source.head` and `source.target` together and
  means the work is ready for independent review;
- `needs_human` may record no source revisions when no workspace exists yet,
  or `source.head` and `source.target` together when one does.

A watchdog report (`outcome` `pass`, `rework`, or `needs_human`):

- records `round` of at least `1` for the completed review;
- requires `source.head`, `source.target`, `source.reviewed`, and
  `ledger.implement`;
- records `source.head` equal to `source.reviewed` for a non-passing review,
  so `rework` and `needs_human` inspect exactly the reported head;
- may record a distinct `source.head` only for `pass`, which distinguishes the
  reviewed code from permitted post-marker final code.

## Ownership boundary

The engine supplies `schema`, every `ledger` reference, and `round`. The worker
supplies the semantic `outcome`, the source revisions it knows, and the Markdown
body. The body carries the completion-and-evidence table, findings, and
limitations as opaque data. The engine preserves it unchanged and never reads
it to infer completion, obligations, or a second verdict: the typed `outcome`
is the only machine outcome channel.

## Refusal

An incompatible report is refused explicitly rather than reinterpreted. This
covers an unknown phase, a `schema` other than `1`, an absent or malformed
required field, a wrong YAML scalar type, a commit or path that is not a valid
identity, a second YAML document, and fields that do not belong to the phase
(for example a review round or a reviewed revision in an implementation
report). A missing or unrecognized schema never silently defaults to `1`, and
stored history is never rewritten to match a newer format.
