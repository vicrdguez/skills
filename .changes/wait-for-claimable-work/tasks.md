# Tasks - Wait for Claimable Work

## Behavioral
- [ ] B1 Immediate selection remains the default -> behavior.md B1
- [ ] B2 Waiting returns immediately for available work -> behavior.md B2
- [ ] B3 Newly claimable work short-circuits waiting -> behavior.md B3
- [ ] B4 An empty queue reaches its local idle timeout -> behavior.md B4
- [ ] B5 Invalid duration options fail before effects -> behavior.md B5
- [ ] B6 Cancellation does not become an empty queue -> behavior.md B6
- [ ] B7 Idle deadline does not interrupt an in-flight claim -> behavior.md B7
- [ ] B8 Failed observations stop waiting -> behavior.md B8
- [ ] B9 Polling reuses canonical eligibility -> behavior.md B9

## Chores
- [ ] C1 Run the existing repository Full Gate and check whitespace; retain existing lifecycle regressions.

## Docs
- [ ] D1 Document wait/poll syntax and local idle semantics in CLI help and README.
- [ ] D2 Update docs/capabilities/work-item-lifecycle.md for delivered waiting only, leaving continuation and harness work planned.
