# Tasks - Validate Marked Artifact Endpoints

## Behavioral

Each task is one public CLI red-green cycle; outline examples stay in its table-driven test. Titles and IDs match `behavior.md` exactly.

- [x] B1 Publish a marked baseline before issue creation
- [x] B2 Resolve markers only in the selected slice history
- [x] B3 Refuse missing or ambiguous required markers
- [x] B4 Enforce endpoint paths modes and exact content
- [x] B5 Enforce phase-appropriate completion ticks
- [x] B6 Require endpoint ancestry and later ledger retirement
- [x] B7 Accept restored intermediate artifact edits
- [x] B8 Inspect and start baseline-only implementation
- [x] B9 Submit only a completed retired contract
- [x] B10 Preserve incomplete work in Needs Human
- [x] B11 Validate Watchdog against the same artifact endpoints
- [x] B12 Validate explicit markerless endpoint SHAs without Adoption state
- [x] B13 Carry explicit endpoints through generated commands
- [x] B14 Keep artifact inspection cost independent of intermediate content

## Chores

- [x] C1 Record `[completion] validate-artifact-endpoints` while this completed ledger is present, then remove the ledger in a subsequent commit using the active handoff contract.

## Docs

- [x] DOC1 Update paired Propose, Implement, Audit artifact instructions, Watchdog guidance, resources, and packet instructions; update the lifecycle capability and explicitly scoped ADR 0003/0004 supersession notes; verify public instruction output and the old-CLI-until-handoff rollout note while leaving slices #2-#5 planned.
