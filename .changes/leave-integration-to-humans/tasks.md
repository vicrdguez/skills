# Tasks - Leave Integration Into main to Humans

## Behavioral
- [ ] B1 Start and resume after main advances without a target pin -> behavior.md B1; intent.md D1
- [ ] B2 Submit a candidate without the target commit -> behavior.md B2; intent.md D2
- [ ] B3 Pass review regardless of mergeability -> behavior.md B3; intent.md D3
- [ ] B4 Retry a pass without conflict rerouting -> behavior.md B4; intent.md D4
- [ ] B5 Observe and reconcile approval without integration rework -> behavior.md B5; intent.md D5
- [ ] B6 Continue existing sync-labeled Rework without reviving synchronization -> behavior.md B6; intent.md D6
- [ ] B7 Publish only to main -> behavior.md B7; intent.md D7
- [ ] B8 Refuse an existing non-main PR without retargeting it -> behavior.md B8; intent.md D8
- [ ] B9 Preserve review and publication validity checks -> behavior.md B9; intent.md D9
- [ ] B10 Render instructions without forced integration -> behavior.md B10; intent.md D10

## Chores
- [ ] C2 Record `[completion] leave-integration-to-humans` while this completed ledger is present, then remove the ledger in a subsequent commit using the active handoff contract.

## Docs
- [ ] C1 Update README.md, docs/capabilities/work-item-lifecycle.md, and docs/adr/0004-extract-workflow-mechanics-without-redesigning-agent-behavior.md for this delivered integration-policy slice only; leave sibling changes planned, document old-worker cutover and stale sync handling, and verify Implement/Watchdog/Audit guidance matches the tested packets -> intent.md D11. Follow skl skill --resource reference/CAPABILITIES-FORMAT.md domain for the capability update.
