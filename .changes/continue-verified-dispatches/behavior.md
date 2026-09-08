# Continue Verified Dispatches Behavior

## Feature: Dispatch Commands

#### Scenario Outline: B1 Selection supplies commands and startup resumes the Claim
- Given an otherwise-eligible unclaimed Work Item in <lane> with valid real Git history
- And its conventional worktree is <worktree> and <change> occurs between selection and startup
- When the caller invokes `skl <lane> next` with an explicit repository and non-default remote
- Then the CLI claims exactly one item using existing ordering and reads back the Claim
- And `work_available` includes `worker_command` identifying the item and `continuation_command` identifying its durable round, both preserving repository, remote, and lane
- And the startup command uses existing `resume --item --repo --remote` and the continuation command uses `next --after`
- And executing the returned startup command in a fresh invocation from another directory using the existing repository root produces <startup> without selecting another item or repinning its obligation
- And successful startup preserves the original Target Snapshot or fixed reviewed head and bundled skills, with semantic handoff commands bound to the conventional worktree
- And the next packet's Result Directory is reused or, when superseded, safely removed only if marker-only; startup never discards result/repair documents
- And explicit resume of this active Claim does not create another round
- And no Agent Worker is launched by the CLI

Examples:
| lane | worktree | change | startup |
| implement | absent for legitimate first implementation | target branch advances | packet retaining the original Target Snapshot |
| implement | existing | target branch advances | packet retaining the original Target Snapshot |
| watchdog | existing | target branch advances but Submission head stays fixed | packet retaining the original reviewed head |
| watchdog | existing | watched Submission head advances | fix_required retaining the Claim and original reviewed obligation |

## Feature: Authoritative Handoff Verification

#### Scenario Outline: B2 Completed stage handoff authorizes continuation
- Given a <lane> worker executed the returned startup command after `next` and completed <handoff> through the resumed packet's worktree-bound command
- And the normal worker Git preparation created the conventional worktree if initially absent
- And the engine durably observed its stage-appropriate publication, required Git guards, and release of that round's Claim
- When a new CLI invocation executes its continuation command
- Then it verifies that round and performs one ordinary selection in the same lane
- And its successful structured outcome includes `previous_handoff` identifying that verified Work Item and stage outcome, distinct from any newly selected item
- And it returns a new dispatch when work is claimable, or `no_work` for immediate selection when it is not
- And successful handoff cleans its active Result Directory without leaving the discarded next packet's marker-only directory behind
- And in both lanes, a superseded directory containing result/repair documents remains intact rather than being treated as empty cleanup
- And it neither selects the other lane nor performs a merge

Examples:
| lane | handoff |
| implement | Awaiting Review Submission |
| implement | Needs Human decision before code, without a Submission |
| implement | Needs Human preserving code in a draft Submission |
| watchdog | mergeable pass to Ready for Merge, including a pushed post-marker head |
| watchdog | first finding-driven failure to Rework |
| watchdog | conflicting pass to Synchronization Rework |
| watchdog | explicit Needs Human |
| watchdog | second finding-driven failure to Needs Human |

#### Scenario Outline: B3 Worker exit status does not establish completion
- Given the worker reports <report> and the backend has <evidence> for its dispatched round
- When its supervisor invokes the returned continuation command without translating worker prose into workflow facts
- Then the CLI returns <result>
- And worker exit status and prose are not accepted as completion credentials

Examples:
| report | evidence | result |
| success | no completed handoff | fix_required without selecting or waiting |
| error after publication or cleanup | proven completed handoff | ordinary same-lane selection or waiting |
| interruption | incomplete handoff | fix_required without selecting or waiting |

#### Scenario Outline: B4 Unproven or wrong-stage handoff halts
- Given an otherwise-valid dispatched round with <problem> and another claimable item
- When continuation is requested with `--wait`
- Then it returns `fix_required` with the known item and actionable explicit recovery guidance
- And it neither selects nor polls for new work, completes the partial transition, releases Claims, deletes Result Documents, nor launches a replacement

Examples:
| problem |
| Claim still held by this round despite otherwise-valid publication |
| target labels present but publication or Claim release incomplete |
| missing or contradictory round completion evidence |
| missing or ambiguous Work Item or Submission attachment needed to establish proof |
| absent completed evidence after temporary Result Documents have disappeared |
| implementation completion claims ready_for_merge, with otherwise-valid attachments and Claim release |
| Watchdog completion claims awaiting_review, with otherwise-valid attachments and Claim release |

#### Scenario Outline: B5 Earlier completion cannot authorize an unfinished later round
- Given round A of <lane> completed and the same item was subsequently dispatched as round B
- And B has no completed handoff, even though its commit head and eventual target may equal A's
- When B's continuation command runs
- Then A's receipt, labels, comments, and completion flags cannot satisfy B
- And it returns `fix_required`, preserving B and selecting nothing

Examples:
| lane |
| implement |
| watchdog |

#### Scenario Outline: B6 Other-lane advancement does not invalidate historical completion
- Given round A in <lane> has a proven completed handoff
- And <advancement> occurred before that continuation is called
- When A's continuation command runs
- Then its original completion is verified without requiring the item to retain A's final state, head, or an aggregate unclaimed flag
- And ordinary selection in A's lane uses current eligibility without disturbing the later Claim or obligation

Examples:
| lane | advancement |
| implement | Watchdog claimed the Awaiting Review Submission |
| implement | Watchdog handed back Rework and a new implementation round was explicitly claimed |
| watchdog | implementation claimed Rework and advanced its head |
| watchdog | implementation resubmitted and a later Watchdog round was explicitly claimed |
| watchdog | a human merged the accepted Submission |
| implement | a human explicitly requeued the completed Needs Human pause |

## Feature: Waiting And Recovery

#### Scenario Outline: B7 Continuation reuses the waiting contract without replay semantics
- Given a reference to a proven completed round and no claimable work in that lane
- When continuation is called with <options>
- Then it checks immediately after verification and uses <window> with <poll>
- And it returns immediately upon claiming newly eligible work, otherwise `no_work` for immediate selection or `idle_timeout` for waiting exhaustion
- And each newly dispatched round's continuation preserves the effective waiting options and starts a fresh window when invoked, not when the preceding worker started
- And a deliberate later invocation with the same completed reference after a received no-work response performs ordinary current-eligibility selection with a fresh window, not rejection or cached response replay

Examples:
| options | window | poll |
| none | immediate, no sleep | none |
| --wait | 15m | 30s |
| --wait=2m | 2m | 30s |
| --wait 2m --poll 5s | 2m | 5s |
| --wait 2m --poll=5s | 2m | 5s |
| --poll=5s | immediate, no sleep | none |

#### Scenario Outline: B8 Waiting validation, cancellation, and operational failures stop safely
- Given a continuation invocation encounters <condition>
- When the command handles that condition
- Then it produces <outcome> using slice 1 semantics where applicable
- And it preserves any possibly acquired Claim and adds no automatic replacement, call replay, or waiting-loop retry policy
- And when selection may have occurred, available diagnostics require explicit recovery

Examples:
| condition | outcome |
| invalid wait or poll duration | invocation error before mutation |
| cancellation before or during waiting | cancellation without another selection attempt |
| forge failure reading completion evidence | nonzero operational error, not no_work or timeout |
| forge failure on initial selection or its round persistence | nonzero operational error, not no_work or timeout |
| forge failure on a later waiting poll | nonzero operational error, not no_work or timeout |

#### Scenario Outline: B9 Invalid continuation references fail safely
- Given <reference> is supplied to `next --after`
- When it is invoked
- Then malformed input is an invocation error and well-formed but unverifiable identity returns `fix_required`
- And it cannot claim, select substitute work, or mutate the referenced handoff

Examples:
| reference |
| malformed or unsupported opaque reference |
| reference from another repository or lane |
| reference whose item or Submission binding contradicts its durable round facts |
| unknown round or fabricated completion encoded by the caller |

#### Scenario Outline: B10 Uncertain selection requires explicit recovery rather than replay
- Given `next` or `next --after` encounters <interruption>
- When the invocation ends
- Then the CLI neither abandons any acquired Claim nor automatically selects replacement work
- And the command contract and any deliverable diagnostic require the caller to stop on an ambiguous result and inspect/resume by item instead of automatically repeating either selection command
- And loss of item identity may require manual inspection; the CLI promises neither safe replay nor duplicate-dispatch prevention if the caller repeats the call

Examples:
| interruption |
| round persistence or packet construction failed after Claim acquisition |
| successor was claimed but its response was lost |
| idle timeout response was lost |
| selection failed or waiting was cancelled |

## Feature: Concrete Durable Projections

#### Scenario Outline: B11 GitHub reconstructs round evidence across later activity
- Given the concrete GitHub adapter persisted <evidence> for a dispatched round through its public interface
- And a fresh adapter reads paginated history after unrelated metadata and later equal-head rounds were appended
- When that history is used by continuation
- Then the exact round's evidence survives and later unfinished rounds receive no earlier authorization
- And no private filesystem record or retained Result Document is required

Examples:
| evidence |
| implementation completion, including Needs Human without a Submission |
| Watchdog completion |

#### Scenario Outline: B12 GitHub evidence must be observed and trustworthy
- Given otherwise-valid attachments and <condition> affecting dispatch or completion evidence
- When the concrete adapter reads durable history through its existing publication/readback or observation path
- Then <outcome>
- And arbitrary worker summary text or current labels alone cannot substitute for trusted evidence
- And unresolved uncertainty does not trigger call replay, handoff repair, history rewriting, or Claim release

Examples:
| condition | outcome |
| metadata write response lost but exact trusted facts read back | existing adapter reconciliation recognizes the applied write without duplicating it |
| metadata write unapplied or readback unavailable | operation halts without claiming completion; transport failures remain operational |
| malformed, conflicting, untrusted, or incorrectly bound round evidence | continuation refuses unverifiable completion |
