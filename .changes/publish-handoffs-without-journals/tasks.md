# Tasks - Publish Safe Handoffs Without Journals

## Behavioral

Each task is one public CLI red-green cycle at the seam pinned in `plan.md`. Scenario Outline rows are variants of that task, not additional tasks.

- [ ] B1 Publish implementation review with release last -> behavior.md B1
- [ ] B2 Publish implementation pauses with release last -> behavior.md B2
- [ ] B3 Publish Watchdog verdicts with release last -> behavior.md B3
- [ ] B4 Keep destinations nonclaimable when source cleanup fails -> behavior.md B4
- [ ] B5 Retry a provable partial handoff forward -> behavior.md B5
- [ ] B6 Recognize verified completed unclaimed handoffs -> behavior.md B6
- [ ] B7 Refuse changed input during handoff recovery -> behavior.md B7
- [ ] B8 Preserve a new Claim after an uncertain release -> behavior.md B8
- [ ] B9 Refuse stale commands at the same SHA -> behavior.md B9
- [ ] B10 Report cleanup warnings after verified publication -> behavior.md B10
- [ ] B11 Stop on unknown or contradictory recovery state -> behavior.md B11
- [ ] B12 Recover without transition journals or timeline direction -> behavior.md B12
- [ ] B13 Keep envelope-shaped decisions opaque -> behavior.md B13
- [ ] B14 Resume only the selected Work Item from current evidence -> behavior.md B14

## Chores

- [ ] C1 Record `[completion] publish-handoffs-without-journals` while this completed ledger is present, then remove the ledger in a subsequent commit using the active handoff contract.

## Docs

- [ ] D1 Update docs/capabilities/work-item-lifecycle.md, docs/adr/0002-use-a-backend-neutral-state-model-without-a-cli-database.md, README.md, and affected Implement/Watchdog skill guidance to describe release-last publication, same-item evidence-based resume, bounded no-journal retry, and cleanup warnings, without claiming unmerged slices 1/2/3/5 or queue-supervisor behavior is delivered.
