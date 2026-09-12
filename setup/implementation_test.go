package setup

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/workflow"
)

func TestGitHubImplementationRejectsDuplicateSourceOwnership(t *testing.T) {
	for _, attached := range []bool{false, true} {
		t.Run(fmt.Sprint(attached), func(t *testing.T) {
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					writes++
					http.Error(w, "unexpected mutation", 500)
					return
				}
				switch r.URL.Path {
				case "/repos/acme/widgets/issues":
					fmt.Fprint(w, `[{"number":7,"title":"widget","state":"open","labels":[{"name":"ready"}]},{"number":8,"title":"widget","state":"open","labels":[{"name":"ready"}]}]`)
				case "/repos/acme/widgets/pulls":
					if attached {
						fmt.Fprint(w, `[{"number":11,"state":"open","body":"original","head":{"ref":"widget","sha":"fixed","repo":{"full_name":"acme/widgets"}},"base":{"ref":"main"}}]`)
					} else {
						fmt.Fprint(w, `[]`)
					}
				default:
					fmt.Fprint(w, `[]`)
				}
			}))
			defer server.Close()
			b := NewGitHubBackend(server.URL, "token", server.Client())
			ctx := context.Background()
			repo := github.RepositoryID{Owner: "acme", Name: "widgets"}
			items, err := b.ImplementationItems(ctx, repo)
			if err != nil || len(items) != 2 {
				t.Fatalf("items = %#v, %v", items, err)
			}
			for _, item := range items {
				if !strings.Contains(item.Problem, "multiple source") {
					t.Errorf("ambiguous ownership accepted: %#v", item)
				}
				if err := b.ClaimImplementation(ctx, repo, item); err == nil {
					t.Error("ambiguous Claim accepted")
				}
				if _, err := b.PublishImplementation(ctx, repo, item, workflow.Submission{Head: "fixed", Base: "main", Body: "replacement"}); err == nil {
					t.Error("ambiguous publication accepted")
				}
			}
			if writes != 0 {
				t.Fatalf("ambiguous ownership caused %d mutations", writes)
			}
		})
	}
}

func TestGitHubImplementationRejectsReassignedSourceBeforePublication(t *testing.T) {
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writes++
			http.Error(w, "unexpected mutation", 500)
			return
		}
		switch r.URL.Path {
		case "/repos/acme/widgets/issues":
			fmt.Fprint(w, `[{"number":7,"title":"renamed","state":"open"},{"number":8,"title":"widget","state":"open","labels":[{"name":"ready"}]}]`)
		case "/repos/acme/widgets/pulls":
			fmt.Fprint(w, `[{"number":11,"state":"open","body":"original","head":{"ref":"widget","sha":"fixed","repo":{"full_name":"acme/widgets"}},"base":{"ref":"main"}}]`)
		default:
			fmt.Fprint(w, `{}`)
		}
	}))
	defer server.Close()
	b := NewGitHubBackend(server.URL, "token", server.Client())
	_, err := b.PublishImplementation(context.Background(), github.RepositoryID{Owner: "acme", Name: "widgets"}, workflow.ImplementationItem{ID: "7", Branch: "widget"}, workflow.Submission{ID: "11", Head: "fixed", Base: "main", Body: "replacement"})
	if err == nil || writes != 0 {
		t.Fatalf("reassigned source allowed publication: %v, writes=%d", err, writes)
	}
}

func TestGitHubImplementationNormalizesPaginatedWork(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/widgets/issues":
			if r.URL.Query().Get("page") == "1" {
				fmt.Fprint(w, "[")
				for i := 0; i < 100; i++ {
					if i > 0 {
						fmt.Fprint(w, ",")
					}
					fmt.Fprintf(w, `{"number":%d,"title":"unrelated","state":"open"}`, 100+i)
				}
				fmt.Fprint(w, "]")
			} else {
				fmt.Fprint(w, `[{"number":7,"title":"widget","state":"open","body":"Blocked by: #2, #3","created_at":"2020-01-01T00:00:00Z","labels":[{"name":"ready"}]}]`)
			}
		case "/repos/acme/widgets/pulls":
			fmt.Fprint(w, `[]`)
		case "/repos/acme/widgets/issues/7/dependencies/blocked_by":
			fmt.Fprint(w, `[{"number":2}]`)
		case "/repos/acme/widgets/issues/7/comments":
			fmt.Fprint(w, `[]`)
		default:
			t.Errorf("unexpected %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	backend := NewGitHubBackend(server.URL, "token", server.Client())
	items, err := backend.ImplementationItems(context.Background(), github.RepositoryID{Owner: "acme", Name: "widgets"})
	if err != nil || len(items) != 1 || items[0].ID != "7" || items[0].Order != 7 || items[0].ClosingReference != "Closes #7" || !slices.Equal(items[0].Blockers, []workflow.WorkItemID{"2", "3"}) || items[0].State != workflow.Ready || items[0].Branch != "widget" || items[0].CreatedAt != "2020-01-01T00:00:00Z" {
		t.Fatalf("items = %#v, %v", items, err)
	}
}

func TestGitHubLifecycleRejectsInvalidIdentitiesBeforeTransport(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Error(w, "unexpected transport", http.StatusBadRequest)
	}))
	defer server.Close()
	b := NewGitHubBackend(server.URL, "token", server.Client())
	ctx := context.Background()
	repo := github.RepositoryID{Owner: "acme", Name: "widgets"}
	guard := func() error { return nil }
	operations := map[string]func(workflow.ImplementationItem) error{
		"claim": func(item workflow.ImplementationItem) error {
			return b.ClaimImplementation(ctx, repo, item)
		},
		"publish": func(item workflow.ImplementationItem) error {
			_, err := b.PublishImplementation(ctx, repo, item, *item.Submission)
			return err
		},
		"await": func(item workflow.ImplementationItem) error {
			return b.AwaitImplementationReview(ctx, repo, item, guard)
		},
		"pause": func(item workflow.ImplementationItem) error {
			return b.PauseImplementation(ctx, repo, item, "opaque decision", guard)
		},
		"retain": func(item workflow.ImplementationItem) error {
			return b.RetainImplementationClaim(ctx, repo, item)
		},
		"complete-review": func(item workflow.ImplementationItem) error {
			return b.CompleteReview(ctx, repo, item, workflow.Rework, guard)
		},
		"transition": func(item workflow.ImplementationItem) error {
			return b.RecordImplementationTransition(ctx, repo, item, workflow.ImplementationTransition{})
		},
		"close-coordination": func(item workflow.ImplementationItem) error {
			return b.CloseCoordination(ctx, repo, item.ID)
		},
		"review-submission": func(item workflow.ImplementationItem) error {
			_, err := b.ReviewSubmission(ctx, repo, item.Submission.ID)
			return err
		},
		"publish-review": func(item workflow.ImplementationItem) error {
			return b.PublishReview(ctx, repo, item, nil, guard)
		},
	}
	for _, id := range []string{"", "0", "-1", "07", "7/labels", "issue:7", "999999999999999999999999"} {
		for _, identity := range []string{"item", "submission"} {
			for name, operation := range operations {
				if identity == "item" && (name == "review-submission" || name == "publish-review") || identity == "submission" && (name == "transition" || name == "close-coordination" || name == "publish" && id == "") {
					continue
				}
				t.Run(name+"/"+identity+"/"+id, func(t *testing.T) {
					item := workflow.ImplementationItem{ID: "7", State: workflow.Rework, Submission: &workflow.Submission{ID: "11"}}
					if identity == "item" {
						item.ID = workflow.WorkItemID(id)
					} else {
						item.Submission.ID = workflow.SubmissionID(id)
					}
					if err := operation(item); err == nil || !strings.Contains(err.Error(), "invalid GitHub issue identity") {
						t.Fatalf("invalid identity accepted: %v", err)
					}
				})
			}
		}
	}
	if requests != 0 {
		t.Fatalf("invalid identities reached transport: %d requests", requests)
	}
}

func TestGitHubImplementationReconcilesMutationTimeouts(t *testing.T) {
	labels := map[int][]string{7: {"ready", "external"}}
	comments := map[int][]map[string]any{7: {{"author_association": "NONE", "body": "<!-- skl.implement/v1\n{\"target_snapshot\":\"snapshot\"}\n-->"}}}
	var pulls []map[string]any
	creations := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/repos/acme/widgets")
		issue := func(number int) map[string]any {
			ls := []map[string]string{}
			for _, name := range labels[number] {
				ls = append(ls, map[string]string{"name": name})
			}
			return map[string]any{"number": number, "title": "widget", "state": "open", "labels": ls}
		}
		var result any = map[string]any{}
		switch {
		case path == "/issues":
			result = []any{issue(7)}
		case path == "/pulls" && r.Method == http.MethodGet:
			for _, pull := range pulls {
				pull["labels"] = issue(11)["labels"]
			}
			result = pulls
		case path == "/pulls" && r.Method == http.MethodPost:
			var payload map[string]any
			json.NewDecoder(r.Body).Decode(&payload)
			creations++
			pulls = append(pulls, map[string]any{"number": 11, "node_id": "PR_11", "title": "widget", "state": "open", "body": payload["body"], "draft": payload["draft"], "head": map[string]any{"ref": "widget", "sha": "fixed", "repo": map[string]string{"full_name": "acme/widgets"}}, "base": map[string]string{"ref": "main"}})
			http.Error(w, "response lost after creation", 500)
			return
		case path == "/pulls/11" && r.Method == http.MethodGet:
			pulls[0]["labels"] = issue(11)["labels"]
			result = pulls[0]
		case path == "/graphql":
			var payload map[string]any
			json.NewDecoder(r.Body).Decode(&payload)
			pulls[0]["draft"] = strings.Contains(payload["query"].(string), "convertPullRequestToDraft")
			http.Error(w, "response lost after draft conversion", 500)
			return
		case path == "/pulls/11/reviews" || path == "/issues/11/timeline":
			result = []any{}
		case path == "/pulls/11" && r.Method == http.MethodPatch:
			var payload map[string]any
			json.NewDecoder(r.Body).Decode(&payload)
			for key, value := range payload {
				if key == "base" {
					value = map[string]any{"ref": value}
				}
				pulls[0][key] = value
			}
			http.Error(w, "response lost after update", 500)
			return
		case path == "/issues/7/dependencies/blocked_by":
			result = []any{}
		case strings.HasSuffix(path, "/comments"):
			var number int
			fmt.Sscanf(path, "/issues/%d/comments", &number)
			if r.Method == http.MethodPost {
				var payload map[string]any
				json.NewDecoder(r.Body).Decode(&payload)
				payload["author_association"] = "OWNER"
				comments[number] = append(comments[number], payload)
				http.Error(w, "response lost after comment", 500)
				return
			}
			result = comments[number]
		case strings.Contains(path, "/labels"):
			var number int
			fmt.Sscanf(path, "/issues/%d/labels", &number)
			if r.Method == http.MethodPost {
				var payload struct {
					Labels []string `json:"labels"`
				}
				json.NewDecoder(r.Body).Decode(&payload)
				labels[number] = append(labels[number], payload.Labels...)
			} else if r.Method == http.MethodDelete {
				name := path[strings.LastIndex(path, "/")+1:]
				var kept []string
				for _, label := range labels[number] {
					if label != name {
						kept = append(kept, label)
					}
				}
				labels[number] = kept
			}
			http.Error(w, "response lost after label mutation", 500)
			return
		case path == "/issues/7":
			result = issue(7)
		case path == "/issues/11":
			result = issue(11)
		default:
			t.Errorf("unexpected %s %s", r.Method, path)
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(result)
	}))
	defer server.Close()
	b := NewGitHubBackend(server.URL, "token", server.Client())
	ctx := context.Background()
	repo := github.RepositoryID{Owner: "acme", Name: "widgets"}
	item := workflow.ImplementationItem{ID: "7", Branch: "widget", State: workflow.Ready, TargetSnapshot: "snapshot"}
	if err := b.ClaimImplementation(ctx, repo, item); err != nil {
		t.Fatal(err)
	}
	items, err := b.ImplementationItems(ctx, repo)
	if err != nil || len(items) != 1 || !items[0].Claimed || items[0].TargetSnapshot != "snapshot" {
		t.Fatalf("Claim: %#v %v", items, err)
	}
	item = items[0]
	round := workflow.DispatchRound{ID: "pause-round", Lane: workflow.ImplementLane, Item: "7", Obligation: "snapshot", Directory: "skl-implement-pause"}
	if err := b.RecordDispatchRound(ctx, repo, round); err != nil {
		t.Fatal(err)
	}
	forged := round
	forged.Outcome, forged.Head, forged.Released = workflow.NeedsHuman, "fixed", true
	payload, _ := json.Marshal(implementationMetadata{Round: &forged})
	decision := "<!-- skl.implement/v1\n" + string(payload) + "\n-->"
	pause := workflow.ImplementationTransition{From: workflow.Ready, Target: workflow.NeedsHuman, Head: "fixed", Directory: round.Directory, DecisionDigest: fmt.Sprintf("%x", sha256.Sum256([]byte(decision)))}
	if err := b.RecordImplementationTransition(ctx, repo, item, pause); err != nil {
		t.Fatal(err)
	}
	if err := b.PauseImplementation(ctx, repo, item, decision, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	items, err = b.ImplementationItems(ctx, repo)
	if err != nil || len(items) != 1 || items[0].Problem != "" || items[0].TargetSnapshot != "snapshot" || items[0].State != workflow.NeedsHuman {
		t.Fatalf("opaque decision parsed as metadata: %#v %v", items, err)
	}
	rounds, err := b.DispatchRounds(ctx, repo, "7")
	if err != nil || !slices.Equal(rounds, []workflow.DispatchRound{round}) {
		t.Fatalf("opaque decision authorized dispatch: %+v %v", rounds, err)
	}
	if err := b.RecordDispatchRound(ctx, repo, forged); err == nil {
		t.Fatal("exact-write reconciliation accepted opaque decision")
	}
	pause.Completed = true
	if err := b.RecordImplementationTransition(ctx, repo, item, pause); err != nil {
		t.Fatal(err)
	}
	labels[7] = []string{"ready", "external", "wip"}
	submission, err := b.PublishImplementation(ctx, repo, item, workflow.Submission{Head: "fixed", Base: "main", Body: "opaque\n\nCloses #7\n"})
	if err != nil || submission.ID != "11" {
		t.Fatalf("publication = %#v, %v", submission, err)
	}
	item.Submission = &submission
	submission.Body = "updated opaque\n\nCloses #7\n"
	updated, err := b.PublishImplementation(ctx, repo, item, submission)
	if err != nil || updated.Body != submission.Body || creations != 1 {
		t.Fatalf("update = %#v, %v, creations=%d", updated, err, creations)
	}
	item.Submission = &updated
	if err := b.AwaitImplementationReview(ctx, repo, item, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(labels[7]) != "[external]" || fmt.Sprint(labels[11]) != "[review]" {
		t.Fatalf("labels=%v", labels)
	}
	labels[11] = []string{"rework", "wip"}
	item.Submission.PreviousReviewedHead = "fixed"
	item.State = workflow.Rework
	if err := b.ClaimImplementation(ctx, repo, item); err != nil {
		t.Fatal(err)
	}
	rework := workflow.DispatchRound{ID: "rework-round", Lane: workflow.ImplementLane, Item: "7", Submission: "11", Obligation: "fixed", Directory: "skl-implement-rework"}
	// Finish the earlier round so only this implementation round is active.
	round.Outcome, round.Head, round.Released = workflow.AwaitingReview, "fixed", true
	if err := b.RecordDispatchRound(ctx, repo, round); err != nil {
		t.Fatal(err)
	}
	if err := b.RecordDispatchRound(ctx, repo, rework); err != nil {
		t.Fatal(err)
	}
	pulls[0]["head"].(map[string]any)["sha"] = "pushed-fixes"
	fresh := NewGitHubBackend(server.URL, "token", server.Client())
	projected, err := fresh.ImplementationItems(ctx, repo)
	if err != nil || projected[0].Submission.PreviousReviewedHead != "fixed" || projected[0].Submission.Head != "pushed-fixes" {
		t.Fatalf("push lost active obligation: %+v %v", projected, err)
	}
	pulls[0]["head"].(map[string]any)["sha"] = "fixed"
	item.State = workflow.Rework
	item.Claimed = true
	transition := workflow.ImplementationTransition{From: workflow.Rework, Target: workflow.NeedsHuman, Head: "fixed"}
	if err := b.RecordImplementationTransition(ctx, repo, item, transition); err != nil {
		t.Fatal(err)
	}
	labels[7] = []string{"done"}
	items, err = b.ImplementationItems(ctx, repo)
	if err != nil || len(items) != 1 || items[0].Problem == "" {
		t.Fatalf("pending drift accepted: %#v %v", items, err)
	}
	labels[7] = []string{"external"}
	updated.Draft = true
	draft, err := b.PublishImplementation(ctx, repo, item, updated)
	if err != nil || !draft.Draft {
		t.Fatalf("draft = %#v %v", draft, err)
	}
	item.Submission = &draft
	interrupted := false
	guard := func() error {
		if !interrupted && slices.Contains(labels[11], "needs-human") && slices.Contains(labels[11], "rework") {
			interrupted = true
			return errors.New("interrupted before final projection")
		}
		return nil
	}
	if err := b.PauseImplementation(ctx, repo, item, "opaque decision", guard); err == nil {
		t.Fatal("fixture did not interrupt pause")
	}
	items, err = b.ImplementationItems(ctx, repo)
	if err != nil || len(items) != 1 || items[0].Problem != "" || items[0].State != workflow.Rework || !items[0].Claimed {
		t.Fatalf("interrupted pause = %#v %v", items, err)
	}
	if err := b.PauseImplementation(ctx, repo, item, "opaque decision", guard); err != nil {
		t.Fatal(err)
	}
	items, err = b.ImplementationItems(ctx, repo)
	if err != nil || items[0].State != workflow.NeedsHuman || items[0].Claimed || items[0].ResumeState != workflow.Rework || !items[0].Submission.Draft {
		t.Fatalf("completed pause = %#v %v", items, err)
	}
}

func TestGitHubDispatchRoundsSurvivePaginationAndLostResponses(t *testing.T) {
	comments := make(map[int][]map[string]any)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var number int
		if _, err := fmt.Sscanf(strings.TrimPrefix(r.URL.Path, "/repos/acme/widgets"), "/issues/%d/comments", &number); err != nil {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPost {
			var payload map[string]any
			json.NewDecoder(r.Body).Decode(&payload)
			payload["author_association"] = "OWNER"
			comments[number] = append(comments[number], payload)
			http.Error(w, "response lost after durable comment", http.StatusInternalServerError)
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		start := (page - 1) * 100
		if start >= len(comments[number]) {
			json.NewEncoder(w).Encode([]any{})
			return
		}
		end := min(start+100, len(comments[number]))
		json.NewEncoder(w).Encode(comments[number][start:end])
	}))
	defer server.Close()
	repo := github.RepositoryID{Owner: "acme", Name: "widgets"}
	ctx := context.Background()
	for _, tc := range []struct {
		item       workflow.WorkItemID
		lane       workflow.DispatchLane
		submission workflow.SubmissionID
		outcome    workflow.State
	}{{"7", workflow.ImplementLane, "", workflow.NeedsHuman}, {"8", workflow.WatchdogLane, "11", workflow.ReadyForMerge}} {
		start := workflow.DispatchRound{ID: "round-" + string(tc.item), Lane: tc.lane, Item: tc.item, Submission: tc.submission, Obligation: "fixed", Directory: "skl-result-fixed"}
		backend := NewGitHubBackend(server.URL, "token", server.Client())
		if err := backend.RecordDispatchRound(ctx, repo, start); err != nil {
			t.Fatal(err)
		}
		for range 105 {
			commentsNumber, _ := strconv.Atoi(string(tc.item))
			comments[commentsNumber] = append(comments[commentsNumber], map[string]any{"author_association": "OWNER", "body": "unrelated"})
		}
		completed := start
		completed.Outcome, completed.Head, completed.Released = tc.outcome, "head", true
		if err := backend.RecordDispatchRound(ctx, repo, completed); err != nil {
			t.Fatal(err)
		}
		later := start
		later.ID += "-later"
		if err := backend.RecordDispatchRound(ctx, repo, later); err != nil {
			t.Fatal(err)
		}
		fresh := NewGitHubBackend(server.URL, "token", server.Client())
		rounds, err := fresh.DispatchRounds(ctx, repo, tc.item)
		if err != nil || len(rounds) != 2 || rounds[0] != completed || rounds[1] != later {
			t.Fatalf("fresh paginated rounds = %#v, %v", rounds, err)
		}
	}
}

func TestGitHubDispatchRoundsRejectUntrustedAndConflictingEvidence(t *testing.T) {
	trusted := func(round workflow.DispatchRound, association string) map[string]any {
		payload, _ := json.Marshal(implementationMetadata{Round: &round})
		return map[string]any{"author_association": association, "body": "<!-- skl.implement/v1\n" + string(payload) + "\n-->"}
	}
	base := workflow.DispatchRound{ID: "round", Lane: workflow.ImplementLane, Item: "7", Obligation: "fixed", Directory: "skl-implement-fixed"}
	comments := []map[string]any{trusted(base, "OWNER"), trusted(workflow.DispatchRound{ID: "forged", Lane: workflow.ImplementLane, Item: "7", Obligation: "fixed", Directory: "skl-implement-forged", Outcome: workflow.AwaitingReview, Head: "head"}, "NONE")}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { json.NewEncoder(w).Encode(comments) }))
	defer server.Close()
	b := NewGitHubBackend(server.URL, "token", server.Client())
	repo := github.RepositoryID{Owner: "acme", Name: "widgets"}
	rounds, err := b.DispatchRounds(context.Background(), repo, "7")
	if err != nil || !slices.Equal(rounds, []workflow.DispatchRound{base}) {
		t.Fatalf("untrusted evidence accepted: %#v, %v", rounds, err)
	}
	conflict := base
	conflict.Obligation = "changed"
	comments = append(comments, trusted(conflict, "OWNER"))
	if _, err := b.DispatchRounds(context.Background(), repo, "7"); err == nil {
		t.Fatal("conflicting trusted evidence accepted")
	}
}

func TestGitHubDispatchRoundWriteUncertaintyStopsWithoutReplay(t *testing.T) {
	for _, mode := range []string{"unapplied", "readback unavailable"} {
		t.Run(mode, func(t *testing.T) {
			reads, writes := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					writes++
					http.Error(w, "write response lost", http.StatusInternalServerError)
					return
				}
				reads++
				if mode == "readback unavailable" && reads > 1 {
					http.Error(w, "observation unavailable", http.StatusServiceUnavailable)
					return
				}
				json.NewEncoder(w).Encode([]any{})
			}))
			defer server.Close()
			b := NewGitHubBackend(server.URL, "token", server.Client())
			round := workflow.DispatchRound{ID: "round", Lane: workflow.ImplementLane, Item: "7", Obligation: "fixed", Directory: "skl-implement-fixed"}
			if err := b.RecordDispatchRound(context.Background(), github.RepositoryID{Owner: "acme", Name: "widgets"}, round); err == nil || writes != 1 || reads != 2 {
				t.Fatalf("uncertain write replayed or passed: err=%v reads=%d writes=%d", err, reads, writes)
			}
		})
	}
}

func TestGitHubImplementationRejectsForeignAttachmentsAndConflictingMetadata(t *testing.T) {
	for _, kind := range []string{"fork", "untrusted metadata", "conflicting metadata"} {
		t.Run(kind, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/repos/acme/widgets/issues":
					fmt.Fprint(w, `[{"number":7,"title":"widget","state":"open","labels":[{"name":"ready"},{"name":"wip"}]}]`)
				case "/repos/acme/widgets/pulls":
					if kind == "fork" {
						fmt.Fprint(w, `[{"number":11,"state":"closed","merged_at":"2020","head":{"ref":"widget","repo":{"full_name":"outsider/widgets"}}}]`)
					} else {
						fmt.Fprint(w, `[]`)
					}
				case "/repos/acme/widgets/issues/7/dependencies/blocked_by":
					fmt.Fprint(w, `[]`)
				case "/repos/acme/widgets/issues/7/comments":
					comments := []map[string]string{}
					if kind != "fork" {
						comments = append(comments, map[string]string{"author_association": "OWNER", "body": "<!-- skl.implement/v1\n{\"target_snapshot\":\"original\"}\n-->"})
						association := "NONE"
						if kind == "conflicting metadata" {
							association = "OWNER"
						}
						comments = append(comments, map[string]string{"author_association": association, "body": "<!-- skl.implement/v1\n{\"target_snapshot\":\"replacement\"}\n-->"})
					}
					json.NewEncoder(w).Encode(comments)
				default:
					t.Errorf("unexpected %s", r.URL)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			b := NewGitHubBackend(server.URL, "token", server.Client())
			items, err := b.ImplementationItems(context.Background(), github.RepositoryID{Owner: "acme", Name: "widgets"})
			if err != nil || len(items) != 1 {
				t.Fatalf("items = %#v %v", items, err)
			}
			item := items[0]
			if kind == "fork" && (item.Submission != nil || item.State == workflow.Merged) || kind == "untrusted metadata" && item.TargetSnapshot != "original" || kind == "conflicting metadata" && item.Problem == "" {
				t.Fatalf("unsafe adoption: %#v", item)
			}
		})
	}
}

func TestGitHubDispatchWriterRejectsMissingObligationBeforePublication(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++; fmt.Fprint(w, `[]`) }))
	defer server.Close()
	b := NewGitHubBackend(server.URL, "token", server.Client())
	round := workflow.DispatchRound{ID: "unrecoverable", Lane: workflow.ImplementLane, Item: "7", Submission: "11", Directory: "skl-implement-invalid"}
	if err := b.RecordDispatchRound(t.Context(), github.RepositoryID{Owner: "acme", Name: "widgets"}, round); err == nil || requests != 0 {
		t.Fatalf("invalid obligation appended: %v requests=%d", err, requests)
	}
}

func TestGitHubIdenticalDecisionAndReceiptRemainDistinctOccurrences(t *testing.T) {
	for _, repaired := range []bool{false, true} {
		for _, fault := range []string{"none", "lost response", "unreadable readback"} {
			t.Run(fmt.Sprintf("repaired=%t/%s", repaired, fault), func(t *testing.T) {
				comments := []map[string]any{}
				labels := []string{"ready", "wip"}
				receiptPhase, unreadable := false, false
				writes := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch {
					case strings.HasSuffix(r.URL.Path, "/comments"):
						if r.Method == http.MethodPost {
							writes++
							var p map[string]any
							json.NewDecoder(r.Body).Decode(&p)
							p["author_association"] = "OWNER"
							comments = append(comments, p)
							if receiptPhase && fault == "unreadable readback" {
								unreadable = true
							}
							if receiptPhase && fault == "lost response" {
								http.Error(w, "applied but response lost", 500)
								return
							}
						} else if unreadable {
							http.Error(w, "readback unavailable", 503)
							return
						}
						json.NewEncoder(w).Encode(comments)
					case r.Method == http.MethodGet:
						ls := []map[string]string{}
						for _, label := range labels {
							ls = append(ls, map[string]string{"name": label})
						}
						json.NewEncoder(w).Encode(map[string]any{"number": 7, "state": "open", "labels": ls})
					case r.Method == http.MethodPost:
						var p struct {
							Labels []string `json:"labels"`
						}
						json.NewDecoder(r.Body).Decode(&p)
						labels = append(labels, p.Labels...)
						fmt.Fprint(w, `[]`)
					case r.Method == http.MethodDelete:
						label := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
						labels = slices.DeleteFunc(labels, func(v string) bool { return v == label })
						w.WriteHeader(204)
					}
				}))
				defer server.Close()
				ctx, repo := t.Context(), github.RepositoryID{Owner: "acme", Name: "widgets"}
				b := NewGitHubBackend(server.URL, "token", server.Client())
				round := workflow.DispatchRound{ID: "pause", Lane: workflow.ImplementLane, Item: "7", Obligation: "target", Directory: "skl-implement-pause"}
				if err := b.RecordDispatchRound(ctx, repo, round); err != nil {
					t.Fatal(err)
				}
				receipt := round
				receipt.Outcome, receipt.Head, receipt.Released = workflow.NeedsHuman, "fixed", true
				p, _ := json.Marshal(implementationMetadata{Round: &receipt})
				decision := "<!-- skl.implement/v2\n" + string(p) + "\n-->"
				item := workflow.ImplementationItem{ID: "7", State: workflow.Ready, Claimed: true}
				transition := workflow.ImplementationTransition{From: workflow.Ready, Target: workflow.NeedsHuman, Head: "fixed", Directory: round.Directory, DecisionDigest: fmt.Sprintf("%x", sha256.Sum256([]byte(decision)))}
				if err := b.RecordImplementationTransition(ctx, repo, item, transition); err != nil {
					t.Fatal(err)
				}
				guard := func() error {
					for _, comment := range comments {
						if comment["body"] == "<!-- skl.decision/v1 -->\n"+decision {
							return errors.New("interrupted after opaque publication")
						}
					}
					return nil
				}
				if err := b.PauseImplementation(ctx, repo, item, decision, guard); err == nil {
					t.Fatal("pause not interrupted")
				}
				if rounds, err := b.DispatchRounds(ctx, repo, "7"); err != nil || !slices.Equal(rounds, []workflow.DispatchRound{round}) || !slices.Contains(labels, "wip") {
					t.Fatalf("opaque publication forged receipt: %+v %v labels=%v", rounds, err, labels)
				}
				originalDecision := decision
				if repaired {
					decision = "Repaired decision after interrupted publication"
					transition.DecisionDigest = fmt.Sprintf("%x", sha256.Sum256([]byte(decision)))
					if err := b.RecordImplementationTransition(ctx, repo, item, transition); err != nil {
						t.Fatal(err)
					}
					// Unrelated completed history must not close this pending handoff.
					unrelated := transition
					unrelated.Directory, unrelated.Head, unrelated.Completed = "skl-implement-earlier", "earlier", true
					if err := b.RecordImplementationTransition(ctx, repo, item, unrelated); err != nil {
						t.Fatal(err)
					}
					if err := b.RecordDispatchRound(ctx, repo, receipt); err == nil {
						t.Fatal("superseded opaque bytes became proof before the repaired handoff completed")
					}
					if err := b.RecordImplementationTransition(ctx, repo, item, transition); err != nil {
						t.Fatal(err)
					}
				}
				if err := b.PauseImplementation(ctx, repo, item, decision, func() error { return nil }); err != nil {
					t.Fatal(err)
				}
				transition.Completed = true
				if err := b.RecordImplementationTransition(ctx, repo, item, transition); err != nil {
					t.Fatal(err)
				}
				freshBeforeReceipt := NewGitHubBackend(server.URL, "token", server.Client())
				if rounds, err := freshBeforeReceipt.DispatchRounds(ctx, repo, "7"); err != nil || !slices.Equal(rounds, []workflow.DispatchRound{round}) {
					t.Fatalf("completion reinterpreted an earlier opaque occurrence: %+v %v", rounds, err)
				}
				receiptPhase = true
				err := b.RecordDispatchRound(ctx, repo, receipt)
				if (fault == "unreadable readback") != (err != nil) {
					t.Fatalf("receipt result: %v", err)
				}
				unreadable = false
				before := writes
				fresh := NewGitHubBackend(server.URL, "token", server.Client())
				if err := fresh.RecordDispatchRound(ctx, repo, receipt); err != nil {
					t.Fatal(err)
				}
				rounds, err := fresh.DispatchRounds(ctx, repo, "7")
				occurrences := 0
				for _, comment := range comments {
					if comment["body"] == originalDecision || comment["body"] == "<!-- skl.decision/v1 -->\n"+originalDecision {
						occurrences++
					}
				}
				if err != nil || !slices.Equal(rounds, []workflow.DispatchRound{receipt}) || writes != before || occurrences != 2 || slices.Contains(labels, "wip") {
					t.Fatalf("genuine receipt lost or duplicated: %+v %v writes=%d/%d identical=%d labels=%v", rounds, err, before, writes, occurrences, labels)
				}
			})
		}
	}
}

func TestGitHubDispatchRoundsRefuseMalformedAndWrongItemHistory(t *testing.T) {
	for _, lane := range []workflow.DispatchLane{workflow.ImplementLane, workflow.WatchdogLane} {
		for _, defect := range []string{"truncated envelope", "invalid JSON", "missing ID", "missing item", "unknown lane", "missing directory", "missing obligation", "wrong item"} {
			t.Run(string(lane)+"/"+defect, func(t *testing.T) {
				round := workflow.DispatchRound{ID: "round", Lane: lane, Item: "7", Submission: "11", Obligation: "head", Directory: "skl-result-round", Head: "head", Released: true, Outcome: workflow.NeedsHuman}
				valid, _ := json.Marshal(implementationMetadata{Round: &round})
				malformed := round
				switch defect {
				case "missing ID":
					malformed.ID = ""
				case "missing item":
					malformed.Item = ""
				case "unknown lane":
					malformed.Lane = "unknown"
				case "missing directory":
					malformed.Directory = ""
				case "missing obligation":
					malformed.Obligation = ""
				case "wrong item":
					malformed.Item = "8"
				}
				payload, _ := json.Marshal(implementationMetadata{Round: &malformed})
				if defect == "invalid JSON" {
					payload = []byte(`{"round":`)
				}
				comments := []map[string]string{{"body": "<!-- skl.implement/v1\n" + string(valid) + "\n-->", "author_association": "OWNER"}, {"body": "<!-- skl.implement/v1\n" + string(payload) + "\n-->", "author_association": "OWNER"}}
				if defect == "truncated envelope" {
					comments[1]["body"] = strings.TrimSuffix(comments[1]["body"], "\n-->")
				}
				reads, writes := 0, 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != http.MethodGet {
						writes++
					} else {
						reads++
					}
					json.NewEncoder(w).Encode(comments)
				}))
				defer server.Close()
				b := NewGitHubBackend(server.URL, "token", server.Client())
				_, err := b.DispatchRounds(t.Context(), github.RepositoryID{Owner: "acme", Name: "widgets"}, "7")
				var refusal *workflow.InvariantError
				if !errors.As(err, &refusal) || reads != 1 || writes != 0 {
					t.Fatalf("hostile persisted history accepted or repaired: %v reads=%d writes=%d", err, reads, writes)
				}
			})
		}
	}
}

func TestGitHubLegacyDispatchHistoryAndExplicitPauseRecovery(t *testing.T) {
	ctx, repo := t.Context(), github.RepositoryID{Owner: "acme", Name: "widgets"}
	old := workflow.DispatchRound{ID: "old", Lane: workflow.ImplementLane, Item: "7", Obligation: "head", Directory: "skl-implement-old", Head: "head", Outcome: workflow.NeedsHuman, Released: true}
	active := workflow.DispatchRound{ID: "active", Lane: workflow.ImplementLane, Item: "7", Obligation: "head", Directory: "skl-implement-active"}
	receipt := active
	receipt.Outcome, receipt.Head, receipt.Released = workflow.NeedsHuman, "head", true
	legacy := func(v any) map[string]any {
		p, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return map[string]any{"body": "<!-- skl.implement/v1\n" + string(p) + "\n-->", "author_association": "OWNER"}
	}
	oldDecision := "completed legacy decision"
	oldTransition := workflow.ImplementationTransition{From: workflow.Ready, Target: workflow.NeedsHuman, Directory: old.Directory, Head: "head", DecisionDigest: fmt.Sprintf("%x", sha256.Sum256([]byte(oldDecision)))}
	comments := []map[string]any{legacy(map[string]any{"transition": oldTransition}), {"body": oldDecision, "author_association": "OWNER"}}
	oldTransition.Completed = true
	comments = append(comments, legacy(map[string]any{"transition": oldTransition}), legacy(map[string]any{"round": old}), legacy(map[string]any{"target_snapshot": "head"}), legacy(map[string]any{"round": active}))
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			writes++
			var p map[string]any
			json.NewDecoder(r.Body).Decode(&p)
			p["author_association"] = "OWNER"
			comments = append(comments, p)
		}
		json.NewEncoder(w).Encode(comments)
	}))
	defer server.Close()
	b := NewGitHubBackend(server.URL, "token", server.Client())
	if err := b.RecordDispatchRound(ctx, repo, old); err != nil || writes != 0 {
		t.Fatalf("legacy receipt was duplicated: %v writes=%d", err, writes)
	}
	decision := "ordinary legacy decision"
	transition := workflow.ImplementationTransition{From: workflow.Ready, Target: workflow.NeedsHuman, Directory: active.Directory, Head: "head", DecisionDigest: fmt.Sprintf("%x", sha256.Sum256([]byte(decision)))}
	comments = append(comments, legacy(map[string]any{"transition": transition}))
	comments = append(comments, map[string]any{"body": decision, "author_association": "OWNER"})
	completed := transition
	completed.Completed = true
	before := len(comments)
	fresh := NewGitHubBackend(server.URL, "token", server.Client())
	if rounds, err := fresh.DispatchRounds(ctx, repo, "7"); err != nil || !slices.Equal(rounds, []workflow.DispatchRound{old, active}) {
		t.Fatalf("legacy pause became proof or erased safe history: %+v %v", rounds, err)
	}
	if err := fresh.RecordDispatchRound(ctx, repo, receipt); err == nil || len(comments) != before {
		t.Fatalf("pending legacy handoff accepted: %v", err)
	}
	item := workflow.ImplementationItem{ID: "7", State: workflow.Ready, Claimed: true}
	if err := fresh.RecordImplementationTransition(ctx, repo, item, transition); err != nil {
		t.Fatal(err)
	}
	if err := fresh.RecordImplementationTransition(ctx, repo, item, completed); err != nil {
		t.Fatal(err)
	}
	if err := fresh.RecordDispatchRound(ctx, repo, receipt); err != nil {
		t.Fatal(err)
	}
	before = len(comments)
	fresh = NewGitHubBackend(server.URL, "token", server.Client())
	if err := fresh.RecordDispatchRound(ctx, repo, receipt); err != nil || len(comments) != before {
		t.Fatalf("recovered receipt duplicated: %v", err)
	}
	if rounds, err := fresh.DispatchRounds(ctx, repo, "7"); err != nil || !slices.Equal(rounds, []workflow.DispatchRound{old, receipt}) {
		t.Fatalf("explicit recovery failed: %+v %v", rounds, err)
	}
}

func TestGitHubAmbiguousLegacyDecisionHistoryCannotAcquireMetadataAuthority(t *testing.T) {
	for _, version := range []string{"v1", "v2"} {
		t.Run(version, func(t *testing.T) {
			round := workflow.DispatchRound{ID: "legacy", Lane: workflow.ImplementLane, Item: "7", Obligation: "head", Directory: "skl-implement-legacy"}
			receipt := round
			receipt.Outcome, receipt.Head, receipt.Released = workflow.NeedsHuman, "head", true
			envelope := func(version string, v any) string {
				p, err := json.Marshal(v)
				if err != nil {
					t.Fatal(err)
				}
				return "<!-- skl.implement/" + version + "\n" + string(p) + "\n-->"
			}
			digest := func(v string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(v))) }
			completed := workflow.ImplementationTransition{From: workflow.Ready, Target: workflow.NeedsHuman, Directory: round.Directory, Head: "head", Completed: true}
			decision2 := envelope(version, map[string]any{"transition": completed, "round": receipt})
			transition2 := completed
			transition2.Completed, transition2.DecisionDigest = false, digest(decision2)
			decision1 := envelope(version, map[string]any{"transition": transition2})
			transition1 := transition2
			transition1.DecisionDigest = digest(decision1)
			comments := []map[string]string{}
			for _, body := range []string{
				envelope("v1", map[string]any{"round": round}),
				envelope("v1", map[string]any{"transition": transition1}),
				decision1,
				envelope("v1", map[string]any{"transition": transition1}),
				envelope("v1", map[string]any{"transition": transition2}),
				decision2,
				envelope("v2", map[string]any{"transition": completed, "round": receipt}),
			} {
				comments = append(comments, map[string]string{"body": body, "author_association": "OWNER"})
			}
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					writes++
				}
				json.NewEncoder(w).Encode(comments)
			}))
			defer server.Close()
			b := NewGitHubBackend(server.URL, "token", server.Client())
			repo := github.RepositoryID{Owner: "acme", Name: "widgets"}
			if rounds, err := b.DispatchRounds(t.Context(), repo, "7"); err != nil || !slices.Equal(rounds, []workflow.DispatchRound{round}) {
				t.Fatalf("ambiguous legacy history authorized completion: %+v %v", rounds, err)
			}
			if err := b.RecordDispatchRound(t.Context(), repo, receipt); err == nil || writes != 0 {
				t.Fatalf("ambiguous legacy history reconciled a false receipt: %v writes=%d", err, writes)
			}
		})
	}
}
