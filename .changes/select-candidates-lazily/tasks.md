# Tasks - Select candidates without reconstructing the queue

## Behavioral

- [ ] B1 Rework selection uses PR age and precedes Ready -> behavior.md B1, intent.md D1
- [ ] B2 Queue-local age and identity determine selection -> behavior.md B2, intent.md D2
- [ ] B3 No work does not trigger unrelated discovery -> behavior.md B3, intent.md D3
- [ ] B4 Dependencies precede Ready context enrichment -> behavior.md B4, intent.md D4
- [ ] B5 Blocker completion is established only for referenced Dependencies -> behavior.md B5, intent.md D5
- [ ] B6 Pagination preserves candidate and Dependency completeness -> behavior.md B6, intent.md D6
- [ ] B7 Publication and subsequent commands use the explicit owning link -> behavior.md B7, intent.md D7
- [ ] B8 Invalid ownership is a selected-item refusal -> behavior.md B8, intent.md D8
- [ ] B9 Claim readback does not rediscover the queue -> behavior.md B9, intent.md D9
- [ ] B10 Acquisition drift cannot return an invalid handoff -> behavior.md B10, intent.md D10
- [ ] B11 Startup does not require project objects -> behavior.md B11, intent.md D11
- [ ] B12 Resume is explicit continuation rather than another selection -> behavior.md B12, intent.md D12
- [ ] B13 Only selected feedback is hydrated and none is truncated -> behavior.md B13, intent.md D13
- [ ] B14 Required observation failure is not an empty queue -> behavior.md B14, intent.md D14
- [ ] B15 Startup composes with safe handoffs and private checkpoints -> behavior.md B15, intent.md D15
- [ ] B16 Unrelated history does not scale Work Start cost -> behavior.md B16, intent.md D16

## Chores

- [ ] C1 Run focused CLI/actual-adapter checks and the existing full test suite; include the reproducible before/after performance report in the implementation handoff.
- [ ] C2 Record `[completion] select-candidates-lazily` while the completed ledger is present, then retire it in a subsequent commit using the active workflow's handoff contract.

## Docs

- [ ] DOC1 Update the current Work Start/ownership capability, README, and paired Implement/Watchdog instructions; preserve the independent endpoint-validation and queue-supervisor scopes and remove only superseded startup descriptions.
