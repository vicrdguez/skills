package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/ledger"
	"github.com/vicrdguez/skills/setup"
)

// Waiting is shared transport behavior, not a second selection authority. Test
// its timing/continuation seam directly; delivery eligibility uses real ledgers.
func TestNextWaitNewlyAvailable(t *testing.T) {
	calls := 0
	out, err := nextWork(context.Background(), time.Second, time.Millisecond, func() (ledger.Selection, error) {
		calls++
		status := "no_work"
		if calls == 3 {
			status = "work_available"
		}
		return ledger.Selection{Status: status}, nil
	})
	if err != nil || out.Status != "work_available" || calls != 3 {
		t.Fatalf("waiting outcome %#v, %v; observations=%d", out, err, calls)
	}
}

func TestNextWaitDurationsAndIdleTimeout(t *testing.T) {
	for _, wait := range []time.Duration{0, 20 * time.Millisecond} {
		calls := 0
		out, err := nextWork(context.Background(), wait, time.Millisecond, func() (ledger.Selection, error) {
			calls++
			return ledger.Selection{Status: ledger.NoWork}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if wait == 0 && (out.Status != "no_work" || calls != 1) {
			t.Fatalf("one-shot: %#v calls=%d", out, calls)
		}
		if wait > 0 && (out.Status != "idle_timeout" || calls < 1) {
			t.Fatalf("idle: %#v calls=%d", out, calls)
		}
	}
}

func TestNextWaitPreservesInFlightAcquisition(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out, err := nextWork(ctx, time.Second, time.Millisecond, func() (ledger.Selection, error) {
		cancel()
		return ledger.Selection{Status: ledger.WorkAvailable}, nil
	})
	if err != nil || out.Status != "work_available" {
		t.Fatalf("cancellation discarded an established Claim: %#v %v", out, err)
	}
}

func TestNextWaitCancellation(t *testing.T) {
	for _, before := range []bool{true, false} {
		ctx, cancel := context.WithCancel(context.Background())
		if before {
			cancel()
		}
		calls := 0
		_, err := nextWork(ctx, time.Hour, time.Minute, func() (ledger.Selection, error) {
			calls++
			cancel()
			return ledger.Selection{Status: ledger.NoWork}, nil
		})
		cancel()
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation diagnostics: %v", err)
		}
		if before && calls != 0 || !before && calls != 1 {
			t.Fatalf("canceled waiting selected again: calls=%d", calls)
		}
	}
}

func TestNextWaitStopsOnFailure(t *testing.T) {
	failure := errors.New("required input unavailable")
	for _, status := range []string{"fix_required", "error"} {
		calls := 0
		out, err := nextWork(context.Background(), time.Hour, time.Millisecond, func() (ledger.Selection, error) {
			calls++
			if status == "error" {
				return ledger.Selection{}, failure
			}
			return ledger.Selection{Status: status}, nil
		})
		if calls != 1 || status == "error" && !errors.Is(err, failure) || status == "fix_required" && (err != nil || out.Status != status) {
			t.Fatalf("refusal retried or hidden: %#v %v calls=%d", out, err, calls)
		}
	}
}

func TestNextWaitInvalidOptionsBeforeBackend(t *testing.T) {
	for _, args := range [][]string{{"--wait", "0s"}, {"--wait", "-1s"}, {"--wait", "bad"}, {"--poll", "0s"}, {"--poll", "-1s"}} {
		var output bytes.Buffer
		app := newApp(func(github.RepositoryID) (setup.Backend, error) {
			t.Fatal("invalid wait reached backend")
			return nil, nil
		}, bytes.NewReader(nil), &output, &output)
		if err := app.Run(append([]string{"skl", "implement", "next"}, args...)); err == nil {
			t.Fatalf("invalid options accepted: %v", args)
		}
	}
}

func TestNextWaitHelp(t *testing.T) {
	for _, phase := range []string{"implement", "watchdog"} {
		var output bytes.Buffer
		app := newApp(func(github.RepositoryID) (setup.Backend, error) { t.Fatal("help reached backend"); return nil, nil }, bytes.NewReader(nil), &output, &output)
		if err := app.Run([]string{"skl", phase, "next", "--help"}); err != nil {
			t.Fatal(err)
		}
		for _, flag := range []string{"--wait", "--poll"} {
			if !strings.Contains(output.String(), flag) {
				t.Fatalf("%s help omits %s", phase, flag)
			}
		}
	}
}

func TestDeliveryCLIWaitCancellation(t *testing.T) {
	newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	cli := newLedgerApp(t, newForgeServer(t))
	accepted := cli.accept(t, root, writeProposal(t, "", singleSlice("waiting")))
	if accepted.Status != "accepted" {
		t.Fatal(accepted)
	}
	// Watchdog has no eligible work before implementation; cancellation must
	// stop this queue-local idle wait without consulting the forge or claiming.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	cli.out.Reset()
	err := cli.app.RunContext(ctx, []string{"skl", "watchdog", "next", "--repo", root, "--wait", "--poll", "10ms"})
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "queue waiting interrupted") {
		t.Fatalf("waiting lost cancellation: %v; output=%s", err, cli.out)
	}
}

// waitLedgerRun runs one waiting implement next against the real ledger and
// returns its JSON outcome with the virtual time it took.
func waitLedgerRun(t *testing.T, cli ledgerCLI, source string) (deliveryOutput, time.Duration) {
	t.Helper()
	start := time.Now()
	out, err := cli.deliveryJSON(t, "skl", "implement", "next", "--repo", source, "--wait=2m", "--poll=30s", "--format", "json")
	if err != nil {
		t.Fatalf("waiting next: %v", err)
	}
	return out, time.Since(start)
}

// Work eligible at the first observation is claimed without sleeping.
func TestDeliveryCLIWaitClaimsImmediatelyAvailableWork(t *testing.T) {
	fixture := newLedgerFixture(t)
	source, _ := deliverySourceRepo(t)
	deliveryAcceptFixture(t, newForgeServer(t), source)
	cli := deliveryNoForgeApp(t)
	synctest.Test(t, func(t *testing.T) {
		out, elapsed := waitLedgerRun(t, cli, source)
		if out.Status != ledger.WorkAvailable || out.Execution == nil || elapsed != 0 {
			t.Fatalf("got %s after %s, want work_available at once", out.Status, elapsed)
		}
	})
	if state := deliveryPersistedState(t, fixture.clone); state.Claim == nil {
		t.Fatal("immediately available work was not claimed")
	}
}

// Every poll applies the ledger's eligibility: a claimed Slice and a Slice
// whose Dependency is unmerged are skipped until the Dependency merges during
// the window, and the next poll then claims the dependent Slice.
func TestDeliveryCLIWaitClaimsLateWorkWithCanonicalEligibility(t *testing.T) {
	fixture := newLedgerFixture(t)
	source, _ := deliverySourceRepo(t)
	spec := dualSlice(deliveryTestProposal)
	spec.depends = map[string][]string{"feature": {"foundation"}}
	if accepted := newLedgerApp(t, newForgeServer(t)).accept(t, source, writeProposal(t, "", spec)); accepted.Status != "accepted" {
		t.Fatalf("accept: %s", mustJSON(t, accepted))
	}
	cli := deliveryNoForgeApp(t)
	held, err := cli.deliveryJSON(t, "skl", "implement", "next", "--repo", source, "--format", "json")
	if err != nil || held.Execution == nil || held.Execution.Item != deliveryTestItem {
		t.Fatalf("claim foundation: %#v %v", held, err)
	}
	synctest.Test(t, func(t *testing.T) {
		done := make(chan struct{})
		var out deliveryOutput
		var elapsed time.Duration
		go func() {
			defer close(done)
			out, elapsed = waitLedgerRun(t, cli, source)
		}()
		// Between the polls at 30s and 60s, the blocking Slice merges.
		time.Sleep(45 * time.Second)
		state := deliveryPersistedState(t, fixture.clone)
		state.Claim, state.State = nil, ledger.Merged
		deliveryCommitState(t, fixture.clone, state)
		<-done
		if out.Status != ledger.WorkAvailable || out.Execution == nil || out.Execution.Item != deliveryTestProposal+"/feature" || elapsed != time.Minute {
			t.Fatalf("got %s for %#v after %s, want the feature claimed on the 60s poll", out.Status, out.Execution, elapsed)
		}
	})
}
