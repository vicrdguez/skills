# Delegate Claimed Work Behavior

These rules govern delivered instructions for optional testing and implementation delegation, including the single-item runner and relevant continuations. Scenarios discriminate required guidance; they do not prescribe one test per scenario or assert that a model followed it.

## Rule: Capability Controls Optional Delegation

Use execution capabilities established by the existing Harness Adapter to select supported instruction recipes. A harness name alone establishes nothing. Known capabilities need no rediscovery; only genuinely unknown capability retains a small, bounded runtime discovery choice. Delegation remains optional even when supported. If unavailable or not established, the owner performs the subwork serially. This fallback applies only to optional implementation/testing delegation, not to the separate mandatory Audit mechanisms.

### Scenario: Supported Delegation Is Offered Without Requiring It

- Given the adapter establishes a supported helper mechanism for the invocation
- When the complete Implement Execution Skill and applicable continuation are rendered
- Then they permit the owner to delegate non-conflicting testing or implementation subwork, including parallel work where supported
- And they use the established recipe without asking the worker to infer support from its harness name
- And they permit serial owner execution without requiring a separate test writer or a fixed helper count.

### Scenario: Unsupported Delegation Does Not Downgrade Audit

- Given optional implementation/testing delegation is known to be unavailable, regardless of harness name
- When implementation instructions are rendered
- Then they direct the owner to perform that subwork serially without an unavailable helper-launch recipe
- And they retain the independently applicable Audit procedure and its capability requirements rather than treating this fallback as permission to skip or weaken them.

### Scenario: Unknown Capability Is Resolved Only When Needed

- Given the adapter has not established whether a supported delegation mechanism is available
- When implementation instructions are rendered
- Then they retain bounded runtime capability discovery before optional helper use
- And they permit only a recipe whose support is established, otherwise serial owner execution
- And they do not require new tools, a registry, or a scheduler to resolve the uncertainty.

## Rule: Delegation Preserves The Claim And Contract

The existing Claim remains with one accountable implementor. Helpers receive a bounded assignment, the selected Work Item and working-location context, authoritative contract references or contents, relevant architectural commitments and standards, required observations, and write responsibilities. Cross-context inputs distinguish these obligations from implementation-detail preferences; helpers may choose details only within the accepted contract. Consequential unresolved decisions return through the owner to the existing human-decision path, not a helper-authored requirement.

The owner prevents conflicting concurrent writers, including conflicting Git operations, by dividing responsibilities or serializing work. Separate helper worktrees are not mandatory. Helpers return their work, evidence, and limitations to the owner; they do not select queue items, acquire another Claim, change Workflow State, or make the final Submission. The runner still processes at most its one selected Work Item, and disabled loops remain disabled.

### Scenario: A Fresh Helper Receives An Assignment, Not Workflow Authority

- Given a claimed Work Item with an agreed architectural boundary and required observable behavior, but no prescribed internal test organization
- When the delivered guidance describes assigning testing subwork to a fresh helper context
- Then it requires supplying the bounded assignment and contractual inputs rather than relying on the owner's conversation history
- And it preserves the architecture and observations without elevating the owner's preferred test organization into an acceptance criterion
- And it requires non-conflicting write responsibilities, serializing overlapping work rather than mandating a worktree per helper
- And it directs results back to the same owner without additional Claims, independent queue selection, or delegated submission.

## Rule: The Owner Verifies The Integrated Result

The owner inspects and integrates helper contributions and remains responsible for the complete final functional state. Helper reports and isolated passing checks are evidence, not a substitute for integrated verification, the normal Audit and Full Gate, or independent fresh-context Watchdog Review. Functional changes after earlier checks require affected checks and a Full Gate covering the final functional state under the existing verification policy. Missing or incomplete helper work is not silently counted as complete; the owner finishes it or uses the existing human-decision path for a material unresolved blocker. Artifact integrity and human-only merging remain unchanged.

### Scenario: Passing Helper Checks Do Not Establish Final Readiness

- Given helpers report passing checks for separate contributions that still need integration
- When the delivered instructions describe completing the claimed Work Item
- Then they require the owner to inspect and integrate the contributions and verify the resulting functional state
- And they require normal Audit, a Full Gate covering that state, and owner-controlled submission for independent Watchdog Review
- And they prohibit claiming completion from helper reports alone or treating a helper as the final publisher or merge authority.
