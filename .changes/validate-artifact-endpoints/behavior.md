# Validate Marked Artifact Endpoints Behavior

## Feature: Slice-scoped artifact evidence through the public CLI

### Background

- Given a real Git Consumer Repository and the existing controlled Backend adapter
- And all unrelated Workflow preconditions are satisfied, including current target, Claim, pushed-head, and Result Document requirements where applicable
- And the slice slug is `ship-widget`, chosen before its Work Item exists
- And marker scope, phase rules, and explicit-input rules are those pinned in `plan.md`
- And each scenario is exercised through `newApp(...).Run`, not a private ledger helper

#### Scenario Outline: B1 Publish a marked baseline before issue creation

- Given the prepared slice has <baseline evidence> and its local and remote heads agree
- When `skl propose publish` preflights and publishes that declared slice
- Then publication has <result>
- And no issue number is needed in the baseline subject
- And a refusal makes no publication mutations, while success preserves the opaque body and applies Ready only after durable publication

Examples:

| baseline evidence | result |
| unique `[baseline] ship-widget` at the head with the complete ledger | completed publication using that baseline SHA |
| complete ledger at the head without a baseline marker | actionable refusal for the missing marker |
| unique baseline marker before the publication head | actionable refusal that baseline must be the published head |
| baseline head missing `behavior.md` | actionable refusal for the incomplete baseline |

Trace: D1.

#### Scenario Outline: B2 Resolve markers only in the selected slice history

- Given the selected branch has one valid baseline and completion pair and <other evidence>
- When `skl implement inspect --item 7` resolves its artifact snapshots
- Then it reports exactly the selected pair without violations
- And it performs no additional issue/PR enumeration to establish marker ownership

Examples:

| other evidence |
| subjects for `ship-widget-extra` and a different slice in reachable history |
| matching subjects on a branch not reachable from the inspected head |
| marker text only in a commit body, after another subject prefix, or with different case |
| the unique selected markers on reachable merge-parent history rather than the first-parent chain |
| selected subjects with a space and explanatory text after the exact slug token |

Trace: D2.

#### Scenario Outline: B3 Refuse missing or ambiguous required markers

- Given the selected history has <marker problem>
- When an invocation requiring the missing or ambiguous endpoint inspects the artifacts
- Then its public output identifies the slice and missing marker or competing marker SHAs
- And startup or handoff refuses before mutation rather than choosing the first, latest, or nearest pair
- And inspection returns the evidence as violations without claiming a valid completed contract

Examples:

| marker problem |
| no baseline marker and no explicit baseline input |
| no completion marker at a review head and no explicit completion input |
| two baseline markers reachable from the selected head |
| two completion markers, including one through a merge parent |

Trace: D2.

#### Scenario Outline: B4 Enforce endpoint paths modes and exact content

- Given an accepted ledger is expected to contain `intent.md`, `behavior.md`, and its optional files
- And the completion has <endpoint difference>
- When `skl implement inspect --item 7` validates the endpoint pair
- Then it reports <integrity result> with affected endpoint/path evidence for a refusal

Examples:

| endpoint difference | integrity result |
| no changes other than permitted completion ticks | no integrity violation |
| an added, missing, renamed, or nested extra path | a path-set violation |
| an executable blob, symlink, or gitlink at either endpoint | a mode/object-type violation |
| changed prose, whitespace, line order, line endings, or final newline | a content violation |
| the ledger path itself is a blob or symlink instead of a directory | an invalid-ledger-shape violation |
| missing `intent.md` or `behavior.md` in both endpoints | a missing-required-artifact violation |

Trace: D3.

#### Scenario Outline: B5 Enforce phase-appropriate completion ticks

- Given the endpoint checkboxes have <checkbox state>
- When the CLI validates <phase>
- Then it reports <result>
- And checkbox recognition and Manual Verification heading scope retain the existing ledger syntax

Examples:

| checkbox state | phase | result |
| unchanged text with existing automated unchecked boxes changed only to lowercase x | completed endpoint pair | valid completion |
| an automated box still unchecked | completed endpoint pair | incomplete-completion violation |
| a Manual Verification box checked at either endpoint | any phase | manual-checkbox violation |
| a checked baseline box unchecked at completion | completed endpoint pair | forbidden reverse-tick violation |
| an unchecked box changed to uppercase X | completed endpoint pair | forbidden content-change violation |
| automated boxes still unchecked and no Completion | baseline-only implementation | valid incomplete phase, not review readiness |

Trace: D3.

#### Scenario Outline: B6 Require endpoint ancestry and later ledger retirement

- Given available endpoint commits and <relationship>
- When the CLI validates the fixed head for review
- Then it reports <result>
- And it does not infer Completion from the first parent of a deletion commit

Examples:

| relationship | result |
| Baseline precedes Completion, later deletion precedes the review head, and the ledger is absent there | valid retired contract |
| Baseline equals explicit Completion with already-complete artifacts, followed by later deletion | valid retired contract |
| Baseline is not an ancestor of Completion | ancestry violation |
| an explicitly supplied endpoint is not an ancestor of the inspected head | reachability violation |
| the completion marker is on the deletion commit and its tree lacks artifacts | missing-completion-artifacts violation |
| the review head still contains any part of the ledger | retirement violation |
| Completion contains the ledger and later unrelated commits precede its deletion | valid retired contract without an immediate-child requirement |

Trace: D4.

#### Scenario Outline: B7 Accept restored intermediate artifact edits

- Given valid uniquely marked endpoint snapshots, valid ancestry, and ledger absence at the review head
- And intermediate commits <temporary change> without changing either accepted endpoint or leaving artifacts at the review head
- When `skl implement inspect --item 7` validates the retired contract
- Then it reports a valid contract with the marked endpoints
- And it does not report an intermediate-content, transition-count, or first-parent merge-tree violation

Examples:

| temporary change |
| edit and restore frozen prose |
| add, remove, or change modes of paths and restore the endpoint path set and modes |
| tick and untick boxes before reaching valid final completion ticks |
| delete and restore the ledger before Completion |
| change the ledger through a merge and restore it before Completion |
| temporarily restore the ledger after retirement and remove it again before the review head |

Trace: D4.

#### Scenario Outline: B8 Inspect and start baseline-only implementation

- Given a valid unique baseline, no completion marker, and <progress>
- When `skl implement inspect --item 7` and the applicable existing implementation startup invocation run
- Then inspection reports Baseline, no Completion, and the present phase without requiring final ticks or retirement
- And startup supplies the same baseline and usable implementation/Audit instructions without discarding progress
- And startup retains its current Git preparation prerequisites until slice #5 lands

Examples:

| progress |
| the published baseline head awaiting its first Claim |
| a claimed descendant head with partial automated ticks being resumed |
| a claimed head containing a restored provisional contract after an earlier temporary edit |

Trace: D5.

#### Scenario Outline: B9 Submit only a completed retired contract

- Given a claimed <implementation kind> and <artifact phase> at the pushed fixed head
- When `skl implement submit --item 7 --body <absolute-file>` runs
- Then it has <result>
- And successful resubmission keeps the attached Submission rather than creating another ledger or Submission
- And a deterministic refusal retains the Claim and Result Documents

Examples:

| implementation kind | artifact phase | result |
| first-pass implementation | valid marked Completion followed by retirement | Awaiting Review |
| first-pass implementation | baseline only or incomplete Completion | actionable refusal |
| first-pass implementation | complete artifacts still present at the head | actionable refusal |
| finding-driven Rework | original valid endpoint pair and ledger still absent | Awaiting Review on the existing Submission |

Trace: D5.

#### Scenario Outline: B10 Preserve incomplete work in Needs Human

- Given a claimed implementation with a valid baseline and <preserved work>
- And automated tasks remain incomplete and no finished Artifact Completion is available
- When `skl implement needs-human --item 7` receives a permitted reason, decision, and <body input>
- Then the CLI publishes Needs Human with the original resume state and <preservation result>
- And it does not require ticking unfinished tasks, inventing Completion, or deleting the ledger
- And supplied endpoint identities remain subject to unambiguous resolution and ancestry checks

Examples:

| preserved work | body input | preservation result |
| no implementation changes outside the ledger and no Submission | no body | decision on the Work Item without a new Submission |
| pushed partial implementation and unfinished artifacts | private Submission body | one draft Submission at the fixed head |
| pushed partial implementation with an existing draft Submission | private Submission body | the same attached draft Submission updated |

Trace: D5.

#### Scenario Outline: B11 Validate Watchdog against the same artifact endpoints

- Given an Awaiting Review Submission with <review evidence>
- When its applicable `skl watchdog next`, `resume`, or `submit` invocation validates the fixed review/final head
- Then it has <result>
- And valid startup packets still expose historical Baseline and Completion contents until slice #5 defers those reads
- And instructions require independent endpoint verification, not a first-parent deletion search or an intermediate-content audit
- And verdict publication otherwise keeps the existing review policy and human merge authority

Examples:

| review evidence | result |
| valid marked retired endpoints and a first review head | startup and verdict validation accept the contract |
| valid original endpoints after finding-driven Rework | validation accepts the unchanged retired contract |
| a pushed Debt Marker descendant of the fixed reviewed head with artifacts absent at both heads | pass validation accepts the contract at both heads |
| a final head reintroducing a ledger path | actionable retirement refusal with Claim and prose retained |
| invalid completion content or ambiguous markers | actionable artifact refusal before Claim or verdict mutation |

Trace: D6.

#### Scenario Outline: B12 Validate explicit markerless endpoint SHAs without Adoption state

- Given existing work has <endpoint evidence> and the operator supplies <explicit inputs>
- When a relevant inspection, startup, or handoff invocation runs
- Then it has <result>
- And accepted inputs use the same shape, tick, phase, and ancestry checks as marked endpoints
- And the command neither rewrites commits nor stores an Adoption record nor discovers fallback endpoints

Examples:

| endpoint evidence | explicit inputs | result |
| no markers and a valid baseline-only implementation | full `--artifact-baseline` SHA | valid incomplete phase |
| no markers and a valid retired contract | both full endpoint SHAs | valid retired contract |
| missing baseline marker and a unique valid completion marker | full baseline SHA only | valid pair using the supplied baseline and marked completion |
| markerless review evidence | baseline SHA only | refusal for missing Completion |
| markerless evidence | branch name, abbreviated SHA, non-commit object, or unavailable SHA | actionable invalid-SHA refusal |
| markerless evidence | available but invalid content or ancestry | the corresponding endpoint violation |
| multiple relevant baseline or completion markers | either or both explicit SHAs | ambiguity refusal, never an override of competing markers |
| a unique marker for the endpoint being overridden | explicit SHA for that endpoint | refusal to override marked evidence |
| markerless evidence | no explicit inputs | missing-marker refusal, including ordinary `next` |

Trace: D7.

#### Scenario Outline: B13 Carry explicit endpoints through generated commands

- Given an invocation accepted <explicit evidence> for its selected Work Item
- When the public CLI renders its packet or actionable continuation instructions
- Then every applicable generated resume, inspect, and submit command carries the supplied endpoint flags and exact SHAs
- And Needs Human and independent Audit guidance tells the worker to retain those same explicit inputs
- And ordinary marked work does not receive synthetic override flags merely because its resolved SHAs are known
- And flags apply only to this invocation's selected Work Item, never to other queue entries or backend records

Examples:

| explicit evidence |
| a baseline-only implementation start or resume |
| a markerless retired pair used by implementation Rework |
| a markerless retired pair used by Watchdog startup or resume |

Trace: D7.

#### Scenario: B14 Keep artifact inspection cost independent of intermediate content

- Given short and long representative real Git histories with identical artifact endpoints and phase
- And the long history includes thousands of unrelated commits plus temporary artifact edits restored at Completion
- When the public inspection CLI is exercised with Git's existing tracing and a repeatable benchmark
- Then both histories produce the same artifact result
- And traced tree/blob reads are confined to the baseline, completion or provisional head, and the fixed head-presence check
- And Git subprocess launches and artifact content loads do not grow per history commit
- And a metadata-only marker scan may grow with reachable history
- And benchmark output records history size and CLI measurements without a wall-clock pass/fail threshold

Trace: D8.
