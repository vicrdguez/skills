Assume the implementation is **wrong until it proves otherwise**. A passing suite is necessary, not sufficient: weak tests pass too.

- **Judge test strength, not presence.** For each test, ask: *would this test fail if the behavior broke?* Mentally (or actually) break the behavior and check the test catches it. A green test that asserts nothing meaningful (tautological, over-mocked, asserting a constant) cannot prove the behavior it claims, and that can block.
- **Scan the whole for the critical class**: security, privacy, authorization, data loss, compatibility, accessibility, an unusable path.

## What blocks

A finding can block for:

- a failing documented check;
- an unmet accepted behavior or Definition of Done item;
- incorrect behavior this change introduces;
- a material critical-class or reliability risk;
- a mandatory project or language rule (`MUST`, `ALWAYS`, `NEVER`) broken in changed code and absent from the Audit ledger;
- a mandatory finding from a project quality skill;
- material accepted behavior with no credible evidence, or a specific evidence gap as the acceptance criteria define it;
- a false claim in the implementation report.

Not every observation blocks. Ordinary polish belonged to the implementer's Audit, so little of it should remain. A `NOTE` from an earlier round becomes `BLOCK` only on new material evidence or a human's `BLOCK`.

## Repeat reviews stay incremental

A repeat review stays incremental, because an unconstrained repeat search finds new blockers every round and never converges. A pre-existing, noncritical thing you merely noticed is a `NOTE`, not another bounce.
