# Tasks - Wait for Claimable Work

## Behavioral
- [x] B1 Immediate selection remains the default -> behavior.md B1
- [x] B2 Waiting returns immediately for available work -> behavior.md B2
- [x] B3 Newly claimable work short-circuits waiting -> behavior.md B3
- [x] B4 An empty queue reaches its local idle timeout -> behavior.md B4
- [x] B5 Invalid duration options fail before effects -> behavior.md B5
- [x] B6 Cancellation does not become an empty queue -> behavior.md B6
- [x] B7 Idle deadline does not interrupt an in-flight claim -> behavior.md B7
- [x] B8 Failed observations stop waiting -> behavior.md B8
- [x] B9 Polling reuses canonical eligibility -> behavior.md B9

## Chores
- [x] C1 Run the existing repository Full Gate and check whitespace; retain existing lifecycle regressions.

## Docs
- [x] D1 Document wait/poll syntax and local idle semantics in CLI help and README.
- [x] D2 Update docs/capabilities/work-item-lifecycle.md for delivered waiting only, leaving continuation and harness work planned.
