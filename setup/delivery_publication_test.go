package setup

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/ledger"
)

// deliveryPull is the forge-side pull request record a controlled HTTP
// server keeps, so the adapter's effect is observed at the transport seam.
type deliveryPull struct {
	Number int
	NodeID string
	Title  string
	Body   string
	Draft  bool
	State  string
	Branch string
	Head   string
	Owner  string
	Base   string
}

func (p deliveryPull) record() map[string]any {
	return map[string]any{
		"number": p.Number, "node_id": p.NodeID, "title": p.Title, "body": p.Body,
		"draft": p.Draft, "state": p.State,
		"head": map[string]any{"ref": p.Branch, "sha": p.Head, "repo": map[string]string{"full_name": p.Owner}},
		"base": map[string]string{"ref": p.Base},
	}
}

// deliveryForge is a controlled GitHub surface. It stores pull requests and
// lets one test observe the exact requests, bodies, and readiness mutations a
// presentation produced, including uncertain creation responses.
type deliveryForge struct {
	t *testing.T

	mu              sync.Mutex
	pulls           map[int]*deliveryPull
	order           []int
	next            int
	branchTip       string
	pageSize        int
	requests        []deliveryRequest
	patchPayloads   []map[string]any
	draftQueries    []string
	createStatus    int
	createStores    bool
	listStatus      int
	listAfterCreate bool
	graphqlStatus   int
}

type deliveryRequest struct{ method, path string }

func newDeliveryForge(t *testing.T) *deliveryForge {
	return &deliveryForge{t: t, pulls: map[int]*deliveryPull{}, next: 10, branchTip: "aaa", createStores: true}
}

func (f *deliveryForge) server() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(f.serve))
}

// add installs one pull request, filling defaults so a test states only the
// facts its scenario is about.
func (f *deliveryForge) add(pull deliveryPull) *deliveryPull {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.put(pull)
}

func (f *deliveryForge) put(pull deliveryPull) *deliveryPull {
	if pull.Number == 0 {
		f.next++
		pull.Number = f.next
	}
	if pull.NodeID == "" {
		pull.NodeID = fmt.Sprintf("PR_node_%d", pull.Number)
	}
	if pull.State == "" {
		pull.State = "open"
	}
	if pull.Owner == "" {
		pull.Owner = "acme/widgets"
	}
	if pull.Base == "" {
		pull.Base = "main"
	}
	if pull.Branch == "" {
		pull.Branch = "widget"
	}
	if pull.Head == "" {
		pull.Head = f.branchTip
	}
	f.pulls[pull.Number] = &pull
	f.order = append(f.order, pull.Number)
	return &pull
}

func (f *deliveryForge) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, deliveryRequest{method: r.Method, path: r.URL.Path})
	f.mu.Unlock()
	switch {
	case r.URL.Path == "/graphql":
		f.serveGraphQL(w, r)
	case r.URL.Path == "/repos/acme/widgets/pulls" && r.Method == http.MethodGet:
		f.serveList(w, r)
	case r.URL.Path == "/repos/acme/widgets/pulls" && r.Method == http.MethodPost:
		f.serveCreate(w, r)
	case strings.HasPrefix(r.URL.Path, "/repos/acme/widgets/pulls/"):
		f.serveOne(w, r)
	default:
		f.t.Errorf("unexpected forge request %s %s", r.Method, r.URL)
		http.NotFound(w, r)
	}
}

func (f *deliveryForge) serveList(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listStatus != 0 && (!f.listAfterCreate || len(f.pulls) > 0) {
		http.Error(w, "listing unavailable", f.listStatus)
		return
	}
	head, base, state := r.URL.Query().Get("head"), r.URL.Query().Get("base"), r.URL.Query().Get("state")
	records := []map[string]any{}
	for _, number := range f.order {
		pull := f.pulls[number]
		if state != "" && pull.State != state {
			continue
		}
		if base != "" && pull.Base != base {
			continue
		}
		if head != "" && head != headUser(pull.Owner)+":"+pull.Branch {
			continue
		}
		records = append(records, pull.record())
	}
	if f.pageSize > 0 {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		start := (page - 1) * f.pageSize
		if start > len(records) {
			start = len(records)
		}
		end := start + f.pageSize
		if end > len(records) {
			end = len(records)
		}
		records = records[start:end]
	}
	json.NewEncoder(w).Encode(records)
}

func headUser(fullName string) string {
	user, _, _ := strings.Cut(fullName, "/")
	return user
}

func (f *deliveryForge) serveCreate(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Title string `json:"title"`
		Head  string `json:"head"`
		Base  string `json:"base"`
		Body  string `json:"body"`
		Draft bool   `json:"draft"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		f.t.Errorf("invalid create payload: %v", err)
	}
	f.mu.Lock()
	status := f.createStatus
	var created *deliveryPull
	if f.createStores {
		created = f.put(deliveryPull{Title: payload.Title, Body: payload.Body, Draft: payload.Draft, Branch: payload.Head, Base: payload.Base})
	}
	f.mu.Unlock()
	if status != 0 {
		http.Error(w, "creation response lost", status)
		return
	}
	if created == nil {
		http.Error(w, "no pull request created", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(created.record())
}

func (f *deliveryForge) serveOne(w http.ResponseWriter, r *http.Request) {
	number, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/repos/acme/widgets/pulls/"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	pull, found := f.pulls[number]
	if !found {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		json.NewEncoder(w).Encode(pull.record())
	case http.MethodPatch:
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		f.patchPayloads = append(f.patchPayloads, payload)
		if body, ok := payload["body"].(string); ok {
			pull.Body = body
		}
		if title, ok := payload["title"].(string); ok {
			pull.Title = title
		}
		json.NewEncoder(w).Encode(pull.record())
	default:
		http.Error(w, "unsupported pull request method", http.StatusMethodNotAllowed)
	}
}

func (f *deliveryForge) serveGraphQL(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Query     string            `json:"query"`
		Variables map[string]string `json:"variables"`
	}
	json.NewDecoder(r.Body).Decode(&payload)
	f.mu.Lock()
	f.draftQueries = append(f.draftQueries, payload.Query)
	status := f.graphqlStatus
	if status == 0 {
		for _, pull := range f.pulls {
			if pull.NodeID != payload.Variables["id"] {
				continue
			}
			pull.Draft = !strings.Contains(payload.Query, "markPullRequestReadyForReview")
		}
	}
	f.mu.Unlock()
	if status != 0 {
		http.Error(w, "readiness mutation rejected", status)
		return
	}
	fmt.Fprint(w, `{"data":{}}`)
}

func (f *deliveryForge) pull(number int) *deliveryPull {
	f.mu.Lock()
	defer f.mu.Unlock()
	if pull := f.pulls[number]; pull != nil {
		snapshot := *pull
		return &snapshot
	}
	return nil
}

func (f *deliveryForge) count(method, path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	total := 0
	for _, request := range f.requests {
		if request.method == method && request.path == path {
			total++
		}
	}
	return total
}

func (f *deliveryForge) requestLog() []deliveryRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]deliveryRequest(nil), f.requests...)
}

func (f *deliveryForge) mutations() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.draftQueries...)
}

func deliveryBackend(server *httptest.Server) *GitHubBackend {
	backend := NewGitHubBackend(server.URL, "token", server.Client())
	backend.BindRepository(github.RepositoryID{Owner: "acme", Name: "widgets"})
	return backend
}

// deliveryBody carries the delimiter shapes the private report format and
// review prose use, so preservation is observable at the transport seam.
const deliveryBody = "---\nschema: 1\noutcome: pass\n---\n\nHuman-facing delivery body.\n\n<!-- review discussion is not published inline -->\n\n| item | state |\n| --- | --- |\n| B10 | complete |\n"

func TestDeliveryPublicationCreatesDraftWithExactPublicBody(t *testing.T) {
	forge := newDeliveryForge(t)
	server := forge.server()
	defer server.Close()

	number, err := deliveryBackend(server).PresentPull(context.Background(), ledger.PullPresentation{
		Title: "widget delivery", Body: deliveryBody, Branch: "widget", Head: "aaa", Approved: false,
	})
	if err != nil || number != 11 {
		t.Fatalf("draft presentation = %d, %v", number, err)
	}
	pull := forge.pull(number)
	if pull == nil {
		t.Fatal("no pull request was created")
	}
	if pull.Draft != true || pull.Title != "widget delivery" || pull.Body != deliveryBody {
		t.Fatalf("created presentation = %#v", pull)
	}
	if pulls := forge.count(http.MethodPost, "/repos/acme/widgets/pulls"); pulls != 1 {
		t.Fatalf("created %d pull requests", pulls)
	}
	if forge.count(http.MethodPatch, fmt.Sprintf("/repos/acme/widgets/pulls/%d", number)) != 0 {
		t.Fatal("creation rewrote the new pull request instead of presenting it once")
	}
}

func TestDeliveryPublicationNeverCreatesUnverifiedApproval(t *testing.T) {
	forge := newDeliveryForge(t)
	forge.branchTip = "branch-moved-after-source-sync"
	server := forge.server()
	defer server.Close()
	_, err := deliveryBackend(server).PresentPull(context.Background(), ledger.PullPresentation{
		Title: "delivery", Body: "public approval", Branch: "widget", Head: "aaa", Approved: true,
	})
	if err == nil {
		t.Fatal("creation at a different head was accepted as approval")
	}
	if pull := forge.pull(11); pull == nil || !pull.Draft {
		t.Fatalf("unreviewed code presented ready: %#v", pull)
	}
	if len(forge.mutations()) != 0 {
		t.Fatal("unreviewed code received a readiness mutation")
	}
}

func TestDeliveryPublicationRefreshesDraftWithoutDuplicating(t *testing.T) {
	forge := newDeliveryForge(t)
	server := forge.server()
	defer server.Close()
	backend := deliveryBackend(server)
	presentation := ledger.PullPresentation{Title: "widget delivery", Body: deliveryBody, Branch: "widget", Head: "aaa", Approved: false}
	ctx := context.Background()

	first, err := backend.PresentPull(ctx, presentation)
	if err != nil {
		t.Fatal(err)
	}
	second, err := backend.PresentPull(ctx, presentation)
	if err != nil || first != second {
		t.Fatalf("repeat presentation = %d, %v; first = %d", second, err, first)
	}
	if pulls := forge.count(http.MethodPost, "/repos/acme/widgets/pulls"); pulls != 1 {
		t.Fatalf("repeat presentation created %d pull requests", pulls)
	}
	if pull := forge.pull(first); pull == nil || pull.Draft != true || pull.Body != deliveryBody {
		t.Fatalf("repeat presentation changed the draft: %#v", pull)
	}
	if mutations := forge.mutations(); len(mutations) != 0 {
		t.Fatalf("pre-approval presentation mutated readiness: %v", mutations)
	}
}

func TestDeliveryPublicationPassReadiesExistingDraft(t *testing.T) {
	forge := newDeliveryForge(t)
	forge.add(deliveryPull{Number: 5, Title: "recorded title", Body: "earlier public body", Draft: true, Head: "aaa"})
	server := forge.server()
	defer server.Close()

	number, err := deliveryBackend(server).PresentPull(context.Background(), ledger.PullPresentation{
		Number: 5, Title: "must not overwrite", Body: "approved public body", Branch: "widget", Head: "aaa", Approved: true,
	})
	if err != nil || number != 5 {
		t.Fatalf("approved presentation = %d, %v", number, err)
	}
	pull := forge.pull(5)
	if pull.Draft != false || pull.Body != "approved public body" {
		t.Fatalf("approved presentation = %#v", pull)
	}
	if pull.Title != "recorded title" {
		t.Fatalf("existing title was rewritten: %q", pull.Title)
	}
	mutations := forge.mutations()
	if len(mutations) != 1 || !strings.Contains(mutations[0], "markPullRequestReadyForReview") {
		t.Fatalf("readiness mutations = %v", mutations)
	}
	if pulls := forge.count(http.MethodPost, "/repos/acme/widgets/pulls"); pulls != 0 {
		t.Fatalf("approved presentation created %d new pull requests", pulls)
	}
	for _, payload := range forge.patchPayloads {
		if len(payload) != 1 || payload["body"] != "approved public body" {
			t.Fatalf("existing pull request update payload = %#v", payload)
		}
	}
}

func TestDeliveryPublicationCompletesPaginatedListing(t *testing.T) {
	forge := newDeliveryForge(t)
	forge.pageSize = 100
	for index := 0; index < 100; index++ {
		forge.add(deliveryPull{Body: deliveryBody, Draft: true, Head: "bbb"})
	}
	forge.add(deliveryPull{Body: "approved public body", Draft: true, Head: "aaa"})
	server := forge.server()
	defer server.Close()

	number, err := deliveryBackend(server).PresentPull(context.Background(), ledger.PullPresentation{
		Body: "approved public body", Branch: "widget", Head: "aaa", Approved: true,
	})
	if err != nil || number != 111 {
		t.Fatalf("paginated presentation = %d, %v", number, err)
	}
	if lists := forge.count(http.MethodGet, "/repos/acme/widgets/pulls"); lists != 2 {
		t.Fatalf("paginated listing stopped after %d pages", lists)
	}
	if pulls := forge.count(http.MethodPost, "/repos/acme/widgets/pulls"); pulls != 0 {
		t.Fatalf("paginated listing created %d pull requests", pulls)
	}
	if pull := forge.pull(number); pull == nil || pull.Draft != false {
		t.Fatalf("paginated match was not readied: %#v", pull)
	}
}

func TestDeliveryPublicationRefusesWrongSourceForReadiness(t *testing.T) {
	cases := map[string]struct {
		number       int
		pull         deliveryPull
		presentation ledger.PullPresentation
		expect       string
	}{
		"listed branch advanced past the expected head": {
			pull:         deliveryPull{Number: 5, Body: deliveryBody, Draft: true, Head: "bbb"},
			presentation: ledger.PullPresentation{Body: deliveryBody, Branch: "widget", Head: "aaa", Approved: true},
			expect:       "not the expected aaa",
		},
		"attached pull request targets another base": {
			number:       5,
			pull:         deliveryPull{Number: 5, Body: deliveryBody, Draft: true, Head: "aaa", Base: "develop"},
			presentation: ledger.PullPresentation{Number: 5, Body: deliveryBody, Branch: "widget", Head: "aaa", Approved: true},
			expect:       "not \"main\"",
		},
		"attached pull request tracks another branch": {
			number:       5,
			pull:         deliveryPull{Number: 5, Body: deliveryBody, Draft: true, Head: "aaa", Branch: "elsewhere"},
			presentation: ledger.PullPresentation{Number: 5, Body: deliveryBody, Branch: "widget", Head: "aaa", Approved: true},
			expect:       "tracks branch",
		},
		"attached pull request is closed": {
			number:       5,
			pull:         deliveryPull{Number: 5, Body: deliveryBody, Draft: true, Head: "aaa", State: "closed"},
			presentation: ledger.PullPresentation{Number: 5, Body: deliveryBody, Branch: "widget", Head: "aaa", Approved: true},
			expect:       "not open",
		},
	}
	for name, scenario := range cases {
		t.Run(name, func(t *testing.T) {
			forge := newDeliveryForge(t)
			forge.add(scenario.pull)
			server := forge.server()
			defer server.Close()

			_, err := deliveryBackend(server).PresentPull(context.Background(), scenario.presentation)
			if err == nil || !strings.Contains(err.Error(), scenario.expect) {
				t.Fatalf("wrong source accepted: %v", err)
			}
			if mutations := forge.mutations(); len(mutations) != 0 {
				t.Fatalf("wrong source mutated readiness: %v", mutations)
			}
			if len(forge.patchPayloads) != 0 {
				t.Fatalf("wrong source was edited: %#v", forge.patchPayloads)
			}
			if pull := forge.pull(scenario.pull.Number); pull != nil && pull.Draft != true {
				t.Fatalf("wrong source became ready: %#v", pull)
			}
		})
	}
}

func TestDeliveryPublicationPreservesOpaquePublicBody(t *testing.T) {
	forge := newDeliveryForge(t)
	forge.add(deliveryPull{Number: 5, Body: "earlier public body", Draft: true, Head: "aaa"})
	server := forge.server()
	defer server.Close()

	number, err := deliveryBackend(server).PresentPull(context.Background(), ledger.PullPresentation{
		Number: 5, Body: deliveryBody, Branch: "widget", Head: "aaa", Approved: false,
	})
	if err != nil || number != 5 {
		t.Fatalf("body refresh = %d, %v", number, err)
	}
	if pull := forge.pull(5); pull.Body != deliveryBody {
		t.Fatalf("public body changed: %q", pull.Body)
	}
	for _, payload := range forge.patchPayloads {
		if payload["body"] != deliveryBody {
			t.Fatalf("body update payload changed the authored text: %#v", payload)
		}
	}
	for _, request := range forge.requestLog() {
		for _, forbidden := range []string{"/labels", "/comments", "/reviews", "/issues", "/merge"} {
			if strings.Contains(request.path, forbidden) {
				t.Fatalf("presentation touched a private or authoritative surface: %s %s", request.method, request.path)
			}
		}
	}
}

func TestDeliveryPublicationResolvesLostCreationWithoutDuplicating(t *testing.T) {
	forge := newDeliveryForge(t)
	forge.createStores = true
	forge.createStatus = http.StatusInternalServerError
	server := forge.server()
	defer server.Close()

	number, err := deliveryBackend(server).PresentPull(context.Background(), ledger.PullPresentation{
		Title: "widget delivery", Body: deliveryBody, Branch: "widget", Head: "aaa", Approved: false,
	})
	if err != nil || number != 11 {
		t.Fatalf("lost creation resolution = %d, %v", number, err)
	}
	if pulls := forge.count(http.MethodPost, "/repos/acme/widgets/pulls"); pulls != 1 {
		t.Fatalf("lost creation created %d pull requests", pulls)
	}
	if pull := forge.pull(number); pull == nil || pull.Body != deliveryBody || pull.Draft != true {
		t.Fatalf("resolved pull request = %#v", pull)
	}
}

func TestDeliveryPublicationRefusesLostCreationWhenListingIsUnavailable(t *testing.T) {
	forge := newDeliveryForge(t)
	forge.createStatus = http.StatusInternalServerError
	forge.listStatus = http.StatusInternalServerError
	forge.listAfterCreate = true
	server := forge.server()
	defer server.Close()

	_, err := deliveryBackend(server).PresentPull(context.Background(), ledger.PullPresentation{
		Title: "widget delivery", Body: deliveryBody, Branch: "widget", Head: "aaa", Approved: true,
	})
	if err == nil || !strings.Contains(err.Error(), "listing was unavailable") {
		t.Fatalf("unavailable resolution listing = %v", err)
	}
	if pulls := forge.count(http.MethodPost, "/repos/acme/widgets/pulls"); pulls != 1 {
		t.Fatalf("unavailable resolution created %d pull requests", pulls)
	}
	if mutations := forge.mutations(); len(mutations) != 0 {
		t.Fatalf("unresolved creation made code ready: %v", mutations)
	}
}

func TestDeliveryPublicationRefusesUnresolvedLostCreation(t *testing.T) {
	forge := newDeliveryForge(t)
	forge.createStores = false
	forge.createStatus = http.StatusInternalServerError
	server := forge.server()
	defer server.Close()

	_, err := deliveryBackend(server).PresentPull(context.Background(), ledger.PullPresentation{
		Title: "widget delivery", Body: deliveryBody, Branch: "widget", Head: "aaa", Approved: false,
	})
	if err == nil || !strings.Contains(err.Error(), "no duplicate was created") {
		t.Fatalf("unresolved creation = %v", err)
	}
	if pulls := forge.count(http.MethodPost, "/repos/acme/widgets/pulls"); pulls != 1 {
		t.Fatalf("unresolved creation created %d pull requests", pulls)
	}
}

func TestDeliveryPublicationUsesOnlyPresentationSurfaces(t *testing.T) {
	forge := newDeliveryForge(t)
	server := forge.server()
	defer server.Close()
	backend := deliveryBackend(server)
	ctx := context.Background()

	number, err := backend.PresentPull(ctx, ledger.PullPresentation{Title: "widget delivery", Body: deliveryBody, Branch: "widget", Head: "aaa", Approved: false})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backend.PresentPull(ctx, ledger.PullPresentation{Number: number, Body: "approved body", Branch: "widget", Head: "aaa", Approved: true}); err != nil {
		t.Fatal(err)
	}
	allowed := func(path string) bool {
		return path == "/graphql" || path == "/repos/acme/widgets/pulls" || strings.HasPrefix(path, "/repos/acme/widgets/pulls/")
	}
	for _, request := range forge.requestLog() {
		if !allowed(request.path) {
			t.Fatalf("presentation used an unauthorized surface: %s %s", request.method, request.path)
		}
	}
	for _, payload := range forge.patchPayloads {
		if _, found := payload["title"]; found {
			t.Fatalf("presentation rewrote the title: %#v", payload)
		}
		for key := range payload {
			if key != "body" {
				t.Fatalf("presentation sent an unexpected field %q: %#v", key, payload)
			}
		}
	}
	if mutations := forge.mutations(); len(mutations) != 1 {
		t.Fatalf("unexpected readiness mutations: %v", mutations)
	}
}
