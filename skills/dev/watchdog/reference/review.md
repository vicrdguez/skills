# Review Result Documents

Write opaque Markdown in the packet's private temporary directory. `summary.md` contains the current finding ledger, stable `W<n>` identities, review round, reviewed head, evidence and dispositions. On a repeat, preserve prior identities and record the supplied human directives you honored. The verdict is a command flag, not parsed from this document.

Finding IDs are local to the PR and monotonic: the same defect keeps its ID for the life of the Submission. The latest summary is the durable ledger of every still-active finding, with `BLOCK`, `HUMAN`, or `NOTE` dispositions. Include the full `Reviewed head` SHA so a repeat worker can recover its fixed point without CLI prose parsing.

An owner, member, or collaborator may direct `W1 WAIVE` (accepted without debt), `W1 BLOCK`, or `W1 NOTE` in a later comment, with a free-form reason. Interpret those directives case-insensitively after trimming; only comments after the finding count, and the latest authorized directive wins. Never infer a decision from reactions, silence, or deleted comments. No stage creates follow-up issues on its own; that remains human or Propose work.

Optional `findings.json` transports structured anchors, not finding prose:

```json
[{"path":"src/example.go","line":12,"side":"RIGHT","body_file":"/absolute/private/directory/W1.md"}]
```

Use `LEFT` for an old-side line or `RIGHT` for a new-side line. Write the finding's opaque Markdown in its `body_file`; anchors use the submitted fixed head. Keep each finding's identity in its agent-authored body.

On pass, `submission.md` is the complete final PR body including the historical Manual Verification section verbatim with every checkbox unchecked. Preserve the Audit ledger and review evidence. The engine appends `Closes #<source-issue>`; no archive or source-issue close occurs at Ready for Merge.
