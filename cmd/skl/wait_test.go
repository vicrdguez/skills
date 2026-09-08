package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/vicrdguez/skills/setup"
	"github.com/vicrdguez/skills/workflow"
)

type waitingMemory struct {
	implementationMemory
	reads, claims int
	observe       func(context.Context) error
	claim         func(context.Context) error
}

func (b *waitingMemory) ImplementationItems(ctx context.Context, repo workflow.RepositoryID) ([]workflow.ImplementationItem, error) {
	b.reads++
	if b.observe != nil {
		if err := b.observe(ctx); err != nil {
			return nil, err
		}
	}
	return b.implementationMemory.ImplementationItems(ctx, repo)
}

func (b *waitingMemory) ClaimImplementation(ctx context.Context, repo workflow.RepositoryID, item workflow.ImplementationItem) error {
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
	b := &waitingMemory{implementationMemory: implementationMemory{work: []workflow.ImplementationItem{{Number: 7, Branch: "widget", State: workflow.Ready}}, remoteHeads: map[string]string{"main": strings.TrimSpace(runGitOutput(t, root, "rev-parse", "main"))}}}
	if lane == "watchdog" {
		runGit(t, root, "rm", "-r", ".changes/widget")
		runGit(t, root, "commit", "-m", "retire")
		b.work[0].State = workflow.AwaitingReview
		b.work[0].Submission = &workflow.Submission{Number: 11, Head: strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))}
	}
	runGit(t, root, "remote", "rename", "origin", "upstream")
	runGit(t, root, "remote", "add", "origin", "https://github.com/other/widgets.git")
	return root, b
}

func waitingCLI(t *testing.T, ctx context.Context, root, lane string, b *waitingMemory, options ...string) (workflow.ImplementationOutcome, error) {
	t.Helper()
	var out, stderr bytes.Buffer
	app := newApp(func() (setup.Backend, error) { return b, nil }, nil, &out, &stderr)
	args := append([]string{"skl", lane, "next", "--repo", root, "--remote", "upstream"}, options...)
	err := app.RunContext(ctx, args)
	var got workflow.ImplementationOutcome
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
		t.Run(lane, func(t *testing.T) {
			root, b := waitFixture(t, lane)
			state := b.work[0].State
			b.work[0].State = workflow.NeedsHuman
			synctest.Test(t, func(t *testing.T) {
				start := time.Now()
				b.observe = func(context.Context) error {
					if b.reads == 2 {
						b.work[0].State = state
					}
					return nil
				}
				got, err := waitingCLI(t, t.Context(), root, lane, b, "--wait")
				if err != nil || got.Status != "work_available" || got.Packet == nil || !got.Item.Claimed || b.claims != 1 || b.reads != 3 || time.Since(start) != 30*time.Second || b.repository.Owner != "acme" {
					t.Fatalf("got %#v, err %v, reads %d claims %d elapsed %s repo %#v", got, err, b.reads, b.claims, time.Since(start), b.repository)
				}
			})
		})
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
				app := newApp(func() (setup.Backend, error) { calls++; return &waitingMemory{}, nil }, nil, &out, &out)
				args := append([]string{"skl", lane, "next", "--repo", root}, strings.Fields(tc.option)...)
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
		for _, option := range []string{"", "--poll=5s", "--wait"} {
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
					if option == "--wait" {
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
