package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	skilldist "github.com/vicrdguez/skills"
	"net/http"
	"net/http/httptest"

	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/setup"
	"github.com/vicrdguez/skills/workflow"
)

type implementationMemory struct {
	memoryBackend
	coordination           []workflow.CoordinationItem
	work                   []workflow.ImplementationItem
	remoteHeads            map[string]string
	afterPublish           func()
	decisions              map[workflow.WorkItemID]string
	failTransition         bool
	beforeTransition       func()
	afterCompletion        func()
	beforeReviewSubmission func(int)
	reviewSubmissionCalls  int
	reviewClock            int
}

func (b *implementationMemory) reviewTime() string {
	b.reviewClock++
	return fmt.Sprintf("2026-01-01T00:00:%02dZ", b.reviewClock)
}

// Expand concise initial fixtures into separate records. Once a mutation is made,
// the stored observations, not the derived Work Item State, remain authoritative.
func implementationFixture(item workflow.ImplementationItem) workflow.ImplementationItem {
	if item.Source != nil {
		return item
	}
	item.Source = &workflow.LifecycleObservation{Open: true, Claimed: item.Claimed}
	if item.State != "" {
		item.Source.States = []workflow.State{item.State}
	}
	if item.Submission != nil {
		submission := *item.Submission
		if submission.Lifecycle == nil {
			state := submission.State
			claimed := submission.Claimed
			if item.State != workflow.Ready {
				item.Source.States, item.Source.Claimed = nil, false
				if state == "" {
					state = item.State
				}
				claimed = claimed || item.Claimed
			}
			submission.Lifecycle = &workflow.LifecycleObservation{Open: true, Claimed: claimed}
			if state != "" {
				submission.Lifecycle.States = []workflow.State{state}
			}
			submission.State, submission.Claimed = state, claimed
		}
		item.Submission = &submission
	}
	return item
}

func (b *implementationMemory) PauseImplementation(_ context.Context, item workflow.ImplementationItem, decision string, guard func() error) error {
	if b.beforeTransition != nil {
		b.beforeTransition()
	}
	if err := guard(); err != nil {
		return err
	}
	if b.decisions == nil {
		b.decisions = make(map[workflow.WorkItemID]string)
	}
	b.decisions[item.ID] = decision
	for i := range b.work {
		if b.work[i].ID == item.ID {
			current := implementationFixture(b.work[i])
			comments := &current.Feedback
			claimAcquiredAt := current.SourceClaimAcquiredAt
			if current.Submission != nil {
				comments = &current.Submission.Comments
				if item.State == workflow.Rework {
					claimAcquiredAt = current.Submission.ClaimAcquiredAt
				}
			}
			claim, claimErr := time.Parse(time.RFC3339Nano, claimAcquiredAt)
			body := workflow.OpaqueImplementationDecision(decision)
			published := slices.ContainsFunc(*comments, func(comment skilldist.ReviewComment) bool {
				created, err := time.Parse(time.RFC3339Nano, comment.CreatedAt)
				return comment.EvidenceAuthorized && comment.Path == "" && comment.Body == body && claimErr == nil && err == nil && created.After(claim)
			})
			if !published {
				*comments = append(*comments, skilldist.ReviewComment{Body: body, CreatedAt: b.reviewTime(), EvidenceAuthorized: true})
			}
			projection := current.Source
			if current.Submission != nil {
				projection = current.Submission.Lifecycle
			}
			projection.Claimed = true
			if !slices.Contains(projection.States, workflow.NeedsHuman) {
				projection.States = append(projection.States, workflow.NeedsHuman)
			}
			if b.failTransition {
				b.failTransition = false
				b.work[i] = workflow.ReconcileImplementation(current)
				return errors.New("interrupted projection")
			}
			projection.States = slices.DeleteFunc(projection.States, func(state workflow.State) bool { return state == workflow.Rework || state == workflow.AwaitingReview })
			projection.Claimed = false
			current.Source.States = []workflow.State{workflow.NeedsHuman}
			current.Source.Claimed = false
			current.Problem = ""
			b.work[i] = workflow.ReconcileImplementation(current)
		}
	}
	return nil
}

func (b *implementationMemory) ImplementationHead(_ context.Context, branch string) (string, error) {
	return b.remoteHeads[branch], nil
}

func (b *implementationMemory) PublishImplementation(_ context.Context, item workflow.ImplementationItem, submission workflow.Submission) (workflow.Submission, error) {
	if number, err := strconv.Atoi(string(item.ID)); err == nil {
		footer := fmt.Sprintf("\n\nCloses #%d\n", number)
		if !strings.HasSuffix(submission.Body, footer) {
			submission.Body += footer
		}
	}
	if submission.ID == "" {
		submission.ID = "11"
	}
	for i := range b.work {
		if b.work[i].ID == item.ID {
			current := implementationFixture(b.work[i])
			submission.Lifecycle = &workflow.LifecycleObservation{Open: true}
			if current.Submission != nil {
				submission.Lifecycle = current.Submission.Lifecycle
				submission.Comments = current.Submission.Comments
				submission.ClaimAcquiredAt = current.Submission.ClaimAcquiredAt
			}
			b.work[i] = current
			b.work[i].Submission = &submission
		}
	}
	if b.afterPublish != nil {
		b.afterPublish()
	}
	return submission, nil
}

func (b *implementationMemory) AwaitImplementationReview(_ context.Context, item workflow.ImplementationItem, guard func() error) error {
	if b.beforeTransition != nil {
		b.beforeTransition()
	}
	if err := guard(); err != nil {
		return err
	}
	for i := range b.work {
		if b.work[i].ID == item.ID {
			current := implementationFixture(b.work[i])
			if current.Submission == nil {
				return workflow.PermitImplementationReview(nil)
			}
			projection := current.Submission.Lifecycle
			if err := workflow.PermitImplementationReview(projection); err != nil {
				return err
			}
			projection.Claimed = true
			if !slices.Contains(projection.States, workflow.AwaitingReview) {
				projection.States = append(projection.States, workflow.AwaitingReview)
			}
			if b.failTransition {
				b.failTransition = false
				b.work[i] = workflow.ReconcileImplementation(current)
				return errors.New("interrupted projection")
			}
			projection.States = slices.DeleteFunc(projection.States, func(state workflow.State) bool { return state == workflow.Rework })
			projection.Claimed = false
			current.Source.States = slices.DeleteFunc(current.Source.States, func(state workflow.State) bool { return state == workflow.Ready || state == workflow.NeedsHuman })
			current.Source.Claimed = false
			current.Problem = ""
			b.work[i] = workflow.ReconcileImplementation(current)
		}
	}
	return nil
}

func (b *implementationMemory) ImplementationItems(context.Context) ([]workflow.ImplementationItem, error) {
	items := append([]workflow.ImplementationItem(nil), b.work...)
	for i := range items {
		items[i] = workflow.ReconcileImplementation(implementationFixture(items[i]))
		if number, err := strconv.Atoi(string(items[i].ID)); err == nil {
			if items[i].Order == 0 {
				items[i].Order = number
			}
		}
	}
	return items, nil
}

func (b *implementationMemory) QueuePage(_ context.Context, queue workflow.QueueKind, _ string) (workflow.QueuePage, error) {
	items, err := b.ImplementationItems(context.Background())
	if err != nil {
		return workflow.QueuePage{}, err
	}
	page := workflow.QueuePage{}
	for _, item := range items {
		var candidate workflow.QueueCandidate
		switch {
		case queue == workflow.ReadyQueue && item.State == workflow.Ready:
			candidate = workflow.QueueCandidate{ID: item.ID, Number: item.Order, CreatedAt: item.CreatedAt, Claimed: item.Claimed, Problem: item.Problem}
		case queue == workflow.ReworkQueue && item.State == workflow.Rework && item.Submission != nil:
			number, err := strconv.Atoi(string(item.Submission.ID))
			if err != nil {
				number = item.Order
			}
			candidate = workflow.QueueCandidate{ID: item.ID, SubmissionID: item.Submission.ID, Number: number, CreatedAt: item.Submission.CreatedAt, Claimed: item.Claimed, Problem: item.Problem}
		case queue == workflow.ReviewQueue && item.State == workflow.AwaitingReview && item.Submission != nil:
			number, err := strconv.Atoi(string(item.Submission.ID))
			if err != nil {
				number = item.Order
			}
			candidate = workflow.QueueCandidate{ID: item.ID, SubmissionID: item.Submission.ID, Number: number, CreatedAt: item.Submission.CreatedAt, Claimed: item.Claimed, Problem: item.Problem}
		default:
			continue
		}
		page.Candidates = append(page.Candidates, candidate)
	}
	return page, nil
}

func (b *implementationMemory) SelectedImplementation(_ context.Context, candidate workflow.QueueCandidate) (workflow.ImplementationItem, error) {
	items, err := b.ImplementationItems(context.Background())
	if err != nil {
		return workflow.ImplementationItem{}, err
	}
	for _, item := range items {
		if candidate.SubmissionID != "" {
			if item.Submission != nil && item.Submission.ID == candidate.SubmissionID {
				return item, nil
			}
			continue
		}
		if item.ID == candidate.ID {
			return item, nil
		}
	}
	return workflow.ImplementationItem{}, workflow.Refuse("selected Work Item is unavailable; repair its attachment")
}

func (b *implementationMemory) ResumedImplementation(_ context.Context, id workflow.WorkItemID, branch string) (workflow.ImplementationItem, error) {
	items, err := b.ImplementationItems(context.Background())
	if err != nil {
		return workflow.ImplementationItem{}, err
	}
	matches := 0
	var found workflow.ImplementationItem
	for _, item := range items {
		if id != "" {
			if item.ID != id {
				continue
			}
		} else if branch == "" || item.Branch != branch || !item.Claimed {
			continue
		}
		matches++
		found = item
	}
	if matches == 0 {
		return workflow.ImplementationItem{}, workflow.Refuse("explicit Work Item is unavailable; repair its attachment before resuming")
	}
	if matches > 1 {
		return workflow.ImplementationItem{}, workflow.Refuse("worktree identity is ambiguous; resume with --item after repairing attachments")
	}
	return found, nil
}

func (b *implementationMemory) ImplementationDependencies(_ context.Context, id workflow.WorkItemID) ([]workflow.BlockerObservation, error) {
	items, err := b.ImplementationItems(context.Background())
	if err != nil {
		return nil, err
	}
	merged := make(map[workflow.WorkItemID]bool)
	var target workflow.ImplementationItem
	for _, item := range items {
		merged[item.ID] = item.State == workflow.Merged
		if item.ID == id {
			target = item
		}
	}
	var observations []workflow.BlockerObservation
	for _, blocker := range target.Blockers {
		observations = append(observations, workflow.BlockerObservation{ID: blocker, Merged: merged[blocker]})
	}
	return observations, nil
}

func (b *implementationMemory) ClaimSelected(_ context.Context, candidate workflow.QueueCandidate, item workflow.ImplementationItem) (workflow.ImplementationItem, error) {
	for i := range b.work {
		current := implementationFixture(b.work[i])
		if candidate.SubmissionID != "" {
			if current.Submission == nil || current.Submission.ID != candidate.SubmissionID {
				continue
			}
		} else if current.ID != candidate.ID {
			continue
		}
		reconciled := workflow.ReconcileImplementation(current)
		if reconciled.Problem != "" {
			return workflow.ImplementationItem{}, workflow.Refuse("selected Work Item has contradictory projections; inspect it before retrying")
		}
		if reconciled.Claimed {
			return workflow.ImplementationItem{}, workflow.Refuse("selected Work Item already carries a Claim; resume with --item if it is yours")
		}
		b.work[i] = current
		if reconciled.State == workflow.Rework || reconciled.State == workflow.AwaitingReview {
			b.work[i].Submission.Lifecycle.Claimed = true
			b.work[i].Submission.ClaimAcquiredAt = b.reviewTime()
		} else {
			b.work[i].Source.Claimed = true
			b.work[i].SourceClaimAcquiredAt = b.reviewTime()
		}
		b.work[i] = workflow.ReconcileImplementation(b.work[i])
		result := item
		result.Claimed = b.work[i].Claimed
		if result.Source != nil && b.work[i].Source != nil {
			result.Source.Claimed = b.work[i].Source.Claimed
			result.SourceClaimAcquiredAt = b.work[i].SourceClaimAcquiredAt
		}
		if result.Submission != nil && b.work[i].Submission != nil {
			result.Submission.Claimed = b.work[i].Submission.Claimed
			result.Submission.ClaimAcquiredAt = b.work[i].Submission.ClaimAcquiredAt
			result.Submission.Lifecycle = b.work[i].Submission.Lifecycle
		}
		return workflow.ReconcileImplementation(result), nil
	}
	return workflow.ImplementationItem{}, workflow.Refuse("selected Work Item disappeared before Claim; inspect its stable identity")
}

type incompleteImplementationMemory struct{ implementationMemory }

func (b *incompleteImplementationMemory) ImplementationItems(context.Context) ([]workflow.ImplementationItem, error) {
	return append([]workflow.ImplementationItem(nil), b.work...), nil
}

func TestStatusRefusesImplementationProjectionWithoutResultDocument(t *testing.T) {
	b := &implementationMemory{work: []workflow.ImplementationItem{{
		ID: "7", Branch: "widget", State: workflow.Ready,
		Submission: &workflow.Submission{ID: "11", State: workflow.AwaitingReview, Head: "fixed"},
	}}, failTransition: true}
	_, err := workflow.ObserveStatus(context.Background(), b)
	if err == nil || !strings.Contains(err.Error(), "original Result Document") || b.work[0].Claimed {
		t.Fatalf("status guessed partial handoff authority: %v, %#v", err, b.work[0])
	}
}

func TestImplementationReviewProjectionBackendParity(t *testing.T) {
	sourceStates := map[string]workflow.State{"ready": workflow.Ready, "needs-human": workflow.NeedsHuman}
	for _, tt := range []struct {
		labels []string
		states []workflow.State
		source []string
		allow  bool
	}{
		{labels: nil, states: nil, allow: true},
		{labels: []string{"rework", "wip"}, states: []workflow.State{workflow.Rework}, allow: true},
		{labels: []string{"rework", "review"}, states: []workflow.State{workflow.Rework, workflow.AwaitingReview}, allow: false},
		{labels: []string{"review", "rework"}, states: []workflow.State{workflow.AwaitingReview, workflow.Rework}, allow: false},
		{labels: []string{"rework", "wip"}, states: []workflow.State{workflow.Rework}, source: []string{"needs-human"}, allow: true},
		{labels: []string{"ready"}, states: []workflow.State{workflow.Ready}, allow: false},
		{labels: []string{"done"}, states: []workflow.State{workflow.ReadyForMerge}, allow: false},
		{labels: []string{"needs-human"}, states: []workflow.State{workflow.NeedsHuman}, allow: false},
		{labels: []string{"review", "needs-human"}, states: []workflow.State{workflow.AwaitingReview, workflow.NeedsHuman}, allow: false},

		{labels: []string{"review", "done"}, states: []workflow.State{workflow.AwaitingReview, workflow.ReadyForMerge}, allow: false},
		{labels: []string{"done", "review"}, states: []workflow.State{workflow.ReadyForMerge, workflow.AwaitingReview}, allow: false},
		{labels: []string{"review", "ready"}, states: []workflow.State{workflow.AwaitingReview, workflow.Ready}, allow: false},
		{labels: []string{"ready", "review"}, states: []workflow.State{workflow.Ready, workflow.AwaitingReview}, allow: false},
	} {
		t.Run(fmt.Sprint(tt.labels, tt.source), func(t *testing.T) {
			labels, source := slices.Clone(tt.labels), slices.Clone(tt.source)
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path := strings.TrimPrefix(r.URL.Path, "/repos/acme/widgets")
				if path == "/graphql" && r.Method == http.MethodPost {
					fmt.Fprint(w, `{"data":{"repository":{"issue":{"closedByPullRequestsReferences":{"nodes":[{"number":11,"repository":{"nameWithOwner":"acme/widgets"}}]}}}}}`)
					return
				}
				if r.Method != http.MethodGet {
					writes++
				}
				labelList := func(names []string) []map[string]string {
					ls := []map[string]string{}
					for _, label := range names {
						ls = append(ls, map[string]string{"name": label})
					}
					return ls
				}
				switch {
				case path == "/pulls/11" && r.Method == http.MethodGet:
					json.NewEncoder(w).Encode(map[string]any{"number": 11, "body": "Closes #7", "head": map[string]any{"repo": map[string]string{"full_name": "acme/widgets"}}})
				case path == "/issues/11" && r.Method == http.MethodGet:
					json.NewEncoder(w).Encode(map[string]any{"number": 11, "state": "open", "labels": labelList(labels)})
				case path == "/issues/7" && r.Method == http.MethodGet:
					json.NewEncoder(w).Encode(map[string]any{"number": 7, "state": "open", "labels": labelList(source)})
				case path == "/issues/11/labels" && r.Method == http.MethodPost:
					var payload struct{ Labels []string }
					json.NewDecoder(r.Body).Decode(&payload)
					labels = append(labels, payload.Labels...)
				case path == "/issues/7/labels" && r.Method == http.MethodPost:
					var payload struct{ Labels []string }
					json.NewDecoder(r.Body).Decode(&payload)
					source = append(source, payload.Labels...)
				case strings.HasPrefix(path, "/issues/11/labels/") && r.Method == http.MethodDelete:
					label := strings.TrimPrefix(path, "/issues/11/labels/")
					labels = slices.DeleteFunc(labels, func(value string) bool { return value == label })
				case strings.HasPrefix(path, "/issues/7/labels/") && r.Method == http.MethodDelete:
					label := strings.TrimPrefix(path, "/issues/7/labels/")
					source = slices.DeleteFunc(source, func(value string) bool { return value == label })
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			native := setup.NewGitHubBackend(server.URL, "token", server.Client())
			native.BindRepository(github.RepositoryID{Owner: "acme", Name: "widgets"})
			item := workflow.ImplementationItem{ID: "7", State: workflow.Rework, Source: &workflow.LifecycleObservation{Open: true}, Submission: &workflow.Submission{ID: "11", Lifecycle: &workflow.LifecycleObservation{Open: true, States: slices.Clone(tt.states), Claimed: slices.Contains(tt.labels, "wip")}}}
			for _, label := range tt.source {
				item.Source.States = append(item.Source.States, sourceStates[label])
			}
			memory := &implementationMemory{work: []workflow.ImplementationItem{item}}
			for name, backend := range map[string]workflow.ImplementationBackend{"HTTP": native, "memory": memory} {
				err := backend.AwaitImplementationReview(context.Background(), item, func() error { return nil })
				if (err == nil) != tt.allow {
					t.Fatalf("%s permission differs: %v", name, err)
				}
			}
			wantLabels, wantStates, wantSource := slices.Clone(tt.labels), slices.Clone(tt.states), slices.Clone(tt.source)
			if tt.allow {
				if !slices.Contains(wantLabels, "review") {
					wantLabels = append(wantLabels, "review")
					wantStates = append(wantStates, workflow.AwaitingReview)
				}
				wantLabels = slices.DeleteFunc(wantLabels, func(label string) bool { return label == "rework" || label == "wip" })
				wantStates = slices.DeleteFunc(wantStates, func(state workflow.State) bool { return state == workflow.Rework })
				wantSource = slices.DeleteFunc(wantSource, func(label string) bool { return label == "needs-human" })
			} else if writes != 0 {
				t.Fatalf("adapter independently restored Claim or projected forbidden state: %d writes", writes)
			}
			var wantSourceStates []workflow.State
			for _, label := range wantSource {
				wantSourceStates = append(wantSourceStates, sourceStates[label])
			}
			if !slices.Equal(labels, wantLabels) || !slices.Equal(memory.work[0].Submission.Lifecycle.States, wantStates) || memory.work[0].Submission.Lifecycle.Claimed || !slices.Equal(source, wantSource) || !slices.Equal(memory.work[0].Source.States, wantSourceStates) {
				t.Fatalf("projection mismatch: HTTP=%v/%v, memory=%#v/%#v", labels, source, memory.work[0].Submission.Lifecycle, memory.work[0].Source)
			}
		})
	}
}
