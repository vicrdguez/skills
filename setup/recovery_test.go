package setup

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

// recoveryIssueRecord is the forge-side descriptive issue a controlled HTTP
// server keeps, so the adapter's effects are observed at the transport seam.
type recoveryIssueRecord struct {
	Number      int
	ID          int64
	Title       string
	Body        string
	State       string
	PullRequest bool
}

type recoveryInlineComment struct {
	Body   string
	Commit string
	Path   string
	Line   int
	Side   string
}

// recoveryFaults injects controlled HTTP failures. A fault with Stores set
// models a mutation whose response was lost after it took effect; without
// Stores the mutation did not apply.
type recoveryFaults struct {
	createIssue         int
	createIssueStores   bool
	listIssues          int
	issueRead           int
	issuePatch          int
	issuePatchStores    bool
	listChildren        int
	attach              int
	attachStores        bool
	createPull          int
	createPullStores    bool
	pullRead            int
	pullPatch           int
	pullPatchStores     bool
	graphql             int
	graphqlStores       bool
	files               int
	comments            int
	createComment       int
	createCommentStores bool
}

// recoveryForge is the controlled GitHub surface for recovery: descriptive
// issues, parent grouping, pull requests, reviewed diffs, and inline
// comments. It records every request so effects, counts, and privacy can be
// asserted without a live forge.
type recoveryForge struct {
	t *testing.T

	mu            sync.Mutex
	issues        map[int]*recoveryIssueRecord
	issueOrder    []int
	nextIssue     int
	pulls         map[int]*deliveryPull
	pullOrder     []int
	nextPull      int
	links         map[int][]int
	files         []pullFile
	comments      []recoveryInlineComment
	issuePageSize int
	pullPageSize  int
	filePageSize  int

	requests []string

	issueCreates  []map[string]any
	issuePatches  []map[string]any
	attachments   []map[string]any
	pullCreates   []map[string]any
	pullPatches   []map[string]any
	commentWrites []map[string]any
	queries       []string

	faults         recoveryFaults
	headAfterReady string
}

func newRecoveryForge(t *testing.T) *recoveryForge {
	return &recoveryForge{
		t: t, issues: map[int]*recoveryIssueRecord{}, pulls: map[int]*deliveryPull{},
		links: map[int][]int{}, nextIssue: 100, nextPull: 10,
	}
}

func (f *recoveryForge) server() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(f.serve))
}

func (f *recoveryForge) putIssue(record recoveryIssueRecord) *recoveryIssueRecord {
	if record.Number == 0 {
		f.nextIssue++
		record.Number = f.nextIssue
	}
	if record.ID == 0 {
		record.ID = int64(record.Number) * 1000
	}
	if record.State == "" {
		record.State = "open"
	}
	stored := record
	f.issues[record.Number] = &stored
	f.issueOrder = append(f.issueOrder, record.Number)
	return &stored
}

func (f *recoveryForge) addIssue(record recoveryIssueRecord) *recoveryIssueRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.putIssue(record)
}

func (f *recoveryForge) putPull(pull deliveryPull) *deliveryPull {
	if pull.Number == 0 {
		f.nextPull++
		pull.Number = f.nextPull
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
		pull.Head = "aaa"
	}
	f.pulls[pull.Number] = &pull
	f.pullOrder = append(f.pullOrder, pull.Number)
	return &pull
}

func (f *recoveryForge) addPull(pull deliveryPull) *deliveryPull {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.putPull(pull)
}

func (f *recoveryForge) link(parent, child int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.links[parent] = append(f.links[parent], child)
}

func (f *recoveryForge) setPullHead(number int, head string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if pull := f.pulls[number]; pull != nil {
		pull.Head = head
	}
}

func (f *recoveryForge) pull(number int) *deliveryPull {
	f.mu.Lock()
	defer f.mu.Unlock()
	if pull := f.pulls[number]; pull != nil {
		snapshot := *pull
		return &snapshot
	}
	return nil
}

func (f *recoveryForge) issue(number int) *recoveryIssueRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	if issue := f.issues[number]; issue != nil {
		snapshot := *issue
		return &snapshot
	}
	return nil
}

func (f *recoveryForge) count(method, path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	total := 0
	for _, request := range f.requests {
		if request == method+" "+path {
			total++
		}
	}
	return total
}

func (f *recoveryForge) requestLog() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.requests...)
}

func (f *recoveryForge) mutations() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.queries...)
}

func (f *recoveryForge) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, r.Method+" "+r.URL.Path)
	f.mu.Unlock()
	switch {
	case r.URL.Path == "/graphql":
		f.serveGraphQL(w, r)
	case r.URL.Path == "/repos/acme/widgets/issues" && r.Method == http.MethodGet:
		f.serveIssueList(w, r)
	case r.URL.Path == "/repos/acme/widgets/issues" && r.Method == http.MethodPost:
		f.serveIssueCreate(w, r)
	case strings.HasPrefix(r.URL.Path, "/repos/acme/widgets/issues/"):
		f.serveIssue(w, r)
	case r.URL.Path == "/repos/acme/widgets/pulls" && r.Method == http.MethodGet:
		f.servePullList(w, r)
	case r.URL.Path == "/repos/acme/widgets/pulls" && r.Method == http.MethodPost:
		f.servePullCreate(w, r)
	case strings.HasPrefix(r.URL.Path, "/repos/acme/widgets/pulls/"):
		f.servePull(w, r)
	default:
		f.t.Errorf("unexpected forge request %s %s", r.Method, r.URL)
		http.NotFound(w, r)
	}
}

func recoveryPage[T any](records []T, r *http.Request, size int) []T {
	if size <= 0 {
		return records
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	start := (page - 1) * size
	if start > len(records) {
		start = len(records)
	}
	end := start + size
	if end > len(records) {
		end = len(records)
	}
	return records[start:end]
}

func recoveryIssueJSON(issue *recoveryIssueRecord) map[string]any {
	record := map[string]any{
		"number": issue.Number, "id": issue.ID, "title": issue.Title, "body": issue.Body,
		"state": issue.State, "labels": []any{}, "created_at": "2024-01-01T00:00:00Z",
	}
	if issue.PullRequest {
		record["pull_request"] = map[string]any{"url": "https://example.invalid/pull"}
	}
	return record
}

func recoveryCommentJSON(comment recoveryInlineComment) map[string]any {
	return map[string]any{
		"body": comment.Body, "commit_id": comment.Commit, "path": comment.Path,
		"line": comment.Line, "side": comment.Side, "author_association": "MEMBER",
		"created_at": "2024-01-01T00:00:00Z", "user": map[string]string{"login": "reviewer"},
	}
}

func (f *recoveryForge) serveIssueList(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.faults.listIssues != 0 {
		http.Error(w, "issue listing unavailable", f.faults.listIssues)
		return
	}
	records := []map[string]any{}
	for _, number := range f.issueOrder {
		records = append(records, recoveryIssueJSON(f.issues[number]))
	}
	json.NewEncoder(w).Encode(recoveryPage(records, r, f.issuePageSize))
}

func (f *recoveryForge) serveIssueCreate(w http.ResponseWriter, r *http.Request) {
	var payload map[string]any
	json.NewDecoder(r.Body).Decode(&payload)
	f.mu.Lock()
	f.issueCreates = append(f.issueCreates, payload)
	status := f.faults.createIssue
	var created *recoveryIssueRecord
	if status == 0 || f.faults.createIssueStores {
		title, _ := payload["title"].(string)
		body, _ := payload["body"].(string)
		created = f.putIssue(recoveryIssueRecord{Title: title, Body: body})
	}
	f.mu.Unlock()
	if status != 0 {
		http.Error(w, "issue creation response lost", status)
		return
	}
	if created == nil {
		http.Error(w, "no issue created", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(recoveryIssueJSON(created))
}

func (f *recoveryForge) serveIssue(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/repos/acme/widgets/issues/"), "/")
	number, err := strconv.Atoi(parts[0])
	if err != nil {
		http.NotFound(w, r)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		if f.faults.issueRead != 0 {
			http.Error(w, "issue unreadable", f.faults.issueRead)
			return
		}
		issue, found := f.issues[number]
		if !found {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(recoveryIssueJSON(issue))
	case len(parts) == 1 && r.Method == http.MethodPatch:
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		f.issuePatches = append(f.issuePatches, payload)
		issue, found := f.issues[number]
		if !found {
			http.NotFound(w, r)
			return
		}
		if f.faults.issuePatch == 0 || f.faults.issuePatchStores {
			if body, ok := payload["body"].(string); ok {
				issue.Body = body
			}
		}
		if f.faults.issuePatch != 0 {
			http.Error(w, "issue update rejected", f.faults.issuePatch)
			return
		}
		json.NewEncoder(w).Encode(recoveryIssueJSON(issue))
	case len(parts) == 2 && parts[1] == "sub_issues" && r.Method == http.MethodGet:
		if f.faults.listChildren != 0 {
			http.Error(w, "children unavailable", f.faults.listChildren)
			return
		}
		children := []map[string]any{}
		for _, child := range f.links[number] {
			if issue := f.issues[child]; issue != nil {
				children = append(children, map[string]any{"number": issue.Number, "id": issue.ID})
			}
		}
		json.NewEncoder(w).Encode(recoveryPage(children, r, 0))
	case len(parts) == 2 && parts[1] == "sub_issues" && r.Method == http.MethodPost:
		var payload struct {
			SubIssueID int64 `json:"sub_issue_id"`
		}
		json.NewDecoder(r.Body).Decode(&payload)
		f.attachments = append(f.attachments, map[string]any{"parent": number, "sub_issue_id": payload.SubIssueID})
		child := 0
		for _, candidate := range f.issues {
			if candidate.ID == payload.SubIssueID {
				child = candidate.Number
			}
		}
		if child == 0 {
			http.Error(w, "unknown sub issue", http.StatusUnprocessableEntity)
			return
		}
		if f.faults.attach != 0 && !f.faults.attachStores {
			http.Error(w, "attach rejected", f.faults.attach)
			return
		}
		f.links[number] = append(f.links[number], child)
		if f.faults.attach != 0 {
			http.Error(w, "attach response lost", f.faults.attach)
			return
		}
		fmt.Fprint(w, `{}`)
	default:
		http.NotFound(w, r)
	}
}

func (f *recoveryForge) servePullList(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	state := r.URL.Query().Get("state")
	base := r.URL.Query().Get("base")
	head := r.URL.Query().Get("head")
	records := []map[string]any{}
	for _, number := range f.pullOrder {
		pull := f.pulls[number]
		if state != "" && state != "all" && pull.State != state {
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
	json.NewEncoder(w).Encode(recoveryPage(records, r, f.pullPageSize))
}

func (f *recoveryForge) servePullCreate(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Title string `json:"title"`
		Head  string `json:"head"`
		Base  string `json:"base"`
		Body  string `json:"body"`
		Draft bool   `json:"draft"`
	}
	json.NewDecoder(r.Body).Decode(&payload)
	f.mu.Lock()
	f.pullCreates = append(f.pullCreates, map[string]any{
		"title": payload.Title, "head": payload.Head, "base": payload.Base, "body": payload.Body, "draft": payload.Draft,
	})
	status := f.faults.createPull
	var created *deliveryPull
	if status == 0 || f.faults.createPullStores {
		created = f.putPull(deliveryPull{Title: payload.Title, Body: payload.Body, Draft: payload.Draft, Branch: payload.Head, Base: payload.Base})
	}
	f.mu.Unlock()
	if status != 0 {
		http.Error(w, "pull request creation response lost", status)
		return
	}
	if created == nil {
		http.Error(w, "no pull request created", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(created.record())
}

func (f *recoveryForge) servePull(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/repos/acme/widgets/pulls/"), "/")
	number, err := strconv.Atoi(parts[0])
	if err != nil {
		http.NotFound(w, r)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		if f.faults.pullRead != 0 {
			http.Error(w, "pull request unreadable", f.faults.pullRead)
			return
		}
		pull, found := f.pulls[number]
		if !found {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(pull.record())
	case len(parts) == 1 && r.Method == http.MethodPatch:
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		f.pullPatches = append(f.pullPatches, payload)
		pull, found := f.pulls[number]
		if !found {
			http.NotFound(w, r)
			return
		}
		if f.faults.pullPatch == 0 || f.faults.pullPatchStores {
			if body, ok := payload["body"].(string); ok {
				pull.Body = body
			}
		}
		if f.faults.pullPatch != 0 {
			http.Error(w, "pull request update rejected", f.faults.pullPatch)
			return
		}
		json.NewEncoder(w).Encode(pull.record())
	case len(parts) == 2 && parts[1] == "files" && r.Method == http.MethodGet:
		if f.faults.files != 0 {
			http.Error(w, "reviewed diff unavailable", f.faults.files)
			return
		}
		json.NewEncoder(w).Encode(recoveryPage(f.files, r, f.filePageSize))
	case len(parts) == 2 && parts[1] == "comments" && r.Method == http.MethodGet:
		if f.faults.comments != 0 {
			http.Error(w, "inline discussion unavailable", f.faults.comments)
			return
		}
		records := []map[string]any{}
		for _, comment := range f.comments {
			records = append(records, recoveryCommentJSON(comment))
		}
		json.NewEncoder(w).Encode(records)
	case len(parts) == 2 && parts[1] == "comments" && r.Method == http.MethodPost:
		var payload struct {
			Body   string `json:"body"`
			Commit string `json:"commit_id"`
			Path   string `json:"path"`
			Line   int    `json:"line"`
			Side   string `json:"side"`
		}
		json.NewDecoder(r.Body).Decode(&payload)
		f.commentWrites = append(f.commentWrites, map[string]any{"body": payload.Body, "commit_id": payload.Commit, "path": payload.Path, "line": payload.Line, "side": payload.Side})
		if f.faults.createComment == 0 || f.faults.createCommentStores {
			f.comments = append(f.comments, recoveryInlineComment{Body: payload.Body, Commit: payload.Commit, Path: payload.Path, Line: payload.Line, Side: payload.Side})
		}
		if f.faults.createComment != 0 {
			http.Error(w, "inline comment response lost", f.faults.createComment)
			return
		}
		fmt.Fprint(w, `{}`)
	default:
		http.NotFound(w, r)
	}
}

func (f *recoveryForge) serveGraphQL(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Query     string            `json:"query"`
		Variables map[string]string `json:"variables"`
	}
	json.NewDecoder(r.Body).Decode(&payload)
	ready := strings.Contains(payload.Query, "markPullRequestReadyForReview")
	f.mu.Lock()
	f.queries = append(f.queries, payload.Query)
	status := f.faults.graphql
	if status == 0 || f.faults.graphqlStores {
		for _, pull := range f.pulls {
			if pull.NodeID != payload.Variables["id"] {
				continue
			}
			if ready && f.headAfterReady != "" {
				pull.Head = f.headAfterReady
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

func recoveryPointer(value string) *string { return &value }

// recoverySatisfied reports whether the adapter described the presentation as
// complete, whether it wrote the remaining effects or observed them already in
// place.
func recoverySatisfied(receipt ledger.RecoveryReceipt) bool {
	return receipt.Status == recoveryPublished || receipt.Status == recoveryAlreadySatisfied
}

func TestRecoveryIssueReusesKnownAttachmentWithoutDuplicate(t *testing.T) {
	forge := newRecoveryForge(t)
	issue := forge.addIssue(recoveryIssueRecord{Title: "Add order cancellation", Body: "public acceptance body"})
	server := forge.server()
	defer server.Close()

	receipt, err := deliveryBackend(server).RecoverPresentation(context.Background(), ledger.RecoveryPresentation{
		Kind: "issue", Number: issue.Number, Title: issue.Title, Body: recoveryPointer("public acceptance body"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Number != issue.Number || !recoverySatisfied(receipt) {
		t.Fatalf("receipt = %#v", receipt)
	}
	if creates := forge.count(http.MethodPost, "/repos/acme/widgets/issues"); creates != 0 {
		t.Fatalf("recovery created %d issues", creates)
	}
	if len(forge.issuePatches) != 0 {
		t.Fatalf("recovery patched the unchanged body: %#v", forge.issuePatches)
	}
}

func TestRecoveryIssueResolvesLostCreateByExactDigest(t *testing.T) {
	forge := newRecoveryForge(t)
	original := "original lost acceptance body"
	issue := forge.addIssue(recoveryIssueRecord{Title: "Add order cancellation", Body: original})
	server := forge.server()
	defer server.Close()

	receipt, err := deliveryBackend(server).RecoverPresentation(context.Background(), ledger.RecoveryPresentation{
		Kind: "issue", Number: 0, Title: issue.Title,
		OriginalBodySHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(original))),
		Body:               recoveryPointer("freshly authored current body"), MayHaveCreated: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Number != issue.Number || !recoverySatisfied(receipt) {
		t.Fatalf("receipt = %#v", receipt)
	}
	if creates := forge.count(http.MethodPost, "/repos/acme/widgets/issues"); creates != 0 {
		t.Fatalf("recovery created %d duplicate issues", creates)
	}
	if len(forge.issuePatches) != 1 || forge.issuePatches[0]["body"] != "freshly authored current body" {
		t.Fatalf("body updates = %#v", forge.issuePatches)
	}
	if observed := forge.issue(issue.Number); observed == nil || observed.Body != "freshly authored current body" {
		t.Fatalf("attachment body = %#v", observed)
	}
}

func TestRecoveryIssueRefusesAmbiguousExactMatches(t *testing.T) {
	forge := newRecoveryForge(t)
	original := "shared original body"
	forge.addIssue(recoveryIssueRecord{Title: "Add order cancellation", Body: original})
	forge.addIssue(recoveryIssueRecord{Title: "Add order cancellation", Body: original})
	server := forge.server()
	defer server.Close()

	_, err := deliveryBackend(server).RecoverPresentation(context.Background(), ledger.RecoveryPresentation{
		Kind: "issue", Title: "Add order cancellation",
		OriginalBodySHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(original))), MayHaveCreated: true,
	})
	if err == nil || !strings.Contains(err.Error(), "multiple open issues") {
		t.Fatalf("ambiguous matches were not refused: %v", err)
	}
	if creates := forge.count(http.MethodPost, "/repos/acme/widgets/issues"); creates != 0 {
		t.Fatalf("ambiguous matches created %d issues", creates)
	}
}

func TestRecoveryIssueRefusesUnconfirmedAbsence(t *testing.T) {
	forge := newRecoveryForge(t)
	forge.addIssue(recoveryIssueRecord{Title: "Add order cancellation", Body: "different body"})
	server := forge.server()
	defer server.Close()

	receipt, err := deliveryBackend(server).RecoverPresentation(context.Background(), ledger.RecoveryPresentation{
		Kind: "issue", Title: "Add order cancellation",
		OriginalBodySHA256: fmt.Sprintf("%x", sha256.Sum256([]byte("unobserved attempt body"))), MayHaveCreated: true,
		Body: recoveryPointer("fresh prose"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != recoveryAmbiguous {
		t.Fatalf("receipt = %#v", receipt)
	}
	if creates := forge.count(http.MethodPost, "/repos/acme/widgets/issues"); creates != 0 {
		t.Fatalf("unconfirmed absence created %d issues", creates)
	}
}

func TestRecoveryIssueCreatesFreshWithoutUncertainAttempt(t *testing.T) {
	forge := newRecoveryForge(t)
	server := forge.server()
	defer server.Close()

	receipt, err := deliveryBackend(server).RecoverPresentation(context.Background(), ledger.RecoveryPresentation{
		Kind: "issue", Title: "Add order cancellation", Body: recoveryPointer("authored acceptance body"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if creates := forge.count(http.MethodPost, "/repos/acme/widgets/issues"); creates != 1 {
		t.Fatalf("fresh creation posted %d issues", creates)
	}
	created := forge.issue(receipt.Number)
	if created == nil || created.Body != "authored acceptance body" || !recoverySatisfied(receipt) {
		t.Fatalf("receipt = %#v, created = %#v", receipt, created)
	}
	if len(forge.issuePatches) != 0 {
		t.Fatalf("fresh creation rewrote its body: %#v", forge.issuePatches)
	}
}

func TestRecoveryIssueResolvesLostCreateResponseWithoutDuplicate(t *testing.T) {
	forge := newRecoveryForge(t)
	forge.faults.createIssue = http.StatusInternalServerError
	forge.faults.createIssueStores = true
	server := forge.server()
	defer server.Close()

	receipt, err := deliveryBackend(server).RecoverPresentation(context.Background(), ledger.RecoveryPresentation{
		Kind: "issue", Title: "Add order cancellation", Body: recoveryPointer("authored acceptance body"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !recoverySatisfied(receipt) {
		t.Fatalf("receipt = %#v", receipt)
	}
	if creates := forge.count(http.MethodPost, "/repos/acme/widgets/issues"); creates != 1 {
		t.Fatalf("lost creation posted %d issues", creates)
	}
}

func TestRecoveryIssueDoesNotAdoptOnFreshCreate(t *testing.T) {
	forge := newRecoveryForge(t)
	existing := forge.addIssue(recoveryIssueRecord{Title: "Add order cancellation", Body: "authored acceptance body"})
	server := forge.server()
	defer server.Close()

	receipt, err := deliveryBackend(server).RecoverPresentation(context.Background(), ledger.RecoveryPresentation{
		Kind: "issue", Title: "Add order cancellation", Body: recoveryPointer("authored acceptance body"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Number == existing.Number {
		t.Fatalf("a fresh create adopted a pre-existing matching issue: %#v", receipt)
	}
	if creates := forge.count(http.MethodPost, "/repos/acme/widgets/issues"); creates != 1 {
		t.Fatalf("fresh create posted %d issues", creates)
	}
}

func TestRecoveryIssueRefusesReassignedAttachment(t *testing.T) {
	forge := newRecoveryForge(t)
	issue := forge.addIssue(recoveryIssueRecord{Title: "Different accepted title", Body: "public body"})
	server := forge.server()
	defer server.Close()

	_, err := deliveryBackend(server).RecoverPresentation(context.Background(), ledger.RecoveryPresentation{
		Kind: "issue", Number: issue.Number, Title: "Add order cancellation", Body: recoveryPointer("public body"),
	})
	if err == nil || !strings.Contains(err.Error(), "presents title") {
		t.Fatalf("reassigned attachment was not refused: %v", err)
	}
	if len(forge.issuePatches) != 0 {
		t.Fatalf("reassigned attachment was rewritten: %#v", forge.issuePatches)
	}
}

func TestRecoveryIssueObserveOnlyMakesNoMutations(t *testing.T) {
	forge := newRecoveryForge(t)
	issue := forge.addIssue(recoveryIssueRecord{Title: "Add order cancellation", Body: "old body"})
	server := forge.server()
	defer server.Close()

	receipt, err := deliveryBackend(server).RecoverPresentation(context.Background(), ledger.RecoveryPresentation{
		Kind: "issue", Number: issue.Number, Title: issue.Title, Body: recoveryPointer("new body"), ObserveOnly: true,
	})
	if err != nil || receipt.Status != recoveryPending || receipt.Number != issue.Number {
		t.Fatalf("receipt = %#v, %v", receipt, err)
	}
	if len(forge.issuePatches) != 0 {
		t.Fatalf("observation wrote a body: %#v", forge.issuePatches)
	}
}

func TestRecoveryIssueGuardBlocksCreate(t *testing.T) {
	forge := newRecoveryForge(t)
	server := forge.server()
	defer server.Close()

	_, err := deliveryBackend(server).RecoverPresentation(context.Background(), ledger.RecoveryPresentation{
		Kind: "issue", Title: "Add order cancellation", Body: recoveryPointer("authored acceptance body"),
		Guard: func() error { return fmt.Errorf("the selected view changed") },
	})
	if err == nil || !strings.Contains(err.Error(), "selected view changed") {
		t.Fatalf("guard failure was ignored: %v", err)
	}
	if creates := forge.count(http.MethodPost, "/repos/acme/widgets/issues"); creates != 0 {
		t.Fatalf("guarded recovery created %d issues", creates)
	}
}

func TestRecoveryIssuePaginatesResolutionListing(t *testing.T) {
	forge := newRecoveryForge(t)
	forge.issuePageSize = 100
	original := "paged original body"
	for index := 0; index < 100; index++ {
		forge.addIssue(recoveryIssueRecord{Title: fmt.Sprintf("Unrelated issue %03d", index), Body: "other"})
	}
	target := forge.addIssue(recoveryIssueRecord{Title: "Add order cancellation", Body: original})
	server := forge.server()
	defer server.Close()

	receipt, err := deliveryBackend(server).RecoverPresentation(context.Background(), ledger.RecoveryPresentation{
		Kind: "issue", Title: "Add order cancellation",
		OriginalBodySHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(original))), MayHaveCreated: true,
	})
	if err != nil || receipt.Number != target.Number {
		t.Fatalf("paged resolution = %#v, %v", receipt, err)
	}
	if lists := forge.count(http.MethodGet, "/repos/acme/widgets/issues"); lists != 2 {
		t.Fatalf("resolution listing used %d pages", lists)
	}
}

func TestRecoveryParentGroupsOnlyMissingChildren(t *testing.T) {
	forge := newRecoveryForge(t)
	parent := forge.addIssue(recoveryIssueRecord{Title: "Parent proposal", Body: "parent body"})
	linked := forge.addIssue(recoveryIssueRecord{Title: "First slice", Body: "first body"})
	missing := forge.addIssue(recoveryIssueRecord{Title: "Second slice", Body: "second body"})
	forge.link(parent.Number, linked.Number)
	server := forge.server()
	defer server.Close()

	receipt, err := deliveryBackend(server).RecoverPresentation(context.Background(), ledger.RecoveryPresentation{
		Kind: "parent", Number: parent.Number, Title: parent.Title, Body: recoveryPointer("parent body"),
		Children: []ledger.ForgeAttachment{{Repository: "acme/widgets", Number: linked.Number}, {Repository: "acme/widgets", Number: missing.Number}},
	})
	if err != nil || !recoverySatisfied(receipt) || receipt.Number != parent.Number {
		t.Fatalf("receipt = %#v, %v", receipt, err)
	}
	if len(forge.attachments) != 1 || forge.attachments[0]["parent"] != parent.Number {
		t.Fatalf("grouping writes = %#v", forge.attachments)
	}
	if observed := forge.issue(missing.Number); observed == nil {
		t.Fatal("missing child disappeared")
	}
}

func TestRecoveryParentReportsKnownNumberWhenGroupingFails(t *testing.T) {
	forge := newRecoveryForge(t)
	parent := forge.addIssue(recoveryIssueRecord{Title: "Parent proposal", Body: "parent body"})
	child := forge.addIssue(recoveryIssueRecord{Title: "First slice", Body: "first body"})
	forge.faults.attach = http.StatusInternalServerError
	server := forge.server()
	defer server.Close()

	receipt, err := deliveryBackend(server).RecoverPresentation(context.Background(), ledger.RecoveryPresentation{
		Kind: "parent", Number: parent.Number, Title: parent.Title, Body: recoveryPointer("parent body"),
		Children: []ledger.ForgeAttachment{{Repository: "acme/widgets", Number: child.Number}},
	})
	if err != nil || receipt.Number != parent.Number || receipt.Status != recoveryPending {
		t.Fatalf("partial grouping = %#v, %v", receipt, err)
	}
}

func TestRecoveryPullAdoptsUniqueBranchAttachmentAfterLostCreate(t *testing.T) {
	forge := newRecoveryForge(t)
	pull := forge.addPull(deliveryPull{Number: 5, Body: "public delivery body", Draft: true, Head: "aaa"})
	server := forge.server()
	defer server.Close()

	receipt, err := deliveryBackend(server).RecoverPresentation(context.Background(), ledger.RecoveryPresentation{
		Kind: "pull", Branch: "widget", Head: pull.Head, Body: recoveryPointer("public delivery body"), MayHaveCreated: true,
	})
	if err != nil || receipt.Number != pull.Number || !recoverySatisfied(receipt) {
		t.Fatalf("receipt = %#v, %v", receipt, err)
	}
	if creates := forge.count(http.MethodPost, "/repos/acme/widgets/pulls"); creates != 0 {
		t.Fatalf("recovery created %d pull requests", creates)
	}
	if len(forge.pullPatches) != 0 || len(forge.mutations()) != 0 {
		t.Fatalf("satisfied presentation wrote: patches=%#v mutations=%v", forge.pullPatches, forge.mutations())
	}
}

func TestRecoveryPullRefusesAmbiguousBranchAttachments(t *testing.T) {
	forge := newRecoveryForge(t)
	forge.addPull(deliveryPull{Number: 5, Body: "one", Draft: true, Head: "aaa"})
	forge.addPull(deliveryPull{Number: 6, Body: "two", Draft: true, Head: "aaa"})
	server := forge.server()
	defer server.Close()

	_, err := deliveryBackend(server).RecoverPresentation(context.Background(), ledger.RecoveryPresentation{
		Kind: "pull", Branch: "widget", Head: "aaa", Body: recoveryPointer("prose"), MayHaveCreated: true,
	})
	if err == nil || !strings.Contains(err.Error(), "multiple pull requests") {
		t.Fatalf("ambiguous attachments were not refused: %v", err)
	}
	if creates := forge.count(http.MethodPost, "/repos/acme/widgets/pulls"); creates != 0 {
		t.Fatalf("ambiguous attachments created %d pull requests", creates)
	}
}

func TestRecoveryPullRefusesClosedBranchAttachmentWithoutReplacement(t *testing.T) {
	forge := newRecoveryForge(t)
	forge.addPull(deliveryPull{Number: 5, Body: "closed", Draft: true, Head: "aaa", State: "closed"})
	server := forge.server()
	defer server.Close()

	receipt, err := deliveryBackend(server).RecoverPresentation(context.Background(), ledger.RecoveryPresentation{
		Kind: "pull", Branch: "widget", Head: "aaa", Body: recoveryPointer("prose"), MayHaveCreated: true,
	})
	if err == nil || receipt.Number != 5 {
		t.Fatalf("closed attachment = %#v, %v", receipt, err)
	}
	if creates := forge.count(http.MethodPost, "/repos/acme/widgets/pulls"); creates != 0 {
		t.Fatalf("closed attachment was replaced by %d pull requests", creates)
	}
}

func TestRecoveryPullCatchesUpExpectedSourceLagBeforeApproval(t *testing.T) {
	forge := newRecoveryForge(t)
	forge.addPull(deliveryPull{Number: 5, Body: "earlier body", Draft: true, Head: "aaa"})
	server := forge.server()
	defer server.Close()

	prepared := ""
	receipt, err := deliveryBackend(server).RecoverPresentation(context.Background(), ledger.RecoveryPresentation{
		Kind: "pull", Number: 5, Branch: "widget", Head: "bbb", Body: recoveryPointer("approved body"), Approved: true,
		PrepareSource: func(observedHead string) error {
			prepared = observedHead
			forge.setPullHead(5, "bbb")
			return nil
		},
	})
	if err != nil || receipt.Number != 5 || !recoverySatisfied(receipt) {
		t.Fatalf("catch-up = %#v, %v", receipt, err)
	}
	if prepared != "aaa" {
		t.Fatalf("source preparation observed head %q", prepared)
	}
	if len(forge.pullPatches) != 1 || len(forge.mutations()) != 1 {
		t.Fatalf("catch-up writes: patches=%#v mutations=%v", forge.pullPatches, forge.mutations())
	}
	if pull := forge.pull(5); pull == nil || pull.Draft || pull.Head != "bbb" || pull.Body != "approved body" {
		t.Fatalf("caught-up pull = %#v", pull)
	}
}

func TestRecoveryPullRefusesUnestablishedHeadWithoutWrites(t *testing.T) {
	forge := newRecoveryForge(t)
	forge.addPull(deliveryPull{Number: 5, Body: "earlier body", Draft: true, Head: "aaa"})
	server := forge.server()
	defer server.Close()

	receipt, err := deliveryBackend(server).RecoverPresentation(context.Background(), ledger.RecoveryPresentation{
		Kind: "pull", Number: 5, Branch: "widget", Head: "bbb", Body: recoveryPointer("approved body"), Approved: true,
		PrepareSource: func(string) error { return nil },
	})
	if err != nil || receipt.Number != 5 || receipt.Status != recoveryPending || !strings.Contains(receipt.Detail, "not the intended bbb") {
		t.Fatalf("unestablished head = %#v, %v", receipt, err)
	}
	if len(forge.pullPatches) != 0 || len(forge.mutations()) != 0 {
		t.Fatalf("unestablished head wrote: patches=%#v mutations=%v", forge.pullPatches, forge.mutations())
	}
}

func TestRecoveryPullObservesSatisfiedPresentationWithoutDuplicate(t *testing.T) {
	forge := newRecoveryForge(t)
	forge.addPull(deliveryPull{Number: 5, Body: "approved body", Draft: false, Head: "aaa"})
	server := forge.server()
	defer server.Close()

	receipt, err := deliveryBackend(server).RecoverPresentation(context.Background(), ledger.RecoveryPresentation{
		Kind: "pull", Number: 5, Branch: "widget", Head: "aaa", Body: recoveryPointer("approved body"), Approved: true,
	})
	if err != nil || !recoverySatisfied(receipt) {
		t.Fatalf("satisfied presentation = %#v, %v", receipt, err)
	}
	if len(forge.pullPatches) != 0 || len(forge.mutations()) != 0 {
		t.Fatalf("satisfied presentation wrote: patches=%#v mutations=%v", forge.pullPatches, forge.mutations())
	}
}

func TestRecoveryPullCompletesOnlyMissingEffectsThenStops(t *testing.T) {
	forge := newRecoveryForge(t)
	forge.addPull(deliveryPull{Number: 5, Body: "old body", Draft: true, Head: "aaa"})
	server := forge.server()
	defer server.Close()
	backend := deliveryBackend(server)
	presentation := ledger.RecoveryPresentation{
		Kind: "pull", Number: 5, Branch: "widget", Head: "aaa", Body: recoveryPointer("approved body"), Approved: true,
	}
	first, err := backend.RecoverPresentation(context.Background(), presentation)
	if err != nil || !recoverySatisfied(first) {
		t.Fatalf("first recovery = %#v, %v", first, err)
	}
	if len(forge.pullPatches) != 1 || len(forge.mutations()) != 1 {
		t.Fatalf("first recovery writes: patches=%#v mutations=%v", forge.pullPatches, forge.mutations())
	}
	second, err := backend.RecoverPresentation(context.Background(), presentation)
	if err != nil || !recoverySatisfied(second) {
		t.Fatalf("repeat recovery = %#v, %v", second, err)
	}
	if len(forge.pullPatches) != 1 || len(forge.mutations()) != 1 {
		t.Fatalf("repeat recovery duplicated effects: patches=%#v mutations=%v", forge.pullPatches, forge.mutations())
	}
}

func TestRecoveryPullConfirmsLostReadyResponseWithoutDraftCorrection(t *testing.T) {
	forge := newRecoveryForge(t)
	forge.addPull(deliveryPull{Number: 5, Body: "approved body", Draft: true, Head: "aaa"})
	forge.faults.graphql = http.StatusInternalServerError
	forge.faults.graphqlStores = true
	server := forge.server()
	defer server.Close()

	receipt, err := deliveryBackend(server).RecoverPresentation(context.Background(), ledger.RecoveryPresentation{
		Kind: "pull", Number: 5, Branch: "widget", Head: "aaa", Body: recoveryPointer("approved body"), Approved: true,
	})
	if err != nil || !recoverySatisfied(receipt) {
		t.Fatalf("lost ready response = %#v, %v", receipt, err)
	}
	mutations := forge.mutations()
	if len(mutations) != 1 || !strings.Contains(mutations[0], "markPullRequestReadyForReview") {
		t.Fatalf("readiness mutations = %v", mutations)
	}
	if pull := forge.pull(5); pull == nil || pull.Draft {
		t.Fatalf("observed ready result was reversed: %#v", pull)
	}
}

func TestRecoveryPullRestoresDraftWhenSourceMovesAfterReady(t *testing.T) {
	forge := newRecoveryForge(t)
	forge.addPull(deliveryPull{Number: 5, Body: "approved body", Draft: true, Head: "aaa"})
	forge.headAfterReady = "zzz-unreviewed"
	server := forge.server()
	defer server.Close()

	receipt, err := deliveryBackend(server).RecoverPresentation(context.Background(), ledger.RecoveryPresentation{
		Kind: "pull", Number: 5, Branch: "widget", Head: "aaa", Body: recoveryPointer("approved body"), Approved: true,
	})
	if err != nil || receipt.Number != 5 || receipt.Status != recoveryPending || !strings.Contains(receipt.Detail, "intended aaa") {
		t.Fatalf("source movement = %#v, %v", receipt, err)
	}
	mutations := forge.mutations()
	if len(mutations) != 2 || !strings.Contains(mutations[1], "convertPullRequestToDraft") {
		t.Fatalf("readiness reconciliation = %v", mutations)
	}
	if pull := forge.pull(5); pull == nil || !pull.Draft || pull.Head != "zzz-unreviewed" || pull.Body != "approved body" {
		t.Fatalf("moved source presentation = %#v", pull)
	}
}

func TestRecoveryPullObserveOnlyMakesNoWrites(t *testing.T) {
	forge := newRecoveryForge(t)
	forge.addPull(deliveryPull{Number: 5, Body: "old body", Draft: true, Head: "aaa"})
	server := forge.server()
	defer server.Close()

	receipt, err := deliveryBackend(server).RecoverPresentation(context.Background(), ledger.RecoveryPresentation{
		Kind: "pull", Number: 5, Branch: "widget", Head: "bbb", Body: recoveryPointer("approved body"), Approved: true, ObserveOnly: true,
		PrepareSource: func(string) error { t.Fatal("observation only prepared source"); return nil },
	})
	if err != nil || receipt.Status != recoveryPending || receipt.Number != 5 {
		t.Fatalf("observation = %#v, %v", receipt, err)
	}
	if len(forge.pullPatches) != 0 || len(forge.mutations()) != 0 {
		t.Fatalf("observation wrote: patches=%#v mutations=%v", forge.pullPatches, forge.mutations())
	}
}

func TestRecoveryPullRefusesClosedKnownAttachment(t *testing.T) {
	forge := newRecoveryForge(t)
	forge.addPull(deliveryPull{Number: 5, Body: "approved body", Draft: false, Head: "aaa", State: "closed"})
	server := forge.server()
	defer server.Close()

	receipt, err := deliveryBackend(server).RecoverPresentation(context.Background(), ledger.RecoveryPresentation{
		Kind: "pull", Number: 5, Branch: "widget", Head: "aaa", Body: recoveryPointer("approved body"), Approved: true,
	})
	if err == nil || receipt.Number != 5 {
		t.Fatalf("closed attachment = %#v, %v", receipt, err)
	}
	if len(forge.pullPatches) != 0 || len(forge.mutations()) != 0 {
		t.Fatalf("closed attachment wrote: patches=%#v mutations=%v", forge.pullPatches, forge.mutations())
	}
}

func TestRecoveryPullCreatesFreshDraftWithoutUncertainAttempt(t *testing.T) {
	forge := newRecoveryForge(t)
	server := forge.server()
	defer server.Close()

	prepared := "unset"
	receipt, err := deliveryBackend(server).RecoverPresentation(context.Background(), ledger.RecoveryPresentation{
		Kind: "pull", Branch: "widget", Head: "aaa", Title: "widget delivery", Body: recoveryPointer("delivery body"),
		PrepareSource: func(observedHead string) error { prepared = observedHead; return nil },
	})
	if err != nil || !recoverySatisfied(receipt) {
		t.Fatalf("fresh pull = %#v, %v", receipt, err)
	}
	if creates := forge.count(http.MethodPost, "/repos/acme/widgets/pulls"); creates != 1 {
		t.Fatalf("fresh pull posted %d creations", creates)
	}
	if prepared != "" {
		t.Fatalf("fresh pull prepared source with observed head %q", prepared)
	}
	if pull := forge.pull(receipt.Number); pull == nil || !pull.Draft || pull.Body != "delivery body" || pull.Title != "widget delivery" {
		t.Fatalf("created pull = %#v", pull)
	}
}

func TestRecoveryPullGuardBlocksBodyWrite(t *testing.T) {
	forge := newRecoveryForge(t)
	forge.addPull(deliveryPull{Number: 5, Body: "old body", Draft: true, Head: "aaa"})
	server := forge.server()
	defer server.Close()

	calls := 0
	_, err := deliveryBackend(server).RecoverPresentation(context.Background(), ledger.RecoveryPresentation{
		Kind: "pull", Number: 5, Branch: "widget", Head: "aaa", Body: recoveryPointer("approved body"),
		Guard: func() error {
			calls++
			if calls > 1 {
				return fmt.Errorf("the selected view was superseded")
			}
			return nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "superseded") {
		t.Fatalf("guard failure was ignored: %v", err)
	}
	if len(forge.pullPatches) != 0 {
		t.Fatalf("guarded recovery wrote a body: %#v", forge.pullPatches)
	}
}

const recoveryReviewedPatch = "@@ -1,4 +1,5 @@\n context one\n+added line\n context two\n context three\n context four\n"

func recoveryReviewedFile(name string) pullFile {
	return pullFile{Filename: name, Status: "modified", Patch: recoveryReviewedPatch}
}

func TestRecoveryFindingsPublishOnlyExplicitValidSelection(t *testing.T) {
	forge := newRecoveryForge(t)
	forge.addPull(deliveryPull{Number: 5, Body: "delivery body", Draft: false, Head: "aaa"})
	forge.files = []pullFile{recoveryReviewedFile("src/app.go")}
	server := forge.server()
	defer server.Close()

	receipt, err := deliveryBackend(server).RecoverPresentation(context.Background(), ledger.RecoveryPresentation{
		Kind: "pull", Number: 5, Branch: "widget", Head: "aaa", Reviewed: "aaa", Body: recoveryPointer("delivery body"), Approved: true,
		Findings: []ledger.SelectedFinding{
			{ID: "W2", Body: "public actionable finding", Commit: "aaa", Path: "src/app.go", Line: 2, Side: "RIGHT"},
			{ID: "W3", Body: "another finding", Commit: "aaa", Path: "src/app.go", Line: 99, Side: "RIGHT"},
		},
	})
	if err != nil || !recoverySatisfied(receipt) {
		t.Fatalf("findings recovery = %#v, %v", receipt, err)
	}
	if len(receipt.Findings) != 2 || receipt.Findings[0].Status != recoveryFindingSatisfied || receipt.Findings[1].Status != recoveryFindingUnresolved {
		t.Fatalf("finding receipts = %#v", receipt.Findings)
	}
	if len(forge.commentWrites) != 1 {
		t.Fatalf("inline writes = %#v", forge.commentWrites)
	}
}

func TestRecoveryFindingsDeduplicateLostResponse(t *testing.T) {
	forge := newRecoveryForge(t)
	forge.addPull(deliveryPull{Number: 5, Body: "delivery body", Draft: false, Head: "aaa"})
	forge.files = []pullFile{recoveryReviewedFile("src/app.go")}
	forge.faults.createComment = http.StatusInternalServerError
	forge.faults.createCommentStores = true
	server := forge.server()
	defer server.Close()

	receipt, err := deliveryBackend(server).RecoverPresentation(context.Background(), ledger.RecoveryPresentation{
		Kind: "pull", Number: 5, Branch: "widget", Head: "aaa", Reviewed: "aaa", Body: recoveryPointer("delivery body"), Approved: true,
		Findings: []ledger.SelectedFinding{{ID: "W2", Body: "public actionable finding", Commit: "aaa", Path: "src/app.go", Line: 2, Side: "RIGHT"}},
	})
	if err != nil || len(receipt.Findings) != 1 || receipt.Findings[0].Status != recoveryFindingSatisfied {
		t.Fatalf("lost inline response = %#v, %v", receipt, err)
	}
	if len(forge.commentWrites) != 1 || len(forge.comments) != 1 {
		t.Fatalf("inline writes = %#v, comments = %d", forge.commentWrites, len(forge.comments))
	}
}

func TestRecoveryFindingsRefuseMovedAnchor(t *testing.T) {
	forge := newRecoveryForge(t)
	forge.addPull(deliveryPull{Number: 5, Body: "delivery body", Draft: false, Head: "aaa"})
	forge.files = []pullFile{recoveryReviewedFile("src/app.go")}
	server := forge.server()
	defer server.Close()

	receipt, err := deliveryBackend(server).RecoverPresentation(context.Background(), ledger.RecoveryPresentation{
		Kind: "pull", Number: 5, Branch: "widget", Head: "aaa", Reviewed: "aaa", Body: recoveryPointer("delivery body"), Approved: true,
		Findings: []ledger.SelectedFinding{{ID: "W2", Body: "finding", Commit: "aaa", Path: "src/app.go", Line: 40, Side: "RIGHT"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(receipt.Findings) != 1 || receipt.Findings[0].Status != recoveryFindingUnresolved || !strings.Contains(receipt.Findings[0].Detail, "not an anchorable") {
		t.Fatalf("moved anchor = %#v", receipt.Findings)
	}
	if len(forge.commentWrites) != 0 {
		t.Fatalf("moved anchor wrote %#v", forge.commentWrites)
	}
}

func TestRecoveryFindingsRefuseStaleReviewedRevision(t *testing.T) {
	forge := newRecoveryForge(t)
	forge.addPull(deliveryPull{Number: 5, Body: "delivery body", Draft: false, Head: "aaa"})
	forge.files = []pullFile{recoveryReviewedFile("src/app.go")}
	server := forge.server()
	defer server.Close()

	receipt, err := deliveryBackend(server).RecoverPresentation(context.Background(), ledger.RecoveryPresentation{
		Kind: "pull", Number: 5, Branch: "widget", Head: "aaa", Reviewed: "bbb", Body: recoveryPointer("delivery body"), Approved: true,
		Findings: []ledger.SelectedFinding{{ID: "W2", Body: "finding", Commit: "bbb", Path: "src/app.go", Line: 2, Side: "RIGHT"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(receipt.Findings) != 1 || receipt.Findings[0].Status != recoveryFindingUnresolved {
		t.Fatalf("stale revision = %#v", receipt.Findings)
	}
	if !recoverySatisfied(receipt) {
		t.Fatalf("independent body publication was not reported: %#v", receipt)
	}
	if len(forge.commentWrites) != 0 {
		t.Fatalf("stale revision wrote %#v", forge.commentWrites)
	}
}

func TestRecoveryFindingsIndependentFromFailedBody(t *testing.T) {
	forge := newRecoveryForge(t)
	forge.addPull(deliveryPull{Number: 5, Body: "old body", Draft: false, Head: "aaa"})
	forge.files = []pullFile{recoveryReviewedFile("src/app.go")}
	forge.faults.pullPatch = http.StatusUnprocessableEntity
	server := forge.server()
	defer server.Close()

	receipt, err := deliveryBackend(server).RecoverPresentation(context.Background(), ledger.RecoveryPresentation{
		Kind: "pull", Number: 5, Branch: "widget", Head: "aaa", Reviewed: "aaa", Body: recoveryPointer("approved body"), Approved: true,
		Findings: []ledger.SelectedFinding{{ID: "W2", Body: "public actionable finding", Commit: "aaa", Path: "src/app.go", Line: 2, Side: "RIGHT"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != recoveryPending {
		t.Fatalf("a rejected body update was not reported as pending: %#v", receipt)
	}
	if len(receipt.Findings) != 1 || receipt.Findings[0].Status != recoveryFindingSatisfied {
		t.Fatalf("finding receipt depended on body publication: %#v", receipt.Findings)
	}
	if len(forge.commentWrites) != 1 {
		t.Fatalf("inline writes = %#v", forge.commentWrites)
	}
}

func TestRecoveryFindingsPaginateReviewedDiff(t *testing.T) {
	forge := newRecoveryForge(t)
	forge.addPull(deliveryPull{Number: 5, Body: "delivery body", Draft: false, Head: "aaa"})
	for index := 0; index < 100; index++ {
		forge.files = append(forge.files, recoveryReviewedFile(fmt.Sprintf("src/filler-%03d.go", index)))
	}
	forge.files = append(forge.files, recoveryReviewedFile("src/target.go"))
	forge.filePageSize = 100
	server := forge.server()
	defer server.Close()

	receipt, err := deliveryBackend(server).RecoverPresentation(context.Background(), ledger.RecoveryPresentation{
		Kind: "pull", Number: 5, Branch: "widget", Head: "aaa", Reviewed: "aaa", Body: recoveryPointer("delivery body"), Approved: true,
		Findings: []ledger.SelectedFinding{{ID: "W2", Body: "paged finding", Commit: "aaa", Path: "src/target.go", Line: 2, Side: "RIGHT"}},
	})
	if err != nil || len(receipt.Findings) != 1 || receipt.Findings[0].Status != recoveryFindingSatisfied {
		t.Fatalf("paged finding = %#v, %v", receipt.Findings, err)
	}
	if lists := forge.count(http.MethodGet, "/repos/acme/widgets/pulls/5/files"); lists != 2 {
		t.Fatalf("reviewed diff listing stopped after %d pages", lists)
	}
}

func TestRecoveryPublicationNeverAppendsPrivateEvidence(t *testing.T) {
	forge := newRecoveryForge(t)
	server := forge.server()
	defer server.Close()

	private := "delivery body\n\n<!-- private operational marker -->\n"
	receipt, err := deliveryBackend(server).RecoverPresentation(context.Background(), ledger.RecoveryPresentation{
		Kind: "issue", Title: "Add order cancellation", Body: recoveryPointer(private),
	})
	if err != nil || !recoverySatisfied(receipt) {
		t.Fatalf("receipt = %#v, %v", receipt, err)
	}
	if len(forge.issueCreates) != 1 {
		t.Fatalf("issue creates = %#v", forge.issueCreates)
	}
	payload := forge.issueCreates[0]
	if len(payload) != 2 || payload["title"] != "Add order cancellation" || payload["body"] != private {
		t.Fatalf("create payload = %#v", payload)
	}
	for _, request := range forge.requestLog() {
		for _, forbidden := range []string{"/labels", "/comments", "/reviews", "/timeline"} {
			if strings.Contains(request, forbidden) {
				t.Fatalf("publication touched a private or authoritative surface: %s", request)
			}
		}
	}
}
