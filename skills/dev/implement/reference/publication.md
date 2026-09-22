# Pull request progress body

Author the deliberately public PR body at `{{.ResultDirectory}}/public.md`. The engine transports those bytes verbatim; it neither generates nor summarizes them and never stores them in the private ledger.

Write a descriptive progress presentation:

- The committed change and the scope it implements against the accepted Contract.
- Current progress: what is delivered, what remains, and the reviewed source head the presentation describes.
- Useful verification: the checks and results that a reviewer can weigh, with material limitations stated plainly.
- Material risks or uncertainties a human should know about.
- Appropriate human checks that remain open.

Describe the delivered behavior and evidence in your own words. Do not paste the private implementation report, the Audit ledger, worker exchanges, or raw decision history. A reader without private access should understand what changed, how it was verified, and what still needs a human.

## Boundaries

- The frozen Contracts and the committed local result remain the authority; this public body is a description, not a verdict or a second Contract.
- The complete Manual Verification obligations remain privately accessible and human-owned through the exact references supplied by the invocation. Public omissions never remove them or mark them satisfied.
- Do not publish a private report, a credential, an internal path, or another operational detail.
- Do not mutate GitHub directly and do not edit the ledger. Continue only with the bound `skl publication recover` command from the invocation; it picks up `public.md` from this result directory.
