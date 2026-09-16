# Integrate Before Audit Behavior

## Feature: Review the integrated result at a bounded cutoff

These rules govern delivered worker procedures and their evidence obligations. Verification uses the instruction-distribution and resource seams in `plan.md`, not execution or evaluation of an Agent Worker.

### Rule: Observe and merge at the pre-Audit step

Immediately before each submission's Audit, on first implementation and finding-driven Rework, the implementor observes the actual integration target once, merges that observed snapshot into the work branch, and resolves conflicts before normal Audit and independent Watchdog Review. Bind the selected remote and target name wherever already known; `main` is the supported target. The SHA is genuinely acquired by the worker at this step, not supplied by startup or inferred from a stale local ref. Preparation-time integration does not satisfy this late observation. Startup remains metadata-only, with no new engine TargetSnapshot enforcement.

#### Scenario: The target advances after preparation

- Given the selected remote is `upstream`, the supported target is `main`, and preparation left target snapshot A locally available
- When the target advances to B before the first submission's pre-Audit step
- Then the procedure directs the worker to observe the current target through ordinary Git at that step and merge B, not assume A is current
- And it requires conflict resolution before normal Audit without adding a conflict-only review stage
- And the deferred Submission resource directs the worker to record B in the existing Verification section as agent-authored Markdown, not a rendering input or engine metadata field

#### Scenario: Finding-driven Rework integrates new upstream work

- Given a finding-driven Rework round has a previous reviewed head and an unchanged historical artifact contract
- And the late target snapshot adds unrelated upstream code and conflicts with the finding fix
- When the implementor integrates that snapshot and resolves the conflict within the accepted contract before resubmission Audit
- Then Audit and repeat Watchdog guidance cover the integration and conflict-resolution effects alongside the finding resolution
- And they do not treat unrelated upstream additions as scope creep or reopen settled preferences merely because integration made them visible
- And concrete regressions or material risks remain reviewable under existing finding policies

### Rule: Evidence must cover the submitted functional state

Verification records the actual integrated target SHA and applicable checks, results, and material limitations. Functional edits after a recorded Full Gate, including Audit fixes, require affected checks and a Full Gate covering the final functional state. Another target merge requires review of its integration effects and verification of the resulting state; earlier evidence is not evidence for changed code. Conflict resolution alone does not introduce another review stage.

Review references retain separate meanings: the review baseline selects the comparison, Artifact Baseline and Artifact Completion identify the frozen ledger endpoints, the invocation's reviewed head fixes the reviewed Submission, and the integrated SHA identifies the target snapshot actually merged. Integration does not replace any of those review or artifact references. Existing same-head review, previous-head incremental review, unavailable-previous-head fallback, endpoint integrity, and permitted Post-Marker Check behavior remain unchanged.

#### Scenario: Audit leads to a functional edit after the gate

- Given integration and the Full Gate covered state H
- When an Audit disposition changes functional code to state J before submission
- Then the procedure requires checks affected by the edit and a Full Gate covering J before claiming final verification
- And Verification and the Audit ledger describe their actual evidence and reviewed states rather than presenting H's results as proof of J

#### Scenario: A further merge changes the result after Audit

- Given snapshot B was integrated and reviewed
- When the worker subsequently merges target snapshot C before submission
- Then the procedure requires review of the additional integration effects and verification of the resulting state, including the Full Gate covering its final functional state
- And Verification identifies C as the integrated snapshot without replacing the review baseline, artifact endpoints, or a Watchdog invocation's fixed reviewed head

### Rule: Later target movement alone does not restart the round

The successfully integrated late snapshot is the round's cutoff. A later target advance without another merge or functional edit does not require reintegration, invalidate evidence for the unchanged candidate, or restart review. Reuse #37/#46: late mergeability changes do not invalidate a historical Watchdog pass or automatically route work to Rework. Post-approval integration and final merge remain human-owned; approval of earlier inputs does not certify a later conflict-resolution result. No new synchronization receipts, queue effects, or post-approval worker mode follow from this change.

#### Scenario: The target moves beyond the integrated snapshot

- Given B was integrated before Audit and the submitted candidate has not changed
- When the target advances to C, including becoming conflicting after Watchdog passes
- Then the procedures retain B as the recorded integration cutoff without starting another integration or review round solely for that movement
- And the historical verdict remains about its fixed reviewed head while late integration and merge remain with the human Merge Authority

### Rule: Failed integration is unfinished work, not permission to guess

If current-target observation or merging fails, the worker cannot claim successful integration or a completed submission Audit. It reports the limitation, preserves existing commits and ordinary working progress, and uses existing repair/resume or Needs Human policies as applicable. There is no automatic destructive rollback, history rewrite, synthetic success record, or new recovery protocol. A consequential conflict choice unresolved by the contract blocks for a human decision under current policies, rather than being guessed or passed to ordinary review as resolved.

#### Scenario: Observation or merge cannot complete

- Given implementation progress exists and the submission's late integration step is still required
- When the worker cannot observe the selected target or cannot complete its merge
- Then the procedure forbids substituting a stale snapshot or claiming either successful integration or a completed submission Audit
- And it preserves progress and describes the failure for ordinary repair or resume without resetting work or adding synchronization state

#### Scenario: A conflict requires an unapproved consequential choice

- Given integration exposes a conflict whose resolution would choose consequential behavior not settled by the contract
- When the implementor cannot resolve that choice under current policies
- Then the procedure requires the existing Needs Human decision handoff, preserving work and the unresolved limitation
- And it neither guesses a resolution nor claims the submission is ready for normal review
