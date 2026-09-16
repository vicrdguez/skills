# Tasks - Select candidates without reconstructing the queue

## Behavioral

- [x] B1 Rework selection uses PR age and precedes Ready -> behavior.md B1, intent.md D1
- [x] B2 Queue-local age and identity determine selection -> behavior.md B2, intent.md D2
- [x] B3 No work does not trigger unrelated discovery -> behavior.md B3, intent.md D3
- [x] B4 Dependencies precede Ready context enrichment -> behavior.md B4, intent.md D4
- [x] B5 Blocker completion is established only for referenced Dependencies -> behavior.md B5, intent.md D5
- [x] B6 Pagination preserves candidate and Dependency completeness -> behavior.md B6, intent.md D6
- [x] B7 Publication and subsequent commands use the explicit owning link -> behavior.md B7, intent.md D7
- [x] B8 Invalid ownership is a selected-item refusal -> behavior.md B8, intent.md D8
- [x] B9 Claim readback does not rediscover the queue -> behavior.md B9, intent.md D9
- [x] B10 Acquisition drift cannot return an invalid handoff -> behavior.md B10, intent.md D10
- [x] B11 Startup does not require project objects -> behavior.md B11, intent.md D11
- [x] B12 Resume is explicit continuation rather than another selection -> behavior.md B12, intent.md D12
- [x] B13 Only selected feedback is hydrated and none is truncated -> behavior.md B13, intent.md D13
- [x] B14 Required observation failure is not an empty queue -> behavior.md B14, intent.md D14
- [x] B15 Startup composes with safe handoffs and private checkpoints -> behavior.md B15, intent.md D15
- [x] B16 Unrelated history does not scale Work Start cost -> behavior.md B16, intent.md D16

## Chores

- [x] C1 Run focused CLI/actual-adapter checks and the existing full test suite; include the reproducible before/after performance report in the implementation handoff.
- [x] C2 Record `[completion] select-candidates-lazily` while the completed ledger is present, then retire it in a subsequent commit using the active workflow's handoff contract.

## Docs

- [x] DOC1 Update the current Work Start/ownership capability, README, and paired Implement/Watchdog instructions; preserve the independent endpoint-validation and queue-supervisor scopes and remove only superseded startup descriptions.
