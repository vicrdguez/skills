Assume the implementation is **wrong until it proves otherwise**. A passing suite is necessary, not sufficient: weak tests pass too.

- **Judge test strength, not presence.** For each test, ask: *would this test fail if the behavior broke?* Mentally (or actually) break the behavior and check the test catches it. A green test that asserts nothing meaningful (tautological, over-mocked, asserting a constant) is a finding.
- **Scan the whole for the critical class**: security, privacy, authorization, data loss, compatibility, accessibility, an unusable path.

## What blocks

A finding can block for:

- a failing documented check;
- an unmet accepted behavior or Definition of Done item;
- incorrect behavior this change introduces;
- a material critical-class or reliability risk;
- a mandatory project or language rule (`MUST`, `ALWAYS`, `NEVER`) broken in changed code and absent from the Audit ledger;
- a mandatory finding from a project quality skill;
- material accepted behavior with no credible evidence, or a specific evidence gap: the obligation, a plausible violation, and why the current evidence cannot distinguish them;
- a test that cannot prove the behavior it claims;
- a false claim in the implementation report, or a `HARD` finding it declined.

Not every observation blocks. Ordinary polish belonged to the implementer's Audit, so little of it should remain. A `NOTE` from last round becomes `BLOCK` only on new material evidence or a human's `BLOCK`.

## Repeat reviews stay incremental

An unconstrained repeat search finds new blockers every round and never converges. Instead:

1. Verify every still-active finding against the final state.
2. Read only what changed since the previous reviewed revision, for regressions, integration effects and false claims in the updated report.
3. Scan the whole only for the critical class.

Assign a new `W<n>` only for a defect the rework introduced or a critical discovery. A pre-existing, noncritical thing you merely noticed is a `NOTE`, not another bounce.
