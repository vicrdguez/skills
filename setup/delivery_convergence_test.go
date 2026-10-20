package setup

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/ledger"
)

func TestDeliveryPublicationConvergesOnExactHead(t *testing.T) {
	t.Parallel()
	for _, scenario := range []struct {
		name       string
		firstStale int
		approved   bool
	}{
		{"attached lookup before approval", 1, true},
		{"approval preflight after matching lookup", 2, true},
		{"attached lookup before draft presentation", 1, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			forge := newDeliveryForge(t)
			forge.add(deliveryPull{Number: 5, Body: "earlier body", Draft: scenario.approved})
			forge.beforePullRead = func(pull *deliveryPull) {
				pull.Head = "aaa"
				if forge.reads >= scenario.firstStale && forge.reads < scenario.firstStale+2 {
					pull.Head = "earlier-implementation"
					if len(forge.patchPayloads) != 0 || len(forge.draftQueries) != 0 {
						t.Error("public presentation changed before the exact head was observed")
					}
				}
			}
			forge.onWrite = func() {
				if forge.pulls[5].Head != "aaa" {
					t.Error("a different head authorized a public update")
				}
			}
			server := forge.server()
			defer server.Close()

			number, err := deliveryBackend(server).PresentPull(context.Background(), ledger.PullPresentation{
				Number: 5, Body: deliveryBody, Branch: "widget", Head: "aaa", Approved: scenario.approved,
			})
			if err != nil || number != 5 {
				t.Fatalf("converged presentation = %d, %v", number, err)
			}
			if pull := forge.pull(5); pull.Head != "aaa" || pull.Body != deliveryBody || pull.Draft == scenario.approved {
				t.Fatalf("converged presentation = %#v", pull)
			}
			if len(forge.mutations()) != 1 || forge.count(http.MethodPost, "/repos/acme/widgets/pulls") != 0 {
				t.Fatalf("presentation did not update only the exact attachment: %v", forge.requestLog())
			}
		})
	}
}

func TestDeliveryPublicationDoesNotRetryIdentityFailureAsHeadLag(t *testing.T) {
	t.Parallel()
	for _, stage := range []struct {
		name       string
		firstRead  int
		staleFirst bool
	}{
		{"lookup", 1, false},
		{"preflight", 2, false},
		{"during convergence", 2, true},
	} {
		for _, scenario := range []struct {
			name   string
			change func(*deliveryPull)
			reason string
		}{
			{"number", func(p *deliveryPull) { p.Number = 6 }, "requested #5"},
			{"repository", func(p *deliveryPull) { p.Owner = "other/widgets" }, "source repository"},
			{"branch", func(p *deliveryPull) { p.Branch = "other" }, "tracks branch"},
			{"base", func(p *deliveryPull) { p.Base = "other" }, "targets base"},
			{"state", func(p *deliveryPull) { p.State = "closed" }, "not open"},
		} {
			t.Run(scenario.name+"/"+stage.name, func(t *testing.T) {
				forge := newDeliveryForge(t)
				forge.add(deliveryPull{Number: 5, Body: "earlier body", Draft: true})
				forge.beforePullRead = func(pull *deliveryPull) {
					if stage.staleFirst || forge.reads >= stage.firstRead {
						pull.Head = "earlier-implementation"
					}
					if forge.reads >= stage.firstRead {
						scenario.change(pull)
					}
				}
				server := forge.server()
				defer server.Close()

				_, err := deliveryBackend(server).PresentPull(context.Background(), ledger.PullPresentation{
					Number: 5, Body: deliveryBody, Branch: "widget", Head: "aaa", Approved: true,
				})
				if err == nil || !strings.Contains(err.Error(), scenario.reason) {
					t.Fatalf("identity refusal = %v, want %q", err, scenario.reason)
				}
				if reads := forge.count(http.MethodGet, "/repos/acme/widgets/pulls/5"); reads != stage.firstRead {
					t.Fatalf("identity failure retried as head lag: %d reads", reads)
				}
				if len(forge.patchPayloads) != 0 || len(forge.mutations()) != 0 {
					t.Fatal("identity failure changed body or readiness")
				}
			})
		}
	}
}

func TestDeliveryPublicationBoundsPersistentHeadMismatch(t *testing.T) {
	t.Parallel()
	for _, firstRead := range []int{1, 2} {
		t.Run(map[int]string{1: "lookup", 2: "preflight"}[firstRead], func(t *testing.T) {
			forge := newDeliveryForge(t)
			forge.add(deliveryPull{Number: 5, Body: "earlier body", Draft: true})
			forge.beforePullRead = func(pull *deliveryPull) {
				if forge.reads >= firstRead {
					pull.Head = "earlier-implementation"
				}
			}
			server := forge.server()
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			number, err := deliveryBackend(server).PresentPull(ctx, ledger.PullPresentation{
				Number: 5, Body: deliveryBody, Branch: "widget", Head: "aaa", Approved: true,
			})
			if number != 0 || err == nil || !strings.Contains(err.Error(), "not the expected aaa") || ctx.Err() != nil {
				t.Fatalf("bounded mismatch refusal = %d, %v (context: %v)", number, err, ctx.Err())
			}
			if reads := forge.count(http.MethodGet, "/repos/acme/widgets/pulls/5"); reads <= firstRead || reads > 10 {
				t.Fatalf("head observations are not bounded retries: %d", reads)
			}
			if pull := forge.pull(5); pull.Body != "earlier body" || !pull.Draft || len(forge.mutations()) != 0 || len(forge.patchPayloads) != 0 {
				t.Fatalf("persistent mismatch changed public presentation: %#v", pull)
			}
		})
	}
}

func TestDeliveryPublicationCancelsHeadConvergenceWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		reads := 0
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodGet {
				t.Errorf("cancelled convergence dispatched %s", r.Method)
			}
			reads++
			pull := deliveryPull{Number: 5, Body: "earlier body", Draft: true, State: "open", Branch: "widget", Head: "earlier-implementation", Owner: "acme/widgets", Base: "main"}
			body, _ := json.Marshal(pull.record())
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
		})}
		backend := NewGitHubBackend("https://api.github.com", "token", client)
		backend.BindRepository(github.RepositoryID{Owner: "acme", Name: "widgets"})
		done := make(chan struct{})
		var err error
		go func() {
			defer close(done)
			_, err = backend.PresentPull(ctx, ledger.PullPresentation{Number: 5, Body: deliveryBody, Branch: "widget", Head: "aaa", Approved: true})
		}()
		synctest.Wait() // The successful stale read has completed; convergence is waiting.
		start := time.Now()
		cancel()
		<-done
		if !errors.Is(err, context.Canceled) || reads != 1 || time.Since(start) > time.Millisecond {
			t.Fatalf("cancellation = %v after %v and %d reads", err, time.Since(start), reads)
		}
	})
}

func TestDeliveryPublicationBoundsTransportDuringConvergence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reads := 0
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodGet {
				t.Errorf("unconverged presentation dispatched %s", r.Method)
			}
			reads++
			if reads > 1 {
				<-r.Context().Done()
				return nil, r.Context().Err()
			}
			pull := deliveryPull{Number: 5, Draft: true, State: "open", Branch: "widget", Head: "earlier-implementation", Owner: "acme/widgets", Base: "main"}
			body, _ := json.Marshal(pull.record())
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
		})}
		backend := NewGitHubBackend("https://api.github.com", "token", client)
		backend.BindRepository(github.RepositoryID{Owner: "acme", Name: "widgets"})
		start := time.Now()
		number, err := backend.PresentPull(context.Background(), ledger.PullPresentation{
			Number: 5, Body: deliveryBody, Branch: "widget", Head: "aaa", Approved: true,
		})
		if number != 0 || !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 10*time.Second || reads < 2 || reads > 10 {
			t.Fatalf("bounded convergence transport = %d, %v after %v and %d reads", number, err, time.Since(start), reads)
		}
	})
}

func TestDeliveryPublicationStopsSupersededConvergence(t *testing.T) {
	t.Parallel()
	for _, alreadyPresented := range []bool{false, true} {
		t.Run(map[bool]string{false: "pending writes", true: "no writes needed"}[alreadyPresented], func(t *testing.T) {
			forge := newDeliveryForge(t)
			body := "earlier body"
			if alreadyPresented {
				body = deliveryBody
			}
			forge.add(deliveryPull{Number: 5, Body: body, Draft: !alreadyPresented})
			var newer atomic.Bool
			forge.beforePullRead = func(pull *deliveryPull) {
				if forge.reads == 1 {
					pull.Head = "earlier-implementation"
				} else {
					pull.Head = "aaa"
					newer.Store(true)
				}
			}
			server := forge.server()
			defer server.Close()
			number, err := deliveryBackend(server).PresentPull(context.Background(), ledger.PullPresentation{
				Number: 5, Body: deliveryBody, Branch: "widget", Head: "aaa", Approved: true,
				Current: func() error {
					if newer.Load() {
						return errors.New("a newer result supersedes this presentation")
					}
					return nil
				},
			})
			if number != 0 || err == nil || !strings.Contains(err.Error(), "supersedes") {
				t.Fatalf("superseded convergence = %d, %v", number, err)
			}
			if len(forge.patchPayloads) != 0 || len(forge.mutations()) != 0 {
				t.Fatal("superseded convergence changed body or readiness")
			}
		})
	}
}
