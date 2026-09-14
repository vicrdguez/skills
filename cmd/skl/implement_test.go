package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	skilldist "github.com/vicrdguez/skills"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/setup"
	"github.com/vicrdguez/skills/workflow"
)

type implementationMemory struct {
	roundWriteError       error
	itemReads, roundReads int
	memoryBackend
	coordination     []workflow.CoordinationItem
	work             []workflow.ImplementationItem
	remoteHeads      map[string]string
	afterPublish     func()
	decisions        map[workflow.WorkItemID]string
	failTransition   bool
	beforeTransition func()
	afterCompletion  func()

	rounds         map[workflow.WorkItemID][]workflow.DispatchRound
	afterRound     func(workflow.DispatchRound) error
	roundReadError error
	reviewClock    int
}

func (b *implementationMemory) DispatchRounds(_ context.Context, item workflow.WorkItemID) ([]workflow.DispatchRound, error) {
	b.roundReads++
	return append([]workflow.DispatchRound(nil), b.rounds[item]...), b.roundReadError
}

func (b *implementationMemory) RecordDispatchRound(_ context.Context, round workflow.DispatchRound) error {
	if b.roundWriteError != nil {
		return b.roundWriteError
	}
	if b.rounds == nil {
		b.rounds = make(map[workflow.WorkItemID][]workflow.DispatchRound)
	}
	for i := range b.rounds[round.Item] {
		if b.rounds[round.Item][i].ID == round.ID {
			b.rounds[round.Item][i] = round
			if b.afterRound != nil {
				return b.afterRound(round)
			}
			return nil
		}
	}
	if !slices.Contains(b.rounds[round.Item], round) {
		b.rounds[round.Item] = append(b.rounds[round.Item], round)
	}
	if b.afterRound != nil {
		return b.afterRound(round)
	}
	return nil

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

func (b *implementationMemory) RecordImplementationTransition(_ context.Context, item workflow.ImplementationItem, transition workflow.ImplementationTransition) error {
	for i := range b.work {
		if b.work[i].ID == item.ID {
			b.work[i].Transition = &transition
		}
	}
	if transition.Completed && b.afterCompletion != nil {
		b.afterCompletion()
	}
	return nil
}

func (b *implementationMemory) RetainImplementationClaim(_ context.Context, item workflow.ImplementationItem) error {
	for i := range b.work {
		if b.work[i].ID == item.ID {
			current := implementationFixture(b.work[i])
			claim := current.Source
			if item.State == workflow.Rework && current.Submission != nil {
				claim = current.Submission.Lifecycle
			}
			claim.Claimed = true
			b.work[i] = workflow.ReconcileImplementation(current)
		}
	}
	return nil
}

func (b *implementationMemory) ImplementationTarget(context.Context) (string, error) {
	return "main", nil
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
			current.ResumeState = item.State
			projection := current.Source
			if current.Submission != nil {
				projection = current.Submission.Lifecycle
			}
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
	if item.Submission != nil {
		submission.PreviousReviewedHead = item.Submission.PreviousReviewedHead
		submission.ReviewedHead = item.Submission.ReviewedHead
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

func TestImplementRefusesInvalidHandoff(t *testing.T) {
	for _, invariant := range []string{"Target Snapshot", "heads differ", "ledger", "Workflow State", "head changed"} {
		t.Run(invariant, func(t *testing.T) {
			root := proposalRepository(t)
			prepareSlice(t, root, "widget")
			backend := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Ready}}, remoteHeads: map[string]string{}}
			start := implementCLI(t, root, backend, "next")
			body := filepath.Join(start.Packet.Facts.Implementation.ResultDirectory, "submission.md")
			if err := os.WriteFile(body, []byte("opaque\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if invariant != "ledger" {
				completeAndRetireSlice(t, root, "widget")
			}
			backend.remoteHeads["widget"] = strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			switch invariant {
			case "Target Snapshot":
				backend.work[0].TargetSnapshot = "deadbeef"
			case "heads differ":
				backend.remoteHeads["widget"] = "deadbeef"
			case "Workflow State":
				backend.work[0].State = workflow.NeedsHuman
				backend.work[0].Source.States = []workflow.State{workflow.NeedsHuman}
			case "head changed":
				backend.afterPublish = func() { backend.remoteHeads["widget"] = "deadbeef" }
			}
			state := backend.work[0].State
			got := implementCLI(t, root, backend, "submit", "--item", "7", "--body", body)
			if got.Status != "fix_required" || !strings.Contains(got.Reason, invariant) || !backend.work[0].Claimed || backend.work[0].State != state {
				t.Fatalf("invalid handoff: %#v %#v", got, backend.work)
			}
			if _, err := os.Stat(body); err != nil {
				t.Fatalf("failed handoff removed prose: %v", err)
			}
		})
	}
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
			current.Source.States = slices.DeleteFunc(current.Source.States, func(state workflow.State) bool { return state == workflow.Ready })
			current.Source.Claimed = false
			b.work[i] = workflow.ReconcileImplementation(current)
		}
	}
	return nil
}

func (b *implementationMemory) ImplementationItems(_ context.Context) ([]workflow.ImplementationItem, error) {
	b.itemReads++

	items := append([]workflow.ImplementationItem(nil), b.work...)
	for i := range items {
		items[i] = workflow.ReconcileImplementation(implementationFixture(items[i]))
		if items[i].Submission != nil {
			submission := *items[i].Submission
			for _, round := range b.rounds[items[i].ID] {
				if round.Outcome != "" || round.Submission != submission.ID {
					continue
				}
				if round.Lane == workflow.ImplementLane && !round.Synchronization {
					if submission.PreviousReviewedHead != "" && submission.PreviousReviewedHead != round.Obligation {
						items[i].Problem = "active dispatch contradicts the reviewed obligation"
					} else {
						submission.PreviousReviewedHead = round.Obligation
					}
				}
				if round.Lane == workflow.WatchdogLane {
					if submission.ReviewedHead != "" && submission.ReviewedHead != round.Obligation {
						items[i].Problem = "active dispatch contradicts the reviewed Submission head"
					} else {
						submission.ReviewedHead = round.Obligation
					}
				}
			}
			items[i].Submission = &submission
		}
		if number, err := strconv.Atoi(string(items[i].ID)); err == nil {
			if items[i].Order == 0 {
				items[i].Order = number
			}
		}
	}
	return items, nil
}

func (b *implementationMemory) ClaimImplementation(_ context.Context, item workflow.ImplementationItem) error {
	for i := range b.work {
		if b.work[i].ID == item.ID {
			b.work[i] = implementationFixture(b.work[i])
			wasClaimed := b.work[i].Claimed
			b.work[i].TargetSnapshot = item.TargetSnapshot
			b.work[i].TargetBranch = item.TargetBranch
			if item.Submission != nil && !wasClaimed {
				b.work[i].Submission = item.Submission
			}
			claim := b.work[i].Source
			if item.State == workflow.Rework || item.State == workflow.AwaitingReview {
				claim = b.work[i].Submission.Lifecycle
			}
			claim.Claimed = true
			if !wasClaimed && b.work[i].Submission != nil {
				b.work[i].Submission.ClaimAcquiredAt = b.reviewTime()
			}
			b.work[i] = workflow.ReconcileImplementation(b.work[i])
		}
	}
	return nil
}

func implementCLI(t *testing.T, root string, backend *implementationMemory, args ...string) setup.ImplementationOutput {
	t.Helper()
	if backend.remoteHeads == nil {
		backend.remoteHeads = make(map[string]string)
	}
	if _, ok := backend.remoteHeads["main"]; !ok {
		backend.remoteHeads["main"] = strings.TrimSpace(runGitOutput(t, root, "rev-parse", "refs/heads/main"))
	}
	var output bytes.Buffer
	app := newApp(func(repository github.RepositoryID) (setup.Backend, error) {
		backend.repository = repository
		return backend, nil
	}, bytes.NewReader(nil), &output, &output)
	command := append([]string{"skl", "implement"}, args...)
	command = append(command, "--repo", root)
	if err := app.Run(command); err != nil {
		t.Fatalf("%v: %v\n%s", command, err, &output)
	}
	var result setup.ImplementationOutput
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("%v: %s", err, &output)
	}
	if result.Packet != nil && result.Packet.Facts.Implementation.ResultDirectory != "" {
		t.Cleanup(func() { os.RemoveAll(result.Packet.Facts.Implementation.ResultDirectory) })
	}
	return result
}

func returnedCLI(t *testing.T, backend setup.Backend, command string) setup.ImplementationOutput {
	t.Helper()
	parsed, err := exec.Command("sh", "-c", "set -- "+command+`; printf '%s\000' "$@"`).Output()
	if err != nil {
		t.Fatal(err)
	}
	fields := bytes.Split(bytes.TrimSuffix(parsed, []byte{0}), []byte{0})
	args := make([]string, len(fields))
	for i := range fields {
		args[i] = string(fields[i])
	}
	var output bytes.Buffer
	app := newApp(func(repository github.RepositoryID) (setup.Backend, error) {
		if bound, ok := backend.(interface{ BindRepository(github.RepositoryID) }); ok {
			bound.BindRepository(repository)
		}
		return backend, nil
	}, bytes.NewReader(nil), &output, &output)
	if err := app.Run(args); err != nil {
		t.Fatalf("returned command failed: %v\n%s", err, &output)
	}
	var result setup.ImplementationOutput
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func newImplementationResultDirectory(t *testing.T) string {
	t.Helper()
	directory, err := os.MkdirTemp("", "skl-implement-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(directory) })
	if err := os.WriteFile(filepath.Join(directory, ".skl-result"), []byte("skl.implement/v1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return directory
}

func TestB2ResolveMarkersOnlyInSelectedSliceHistory(t *testing.T) {
	for _, other := range []string{
		"other slices",
		"unreachable markers",
		"non-subject markers",
		"merge-parent markers",
		"explanatory subjects",
	} {
		t.Run(other, func(t *testing.T) {
			root := proposalRepository(t)
			baselineSubject, completionSubject := "[baseline] ship-widget", "[completion] ship-widget"
			if other == "explanatory subjects" {
				baselineSubject += " accepted contract"
				completionSubject += " finished contract"
			}
			runGit(t, root, "switch", "-c", "ship-widget", "main")
			writeLedger(t, root, "ship-widget", true)
			runGit(t, root, "add", ".changes/ship-widget")
			runGit(t, root, "commit", "-m", baselineSubject)
			baseline := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))

			switch other {
			case "other slices":
				runGit(t, root, "commit", "--allow-empty", "-m", "[baseline] ship-widget-extra")
				runGit(t, root, "commit", "--allow-empty", "-m", "[completion] another-slice")
			case "unreachable markers":
				runGit(t, root, "branch", "unreachable", "main")
				runGit(t, root, "switch", "unreachable")
				runGit(t, root, "commit", "--allow-empty", "-m", "[baseline] ship-widget")
				runGit(t, root, "commit", "--allow-empty", "-m", "[completion] ship-widget")
				runGit(t, root, "switch", "ship-widget")
			case "non-subject markers":
				runGit(t, root, "commit", "--allow-empty", "-m", "note", "-m", "[baseline] ship-widget")
				runGit(t, root, "commit", "--allow-empty", "-m", "prefix [completion] ship-widget")
				runGit(t, root, "commit", "--allow-empty", "-m", "[Completion] ship-widget")
			}
			runGit(t, root, "commit", "--allow-empty", "-m", completionSubject)
			completion := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			runGit(t, root, "rm", "-r", ".changes/ship-widget")
			runGit(t, root, "commit", "-m", "retire")
			if other == "merge-parent markers" {
				evidence := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
				runGit(t, root, "switch", "-C", "ship-widget", "main")
				runGit(t, root, "merge", "--no-ff", evidence, "-m", "merge evidence")
			}

			backend := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "ship-widget", State: workflow.AwaitingReview}}}
			got := implementCLI(t, root, backend, "inspect", "--item", "7")
			if got.Status != "inspected" || got.Ledger == nil || got.Ledger.Baseline != baseline || got.Ledger.Completion != completion || len(got.Ledger.Violations) != 0 {
				t.Fatalf("inspection = %#v", got)
			}
			if backend.work[0].Claimed {
				t.Fatal("inspection mutated Work Item")
			}
		})
	}
}

func TestB3RefuseMissingOrAmbiguousRequiredMarkers(t *testing.T) {
	for _, problem := range []string{"missing baseline", "missing completion", "ambiguous baseline", "ambiguous completion"} {
		t.Run(problem, func(t *testing.T) {
			root := proposalRepository(t)
			runGit(t, root, "switch", "-c", "ship-widget", "main")
			writeLedger(t, root, "ship-widget", true)
			runGit(t, root, "add", ".changes/ship-widget")
			baselineSubject := "[baseline] ship-widget"
			if problem == "missing baseline" {
				baselineSubject = "unmarked baseline"
			}
			runGit(t, root, "commit", "-m", baselineSubject)
			var competing []string
			if problem == "ambiguous baseline" {
				competing = append(competing, strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD")))
				runGit(t, root, "commit", "--allow-empty", "-m", "[baseline] ship-widget duplicate")
				competing = append(competing, strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD")))
			}
			if problem != "missing completion" {
				if problem == "ambiguous completion" {
					runGit(t, root, "switch", "-c", "completion-side")
					runGit(t, root, "commit", "--allow-empty", "-m", "[completion] ship-widget side")
					competing = append(competing, strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD")))
					runGit(t, root, "switch", "ship-widget")
				}
				runGit(t, root, "commit", "--allow-empty", "-m", "[completion] ship-widget")
				if problem == "ambiguous completion" {
					competing = append(competing, strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD")))
					runGit(t, root, "merge", "--no-ff", "completion-side", "-m", "merge competing completion")
				}
				runGit(t, root, "rm", "-r", ".changes/ship-widget")
				runGit(t, root, "commit", "-m", "retire")
			}
			head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			item := workflow.ImplementationItem{ID: "7", Branch: "ship-widget", State: workflow.AwaitingReview}
			backend := &implementationMemory{work: []workflow.ImplementationItem{item}}
			got := implementCLI(t, root, backend, "inspect", "--item", "7")
			violations := strings.Join(got.Ledger.Violations, "\n")
			kind := strings.TrimPrefix(problem, "missing ")
			kind = strings.TrimPrefix(kind, "ambiguous ")
			if !strings.Contains(violations, "ship-widget") || !strings.Contains(violations, kind) {
				t.Fatalf("violations = %q", violations)
			}
			for _, sha := range competing {
				if !strings.Contains(violations, sha) {
					t.Fatalf("violations %q omit competing SHA %s", violations, sha)
				}
			}

			backend.work[0].State = workflow.Ready
			if strings.Contains(problem, "completion") {
				backend.work[0].State = workflow.Rework
				backend.work[0].Submission = &workflow.Submission{ID: "11", Head: head}
			}
			start := implementCLI(t, root, backend, "next")
			if start.Status != "fix_required" || backend.work[0].Claimed {
				t.Fatalf("startup chose invalid evidence: %#v, work=%#v", start, backend.work)
			}
		})
	}
}

func TestB4EnforceEndpointPathsModesAndExactContent(t *testing.T) {
	type mutation struct {
		name, want string
		baseline   func(*testing.T, string)
		completion func(*testing.T, string)
	}
	write := func(t *testing.T, root, path, contents string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, path), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	removeBehavior := func(t *testing.T, root string) {
		t.Helper()
		if err := os.Remove(filepath.Join(root, ".changes/ship-widget/behavior.md")); err != nil {
			t.Fatal(err)
		}
	}
	cases := []mutation{
		{name: "permitted ticks"},
		{name: "added path", want: "path set", completion: func(t *testing.T, root string) { write(t, root, ".changes/ship-widget/extra.md", "extra\n") }},
		{name: "missing path", want: "path set", completion: removeBehavior},
		{name: "renamed path", want: "path set", completion: func(t *testing.T, root string) {
			runGit(t, root, "mv", ".changes/ship-widget/behavior.md", ".changes/ship-widget/renamed.md")
		}},
		{name: "nested extra path", want: "path set", completion: func(t *testing.T, root string) { write(t, root, ".changes/ship-widget/nested/extra.md", "extra\n") }},
		{name: "executable", want: "mode 100644 blob", completion: func(t *testing.T, root string) {
			if err := os.Chmod(filepath.Join(root, ".changes/ship-widget/intent.md"), 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "symlink entry", want: "mode 100644 blob", completion: func(t *testing.T, root string) {
			if err := os.Symlink("intent.md", filepath.Join(root, ".changes/ship-widget/link.md")); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "gitlink entry", want: "mode 100644 blob", completion: func(t *testing.T, root string) {
			link := filepath.Join(root, ".changes/ship-widget/link")
			if err := os.Mkdir(link, 0o755); err != nil {
				t.Fatal(err)
			}
			runGit(t, link, "init")
			runGit(t, link, "config", "user.name", "Test")
			runGit(t, link, "config", "user.email", "test@example.com")
			runGit(t, link, "commit", "--allow-empty", "-m", "linked")
		}},
		{name: "changed prose", want: "content changed", completion: func(t *testing.T, root string) { write(t, root, ".changes/ship-widget/behavior.md", "changed\n") }},
		{name: "changed whitespace", want: "content changed", completion: func(t *testing.T, root string) { write(t, root, ".changes/ship-widget/behavior.md", "behavior.md  \n") }},
		{name: "changed line order", want: "content changed", baseline: func(t *testing.T, root string) { write(t, root, ".changes/ship-widget/behavior.md", "one\ntwo\n") }, completion: func(t *testing.T, root string) { write(t, root, ".changes/ship-widget/behavior.md", "two\none\n") }},
		{name: "changed line endings", want: "content changed", completion: func(t *testing.T, root string) { write(t, root, ".changes/ship-widget/behavior.md", "behavior.md\r\n") }},
		{name: "changed final newline", want: "content changed", completion: func(t *testing.T, root string) { write(t, root, ".changes/ship-widget/behavior.md", "behavior.md") }},
		{name: "ledger root blob", want: "invalid ledger shape", baseline: func(t *testing.T, root string) {
			if err := os.RemoveAll(filepath.Join(root, ".changes/ship-widget")); err != nil {
				t.Fatal(err)
			}
			write(t, root, ".changes/ship-widget", "not a directory\n")
		}},
		{name: "ledger root symlink", want: "invalid ledger shape", baseline: func(t *testing.T, root string) {
			if err := os.RemoveAll(filepath.Join(root, ".changes/ship-widget")); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("elsewhere", filepath.Join(root, ".changes/ship-widget")); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "missing required artifact", want: "ledger misses behavior.md", baseline: removeBehavior},
		{name: "nested required artifacts", want: "ledger misses intent.md", baseline: func(t *testing.T, root string) {
			if err := os.Mkdir(filepath.Join(root, ".changes/ship-widget/nested"), 0o755); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"intent.md", "behavior.md"} {
				if err := os.Rename(filepath.Join(root, ".changes/ship-widget", name), filepath.Join(root, ".changes/ship-widget/nested", name)); err != nil {
					t.Fatal(err)
				}
			}
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			root := proposalRepository(t)
			runGit(t, root, "switch", "-c", "ship-widget", "main")
			writeLedger(t, root, "ship-widget", true)
			write(t, root, ".changes/ship-widget/intent.md", "# Intent\n- [ ] Ship\n## Manual verification\n- [ ] Human\n")
			if test.baseline != nil {
				test.baseline(t, root)
			}
			runGit(t, root, "add", "-A")
			runGit(t, root, "commit", "-m", "[baseline] ship-widget")
			if _, err := os.Stat(filepath.Join(root, ".changes/ship-widget/intent.md")); err == nil {
				write(t, root, ".changes/ship-widget/intent.md", "# Intent\n- [x] Ship\n## Manual verification\n- [ ] Human\n")
			}
			if test.completion != nil {
				test.completion(t, root)
			}
			runGit(t, root, "add", "-A")
			runGit(t, root, "commit", "--allow-empty", "-m", "[completion] ship-widget")
			if err := os.RemoveAll(filepath.Join(root, ".changes/ship-widget")); err != nil {
				t.Fatal(err)
			}
			runGit(t, root, "add", "-A")
			runGit(t, root, "commit", "-m", "retire")

			backend := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "ship-widget", State: workflow.AwaitingReview}}}
			got := implementCLI(t, root, backend, "inspect", "--item", "7")
			violations := strings.Join(got.Ledger.Violations, "\n")
			if test.want == "" && violations != "" || test.want != "" && !strings.Contains(violations, test.want) {
				t.Fatalf("violations = %q, want %q", violations, test.want)
			}
		})
	}
}

func TestB5EnforcePhaseAppropriateCompletionTicks(t *testing.T) {
	for _, test := range []struct {
		name, baseline, completion, want string
		baselineOnly                     bool
	}{
		{"lowercase completion ticks", "# Intent\n1) [ ] Ship\n* [ ] Test\n## Manual Verification ###\n- [ ] Human\n", "# Intent\n1) [x] Ship\n* [x] Test\n## Manual Verification ###\n- [ ] Human\n", "", false},
		{"automated box incomplete", "- [ ] Ship\n", "- [ ] Ship\n", "agent-verifiable checkbox unchecked", false},
		{"manual box checked at baseline", "## Manual verification\n- [x] Human\n", "## Manual verification\n- [x] Human\n", "Manual Verification", false},
		{"manual box checked at completion", "- [ ] Ship\n## Manual verification\n- [ ] Human\n", "- [x] Ship\n## Manual verification\n- [x] Human\n", "Manual Verification", false},
		{"reverse tick", "- [x] Ship\n", "- [ ] Ship\n", "content changed", false},
		{"uppercase tick", "- [ ] Ship\n", "- [X] Ship\n", "content changed", false},
		{"baseline only incomplete", "- [ ] Ship\n## Manual verification\n- [ ] Human\n", "", "", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := proposalRepository(t)
			runGit(t, root, "switch", "-c", "ship-widget", "main")
			writeLedger(t, root, "ship-widget", true)
			if err := os.WriteFile(filepath.Join(root, ".changes/ship-widget/intent.md"), []byte(test.baseline), 0o644); err != nil {
				t.Fatal(err)
			}
			runGit(t, root, "add", ".changes/ship-widget")
			runGit(t, root, "commit", "-m", "[baseline] ship-widget")
			state := workflow.Ready
			if !test.baselineOnly {
				if err := os.WriteFile(filepath.Join(root, ".changes/ship-widget/intent.md"), []byte(test.completion), 0o644); err != nil {
					t.Fatal(err)
				}
				runGit(t, root, "add", ".changes/ship-widget/intent.md")
				runGit(t, root, "commit", "--allow-empty", "-m", "[completion] ship-widget")
				runGit(t, root, "rm", "-r", ".changes/ship-widget")
				runGit(t, root, "commit", "-m", "retire")
				state = workflow.AwaitingReview
			}
			backend := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "ship-widget", State: state}}}
			got := implementCLI(t, root, backend, "inspect", "--item", "7")
			violations := strings.Join(got.Ledger.Violations, "\n")
			if test.want == "" && violations != "" || test.want != "" && !strings.Contains(violations, test.want) {
				t.Fatalf("violations = %q, want %q", violations, test.want)
			}
			if test.baselineOnly && (got.Ledger.Completion != "" || got.Ledger.Phase != "present") {
				t.Fatalf("baseline-only phase = %#v", got.Ledger)
			}
		})
	}
}

func TestB6RequireEndpointAncestryAndLaterLedgerRetirement(t *testing.T) {
	t.Run("markerless equal explicit endpoints before later retirement", func(t *testing.T) {
		root := proposalRepository(t)
		runGit(t, root, "switch", "-c", "ship-widget", "main")
		writeLedger(t, root, "ship-widget", true)
		runGit(t, root, "add", ".changes/ship-widget")
		runGit(t, root, "commit", "-m", "legacy completed artifacts")
		endpoint := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
		runGit(t, root, "rm", "-r", ".changes/ship-widget")
		runGit(t, root, "commit", "-m", "retire later")

		backend := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "ship-widget", State: workflow.AwaitingReview}}}
		got := implementCLI(t, root, backend, "inspect", "--item", "7", "--artifact-baseline", endpoint, "--artifact-completion", endpoint)
		if got.Status != "inspected" || got.Ledger == nil || got.Ledger.Baseline != endpoint || got.Ledger.Completion != endpoint || got.Ledger.Phase != "retired" || len(got.Ledger.Violations) != 0 {
			t.Fatalf("equal explicit endpoint inspection = %#v", got)
		}
	})

	for _, test := range []struct {
		name, want string
		arrange    func(*testing.T, string)
	}{
		{"valid retired contract", "", func(t *testing.T, root string) {
			prepareSlice(t, root, "ship-widget")
			runGit(t, root, "commit", "--allow-empty", "-m", "[completion] ship-widget")
			runGit(t, root, "rm", "-r", ".changes/ship-widget")
			runGit(t, root, "commit", "-m", "retire")
		}},
		{"unrelated commits before deletion", "", func(t *testing.T, root string) {
			prepareSlice(t, root, "ship-widget")
			runGit(t, root, "commit", "--allow-empty", "-m", "[completion] ship-widget")
			runGit(t, root, "commit", "--allow-empty", "-m", "unrelated one")
			runGit(t, root, "commit", "--allow-empty", "-m", "unrelated two")
			runGit(t, root, "rm", "-r", ".changes/ship-widget")
			runGit(t, root, "commit", "-m", "retire later")
		}},
		{"baseline not ancestor of completion", "not an ancestor", func(t *testing.T, root string) {
			runGit(t, root, "switch", "-c", "baseline-side", "main")
			writeLedger(t, root, "ship-widget", true)
			runGit(t, root, "add", ".changes/ship-widget")
			runGit(t, root, "commit", "-m", "[baseline] ship-widget")
			runGit(t, root, "switch", "-c", "ship-widget", "main")
			writeLedger(t, root, "ship-widget", true)
			runGit(t, root, "add", ".changes/ship-widget")
			runGit(t, root, "commit", "-m", "[completion] ship-widget")
			runGit(t, root, "rm", "-r", ".changes/ship-widget")
			runGit(t, root, "commit", "-m", "retire")
			runGit(t, root, "merge", "--no-ff", "-s", "ours", "baseline-side", "-m", "include baseline evidence")
		}},
		{"completion marker lacks artifacts", "missing ledger directory", func(t *testing.T, root string) {
			prepareSlice(t, root, "ship-widget")
			runGit(t, root, "rm", "-r", ".changes/ship-widget")
			runGit(t, root, "commit", "-m", "[completion] ship-widget")
		}},
		{"review head retains ledger", "must be absent at review head", func(t *testing.T, root string) {
			prepareSlice(t, root, "ship-widget")
			runGit(t, root, "commit", "--allow-empty", "-m", "[completion] ship-widget")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := proposalRepository(t)
			test.arrange(t, root)
			backend := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "ship-widget", State: workflow.AwaitingReview}}}
			got := implementCLI(t, root, backend, "inspect", "--item", "7")
			violations := strings.Join(got.Ledger.Violations, "\n")
			if test.want == "" && violations != "" || test.want != "" && !strings.Contains(violations, test.want) {
				t.Fatalf("violations = %q, want %q", violations, test.want)
			}
			if test.want == "" && (got.Ledger.Phase != "retired" || got.Ledger.Deletion != "") {
				t.Fatalf("retirement evidence = %#v", got.Ledger)
			}
		})
	}
}

func TestB7AcceptRestoredIntermediateArtifactEdits(t *testing.T) {
	type temporaryChange struct {
		name             string
		beforeCompletion func(*testing.T, string)
		afterRetirement  func(*testing.T, string)
	}
	write := func(t *testing.T, root, name, contents string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, ".changes/ship-widget", name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	commit := func(t *testing.T, root, message string) {
		t.Helper()
		runGit(t, root, "add", "-A")
		runGit(t, root, "commit", "-m", message)
	}
	for _, test := range []temporaryChange{
		{"edit and restore prose", func(t *testing.T, root string) {
			write(t, root, "behavior.md", "temporary\n")
			commit(t, root, "temporary prose")
			write(t, root, "behavior.md", "behavior.md\n")
			commit(t, root, "restore prose")
		}, nil},
		{"change paths and modes then restore", func(t *testing.T, root string) {
			write(t, root, "extra.md", "temporary\n")
			if err := os.Chmod(filepath.Join(root, ".changes/ship-widget/behavior.md"), 0o755); err != nil {
				t.Fatal(err)
			}
			commit(t, root, "temporary tree")
			if err := os.Remove(filepath.Join(root, ".changes/ship-widget/extra.md")); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(filepath.Join(root, ".changes/ship-widget/behavior.md"), 0o644); err != nil {
				t.Fatal(err)
			}
			commit(t, root, "restore tree")
		}, nil},
		{"tick and untick", func(t *testing.T, root string) {
			write(t, root, "intent.md", "- [x] Ship\n")
			commit(t, root, "temporary tick")
			write(t, root, "intent.md", "- [ ] Ship\n")
			commit(t, root, "restore tick")
		}, nil},
		{"delete and restore", func(t *testing.T, root string) {
			if err := os.RemoveAll(filepath.Join(root, ".changes/ship-widget")); err != nil {
				t.Fatal(err)
			}
			commit(t, root, "temporary deletion")
			writeLedger(t, root, "ship-widget", true)
			write(t, root, "intent.md", "- [ ] Ship\n")
			commit(t, root, "restore ledger")
		}, nil},
		{"merge change and restore", func(t *testing.T, root string) {
			runGit(t, root, "switch", "-c", "temporary-merge")
			write(t, root, "behavior.md", "merged temporary change\n")
			commit(t, root, "change in merge parent")
			runGit(t, root, "switch", "ship-widget")
			runGit(t, root, "merge", "--no-ff", "temporary-merge", "-m", "merge temporary change")
			write(t, root, "behavior.md", "behavior.md\n")
			commit(t, root, "restore merge change")
		}, nil},
		{"restore after retirement", nil, func(t *testing.T, root string) {
			writeLedger(t, root, "ship-widget", true)
			write(t, root, "intent.md", "- [x] Ship\n")
			commit(t, root, "temporary restoration")
			if err := os.RemoveAll(filepath.Join(root, ".changes/ship-widget")); err != nil {
				t.Fatal(err)
			}
			commit(t, root, "retire again")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := proposalRepository(t)
			runGit(t, root, "switch", "-c", "ship-widget", "main")
			writeLedger(t, root, "ship-widget", true)
			write(t, root, "intent.md", "- [ ] Ship\n")
			commit(t, root, "[baseline] ship-widget")
			baseline := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			if test.beforeCompletion != nil {
				test.beforeCompletion(t, root)
			}
			write(t, root, "intent.md", "- [x] Ship\n")
			commit(t, root, "[completion] ship-widget")
			completion := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			if err := os.RemoveAll(filepath.Join(root, ".changes/ship-widget")); err != nil {
				t.Fatal(err)
			}
			commit(t, root, "retire")
			if test.afterRetirement != nil {
				test.afterRetirement(t, root)
			}

			backend := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "ship-widget", State: workflow.AwaitingReview}}}
			got := implementCLI(t, root, backend, "inspect", "--item", "7")
			if got.Ledger.Baseline != baseline || got.Ledger.Completion != completion || got.Ledger.Phase != "retired" || len(got.Ledger.Violations) != 0 {
				t.Fatalf("restored history rejected: %#v", got.Ledger)
			}
		})
	}
}

func TestB8InspectAndStartBaselineOnlyImplementation(t *testing.T) {
	for _, progress := range []string{"published baseline", "claimed partial ticks", "claimed restored provisional contract"} {
		t.Run(progress, func(t *testing.T) {
			root := proposalRepository(t)
			runGit(t, root, "switch", "-c", "ship-widget", "main")
			writeLedger(t, root, "ship-widget", true)
			if progress != "published baseline" {
				if err := os.WriteFile(filepath.Join(root, ".changes/ship-widget/intent.md"), []byte("- [ ] First\n- [ ] Second\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			runGit(t, root, "add", ".changes/ship-widget")
			runGit(t, root, "commit", "-m", "[baseline] ship-widget")
			baseline := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			claimed := progress != "published baseline"
			if claimed {
				if progress == "claimed restored provisional contract" {
					if err := os.WriteFile(filepath.Join(root, ".changes/ship-widget/intent.md"), []byte("temporary prose\n"), 0o644); err != nil {
						t.Fatal(err)
					}
					runGit(t, root, "commit", "-am", "temporary edit")
				}
				if err := os.WriteFile(filepath.Join(root, ".changes/ship-widget/intent.md"), []byte("- [x] First\n- [ ] Second\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				runGit(t, root, "commit", "-am", "provisional progress")
			}
			head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			target := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "refs/remotes/origin/main"))
			item := workflow.ImplementationItem{ID: "7", Branch: "ship-widget", State: workflow.Ready, Claimed: claimed}
			if claimed {
				item.TargetBranch, item.TargetSnapshot = "main", target
			}
			backend := &implementationMemory{work: []workflow.ImplementationItem{item}, remoteHeads: map[string]string{"main": target}}

			inspected := implementCLI(t, root, backend, "inspect", "--item", "7")
			if inspected.Ledger == nil || inspected.Ledger.Baseline != baseline || inspected.Ledger.Completion != "" || inspected.Ledger.Phase != "present" || len(inspected.Ledger.Violations) != 0 {
				t.Fatalf("inspection = %#v", inspected)
			}
			operation := "next"
			args := []string{operation}
			if claimed {
				args = []string{"resume", "--item", "7"}
			}
			started := implementCLI(t, root, backend, args...)
			if started.Status != "work_available" || started.Packet == nil || started.Packet.Facts.Implementation.ArtifactBaseline != baseline || started.Packet.Facts.Implementation.ArtifactCompletion != "" {
				t.Fatalf("startup = %#v", started)
			}
			if !strings.Contains(started.Packet.Instructions, "Audit") || strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD")) != head {
				t.Fatalf("startup discarded progress or instructions: %s", started.Packet.Instructions)
			}
		})
	}
}

func TestB9SubmitOnlyACompletedRetiredContract(t *testing.T) {
	for _, test := range []struct {
		name, phase, want string
		rework            bool
	}{
		{"first pass retired", "retired", "awaiting_review", false},
		{"first pass baseline only", "baseline", "fix_required", false},
		{"first pass incomplete completion", "incomplete", "fix_required", false},
		{"first pass completion still present", "present", "fix_required", false},
		{"finding driven rework", "retired", "awaiting_review", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := proposalRepository(t)
			prepareSlice(t, root, "widget")
			state := workflow.Ready
			var submission *workflow.Submission
			if test.rework {
				head := completeAndRetireSlice(t, root, "widget")
				state = workflow.Rework
				submission = &workflow.Submission{ID: "42", Head: head}
			}
			backend := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: state, Claimed: true, TargetBranch: "main", TargetSnapshot: strings.TrimSpace(runGitOutput(t, root, "rev-parse", "main")), Submission: submission}}, remoteHeads: map[string]string{}}
			args := []string{"resume", "--item", "7"}
			if test.rework {
				args = append(args, "--reviewed-head", submission.Head)
			}
			start := implementCLI(t, root, backend, args...)
			if start.Packet == nil {
				t.Fatalf("startup = %#v", start)
			}
			body := filepath.Join(start.Packet.Facts.Implementation.ResultDirectory, "submission.md")
			if err := os.WriteFile(body, []byte("opaque audit\n"), 0600); err != nil {
				t.Fatal(err)
			}
			switch test.phase {
			case "retired":
				if test.rework {
					break
				}
				completeAndRetireSlice(t, root, "widget")
			case "incomplete":
				if err := os.WriteFile(filepath.Join(root, ".changes/widget/intent.md"), []byte("- [ ] unfinished\n"), 0644); err != nil {
					t.Fatal(err)
				}
				runGit(t, root, "commit", "-am", "[completion] widget")
				runGit(t, root, "rm", "-r", ".changes/widget")
				runGit(t, root, "commit", "-m", "retire widget")
			case "present":
				runGit(t, root, "commit", "--allow-empty", "-m", "[completion] widget")
			}
			backend.remoteHeads["widget"] = strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			got := implementCLI(t, root, backend, "submit", "--item", "7", "--body", body)
			if got.Status != test.want {
				t.Fatalf("submit = %#v, want %s", got, test.want)
			}
			if test.want == "fix_required" {
				if !backend.work[0].Claimed {
					t.Fatal("refusal released Claim")
				}
				if _, err := os.Stat(body); err != nil {
					t.Fatalf("refusal removed Result Document: %v", err)
				}
			} else if backend.work[0].Claimed || backend.work[0].Submission == nil || test.rework && backend.work[0].Submission.ID != "42" {
				t.Fatalf("successful handoff = %#v", backend.work[0])
			}
		})
	}

}

func TestImplementSubmitsCompletedFirstImplementation(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	backend := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Ready}}, remoteHeads: map[string]string{}}
	start := implementCLI(t, root, backend, "next")
	body := filepath.Join(start.Packet.Facts.Implementation.ResultDirectory, "submission.md")
	if err := os.WriteFile(body, []byte("opaque audit [not even Markdown\n"), 0600); err != nil {
		t.Fatal(err)
	}
	completeAndRetireSlice(t, root, "widget")
	backend.remoteHeads["widget"] = strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	got := implementCLI(t, root, backend, "submit", "--item", "7", "--body", body)
	if got.Status != "awaiting_review" || backend.work[0].Claimed || backend.work[0].State != workflow.AwaitingReview {
		t.Fatalf("submit: %#v %#v", got, backend.work)
	}
	submission := backend.work[0].Submission
	if submission == nil || submission.ID != "11" || submission.Head != backend.remoteHeads["widget"] || submission.Body != "opaque audit [not even Markdown\n\n\nCloses #7\n" {
		t.Fatalf("submission = %#v", submission)
	}
}

func TestImplementClaimsOldestEligibleWork(t *testing.T) {
	root := proposalRepository(t)
	backend := &implementationMemory{work: []workflow.ImplementationItem{
		{ID: "1", State: workflow.Ready, CreatedAt: "2020", Blockers: []workflow.WorkItemID{"9"}},
		{ID: "2", State: workflow.Ready, CreatedAt: "2021"},
		{ID: "3", State: workflow.Rework, CreatedAt: "2023"},
		{ID: "5", Order: 5, State: workflow.Rework, CreatedAt: "2022"},
		{ID: "4", Order: 4, State: workflow.Rework, CreatedAt: "2022"},
		{ID: "6", State: workflow.Rework, CreatedAt: "2010", Claimed: true},
		{ID: "7", State: workflow.NeedsHuman, CreatedAt: "2010"},
		{ID: "9", State: workflow.ReadyForMerge},
	}}
	for i := range backend.work {
		item := &backend.work[i]
		item.Branch = fmt.Sprintf("slice-%s", item.ID)
		prepareSlice(t, root, item.Branch)
		if item.State == workflow.Rework {
			completeAndRetireSlice(t, root, item.Branch)
			head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			number, err := strconv.Atoi(string(item.ID))
			if err != nil {
				t.Fatal(err)
			}
			item.Submission = &workflow.Submission{ID: workflow.SubmissionID(strconv.Itoa(number + 100)), Head: head, PreviousReviewedHead: head, Base: "main"}
		}
	}
	for _, want := range []int{4, 5, 3, 2} {
		got := implementCLI(t, root, backend, "next")
		if got.Status != "work_available" || got.Item.Number != want || !got.Item.Claimed {
			t.Fatalf("got %#v, want #%d", got, want)
		}
	}
	if backend.work[0].Claimed || backend.work[6].Claimed || backend.work[1].State != workflow.Ready {
		t.Fatalf("unexpected projections: %#v", backend.work)
	}
}

func TestImplementReportsNoEligibleWork(t *testing.T) {
	for _, work := range [][]workflow.ImplementationItem{nil, {
		{ID: "1", State: workflow.Ready, Blockers: []workflow.WorkItemID{"4"}},
		{ID: "2", State: workflow.Rework, Claimed: true},
		{ID: "3", State: workflow.NeedsHuman},
		{ID: "4", State: workflow.ReadyForMerge},
	}} {
		backend := &implementationMemory{work: work}
		before := append([]workflow.ImplementationItem(nil), work...)
		got := implementCLI(t, proposalRepository(t), backend, "next")
		if got.Status != "no_work" || got.Item != nil || !reflect.DeepEqual(before, backend.work) {
			t.Fatalf("no-work mutated projections: %#v %#v", got, backend.work)
		}
	}
}

func TestImplementAndWatchdogUseNumericTieBreak(t *testing.T) {
	for _, command := range []string{"implement", "watchdog"} {
		t.Run(command, func(t *testing.T) {
			root := proposalRepository(t)
			b := &implementationMemory{work: []workflow.ImplementationItem{
				{ID: "10", Order: 10, Branch: "ten", State: workflow.Ready, CreatedAt: "2026"},
				{ID: "2", Order: 2, Branch: "two", State: workflow.Ready, CreatedAt: "2026"},
			}}
			for i := range b.work {
				item := &b.work[i]
				prepareSlice(t, root, item.Branch)
				if command == "watchdog" {
					completeAndRetireSlice(t, root, item.Branch)
					item.State = workflow.AwaitingReview
					item.Submission = &workflow.Submission{ID: workflow.SubmissionID(item.ID), Head: strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD")), CreatedAt: "2026"}
				}
			}
			for _, want := range []int{2, 10} {
				var got setup.ImplementationOutput
				if command == "implement" {
					got = implementCLI(t, root, b, "next")
				} else {
					got = watchdogCLI(t, root, b, "next")
				}
				if got.Status != "work_available" || got.Item == nil || got.Item.Number != want || !got.Item.Claimed {
					t.Fatalf("numeric tie-break: %#v, want #%d", got, want)
				}
			}
		})
	}
}

func TestImplementLifecycleOrdersOpaqueIDsByBackendFact(t *testing.T) {
	root := proposalRepository(t)
	b := &implementationMemory{work: []workflow.ImplementationItem{
		{ID: "alpha", Order: 10, Branch: "ten", State: workflow.Ready, CreatedAt: "2026"},
		{ID: "zulu", Order: 2, Branch: "two", State: workflow.Ready, CreatedAt: "2026"},
	}, remoteHeads: map[string]string{"main": strings.TrimSpace(runGitOutput(t, root, "rev-parse", "main"))}}
	for _, item := range b.work {
		prepareSlice(t, root, item.Branch)
	}
	for _, want := range []workflow.WorkItemID{"zulu", "alpha"} {
		got, err := workflow.StartImplementation(context.Background(), root, "origin", "", "", "", workflow.ArtifactEndpoints{}, b)
		if err != nil || got.Status != "work_available" || got.Item == nil || got.Item.ID != want || !got.Item.Claimed {
			t.Fatalf("opaque implementation tie-break: %#v, %v; want %q", got, err, want)
		}
		t.Cleanup(func() { os.RemoveAll(got.Facts.Implementation.ResultDirectory) })
	}
	for i := range b.work {
		item := &b.work[i]
		runGit(t, root, "switch", item.Branch)
		completeAndRetireSlice(t, root, item.Branch)
		item.State, item.Claimed = workflow.AwaitingReview, false
		item.Source = &workflow.LifecycleObservation{Open: true}
		item.Submission = &workflow.Submission{ID: workflow.SubmissionID("review-" + item.ID), Head: strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD")), CreatedAt: "2026"}
		item.Submission.Lifecycle = &workflow.LifecycleObservation{Open: true, States: []workflow.State{workflow.AwaitingReview}}
	}
	runGit(t, root, "switch", "main")
	for _, item := range b.work {
		runGit(t, root, "worktree", "add", filepath.Join(root, ".worktrees", item.Branch), item.Branch)
	}
	for _, want := range []workflow.WorkItemID{"zulu", "alpha"} {
		got, err := workflow.StartWatchdog(context.Background(), root, "origin", "", workflow.ArtifactEndpoints{}, b)
		if err != nil || got.Status != "work_available" || got.Item == nil || got.Item.ID != want || !got.Item.Claimed || got.Item.Submission.ID != workflow.SubmissionID("review-"+want) {
			t.Fatalf("opaque review tie-break: %#v, %v; want %q", got, err, want)
		}
		t.Cleanup(func() { os.RemoveAll(got.Facts.Watchdog.ResultDirectory) })
	}
}

func TestImplementResumesInterruptedClaim(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	backend := &implementationMemory{work: []workflow.ImplementationItem{
		{ID: "1", State: workflow.Ready},
		{ID: "7", Branch: "widget", State: workflow.Ready, Claimed: true},
	}}
	got := implementCLI(t, root, backend, "resume", "--item", "7")
	if got.Status != "work_available" || got.Item.Number != 7 || !got.Item.Claimed || backend.work[0].Claimed {
		t.Fatalf("resume: %#v %#v", got, backend.work)
	}
}

func TestImplementResumePreservesTargetAndRejectsAmbiguousHistory(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	target := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "origin/main"))
	b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Ready, Claimed: true}}}
	first := implementCLI(t, root, b, "resume", "--item", "7")
	if first.Packet == nil || b.work[0].TargetSnapshot != target {
		t.Fatalf("resume did not persist target: %#v", first)
	}
	runGit(t, root, "commit", "--allow-empty", "-m", "implementation")
	runGit(t, root, "update-ref", "refs/remotes/origin/main", "HEAD")
	second := implementCLI(t, root, b, "resume", "--item", "7", "--target-snapshot", target)
	if second.Packet == nil || second.Packet.Facts.Implementation.TargetSnapshot != target {
		t.Fatalf("target moved: %#v", second)
	}
	wrong := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	if got := implementCLI(t, root, b, "resume", "--item", "7", "--target-snapshot", wrong); got.Status != "fix_required" {
		t.Fatalf("accepted changed obligation: %#v", got)
	}
	b.work[0].TargetSnapshot = ""
	if got := implementCLI(t, root, b, "resume", "--item", "7"); got.Status != "fix_required" {
		t.Fatalf("guessed after history changed: %#v", got)
	}
	b.work[0].Submission = &workflow.Submission{ID: "11", Draft: true, Base: "main"}
	b.work[0].Submission.Lifecycle = &workflow.LifecycleObservation{Open: true}
	got := implementCLI(t, root, b, "resume", "--item", "7", "--target-snapshot", target)
	if got.Packet == nil || got.Packet.Facts.Implementation.TargetSnapshot != target {
		t.Fatalf("draft suppressed target: %#v", got)
	}
}

func TestInstalledImplementActivationLoadsDefinitionsOnce(t *testing.T) {
	for _, harness := range []string{".pi/agent/skills", ".codex/skills", ".claude/skills"} {
		t.Run(harness, func(t *testing.T) {
			home := t.TempDir()
			root := proposalRepository(t)
			prepareSlice(t, root, "widget")
			b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Ready}}, remoteHeads: map[string]string{"main": strings.TrimSpace(runGitOutput(t, root, "rev-parse", "main"))}}
			var output bytes.Buffer
			app := newAppWithSkillHome(func(github.RepositoryID) (setup.Backend, error) { return b, nil }, bytes.NewReader(nil), &output, &output, home)
			if err := app.Run([]string{"skl", "install"}); err != nil {
				t.Fatal(err)
			}
			stub := readFile(t, filepath.Join(home, harness, "implement/SKILL.md"))
			_, rest, ok := strings.Cut(stub, "Run `")
			command, _, end := strings.Cut(rest, "`")
			if !ok || !end {
				t.Fatal("stub has no activation command")
			}
			instructions := ""
			for step := 0; step < 2; step++ {
				output.Reset()
				args := strings.Fields(command)
				if args[1] == "implement" {
					args = append(args, "--repo", root)
				}
				if err := app.Run(args); err != nil {
					t.Fatal(err)
				}
				if args[1] == "skill" {
					instructions += output.String()
					if !strings.Contains(output.String(), "`skl implement next`") {
						t.Fatal("generic packet has no Work Start command")
					}
					command = "skl implement next"
					continue
				}
				var outcome setup.ImplementationOutput
				if err := json.Unmarshal(output.Bytes(), &outcome); err != nil || outcome.Packet == nil {
					t.Fatalf("activation did not reach work: %s, %v", &output, err)
				}
				t.Cleanup(func() { os.RemoveAll(outcome.Packet.Facts.Implementation.ResultDirectory) })
				instructions += outcome.Packet.Instructions
				break
			}
			for _, name := range []string{"implement", "tdd", "audit", "design", "domain"} {
				definition := readRepositoryFile(t, "skills/dev/"+name+"/SKILL.md")
				if count := strings.Count(instructions, definition); count != 1 {
					t.Errorf("activation loaded %s %d times", name, count)
				}
			}
			if !b.work[0].Claimed {
				t.Fatal("activation did not claim work")
			}
		})
	}
}

func TestImplementUsesSelectedGitHubRemoteThroughout(t *testing.T) {
	for _, layout := range []string{"upstream only", "non-GitHub origin", "ambiguous", "explicit over origin"} {
		t.Run(layout, func(t *testing.T) {
			root := proposalRepository(t)
			baseline := prepareSlice(t, root, "widget")
			runGit(t, root, "remote", "rename", "origin", "upstream")
			if layout == "non-GitHub origin" {
				runGit(t, root, "remote", "add", "origin", "https://example.com/acme/widgets.git")
			}
			if layout == "ambiguous" || layout == "explicit over origin" {
				other := "fork"
				if layout == "explicit over origin" {
					other = "origin"
				}
				runGit(t, root, "remote", "add", other, "https://github.com/other/widgets.git")
			}
			b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Ready}}}
			if layout == "ambiguous" {
				var output bytes.Buffer
				app := newApp(func(github.RepositoryID) (setup.Backend, error) { return b, nil }, bytes.NewReader(nil), &output, &output)
				err := app.Run([]string{"skl", "implement", "next", "--repo", root})
				if err == nil || !strings.Contains(err.Error(), "--remote") || b.work[0].Claimed {
					t.Fatalf("ambiguous inference = %v, %s", err, &output)
				}
			}
			// Only the selected remote has the published branch available locally.
			runGit(t, root, "switch", "main")
			runGit(t, root, "branch", "-D", "widget")
			runGit(t, root, "update-ref", "refs/remotes/upstream/widget", baseline)
			args := []string{"next"}
			if layout == "ambiguous" || layout == "explicit over origin" {
				args = append(args, "--remote", "upstream")
			}
			start := implementCLI(t, root, b, args...)
			if start.Packet == nil || b.repository != (github.RepositoryID{Owner: "acme", Name: "widgets"}) {
				t.Fatalf("wrong remote: %#v, %#v", start, b.repository)
			}
			facts := start.Packet.Facts.Implementation
			if !strings.Contains(facts.ResumeCommand, "--remote 'upstream'") || !strings.Contains(facts.SubmitCommand, "--remote 'upstream'") {
				t.Fatalf("retry lost remote: %#v", facts)
			}
			runGit(t, root, "switch", "-c", "widget", "refs/remotes/upstream/widget")
			for _, operation := range []string{"resume", "inspect"} {
				got := implementCLI(t, root, b, operation, "--remote", "upstream", "--item", "7")
				if got.Item == nil || b.repository.Owner != "acme" {
					t.Fatalf("%s lost remote: %#v", operation, got)
				}
			}
			decision := filepath.Join(facts.ResultDirectory, "decision.md")
			if err := os.WriteFile(decision, []byte("human decision"), 0600); err != nil {
				t.Fatal(err)
			}
			got := implementCLI(t, root, b, "needs-human", "--remote", "upstream", "--item", "7", "--reason", "mandatory_rule", "--decision", decision)
			if got.Status != "needs_human" || b.repository.Owner != "acme" {
				t.Fatalf("pause lost remote: %#v", got)
			}
			b.work[0].State, b.work[0].Claimed = workflow.Ready, true
			b.work[0].Source = &workflow.LifecycleObservation{Open: true, States: []workflow.State{workflow.Ready}, Claimed: true}
			start = implementCLI(t, root, b, "resume", "--remote", "upstream", "--item", "7")
			body := filepath.Join(start.Packet.Facts.Implementation.ResultDirectory, "submission.md")
			if err := os.WriteFile(body, []byte("audit"), 0600); err != nil {
				t.Fatal(err)
			}
			completeAndRetireSlice(t, root, "widget")
			b.remoteHeads["widget"] = strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			got = implementCLI(t, root, b, "submit", "--remote", "upstream", "--item", "7", "--body", body)
			if got.Status != "awaiting_review" || b.repository.Owner != "acme" {
				t.Fatalf("submit lost remote: %#v", got)
			}
		})
	}
}

func TestImplementObservesLiveTargetAndKeepsPersistedSnapshot(t *testing.T) {
	for _, available := range []bool{true, false} {
		t.Run(fmt.Sprint(available), func(t *testing.T) {
			root := proposalRepository(t)
			prepareSlice(t, root, "widget")
			runGit(t, root, "switch", "main")
			runGit(t, root, "commit", "--allow-empty", "-m", "new backend target")
			target := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			runGit(t, root, "switch", "widget")
			if !available {
				target = strings.Repeat("f", 40)
			}
			b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Ready}}, remoteHeads: map[string]string{"main": target}}
			cached := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "origin/main"))
			if got := implementCLI(t, root, b, "next", "--target-snapshot", cached); got.Status != "fix_required" || b.work[0].Claimed {
				t.Fatalf("new Claim accepted caller-selected target: %#v", got)
			}
			got := implementCLI(t, root, b, "next")
			if !available {
				if got.Status != "fix_required" || !strings.Contains(got.Reason, target) || !strings.Contains(got.Reason, "fetch") || b.work[0].Claimed {
					t.Fatalf("unavailable live target: %#v", got)
				}
				return
			}
			if got.Packet == nil || got.Packet.Facts.Implementation.TargetSnapshot != target || b.work[0].TargetSnapshot != target {
				t.Fatalf("pinned stale target: %#v", got)
			}
			b.remoteHeads["main"] = strings.Repeat("f", 40)
			got = implementCLI(t, root, b, "resume", "--item", "7")
			if got.Packet == nil || got.Packet.Facts.Implementation.TargetSnapshot != target {
				t.Fatalf("persisted target moved: %#v", got)
			}
		})
	}
}

func TestImplementPinsTargetAndBundlesInstructions(t *testing.T) {
	root := proposalRepository(t)
	baseline := prepareSlice(t, root, "widget")
	target := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "origin/main"))
	backend := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Ready}}}
	got := implementCLI(t, root, backend, "next")
	if got.Packet == nil || got.Packet.Facts.Implementation.TargetSnapshot != target || got.Packet.Facts.Implementation.ArtifactBaseline != baseline {
		t.Fatalf("packet = %#v", got)
	}
	if got.Packet.Skill != "implement" || !reflect.DeepEqual(got.Packet.IncludedSkills, []string{"tdd", "audit", "design", "domain"}) {
		t.Fatalf("manifest = %#v", got.Packet)
	}
	if !strings.Contains(got.Packet.Markdown(), "git merge "+target) || !strings.Contains(got.Packet.Facts.Implementation.ResumeCommand, target) {
		t.Fatalf("missing concrete Git/retry facts: %s", got.Packet.Markdown())
	}
	_, markdown, found := strings.Cut(got.Packet.Markdown(), "\n\n## Work Start\n")
	if !found {
		t.Fatal("packet lacks Work Start instructions")
	}
	facts := got.Packet.Facts.Implementation
	golden := readRepositoryFile(t, "cmd/skl/testdata/implement-start.golden.md")
	normalized := strings.NewReplacer(facts.Worktree, "<worktree>", facts.ResultDirectory, "<result>", baseline, "<baseline>", target, "<target>").Replace("## Work Start\n" + markdown)
	if normalized != golden {
		t.Fatalf("Work Start Markdown differs from golden:\n%s", normalized)
	}
	if strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD")) != baseline {
		t.Fatal("Work Start changed Git")
	}
}

func TestImplementDispatchSuppliesRootBoundWorkerAndContinuationCommands(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	runGit(t, root, "remote", "rename", "origin", "upstream")
	b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Ready}}}

	start := implementCLI(t, root, b, "next", "--remote", "upstream")
	if start.Status != "work_available" || start.WorkerCommand == "" || start.ContinuationCommand == "" {
		t.Fatalf("dispatch commands: %#v", start)
	}
	physicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, wanted := range []string{"skl implement resume", "--item 7", "--repo '" + physicalRoot + "'", "--remote 'upstream'"} {
		if !strings.Contains(start.WorkerCommand, wanted) {
			t.Fatalf("worker command %q lacks %q", start.WorkerCommand, wanted)
		}
	}
	if !strings.Contains(start.ContinuationCommand, "skl implement next --after '") || !strings.Contains(start.ContinuationCommand, "--repo '") || !strings.Contains(start.ContinuationCommand, "--remote 'upstream'") {
		t.Fatalf("continuation command = %q", start.ContinuationCommand)
	}
	document := filepath.Join(start.Packet.Facts.Implementation.ResultDirectory, "decision.md")
	if err := os.WriteFile(document, []byte("preserve me"), 0600); err != nil {
		t.Fatal(err)
	}
	originalTarget := start.Packet.Facts.Implementation.TargetSnapshot
	runGit(t, root, "switch", "main")
	runGit(t, root, "commit", "--allow-empty", "-m", "target advanced after dispatch")
	runGit(t, root, "update-ref", "refs/remotes/upstream/main", "HEAD")
	b.remoteHeads["main"] = strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	runGit(t, root, "switch", "widget")

	resumed := returnedCLI(t, b, start.WorkerCommand)
	if resumed.Status != "work_available" || resumed.ContinuationCommand != start.ContinuationCommand || resumed.Packet.Facts.Implementation.ResultDirectory != start.Packet.Facts.Implementation.ResultDirectory || resumed.Packet.Facts.Implementation.TargetSnapshot != originalTarget {
		t.Fatalf("resume minted another dispatch: start=%#v resumed=%#v", start, resumed)
	}
	if data, err := os.ReadFile(document); err != nil || string(data) != "preserve me" {
		t.Fatalf("resume discarded Result Document: %q, %v", data, err)
	}
}

func TestImplementContinuationVerifiesHandoffBeforeSelectingAgain(t *testing.T) {
	root := proposalRepository(t)
	for _, branch := range []string{"first", "second"} {
		prepareSlice(t, root, branch)
	}
	b := &implementationMemory{work: []workflow.ImplementationItem{
		{ID: "7", Branch: "first", State: workflow.Ready, CreatedAt: "2020"},
		{ID: "8", Branch: "second", State: workflow.Ready, CreatedAt: "2021"},
	}, remoteHeads: map[string]string{}}

	start := implementCLI(t, root, b, "next")
	body := filepath.Join(start.Packet.Facts.Implementation.ResultDirectory, "submission.md")
	if err := os.WriteFile(body, []byte("opaque"), 0600); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "switch", "first")
	completeAndRetireSlice(t, root, "first")
	b.remoteHeads["first"] = strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	if got := implementCLI(t, root, b, "submit", "--item", "7", "--body", body); got.Status != "awaiting_review" {
		t.Fatalf("handoff: %#v", got)
	}
	parts := strings.Fields(start.ContinuationCommand)
	reference := strings.Trim(parts[4], "'")
	continued := implementCLI(t, root, b, "next", "--after", reference)
	if continued.Status != "work_available" || continued.Item.Number != 8 || continued.PreviousHandoff == nil || continued.PreviousHandoff.Number != 7 || continued.PreviousHandoff.Outcome != workflow.AwaitingReview {
		t.Fatalf("continuation: %#v", continued)
	}
}

func TestContinuationRefusesIncompleteOrWrongStageEvidence(t *testing.T) {
	for _, condition := range []string{"incomplete", "Claim still held", "wrong stage"} {
		t.Run(condition, func(t *testing.T) {
			root := proposalRepository(t)
			for _, branch := range []string{"first", "second"} {
				prepareSlice(t, root, branch)
			}
			b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "first", State: workflow.Ready, CreatedAt: "2020"}, {ID: "8", Branch: "second", State: workflow.Ready, CreatedAt: "2021"}}}
			start := implementCLI(t, root, b, "next")
			if condition != "incomplete" {
				round := b.rounds["7"][0]
				round.Outcome, round.Head = workflow.AwaitingReview, strings.Repeat("a", 40)
				if condition == "wrong stage" {
					round.Outcome, round.Released = workflow.ReadyForMerge, true
				}
				b.rounds["7"][0] = round
			}
			reference := strings.Trim(strings.Fields(start.ContinuationCommand)[4], "'")
			got := implementCLI(t, root, b, "next", "--after", reference, "--wait=1ms", "--poll=1ms")
			if got.Status != "fix_required" || got.Item == nil || got.Item.Number != 7 || b.work[1].Claimed {
				t.Fatalf("unsafe continuation: %#v %#v", got, b.work)
			}
		})
	}
}

func TestCompletedImplementationRoundSurvivesWatchdogAdvancement(t *testing.T) {
	root := proposalRepository(t)
	for _, branch := range []string{"first", "second"} {
		prepareSlice(t, root, branch)
	}
	b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "first", State: workflow.Ready, CreatedAt: "2020"}, {ID: "8", Branch: "second", State: workflow.Ready, CreatedAt: "2021"}}, remoteHeads: map[string]string{}}
	start := implementCLI(t, root, b, "next")
	body := filepath.Join(start.Packet.Facts.Implementation.ResultDirectory, "submission.md")
	os.WriteFile(body, []byte("opaque"), 0600)
	runGit(t, root, "switch", "first")
	completeAndRetireSlice(t, root, "first")
	b.remoteHeads["first"] = strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	implementCLI(t, root, b, "submit", "--item", "7", "--body", body)
	b.work[0].Claimed = true // The other lane advanced after the historical receipt.
	b.work[0].Submission.Claimed = true
	reference := strings.Trim(strings.Fields(start.ContinuationCommand)[4], "'")
	got := implementCLI(t, root, b, "next", "--after", reference)
	if got.Status != "work_available" || got.Item.Number != 8 || got.PreviousHandoff == nil || got.PreviousHandoff.Number != 7 || !b.work[0].Claimed {
		t.Fatalf("historical completion lost or later Claim disturbed: %#v %#v", got, b.work)
	}
}

func TestEarlierCompletionCannotAuthorizeLaterEqualHeadRound(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Ready}}, remoteHeads: map[string]string{}}
	first := implementCLI(t, root, b, "next")
	body := filepath.Join(first.Packet.Facts.Implementation.ResultDirectory, "submission.md")
	os.WriteFile(body, []byte("opaque"), 0600)
	completeAndRetireSlice(t, root, "widget")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	b.remoteHeads["widget"] = head
	implementCLI(t, root, b, "submit", "--item", "7", "--body", body)
	b.work[0].State = workflow.Rework
	b.work[0].Claimed = false
	b.work[0].Source.States = nil
	b.work[0].Source.Claimed = false
	b.work[0].Submission.Head = head
	b.work[0].Submission.PreviousReviewedHead = head
	b.work[0].Submission.Lifecycle.States = []workflow.State{workflow.Rework}
	b.work[0].Submission.Lifecycle.Claimed = false
	second := implementCLI(t, root, b, "next")
	if second.ContinuationCommand == "" {
		t.Fatalf("second dispatch = %#v", second)
	}
	reference := strings.Trim(strings.Fields(second.ContinuationCommand)[4], "'")
	got := implementCLI(t, root, b, "next", "--after", reference)
	if got.Status != "fix_required" || got.PreviousHandoff != nil || !b.work[0].Claimed {
		t.Fatalf("earlier equal-head receipt authorized later round: %#v %#v", got, b.rounds["7"])
	}
}

func TestImplementationCompletionRejectsChangedRoundObligation(t *testing.T) {
	root := proposalRepository(t)
	baseline := prepareSlice(t, root, "widget")
	completeAndRetireSlice(t, root, "widget")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Rework, Submission: &workflow.Submission{ID: "11", Head: head, PreviousReviewedHead: head}}}, remoteHeads: map[string]string{"widget": head}}
	start := implementCLI(t, root, b, "next")
	body := filepath.Join(start.Packet.Facts.Implementation.ResultDirectory, "submission.md")
	os.WriteFile(body, []byte("opaque"), 0600)
	b.work[0].Submission.PreviousReviewedHead = baseline
	got := implementCLI(t, root, b, "submit", "--item", "7", "--body", body)
	if got.Status != "fix_required" || !strings.Contains(got.Reason, "active dispatch contradicts") {
		t.Fatalf("changed reviewed obligation completed old round: %#v", got)
	}
}

func TestContinuationReferencesFailBeforeSelection(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Ready}, {ID: "8", Branch: "widget", State: workflow.Ready}}}
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) { return b, nil }, bytes.NewReader(nil), &output, &output)
	if err := app.Run([]string{"skl", "implement", "next", "--repo", root, "--after", "not-a-reference"}); err == nil || b.work[0].Claimed {
		t.Fatalf("malformed reference mutated selection: %v %#v", err, b.work)
	}

	start := implementCLI(t, root, b, "next")
	before, _ := json.Marshal([]any{b.work, b.rounds})
	defer func() {
		after, _ := json.Marshal([]any{b.work, b.rounds})
		if !bytes.Equal(before, after) {
			t.Fatalf("invalid references mutated work: %s", after)
		}
	}()
	reference := strings.Trim(strings.Fields(start.ContinuationCommand)[4], "'")
	round := b.rounds["7"][0]
	b.rounds["7"] = nil
	got := implementCLI(t, root, b, "next", "--after", reference)
	if got.Status != "fix_required" || got.Item == nil || got.Item.Number != 7 {
		t.Fatalf("unknown round = %#v", got)
	}
	b.rounds["7"] = []workflow.DispatchRound{round}
	if got := watchdogCLI(t, root, b, "next", "--after", reference); got.Status != "fix_required" || !strings.Contains(got.Reason, "another workflow lane") {
		t.Fatalf("cross-lane reference = %#v", got)
	}
	decoded, _ := base64.RawURLEncoding.DecodeString(reference)
	var binding map[string]any
	json.Unmarshal(decoded, &binding)
	binding["submission"] = "11"
	changed, _ := json.Marshal(binding)
	if got := implementCLI(t, root, b, "next", "--after", base64.RawURLEncoding.EncodeToString(changed)); got.Status != "fix_required" || !strings.Contains(got.Reason, "binding") {
		t.Fatalf("contradictory Submission binding = %#v", got)
	}
	runGit(t, root, "remote", "set-url", "origin", "https://github.com/other/widgets.git")
	if got := implementCLI(t, root, b, "next", "--after", reference); got.Status != "fix_required" || !strings.Contains(got.Reason, "another repository") {
		t.Fatalf("cross-repository reference = %#v", got)
	}
}

func TestContinuationUsesFreshGitHubRoundEvidence(t *testing.T) {
	comments := make(map[int][]map[string]any)
	active := make(map[int]bool)
	pages := make(map[int]int)
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/repos/acme/widgets")
		var number int
		if _, err := fmt.Sscanf(path, "/issues/%d/comments", &number); err == nil {
			if r.Method == http.MethodPost {
				writes++
				var payload map[string]any
				json.NewDecoder(r.Body).Decode(&payload)
				payload["author_association"] = "OWNER"
				comments[number] = append(comments[number], payload)
			}
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			if page < 1 {
				page = 1
			}
			pages[page]++
			start := min((page-1)*100, len(comments[number]))
			end := min(start+100, len(comments[number]))
			json.NewEncoder(w).Encode(comments[number][start:end])
			return
		}
		switch path {
		case "/issues":
			labels := `[{"name":"needs-human"}]`
			if active[7] {
				labels = `[{"name":"ready"},{"name":"wip"}]`
			}
			fmt.Fprintf(w, `[{"number":7,"title":"first","state":"open","labels":%s},{"number":8,"title":"second","state":"open","labels":[]}]`, labels)
		case "/pulls":
			labels := `[{"name":"done"}]`
			if active[8] {
				labels = `[{"name":"review"},{"name":"wip"}]`
			}
			fmt.Fprintf(w, `[{"number":11,"state":"open","labels":%s,"head":{"ref":"second","sha":"head","repo":{"full_name":"acme/widgets"}},"base":{"ref":"main"}}]`, labels)
		case "/issues/7/dependencies/blocked_by", "/pulls/11/comments", "/pulls/11/reviews", "/issues/11/timeline":
			fmt.Fprint(w, `[]`)
		case "/pulls/11":
			fmt.Fprint(w, `{"number":11,"state":"open","head":{"sha":"head"},"base":{"ref":"main"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	root := proposalRepository(t)
	for _, tc := range []struct {
		lane       string
		item       workflow.WorkItemID
		submission workflow.SubmissionID
		outcome    workflow.State
	}{{"implement", "7", "", workflow.NeedsHuman}, {"watchdog", "8", "11", workflow.ReadyForMerge}} {
		backend := setup.NewGitHubBackend(server.URL, "token", server.Client())
		backend.BindRepository(github.RepositoryID{Owner: "acme", Name: "widgets"})
		round := workflow.DispatchRound{ID: "durable-" + tc.lane, Lane: workflow.DispatchLane(tc.lane), Item: tc.item, Submission: tc.submission, Obligation: "head", Directory: "skl-" + tc.lane + "-fixed"}
		if err := backend.RecordDispatchRound(t.Context(), round); err != nil {
			t.Fatal(err)
		}
		number, _ := strconv.Atoi(string(tc.item))
		for range 105 {
			comments[number] = append(comments[number], map[string]any{"body": "unrelated human comment", "author_association": "OWNER"})
		}
		round.Outcome, round.Head, round.Released = tc.outcome, "head", true
		if err := backend.RecordDispatchRound(t.Context(), round); err != nil {
			t.Fatal(err)
		}
		comments[number] = append(comments[number], map[string]any{"body": "<!-- skl.implement/v1\n{\"target_branch\":\"main\"}\n-->", "author_association": "OWNER"})
		later := round
		later.ID += "-later"
		later.Directory += "-later"
		later.Outcome, later.Head, later.Released = "", "", false
		if err := backend.RecordDispatchRound(t.Context(), later); err != nil {
			t.Fatal(err)
		}
		active[number] = true
		beforeWrites := writes
		before, _ := json.Marshal(comments)
		pages = make(map[int]int)
		referenceJSON, _ := json.Marshal(map[string]any{"v": 1, "owner": "acme", "repository": "widgets", "lane": tc.lane, "item": tc.item, "round": round.ID, "submission": tc.submission})
		reference := base64.RawURLEncoding.EncodeToString(referenceJSON)
		var output bytes.Buffer
		app := newApp(func(repository github.RepositoryID) (setup.Backend, error) {
			backend := setup.NewGitHubBackend(server.URL, "token", server.Client())
			backend.BindRepository(repository)
			return backend, nil
		}, bytes.NewReader(nil), &output, &output)
		if err := app.Run([]string{"skl", tc.lane, "next", "--repo", root, "--after", reference}); err != nil {
			t.Fatal(err)
		}
		var got setup.ImplementationOutput
		want, _ := strconv.Atoi(string(tc.item))
		if err := json.Unmarshal(output.Bytes(), &got); err != nil || got.Status != "no_work" || got.PreviousHandoff == nil || got.PreviousHandoff.Number != want || got.PreviousHandoff.Outcome != tc.outcome {
			t.Fatalf("fresh %s HTTP continuation = %#v, %v", tc.lane, got, err)
		}
		if pages[2] == 0 || writes != beforeWrites {
			t.Fatalf("historical continuation did not read pagination without mutation: pages=%v writes=%d/%d", pages, beforeWrites, writes)
		}
		laterJSON, _ := json.Marshal(map[string]any{"v": 1, "owner": "acme", "repository": "widgets", "lane": tc.lane, "item": tc.item, "round": later.ID, "submission": tc.submission})
		pages = make(map[int]int)
		fresh := setup.NewGitHubBackend(server.URL, "token", server.Client())
		refused := returnedCLI(t, fresh, "skl "+tc.lane+" next --repo '"+root+"' --after '"+base64.RawURLEncoding.EncodeToString(laterJSON)+"' --wait=1ms")
		after, _ := json.Marshal(comments)
		if refused.Status != "fix_required" || refused.Item == nil || refused.Item.Number != number || refused.PreviousHandoff != nil || pages[2] == 0 || writes != beforeWrites || !bytes.Equal(before, after) || !active[number] {
			t.Fatalf("earlier equal-head receipt authorized later round or mutated history: %+v pages=%v", refused, pages)
		}

	}
}

func TestImplementStartsFindingDrivenRework(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	completeAndRetireSlice(t, root, "widget")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	comments := []skilldist.ReviewComment{{Body: "W1 BLOCK evidence", Author: "reviewer"}, {Body: "W1 NOTE reason", Author: "owner", Association: "OWNER"}}
	backend := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Rework, Submission: &workflow.Submission{ID: "11", Head: head, PreviousReviewedHead: head, Comments: comments}}}}
	got := implementCLI(t, root, backend, "next")
	if got.Status != "work_available" || got.Packet == nil {
		t.Fatalf("rework = %#v", got)
	}
	facts := got.Packet.Facts.Implementation
	if facts.Submission != 11 || !reflect.DeepEqual(facts.Comments, comments) || facts.TargetSnapshot != "" {
		t.Fatalf("facts = %#v", facts)
	}
	if !strings.Contains(got.Packet.Markdown(), "current PR comparison") || strings.Contains(got.Packet.Markdown(), "git merge ") || strings.Contains(got.Packet.Markdown(), head+"...HEAD") {
		t.Fatal("rework packet synchronizes target or requires a previous review cache")
	}
}

func TestImplementResubmitsExistingRework(t *testing.T) {
	for _, hasSubmission := range []bool{true, false} {
		root := proposalRepository(t)
		prepareSlice(t, root, "widget")

		completeAndRetireSlice(t, root, "widget")
		head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
		backend := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Rework}}, remoteHeads: map[string]string{"widget": head}}
		var directory string
		if hasSubmission {
			backend.work[0].Submission = &workflow.Submission{ID: "42", Head: head, PreviousReviewedHead: head}
			directory = implementCLI(t, root, backend, "next").Packet.Facts.Implementation.ResultDirectory
		} else {
			var err error
			directory, err = os.MkdirTemp("", "skl-implement-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.RemoveAll(directory) })
			if err := os.WriteFile(filepath.Join(directory, ".skl-result"), []byte("skl.implement/v1\n"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		body := filepath.Join(directory, "submission.md")
		if err := os.WriteFile(body, []byte("current Audit and rework dispositions\n"), 0600); err != nil {
			t.Fatal(err)

		}
		got := implementCLI(t, root, backend, "submit", "--item", "7", "--body", body)
		if hasSubmission {
			if got.Status != "awaiting_review" || backend.work[0].Submission.ID != "42" {
				t.Fatalf("rework = %#v", got)
			}
		} else if got.Status != "fix_required" || backend.work[0].Submission != nil {
			t.Fatalf("rework created new PR: %#v", got)
		}
	}
}

func TestImplementPausesBeforeCodeExists(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	backend := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Ready}}}
	start := implementCLI(t, root, backend, "next")
	decision := filepath.Join(start.Packet.Facts.Implementation.ResultDirectory, "decision.md")
	if err := os.WriteFile(decision, []byte("contradiction and recommendation\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got := implementCLI(t, root, backend, "needs-human", "--item", "7", "--reason", "contradictory_artifacts", "--decision", decision)
	if got.Status != "needs_human" || backend.work[0].Claimed || backend.work[0].ResumeState != workflow.Ready || backend.work[0].Submission != nil || backend.decisions["7"] != "contradiction and recommendation\n" {
		t.Fatalf("pause: %#v %#v", got, backend)
	}
	if _, err := os.Stat(filepath.Join(root, ".changes/widget/intent.md")); err != nil {
		t.Fatal("pause retired incomplete ledger")
	}
	reference := strings.Trim(strings.Fields(start.ContinuationCommand)[4], "'")
	continued := implementCLI(t, root, backend, "next", "--after", reference)
	if continued.Status != "no_work" || continued.PreviousHandoff == nil || continued.PreviousHandoff.Number != 7 || continued.PreviousHandoff.Outcome != workflow.NeedsHuman {
		t.Fatalf("Needs Human without Submission was not verified: %#v", continued)
	}
}

func TestB10PreserveIncompleteWorkInNeedsHuman(t *testing.T) {
	const unfinished = "# Intent\n- [ ] Implement widget\n- [ ] Verify widget automatically\n"
	for _, test := range []struct {
		name, preserved string
		body, existing  bool
		want            string
		baseline        func(*testing.T, string)
	}{
		{"baseline only", "none", false, false, "needs_human", nil},
		{"pushed partial implementation", "partial", true, false, "needs_human", nil},
		{"existing draft Submission", "partial", true, true, "needs_human", nil},
		{"ambiguous baseline", "ambiguous", false, false, "fix_required", nil},
		{"unordered Completion", "unordered", false, false, "fix_required", nil},
		{"baseline missing ledger directory", "malformed", true, false, "fix_required", func(t *testing.T, root string) {
			runGit(t, root, "switch", "-c", "widget", "main")
			runGit(t, root, "commit", "--allow-empty", "-m", "[baseline] widget")
		}},
		{"baseline ledger root is a file", "malformed", true, false, "fix_required", func(t *testing.T, root string) {
			runGit(t, root, "switch", "-c", "widget", "main")
			if err := os.MkdirAll(filepath.Join(root, ".changes"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, ".changes/widget"), []byte("not a directory\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			runGit(t, root, "add", ".changes/widget")
			runGit(t, root, "commit", "-m", "[baseline] widget")
		}},
		{"baseline artifact has executable mode", "malformed", true, false, "fix_required", func(t *testing.T, root string) {
			runGit(t, root, "switch", "-c", "widget", "main")
			writeLedger(t, root, "widget", true)
			if err := os.Chmod(filepath.Join(root, ".changes/widget/intent.md"), 0o755); err != nil {
				t.Fatal(err)
			}
			runGit(t, root, "add", ".changes/widget")
			runGit(t, root, "commit", "-m", "[baseline] widget")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := proposalRepository(t)
			if test.want == "needs_human" {
				runGit(t, root, "switch", "-c", "widget", "main")
				writeLedger(t, root, "widget", true)
				if err := os.WriteFile(filepath.Join(root, ".changes/widget/intent.md"), []byte(unfinished), 0o644); err != nil {
					t.Fatal(err)
				}
				runGit(t, root, "add", ".changes/widget")
				runGit(t, root, "commit", "-m", "[baseline] widget")
				runGit(t, root, "update-ref", "refs/remotes/origin/widget", "HEAD")
			} else if test.baseline == nil {
				prepareSlice(t, root, "widget")
			} else {
				test.baseline(t, root)
			}
			baseline := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			if test.preserved == "ambiguous" {
				runGit(t, root, "commit", "--allow-empty", "-m", "[baseline] widget duplicate")
			}
			if test.preserved == "unordered" {
				runGit(t, root, "switch", "-c", "completion-side", "main")
				writeLedger(t, root, "widget", true)
				runGit(t, root, "add", ".changes/widget")
				runGit(t, root, "commit", "-m", "[completion] widget")
				completion := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
				runGit(t, root, "switch", "widget")
				runGit(t, root, "merge", "--no-ff", "completion-side", "-m", "retain completion evidence")
				if gitOK := runGitOutput(t, root, "rev-parse", completion); strings.TrimSpace(gitOK) == baseline {
					t.Fatal("fixture endpoints unexpectedly equal")
				}
			}
			item := workflow.ImplementationItem{ID: "7", Branch: "widget", State: workflow.Ready, Claimed: true, TargetBranch: "main", TargetSnapshot: strings.TrimSpace(runGitOutput(t, root, "rev-parse", "main"))}
			if test.existing {
				item.Submission = &workflow.Submission{ID: "42", Draft: true, Head: baseline, Body: "previous draft\n"}
			}
			backend := &implementationMemory{work: []workflow.ImplementationItem{item}, remoteHeads: map[string]string{}}
			start := implementCLI(t, root, backend, "resume", "--item", "7")
			if start.Status == "fix_required" {
				start = setup.ImplementationOutput{}
				start.Packet = &skilldist.Packet{Facts: skilldist.InvocationFacts{Implementation: &skilldist.ImplementationFacts{ResultDirectory: newImplementationResultDirectory(t)}}}
			}
			directory := start.Packet.Facts.Implementation.ResultDirectory
			decision := filepath.Join(directory, "decision.md")
			if err := os.WriteFile(decision, []byte("human decision\n"), 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{"needs-human", "--item", "7", "--reason", "mandatory_rule", "--decision", decision}
			if test.body {
				if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("partial implementation\n"), 0644); err != nil {
					t.Fatal(err)
				}
				runGit(t, root, "commit", "-am", "partial implementation")
				body := filepath.Join(directory, "submission.md")
				if err := os.WriteFile(body, []byte("preserve work\n"), 0600); err != nil {
					t.Fatal(err)
				}
				args = append(args, "--body", body)
				backend.remoteHeads["widget"] = strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			}
			head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			got := implementCLI(t, root, backend, args...)
			if got.Status != test.want {
				t.Fatalf("pause = %#v, want %s", got, test.want)
			}
			if test.want == "needs_human" {
				paused := backend.work[0]
				if got.Item == nil || got.Item.ResumeState != item.State || got.Item.Claimed || paused.State != workflow.NeedsHuman || paused.ResumeState != item.State || paused.Claimed || backend.decisions["7"] != "human decision\n" {
					t.Fatalf("preservation = %#v, work = %#v", got, paused)
				}
				if !test.body {
					if got.Item.Submission != nil || paused.Submission != nil {
						t.Fatalf("no-body pause created a Submission: %#v, work = %#v", got.Item, paused)
					}
				} else if got.Item.Submission == nil || !got.Item.Submission.Draft || got.Item.Submission.Head != head || paused.Submission == nil || !paused.Submission.Draft || paused.Submission.Head != head || backend.remoteHeads["widget"] != head || paused.Submission.Body != "preserve work\n\n\nCloses #7\n" || test.existing && (got.Item.Submission.Number != 42 || paused.Submission.ID != "42") {
					t.Fatalf("draft not preserved at pushed head %s: %#v, work = %#v", head, got.Item, paused)
				}
				if current := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD")); current != head {
					t.Fatalf("pause moved head: %s, want %s", current, head)
				}
				for name, want := range map[string]string{"intent.md": unfinished, "behavior.md": "behavior.md\n"} {
					path := ".changes/widget/" + name
					for _, ref := range []string{baseline, "HEAD"} {
						if contents := runGitOutput(t, root, "show", ref+":"+path); contents != want {
							t.Fatalf("%s at %s changed unfinished artifacts: %q, want %q", path, ref, contents, want)
						}
					}
					if contents := readFile(t, filepath.Join(root, path)); contents != want {
						t.Fatalf("working tree %s changed unfinished artifacts: %q, want %q", path, contents, want)
					}
				}
				inspected := implementCLI(t, root, backend, "inspect", "--item", "7")
				if inspected.Status != "inspected" || inspected.Head != head || inspected.Ledger == nil || inspected.Ledger.Baseline != baseline || inspected.Ledger.Completion != "" || inspected.Ledger.Phase != "present" || len(inspected.Ledger.Violations) != 0 {
					t.Fatalf("pause lost incomplete ledger or invented Completion: %#v", inspected)
				}
			}
			if test.want == "fix_required" && (!backend.work[0].Claimed || backend.work[0].State != workflow.Ready) {
				t.Fatalf("refusal mutated work = %#v", backend.work[0])
			}
		})
	}
}

func TestB12ValidateExplicitMarkerlessEndpointSHAsWithoutAdoptionState(t *testing.T) {
	type fixture struct {
		root, baseline, completion, head string
	}
	markerless := func(t *testing.T, completionMarker, retired bool) fixture {
		t.Helper()
		root := proposalRepository(t)
		runGit(t, root, "switch", "-c", "widget", "main")
		writeLedger(t, root, "widget", true)
		runGit(t, root, "add", ".changes/widget")
		runGit(t, root, "commit", "-m", "legacy baseline")
		baseline := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
		if !retired {
			return fixture{root, baseline, "", baseline}
		}
		message := "legacy completion"
		if completionMarker {
			message = "[completion] widget"
		}
		runGit(t, root, "commit", "--allow-empty", "-m", message)
		completion := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
		runGit(t, root, "rm", "-r", ".changes/widget")
		runGit(t, root, "commit", "-m", "legacy retirement")
		return fixture{root, baseline, completion, strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))}
	}
	for _, test := range []struct {
		name string
		run  func(*testing.T) (setup.ImplementationOutput, *implementationMemory)
		want string
	}{
		{"markerless baseline-only implementation", func(t *testing.T) (setup.ImplementationOutput, *implementationMemory) {
			f := markerless(t, false, false)
			b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Ready}}}
			return implementCLI(t, f.root, b, "inspect", "--item", "7", "--artifact-baseline", f.baseline), b
		}, "valid present"},
		{"markerless retired pair", func(t *testing.T) (setup.ImplementationOutput, *implementationMemory) {
			f := markerless(t, false, true)
			b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.AwaitingReview}}}
			return implementCLI(t, f.root, b, "inspect", "--item", "7", "--artifact-baseline", f.baseline, "--artifact-completion", f.completion), b
		}, "valid retired"},
		{"supplied baseline with marked completion", func(t *testing.T) (setup.ImplementationOutput, *implementationMemory) {
			f := markerless(t, true, true)
			b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.AwaitingReview}}}
			return implementCLI(t, f.root, b, "inspect", "--item", "7", "--artifact-baseline", f.baseline), b
		}, "valid retired"},
		{"markerless review misses Completion", func(t *testing.T) (setup.ImplementationOutput, *implementationMemory) {
			f := markerless(t, false, true)
			b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.AwaitingReview, Submission: &workflow.Submission{ID: "11", Head: f.head}}}}
			return watchdogCLI(t, f.root, b, "next", "--artifact-baseline", f.baseline), b
		}, "fix_required"},
		{"invalid explicit identities", func(t *testing.T) (setup.ImplementationOutput, *implementationMemory) {
			f := markerless(t, false, true)
			invalid := []string{"widget", f.baseline[:12], strings.TrimSpace(runGitOutput(t, f.root, "rev-parse", f.baseline+"^{tree}")), strings.Repeat("f", 40)}
			b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Ready}}}
			for _, endpoint := range invalid {
				got := implementCLI(t, f.root, b, "inspect", "--item", "7", "--artifact-baseline", endpoint)
				if got.Ledger == nil || !strings.Contains(strings.Join(got.Ledger.Violations, "\n"), "full commit SHA") && !strings.Contains(strings.Join(got.Ledger.Violations, "\n"), "commit object") {
					t.Fatalf("accepted invalid endpoint %q: %#v", endpoint, got)
				}
			}
			return implementCLI(t, f.root, b, "inspect", "--item", "7", "--artifact-baseline", invalid[0]), b
		}, "full commit SHA"},
		{"unreachable explicit baseline", func(t *testing.T) (setup.ImplementationOutput, *implementationMemory) {
			root := proposalRepository(t)
			runGit(t, root, "switch", "-c", "unreachable", "main")
			writeLedger(t, root, "widget", true)
			runGit(t, root, "add", ".changes/widget")
			runGit(t, root, "commit", "-m", "legacy baseline")
			baseline := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			runGit(t, root, "switch", "-c", "widget", "main")
			b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Ready}}}
			return implementCLI(t, root, b, "inspect", "--item", "7", "--artifact-baseline", baseline), b
		}, "not reachable"},
		{"invalid explicit endpoint content", func(t *testing.T) (setup.ImplementationOutput, *implementationMemory) {
			f := markerless(t, false, false)
			if err := os.WriteFile(filepath.Join(f.root, ".changes/widget/behavior.md"), []byte("changed\n"), 0644); err != nil {
				t.Fatal(err)
			}
			runGit(t, f.root, "commit", "-am", "bad legacy completion")
			bad := strings.TrimSpace(runGitOutput(t, f.root, "rev-parse", "HEAD"))
			runGit(t, f.root, "rm", "-r", ".changes/widget")
			runGit(t, f.root, "commit", "-m", "retire bad contract")
			b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.AwaitingReview}}}
			return implementCLI(t, f.root, b, "inspect", "--item", "7", "--artifact-baseline", f.baseline, "--artifact-completion", bad), b
		}, "content changed"},
		{"ambiguous markers defeat explicit input", func(t *testing.T) (setup.ImplementationOutput, *implementationMemory) {
			root := proposalRepository(t)
			prepareSlice(t, root, "widget")
			first := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			runGit(t, root, "commit", "--allow-empty", "-m", "[baseline] widget duplicate")
			b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Ready}}}
			return implementCLI(t, root, b, "inspect", "--item", "7", "--artifact-baseline", first), b
		}, "ambiguous"},
		{"unique marker is authoritative", func(t *testing.T) (setup.ImplementationOutput, *implementationMemory) {
			root := proposalRepository(t)
			baseline := prepareSlice(t, root, "widget")
			runGit(t, root, "commit", "--allow-empty", "-m", "other commit")
			other := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Ready}}}
			if baseline == other {
				t.Fatal("fixture did not create distinct commit")
			}
			return implementCLI(t, root, b, "inspect", "--item", "7", "--artifact-baseline", other), b
		}, "authoritative"},
		{"markerless evidence without inputs", func(t *testing.T) (setup.ImplementationOutput, *implementationMemory) {
			f := markerless(t, false, true)
			b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Ready}}}
			return implementCLI(t, f.root, b, "next"), b
		}, "missing"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, backend := test.run(t)
			detail := got.Reason
			if got.Ledger != nil {
				detail += strings.Join(got.Ledger.Violations, "\n")
			}
			if strings.HasPrefix(test.want, "valid ") {
				phase := strings.TrimPrefix(test.want, "valid ")
				if got.Status != "inspected" || got.Ledger == nil || got.Ledger.Phase != phase || len(got.Ledger.Violations) != 0 {
					t.Fatalf("explicit evidence = %#v", got)
				}
			} else if test.want == "fix_required" {
				if got.Status != test.want || backend.work[0].Claimed {
					t.Fatalf("review accepted incomplete evidence: %#v", got)
				}
			} else if !strings.Contains(strings.ToLower(detail), strings.ToLower(test.want)) {
				t.Fatalf("result = %#v, want %q", got, test.want)
			}
		})
	}

	root := proposalRepository(t)
	for _, args := range [][]string{{"skl", "status", "--artifact-baseline", strings.Repeat("a", 40)}, {"skl", "setup", "--artifact-completion", strings.Repeat("b", 40)}, {"skl", "skill", "--artifact-baseline", strings.Repeat("c", 40)}} {
		app := newApp(func(github.RepositoryID) (setup.Backend, error) { return &implementationMemory{}, nil }, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		args = append(args, "--repo", root)
		if err := app.Run(args); err == nil {
			t.Fatalf("unrelated operation accepted endpoint flag: %v", args)
		}
	}
}

func TestImplementPausesPreservingWork(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	backend := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Ready}}, remoteHeads: map[string]string{}}
	start := implementCLI(t, root, backend, "next")
	directory := start.Packet.Facts.Implementation.ResultDirectory
	for _, name := range []string{"submission.md", "decision.md"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(name+" opaque\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("completed work\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "commit", "-am", "implement")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	backend.remoteHeads["widget"] = head
	got := implementCLI(t, root, backend, "needs-human", "--item", "7", "--reason", "mandatory_rule", "--decision", filepath.Join(directory, "decision.md"), "--body", filepath.Join(directory, "submission.md"))
	if got.Status != "needs_human" || got.Item.Submission == nil || !got.Item.Submission.Draft || got.Item.Submission.Head != head || got.Item.Claimed || got.Item.ResumeState != workflow.Ready {
		t.Fatalf("draft pause: %#v", got)
	}
	if _, err := os.Stat(filepath.Join(root, ".changes/widget/intent.md")); err != nil {
		t.Fatal("draft retired incomplete artifacts")
	}
	reference := strings.Trim(strings.Fields(start.ContinuationCommand)[4], "'")
	continued := implementCLI(t, root, backend, "next", "--after", reference)
	if continued.Status != "no_work" || continued.PreviousHandoff == nil || continued.PreviousHandoff.Outcome != workflow.NeedsHuman {
		t.Fatalf("draft Needs Human continuation: %#v", continued)
	}
}

func TestImplementPublishesOpaqueResultAndCleansSuccessfulDirectory(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	backend := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Ready}}, remoteHeads: map[string]string{}}
	start := implementCLI(t, root, backend, "next")
	directory := start.Packet.Facts.Implementation.ResultDirectory
	body := filepath.Join(directory, "submission.md")
	prose := "\x00[ broken Markdown\nVerdict: needs-human\n\n\n"
	if err := os.WriteFile(body, []byte(prose), 0600); err != nil {
		t.Fatal(err)
	}
	completeAndRetireSlice(t, root, "widget")
	backend.remoteHeads["widget"] = strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	got := implementCLI(t, root, backend, "submit", "--item", "7", "--body", body)
	if got.Status != "awaiting_review" || backend.work[0].Submission.Body != prose+"\n\nCloses #7\n" {
		t.Fatalf("opaque publication = %#v", got)
	}
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatalf("successful operation remains: %v", err)
	}
}

func TestImplementResumesConventionalWorktreeWithoutSelectingAnotherItem(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	runGit(t, root, "switch", "main")
	worktree := filepath.Join(root, ".worktrees", "widget")
	runGit(t, root, "worktree", "add", worktree, "widget")
	backend := &implementationMemory{work: []workflow.ImplementationItem{{ID: "1", Branch: "other", State: workflow.NeedsHuman}, {ID: "7", Branch: "widget", State: workflow.Ready}}}
	start := implementCLI(t, root, backend, "next")
	got := returnedCLI(t, backend, start.WorkerCommand)
	if got.Status != "work_available" || got.Item.Number != 7 || backend.work[0].Claimed {
		t.Fatalf("resume = %#v", got)
	}
	got = implementCLI(t, root, backend, "resume")
	if got.Status != "fix_required" || backend.work[0].Claimed {
		t.Fatalf("root resume claimed another item: %#v", got)
	}
}

func TestReturnedWorkerCommandQuotesRepositoryPath(t *testing.T) {
	root := proposalRepository(t)
	quoted := root + " repo's"
	if err := os.Rename(root, quoted); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(quoted) })
	prepareSlice(t, quoted, "widget")
	b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Ready}}}
	start := implementCLI(t, quoted, b, "next")
	if got := returnedCLI(t, b, start.WorkerCommand); got.Status != "work_available" || got.Item.Number != 7 {
		t.Fatalf("quoted worker command failed: %#v", got)
	}
}

func TestImplementInspectionSuppliesFixedLedgerEvidenceWithoutClaiming(t *testing.T) {
	root := proposalRepository(t)
	baseline := prepareSlice(t, root, "widget")
	backend := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Ready}}}
	got := implementCLI(t, root, backend, "inspect", "--item", "7")
	if got.Status != "inspected" || got.Ledger == nil || got.Ledger.Baseline != baseline || got.Head != baseline || backend.work[0].Claimed {
		t.Fatalf("inspection = %#v", got)
	}
}

func TestImplementRejectsContradictorySourceAndSubmission(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	b := &implementationMemory{work: []workflow.ImplementationItem{{
		ID: "7", Branch: "widget", State: workflow.Ready, Claimed: true,
		Source:     &workflow.LifecycleObservation{Open: true, States: []workflow.State{workflow.Ready}, Claimed: true},
		Submission: &workflow.Submission{ID: "11", State: workflow.Rework, Lifecycle: &workflow.LifecycleObservation{Open: true, States: []workflow.State{workflow.Rework}}},
	}}}
	got := implementCLI(t, root, b, "resume", "--item", "7")
	if got.Status != "fix_required" || !strings.Contains(got.Reason, "source Ready contradicts Submission lifecycle") || !b.work[0].Claimed || b.work[0].TargetSnapshot != "" {
		t.Fatalf("contradictory resume: %#v, %#v", got, b.work[0])
	}
}

type incompleteImplementationMemory struct{ implementationMemory }

func (b *incompleteImplementationMemory) ImplementationItems(context.Context) ([]workflow.ImplementationItem, error) {
	return append([]workflow.ImplementationItem(nil), b.work...), nil
}

func TestImplementRequiresLifecycleObservations(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	for _, source := range []*workflow.LifecycleObservation{nil, {Open: true, States: []workflow.State{workflow.Ready}, Claimed: true}} {
		b := &incompleteImplementationMemory{implementationMemory{work: []workflow.ImplementationItem{{
			ID: "7", Branch: "widget", State: workflow.Ready, Claimed: true, Source: source,
			Submission: &workflow.Submission{ID: "11", State: workflow.Rework},
		}}}}
		got, err := workflow.InspectImplementation(context.Background(), root, "7", workflow.ArtifactEndpoints{}, b)
		if err != nil || got.Item == nil || !strings.Contains(got.Item.Problem, "missing lifecycle observations") {
			t.Fatalf("incomplete observation accepted: %#v, %v; item=%#v", got, err, got.Item)
		}
		resumed, err := workflow.StartImplementation(context.Background(), root, "origin", "7", "", "", workflow.ArtifactEndpoints{}, b)
		if err != nil || resumed.Status != "fix_required" || !b.work[0].Claimed || b.work[0].TargetSnapshot != "" {
			t.Fatalf("incomplete observation resumed: %#v, %v", resumed, err)
		}
	}
}

func TestImplementReviewPermissionUsesFreshLifecycle(t *testing.T) {
	for _, state := range []workflow.State{workflow.NeedsHuman, workflow.ReadyForMerge, workflow.Ready} {
		t.Run(string(state), func(t *testing.T) {
			root := proposalRepository(t)
			prepareSlice(t, root, "widget")
			b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Ready}}}
			start := implementCLI(t, root, b, "next")
			body := filepath.Join(start.Packet.Facts.Implementation.ResultDirectory, "submission.md")
			if err := os.WriteFile(body, []byte("opaque"), 0600); err != nil {
				t.Fatal(err)
			}
			completeAndRetireSlice(t, root, "widget")
			b.remoteHeads["widget"] = strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			b.beforeTransition = func() { b.work[0].Submission.Lifecycle.States = []workflow.State{state} }
			got := implementCLI(t, root, b, "submit", "--item", "7", "--body", body)
			if got.Status != "fix_required" || !b.work[0].Claimed || !slices.Equal(b.work[0].Submission.Lifecycle.States, []workflow.State{state}) {
				t.Fatalf("fresh forbidden lifecycle mutated: %#v, %#v", got, b.work[0].Submission.Lifecycle)
			}
			if _, err := os.Stat(body); err != nil {
				t.Fatalf("refusal removed Result Document: %v", err)
			}
		})
	}
}

func TestImplementRetainsClaimBeforeHandoffReadback(t *testing.T) {
	for _, target := range []workflow.State{workflow.AwaitingReview, workflow.NeedsHuman} {
		t.Run(string(target), func(t *testing.T) {
			root := proposalRepository(t)
			prepareSlice(t, root, "widget")
			b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Ready}}}
			start := implementCLI(t, root, b, "next")
			directory := start.Packet.Facts.Implementation.ResultDirectory
			body, decision := filepath.Join(directory, "submission.md"), filepath.Join(directory, "decision.md")
			for _, path := range []string{body, decision} {
				if err := os.WriteFile(path, []byte("opaque"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			completeAndRetireSlice(t, root, "widget")
			head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			b.remoteHeads["widget"] = head
			b.failTransition = true
			b.beforeTransition = func() {
				b.work[0].Source.States, b.work[0].Source.Claimed = nil, false
				if target == workflow.NeedsHuman {
					b.work[0].Source.States = []workflow.State{workflow.NeedsHuman}
				}
			}
			handoff := func() (workflow.ImplementationOutcome, error) {
				if target == workflow.AwaitingReview {
					return workflow.SubmitImplementation(context.Background(), root, "origin", "7", body, workflow.ArtifactEndpoints{}, b)
				}
				return workflow.PauseImplementation(context.Background(), root, "origin", "7", "mandatory_rule", decision, body, workflow.ArtifactEndpoints{}, b)
			}
			got, err := handoff()
			if err == nil || !strings.Contains(err.Error(), "interrupted projection") || !b.work[0].Source.Claimed || b.work[0].Transition.Completed || b.work[0].Transition.Head != head {
				t.Fatalf("failed projection was finalized before recovery: %#v, %v; item=%#v", got, err, b.work[0])
			}
			if _, err := os.Stat(directory); err != nil {
				t.Fatalf("failed handoff removed Result Documents: %v", err)
			}
			id := b.work[0].Submission.ID
			b.beforeTransition = nil
			got, err = handoff()
			if err != nil || got.Status != string(target) || got.Item.Claimed || !got.Item.Transition.Completed || got.Item.Submission.ID != id || got.Item.Submission.Head != head {
				t.Fatalf("retry lost durable handoff: %#v, %v", got, err)
			}
		})
	}
}

func TestStatusRetainsImplementationClaimOnProjectionFailure(t *testing.T) {
	b := &implementationMemory{work: []workflow.ImplementationItem{{
		ID: "7", Branch: "widget", State: workflow.Ready,
		Submission: &workflow.Submission{ID: "11", State: workflow.AwaitingReview, Head: "fixed"},
	}}, failTransition: true}
	_, err := workflow.ObserveStatus(context.Background(), b)
	if err == nil || !strings.Contains(err.Error(), "interrupted projection") || !b.work[0].Claimed {
		t.Fatalf("status dropped failed implementation Claim: %v, %#v", err, b.work[0])
	}
	got, err := workflow.ObserveStatus(context.Background(), b)
	if err != nil || got.Items[0].Claimed || got.Items[0].State != workflow.AwaitingReview {
		t.Fatalf("status retry: %#v, %v", got, err)
	}
}

func TestImplementationReviewProjectionBackendParity(t *testing.T) {
	for _, tt := range []struct {
		labels []string
		states []workflow.State
		allow  bool
	}{
		{nil, nil, true},
		{[]string{"rework", "wip"}, []workflow.State{workflow.Rework}, true},
		{[]string{"rework", "review"}, []workflow.State{workflow.Rework, workflow.AwaitingReview}, true},
		{[]string{"review", "rework"}, []workflow.State{workflow.AwaitingReview, workflow.Rework}, true},
		{[]string{"ready"}, []workflow.State{workflow.Ready}, false},
		{[]string{"done"}, []workflow.State{workflow.ReadyForMerge}, false},
		{[]string{"needs-human"}, []workflow.State{workflow.NeedsHuman}, false},
		{[]string{"review", "needs-human"}, []workflow.State{workflow.AwaitingReview, workflow.NeedsHuman}, false},
		// Preserve the existing first-state policy, even for these unusual overlaps.
		{[]string{"review", "done"}, []workflow.State{workflow.AwaitingReview, workflow.ReadyForMerge}, true},
		{[]string{"done", "review"}, []workflow.State{workflow.ReadyForMerge, workflow.AwaitingReview}, false},
		{[]string{"review", "ready"}, []workflow.State{workflow.AwaitingReview, workflow.Ready}, true},
		{[]string{"ready", "review"}, []workflow.State{workflow.Ready, workflow.AwaitingReview}, false},
	} {
		t.Run(fmt.Sprint(tt.labels), func(t *testing.T) {
			labels := slices.Clone(tt.labels)
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path := strings.TrimPrefix(r.URL.Path, "/repos/acme/widgets")
				if r.Method != http.MethodGet {
					writes++
				}
				switch {
				case path == "/issues/11" && r.Method == http.MethodGet:
					ls := []map[string]string{}
					for _, label := range labels {
						ls = append(ls, map[string]string{"name": label})
					}
					json.NewEncoder(w).Encode(map[string]any{"number": 11, "state": "open", "labels": ls})
				case path == "/issues/7" && r.Method == http.MethodGet:
					fmt.Fprint(w, `{"number":7,"state":"open","labels":[]}`)
				case path == "/issues/11/labels" && r.Method == http.MethodPost:
					var payload struct{ Labels []string }
					json.NewDecoder(r.Body).Decode(&payload)
					labels = append(labels, payload.Labels...)
				case strings.HasPrefix(path, "/issues/11/labels/") && r.Method == http.MethodDelete:
					label := strings.TrimPrefix(path, "/issues/11/labels/")
					labels = slices.DeleteFunc(labels, func(value string) bool { return value == label })
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			native := setup.NewGitHubBackend(server.URL, "token", server.Client())
			native.BindRepository(github.RepositoryID{Owner: "acme", Name: "widgets"})
			item := workflow.ImplementationItem{ID: "7", State: workflow.Rework, Source: &workflow.LifecycleObservation{Open: true}, Submission: &workflow.Submission{ID: "11", Lifecycle: &workflow.LifecycleObservation{Open: true, States: slices.Clone(tt.states), Claimed: slices.Contains(tt.labels, "wip")}}}
			memory := &implementationMemory{work: []workflow.ImplementationItem{item}}
			for name, backend := range map[string]workflow.ImplementationBackend{"HTTP": native, "memory": memory} {
				err := backend.AwaitImplementationReview(context.Background(), item, func() error { return nil })
				if (err == nil) != tt.allow {
					t.Fatalf("%s permission differs: %v", name, err)
				}
			}
			wantLabels, wantStates := slices.Clone(tt.labels), slices.Clone(tt.states)
			if tt.allow {
				if !slices.Contains(wantLabels, "review") {
					wantLabels = append(wantLabels, "review")
					wantStates = append(wantStates, workflow.AwaitingReview)
				}
				wantLabels = slices.DeleteFunc(wantLabels, func(label string) bool { return label == "rework" || label == "wip" })
				wantStates = slices.DeleteFunc(wantStates, func(state workflow.State) bool { return state == workflow.Rework })
			} else if writes != 0 {
				t.Fatalf("adapter independently restored Claim or projected forbidden state: %d writes", writes)
			}
			if !slices.Equal(labels, wantLabels) || !slices.Equal(memory.work[0].Submission.Lifecycle.States, wantStates) || memory.work[0].Submission.Lifecycle.Claimed {
				t.Fatalf("projection mismatch: HTTP=%v, memory=%#v", labels, memory.work[0].Submission.Lifecycle)
			}
		})
	}
}

func TestImplementInspectsCanonicalAttachments(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	for _, tt := range []struct {
		name            string
		source          workflow.State
		submission      []workflow.State
		pending         workflow.State
		want            workflow.State
		problem         string
		observedProblem string
	}{
		{name: "first review source cleanup", source: workflow.Ready, submission: []workflow.State{workflow.AwaitingReview}, want: workflow.Ready},
		{name: "human requeues rework", source: workflow.NeedsHuman, submission: []workflow.State{workflow.Rework}, want: workflow.Rework},
		{name: "human requeues review", source: workflow.NeedsHuman, submission: []workflow.State{workflow.AwaitingReview}, want: workflow.AwaitingReview},
		{name: "paused submission", submission: []workflow.State{workflow.NeedsHuman}, want: workflow.NeedsHuman},
		{name: "ambiguous requeue", source: workflow.NeedsHuman, submission: []workflow.State{workflow.Rework, workflow.AwaitingReview}, want: workflow.NeedsHuman, problem: "contradictory lifecycle projections"},
		{name: "existing review recovery", submission: []workflow.State{workflow.AwaitingReview, workflow.Rework}, pending: workflow.Rework, want: workflow.Rework},
		{name: "ownership problem survives validation", source: workflow.Ready, submission: []workflow.State{workflow.Rework}, observedProblem: "multiple source issues own the conventional branch", want: workflow.Ready, problem: "multiple source issues own the conventional branch"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			source := &workflow.LifecycleObservation{Open: true}
			if tt.source != "" {
				source.States = []workflow.State{tt.source}
			}
			b := &implementationMemory{work: []workflow.ImplementationItem{{
				ID: "7", Branch: "widget", Source: source, Problem: tt.observedProblem,
				Submission: &workflow.Submission{ID: "11", Lifecycle: &workflow.LifecycleObservation{Open: true, States: tt.submission}, PendingReview: tt.pending},
			}}}
			got := implementCLI(t, root, b, "inspect", "--item", "7")
			if got.Status != "inspected" || got.Item.State != tt.want || got.Item.Problem != tt.problem {
				t.Fatalf("canonical attachment: %#v", got.Item)
			}
		})
	}
}

func TestImplementInspectsPendingLifecycleProgress(t *testing.T) {
	root := proposalRepository(t)
	baseline := prepareSlice(t, root, "widget")
	for _, tt := range []struct {
		name         string
		from, target workflow.State
		source       workflow.LifecycleObservation
		submission   *workflow.LifecycleObservation
		problem      string
		want         workflow.State
		resume       workflow.State
		wantProblem  string
	}{
		{name: "pause overlap", from: workflow.Ready, target: workflow.NeedsHuman, source: workflow.LifecycleObservation{Open: true, States: []workflow.State{workflow.Ready, workflow.NeedsHuman}}, submission: &workflow.LifecycleObservation{Open: true, States: []workflow.State{workflow.NeedsHuman}}, want: workflow.Ready, resume: workflow.Ready},
		{name: "pause source claim", from: workflow.Ready, target: workflow.NeedsHuman, source: workflow.LifecycleObservation{Open: true, States: []workflow.State{workflow.NeedsHuman}, Claimed: true}, want: workflow.Ready, resume: workflow.Ready},
		{name: "pause issue only complete", from: workflow.Ready, target: workflow.NeedsHuman, source: workflow.LifecycleObservation{Open: true, States: []workflow.State{workflow.NeedsHuman}}, want: workflow.NeedsHuman},
		{name: "rework pause overlap", from: workflow.Rework, target: workflow.NeedsHuman, source: workflow.LifecycleObservation{Open: true}, submission: &workflow.LifecycleObservation{Open: true, States: []workflow.State{workflow.Rework, workflow.NeedsHuman}, Claimed: true}, want: workflow.Rework, resume: workflow.Rework},
		{name: "rework pause complete", from: workflow.Rework, target: workflow.NeedsHuman, source: workflow.LifecycleObservation{Open: true, States: []workflow.State{workflow.NeedsHuman}}, submission: &workflow.LifecycleObservation{Open: true, States: []workflow.State{workflow.NeedsHuman}}, want: workflow.NeedsHuman},
		{name: "review overlap", from: workflow.Rework, target: workflow.AwaitingReview, source: workflow.LifecycleObservation{Open: true}, submission: &workflow.LifecycleObservation{Open: true, States: []workflow.State{workflow.Rework, workflow.AwaitingReview}, Claimed: true}, want: workflow.Rework, resume: workflow.Rework},
		{name: "review submission claim", from: workflow.Rework, target: workflow.AwaitingReview, source: workflow.LifecycleObservation{Open: true}, submission: &workflow.LifecycleObservation{Open: true, States: []workflow.State{workflow.AwaitingReview}, Claimed: true}, want: workflow.Rework, resume: workflow.Rework},
		{name: "review source cleanup", from: workflow.Ready, target: workflow.AwaitingReview, source: workflow.LifecycleObservation{Open: true, States: []workflow.State{workflow.Ready}}, submission: &workflow.LifecycleObservation{Open: true, States: []workflow.State{workflow.AwaitingReview}}, want: workflow.Ready, resume: workflow.Ready},
		{name: "review complete", from: workflow.Ready, target: workflow.AwaitingReview, source: workflow.LifecycleObservation{Open: true}, submission: &workflow.LifecycleObservation{Open: true, States: []workflow.State{workflow.AwaitingReview}}, want: workflow.AwaitingReview},
		{name: "review missing submission", from: workflow.Ready, target: workflow.AwaitingReview, source: workflow.LifecycleObservation{Open: true}, want: workflow.Ready, resume: workflow.Ready},
		{name: "unrelated metadata problem", from: workflow.Ready, target: workflow.NeedsHuman, source: workflow.LifecycleObservation{Open: true, States: []workflow.State{workflow.Ready}}, submission: &workflow.LifecycleObservation{Open: true, States: []workflow.State{workflow.NeedsHuman}}, problem: "conflicting Target Snapshot metadata", wantProblem: "conflicting Target Snapshot metadata"},
		{name: "source drift", from: workflow.Rework, target: workflow.NeedsHuman, source: workflow.LifecycleObservation{Open: true, States: []workflow.State{workflow.ReadyForMerge}}, wantProblem: "projections contradict the pending implementation transition"},
		{name: "submission drift", from: workflow.Rework, target: workflow.AwaitingReview, source: workflow.LifecycleObservation{Open: true}, submission: &workflow.LifecycleObservation{Open: true, States: []workflow.State{workflow.Ready}}, wantProblem: "projections contradict the pending implementation transition"},
		{name: "closed source", from: workflow.Ready, target: workflow.NeedsHuman, source: workflow.LifecycleObservation{States: []workflow.State{workflow.Ready}, Claimed: true}, wantProblem: "projections contradict the pending implementation transition"},
		{name: "closed submission", from: workflow.Rework, target: workflow.AwaitingReview, source: workflow.LifecycleObservation{Open: true}, submission: &workflow.LifecycleObservation{States: []workflow.State{workflow.Rework}, Claimed: true}, wantProblem: "projections contradict the pending implementation transition"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			transition := workflow.ImplementationTransition{From: tt.from, Target: tt.target, Head: baseline, Directory: "original-operation"}
			item := workflow.ImplementationItem{ID: "opaque-work", Branch: "widget", Source: &tt.source, Problem: tt.problem, TargetSnapshot: baseline, Transition: &transition}
			if tt.submission != nil {
				item.Submission = &workflow.Submission{ID: "opaque-submission", Head: baseline, Lifecycle: tt.submission}
			}
			b := &implementationMemory{work: []workflow.ImplementationItem{item}}
			got, err := workflow.InspectImplementation(context.Background(), root, item.ID, workflow.ArtifactEndpoints{}, b)
			if err != nil || got.Status != "inspected" || got.Item.Problem != tt.wantProblem {
				t.Fatalf("inspection: %#v, %v; item=%#v", got, err, got.Item)
			}
			if tt.wantProblem == "" && (got.Item.State != tt.want || got.Item.ResumeState != tt.resume) {
				t.Fatalf("wrong completion/resume: %#v", got.Item)
			}
			claimed := tt.source.Claimed || tt.submission != nil && tt.submission.Claimed
			if got.Item.Claimed != claimed || got.Head != baseline || got.Item.TargetSnapshot != baseline || *got.Item.Transition != transition || !reflect.DeepEqual(b.work[0], item) {
				t.Fatalf("inspection changed obligations: %#v", got.Item)
			}
			if got.Item.Submission != nil && (got.Item.Submission.ID != "opaque-submission" || got.Item.Submission.Head != baseline) {
				t.Fatalf("inspection lost Submission identity or head: %#v", got.Item.Submission)
			}
		})
	}
}

func TestImplementReconcilesInterruptedHandoffs(t *testing.T) {
	for _, kind := range []string{"submit", "needs-human"} {
		t.Run(kind, func(t *testing.T) {
			root := proposalRepository(t)
			prepareSlice(t, root, "widget")
			b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Ready}}, remoteHeads: map[string]string{}}
			start := implementCLI(t, root, b, "next")
			dir := start.Packet.Facts.Implementation.ResultDirectory
			for _, name := range []string{"submission.md", "decision.md"} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte("opaque\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			args := []string{kind, "--item", "7"}
			if kind == "submit" {
				completeAndRetireSlice(t, root, "widget")
				args = append(args, "--body", filepath.Join(dir, "submission.md"))
			} else {
				args = append(args, "--reason", "mandatory_rule", "--decision", filepath.Join(dir, "decision.md"))
			}
			b.remoteHeads["widget"] = strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			b.failTransition = true
			var output bytes.Buffer
			app := newApp(func(github.RepositoryID) (setup.Backend, error) { return b, nil }, bytes.NewReader(nil), &output, &output)
			command := append([]string{"skl", "implement"}, args...)
			command = append(command, "--repo", root)
			if err := app.Run(command); err == nil {
				t.Fatal("fixture did not interrupt transition")
			}
			got := implementCLI(t, root, b, args...)
			want := "awaiting_review"
			if kind == "needs-human" {
				want = "needs_human"
			}
			if got.Status != want || got.Item.Claimed || got.Item.Transition == nil || !got.Item.Transition.Completed {
				t.Fatalf("retry did not reconcile: %#v", got)
			}
			if got := implementCLI(t, root, b, args...); got.Status != want {
				t.Fatalf("completed retry: %#v", got)
			}
		})
	}
}

func TestImplementChecksContradictionsAndHeadDuringProjection(t *testing.T) {
	for _, fault := range []string{"contradiction", "local movement", "remote movement", "completion local movement", "completion remote movement"} {
		t.Run(fault, func(t *testing.T) {
			root := proposalRepository(t)
			prepareSlice(t, root, "widget")
			b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Ready}}, remoteHeads: map[string]string{}}
			start := implementCLI(t, root, b, "next")
			body := filepath.Join(start.Packet.Facts.Implementation.ResultDirectory, "submission.md")
			if err := os.WriteFile(body, []byte("opaque"), 0600); err != nil {
				t.Fatal(err)
			}
			completeAndRetireSlice(t, root, "widget")
			b.remoteHeads["widget"] = strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			switch fault {
			case "contradiction":
				b.work[0].Problem = "contradictory lifecycle projections"
			case "local movement":
				b.beforeTransition = func() { runGit(t, root, "commit", "--allow-empty", "-m", "movement") }
			case "remote movement":
				b.beforeTransition = func() { b.remoteHeads["widget"] = "moved" }
			case "completion local movement":
				b.afterCompletion = func() { runGit(t, root, "commit", "--allow-empty", "-m", "late movement") }
			case "completion remote movement":
				b.afterCompletion = func() { b.remoteHeads["widget"] = "moved" }
			}
			got := implementCLI(t, root, b, "submit", "--item", "7", "--body", body)
			if got.Status != "fix_required" || !b.work[0].Claimed || b.work[0].State != workflow.Ready {
				t.Fatalf("unsafe projection: %#v %#v", got, b.work)
			}
			continued := returnedCLI(t, b, start.ContinuationCommand+" --wait=1ms")
			if continued.Status != "fix_required" || !b.work[0].Claimed {
				t.Fatalf("failed handoff authorized continuation: %+v", continued)
			}
		})
	}
}

func TestExplicitEmptyContinuationRefusesBeforeBackend(t *testing.T) {
	for _, lane := range []string{"implement", "watchdog"} {
		for _, flags := range [][]string{{"--after="}, {"--after", ""}, {"--after=", "--item=7"}, {"--after=", "--reviewed-head="}} {
			t.Run(lane+strings.Join(flags, "/"), func(t *testing.T) {
				root, backend := waitFixture(t, lane)
				calls := 0
				app := newApp(func(github.RepositoryID) (setup.Backend, error) { calls++; return backend, nil }, nil, &bytes.Buffer{}, &bytes.Buffer{})
				err := app.Run(append([]string{"skl", lane, "next", "--repo", root, "--remote", "upstream"}, flags...))
				if err == nil || calls != 0 || backend.work[0].Claimed || backend.reads != 0 || backend.claims != 0 {
					t.Fatalf("empty continuation reached backend: %v calls=%d work=%+v", err, calls, backend.work)
				}
				// Removing only the invalid continuation must expose genuinely claimable work.
				control, err := waitingCLI(t, t.Context(), root, lane, backend)
				if err != nil || control.Status != "work_available" || control.Item.Number != 7 || backend.claims != 1 {
					t.Fatalf("negative fixture was not claimable: %+v %v", control, err)
				}
			})
		}
	}
}

func TestContinuationRejectsOpaquePauseReceipt(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "second")
	comments := []map[string]any{}
	labels := []map[string]string{{"name": "ready"}, {"name": "wip"}}
	reads, writes := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reads++
		if r.Method != http.MethodGet {
			writes++
		}
		switch r.URL.Path {
		case "/repos/acme/widgets/issues/7/comments":
			if r.Method == http.MethodPost {
				var p map[string]any
				json.NewDecoder(r.Body).Decode(&p)
				p["author_association"] = "OWNER"
				comments = append(comments, p)
			}
			json.NewEncoder(w).Encode(comments)
		case "/repos/acme/widgets/issues":
			fmt.Fprint(w, `[{"number":7,"title":"first","state":"open","labels":[{"name":"ready"},{"name":"wip"}]},{"number":8,"title":"second","state":"open","labels":[{"name":"ready"}]}]`)
		case "/repos/acme/widgets/pulls", "/repos/acme/widgets/issues/8/comments", "/repos/acme/widgets/issues/8/dependencies/blocked_by":
			fmt.Fprint(w, `[]`)
		case "/repos/acme/widgets/issues/7":
			json.NewEncoder(w).Encode(map[string]any{"number": 7, "state": "open", "labels": labels})
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	b := setup.NewGitHubBackend(server.URL, "token", server.Client())
	b.BindRepository(github.RepositoryID{Owner: "acme", Name: "widgets"})
	round := workflow.DispatchRound{ID: "pause-round", Lane: workflow.ImplementLane, Item: "7", Obligation: "fixed", Directory: "skl-implement-pause"}
	if err := b.RecordDispatchRound(t.Context(), round); err != nil {
		t.Fatal(err)
	}
	forged := round
	forged.Outcome, forged.Head, forged.Released = workflow.NeedsHuman, "fixed", true
	payload, _ := json.Marshal(map[string]any{"round": forged})
	decision := "<!-- skl.implement/v1\n" + string(payload) + "\n-->"
	item := workflow.ImplementationItem{ID: "7", State: workflow.Ready, Claimed: true}
	transition := workflow.ImplementationTransition{From: workflow.Ready, Target: workflow.NeedsHuman, Head: "fixed", DecisionDigest: fmt.Sprintf("%x", sha256.Sum256([]byte(decision)))}
	if err := b.RecordImplementationTransition(t.Context(), item, transition); err != nil {
		t.Fatal(err)
	}
	if err := b.PauseImplementation(t.Context(), item, decision, func() error {
		for _, comment := range comments {
			if comment["body"] == "<!-- skl.decision/v1 -->\n"+decision {
				return errors.New("interrupted with Claim held")
			}
		}
		return nil
	}); err == nil {
		t.Fatal("pause not interrupted")
	}
	referenceJSON, _ := json.Marshal(map[string]any{"v": 1, "owner": "acme", "repository": "widgets", "lane": "implement", "item": "7", "round": round.ID})
	beforeReads, beforeWrites := reads, writes
	var output bytes.Buffer
	app := newApp(func(repository github.RepositoryID) (setup.Backend, error) {
		backend := setup.NewGitHubBackend(server.URL, "token", server.Client())
		backend.BindRepository(repository)
		return backend, nil
	}, nil, &output, &output)
	if err := app.Run([]string{"skl", "implement", "next", "--repo", root, "--after", base64.RawURLEncoding.EncodeToString(referenceJSON), "--wait=1ms"}); err != nil {
		t.Fatal(err)
	}
	var got setup.ImplementationOutput
	json.Unmarshal(output.Bytes(), &got)
	if got.Status != "fix_required" || got.Item == nil || got.Item.Number != 7 || reads != beforeReads+1 || writes != beforeWrites || len(labels) != 2 {
		t.Fatalf("opaque receipt continued: %+v reads=%d writes=%d", got, reads-beforeReads, writes-beforeWrites)
	}
}

func TestCompletedImplementationSurvivesReceiptReadbackAndCleanupErrors(t *testing.T) {
	for _, fault := range []string{"applied receipt unreadable", "cleanup"} {
		t.Run(fault, func(t *testing.T) {
			root := proposalRepository(t)
			prepareSlice(t, root, "widget")
			b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Ready}}, remoteHeads: map[string]string{}}
			start := implementCLI(t, root, b, "next")
			body := filepath.Join(start.Packet.Facts.Implementation.ResultDirectory, "submission.md")
			os.WriteFile(body, []byte("opaque"), 0600)
			completeAndRetireSlice(t, root, "widget")
			b.remoteHeads["widget"] = strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			b.afterRound = func(round workflow.DispatchRound) error {
				if round.Outcome != "" {
					if fault == "cleanup" {
						return os.WriteFile(filepath.Join(filepath.Dir(body), "unexpected"), []byte("preserve"), 0600)
					}
					b.roundReadError = errors.New("receipt applied but readback unavailable")
					return b.roundReadError
				}
				return nil
			}
			var output bytes.Buffer
			app := newApp(func(github.RepositoryID) (setup.Backend, error) { return b, nil }, nil, &output, &output)
			if err := app.Run([]string{"skl", "implement", "submit", "--repo", root, "--item", "7", "--body", body}); err == nil {
				t.Fatal("fault not injected")
			}
			if b.work[0].Claimed || b.work[0].Transition == nil || !b.work[0].Transition.Completed {
				t.Fatalf("completed handoff rolled back: %+v", b.work)
			}
			if _, err := os.Stat(body); err != nil {
				t.Fatalf("failure removed worker prose: %v", err)
			}
			b.roundReadError = nil
			b.afterRound = nil
			got := returnedCLI(t, b, start.ContinuationCommand)
			if got.Status != "no_work" || got.PreviousHandoff == nil || got.PreviousHandoff.Outcome != workflow.AwaitingReview {
				t.Fatalf("post-proof error revoked handoff: %+v", got)
			}
		})
	}
}

func TestReworkMissingReviewedHeadRetainsRecoverableHistory(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	completeAndRetireSlice(t, root, "widget")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	old := workflow.DispatchRound{ID: "historical", Lane: workflow.ImplementLane, Item: "7", Obligation: head, Directory: "skl-implement-old", Head: head, Outcome: workflow.AwaitingReview, Released: true}
	b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Rework, Submission: &workflow.Submission{ID: "11", Head: head}}}, rounds: map[workflow.WorkItemID][]workflow.DispatchRound{"7": {old}}}
	got := implementCLI(t, root, b, "next")
	if got.Status != "fix_required" || !b.work[0].Claimed || len(b.rounds["7"]) != 1 || b.rounds["7"][0] != old {
		t.Fatalf("missing obligation poisoned history: %+v %+v", got, b.rounds)
	}
	got = implementCLI(t, root, b, "resume", "--item", "7", "--reviewed-head", head)
	if got.Status != "work_available" || len(b.rounds["7"]) != 2 || b.rounds["7"][0] != old || b.rounds["7"][1].Obligation != head {
		t.Fatalf("explicit recovery failed: %+v %+v", got, b.rounds)
	}
}

// Observe real GitHub head-scoped metadata while retaining the existing mutation fixture.
type githubProjectedImplementation struct {
	*implementationMemory
	projection *setup.GitHubBackend
}

func (b *githubProjectedImplementation) ImplementationItems(ctx context.Context) ([]workflow.ImplementationItem, error) {
	return b.projection.ImplementationItems(ctx)
}

func TestReworkPushSubmitsThroughGitHubProjection(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	completeAndRetireSlice(t, root, "widget")
	reviewed := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	memory := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Rework, Submission: &workflow.Submission{ID: "11", Head: reviewed, Base: "main"}}}, remoteHeads: map[string]string{"widget": reviewed}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		item := memory.work[0]
		switch r.URL.Path {
		case "/repos/acme/widgets/issues":
			fmt.Fprint(w, `[{"number":7,"title":"widget","state":"open","labels":[]}]`)
		case "/repos/acme/widgets/pulls", "/repos/acme/widgets/pulls/11":
			label := "rework"
			if item.State == workflow.AwaitingReview {
				label = "review"
			}
			labels := []map[string]string{{"name": label}}
			if item.Claimed {
				labels = append(labels, map[string]string{"name": "wip"})
			}
			pull := map[string]any{"number": 11, "state": "open", "labels": labels, "head": map[string]any{"ref": "widget", "sha": memory.remoteHeads["widget"], "repo": map[string]string{"full_name": "acme/widgets"}}, "base": map[string]string{"ref": "main"}}
			if strings.HasSuffix(r.URL.Path, "/11") {
				json.NewEncoder(w).Encode(pull)
			} else {
				json.NewEncoder(w).Encode([]any{pull})
			}
		case "/repos/acme/widgets/issues/7/comments":
			metadata := []any{map[string]any{"reviewed_head": reviewed, "review_round_head": reviewed}}
			for _, round := range memory.rounds["7"] {
				metadata = append(metadata, map[string]any{"round": round})
			}
			if item.Transition != nil {
				metadata = append(metadata, map[string]any{"transition": item.Transition})
			}
			comments := []map[string]string{}
			for _, value := range metadata {
				p, _ := json.Marshal(value)
				comments = append(comments, map[string]string{"author_association": "OWNER", "body": "<!-- skl.implement/v1\n" + string(p) + "\n-->"})
			}
			json.NewEncoder(w).Encode(comments)
		case "/repos/acme/widgets/issues/11/comments", "/repos/acme/widgets/pulls/11/comments", "/repos/acme/widgets/pulls/11/reviews", "/repos/acme/widgets/issues/11/timeline":
			fmt.Fprint(w, `[]`)
		default:
			t.Errorf("unexpected request %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	projection := setup.NewGitHubBackend(server.URL, "token", server.Client())
	projection.BindRepository(github.RepositoryID{Owner: "acme", Name: "widgets"})
	b := &githubProjectedImplementation{memory, projection}
	start := returnedCLI(t, b, "skl implement next --repo '"+root+"'")
	runGit(t, root, "switch", "main")
	worktree := start.Packet.Facts.Implementation.Worktree
	runGit(t, root, "worktree", "add", worktree, "widget")
	resumed := returnedCLI(t, b, start.WorkerCommand)
	runGit(t, worktree, "commit", "--allow-empty", "-m", "fix findings")
	pushed := strings.TrimSpace(runGitOutput(t, worktree, "rev-parse", "HEAD"))
	memory.remoteHeads["widget"] = pushed
	os.WriteFile(filepath.Join(resumed.Packet.Facts.Implementation.ResultDirectory, "submission.md"), []byte("fixes"), 0600)
	got := returnedCLI(t, b, resumed.Packet.Facts.Implementation.SubmitCommand)
	if got.Status != "awaiting_review" || memory.rounds["7"][0].Obligation != reviewed || memory.rounds["7"][0].Head != pushed {
		t.Fatalf("pushed Rework rejected: %+v %+v", got, memory.rounds)
	}
	if got := returnedCLI(t, b, start.ContinuationCommand); got.Status != "no_work" || got.PreviousHandoff == nil {
		t.Fatalf("Rework continuation: %+v", got)
	}
}

func TestReturnedCommandHandoffLifecycle(t *testing.T) {
	for _, tc := range []struct {
		lane, kind, want string
		existing         bool
	}{
		{"implement", "submission", "awaiting_review", false},
		{"implement", "decision", "needs_human", true},
		{"implement", "draft", "needs_human", false},
		{"watchdog", "markers", "ready_for_merge", true},
		{"watchdog", "rework", "rework", true},
		{"watchdog", "conflict", "rework", true},
		{"watchdog", "needs-human", "needs_human", true},
		{"watchdog", "second failure", "needs_human", true},
	} {
		for _, documents := range []bool{false, true} {
			t.Run(tc.kind+fmt.Sprint(documents), func(t *testing.T) {
				root := proposalRepository(t)
				root, _ = filepath.EvalSymlinks(root)
				prepareSlice(t, root, "widget")
				if tc.lane == "watchdog" {
					completeAndRetireSlice(t, root, "widget")
				}
				head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
				target := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "main"))
				runGit(t, root, "remote", "rename", "origin", "upstream")
				b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Ready}}, remoteHeads: map[string]string{"main": target, "widget": head}}
				if tc.lane == "watchdog" {
					b.work[0].State = workflow.AwaitingReview
					b.work[0].Submission = &workflow.Submission{ID: "11", Head: head, Base: "main", Mergeability: "mergeable"}
					if tc.kind == "conflict" {
						b.work[0].Submission.Mergeability = "conflicting"
					}
				}
				runGit(t, root, "switch", "main")
				worktree := filepath.Join(root, ".worktrees", "widget")
				if tc.existing {
					runGit(t, root, "worktree", "add", worktree, "widget")
				}
				if tc.kind == "second failure" {
					gitDir := strings.TrimSpace(runGitOutput(t, worktree, "rev-parse", "--absolute-git-dir"))
					if err := os.WriteFile(filepath.Join(gitDir, ".watchdog"), []byte("1:"+head+"\n"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				start := returnedCLI(t, b, "skl "+tc.lane+" next --repo '"+root+"' --remote upstream")
				directory := ""
				if tc.lane == "implement" {
					directory = start.Packet.Facts.Implementation.ResultDirectory
				} else {
					directory = start.Packet.Facts.Watchdog.ResultDirectory
				}
				t.Cleanup(func() { os.RemoveAll(directory) })
				document := filepath.Join(directory, "submission.md")
				if tc.lane == "watchdog" {
					document = filepath.Join(directory, "repair.md")
				}
				if documents {
					os.WriteFile(document, []byte("worker repair document"), 0600)
				}
				runGit(t, root, "commit", "--allow-empty", "-m", "target moves while worker is dispatched")
				b.remoteHeads["main"] = strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
				runGit(t, root, "update-ref", "refs/remotes/upstream/main", "HEAD")
				resumed := returnedCLI(t, b, start.WorkerCommand)
				if resumed.Status != "work_available" || resumed.ContinuationCommand != start.ContinuationCommand || len(b.rounds["7"]) != 1 {
					t.Fatalf("startup changed round: %+v", resumed)
				}
				active, command := "", ""
				if tc.lane == "implement" {
					f := resumed.Packet.Facts.Implementation
					active, command = f.ResultDirectory, f.SubmitCommand
					if f.TargetSnapshot != target {
						t.Fatalf("target repinned: %+v", f)
					}
				} else {
					f := resumed.Packet.Facts.Watchdog
					active, command = f.ResultDirectory, f.SubmitCommand
					if f.ReviewedHead != head {
						t.Fatalf("review repinned: %+v", f)
					}
				}
				if !strings.Contains(command, "--repo '"+worktree+"'") || !strings.Contains(command, "--remote 'upstream'") {
					t.Fatalf("handoff not worktree/remote bound: %s", command)
				}
				if documents {
					if content, err := os.ReadFile(document); err != nil || string(content) != "worker repair document" {
						t.Fatalf("startup discarded document: %q %v", content, err)
					}
				}
				if active != directory {
					if _, err := os.Stat(directory); !documents && !os.IsNotExist(err) {
						t.Fatalf("orphaned discarded packet: %v", err)
					}
				}
				if !tc.existing {
					runGit(t, root, "worktree", "add", worktree, "widget")
				}
				if tc.lane == "implement" {
					if tc.kind == "submission" {
						completeAndRetireSlice(t, worktree, "widget")
					}
					if tc.kind == "draft" {
						if err := os.WriteFile(filepath.Join(worktree, "widget.go"), []byte("package widget\n\nfunc Value() int { return 1 }\n"), 0644); err != nil {
							t.Fatal(err)
						}
						runGit(t, worktree, "add", "widget.go")
						runGit(t, worktree, "commit", "-m", "implementation work")
					}
					b.remoteHeads["widget"] = strings.TrimSpace(runGitOutput(t, worktree, "rev-parse", "HEAD"))
					os.WriteFile(filepath.Join(active, "submission.md"), []byte("opaque submission"), 0600)
					if tc.kind != "submission" {
						os.WriteFile(filepath.Join(active, "decision.md"), []byte("opaque decision"), 0600)
						command = strings.Replace(command, "implement submit", "implement needs-human", 1) + " --reason mandatory_rule --decision '" + filepath.Join(active, "decision.md") + "'"
						if tc.kind == "decision" {
							command = strings.Replace(command, " --body '"+filepath.Join(active, "submission.md")+"'", "", 1)
						}
					}
				} else {
					os.WriteFile(filepath.Join(active, "summary.md"), []byte("opaque review"), 0600)
					verdict := tc.kind
					if tc.kind == "second failure" {
						verdict = "rework"
					}
					if tc.kind == "markers" || tc.kind == "conflict" {
						verdict = "pass"
						os.WriteFile(filepath.Join(active, "body.md"), []byte("manual verification"), 0600)
						command += " --body '" + filepath.Join(active, "body.md") + "'"
					}
					if tc.kind == "markers" {
						runGit(t, worktree, "commit", "--allow-empty", "-m", "post-marker check head")
						final := strings.TrimSpace(runGitOutput(t, worktree, "rev-parse", "HEAD"))
						b.remoteHeads["widget"] = final
						b.work[0].Submission.Head = final
						command += " --head " + final
					}
					command += " --verdict " + verdict
				}
				got := returnedCLI(t, b, command)
				if got.Status != tc.want || b.work[0].Claimed {
					t.Fatalf("packet handoff failed: %+v", got)
				}
				if tc.kind == "draft" {
					if got.Item.Submission == nil || !got.Item.Submission.Draft || runGitOutput(t, worktree, "show", got.Item.Submission.Head+":widget.go") != "package widget\n\nfunc Value() int { return 1 }\n" {
						t.Fatalf("draft did not preserve source work at its published head: %+v", got)
					}
				}
				if _, err := os.Stat(active); !os.IsNotExist(err) {
					t.Fatalf("active result directory remains: %v", err)
				}
				if documents && active != directory {
					if _, err := os.Stat(document); err != nil {
						t.Fatalf("superseded worker document removed: %v", err)
					}
				}
				got = returnedCLI(t, b, start.ContinuationCommand)
				if got.Status != "no_work" || got.PreviousHandoff == nil || string(got.PreviousHandoff.Outcome) != tc.want || got.PreviousHandoff.Number != 7 {
					t.Fatalf("handoff continuation: %+v", got)
				}
			})
		}
	}
}

func TestContinuationRefusalMatrixHasNoEffects(t *testing.T) {
	for _, lane := range []string{"implement", "watchdog"} {
		for _, problem := range []string{"held Claim", "missing publication", "missing release", "missing proof", "contradictory proof", "missing item", "ambiguous item", "ambiguous attachment", "missing attachment", "wrong stage", "documents disappeared"} {
			t.Run(lane+"/"+problem, func(t *testing.T) {
				root := proposalRepository(t)
				prepareSlice(t, root, "widget")
				completeAndRetireSlice(t, root, "widget")
				head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
				b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Ready}}, remoteHeads: map[string]string{"main": strings.TrimSpace(runGitOutput(t, root, "rev-parse", "main")), "widget": head}}
				if lane == "watchdog" {
					b.work[0].State = workflow.AwaitingReview
					b.work[0].Submission = &workflow.Submission{ID: "11", Head: head, Base: "main", Mergeability: "mergeable"}
					runGit(t, root, "switch", "main")
					runGit(t, root, "worktree", "add", filepath.Join(root, ".worktrees", "widget"), "widget")
				}
				start := returnedCLI(t, b, "skl "+lane+" next --repo '"+root+"'")
				directory, command := "", ""
				if lane == "implement" {
					directory = start.Packet.Facts.Implementation.ResultDirectory
					os.WriteFile(filepath.Join(directory, "submission.md"), []byte("valid publication"), 0600)
					command = "skl implement submit --repo '" + root + "' --item 7 --body '" + filepath.Join(directory, "submission.md") + "'"
				} else {
					directory = start.Packet.Facts.Watchdog.ResultDirectory
					summary := filepath.Join(directory, "summary.md")
					os.WriteFile(summary, []byte("valid review"), 0600)
					command = start.Packet.Facts.Watchdog.SubmitCommand + " --verdict pass --body '" + summary + "'"
				}
				got := returnedCLI(t, b, command)
				if got.Status != "awaiting_review" && got.Status != "ready_for_merge" {
					t.Fatalf("invalid positive control: %+v", got)
				}
				// A completed semantic handoff is the otherwise-valid control for each single defect.
				successor := workflow.ImplementationItem{ID: "8", Branch: "widget", State: workflow.Ready}
				if lane == "watchdog" {
					successor.State = workflow.AwaitingReview
					successor.Submission = &workflow.Submission{ID: "12", Head: head, Base: "main", Mergeability: "mergeable"}
				}
				b.work = append(b.work, successor)
				os.Mkdir(directory, 0700)
				defer os.RemoveAll(directory)
				document := filepath.Join(directory, "repair.md")
				os.WriteFile(document, []byte("retain recovery document"), 0600)
				switch problem {
				case "held Claim":
					b.work[0].Claimed = true
					if lane == "implement" {
						b.work[0].State = workflow.Ready
						b.work[0].Transition.Completed = false
					} else {
						b.work[0].State = workflow.AwaitingReview
						b.work[0].Submission.PendingReview = ""
						b.work[0].Submission.Lifecycle = &workflow.LifecycleObservation{States: []workflow.State{workflow.AwaitingReview}, Open: true, Claimed: true}
					}
				case "missing publication":
					b.rounds["7"][0].Head = ""
				case "missing attachment":
					b.work[0].Submission = nil
				case "ambiguous attachment":
					b.work[0].Problem = "multiple Submissions share the conventional branch"
				case "missing release":
					b.rounds["7"][0].Released = false
				case "missing proof":
					b.rounds["7"] = nil
				case "contradictory proof":
					conflict := b.rounds["7"][0]
					conflict.Head = "contradictory"
					b.rounds["7"] = append(b.rounds["7"], conflict)
				case "missing item":
					b.work = b.work[1:]
				case "ambiguous item":
					b.work = append(b.work, b.work[0])
				case "wrong stage":
					if lane == "implement" {
						b.rounds["7"][0].Outcome = workflow.ReadyForMerge
					} else {
						b.rounds["7"][0].Outcome = workflow.AwaitingReview
					}
				case "documents disappeared":
					b.rounds["7"] = nil
					os.RemoveAll(directory)
				}
				before, _ := json.Marshal([]any{b.work, b.rounds, b.decisions})
				b.itemReads, b.roundReads = 0, 0
				got = returnedCLI(t, b, start.ContinuationCommand+" --wait=2ms --poll=1ms")
				after, _ := json.Marshal([]any{b.work, b.rounds, b.decisions})
				if got.Status != "fix_required" || got.Item == nil || got.Item.Number != 7 || got.Reason == "" || !bytes.Equal(before, after) || b.itemReads > 1 || b.roundReads != 1 {
					t.Fatalf("refusal effects: %+v reads=%d/%d before=%s after=%s", got, b.itemReads, b.roundReads, before, after)
				}
				for _, guidance := range []string{"stop", "inspect", "resume", "--item 7", "do not replay next or next --after"} {
					if !strings.Contains(got.Reason, guidance) {
						t.Fatalf("refusal lacks %q recovery guidance: %s", guidance, got.Reason)
					}
				}
				if problem != "documents disappeared" {
					if content, err := os.ReadFile(document); err != nil || string(content) != "retain recovery document" {
						t.Fatalf("refusal removed document: %q %v", content, err)
					}
				}
			})
		}
	}
}

func TestHistoricalContinuationAfterLaterActivity(t *testing.T) {
	for _, activity := range []string{"Watchdog Claim", "bounce and implementation Claim", "implementation pushed head", "later Watchdog round", "human merge", "human requeue"} {
		t.Run(activity, func(t *testing.T) {
			root := proposalRepository(t)
			prepareSlice(t, root, "widget")
			completeAndRetireSlice(t, root, "widget")
			head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Ready}}, remoteHeads: map[string]string{"widget": head, "main": strings.TrimSpace(runGitOutput(t, root, "rev-parse", "main"))}}
			initial := implementCLI(t, root, b, "next")
			directory := initial.Packet.Facts.Implementation.ResultDirectory
			if activity == "human requeue" {
				os.WriteFile(filepath.Join(directory, "decision.md"), []byte("pause"), 0600)
				got := implementCLI(t, root, b, "needs-human", "--item", "7", "--reason", "mandatory_rule", "--decision", filepath.Join(directory, "decision.md"))
				if got.Status != "needs_human" {
					t.Fatalf("pause: %+v", got)
				}
			} else {
				os.WriteFile(filepath.Join(directory, "submission.md"), []byte("publication"), 0600)
				if got := implementCLI(t, root, b, "submit", "--item", "7", "--body", filepath.Join(directory, "submission.md")); got.Status != "awaiting_review" {
					t.Fatalf("publication: %+v", got)
				}
				b.work[0].Submission.Mergeability = "mergeable"
			}
			original := initial
			lane := "implement"
			if activity == "human requeue" {
				b.work[0].State = workflow.Ready
			} else {
				watch := watchdogCLI(t, root, b, "next")
				if activity != "Watchdog Claim" {
					summary := filepath.Join(watch.Packet.Facts.Watchdog.ResultDirectory, "summary.md")
					os.WriteFile(summary, []byte("review"), 0600)
					verdict := "rework"
					if activity == "human merge" {
						verdict = "pass"
					}
					got := returnedCLI(t, b, watch.Packet.Facts.Watchdog.SubmitCommand+" --verdict "+verdict+" --body '"+summary+"'")
					if got.Status != "rework" && got.Status != "ready_for_merge" {
						t.Fatalf("review: %+v", got)
					}
					if activity != "bounce and implementation Claim" {
						original = watch
						lane = "watchdog"
					}
					if activity == "human merge" {
						b.work[0].State = workflow.Merged
						b.work[0].Submission.Merged = true
					} else {
						rework := implementCLI(t, root, b, "next")
						if rework.Status == "fix_required" {
							rework = implementCLI(t, root, b, "resume", "--item", "7", "--reviewed-head", head)
						}
						if rework.Status != "work_available" {
							t.Fatalf("rework: %+v", rework)
						}
						if activity != "bounce and implementation Claim" {
							implementationWorktree := rework.Packet.Facts.Implementation.Worktree
							runGit(t, implementationWorktree, "commit", "--allow-empty", "-m", "pushed rework fixes")
							pushed := strings.TrimSpace(runGitOutput(t, implementationWorktree, "rev-parse", "HEAD"))
							b.remoteHeads["widget"] = pushed
							b.work[0].Submission.Head = pushed
							if activity == "later Watchdog round" {
								body := filepath.Join(rework.Packet.Facts.Implementation.ResultDirectory, "submission.md")
								os.WriteFile(body, []byte("fixes"), 0600)
								got := implementCLI(t, root, b, "submit", "--item", "7", "--body", body)
								if got.Status != "awaiting_review" {
									t.Fatalf("resubmit: %+v", got)
								}
								got = watchdogCLI(t, root, b, "next")
								if got.Status != "work_available" {
									t.Fatalf("later review: %+v", got)
								}
							}
						}
					}
				}
			}
			prepareSlice(t, root, "second")
			completeAndRetireSlice(t, root, "second")
			secondHead := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			successor := workflow.ImplementationItem{ID: "8", Branch: "second", State: workflow.Ready, CreatedAt: "1900"}
			if lane == "watchdog" {
				successor.State = workflow.AwaitingReview
				successor.Submission = &workflow.Submission{ID: "12", Head: secondHead, Base: "main", Mergeability: "mergeable", CreatedAt: "1900"}
				runGit(t, root, "switch", "main")
				runGit(t, root, "worktree", "add", filepath.Join(root, ".worktrees", "second"), "second")
			}
			b.work = append(b.work, successor)
			// Keep the explicit human requeue eligible, but select the older successor.
			b.work[0].CreatedAt = "2020"
			before, _ := json.Marshal([]any{b.work[0], b.rounds["7"]})
			got := returnedCLI(t, b, original.ContinuationCommand)
			after, _ := json.Marshal([]any{b.work[0], b.rounds["7"]})
			if got.Status != "work_available" || got.Item.Number != 8 || got.PreviousHandoff == nil || got.PreviousHandoff.Number != 7 || !bytes.Equal(before, after) {
				t.Fatalf("historical proof disturbed later activity: %+v before=%s after=%s", got, before, after)
			}
		})
	}
}

func TestUnappliedCompletionReceiptRecoversOnlyThroughExplicitHandoff(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Ready}}, remoteHeads: map[string]string{}}
	start := implementCLI(t, root, b, "next")
	body := filepath.Join(start.Packet.Facts.Implementation.ResultDirectory, "submission.md")
	os.WriteFile(body, []byte("opaque"), 0600)
	completeAndRetireSlice(t, root, "widget")
	b.remoteHeads["widget"] = strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	b.afterCompletion = func() { b.roundWriteError = errors.New("receipt unapplied") }
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) { return b, nil }, nil, &output, &output)
	if err := app.Run([]string{"skl", "implement", "submit", "--repo", root, "--item", "7", "--body", body}); err == nil {
		t.Fatal("receipt failure not injected")
	}
	b.roundWriteError = nil
	b.afterCompletion = nil
	if got := returnedCLI(t, b, start.ContinuationCommand); got.Status != "fix_required" {
		t.Fatalf("missing receipt continued: %+v", got)
	}
	if _, err := os.Stat(body); err != nil {
		t.Fatal("continuation removed repair prose")
	}
	if got := implementCLI(t, root, b, "submit", "--item", "7", "--body", body); got.Status != "awaiting_review" {
		t.Fatalf("explicit recovery failed: %+v", got)
	}
	if got := returnedCLI(t, b, start.ContinuationCommand); got.Status != "no_work" || got.PreviousHandoff == nil {
		t.Fatalf("explicit handoff failed to recover receipt: %+v", got)
	}
}

func TestReworkResumeRejectsOverrideBeforeConcreteMetadataMutation(t *testing.T) {
	root := proposalRepository(t)
	other := prepareSlice(t, root, "widget")
	completeAndRetireSlice(t, root, "widget")
	reviewed := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	runGit(t, root, "commit", "--allow-empty", "-m", "rework fixes")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	directory, err := os.MkdirTemp("", "skl-implement-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(directory)
	os.WriteFile(filepath.Join(directory, ".skl-result"), []byte("skl.implement/v1\n"), 0600)
	round := workflow.DispatchRound{ID: "active", Lane: workflow.ImplementLane, Item: "7", Submission: "11", Obligation: reviewed, Directory: filepath.Base(directory)}
	comments := []map[string]any{}
	for _, v := range []any{map[string]any{"reviewed_head": reviewed, "review_round_head": head}, map[string]any{"round": round}} {
		p, _ := json.Marshal(v)
		comments = append(comments, map[string]any{"body": "<!-- skl.implement/v1\n" + string(p) + "\n-->", "author_association": "OWNER"})
	}
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/widgets/issues":
			fmt.Fprint(w, `[{"number":7,"title":"widget","state":"open","labels":[]}]`)
		case "/repos/acme/widgets/pulls", "/repos/acme/widgets/pulls/11":
			pull := map[string]any{"number": 11, "state": "open", "labels": []map[string]string{{"name": "rework"}, {"name": "wip"}}, "head": map[string]any{"ref": "widget", "sha": head, "repo": map[string]string{"full_name": "acme/widgets"}}, "base": map[string]string{"ref": "main"}}
			if r.URL.Path == "/repos/acme/widgets/pulls" {
				json.NewEncoder(w).Encode([]any{pull})
			} else {
				json.NewEncoder(w).Encode(pull)
			}
		case "/repos/acme/widgets/issues/7/comments":
			if r.Method == http.MethodPost {
				writes++
				var p map[string]any
				json.NewDecoder(r.Body).Decode(&p)
				p["author_association"] = "OWNER"
				comments = append(comments, p)
			}
			json.NewEncoder(w).Encode(comments)
		case "/repos/acme/widgets/issues/11/comments", "/repos/acme/widgets/pulls/11/comments", "/repos/acme/widgets/pulls/11/reviews", "/repos/acme/widgets/issues/11/timeline":
			fmt.Fprint(w, `[]`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	b := setup.NewGitHubBackend(server.URL, "token", server.Client())
	before, _ := json.Marshal(comments)
	got := returnedCLI(t, b, "skl implement resume --repo '"+root+"' --item 7 --reviewed-head "+other)
	after, _ := json.Marshal(comments)
	if got.Status != "fix_required" || got.Item == nil || got.Item.Number != 7 || writes != 0 || !bytes.Equal(before, after) {
		t.Fatalf("override mutated active obligation: %+v writes=%d", got, writes)
	}
	fresh := setup.NewGitHubBackend(server.URL, "token", server.Client())
	fresh.BindRepository(github.RepositoryID{Owner: "acme", Name: "widgets"})
	items, err := fresh.ImplementationItems(t.Context())
	if err != nil || items[0].Problem != "" || items[0].Submission.PreviousReviewedHead != reviewed || !items[0].Claimed {
		t.Fatalf("refusal poisoned projection: %+v %v", items, err)
	}
	got = returnedCLI(t, fresh, "skl implement resume --repo '"+root+"' --item 7 --reviewed-head "+reviewed)
	if got.Status != "work_available" || got.Packet.Facts.Implementation.PreviousReviewedHead != reviewed {
		t.Fatalf("original obligation no longer resumes: %+v", got)
	}
	rounds, err := fresh.DispatchRounds(t.Context(), "7")
	if err != nil || !slices.Equal(rounds, []workflow.DispatchRound{round}) {
		t.Fatalf("resume replaced round: %+v %v", rounds, err)
	}
}
