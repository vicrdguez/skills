package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/setup"
	"github.com/vicrdguez/skills/workflow"
)

type waitingMemory struct {
	implementationMemory
	reads, claims int
	observe       func(context.Context) error
	claim         func(context.Context) error
}

func (b *waitingMemory) ImplementationItems(ctx context.Context, repo github.RepositoryID) ([]workflow.ImplementationItem, error) {
	b.reads++
	if b.observe != nil {
		if err := b.observe(ctx); err != nil {
			return nil, err
		}
	}
	return b.implementationMemory.ImplementationItems(ctx, repo)
}

func (b *waitingMemory) ClaimImplementation(ctx context.Context, repo github.RepositoryID, item workflow.ImplementationItem) error {
	b.claims++
	if err := b.implementationMemory.ClaimImplementation(ctx, repo, item); err != nil {
		return err
	}
	if b.claim != nil {
		return b.claim(ctx)
	}
	return nil
}

func waitFixture(t *testing.T, lane string) (string, *waitingMemory) {
	t.Helper()
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	b := &waitingMemory{implementationMemory: implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Ready}}, remoteHeads: map[string]string{"main": strings.TrimSpace(runGitOutput(t, root, "rev-parse", "main"))}}}
	if lane == "watchdog" {
		runGit(t, root, "rm", "-r", ".changes/widget")
		runGit(t, root, "commit", "-m", "retire")
		b.work[0].State = workflow.AwaitingReview
		b.work[0].Submission = &workflow.Submission{ID: "11", Head: strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))}
	}
	runGit(t, root, "remote", "rename", "origin", "upstream")
	runGit(t, root, "remote", "add", "origin", "https://github.com/other/widgets.git")
	return root, b
}

func waitingCLI(t *testing.T, ctx context.Context, root, lane string, b *waitingMemory, options ...string) (setup.ImplementationOutput, error) {
	t.Helper()
	var out, stderr bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) { return b, nil }, nil, &out, &stderr)
	args := append([]string{"skl", lane, "next", "--repo", root, "--remote", "upstream"}, options...)
	err := app.RunContext(ctx, args)
	var got setup.ImplementationOutput
	if err != nil && out.Len() != 0 {
		t.Fatalf("operational failure wrote an outcome: %s", &out)
	}
	if err == nil {
		if decodeErr := json.Unmarshal(out.Bytes(), &got); decodeErr != nil {
			t.Fatalf("not one JSON outcome: %s: %v", &out, decodeErr)
		}
	}
	if got.Packet != nil {
		dir := ""
		if lane == "implement" {
			dir = got.Packet.Facts.Implementation.ResultDirectory
		} else {
			dir = got.Packet.Facts.Watchdog.ResultDirectory
		}
		t.Cleanup(func() { os.RemoveAll(dir) })
	}
	return got, err
}

func TestNextWaitNewlyAvailable(t *testing.T) {
	for _, lane := range []string{"implement", "watchdog"} {
		for _, options := range []string{"--wait", "--wait=2m --poll 5s", "--wait 2m --poll=5s"} {
			t.Run(lane+"/"+options, func(t *testing.T) {
				root, b := waitFixture(t, lane)
				state := b.work[0].State
				b.work[0].State = workflow.NeedsHuman
				synctest.Test(t, func(t *testing.T) {
					start := time.Now()
					b.observe = func(context.Context) error {
						if b.reads == 1 {
							time.Sleep(2 * time.Second)
						}
						if b.reads == 2 {
							b.work[0].State = state
						}
						return nil
					}
					elapsed := 7 * time.Second
					if options == "--wait" {
						elapsed = 32 * time.Second
					}
					got, err := waitingCLI(t, t.Context(), root, lane, b, strings.Fields(options)...)
					if err != nil || got.Status != "work_available" || got.Packet == nil || !got.Item.Claimed || b.claims != 1 || b.reads != 3 || time.Since(start) != elapsed || b.repository.Owner != "acme" {
						t.Fatalf("got %#v, err %v, reads %d claims %d elapsed %s repo %#v", got, err, b.reads, b.claims, time.Since(start), b.repository)
					}
					for _, wanted := range []string{"--wait 15m0s", "--poll 30s"} {
						if options != "--wait" {
							wanted = map[string]string{"--wait 15m0s": "--wait 2m0s", "--poll 30s": "--poll 5s"}[wanted]
						}
						if !strings.Contains(got.ContinuationCommand, wanted) {
							t.Fatalf("continuation %q lacks effective option %q", got.ContinuationCommand, wanted)
						}
					}
				})
			})
		}
	}
}

func TestNextWaitDurationsAndIdleTimeout(t *testing.T) {
	for _, lane := range []string{"implement", "watchdog"} {
		for _, tc := range []struct {
			name    string
			options []string
			elapsed time.Duration
			poll    time.Duration
			reads   int
		}{
			{"default", []string{"--wait"}, 15 * time.Minute, 30 * time.Second, 30},
			{"remaining", []string{"--wait", "45s", "--poll", "30s"}, 45 * time.Second, 30 * time.Second, 2},
			{"equals", []string{"--wait=2m", "--poll=5s"}, 2 * time.Minute, 5 * time.Second, 24},
			{"long poll", []string{"--wait", "2s", "--poll=1m"}, 2 * time.Second, time.Minute, 1},
		} {
			t.Run(lane+"/"+tc.name, func(t *testing.T) {
				root := proposalRepository(t)
				runGit(t, root, "remote", "rename", "origin", "upstream")
				b := &waitingMemory{}
				tmp := t.TempDir()
				t.Setenv("TMPDIR", tmp)
				before := runGitOutput(t, root, "status", "--porcelain", "--untracked-files=all")
				synctest.Test(t, func(t *testing.T) {
					start := time.Now()
					b.observe = func(context.Context) error {
						if elapsed := time.Since(start); elapsed != time.Duration(b.reads-1)*tc.poll {
							t.Fatalf("poll %d at %s", b.reads, elapsed)
						}
						return nil
					}
					got, err := waitingCLI(t, t.Context(), root, lane, b, tc.options...)
					if err != nil || got.Status != "idle_timeout" || got.Packet != nil || got.Item != nil || !strings.Contains(got.Reason, "not global completion") || b.reads != tc.reads || b.claims != 0 || time.Since(start) != tc.elapsed {
						t.Fatalf("got %#v err %v reads %d claims %d elapsed %s", got, err, b.reads, b.claims, time.Since(start))
					}
				})
				files, err := os.ReadDir(tmp)
				if err != nil || len(files) != 0 {
					t.Fatalf("empty wait wrote private state: %v %v", files, err)
				}
				if after := runGitOutput(t, root, "status", "--porcelain", "--untracked-files=all"); after != before {
					t.Fatalf("empty wait changed repository: %s", after)
				}
			})
		}
	}
}

func TestNextWaitInvalidOptionsBeforeBackend(t *testing.T) {
	root := proposalRepository(t)
	for _, lane := range []string{"implement", "watchdog"} {
		for _, tc := range []struct{ option, flag string }{
			{"--wait=", "wait"}, {"--wait 0s", "wait"}, {"--wait -1s", "wait"},
			{"--wait nonsense", "wait"}, {"--wait 15", "wait"}, {"--wait 999999999999999999999h", "wait"},
			{"--wait --poll", "poll"}, {"--wait --poll 0s", "poll"}, {"--wait --poll -1s", "poll"}, {"--wait --poll nonsense", "poll"},
			{"--poll=", "poll"}, {"--poll 0", "poll"}, {"--poll 15", "poll"}, {"--poll 999999999999999999999h", "poll"},
		} {
			t.Run(lane+"/"+tc.option, func(t *testing.T) {
				calls := 0
				var out bytes.Buffer
				app := newApp(func(github.RepositoryID) (setup.Backend, error) { calls++; return &waitingMemory{}, nil }, nil, &out, &out)
				args := append([]string{"skl", lane, "next", "--repo", root, "--after", "eyJ2IjoxLCJvd25lciI6ImFjbWUiLCJyZXBvc2l0b3J5Ijoid2lkZ2V0cyIsImxhbmUiOiJpbXBsZW1lbnQiLCJpdGVtIjoiNyIsInJvdW5kIjoicHJldmlvdXMifQ"}, strings.Fields(tc.option)...)
				err := app.Run(args)
				if err == nil || !strings.Contains(err.Error(), tc.flag) || calls != 0 {
					t.Fatalf("err %v backend constructions %d", err, calls)
				}
			})
		}
	}
}

func TestNextWaitAvailableImmediately(t *testing.T) {
	for _, lane := range []string{"implement", "watchdog"} {
		for _, option := range []string{"", "--poll=5s", "--wait", "--wait -poll 5s"} {
			t.Run(lane+"/"+option, func(t *testing.T) {
				root, b := waitFixture(t, lane)
				synctest.Test(t, func(t *testing.T) {
					start := time.Now()
					got, err := waitingCLI(t, t.Context(), root, lane, b, strings.Fields(option)...)
					if err != nil || got.Status != "work_available" || got.Packet == nil || got.Item.Number != 7 || !got.Item.Claimed || b.reads != 2 || b.claims != 1 || time.Since(start) != 0 || b.repository.Owner != "acme" {
						t.Fatalf("claim = %#v err %v reads %d claims %d elapsed %s", got, err, b.reads, b.claims, time.Since(start))
					}
					remote := ""
					if lane == "implement" {
						remote = got.Packet.Facts.Implementation.Remote
					} else {
						remote = got.Packet.Facts.Watchdog.Remote
					}
					if remote != "upstream" {
						t.Fatalf("packet lost remote: %s", remote)
					}
					if strings.Contains(option, "--wait") {
						return
					}
					got, err = waitingCLI(t, t.Context(), root, lane, b, strings.Fields(option)...)
					if err != nil || got.Status != "no_work" || got.Packet != nil || got.Item != nil || b.reads != 3 || b.claims != 1 || time.Since(start) != 0 {
						t.Fatalf("empty immediate = %#v err %v", got, err)
					}
				})
			})
		}
	}
}

func TestNextWaitCancellation(t *testing.T) {
	for _, lane := range []string{"implement", "watchdog"} {
		for _, point := range []string{"before selection", "during sleep", "external deadline", "uncertain claim", "successful claim"} {
			t.Run(lane+"/"+point, func(t *testing.T) {
				root, b := waitFixture(t, lane)
				synctest.Test(t, func(t *testing.T) {
					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					start := time.Now()
					switch point {
					case "before selection":
						cancel()
					case "during sleep":
						b.work = nil
						go func() { time.Sleep(5 * time.Second); cancel() }()
					case "external deadline":
						b.work = nil
						ctx, cancel = context.WithTimeout(ctx, 5*time.Second)
						defer cancel()
					case "uncertain claim":
						b.claim = func(context.Context) error { cancel(); return ctx.Err() }
						b.observe = func(context.Context) error { return ctx.Err() }
					case "successful claim":
						b.claim = func(context.Context) error { cancel(); return nil }
					}
					got, err := waitingCLI(t, ctx, root, lane, b, "--wait")
					if point == "successful claim" {
						if err != nil || got.Status != "work_available" || got.Packet == nil || !got.Item.Claimed || b.claims != 1 {
							t.Fatalf("cancellation masked successful claim: %#v %v", got, err)
						}
						return
					}
					interruption := context.Canceled
					if point == "external deadline" {
						interruption = context.DeadlineExceeded
					}
					if !errors.Is(err, interruption) || !strings.Contains(err.Error(), "interrupted") || got.Status != "" {
						t.Fatalf("cancellation became outcome: %#v %v", got, err)
					}
					switch point {
					case "before selection":
						if b.reads != 0 || b.claims != 0 || time.Since(start) != 0 {
							t.Fatalf("effects after cancellation: reads %d claims %d elapsed %s", b.reads, b.claims, time.Since(start))
						}
					case "during sleep", "external deadline":
						if b.reads != 1 || b.claims != 0 || time.Since(start) != 5*time.Second {
							t.Fatalf("continued after sleep cancellation: reads %d claims %d elapsed %s", b.reads, b.claims, time.Since(start))
						}
					case "uncertain claim":
						wantReads := 2
						if lane == "watchdog" {
							wantReads = 1
						}
						if b.reads != wantReads || b.claims != 1 || !b.work[0].Claimed || time.Since(start) != 0 || !strings.Contains(err.Error(), "inspect") || !strings.Contains(err.Error(), "resume") {
							t.Fatalf("uncertain claim retried or lost: %v reads %d claims %d work %#v", err, b.reads, b.claims, b.work)
						}
					}
				})
			})
		}
	}
}

func TestNextWaitLateObservation(t *testing.T) {
	for _, lane := range []string{"implement", "watchdog"} {
		for _, result := range []string{"claim", "empty", "error", "refusal"} {
			t.Run(lane+"/"+result, func(t *testing.T) {
				root, b := waitFixture(t, lane)
				work := b.work
				b.work = nil
				failure := errors.New("forge unavailable")
				synctest.Test(t, func(t *testing.T) {
					start := time.Now()
					b.observe = func(ctx context.Context) error {
						if _, ok := ctx.Deadline(); ok {
							t.Fatal("idle window became a request deadline")
						}
						if b.reads != 2 {
							return nil
						}
						if time.Since(start) != 30*time.Second {
							t.Fatalf("selection started at %s", time.Since(start))
						}
						time.Sleep(20 * time.Second)
						switch result {
						case "claim":
							b.work = work
						case "error":
							return failure
						case "refusal":
							return workflow.Refuse("invalid evidence; inspect and repair")
						}
						return nil
					}
					got, err := waitingCLI(t, context.Background(), root, lane, b, "--wait", "45s", "--poll", "30s")
					wantReads, wantClaims := 2, 0
					switch result {
					case "claim":
						wantReads, wantClaims = 3, 1
						if err != nil || got.Status != "work_available" || got.Packet == nil || !got.Item.Claimed {
							t.Fatalf("late claim lost: %#v %v", got, err)
						}
					case "empty":
						if err != nil || got.Status != "idle_timeout" {
							t.Fatalf("late empty: %#v %v", got, err)
						}
					case "error":
						if !errors.Is(err, failure) || got.Status != "" {
							t.Fatalf("late failure lost: %#v %v", got, err)
						}
					case "refusal":
						if err != nil || got.Status != "fix_required" || !strings.Contains(got.Reason, "invalid evidence") {
							t.Fatalf("late refusal lost: %#v %v", got, err)
						}
					}
					if b.reads != wantReads || b.claims != wantClaims || time.Since(start) != 50*time.Second {
						t.Fatalf("late attempt retried: reads %d claims %d elapsed %s", b.reads, b.claims, time.Since(start))
					}
				})
			})
		}
	}
}

func TestNextWaitStopsOnFailure(t *testing.T) {
	for _, lane := range []string{"implement", "watchdog"} {
		for _, observation := range []int{1, 2} {
			for _, failure := range []string{"forge unavailable", "rate limited", "invalid evidence", "ambiguous readback", "contradictory readback"} {
				t.Run(fmt.Sprintf("%s/%d/%s", lane, observation, failure), func(t *testing.T) {
					root, b := waitFixture(t, lane)
					work := b.work
					b.work = nil
					if failure == "invalid evidence" {
						substitute := workflow.ImplementationItem{ID: "8", Branch: "substitute", State: work[0].State, CreatedAt: "2026"}
						prepareSlice(t, root, substitute.Branch)
						if lane == "watchdog" {
							runGit(t, root, "rm", "-r", ".changes/substitute")
							runGit(t, root, "commit", "-m", "retire")
							substitute.Submission = &workflow.Submission{ID: "12", Head: strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD")), CreatedAt: "2026"}
						}
						work = append(work, substitute)
						if lane == "implement" {
							work[0].Problem = "invalid workflow evidence"
						} else {
							work[0].Submission.Head = strings.TrimSpace(runGitOutput(t, root, "rev-parse", "widget^"))
						}
					}
					synctest.Test(t, func(t *testing.T) {
						start := time.Now()
						problem := errors.New(failure + "; inspect and explicitly resume")
						b.observe = func(context.Context) error {
							if b.reads == observation {
								if failure == "forge unavailable" || failure == "rate limited" {
									return problem
								}
								b.work = work
							}
							if b.reads == observation+1 && failure == "ambiguous readback" {
								return problem
							}
							return nil
						}
						if failure == "contradictory readback" {
							b.claim = func(context.Context) error { b.work[0].Problem = "changed during Claim"; return nil }
						}
						got, err := waitingCLI(t, t.Context(), root, lane, b, "--wait")
						if failure == "invalid evidence" || failure == "contradictory readback" {
							if err != nil || got.Status != "fix_required" || got.Reason == "" {
								t.Fatalf("lost refusal: %#v %v", got, err)
							}
						} else if !errors.Is(err, problem) || got.Status != "" {
							t.Fatalf("lost operational error: %#v %v", got, err)
						}
						wantClaims := 0
						if strings.Contains(failure, "readback") {
							wantClaims = 1
							if !b.work[0].Claimed {
								t.Fatal("uncertain Claim released")
							}
						}
						if b.reads != observation+wantClaims || b.claims != wantClaims || time.Since(start) != time.Duration(observation-1)*30*time.Second {
							t.Fatalf("failure retried: reads %d claims %d elapsed %s", b.reads, b.claims, time.Since(start))
						}
					})
				})
			}
		}
	}
}

func TestNextWaitCanonicalEligibility(t *testing.T) {
	for _, lane := range []string{"implement", "watchdog"} {
		t.Run(lane, func(t *testing.T) {
			root := proposalRepository(t)
			runGit(t, root, "remote", "rename", "origin", "upstream")
			// prepareSlice publishes fixture refs under origin; explicit upstream is
			// still the selected Backend and packet remote throughout polling.
			b := &waitingMemory{implementationMemory: implementationMemory{remoteHeads: map[string]string{"main": strings.TrimSpace(runGitOutput(t, root, "rev-parse", "main"))}}}
			work := []workflow.ImplementationItem{
				{ID: "1", State: workflow.Ready, CreatedAt: "2000", Blockers: []workflow.WorkItemID{"9"}},
				{ID: "2", State: workflow.Ready, CreatedAt: "2022"},
				{ID: "3", State: workflow.Rework, CreatedAt: "2024"},
				{ID: "4", State: workflow.AwaitingReview, CreatedAt: "2000"},
				{ID: "5", State: workflow.AwaitingReview, CreatedAt: "2026"},
				{ID: "6", State: workflow.Ready, CreatedAt: "1990", Claimed: true},
				{ID: "7", State: workflow.NeedsHuman, CreatedAt: "1990", ResumeState: workflow.Ready},
				{ID: "9", State: workflow.ReadyForMerge},
			}
			if lane == "watchdog" {
				work[5].State = workflow.AwaitingReview
				work[6].ResumeState = workflow.AwaitingReview
			}
			for i := range work {
				item := &work[i]
				item.Branch = fmt.Sprintf("slice-%s", item.ID)
				prepareSlice(t, root, item.Branch)
				if item.State != workflow.Ready && item.ResumeState != workflow.Ready {
					runGit(t, root, "rm", "-r", ".changes/"+item.Branch)
					runGit(t, root, "commit", "-m", "retire")
					head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
					number, err := strconv.Atoi(string(item.ID))
					if err != nil {
						t.Fatal(err)
					}
					item.Submission = &workflow.Submission{ID: workflow.SubmissionID(strconv.Itoa(number + 100)), Head: head, PreviousReviewedHead: head, Base: "main", State: item.State, Claimed: item.Claimed, CreatedAt: "2025"}
					if item.ID == "5" {
						item.Submission.CreatedAt = "2023"
					}
					if item.ID == "6" {
						item.Submission.CreatedAt = "1990"
						item.Submission.ReviewedHead = head
					}
				}
			}
			before, err := json.Marshal(work)
			if err != nil {
				t.Fatal(err)
			}
			wants := []int{3, 2}
			if lane == "watchdog" {
				wants = []int{5, 4}
			}
			synctest.Test(t, func(t *testing.T) {
				for _, want := range wants {
					b.work = nil
					firstRead, claims := b.reads+1, b.claims
					start := time.Now()
					b.observe = func(context.Context) error {
						if b.reads == firstRead+1 {
							b.work = work
						}
						return nil
					}
					got, err := waitingCLI(t, t.Context(), root, lane, b, "--wait")
					if err != nil || got.Item == nil || got.Item.Number != want || !got.Item.Claimed || b.reads != firstRead+2 || b.claims != claims+1 || time.Since(start) != 30*time.Second {
						t.Fatalf("ordering: %#v %v reads %d claims %d", got, err, b.reads, b.claims)
					}
				}
				b.observe = nil
				got, err := waitingCLI(t, t.Context(), root, lane, b, "--wait=45s", "--poll=30s")
				if err != nil || got.Status != "idle_timeout" || b.claims != 2 {
					t.Fatalf("ineligible work claimed: %#v %v", got, err)
				}
				if lane == "implement" {
					firstRead := b.reads + 1
					b.observe = func(context.Context) error {
						if b.reads == firstRead+1 {
							work[7].Submission.Merged = true
						}
						return nil
					}
					got, err = waitingCLI(t, t.Context(), root, lane, b, "--wait")
					if err != nil || got.Item == nil || got.Item.Number != 1 || b.reads != firstRead+2 || b.claims != 3 {
						t.Fatalf("merged blocker did not release dependent: %#v %v", got, err)
					}
				}
			})
			var original []workflow.ImplementationItem
			if err := json.Unmarshal(before, &original); err != nil {
				t.Fatal(err)
			}
			for i := range work {
				if lane == "implement" && (i == 0 || i == 1 || i == 2) || lane == "watchdog" && (i == 3 || i == 4) {
					continue
				}
				if i == 7 && lane == "implement" {
					original[i].Submission.Merged = true
				}
				if !reflect.DeepEqual(original[i], work[i]) {
					t.Fatalf("ineligible item mutated: %#v -> %#v", original[i], work[i])
				}
			}
			if _, err := os.Stat(filepath.Join(root, ".worktrees")); !os.IsNotExist(err) {
				t.Fatalf("waiting launched/prepared a worker: %v", err)
			}
		})
	}
}

func TestNextWaitHelp(t *testing.T) {
	for _, lane := range []string{"implement", "watchdog"} {
		t.Run(lane, func(t *testing.T) {
			var out bytes.Buffer
			app := newApp(func(github.RepositoryID) (setup.Backend, error) {
				t.Fatal("help constructed a Backend")
				return nil, nil
			}, nil, &out, &out)
			if err := app.Run([]string{"skl", lane, "next", "--help"}); err != nil {
				t.Fatal(err)
			}
			for _, syntax := range []string{"--wait (15m)", "--wait=2m", "--wait 2m", "--poll", "30s", "idle_timeout", "not global completion"} {
				if !strings.Contains(out.String(), syntax) {
					t.Errorf("help lacks %q: %s", syntax, &out)
				}
			}
		})
	}
}

func completedWaitFixture(t *testing.T, lane string) (string, *waitingMemory, string) {
	t.Helper()
	root, b := waitFixture(t, lane)
	start, err := waitingCLI(t, t.Context(), root, lane, b)
	if err != nil {
		t.Fatal(err)
	}
	if lane == "implement" {
		dir := start.Packet.Facts.Implementation.ResultDirectory
		os.WriteFile(filepath.Join(dir, "decision.md"), []byte("pause"), 0600)
		got := returnedCLI(t, b, "skl implement needs-human --repo '"+root+"' --remote upstream --item 7 --reason mandatory_rule --decision '"+filepath.Join(dir, "decision.md")+"'")
		if got.Status != "needs_human" {
			t.Fatalf("fixture pause: %+v", got)
		}
	} else {
		dir := start.Packet.Facts.Watchdog.ResultDirectory
		os.WriteFile(filepath.Join(dir, "summary.md"), []byte("pause"), 0600)
		b.remoteHeads["widget"] = b.work[0].Submission.Head
		got := returnedCLI(t, b, "skl watchdog submit --repo '"+root+"' --remote upstream --item 7 --reviewed-head "+b.work[0].Submission.Head+" --verdict needs-human --summary '"+filepath.Join(dir, "summary.md")+"'")
		if got.Status != "needs_human" {
			t.Fatalf("fixture review pause: %+v", got)
		}
	}
	b.reads, b.claims, b.roundReads = 0, 0, 0
	return root, b, strings.Trim(strings.Fields(start.ContinuationCommand)[4], "'")
}

func TestVerifiedContinuationWaitingOptionsAndFreshReuse(t *testing.T) {
	for _, lane := range []string{"implement", "watchdog"} {
		for _, tc := range []struct {
			options      string
			window, poll time.Duration
			polls        int
		}{
			{"", 0, 0, 1}, {"--wait", 15 * time.Minute, 30 * time.Second, 30}, {"--wait=2m", 2 * time.Minute, 30 * time.Second, 4}, {"--wait 2m --poll 5s", 2 * time.Minute, 5 * time.Second, 24}, {"--wait 2m --poll=5s", 2 * time.Minute, 5 * time.Second, 24}, {"--poll=5s", 0, 0, 1},
		} {
			t.Run(lane+tc.options, func(t *testing.T) {
				root, b, reference := completedWaitFixture(t, lane)
				synctest.Test(t, func(t *testing.T) {
					// Worker time and evidence observation are outside each fresh idle window.
					time.Sleep(time.Hour)
					options := append([]string{"--after", reference}, strings.Fields(tc.options)...)
					for attempt := 0; attempt < 2; attempt++ {
						b.reads = 0
						start := time.Now()
						b.observe = func(context.Context) error {
							if b.reads == 1 {
								time.Sleep(7 * time.Second)
							} else if elapsed := time.Since(start) - 7*time.Second; elapsed != time.Duration(b.reads-2)*tc.poll {
								t.Fatalf("poll %d at %s", b.reads, elapsed)
							}
							return nil
						}
						got, err := waitingCLI(t, t.Context(), root, lane, b, options...)
						want := "no_work"
						if tc.window > 0 {
							want = "idle_timeout"
						}
						if err != nil || got.Status != want || got.PreviousHandoff == nil || b.claims != 0 || b.reads != tc.polls+1 || time.Since(start) != tc.window+7*time.Second {
							t.Fatalf("verified waiting: %+v err=%v reads=%d elapsed=%s", got, err, b.reads, time.Since(start))
						}
						time.Sleep(2 * time.Hour)
					}
					b.observe = nil
					successor := workflow.ImplementationItem{ID: "8", Branch: "widget", State: workflow.Ready}
					if lane == "watchdog" {
						successor.State = workflow.AwaitingReview
						successor.Submission = &workflow.Submission{ID: "12", Head: b.work[0].Submission.Head}
					}
					b.work = append(b.work, successor)
					got, err := waitingCLI(t, t.Context(), root, lane, b, options...)
					if err != nil || got.Status != "work_available" || got.Item.Number != 8 || got.PreviousHandoff.Number != 7 || b.claims != 1 {
						t.Fatalf("reuse replayed empty response: %+v %v", got, err)
					}
					if tc.window > 0 && (!strings.Contains(got.ContinuationCommand, "--wait "+tc.window.String()) || !strings.Contains(got.ContinuationCommand, "--poll "+tc.poll.String())) {
						t.Fatalf("lost options: %s", got.ContinuationCommand)
					}
					if tc.options == "--poll=5s" && (!strings.Contains(got.ContinuationCommand, "--poll 5s") || strings.Contains(got.ContinuationCommand, "--wait")) {
						t.Fatalf("poll enabled waiting: %s", got.ContinuationCommand)
					}
				})
			})
		}
	}
}

func TestVerifiedContinuationPostClaimFailureRequiresKnownRecovery(t *testing.T) {
	for _, lane := range []string{"implement", "watchdog"} {
		for _, fault := range []string{"applied round unreadable", "unapplied round", "packet allocation"} {
			t.Run(lane+fault, func(t *testing.T) {
				root, b, reference := completedWaitFixture(t, lane)
				successor := workflow.ImplementationItem{ID: "8", Branch: "widget", State: workflow.Ready}
				if lane == "watchdog" {
					successor.State = workflow.AwaitingReview
					successor.Submission = &workflow.Submission{ID: "12", Head: b.work[0].Submission.Head}
				}
				b.work = append(b.work, successor)
				switch fault {
				case "applied round unreadable":
					b.afterRound = func(round workflow.DispatchRound) error {
						if round.Item == "8" {
							return errors.New("lost dispatch response")
						}
						return nil
					}
				case "unapplied round":
					b.roundWriteError = errors.New("dispatch write unapplied")
				case "packet allocation":
					file := filepath.Join(root, "not-a-directory")
					os.WriteFile(file, []byte("block allocation"), 0600)
					t.Setenv("TMPDIR", file)
				}
				got, err := waitingCLI(t, t.Context(), root, lane, b, "--after", reference, "--wait=1s", "--poll=1ms")
				if err == nil || got.Status != "" || !b.work[1].Claimed || b.claims != 1 || !strings.Contains(err.Error(), "--item 8") || !strings.Contains(err.Error(), "stop") || !strings.Contains(err.Error(), "resume") {
					t.Fatalf("uncertain dispatch lacks known recovery: %+v %v claims=%d", got, err, b.claims)
				}
				if fault != "packet allocation" {
					b.afterRound = nil
					b.roundWriteError = nil
					got = returnedCLI(t, b, "skl "+lane+" resume --repo '"+root+"' --remote upstream --item 8")
					if got.Status != "work_available" || len(b.rounds["8"]) != 1 || !b.work[1].Claimed {
						t.Fatalf("explicit recovery replaced round: %+v %+v", got, b.rounds)
					}
				}
			})
		}
	}
}

func TestVerifiedContinuationOperationalFailuresAndCancellation(t *testing.T) {
	for _, lane := range []string{"implement", "watchdog"} {
		for _, fault := range []string{"evidence read", "initial selection", "later poll", "cancel before", "cancel waiting"} {
			t.Run(lane+fault, func(t *testing.T) {
				root, b, reference := completedWaitFixture(t, lane)
				synctest.Test(t, func(t *testing.T) {
					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					switch fault {
					case "evidence read":
						b.roundReadError = errors.New("forge evidence unavailable")
					case "initial selection":
						b.observe = func(context.Context) error {
							if b.reads == 2 {
								return errors.New("forge selection unavailable")
							}
							return nil
						}
					case "later poll":
						b.observe = func(context.Context) error {
							if b.reads == 3 {
								return errors.New("forge poll unavailable")
							}
							return nil
						}
					case "cancel before":
						cancel()
					case "cancel waiting":
						go func() { time.Sleep(5 * time.Second); cancel() }()
					}
					before, _ := json.Marshal([]any{b.work, b.rounds})
					got, err := waitingCLI(t, ctx, root, lane, b, "--after", reference, "--wait=1m", "--poll=10s")
					after, _ := json.Marshal([]any{b.work, b.rounds})
					if err == nil || got.Status != "" || b.claims != 0 || !bytes.Equal(before, after) || b.reads > 3 {
						t.Fatalf("failure became idle/retry: %+v %v reads=%d claims=%d", got, err, b.reads, b.claims)
					}
					if fault == "evidence read" && (!strings.Contains(err.Error(), "--item 7") || !strings.Contains(err.Error(), "stop")) {
						t.Fatalf("evidence error lost recovery: %v", err)
					}
				})
			})
		}
	}
}

type lostDispatchOutput struct{}

func (lostDispatchOutput) Write([]byte) (int, error) { return 0, errors.New("output connection lost") }

func TestContinuationLostResponseRetainsSelectionAndRequiresInspection(t *testing.T) {
	for _, lane := range []string{"implement", "watchdog"} {
		for _, available := range []bool{false, true} {
			t.Run(lane+fmt.Sprint(available), func(t *testing.T) {
				root, b, reference := completedWaitFixture(t, lane)
				if available {
					successor := workflow.ImplementationItem{ID: "8", Branch: "widget", State: workflow.Ready}
					if lane == "watchdog" {
						successor.State = workflow.AwaitingReview
						successor.Submission = &workflow.Submission{ID: "12", Head: b.work[0].Submission.Head}
					}
					b.work = append(b.work, successor)
				}
				synctest.Test(t, func(t *testing.T) {
					app := newApp(func(github.RepositoryID) (setup.Backend, error) { return b, nil }, nil, lostDispatchOutput{}, &bytes.Buffer{})
					err := app.Run([]string{"skl", lane, "next", "--repo", root, "--remote", "upstream", "--after", reference, "--wait=1s"})
					want := 0
					if available {
						want = 1
					}
					if err == nil || !strings.Contains(err.Error(), "stop") || b.claims != want || available && !b.work[1].Claimed {
						t.Fatalf("lost response replayed or abandoned: %v claims=%d", err, b.claims)
					}
					if available && !strings.Contains(err.Error(), "--item 8") {
						t.Fatalf("known successor lost: %v", err)
					}
				})
			})
		}
	}
}

func TestNextHelpRequiresStopOnAmbiguousResponses(t *testing.T) {
	for _, lane := range []string{"implement", "watchdog"} {
		var output bytes.Buffer
		app := newApp(func(github.RepositoryID) (setup.Backend, error) { t.Fatal("help reached backend"); return nil, nil }, nil, &output, &output)
		if err := app.Run([]string{"skl", lane, "next", "--help"}); err != nil {
			t.Fatal(err)
		}
		for _, wanted := range []string{"stop", "inspect", "resume", "next --after", "idle_timeout"} {
			if !strings.Contains(output.String(), wanted) {
				t.Fatalf("help lacks %s: %s", wanted, &output)
			}
		}
	}
}

func TestVerifiedContinuationClaimsNewlyEligibleWorkDuringWaiting(t *testing.T) {
	for _, lane := range []string{"implement", "watchdog"} {
		for _, options := range []string{"--wait", "--wait 2m --poll 5s", "--wait 2m --poll=5s"} {
			t.Run(lane+options, func(t *testing.T) {
				root, b, reference := completedWaitFixture(t, lane)
				successor := workflow.ImplementationItem{ID: "8", Branch: "widget", State: workflow.NeedsHuman}
				state := workflow.Ready
				if lane == "watchdog" {
					state = workflow.AwaitingReview
					successor.Submission = &workflow.Submission{ID: "12", Head: b.work[0].Submission.Head}
				}
				b.work = append(b.work, successor)
				synctest.Test(t, func(t *testing.T) {
					start := time.Now()
					b.observe = func(context.Context) error {
						if b.reads == 2 {
							time.Sleep(2 * time.Second)
						}
						if b.reads == 3 {
							b.work[1].State = state
						}
						return nil
					}
					got, err := waitingCLI(t, t.Context(), root, lane, b, append([]string{"--after", reference}, strings.Fields(options)...)...)
					elapsed := 7 * time.Second
					if options == "--wait" {
						elapsed = 32 * time.Second
					}
					if err != nil || got.Status != "work_available" || got.Item.Number != 8 || got.PreviousHandoff.Number != 7 || b.claims != 1 || b.reads != 4 || time.Since(start) != elapsed {
						t.Fatalf("continuation failed to poll current eligibility: %+v %v reads=%d elapsed=%s", got, err, b.reads, time.Since(start))
					}
				})
			})
		}
	}
}
