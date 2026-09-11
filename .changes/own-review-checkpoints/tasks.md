# Tasks - Make Review Checkpoints CLI-Owned

## Behavioral

- [ ] B1: Select checkpoint storage by Work Item worktree -> behavior.md B1; intent D1. Prove private Git-directory placement, caller independence, isolated operational counts, and worker-facing encapsulation through CLI execution.
- [ ] B2: Derive review scope without discarding a valid count -> behavior.md B2; intent D2. Table-drive missing/zero/usable/unavailable/nonancestor checkpoints and render concrete scope/count/number facts.
- [ ] B3: Resume interrupted review using fresh invocation facts -> behavior.md B3; intent D2. Refresh actual PR head without consuming a round or relying on stale hidden metadata.
- [ ] B4: Refuse invalid checkpoint data and submit inputs -> behavior.md B4; intent D3. Validate counts/full SHAs and surface read/resolution failures without silent reset or publication.
- [ ] B5: Count completed verdicts and cap only automatic failure-driven Rework -> behavior.md B5; intent D4. Exercise the verdict/count table through real adapter publication.
- [ ] B6: Distinguish a new same-SHA review from retrying the previous round -> behavior.md B6; intent D4. Prove explicit human requeue consumes the next round while exact retries do not.
- [ ] B7: Complete evidence before recording the round -> behavior.md B7; intent D5. Inject actual GitHub adapter HTTP faults and verify exact receipt reuse before checkpointing.
- [ ] B8: Replace the single checkpoint atomically -> behavior.md B8; intent D5. Exercise real storage failures and fixed-number repair retries with no partial authoritative record.
- [ ] B9: Retry after checkpoint replacement using the original intended round -> behavior.md B9; intent D5. Restart with retained command/result context only where source-stage protection or completed-unclaimed state is provable; ambiguous claimed states belong to B10.
- [ ] B10: Stop when a retained command cannot prove the intended completion -> behavior.md B10; intent D5. Refuse mismatched/ambiguous rounds and preserve later-stage Claims without inventing recovery storage.
- [ ] B11: Preserve actual reviewed head separately from final Debt Marker head -> behavior.md B11; intent D6. Keep pushed-head/ancestry guards and worker-owned Post-Marker Check guidance.
- [ ] B12: Continue implementation and Audit without previous-review extraction -> behavior.md B12; intent D7. Remove only the previous-review cache obligation from mechanics and rendered worker guidance.
- [ ] B13: Retain completed history during nonterminal work -> behavior.md B13; intent D8. Prove pauses, finding-driven Rework, ledger retirement, and working-tree clean retain review scope/count.
- [ ] B14: Delete only after verified done and report cleanup warnings -> behavior.md B14; intent D8. Preserve successful outcomes and avoid republishing when local cleanup fails.
- [ ] B15: Keep count policy independent of other completion callers and policies -> behavior.md B15; intent D9. Cover status callers and conflict diversion; remove timeline counting without removing partial-handoff reconstruction.
- [ ] B16: Accept checkpoint loss when a dedicated worktree is recreated -> behavior.md B16; intent D8. Demonstrate count-zero/full-scope reset and first-failure Rework without history reconstruction.

## Chores

- [ ] C1: Run `go test ./...` and `git diff --check`; check structured/rendered packet agreement, all D-to-B mappings, and replacement of stale review-pin/bounce assertions. Preserve opaque-ID/provider presentation and all out-of-scope behavior.
- [ ] C2: Record `[completion] own-review-checkpoints` while this completed ledger is present, then remove the ledger in a subsequent commit using the active handoff contract.

## Docs

- [ ] D1: Update `docs/capabilities/work-item-lifecycle.md` to describe this slice's CLI-owned Review Checkpoint, completed-review limit, scope fallback, fixed-number retry contract, cleanup warnings, and accepted worktree-loss reset as implemented. Leave target policy, artifact markers, general handoff/journal removal, and object-free startup under their own planned corrections.
