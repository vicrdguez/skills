package setup

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

func TestReadinessOnlyPreservesProseAndConvergesOnExactAttachment(t *testing.T) {
	for _, draft := range []bool{true, false} {
		t.Run(map[bool]string{true: "draft", false: "already ready"}[draft], func(t *testing.T) {
			forge := newDeliveryForge(t)
			forge.add(deliveryPull{Number: 5, Body: "Human body", Title: "Human title", Base: "proposal/feature", Draft: draft})
			forge.beforePullRead = func(p *deliveryPull) {
				if forge.reads == 1 {
					p.Head = "old"
				} else {
					p.Head = "aaa"
				}
			}
			server := forge.server()
			defer server.Close()
			err := deliveryBackend(server).ReadyPull(t.Context(), ledger.PullReadiness{Number: 5, Branch: "widget", Head: "aaa", Base: "proposal/feature"})
			if err != nil {
				t.Fatal(err)
			}
			pull := forge.pull(5)
			if pull.Draft || pull.Body != "Human body" || pull.Title != "Human title" {
				t.Fatalf("readiness changed prose: %+v", pull)
			}
			for _, r := range forge.requestLog() {
				if r.method != http.MethodGet && r.path != "/graphql" {
					t.Fatalf("non-readiness effect: %+v", r)
				}
			}
			want := 0
			if draft {
				want = 1
			}
			if len(forge.mutations()) != want {
				t.Fatalf("readiness effects = %v", forge.mutations())
			}
		})
	}
}

func TestReadinessOnlyRefusesWrongClosedOrUnconvergedAttachments(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		change func(*deliveryPull)
	}{
		{"number", func(p *deliveryPull) { p.Number = 6 }},
		{"repository", func(p *deliveryPull) { p.Owner = "other/widgets" }},
		{"branch", func(p *deliveryPull) { p.Branch = "other" }},
		{"base", func(p *deliveryPull) { p.Base = "main" }},
		{"closed", func(p *deliveryPull) { p.State = "closed" }},
		{"persistent stale head", func(p *deliveryPull) { p.Head = "old" }},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			forge := newDeliveryForge(t)
			forge.add(deliveryPull{Number: 5, Draft: true, Base: "proposal/feature"})
			forge.beforePullRead = scenario.change
			server := forge.server()
			defer server.Close()
			err := deliveryBackend(server).ReadyPull(t.Context(), ledger.PullReadiness{Number: 5, Branch: "widget", Head: "aaa", Base: "proposal/feature"})
			if err == nil || len(forge.mutations()) != 0 || len(forge.patchPayloads) != 0 {
				t.Fatalf("unsafe recovery = %v, %v", err, forge.requestLog())
			}
		})
	}
}

func TestReadinessOnlyReportsUnconfirmedEffectsAndStopsSupersession(t *testing.T) {
	for _, scenario := range []string{"mutation failure", "readback failure", "head moves", "superseded"} {
		t.Run(scenario, func(t *testing.T) {
			forge := newDeliveryForge(t)
			forge.add(deliveryPull{Number: 5, Draft: true, Base: "proposal/feature"})
			var newer atomic.Bool
			switch scenario {
			case "mutation failure":
				forge.graphqlStatus = http.StatusServiceUnavailable
			case "readback failure":
				forge.failReadsAfter = 1
			case "head moves":
				forge.moveHeadOnReady = "newer"
			case "superseded":
				forge.beforePullRead = func(*deliveryPull) { newer.Store(true) }
			}
			server := forge.server()
			defer server.Close()
			err := deliveryBackend(server).ReadyPull(t.Context(), ledger.PullReadiness{
				Number: 5, Branch: "widget", Head: "aaa", Base: "proposal/feature",
				Current: func() error {
					if newer.Load() {
						return errors.New("newer local result")
					}
					return nil
				},
			})
			if scenario == "superseded" {
				if err == nil || !strings.Contains(err.Error(), "newer local result") || len(forge.mutations()) != 0 {
					t.Fatalf("supersession = %v", err)
				}
			} else if !errors.Is(err, ledger.ErrPresentationUncertain) {
				t.Fatalf("unconfirmed effect not reported uncertain: %v", err)
			}
			if len(forge.patchPayloads) != 0 {
				t.Fatal("readiness wrote public prose")
			}
		})
	}
}

func TestReadinessOnlyCancellationStopsWithoutEffects(t *testing.T) {
	forge := newDeliveryForge(t)
	forge.add(deliveryPull{Number: 5, Draft: true, Base: "proposal/feature", Head: "old"})
	ctx, cancel := context.WithCancel(t.Context())
	forge.beforePullRead = func(*deliveryPull) { cancel() }
	server := forge.server()
	defer server.Close()
	err := deliveryBackend(server).ReadyPull(ctx, ledger.PullReadiness{Number: 5, Branch: "widget", Head: "aaa", Base: "proposal/feature"})
	if !errors.Is(err, context.Canceled) || len(forge.mutations()) != 0 {
		t.Fatalf("cancelled recovery = %v", err)
	}
}
