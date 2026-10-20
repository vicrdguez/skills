package ledger_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

type recoveryForge struct {
	mergeForgeStub
	onReady func(ledger.PullReadiness) error
	ready   int
}

func (f *recoveryForge) ReadyPull(_ context.Context, p ledger.PullReadiness) error {
	f.ready++
	if f.onReady != nil {
		return f.onReady(p)
	}
	return p.Current()
}

func TestReadinessRecoveryStopsObservableInvalidationBeforeMerge(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"new result", "decision", "submission", "target", "source branch", "proposal spent", "remote moves", "uncertain"} {
		t.Run(scenario, func(t *testing.T) {
			l, source, store := mergeFixture(t)
			forge := &recoveryForge{mergeForgeStub: mergeForgeStub{commit: "squash"}}
			forge.onReady = func(p ledger.PullReadiness) error {
				switch scenario {
				case "new result":
					p := deliveryReportPath(deliveryPublicationProject, deliveryPublicationItem, ledger.WatchdogPhase)
					l.addFile(p, deliveryGitShow(t, l.root, "HEAD", p)+"\nUpdated review evidence.\n")
					l.commitAll("new review result")
				case "decision":
					result, err := ledger.ApplyDecision(store, ledger.DecisionInput{Project: deliveryPublicationProject, Item: deliveryPublicationItem,
						Request: ledger.Reference{Commit: l.head(), Path: deliveryReportPath(deliveryPublicationProject, deliveryPublicationItem, ledger.WatchdogPhase)},
						Answer:  "Integrate again.\n", Route: ledger.RouteWatchdog})
					if err != nil || result.Status != ledger.DecisionApplied {
						t.Fatalf("decision = %+v, %v", result, err)
					}
				case "proposal spent":
					mergeProposal(l, `,"integration_target":{"repository":"acme/widgets","branch":"`+mergeProposalBranch+`"},"completion":{"target":{"repository":"acme/widgets","branch":"main"}}`)
				case "remote moves":
					moved := source.commit(t, "external")
					deliveryGit(t, source.root, "push", "-q", source.remote, moved+":refs/heads/"+deliveryPublicationBranch)
				case "uncertain":
					return ledger.ErrPresentationUncertain
				default:
					state := l.committedState(deliveryPublicationProject, deliveryPublicationSlice, deliveryPublicationBranch)
					switch scenario {
					case "submission":
						state.Submission.Number = 10
					case "target":
						state.Target.Branch = "other"
					case "source branch":
						state.Branch = "other"
					}
					l.commitState(deliveryPublicationProject, deliveryPublicationSlice, deliveryPublicationBranch, state)
				}
				// Real adapters consult this before every further effect. Even if
				// one returns success, the engine must revalidate before merging.
				if p.Current() == nil {
					t.Error("invalidation did not stop the adapter guard")
				}
				return nil
			}
			attempt := ledger.RecoverAndMergeReady(t.Context(), store, deliveryWidgets(), source.root, source.remote, deliveryPublicationItem, forge)
			if attempt.Status != ledger.MergeRefused || len(forge.requests) != 0 || attempt.Readiness == nil {
				t.Fatalf("invalidated recovery = %+v, requests %+v", attempt, forge.requests)
			}
			if scenario == "uncertain" && attempt.Readiness.Status != ledger.IssueUncertain {
				t.Fatalf("uncertainty lost: %+v", attempt)
			}
		})
	}
}

func TestReadinessRecoveryRequiresKnownMatchingRepositoryAndVerifiableRemote(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"no attachment", "foreign repository", "missing source", "unavailable remote"} {
		t.Run(scenario, func(t *testing.T) {
			l, source, store := mergeFixture(t)
			switch scenario {
			case "no attachment", "foreign repository":
				state := l.committedState(deliveryPublicationProject, deliveryPublicationSlice, deliveryPublicationBranch)
				if scenario == "no attachment" {
					state.Submission = nil
				} else {
					state.Submission.Repository = "other/widgets"
				}
				l.commitState(deliveryPublicationProject, deliveryPublicationSlice, deliveryPublicationBranch, state)
			case "missing source":
				deliveryGit(t, source.root, "push", "-q", source.remote, ":refs/heads/"+deliveryPublicationBranch)
			case "unavailable remote":
				deliveryGit(t, source.root, "remote", "set-url", source.remote, t.TempDir()+"/absent.git")
			}
			forge := &recoveryForge{mergeForgeStub: mergeForgeStub{commit: "squash"}}
			attempt := ledger.RecoverAndMergeReady(t.Context(), store, deliveryWidgets(), source.root, source.remote, deliveryPublicationItem, forge)
			if attempt.Status != ledger.MergeRefused || forge.ready != 0 || len(forge.requests) != 0 {
				t.Fatalf("unsafe recovery = %+v", attempt)
			}
			if scenario == "no attachment" {
				if !strings.Contains(attempt.Reason, "explicit PR recovery") {
					t.Fatalf("missing guidance: %+v", attempt)
				}
				pending, err := ledger.PendingMerges(store, deliveryWidgets())
				if err != nil || len(pending) != 1 {
					t.Fatalf("missing attachment silently skipped: %v, %v", pending, err)
				}
			}
		})
	}
}

func TestReadinessRecoveryRefusalLeavesHistoricalPassAndNoDurableAttempt(t *testing.T) {
	t.Parallel()
	l, source, store := mergeFixture(t)
	before := l.head()
	forge := &recoveryForge{onReady: func(ledger.PullReadiness) error { return errors.New("forge unavailable") }}
	result := ledger.RecoverAndMergeReady(t.Context(), store, deliveryWidgets(), source.root, source.remote, deliveryPublicationItem, forge)
	if result.Status != ledger.MergeRefused || l.head() != before || deliverySelect(t, store).Outcome != "pass" {
		t.Fatalf("failed recovery changed local PASS: %+v", result)
	}
}
