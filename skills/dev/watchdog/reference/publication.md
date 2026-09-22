# Final Review Package

Author the deliberately public Final Review Package at `{{.ResultDirectory}}/public.md` for the latest approved review result. The engine transports those bytes verbatim; it neither generates nor summarizes them and never stores them in the private ledger.

Write a descriptive human-facing package:

- The delivered outcome measured against the accepted commitments and scope.
- The reviewed source revision and the final source revision the presentation describes.
- Grouped useful verification and the results a human can weigh, with material limitations stated plainly.
- Material risks, residual limitations, or follow-up a human should know about.
- The appropriate human checks that remain open.

Copy the `Manual verification` checklist from the accepted `intent.md` into the public package with every box unchecked. Ticking those boxes remains human work; the public package may omit private operational detail but never removes an obligation.

Describe the approved outcome in your own words. Do not paste the private Watchdog report, the complete Phase Report, the Audit ledger, or raw decision history. Detailed findings stay private: only an explicitly selected actionable finding with its reviewed commit, path, line, and side may be published inline, through its own explicit selection.

## Boundaries

- The frozen Contracts and the recorded review result remain the authority; this public package is a description, not a verdict or a second Contract.
- The complete Manual Verification obligations remain privately accessible and human-owned through the exact references supplied by the invocation. Public omissions never remove them or mark them satisfied.
- Do not publish a private report, a credential, an internal path, or another operational detail.
- Do not mutate GitHub directly and do not edit the ledger. Continue only with the bound `skl publication recover` command from the invocation; it picks up `public.md` from this result directory.
