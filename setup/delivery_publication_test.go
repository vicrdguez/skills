package setup

import (
	"context"
	"encoding/json"
	"errors"
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

	// graphqlDraftStatus, when non-zero, fails only a draft conversion, so a
	// readiness correction can succeed or fail independently of the non-draft
	// mutation it reverses.
	graphqlDraftStatus int
	// moveHeadOnReady, when set, advances the matching pull request's head exactly
	// when a non-draft readiness mutation arrives, simulating a branch that moves
	// between the last preflight read and the readiness mutation.
	moveHeadOnReady string
	// failReadsAfter, when greater than zero, fails pull request reads once that
	// many have succeeded, so a final confirmation can be unavailable.
	failReadsAfter int
	reads          int
	beforePullRead func(*deliveryPull)
	// transientReads and transientPatches fail that many pull request reads
	// or body updates with a server error before serving normally.
	transientReads, transientPatches int
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
		f.reads++
		if f.beforePullRead != nil {
			f.beforePullRead(pull)
		}
		if f.transientReads > 0 {
			f.transientReads--
			http.Error(w, "temporarily unavailable", http.StatusBadGateway)
			return
		}
		if f.failReadsAfter > 0 && f.reads > f.failReadsAfter {
			http.Error(w, "pull request unreadable", http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(pull.record())
	case http.MethodPatch:
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		f.patchPayloads = append(f.patchPayloads, payload)
		if f.transientPatches > 0 {
			f.transientPatches--
			http.Error(w, "temporarily unavailable", http.StatusBadGateway)
			return
		}
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
	ready := strings.Contains(payload.Query, "markPullRequestReadyForReview")
	status := f.graphqlStatus
	if status == 0 && !ready {
		status = f.graphqlDraftStatus
	}
	if status == 0 {
		for _, pull := range f.pulls {
			if pull.NodeID != payload.Variables["id"] {
				continue
			}
			if ready && f.moveHeadOnReady != "" {
				pull.Head = f.moveHeadOnReady
			}
			pull.Draft = !ready
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
	if !errors.Is(err, ledger.ErrPresentationUncertain) || !strings.Contains(err.Error(), "not retried") {
		t.Fatalf("unresolved creation = %v, want reported uncertainty", err)
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

// TestDeliveryPublicationCorrectsReadinessWhenSourceMovesBeforeApproval covers
// the observed gap where the branch advances between the last preflight read and
// the native readiness mutation. The pull request must not stay presented as an
// approval of code that was never reviewed, and the newer source and the
// authored body must survive the correction.
func TestDeliveryPublicationCorrectsReadinessWhenSourceMovesBeforeApproval(t *testing.T) {
	forge := newDeliveryForge(t)
	forge.add(deliveryPull{Number: 5, Body: "earlier public body", Draft: true, Head: "aaa"})
	forge.moveHeadOnReady = "bbb-unreviewed"
	server := forge.server()
	defer server.Close()

	_, err := deliveryBackend(server).PresentPull(context.Background(), ledger.PullPresentation{
		Number: 5, Body: "approved aaa", Branch: "widget", Head: "aaa", Approved: true,
	})
	if err == nil || !strings.Contains(err.Error(), "not the expected aaa") {
		t.Fatalf("source movement was not refused: %v", err)
	}
	pull := forge.pull(5)
	if pull == nil || !pull.Draft {
		t.Fatalf("unreviewed code stayed ready: %#v", pull)
	}
	if pull.Head != "bbb-unreviewed" {
		t.Fatalf("newer source was rewritten: %q", pull.Head)
	}
	if pull.Body != "approved aaa" {
		t.Fatalf("authored body was not preserved: %q", pull.Body)
	}
	mutations := forge.mutations()
	if len(mutations) != 2 || !strings.Contains(mutations[0], "markPullRequestReadyForReview") || !strings.Contains(mutations[1], "convertPullRequestToDraft") {
		t.Fatalf("readiness reconciliation mutations = %v", mutations)
	}
}

// TestDeliveryPublicationReportsUnresolvedReadinessCorrection covers the
// correction itself failing. The adapter must not report the mismatch as if the
// ready presentation had been made safe; it names the unresolved state so the
// operator inspects the pull request.
func TestDeliveryPublicationReportsUnresolvedReadinessCorrection(t *testing.T) {
	forge := newDeliveryForge(t)
	forge.add(deliveryPull{Number: 5, Body: "earlier public body", Draft: true, Head: "aaa"})
	forge.moveHeadOnReady = "bbb-unreviewed"
	forge.graphqlDraftStatus = http.StatusInternalServerError
	server := forge.server()
	defer server.Close()

	_, err := deliveryBackend(server).PresentPull(context.Background(), ledger.PullPresentation{
		Number: 5, Body: "approved aaa", Branch: "widget", Head: "aaa", Approved: true,
	})
	if err == nil || !strings.Contains(err.Error(), "unresolved") {
		t.Fatalf("unresolved correction was not reported: %v", err)
	}
	pull := forge.pull(5)
	if pull == nil || pull.Draft {
		t.Fatalf("test expected the uncorrected ready state to stay observable: %#v", pull)
	}
	if pull.Head != "bbb-unreviewed" || pull.Body != "approved aaa" {
		t.Fatalf("correction failure rewrote source or body: %#v", pull)
	}
}

func TestDeliveryPublicationPreservesNewerApprovalDuringCorrection(t *testing.T) {
	for _, scenario := range []struct {
		name       string
		beforeRead int
		head, body string
	}{
		{"new approval before mismatch read", 3, "bbb-unreviewed", "approved bbb"},
		{"new approval before correction read", 4, "ccc-reviewed", "approved ccc"},
		{"further source movement with unchanged body", 4, "ccc-reviewed", "approved aaa"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			forge := newDeliveryForge(t)
			forge.add(deliveryPull{Number: 5, Body: "earlier public body", Draft: true, Head: "aaa"})
			forge.moveHeadOnReady = "bbb-unreviewed"
			forge.beforePullRead = func(pull *deliveryPull) {
				if forge.reads == scenario.beforeRead {
					pull.Head, pull.Body = scenario.head, scenario.body
					pull.Draft = false
				}
			}
			server := forge.server()
			defer server.Close()

			_, err := deliveryBackend(server).PresentPull(context.Background(), ledger.PullPresentation{
				Number: 5, Body: "approved aaa", Branch: "widget", Head: "aaa", Approved: true,
			})
			if err == nil || !strings.Contains(err.Error(), "unresolved") {
				t.Errorf("superseded correction was not reported unresolved: %v", err)
			}
			pull := forge.pull(5)
			if pull.Draft || pull.Head != scenario.head || pull.Body != scenario.body {
				t.Fatalf("old attempt changed newer approval: %#v", pull)
			}
			if mutations := forge.mutations(); len(mutations) != 1 {
				t.Fatalf("old attempt dispatched a correction over newer work: %v", mutations)
			}
		})
	}
}

func TestDeliveryPublicationReportsUnconfirmedReadinessCorrection(t *testing.T) {
	for _, unreadable := range []bool{false, true} {
		t.Run(fmt.Sprintf("unreadable=%t", unreadable), func(t *testing.T) {
			forge := newDeliveryForge(t)
			forge.add(deliveryPull{Number: 5, Body: "earlier public body", Draft: true, Head: "aaa"})
			forge.moveHeadOnReady = "bbb-unreviewed"
			if unreadable {
				forge.failReadsAfter = 4
			} else {
				forge.beforePullRead = func(pull *deliveryPull) {
					if forge.reads == 5 {
						pull.Draft = false
					}
				}
			}
			server := forge.server()
			defer server.Close()

			_, err := deliveryBackend(server).PresentPull(context.Background(), ledger.PullPresentation{
				Number: 5, Body: "approved aaa", Branch: "widget", Head: "aaa", Approved: true,
			})
			if err == nil || !strings.Contains(err.Error(), "correction remains unresolved") {
				t.Fatalf("unconfirmed correction was not reported unresolved: %v", err)
			}
			if mutations := forge.mutations(); len(mutations) != 2 {
				t.Fatalf("expected one bounded correction, got %v", mutations)
			}
		})
	}
}

// TestDeliveryPublicationPreservesNewerProseOnSourceMismatch covers preserving
// unrelated authored content: a presentation for a stale head must not overwrite
// the pull request's current body or readiness.
func TestDeliveryPublicationPreservesNewerProseOnSourceMismatch(t *testing.T) {
	forge := newDeliveryForge(t)
	forge.add(deliveryPull{Number: 5, Body: "newer authored prose", Draft: true, Head: "bbb-unreviewed"})
	server := forge.server()
	defer server.Close()

	_, err := deliveryBackend(server).PresentPull(context.Background(), ledger.PullPresentation{
		Number: 5, Body: "approved aaa", Branch: "widget", Head: "aaa", Approved: true,
	})
	if err == nil || !strings.Contains(err.Error(), "not the expected aaa") {
		t.Fatalf("stale presentation was not refused: %v", err)
	}
	pull := forge.pull(5)
	if pull.Body != "newer authored prose" || !pull.Draft {
		t.Fatalf("stale presentation rewrote newer prose or readiness: %#v", pull)
	}
	if len(forge.patchPayloads) != 0 || len(forge.mutations()) != 0 {
		t.Fatalf("stale presentation wrote to the forge: patches=%v mutations=%v", forge.patchPayloads, forge.mutations())
	}
}

// TestDeliveryPublicationDoesNotReverseReadinessItDidNotEstablish covers the
// ownership boundary: when the ready presentation already exists, this attempt
// dispatches no readiness mutation, so an unconfirmable final read must not
// convert someone else's ready state back to draft.
func TestDeliveryPublicationDoesNotReverseReadinessItDidNotEstablish(t *testing.T) {
	forge := newDeliveryForge(t)
	forge.add(deliveryPull{Number: 5, Body: "approved aaa", Draft: false, Head: "aaa"})
	forge.failReadsAfter = 2
	server := forge.server()
	defer server.Close()

	_, err := deliveryBackend(server).PresentPull(context.Background(), ledger.PullPresentation{
		Number: 5, Body: "approved aaa", Branch: "widget", Head: "aaa", Approved: true,
	})
	if err == nil {
		t.Fatal("unreadable final confirmation was accepted")
	}
	if mutations := forge.mutations(); len(mutations) != 0 {
		t.Fatalf("readiness this attempt did not establish was mutated: %v", mutations)
	}
	if pull := forge.pull(5); pull == nil || pull.Draft {
		t.Fatalf("unrelated ready presentation was reverted: %#v", pull)
	}
}

// TestDeliveryPublicationRetriesRepeatableRequestsBoundedly covers B4: reads
// and body updates are retried immediately after a server error, within a
// fixed bound, and a persistent outage is reported rather than retried on.
func TestDeliveryPublicationRetriesRepeatableRequestsBoundedly(t *testing.T) {
	t.Run("transient failures recover", func(t *testing.T) {
		forge := newDeliveryForge(t)
		existing := forge.add(deliveryPull{Body: "old body", Draft: true})
		forge.transientReads, forge.transientPatches = 2, 2
		server := forge.server()
		defer server.Close()

		number, err := deliveryBackend(server).PresentPull(context.Background(), ledger.PullPresentation{
			Number: existing.Number, Body: deliveryBody, Branch: "widget", Head: "aaa",
		})
		if err != nil || number != existing.Number {
			t.Fatalf("presentation after transient failures = %d, %v", number, err)
		}
		if pull := forge.pull(existing.Number); pull.Body != deliveryBody || !pull.Draft {
			t.Fatalf("presented pull = %#v", pull)
		}
		if patches := forge.count(http.MethodPatch, fmt.Sprintf("/repos/acme/widgets/pulls/%d", existing.Number)); patches != 3 {
			t.Fatalf("body updates = %d, want two failures and one success", patches)
		}
	})

	t.Run("persistent failure is bounded", func(t *testing.T) {
		forge := newDeliveryForge(t)
		existing := forge.add(deliveryPull{Body: "old body", Draft: true})
		forge.transientReads = 1000
		server := forge.server()
		defer server.Close()

		_, err := deliveryBackend(server).PresentPull(context.Background(), ledger.PullPresentation{
			Number: existing.Number, Body: deliveryBody, Branch: "widget", Head: "aaa",
		})
		if err == nil || !strings.Contains(err.Error(), "502") {
			t.Fatalf("persistent outage = %v, want the server error", err)
		}
		if reads := forge.count(http.MethodGet, fmt.Sprintf("/repos/acme/widgets/pulls/%d", existing.Number)); reads != presentationAttempts {
			t.Fatalf("reads = %d, want the bound %d", reads, presentationAttempts)
		}
		if patches := forge.count(http.MethodPatch, fmt.Sprintf("/repos/acme/widgets/pulls/%d", existing.Number)); patches != 0 {
			t.Fatalf("an unreadable pull request was updated %d times", patches)
		}
	})
}

// TestDeliveryPublicationStopsUpdatesForSupersededInputs covers B4: once the
// selected local result is superseded, the adapter makes no further write, so
// an approval is not applied after its inputs changed.
func TestDeliveryPublicationStopsUpdatesForSupersededInputs(t *testing.T) {
	forge := newDeliveryForge(t)
	existing := forge.add(deliveryPull{Body: "old body", Draft: true})
	server := forge.server()
	defer server.Close()

	checks := 0
	_, err := deliveryBackend(server).PresentPull(context.Background(), ledger.PullPresentation{
		Number: existing.Number, Body: deliveryBody, Branch: "widget", Head: "aaa", Approved: true,
		Current: func() error {
			checks++
			if checks > 1 {
				return errors.New("a later local review result supersedes the selected result")
			}
			return nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "further pull request updates stopped") {
		t.Fatalf("superseded presentation = %v", err)
	}
	if pull := forge.pull(existing.Number); pull.Body != deliveryBody || !pull.Draft {
		t.Fatalf("pull = %#v, want the first update only and no ready presentation", pull)
	}
	if mutations := forge.mutations(); len(mutations) != 0 {
		t.Fatalf("readiness was changed for superseded inputs: %v", mutations)
	}
}
