# Tasks - Validate Marked Artifact Endpoints

## Behavioral

Each task is one public CLI red-green cycle; outline examples stay in its table-driven test. Titles and IDs match `behavior.md` exactly.

- [ ] B1 Publish a marked baseline before issue creation
- [ ] B2 Resolve markers only in the selected slice history
- [ ] B3 Refuse missing or ambiguous required markers
- [ ] B4 Enforce endpoint paths modes and exact content
- [ ] B5 Enforce phase-appropriate completion ticks
- [ ] B6 Require endpoint ancestry and later ledger retirement
- [ ] B7 Accept restored intermediate artifact edits
- [ ] B8 Inspect and start baseline-only implementation
- [ ] B9 Submit only a completed retired contract
- [ ] B10 Preserve incomplete work in Needs Human
- [ ] B11 Validate Watchdog against the same artifact endpoints
- [ ] B12 Validate explicit markerless endpoint SHAs without Adoption state
- [ ] B13 Carry explicit endpoints through generated commands
- [ ] B14 Keep artifact inspection cost independent of intermediate content

## Chores

- [ ] C1 Record `[completion] validate-artifact-endpoints` while this completed ledger is present, then remove the ledger in a subsequent commit using the active handoff contract.

## Docs

- [ ] DOC1 Update paired Propose, Implement, Audit artifact instructions, Watchdog guidance, resources, and packet instructions; update the lifecycle capability and explicitly scoped ADR 0003/0004 supersession notes; verify public instruction output and the old-CLI-until-handoff rollout note while leaving slices #2-#5 planned.
