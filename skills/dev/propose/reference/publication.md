# Descriptive issue body

Author the deliberately public issue body at `{{.ResultDirectory}}/public.md`. The engine transports those bytes verbatim; it neither generates nor summarizes them and never stores them in the private ledger.

For multi-slice work the owner authors distinct, self-contained presentations:

- **Parent issue** — the whole proposal's human-facing commitment: the problem, the shared outcome, a descriptive slice breakdown, sequencing, and what a person should check before accepting the direction.
- **Child issue** — one slice's own scope and value: what it delivers, what it deliberately excludes, its declared dependencies described plainly, and how a person can tell it is done.

Describe the accepted intent and boundaries from the frozen `intent.md` and `behavior.md` in your own words. Do not paste the Contracts, worker exchanges, or raw decision history. A reader without private access should still understand the intent, the expected outcome, and the limits.

## Boundaries

- The frozen Contracts remain the authority; this public body is a description, not a second Contract.
- The complete Manual Verification obligations remain privately accessible and human-owned through the exact references supplied by the invocation. Public omissions never remove them or mark them satisfied.
- Do not publish a private report, a credential, an internal path, or another operational detail.
- Do not mutate GitHub directly and do not edit the ledger. Continue only with the bound `skl publication recover` command from the invocation; it picks up `public.md` from this result directory.
