# Wait for Claimable Work Behavior

## Feature: Bounded Queue Selection

#### Scenario Outline: B1 Immediate selection remains the default
- Given the <lane> queue has <availability> under an explicit repository and non-default remote
- When `skl <lane> next` runs without `--wait`
- Then it performs one existing selection operation without sleeping
- And it returns <outcome> using the selected repository and remote

Examples:
| lane | availability | outcome |
| implement | eligible Ready or Rework item | work_available with its read-back Claim and packet |
| implement | no eligible item | no_work |
| watchdog | eligible Awaiting Review Submission | work_available with its read-back Claim and packet |
| watchdog | no eligible item | no_work |

#### Scenario Outline: B2 Waiting returns immediately for available work
- Given an eligible item in <lane>
- When `skl <lane> next --wait` starts
- Then selection runs immediately without an initial sleep
- And one item is claimed and returned without waiting for the 15-minute idle window

Examples:
| lane |
| implement |
| watchdog |

#### Scenario Outline: B3 Newly claimable work short-circuits waiting
- Given the <lane> queue is initially empty and an item becomes eligible before the next poll
- When `next` runs with <options>
- Then it checks immediately and subsequently uses <interval> between completed empty observations
- And it returns the newly claimed item on that poll without waiting for <maximum>
- And it does not launch an Agent Worker

Examples:
| lane | options | interval | maximum |
| implement | --wait | 30s | 15m |
| watchdog | --wait | 30s | 15m |
| implement | --wait=2m --poll 5s | 5s | 2m |
| watchdog | --wait 2m --poll=5s | 5s | 2m |

#### Scenario Outline: B4 An empty queue reaches its local idle timeout
- Given all <lane> observations find no claimable work
- And work may still be running in the other lane or waiting for human merge
- When `next --wait 45s --poll 30s` runs with controlled time
- Then it checks at entry and after 30 seconds and sleeps only the remaining 15 seconds
- And it starts no new selection at or after the deadline
- And it returns structured `idle_timeout`, not a claim of global completion
- And empty waiting does not create a Claim, Result Document directory, or persistent run record

Examples:
| lane |
| implement |
| watchdog |

#### Scenario Outline: B5 Invalid duration options fail before effects
- Given a valid Consumer Repository and <options>
- When either lane's `next` is invoked
- Then malformed, missing required poll values, zero, negative, overflowing, or unitless durations produce an invocation error before backend reads or mutations
- And the error identifies the offending option

Examples:
| options |
| --wait= |
| --wait 0s |
| --wait -1s |
| --wait nonsense |
| --wait 15 |
| --wait 999999999999999999999h |
| --wait --poll |
| --wait --poll 0s |
| --wait --poll -1s |
| --wait --poll nonsense |

#### Scenario Outline: B6 Cancellation does not become an empty queue
- Given a waiting invocation is cancelled <point>
- When cancellation is observed
- Then it stops with an interruption error and starts no further selection or sleep
- And it never releases a possibly acquired Claim or automatically retries selection
- And diagnostics distinguish interruption from idle timeout and advise explicit inspection/resume if a Claim may have been acquired

Examples:
| point |
| before the first selection |
| during a sleep after an empty observation |
| during a backend operation whose Claim effect is uncertain |

#### Scenario Outline: B7 Idle deadline does not interrupt an in-flight claim
- Given a selection starts before the idle deadline
- And that selection returns <outcome> after the deadline
- When the operation finishes
- Then the caller receives <result>
- And no additional poll is started

Examples:
| outcome | result |
| a durably read-back Claim | work_available and its packet |
| no eligible item | idle_timeout |
| operational error | the operational error |
| deterministic refusal | the refusal |

#### Scenario Outline: B8 Failed observations stop waiting
- Given <failure> occurs <point> in either lane
- When `next --wait` runs
- Then it returns <result> immediately after existing backend retry handling
- And it does not sleep, skip the refused candidate, or select substitute work as a waiting-loop recovery
- And any existing Claim and repair facts are retained

Examples:
| failure | point | result |
| forge unavailable or rate limited | initial observation | nonzero operational error |
| forge unavailable or rate limited | later poll | nonzero operational error |
| otherwise-selected item has invalid workflow evidence | initial observation | existing fix_required outcome |
| Claim write/readback has ambiguous effects | later poll | existing actionable error/refusal |

#### Scenario: B9 Polling reuses canonical eligibility
- Given Ready, Rework, held-Claim, Needs Human, dependency-blocked, and Awaiting Review items with otherwise-valid attachments
- When both lanes' waiting commands encounter these records across successive observations
- Then implementation retains existing Rework-first ordering and Watchdog retains oldest eligible Submission ordering
- And held Claims and ineligible states are skipped without mutation
- And Ready dependents become claimable only after the existing workflow observes their blockers as Merged, not merely Ready for Merge
- And explicit resume and existing handoff, bounce, and merge behavior remain unchanged
