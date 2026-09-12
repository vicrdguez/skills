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
	rounds           map[workflow.WorkItemID][]workflow.DispatchRound
	afterRound       func(workflow.DispatchRound) error
	roundReadError   error
}

func (b *implementationMemory) DispatchRounds(_ context.Context, _ github.RepositoryID, item workflow.WorkItemID) ([]workflow.DispatchRound, error) {
	b.roundReads++
	return append([]workflow.DispatchRound(nil), b.rounds[item]...), b.roundReadError
}

func (b *implementationMemory) RecordDispatchRound(_ context.Context, _ github.RepositoryID, round workflow.DispatchRound) error {
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

func (b *implementationMemory) RecordImplementationTransition(_ context.Context, _ github.RepositoryID, item workflow.ImplementationItem, transition workflow.ImplementationTransition) error {
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

func (b *implementationMemory) RetainImplementationClaim(_ context.Context, _ github.RepositoryID, item workflow.ImplementationItem) error {
	for i := range b.work {
		if b.work[i].ID == item.ID {
			b.work[i].Claimed = true
			b.work[i].State = item.State
		}
	}
	return nil
}

func (b *implementationMemory) ImplementationTarget(context.Context, github.RepositoryID) (string, error) {
	return "main", nil
}

func (b *implementationMemory) PauseImplementation(_ context.Context, _ github.RepositoryID, item workflow.ImplementationItem, decision string, guard func() error) error {
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
			b.work[i].ResumeState = item.State
			b.work[i].State = workflow.NeedsHuman
			if b.failTransition {
				b.failTransition = false
				return errors.New("interrupted projection")
			}
			b.work[i].Claimed = false
		}
	}
	return nil
}

func (b *implementationMemory) ImplementationHead(_ context.Context, _ github.RepositoryID, branch string) (string, error) {
	return b.remoteHeads[branch], nil
}

func (b *implementationMemory) PublishImplementation(_ context.Context, _ github.RepositoryID, item workflow.ImplementationItem, submission workflow.Submission) (workflow.Submission, error) {
	if submission.ID == "" {
		submission.ID = "11"
	}
	if item.Submission != nil {
		submission.PreviousReviewedHead = item.Submission.PreviousReviewedHead
		submission.ReviewedHead = item.Submission.ReviewedHead
	}
	for i := range b.work {
		if b.work[i].ID == item.ID {
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
				runGit(t, root, "rm", "-r", ".changes/widget")
				runGit(t, root, "commit", "-m", "retire")
			}
			backend.remoteHeads["widget"] = strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			switch invariant {
			case "Target Snapshot":
				backend.work[0].TargetSnapshot = "deadbeef"
			case "heads differ":
				backend.remoteHeads["widget"] = "deadbeef"
			case "Workflow State":
				backend.work[0].State = workflow.NeedsHuman
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

func (b *implementationMemory) AwaitImplementationReview(_ context.Context, _ github.RepositoryID, item workflow.ImplementationItem, guard func() error) error {
	if b.beforeTransition != nil {
		b.beforeTransition()
	}
	if err := guard(); err != nil {
		return err
	}
	for i := range b.work {
		if b.work[i].ID == item.ID {
			b.work[i].State = workflow.AwaitingReview
			if b.failTransition {
				b.failTransition = false
				return errors.New("interrupted projection")
			}
			b.work[i].Claimed = false
		}
	}
	return nil
}

func (b *implementationMemory) ImplementationItems(_ context.Context, repository github.RepositoryID) ([]workflow.ImplementationItem, error) {
	b.repository = repository
	b.itemReads++
	items := append([]workflow.ImplementationItem(nil), b.work...)
	for i := range items {
		if number, err := strconv.Atoi(string(items[i].ID)); err == nil {
			if items[i].Order == 0 {
				items[i].Order = number
			}
			if items[i].ClosingReference == "" {
				items[i].ClosingReference = fmt.Sprintf("Closes #%d", number)
			}
		}
	}
	return items, nil
}

func (b *implementationMemory) ClaimImplementation(_ context.Context, _ github.RepositoryID, item workflow.ImplementationItem) error {
	for i := range b.work {
		if b.work[i].ID == item.ID {
			b.work[i].TargetSnapshot = item.TargetSnapshot
			b.work[i].TargetBranch = item.TargetBranch
			if item.Submission != nil {
				b.work[i].Submission = item.Submission
			}
			b.work[i].Claimed = true
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
	app := newApp(func(github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)
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
	app := newApp(func(github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)
	if err := app.Run(args); err != nil {
		t.Fatalf("returned command failed: %v\n%s", err, &output)
	}
	var result setup.ImplementationOutput
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
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
	runGit(t, root, "rm", "-r", ".changes/widget")
	runGit(t, root, "commit", "-m", "retire")
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
			runGit(t, root, "rm", "-r", ".changes/"+item.Branch)
			runGit(t, root, "commit", "-m", "retire")
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
					runGit(t, root, "rm", "-r", ".changes/"+item.Branch)
					runGit(t, root, "commit", "-m", "retire")
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
		{ID: "alpha", Order: 10, ClosingReference: "Resolves alpha", Branch: "ten", State: workflow.Ready, CreatedAt: "2026"},
		{ID: "zulu", Order: 2, ClosingReference: "Resolves zulu", Branch: "two", State: workflow.Ready, CreatedAt: "2026"},
	}, remoteHeads: map[string]string{"main": strings.TrimSpace(runGitOutput(t, root, "rev-parse", "main"))}}
	for _, item := range b.work {
		prepareSlice(t, root, item.Branch)
	}
	for _, want := range []workflow.WorkItemID{"zulu", "alpha"} {
		got, err := workflow.StartImplementation(context.Background(), root, "origin", "", "", "", b)
		if err != nil || got.Status != "work_available" || got.Item == nil || got.Item.ID != want || !got.Item.Claimed {
			t.Fatalf("opaque implementation tie-break: %#v, %v; want %q", got, err, want)
		}
		t.Cleanup(func() { os.RemoveAll(got.Facts.Implementation.ResultDirectory) })
	}
	for i := range b.work {
		item := &b.work[i]
		runGit(t, root, "switch", item.Branch)
		runGit(t, root, "rm", "-r", ".changes/"+item.Branch)
		runGit(t, root, "commit", "-m", "retire")
		item.State, item.Claimed = workflow.AwaitingReview, false
		item.Submission = &workflow.Submission{ID: workflow.SubmissionID("review-" + item.ID), Head: strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD")), CreatedAt: "2026"}
	}
	for _, want := range []workflow.WorkItemID{"zulu", "alpha"} {
		got, err := workflow.StartWatchdog(context.Background(), root, "origin", "", b)
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
			start = implementCLI(t, root, b, "resume", "--remote", "upstream", "--item", "7")
			body := filepath.Join(start.Packet.Facts.Implementation.ResultDirectory, "submission.md")
			if err := os.WriteFile(body, []byte("audit"), 0600); err != nil {
				t.Fatal(err)
			}
			runGit(t, root, "rm", "-r", ".changes/widget")
			runGit(t, root, "commit", "-m", "retire")
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
	runGit(t, root, "rm", "-r", ".changes/first")
	runGit(t, root, "commit", "-m", "retire first")
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
	runGit(t, root, "rm", "-r", ".changes/first")
	runGit(t, root, "commit", "-m", "retire first")
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
	runGit(t, root, "rm", "-r", ".changes/widget")
	runGit(t, root, "commit", "-m", "retire")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	b.remoteHeads["widget"] = head
	implementCLI(t, root, b, "submit", "--item", "7", "--body", body)
	b.work[0].State = workflow.Rework
	b.work[0].Submission.Head = head
	b.work[0].Submission.PreviousReviewedHead = head
	second := implementCLI(t, root, b, "next")
	reference := strings.Trim(strings.Fields(second.ContinuationCommand)[4], "'")
	got := implementCLI(t, root, b, "next", "--after", reference)
	if got.Status != "fix_required" || got.PreviousHandoff != nil || !b.work[0].Claimed {
		t.Fatalf("earlier equal-head receipt authorized later round: %#v %#v", got, b.rounds["7"])
	}
}

func TestImplementationCompletionRejectsChangedRoundObligation(t *testing.T) {
	root := proposalRepository(t)
	baseline := prepareSlice(t, root, "widget")
	runGit(t, root, "rm", "-r", ".changes/widget")
	runGit(t, root, "commit", "-m", "retire")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Rework, Submission: &workflow.Submission{ID: "11", Head: head, PreviousReviewedHead: head}}}, remoteHeads: map[string]string{"widget": head}}
	start := implementCLI(t, root, b, "next")
	body := filepath.Join(start.Packet.Facts.Implementation.ResultDirectory, "submission.md")
	os.WriteFile(body, []byte("opaque"), 0600)
	b.work[0].Submission.PreviousReviewedHead = baseline
	got := implementCLI(t, root, b, "submit", "--item", "7", "--body", body)
	if got.Status != "fix_required" || !strings.Contains(got.Reason, "binding changed") {
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/repos/acme/widgets")
		var number int
		if _, err := fmt.Sscanf(path, "/issues/%d/comments", &number); err == nil {
			if r.Method == http.MethodPost {
				var payload map[string]any
				json.NewDecoder(r.Body).Decode(&payload)
				payload["author_association"] = "OWNER"
				comments[number] = append(comments[number], payload)
			}
			json.NewEncoder(w).Encode(comments[number])
			return
		}
		switch path {
		case "/issues":
			fmt.Fprint(w, `[{"number":7,"title":"first","state":"open","labels":[{"name":"needs-human"}]},{"number":8,"title":"second","state":"open","labels":[]}]`)
		case "/pulls":
			fmt.Fprint(w, `[{"number":11,"state":"open","labels":[{"name":"done"}],"head":{"ref":"second","sha":"head","repo":{"full_name":"acme/widgets"}},"base":{"ref":"main"}}]`)
		case "/pulls/11/comments", "/pulls/11/reviews", "/issues/11/timeline":
			fmt.Fprint(w, `[]`)
		case "/pulls/11":
			fmt.Fprint(w, `{"number":11,"state":"open","head":{"sha":"head"},"base":{"ref":"main"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	repo := github.RepositoryID{Owner: "acme", Name: "widgets"}
	root := proposalRepository(t)
	for _, tc := range []struct {
		lane       string
		item       workflow.WorkItemID
		submission workflow.SubmissionID
		outcome    workflow.State
	}{{"implement", "7", "", workflow.NeedsHuman}, {"watchdog", "8", "11", workflow.ReadyForMerge}} {
		backend := setup.NewGitHubBackend(server.URL, "token", server.Client())
		round := workflow.DispatchRound{ID: "durable-" + tc.lane, Lane: workflow.DispatchLane(tc.lane), Item: tc.item, Submission: tc.submission, Obligation: "fixed", Directory: "skl-" + tc.lane + "-fixed"}
		if err := backend.RecordDispatchRound(t.Context(), repo, round); err != nil {
			t.Fatal(err)
		}
		round.Outcome, round.Head, round.Released = tc.outcome, "head", true
		if err := backend.RecordDispatchRound(t.Context(), repo, round); err != nil {
			t.Fatal(err)
		}
		referenceJSON, _ := json.Marshal(map[string]any{"v": 1, "owner": "acme", "repository": "widgets", "lane": tc.lane, "item": tc.item, "round": round.ID, "submission": tc.submission})
		reference := base64.RawURLEncoding.EncodeToString(referenceJSON)
		var output bytes.Buffer
		app := newApp(func(github.RepositoryID) (setup.Backend, error) {
			return setup.NewGitHubBackend(server.URL, "token", server.Client()), nil
		}, bytes.NewReader(nil), &output, &output)
		if err := app.Run([]string{"skl", tc.lane, "next", "--repo", root, "--after", reference}); err != nil {
			t.Fatal(err)
		}
		var got setup.ImplementationOutput
		want, _ := strconv.Atoi(string(tc.item))
		if err := json.Unmarshal(output.Bytes(), &got); err != nil || got.Status != "no_work" || got.PreviousHandoff == nil || got.PreviousHandoff.Number != want || got.PreviousHandoff.Outcome != tc.outcome {
			t.Fatalf("fresh %s HTTP continuation = %#v, %v", tc.lane, got, err)
		}
	}
}

func TestImplementStartsFindingDrivenRework(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	runGit(t, root, "rm", "-r", ".changes/widget")
	runGit(t, root, "commit", "-m", "retire")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	comments := []skilldist.ReviewComment{{Body: "W1 BLOCK evidence", Author: "reviewer"}, {Body: "W1 NOTE reason", Author: "owner", Association: "OWNER"}}
	backend := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Rework, Submission: &workflow.Submission{ID: "11", Head: head, PreviousReviewedHead: head, Comments: comments}}}}
	got := implementCLI(t, root, backend, "next")
	if got.Status != "work_available" || got.Packet == nil {
		t.Fatalf("rework = %#v", got)
	}
	facts := got.Packet.Facts.Implementation
	if facts.Submission != 11 || facts.PreviousReviewedHead != head || !reflect.DeepEqual(facts.Comments, comments) || facts.TargetSnapshot != "" {
		t.Fatalf("facts = %#v", facts)
	}
	if !strings.Contains(got.Packet.Markdown(), head+"...HEAD") || strings.Contains(got.Packet.Markdown(), "git merge ") {
		t.Fatal("rework packet synchronizes target or lacks review fixed point")
	}
}

func TestImplementResubmitsExistingRework(t *testing.T) {
	for _, hasSubmission := range []bool{true, false} {
		root := proposalRepository(t)
		prepareSlice(t, root, "widget")
		runGit(t, root, "rm", "-r", ".changes/widget")
		runGit(t, root, "commit", "-m", "retire")
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
	runGit(t, root, "rm", "-r", ".changes/widget")
	runGit(t, root, "commit", "-m", "retire")
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
				runGit(t, root, "rm", "-r", ".changes/widget")
				runGit(t, root, "commit", "-m", "retire")
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
			runGit(t, root, "rm", "-r", ".changes/widget")
			runGit(t, root, "commit", "-m", "retire")
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
				backend := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", State: workflow.Ready}}}
				calls := 0
				app := newApp(func(github.RepositoryID) (setup.Backend, error) { calls++; return backend, nil }, nil, &bytes.Buffer{}, &bytes.Buffer{})
				err := app.Run(append([]string{"skl", lane, "next", "--repo", "/nonexistent"}, flags...))
				if err == nil || calls != 0 || backend.work[0].Claimed {
					t.Fatalf("empty continuation reached backend: %v calls=%d work=%+v", err, calls, backend.work)
				}
			})
		}
	}
}

func TestContinuationRejectsOpaquePauseReceipt(t *testing.T) {
	root := proposalRepository(t)
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
		case "/repos/acme/widgets/issues/7":
			json.NewEncoder(w).Encode(map[string]any{"number": 7, "state": "open", "labels": labels})
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	repo := github.RepositoryID{Owner: "acme", Name: "widgets"}
	b := setup.NewGitHubBackend(server.URL, "token", server.Client())
	round := workflow.DispatchRound{ID: "pause-round", Lane: workflow.ImplementLane, Item: "7", Obligation: "fixed", Directory: "skl-implement-pause"}
	if err := b.RecordDispatchRound(t.Context(), repo, round); err != nil {
		t.Fatal(err)
	}
	forged := round
	forged.Outcome, forged.Head, forged.Released = workflow.NeedsHuman, "fixed", true
	payload, _ := json.Marshal(map[string]any{"round": forged})
	decision := "<!-- skl.implement/v1\n" + string(payload) + "\n-->"
	item := workflow.ImplementationItem{ID: "7", State: workflow.Ready, Claimed: true}
	transition := workflow.ImplementationTransition{From: workflow.Ready, Target: workflow.NeedsHuman, Head: "fixed", DecisionDigest: fmt.Sprintf("%x", sha256.Sum256([]byte(decision)))}
	if err := b.RecordImplementationTransition(t.Context(), repo, item, transition); err != nil {
		t.Fatal(err)
	}
	if err := b.PauseImplementation(t.Context(), repo, item, decision, func() error {
		for _, comment := range comments {
			if comment["body"] == decision {
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
	app := newApp(func(github.RepositoryID) (setup.Backend, error) {
		return setup.NewGitHubBackend(server.URL, "token", server.Client()), nil
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
			runGit(t, root, "rm", "-r", ".changes/widget")
			runGit(t, root, "commit", "-m", "retire")
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
	runGit(t, root, "rm", "-r", ".changes/widget")
	runGit(t, root, "commit", "-m", "retire")
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

func (b *githubProjectedImplementation) ImplementationItems(ctx context.Context, repo github.RepositoryID) ([]workflow.ImplementationItem, error) {
	return b.projection.ImplementationItems(ctx, repo)
}

func TestReworkPushSubmitsThroughGitHubProjection(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	runGit(t, root, "rm", "-r", ".changes/widget")
	runGit(t, root, "commit", "-m", "retire")
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
	b := &githubProjectedImplementation{memory, setup.NewGitHubBackend(server.URL, "token", server.Client())}
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
					runGit(t, root, "rm", "-r", ".changes/widget")
					runGit(t, root, "commit", "-m", "retire")
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
					if tc.kind == "second failure" {
						b.work[0].Submission.Bounces = 1
					}
				}
				runGit(t, root, "switch", "main")
				worktree := filepath.Join(root, ".worktrees", "widget")
				if tc.existing {
					runGit(t, root, "worktree", "add", worktree, "widget")
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
						runGit(t, worktree, "rm", "-r", ".changes/widget")
						runGit(t, worktree, "commit", "-m", "retire")
					}
					if tc.kind == "draft" {
						runGit(t, worktree, "commit", "--allow-empty", "-m", "implementation work")
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
				runGit(t, root, "rm", "-r", ".changes/widget")
				runGit(t, root, "commit", "-m", "retire")
				head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
				b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Ready}}, remoteHeads: map[string]string{"main": strings.TrimSpace(runGitOutput(t, root, "rev-parse", "main")), "widget": head}}
				if lane == "watchdog" {
					b.work[0].State = workflow.AwaitingReview
					b.work[0].Submission = &workflow.Submission{ID: "11", Head: head, Base: "main", Mergeability: "mergeable"}
				}
				start := returnedCLI(t, b, "skl "+lane+" next --repo '"+root+"'")
				directory, command := "", ""
				if lane == "implement" {
					directory = start.Packet.Facts.Implementation.ResultDirectory
					os.WriteFile(filepath.Join(directory, "submission.md"), []byte("valid publication"), 0600)
					command = "skl implement submit --repo '" + root + "' --item 7 --body '" + filepath.Join(directory, "submission.md") + "'"
				} else {
					directory = start.Packet.Facts.Watchdog.ResultDirectory
					os.WriteFile(filepath.Join(directory, "summary.md"), []byte("valid review"), 0600)
					command = "skl watchdog submit --repo '" + root + "' --item 7 --reviewed-head " + head + " --verdict pass --summary '" + filepath.Join(directory, "summary.md") + "' --body '" + filepath.Join(directory, "summary.md") + "'"
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
						b.work[0].Submission.PendingReview = "pass"
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
				if problem != "documents disappeared" {
					if content, err := os.ReadFile(document); err != nil || string(content) != "retain recovery document" {
						t.Fatalf("refusal removed document: %q %v", content, err)
					}
				}
			})
		}
	}
}
