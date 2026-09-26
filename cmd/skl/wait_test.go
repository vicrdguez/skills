package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/setup"
	"github.com/vicrdguez/skills/workflow"
)

// Waiting is shared transport behavior, not a second selection authority. Test
// its timing/continuation seam directly; delivery eligibility uses real ledgers.
func TestNextWaitNewlyAvailable(t *testing.T) {
	calls := 0
	out, err := nextWork(context.Background(), time.Second, time.Millisecond, func() (workflow.ImplementationOutcome, error) {
		calls++
		status := "no_work"
		if calls == 3 {
			status = "work_available"
		}
		return workflow.ImplementationOutcome{Status: status}, nil
	})
	if err != nil || out.Status != "work_available" || calls != 3 {
		t.Fatalf("waiting outcome %#v, %v; observations=%d", out, err, calls)
	}
}

func TestNextWaitDurationsAndIdleTimeout(t *testing.T) {
	for _, wait := range []time.Duration{0, 20 * time.Millisecond} {
		calls := 0
		out, err := nextWork(context.Background(), wait, time.Millisecond, func() (workflow.ImplementationOutcome, error) {
			calls++
			return workflow.ImplementationOutcome{Status: "no_work"}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if wait == 0 && (out.Status != "no_work" || calls != 1) {
			t.Fatalf("one-shot: %#v calls=%d", out, calls)
		}
		if wait > 0 && (out.Status != "idle_timeout" || calls < 1 || !strings.Contains(out.Reason, "not global completion")) {
			t.Fatalf("idle: %#v calls=%d", out, calls)
		}
	}
}

func TestNextWaitPreservesInFlightAcquisition(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out, err := nextWork(ctx, time.Second, time.Millisecond, func() (workflow.ImplementationOutcome, error) {
		cancel()
		return workflow.ImplementationOutcome{Status: "work_available"}, nil
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
		_, err := nextWork(ctx, time.Hour, time.Minute, func() (workflow.ImplementationOutcome, error) {
			calls++
			cancel()
			return workflow.ImplementationOutcome{Status: "no_work"}, nil
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
		out, err := nextWork(context.Background(), time.Hour, time.Millisecond, func() (workflow.ImplementationOutcome, error) {
			calls++
			if status == "error" {
				return workflow.ImplementationOutcome{}, failure
			}
			return workflow.ImplementationOutcome{Status: status}, nil
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
