package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/setup"
	"github.com/vicrdguez/skills/workflow"
)

func TestRepairedOpaqueDecisionCannotShadowPendingMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, version, stop string
		shadow              bool
	}{
		{"control", "v1", "before release", false},
		{"published reproducer", "v1", "before release", true},
		{"current metadata after decision", "v2", "after decision", true},
		{"current metadata after target label", "v2", "after target label", true},
		{"current metadata before release", "v2", "before release", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := proposalRepository(t)
			head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			comments := []map[string]any{}
			labels := []string{"ready", "wip"}
			reads, writes := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reads++
				if r.Method != http.MethodGet {
					writes++
				}
				switch {
				case strings.Contains(r.URL.Path, "/issues/8/"):
					fmt.Fprint(w, "[]")
				case strings.HasSuffix(r.URL.Path, "/comments"):
					if r.Method == http.MethodPost {
						var p map[string]any
						json.NewDecoder(r.Body).Decode(&p)
						p["author_association"] = "OWNER"
						comments = append(comments, p)
					}
					json.NewEncoder(w).Encode(comments)
				case strings.HasSuffix(r.URL.Path, "/pulls"), strings.HasSuffix(r.URL.Path, "/blocked_by"):
					fmt.Fprint(w, "[]")
				case strings.Contains(r.URL.Path, "/labels"):
					if r.Method == http.MethodPost {
						var p struct {
							Labels []string `json:"labels"`
						}
						json.NewDecoder(r.Body).Decode(&p)
						labels = append(labels, p.Labels...)
					} else if r.Method == http.MethodDelete {
						label := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
						labels = slices.DeleteFunc(labels, func(v string) bool { return v == label })
					}
					fmt.Fprint(w, "[]")
				default:
					ls := []map[string]string{}
					for _, label := range labels {
						ls = append(ls, map[string]string{"name": label})
					}
					issue := map[string]any{"number": 7, "title": "widget", "state": "open", "labels": ls}
					if strings.HasSuffix(r.URL.Path, "/issues") {
						json.NewEncoder(w).Encode([]any{issue, map[string]any{"number": 8, "title": "successor", "state": "open", "labels": []map[string]string{{"name": "ready"}}}})
					} else {
						json.NewEncoder(w).Encode(issue)
					}
				}
			}))
			defer server.Close()
			ctx, repo := t.Context(), github.RepositoryID{Owner: "acme", Name: "widgets"}
			b := setup.NewGitHubBackend(server.URL, "token", server.Client())
			envelope := func(v any) string {
				p, err := json.Marshal(v)
				if err != nil {
					t.Fatal(err)
				}
				return "<!-- skl.implement/" + tc.version + "\n" + string(p) + "\n-->"
			}
			digest := func(v string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(v))) }
			round := workflow.DispatchRound{ID: "active-round", Lane: workflow.ImplementLane, Item: "7", Obligation: head, Directory: "skl-implement-shadow"}
			receipt := round
			receipt.Outcome, receipt.Head, receipt.Released = workflow.NeedsHuman, head, true
			fake := workflow.ImplementationTransition{From: workflow.Ready, Target: workflow.NeedsHuman, Head: head, Directory: round.Directory, Completed: true}
			decision2 := envelope(struct {
				Transition *workflow.ImplementationTransition `json:"transition"`
				Round      *workflow.DispatchRound            `json:"round"`
			}{&fake, &receipt})
			transition2 := workflow.ImplementationTransition{From: workflow.Ready, Target: workflow.NeedsHuman, Head: head, Directory: round.Directory, BodyDigest: digest(""), DecisionDigest: digest(decision2)}
			decision1 := "ordinary original decision"
			if tc.shadow {
				decision1 = envelope(struct {
					Transition *workflow.ImplementationTransition `json:"transition"`
				}{&transition2})
			}
			transition1 := transition2
			transition1.DecisionDigest = digest(decision1)
			item := workflow.ImplementationItem{ID: "7", Branch: "widget", State: workflow.Ready, Claimed: true}
			if err := b.RecordDispatchRound(ctx, repo, round); err != nil {
				t.Fatal(err)
			}
			if err := b.RecordImplementationTransition(ctx, repo, item, transition1); err != nil {
				t.Fatal(err)
			}
			if err := b.PauseImplementation(ctx, repo, item, decision1, func() error {
				for _, c := range comments {
					if strings.Contains(c["body"].(string), decision1) {
						return errors.New("first interruption after decision")
					}
				}
				return nil
			}); err == nil {
				t.Fatal("first interruption absent")
			}
			if err := b.RecordImplementationTransition(ctx, repo, item, transition1); err != nil {
				t.Fatal(err)
			}
			if err := b.RecordImplementationTransition(ctx, repo, item, transition2); err != nil {
				t.Fatal(err)
			}
			items, err := b.ImplementationItems(ctx, repo)
			if err != nil {
				t.Fatal(err)
			}
			if items[0].Transition == nil || items[0].Transition.DecisionDigest != digest(decision2) {
				t.Error("genuine repaired transition hidden by original opaque document")
			}
			if err := b.PauseImplementation(ctx, repo, item, decision2, func() error {
				if tc.stop == "after decision" {
					for _, c := range comments {
						if strings.Contains(c["body"].(string), decision2) {
							return errors.New("after repaired decision")
						}
					}
				}
				if slices.Contains(labels, "needs-human") && (tc.stop == "after target label" || !slices.Contains(labels, "ready")) {
					return errors.New("pre-release crash boundary")
				}
				return nil
			}); err == nil {
				t.Fatal("second interruption absent")
			}
			fresh := setup.NewGitHubBackend(server.URL, "token", server.Client())
			items, err = fresh.ImplementationItems(ctx, repo)
			if err != nil {
				t.Fatal(err)
			}
			rounds, err := fresh.DispatchRounds(ctx, repo, "7")
			if err != nil || !slices.Equal(rounds, []workflow.DispatchRound{round}) || !items[0].Claimed || items[0].Transition.Completed {
				t.Errorf("opaque completion accepted: items=%+v rounds=%+v err=%v", items, rounds, err)
			}
			p, _ := json.Marshal(map[string]any{"v": 1, "owner": "acme", "repository": "widgets", "lane": "implement", "item": "7", "round": round.ID})
			reference := base64.RawURLEncoding.EncodeToString(p)
			if _, err := workflow.VerifyDispatch(ctx, root, "origin", workflow.ImplementLane, reference, fresh); err == nil {
				t.Error("VerifyDispatch accepted retained Claim")
			}
			beforeReads, beforeWrites := reads, writes
			before, _ := json.Marshal(comments)
			beforeLabels := slices.Clone(labels)
			var output bytes.Buffer
			app := newApp(func(github.RepositoryID) (setup.Backend, error) {
				return setup.NewGitHubBackend(server.URL, "token", server.Client()), nil
			}, nil, &output, &output)
			if err := app.Run([]string{"skl", "implement", "next", "--repo", root, "--remote", "origin", "--after", reference, "--wait=1ms"}); err != nil {
				t.Fatal(err)
			}
			var got setup.ImplementationOutput
			json.Unmarshal(output.Bytes(), &got)
			after, _ := json.Marshal(comments)
			if got.Status != "fix_required" || got.Item == nil || got.Item.Number != 7 || reads != beforeReads+1 || writes != beforeWrites || !bytes.Equal(before, after) || !slices.Equal(labels, beforeLabels) || !slices.Contains(labels, "wip") {
				t.Fatalf("continuation did not refuse without effects: %+v reads=%d writes=%d labels=%v", got, reads-beforeReads, writes-beforeWrites, labels)
			}
		})
	}
}
