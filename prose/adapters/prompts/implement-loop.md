---
description: Disabled legacy Implementation loop
---

<!-- skl-owned: skl.pi/v1 -->

This queue-draining Implementation loop is disabled. It launches no worker, claims no Work Item, and performs no workflow operation.

Work on one Work Item at a time instead:

- run `skl implement next` and follow the complete Execution Skill it returns for the single Work Item it selects; or
- run `skl implement resume --item <proposal>/<slice> --claim <acquisition-commit>` to continue a specific Work Item's Claim.

The `implement-runner` agent stays available for that one-item use. `skl install` owns this file, so a local edit is replaced on the next refresh.
