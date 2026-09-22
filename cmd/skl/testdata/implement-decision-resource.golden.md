# Decision Result Document

Write `/tmp/implement-result/decision.md` and replace this guidance with your own prose. This file is the invocation's Result Document, written at the destination named below. The engine publishes it unchanged and never parses, judges, or cross-checks the prose against the semantic command that carries the decision, so the document must stand on its own: state the evidence you established, the alternatives you weighed, and the outcome you recommend. Do not restate settled invocation facts as if you had re-derived them, do not claim work you did not do, and do not encode the decision as machine-readable fields for the engine to read back. The permitted decision `--reason` is carried by the semantic Needs Human command, not parsed from this file.

## Human Decision

State the frozen requirement, the mandatory project rule, or the blocking requirement that stops progress, the current Workflow State, and the completed work. Describe the options, their consequences, and your recommendation.

Implementation work exists: push the branch and also write `/tmp/implement-result/submission.md`, the applicable Submission instructions for the procedure this invocation established, then supply that file as `--body` on the Needs Human command so one draft Submission preserves the work.

Do not invent Completion, tick unfinished work, or retire an incomplete ledger during this pause.
