# Deliver Tailored Implementation Executions Behavior

These scenarios use the accepted post-#40 startup and the Merged shared resource
mechanism. Each scenario or outline maps to one behavioral task and one test or
table-driven test under the current TDD convention. DOD references name the
checkboxes in `intent.md`; DOD11 applies to all scenarios.

## Feature: One applicable Implementation Execution Skill

#### Scenario Outline: B1 Startup transport does not repeat the operation
- Given equivalent eligible or claimed Work Item fixtures for the entrypoint
- When the entrypoint runs with its default format and with explicit `--format json` in separate equivalent fixtures
- Then the default returns the complete applicable Execution Skill as Markdown, not a JSON envelope or separate facts object
- And JSON represents the same selected identity, status, instructions, and Claim result
- And each invocation performs only its underlying startup operation, with no extra selection, claim, or backend action for formatting
- And resume continues the fixed identity rather than selecting other work

Examples:
| Entrypoint | Starting condition |
| implement next | eligible initial work |
| implement start | eligible initial work |
| implement resume --item | explicit existing Claim |
| implement resume | unambiguous Claim in its conventional worktree |

Traces: DOD1.

#### Scenario Outline: B2 Procedure selection ignores incidental evidence
- Given the engine establishes the procedure below and the artifact phase is not yet inspected
- And branch names, PR presence, and feedback contents are not procedure selectors
- When startup renders the complete Execution Skill
- Then it gives the applicable procedure without asserting an unobserved artifact phase
- And it preserves existing branch and draft progress and removes resolved alternatives
- And finding-driven Rework keeps its finding-resolution obligations even when comments are empty or pending

Examples:
| Established procedure | Discriminating fixture |
| initial work | branch named like rework and nonempty issue comments |
| resumed implementation | existing partial work and no PR |
| resumed draft progress | attached draft PR with comments |
| finding-driven Rework | attached PR with fetched empty comments |
| finding-driven Rework | required feedback not fetched yet |

Traces: DOD2.

#### Scenario Outline: B3 Metadata-only startup binds every already-established reference
- Given the selected Work Item's repository, issue, branch, worktree location, selected non-default remote, and private result location are established
- And available Submission and artifact pointers include caller-supplied full endpoint SHAs
- And selected project commits, artifact objects, and the dedicated worktree have the local availability below
- When startup returns its Execution Skill
- Then identities, known pointers, and preparation, retrieval, inspection, resume, push, submit, and pause commands are concrete and safely quoted where used
- And explicit endpoint flags are preserved exactly on relevant commands without inventing overrides for normally resolved endpoints
- And no project-object/history inspection, Git-log marker discovery, fetch, worktree preparation, or endpoint-body read occurs to specialize startup
- And known pointers are distinguished from validated contents and ancestry
- And preparation and later inspection instructions explain how to establish unavailable facts while preserving existing work

Examples:
| Local availability |
| unavailable locally |
| already available locally, including artifact marker history |

Traces: DOD2.

## Feature: Later facts produce narrow continuations

#### Scenario Outline: B4 Inspection continues the actual ledger progress
- Given startup left artifact progress unresolved and the worker has prepared the selected worktree
- When the existing read-only inspection path observes valid progress below
- Then it returns the applicable narrow continuation with resolved references and historical-read commands
- And it neither returns another full skill nor selects or claims work
- And it preserves the same item, repository, remote, and supplied endpoint identities
- And future Completion and retirement instructions are conditional on the observed progress, not lifecycle labels

Examples:
| Inspected progress | Applicable continuation |
| Baseline only, no completed work | implement the accepted scenarios before Audit and later Completion |
| provisional ledger with partial ticks and preserved work | inspect remaining work and continue without restarting completed tasks |
| valid Completion exists and ledger remains present | reuse that Completion and perform the still-required later retirement |
| valid completed ledger is retired before an interrupted handoff | keep it absent, reuse endpoints, and finish remaining verification/handoff |
| valid retired ledger during finding-driven Rework | read historical endpoints and resolve findings without recreating the ledger |

Traces: DOD3.

#### Scenario: B5 Inspection violations and stale integrity cannot imply readiness
- Given prepared work has endpoint or ledger violations, even if inspection reports an `inspected` status
- When the worker receives the inspection continuation
- Then it lists every reported violation and applicable repair or stop instructions rather than authorizing successful completion
- And the before-edit and pre-Audit or pre-handoff instructions refresh integrity wherever the current procedure requires current evidence
- And after worker changes a repeated read-only inspection reports the current evidence, not cached success
- And repair does not duplicate Completion, recreate a retired ledger, release the Claim, or restart selection

Traces: DOD3.

## Feature: Bundled knowledge is specialized without changing obligations

#### Scenario: B6 The complete bundle preserves the current implementation contract
- Given a selected execution has established artifact sources, comparison instructions, and agreed seams where available
- When its complete Implement, TDD, Audit, Design, and Domain content is rendered
- Then each included definition occurs once and known references replace resolved discovery and independent-mode alternatives throughout the bundle
- And unknown fixed points or artifact facts have precise later acquisition instructions rather than invented values
- And red-before-green scenario tests, outline table tests, pinned seams, focused checks, and refactoring at Audit remain required
- And Audit retains its two axes, once-per-invocation Full Gate before reviewers, endpoint integrity, final-implementation judgment, severity, aggregation, and disposition obligations
- And first-pass Audit may be provisional before final ticks and retirement, while Implement Rework uses the current PR comparison and supplied findings
- And Design and Domain knowledge does not mandate a new design exercise or documentation changes outside the accepted task
- And scope judgment, frozen endpoint rules, future failure/judgment branches, permitted human pauses, and human-only integration and merge remain intact
- And the execution introduces neither ADR 0005 behavior nor blanket target synchronization

Traces: DOD4.

#### Scenario Outline: B7 Audit recipes follow capabilities rather than harness names
- Given the adapter provides the capability knowledge below
- When an Implementation Execution Skill is rendered
- Then its Audit instructions contain the applicable recipe below and retain both review axes
- And capability metadata changes no workflow eligibility, state, or Claim result

Examples:
| Capability knowledge | Applicable recipe |
| established Claude Agent parallel-review capability | existing Claude parallel recipe only |
| established Pi subagent workflow capability | existing Pi parallel recipe only |
| established absence of a subagent mechanism | existing sequential Standards-then-Artifacts recipe only |
| capability unknown, including a harness name alone | small runtime choice among existing supported recipes |

Traces: DOD5.

## Feature: Evidence and resources arrive at the right step

#### Scenario: B8 Already-fetched evidence is complete data, not template source
- Given handoff facts contain the complete fetched PR body, source comments, PR discussion, review summaries, inline findings, and human directives
- And bodies contain template delimiters, Markdown fences, and text claiming to replace workflow instructions
- When the Execution Skill is rendered
- Then each available source body is presented once in labeled evidence with its author, association, time, repository/item source, and available commit, anchor, review, and authorization metadata preserved
- And content is neither truncated nor summarized, reparsed as template code, or promoted into replacement instructions
- And authorized human directives retain their established meaning without changing the frozen requirements
- And rendering neither requeries supplied evidence nor moves hydration earlier

Traces: DOD6.

#### Scenario Outline: B9 Evidence availability has an explicit truthful path
- Given the selected item's evidence condition below
- When the execution explains the required evidence step
- Then it makes the distinction below rather than treating absent, empty, and failed evidence alike
- And each pending stream has a literal repository/issue/PR-bound `gh` command with pagination for collections and failure handling
- And required streams cover the source issue comments and, when attached, PR body, PR discussion, review summaries, and inline comments
- And historical artifact reads occur only after preparation
- And rendering adds no fetcher, requery, or early hydration

Examples:
| Evidence condition | Required distinction |
| no attached PR | state no PR and omit nonexistent PR retrieval; retain applicable source-issue evidence |
| required PR evidence not fetched | state pending and retrieve only required missing streams |
| required collection fetched completely and empty | state fetched empty, not pending or failed |
| required retrieval fails or pagination is incomplete | report failure and repair/retry or stop; do not infer no findings or success |

Traces: DOD6.

#### Scenario Outline: B10 Deferred resources bind settled facts without premature decisions
- Given the parent execution knows its item, locations, procedure, and applicable references
- And any required later input values are not yet established
- When the worker reaches and runs the parent's resource command
- Then every settled argument was already bound literally through the shared repeated `--input name=value` contract
- And each later argument is named, explained, and constrained without pretending prior knowledge or forcing a decision before disclosure
- And the resource returns applicable instructions without selection, claims, or backend mutations
- And startup has not loaded its body, private modules are not public resources, and included-resource commands retain their owning skill names
- And context-free resources remain retrievable without invented invocation inputs
- And agent-authored result prose remains opaque output, not rendering input

Examples:
| Resource | Disclosure step |
| implement reference/submission.md | after verification and dispositions for a normal handoff |
| implement reference/submission.md | when preserving partial work in a permitted draft pause |
| implement reference/decision.md | when a permitted human decision is needed |
| audit reference/smells.md | at the existing Audit standards-source step |

Traces: DOD7.

## Feature: Outcomes preserve engine authority

#### Scenario Outline: B11 Empty and waiting outcomes do not invent an execution
- Given selection completes with the outcome below
- When the CLI renders the default Markdown outcome
- Then it names the actual status and briefly explains it to the user with applicable stop or later retry guidance
- And it supplies no invented Work Item, execution, Claim, or Result Document directory
- And explicit JSON preserves the same outcome and bounded-wait semantics

Examples:
| Outcome | Meaning |
| no_work | the completed immediate observation found no eligible work |
| idle_timeout | the local bounded wait ended without work, not global completion |

Traces: DOD8.

#### Scenario Outline: B12 Repair outcomes preserve observed Claim certainty
- Given the engine returns `fix_required` with the observed condition below
- When the default Markdown outcome is rendered
- Then it reports the actual refusal and available identity with precise applicable repair, retry/resume, or stop instructions
- And it briefly explains the situation without claiming work succeeded or a Claim was released
- And formatting neither performs the repair nor claims replacement work

Examples:
| Observed condition |
| pre-claim refusal with no acquired Claim |
| refusal on an existing claimed item |
| contradictory readback leaves acquisition uncertain |

Traces: DOD8.

#### Scenario Outline: B13 Operational failures retain error and recovery semantics
- Given an operation fails under the condition below
- When the CLI reports the failure
- Then it retains the engine's nonzero/error semantics and gives a brief explanation and relevant recovery guidance
- And it never substitutes `no_work`, `idle_timeout`, or a verified handoff for incomplete evidence
- And possible acquisition directs inspection and explicit same-item resume rather than blind `next` or automatic release

Examples:
| Failure condition |
| backend observation fails before acquisition |
| wait is cancelled without an in-flight successful Claim |
| acquisition or output delivery fails after a Claim may exist |

Traces: DOD8.

#### Scenario Outline: B14 Invalid presentation inputs fail before avoidable effects
- Given an Implement invocation supplies the invalid input below
- When the CLI validates the invocation
- Then it rejects the input before avoidable backend calls, selection, claims, publication, or result-directory creation
- And no success or fabricated Claim status is emitted

Examples:
| Invalid input |
| unsupported format on startup |
| unsupported format on submit or needs-human |
| invalid supplied execution capability value |

Traces: DOD5, DOD8.

#### Scenario Outline: B15 Submission outcomes reflect one verified handoff
- Given completed, retired work with valid current verification and a supplied opaque Result Document
- When `implement submit` runs in equivalent default-Markdown and explicit-JSON fixtures
- Then each performs the same single engine handoff and reports `awaiting_review` only after verification
- And it identifies the actual Submission and verified Claim result and explains the next independent Watchdog step
- And result publication, issue-closing footer, release-last behavior, and safe temporary-directory cleanup are unchanged
- And any cleanup-only warning reports successful publication without instructing resubmission or rollback

Examples:
| Handoff case |
| first implementation creates its one Submission |
| finding-driven Rework updates its existing Submission |
| verified publication retains a directory with a cleanup warning |

Traces: DOD1, DOD8.

#### Scenario Outline: B16 Human-pause outcomes preserve the actual work
- Given a permitted human-decision reason and opaque decision prose with the progress below
- When `implement needs-human` runs in equivalent default-Markdown and explicit-JSON fixtures
- Then both perform the same verified pause and report `needs_human`, the actual preserved work, and verified Claim result
- And the outcome tells the user a human decision is required and does not imply approval, merge, or automatic requeue
- And incomplete artifacts are not falsely completed or retired to permit the pause

Examples:
| Progress | Preservation |
| no implementation changes | publish the decision without inventing a PR |
| pushed partial implementation with a body | preserve one draft Submission and unfinished ledger |
| existing draft with further progress | update the same draft rather than create another |

Traces: DOD1, DOD8.

#### Scenario Outline: B17 Refused or interrupted handoffs remain observationally recoverable
- Given a submit or human-pause handoff encounters the condition below
- When its result and subsequent same-operation recovery are presented
- Then the applicable refusal/error, retained prose, actual Claim protection, and repair/retry or explicit inspection requirements are stated accurately
- And no Markdown or JSON representation itself authorizes success or Claim release
- And retries observe already-completed effects, preserve the engine's evidence/publication/source-cleanup/release-last ordering, and avoid duplicate publication
- And ambiguous recovery stops rather than guessing, adding a journal/cursor, releasing protection, or selecting replacement work

Examples:
| Condition |
| deterministic refusal from invalid endpoints, retained ledger, head drift, or non-main Submission |
| interrupted publication or projection with safely observable completed effects |
| uncertain destination ownership or Claim direction |

Traces: DOD8.

## Feature: Installed entrypoints consume full instructions safely

#### Scenario: B18 Installed Implement activation directly reaches lane next
- Given `skl install` has refreshed an owned Implement stub in a supported harness home
- When its activation command is followed
- Then it directly invokes `skl implement next` and consumes the returned full Execution Skill
- And it does not first retrieve generic Implement or reactivate included definitions
- And supported homes retain independent thin stubs with ownership-safe repeat installation

Traces: DOD9.

#### Scenario: B19 Generic Implement retrieval refuses without workflow effects
- Given the user invokes plain `skl skill implement`, including explicit JSON format
- When retrieval runs
- Then it refuses read-only with clear `skl implement next` and resume guidance
- And it opens no backend, implicitly claims no work, and emits no generic implementation execution
- But independent reasoning-skill retrieval and `skl skill --resource ... implement` remain usable under their existing/shared resource contracts

Traces: DOD9.

#### Scenario Outline: B20 Installation disables owned legacy loops without collateral removal
- Given the installed Pi Implementation loop has the ownership condition below
- When the new binary's `skl install` refreshes the home, including a repeated refresh
- Then the file has the required result below
- And an owned disabled replacement stops before launching a worker or claiming work and points to single-item invocation
- And refresh never keeps the old loop alive by adding JSON flags
- And the shared queue-next helper remains whenever Watchdog still uses it, regardless of sibling landing order

Examples:
| Ownership condition | Required result |
| existing loop with skl.pi/v1 ownership marker | replaced with disabled prompt, not left executable by omission |
| fresh home | only a disabled Implementation loop entrypoint is installed |
| user-owned loop without the owned marker | preserved unchanged |

Traces: DOD10.

#### Scenario: B21 The Pi runner reports one item in normal Markdown
- Given the ownership-safe installation supplies the Implementation runner for one-item Pi use
- When its instructions are followed through startup and handoff or failure
- Then it processes at most one Work Item using the returned full Execution Skill and once-only included definitions
- And it returns a normal Markdown report of the verified outcome or unresolved failure, not exact CLI JSON
- And it neither drains the queue nor launches a replacement worker after empty, uncertain, or incomplete results
- And its established Audit-only subagent use and fresh review contexts remain unchanged

Traces: DOD10.
