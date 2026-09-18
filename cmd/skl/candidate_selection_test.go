package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/setup"
)

// candidateForge is a controlled GitHub observation surface for the
// candidate-first selection scenarios. It records every request so tests can
// assert which records were read and which were left untouched.
type candidateForge struct {
	mu       sync.Mutex
	requests []string

	pulls  map[int]map[string]any
	issues map[int]map[string]any

	reworkPages [][]int
	reviewPages [][]int
	readyPages  [][]int

	dependencies map[int][]map[string]any
	evidence     map[int][]map[string]any
	owners       map[int][]int
	comments     map[string][]map[string]any
	timeline     map[int][]map[string]any
	fail         map[string]int
	errorAfter   map[string]bool
	heads        map[string]string

	onPullRead   func(number int)
	onLabelWrite func(number int)

	graphqlError string
	nextPR       int
}

func newCandidateForge() *candidateForge {
	return &candidateForge{
		pulls: make(map[int]map[string]any), issues: make(map[int]map[string]any),
		dependencies: make(map[int][]map[string]any), evidence: make(map[int][]map[string]any),
		owners: make(map[int][]int), comments: make(map[string][]map[string]any),
		timeline: make(map[int][]map[string]any), fail: make(map[string]int), errorAfter: make(map[string]bool),
		heads: make(map[string]string), nextPR: 11,
	}
}

func (f *candidateForge) addPull(number int, created, body, ref, sha string, labels ...string) {
	objects := make([]map[string]string, 0, len(labels))
	for _, label := range labels {
		objects = append(objects, map[string]string{"name": label})
	}
	f.pulls[number] = map[string]any{"number": number, "node_id": "PR_" + strconv.Itoa(number), "state": "open", "created_at": created, "body": body, "labels": objects, "head": map[string]any{"ref": ref, "sha": sha, "repo": map[string]string{"full_name": "acme/widgets"}}, "base": map[string]string{"ref": "main"}}
}

func (f *candidateForge) addIssue(number int, created, body string, labels ...string) {
	objects := make([]map[string]string, 0, len(labels))
	for _, label := range labels {
		objects = append(objects, map[string]string{"name": label})
	}
	f.issues[number] = map[string]any{"number": number, "state": "open", "created_at": created, "body": body, "labels": objects}
}

func (f *candidateForge) record(r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.Method+" "+r.URL.RequestURI())
}

func (f *candidateForge) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.requests)
}

func (f *candidateForge) matching(match func(string) bool) int {
	count := 0
	for _, request := range f.seen() {
		if match(request) {
			count++
		}
	}
	return count
}

func (f *candidateForge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/repos/acme/widgets")
	status := f.fail[r.Method+" "+path+"?"+r.URL.RawQuery]
	if status == 0 {
		status = f.fail[r.Method+" "+path]
	}
	if r.Method == http.MethodPost && path == "/graphql" && status == 0 {
		f.serveGraphQL(w, r)
		return
	}
	f.record(r)
	if status != 0 {
		http.Error(w, "injected failure", status)
		return
	}
	if r.Method != http.MethodGet {
		f.serveMutation(w, r, path)
		return
	}
	write := func(value any) { _ = json.NewEncoder(w).Encode(value) }
	segments := strings.Split(strings.Trim(path, "/"), "/")
	switch {
	case path == "/issues" && r.URL.Query().Get("labels") == "ready":
		write(f.readyQueue(r))
	case path == "/issues":
		values := []any{}
		for _, issue := range f.issues {
			values = append(values, issue)
		}
		for _, pull := range f.pulls {
			entry := make(map[string]any, len(pull))
			for key, value := range pull {
				entry[key] = value
			}
			entry["pull_request"] = map[string]string{"url": "pull"}
			values = append(values, entry)
		}
		write(values)
	case path == "/pulls":
		head := r.URL.Query().Get("head")
		values := []any{}
		for _, pull := range f.pulls {
			ref, _ := pull["head"].(map[string]any)["ref"].(string)
			if head != "" && head != "acme:"+ref {
				continue
			}
			values = append(values, pull)
		}
		write(values)
	case len(segments) == 2 && segments[0] == "pulls":
		number, _ := strconv.Atoi(segments[1])
		if f.onPullRead != nil {
			f.onPullRead(number)
		}
		if pull, ok := f.pulls[number]; ok {
			write(pull)
			return
		}
		http.NotFound(w, r)
	case len(segments) == 2 && segments[0] == "issues":
		number, _ := strconv.Atoi(segments[1])
		if issue, ok := f.issues[number]; ok {
			write(issue)
			return
		}
		if pull, ok := f.pulls[number]; ok {
			write(pull)
			return
		}
		http.NotFound(w, r)
	case len(segments) == 4 && segments[0] == "git" && segments[1] == "ref" && segments[2] == "heads":
		if sha, ok := f.heads[segments[3]]; ok {
			write(map[string]any{"object": map[string]string{"sha": sha}})
			return
		}
		http.NotFound(w, r)
	case len(segments) == 3 && segments[0] == "issues" && segments[2] == "comments":
		number, _ := strconv.Atoi(segments[1])
		write(pageSlice(f.comments[fmt.Sprintf("/issues/%d/comments", number)], r))
	case len(segments) == 3 && segments[0] == "pulls" && segments[2] == "comments":
		number, _ := strconv.Atoi(segments[1])
		write(pageSlice(f.comments[fmt.Sprintf("/pulls/%d/comments", number)], r))
	case len(segments) == 3 && segments[0] == "pulls" && segments[2] == "reviews":
		number, _ := strconv.Atoi(segments[1])
		write(pageSlice(f.comments[fmt.Sprintf("/pulls/%d/reviews", number)], r))
	case len(segments) == 3 && segments[0] == "issues" && segments[2] == "timeline":
		number, _ := strconv.Atoi(segments[1])
		write(f.timeline[number])
	case len(segments) == 4 && segments[0] == "issues" && segments[2] == "dependencies" && segments[3] == "blocked_by":
		number, _ := strconv.Atoi(segments[1])
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		all := f.dependencies[number]
		start := (page - 1) * 100
		if start >= len(all) {
			write([]any{})
			return
		}
		end := min(start+100, len(all))
		write(all[start:end])
	default:
		http.NotFound(w, r)
	}
}

func pageSlice(values []map[string]any, r *http.Request) []map[string]any {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	start := (page - 1) * 100
	if start >= len(values) {
		return nil
	}
	return values[start:min(start+100, len(values))]
}

func (f *candidateForge) readyQueue(r *http.Request) []any {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	if page > len(f.readyPages) {
		return nil
	}
	values := []any{}
	for _, number := range f.readyPages[page-1] {
		values = append(values, f.issues[number])
	}
	return values
}

func (f *candidateForge) serveMutation(w http.ResponseWriter, r *http.Request, path string) {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	switch {
	case path == "/pulls" && r.Method == http.MethodPost:
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		body, _ := payload["body"].(string)
		number := f.nextPR
		f.addPull(number, "2026-01-01T00:00:00Z", body, "widget", f.heads["widget"])
		f.pulls[number]["labels"] = appendLabels(nil, "review")
		_ = json.NewEncoder(w).Encode(f.pulls[number])
	case len(segments) == 2 && segments[0] == "pulls" && r.Method == http.MethodPatch:
		number, _ := strconv.Atoi(segments[1])
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		for key, value := range payload {
			if key == "base" {
				value = map[string]string{"ref": value.(string)}
			}
			f.pulls[number][key] = value
		}
		_ = json.NewEncoder(w).Encode(f.pulls[number])
	case len(segments) == 3 && segments[0] == "issues" && segments[2] == "labels" && r.Method == http.MethodPost:
		var payload struct {
			Labels []string `json:"labels"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		number, _ := strconv.Atoi(segments[1])
		f.addLabel(number, payload.Labels...)
		if f.onLabelWrite != nil {
			f.onLabelWrite(number)
		}
		if f.errorAfter[r.Method+" "+"/issues/"+segments[1]+"/labels"] {
			http.Error(w, "accepted mutation response lost", http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode([]any{})
	case len(segments) == 4 && segments[0] == "issues" && segments[2] == "labels" && r.Method == http.MethodDelete:
		number, _ := strconv.Atoi(segments[1])
		f.removeLabel(number, segments[3])
		_ = json.NewEncoder(w).Encode([]any{})
	default:
		http.NotFound(w, r)
	}
}

func (f *candidateForge) addLabel(number int, labels ...string) {
	target := f.pulls[number]
	if target == nil {
		target = f.issues[number]
	}
	if target == nil {
		return
	}
	for _, label := range labels {
		f.timeline[number] = append(f.timeline[number], map[string]any{"event": "labeled", "created_at": "2026-01-01T00:00:01Z", "label": map[string]string{"name": label}})
	}
	target["labels"] = appendLabels(target["labels"], labels...)
}

func (f *candidateForge) removeLabel(number int, label string) {
	target := f.pulls[number]
	if target == nil {
		target = f.issues[number]
	}
	if target == nil {
		return
	}
	f.timeline[number] = append(f.timeline[number], map[string]any{"event": "unlabeled", "created_at": "2026-01-01T00:00:02Z", "label": map[string]string{"name": label}})
	target["labels"] = slices.DeleteFunc(target["labels"].([]map[string]string), func(current map[string]string) bool { return current["name"] == label })
}

func appendLabels(existing any, labels ...string) []map[string]string {
	values, _ := existing.([]map[string]string)
	for _, label := range labels {
		present := false
		for _, value := range values {
			present = present || value["name"] == label
		}
		if !present {
			values = append(values, map[string]string{"name": label})
		}
	}
	return values
}

func hasLabel(record map[string]any, wanted string) bool {
	values, _ := record["labels"].([]map[string]string)
	for _, value := range values {
		if value["name"] == wanted {
			return true
		}
	}
	return false
}

func (f *candidateForge) serveGraphQL(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	_ = json.NewDecoder(r.Body).Decode(&payload)
	purpose := "graphql:other"
	switch {
	case strings.Contains(payload.Query, "pullRequests("):
		purpose = "graphql:queue"
	case strings.Contains(payload.Query, "closedByPullRequestsReferences"):
		purpose = "graphql:owners"
	case strings.Contains(payload.Query, "lastEditedAt"):
		purpose = "graphql:body"
	case strings.Contains(payload.Query, "mutation"):
		purpose = "graphql:mutation"
	}
	f.mu.Lock()
	f.requests = append(f.requests, purpose)
	f.mu.Unlock()
	write := func(value any) { _ = json.NewEncoder(w).Encode(value) }
	response := func(data map[string]any) { write(map[string]any{"data": data}) }
	switch {
	case strings.Contains(payload.Query, "pullRequests("):
		label := ""
		if raw, ok := payload.Variables["label"].([]any); ok && len(raw) == 1 {
			label, _ = raw[0].(string)
		}
		after, _ := payload.Variables["after"].(string)
		pages := f.reviewPages
		if label == "rework" {
			pages = f.reworkPages
		}
		page := 0
		if after != "" {
			page, _ = strconv.Atoi(after)
		}
		nodes := []any{}
		if page < len(pages) {
			for _, number := range pages[page] {
				pull := f.pulls[number]
				if pull == nil {
					continue
				}
				head := pull["head"].(map[string]any)
				nodes = append(nodes, map[string]any{"number": number, "createdAt": pull["created_at"], "headRefName": head["ref"], "headRefOid": head["sha"], "isDraft": false, "labels": map[string]any{"nodes": pull["labels"]}})
			}
		}
		next := ""
		if page+1 < len(pages) {
			next = strconv.Itoa(page + 1)
		}
		data := map[string]any{"repository": map[string]any{"pullRequests": map[string]any{"nodes": nodes, "pageInfo": map[string]any{"hasNextPage": next != "", "endCursor": next}}}}
		if f.graphqlError != "" {
			write(map[string]any{"data": data, "errors": []map[string]any{{"message": f.graphqlError}}})
			return
		}
		response(data)
	case strings.Contains(payload.Query, "closedByPullRequestsReferences"):
		issueNumber := 0
		if raw, ok := payload.Variables["number"]; ok {
			switch value := raw.(type) {
			case float64:
				issueNumber = int(value)
			case string:
				issueNumber, _ = strconv.Atoi(value)
			}
		}
		nodes := []any{}
		if strings.Contains(payload.Query, "includeClosedPrs:true") {
			for _, node := range f.evidence[issueNumber] {
				node["repository"] = map[string]string{"nameWithOwner": "acme/widgets"}
				nodes = append(nodes, node)
			}
		} else {
			owners := slices.Clone(f.owners[issueNumber])
			for number, pull := range f.pulls {
				body, _ := pull["body"].(string)
				if strings.Contains(body, "Closes #"+strconv.Itoa(issueNumber)) && !slices.Contains(owners, number) {
					owners = append(owners, number)
				}
			}
			for _, number := range owners {
				nodes = append(nodes, map[string]any{"number": number, "merged": false, "mergedAt": "", "state": "OPEN", "repository": map[string]string{"nameWithOwner": "acme/widgets"}})
			}
		}
		response(map[string]any{"repository": map[string]any{"issue": map[string]any{"state": "OPEN", "closedByPullRequestsReferences": map[string]any{"nodes": nodes, "pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""}}}}})
	case strings.Contains(payload.Query, "lastEditedAt"):
		id, _ := payload.Variables["id"].(string)
		number, _ := strconv.Atoi(strings.TrimPrefix(id, "PR_"))
		pull := f.pulls[number]
		body, _ := pull["body"].(string)
		response(map[string]any{"node": map[string]any{"body": body, "createdAt": "2026-01-01T00:00:00Z", "lastEditedAt": "", "id": id}})
	case strings.Contains(payload.Query, "mutation"):
		response(map[string]any{})
	default:
		http.Error(w, "unexpected query", http.StatusBadRequest)
	}
}

func selectionRepository(t *testing.T) string {
	t.Helper()
	root := proposalRepository(t)
	runGit(t, root, "remote", "set-url", "origin", "git@github.com:acme/widgets.git")
	return root
}

func selectionRun(t *testing.T, root string, forge *candidateForge, args ...string) (setup.ImplementationOutput, error) {
	t.Helper()
	server := httptest.NewServer(forge)
	t.Cleanup(server.Close)
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) {
		backend := setup.NewGitHubBackend(server.URL, "token", server.Client())
		backend.BindRepository(github.RepositoryID{Owner: "acme", Name: "widgets"})
		return backend, nil
	}, bytes.NewReader(nil), &output, &output)
	command := append([]string{"skl"}, args...)
	command = append(command, "--repo", root)
	err := app.Run(command)
	if err != nil {
		return setup.ImplementationOutput{}, err
	}
	var result setup.ImplementationOutput
	if decode := json.Unmarshal(output.Bytes(), &result); decode != nil {
		t.Fatalf("%v: %s: %v", command, &output, decode)
	}
	if result.Packet != nil {
		t.Cleanup(func() {
			if result.Packet.Facts.Implementation != nil {
				_ = os.RemoveAll(result.Packet.Facts.Implementation.ResultDirectory)
			}
			if result.Packet.Facts.Watchdog != nil {
				_ = os.RemoveAll(result.Packet.Facts.Watchdog.ResultDirectory)
			}
		})
	}
	return result, nil
}

// assertNoProjectObjectReads fails when any recorded git subprocess inspected
// commits, trees, or blobs instead of only refs and configuration.
func assertNoProjectObjectReads(t *testing.T, traceFile string) {
	t.Helper()
	contents, err := os.ReadFile(traceFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(contents), "\n") {
		var event struct {
			Event string   `json:"event"`
			Argv  []string `json:"argv"`
		}
		if line == "" || json.Unmarshal([]byte(line), &event) != nil || event.Event != "start" {
			continue
		}
		command := strings.Join(event.Argv, " ")
		if strings.Contains(command, "rev-parse --show-toplevel") || strings.Contains(command, "rev-parse --git-dir") {
			// Repository layout discovery, not project object inspection.
			continue
		}
		for _, forbidden := range []string{"cat-file", "rev-list", "rev-parse", " log ", "merge-base", " show "} {
			if strings.Contains(command, forbidden) {
				t.Fatalf("startup inspected project objects: %s", command)
			}
		}
	}
}

func selectionStatusRun(t *testing.T, root string, forge *candidateForge) setup.StatusOutput {
	t.Helper()
	server := httptest.NewServer(forge)
	t.Cleanup(server.Close)
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) {
		backend := setup.NewGitHubBackend(server.URL, "token", server.Client())
		backend.BindRepository(github.RepositoryID{Owner: "acme", Name: "widgets"})
		return backend, nil
	}, bytes.NewReader(nil), &output, &output)
	if err := app.Run([]string{"skl", "status", "--repo", root}); err != nil {
		t.Fatal(err)
	}
	var result setup.StatusOutput
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestB1ReworkSelectionUsesSubmissionAgeAndPrecedesReady(t *testing.T) {
	root := selectionRepository(t)
	forge := newCandidateForge()
	// Rework PR ages have the opposite ordering of their source issue ages.
	forge.addPull(30, "2021-01-01T00:00:00Z", "older review\n\nCloses #3\n", "slice-three", strings.Repeat("a", 40), "rework")
	forge.addPull(31, "2022-01-01T00:00:00Z", "newer review\n\nCloses #2\n", "slice-two", strings.Repeat("b", 40), "rework")
	forge.addIssue(3, "2030-01-01T00:00:00Z", "Branch: `slice-three`\n")
	forge.addIssue(2, "2001-01-01T00:00:00Z", "Branch: `slice-two`\n")
	forge.addIssue(1, "2000-01-01T00:00:00Z", "Branch: `slice-one`\n", "ready")
	forge.reworkPages = [][]int{{30, 31}}
	forge.readyPages = [][]int{{1}}
	forge.owners[3] = []int{30}
	forge.owners[2] = []int{31}

	got, err := selectionRun(t, root, forge, "implement", "next")
	if err != nil || got.Status != "work_available" || got.Item == nil || got.Item.Number != 3 {
		t.Fatalf("selection = %#v, %v", got, err)
	}
	if got.Packet.Facts.Implementation.Branch != "slice-three" {
		t.Fatalf("packet branch = %#v", got.Packet.Facts.Implementation)
	}
	for _, forbidden := range []string{"/issues/1/dependencies/blocked_by", "labels=ready", "/issues/2/dependencies/blocked_by"} {
		if forge.matching(func(request string) bool { return strings.Contains(request, forbidden) }) != 0 {
			t.Fatalf("eligible rework still read Ready context: %v", forge.seen())
		}
	}
}

func TestB2QueueLocalAgeAndIdentityDetermineSelection(t *testing.T) {
	t.Run("rework", func(t *testing.T) {
		root := selectionRepository(t)
		forge := newCandidateForge()
		forge.addPull(20, "2020-01-01T00:00:00Z", "claimed\n\nCloses #8\n", "slice-eight", strings.Repeat("a", 40), "rework", "wip")
		forge.addPull(23, "2021-01-01T00:00:00Z", "later number\n\nCloses #9\n", "slice-nine", strings.Repeat("b", 40), "rework")
		forge.addPull(22, "2021-01-01T00:00:00Z", "tie\n\nCloses #10\n", "slice-ten", strings.Repeat("c", 40), "rework")
		forge.issues[8] = map[string]any{"number": 8, "state": "open", "body": "Branch: `slice-eight`\n"}
		forge.issues[9] = map[string]any{"number": 9, "state": "open", "body": "Branch: `slice-nine`\n"}
		forge.issues[10] = map[string]any{"number": 10, "state": "open", "body": "Branch: `slice-ten`\n"}
		forge.owners[8], forge.owners[9], forge.owners[10] = []int{20}, []int{23}, []int{22}
		forge.reworkPages = [][]int{{20, 23, 22}}

		got, err := selectionRun(t, root, forge, "implement", "next")
		if err != nil || got.Status != "work_available" || got.Item == nil || got.Item.Number != 10 {
			t.Fatalf("selection = %#v, %v", got, err)
		}
	})
	t.Run("ready", func(t *testing.T) {
		root := selectionRepository(t)
		forge := newCandidateForge()
		forge.addIssue(20, "2020-01-01T00:00:00Z", "Branch: `slice-twenty`\n", "ready", "wip")
		forge.addIssue(23, "2021-01-01T00:00:00Z", "Branch: `slice-twenty-three`\n", "ready")
		forge.addIssue(22, "2021-01-01T00:00:00Z", "Branch: `slice-twenty-two`\n", "ready")
		forge.readyPages = [][]int{{20, 23, 22}}

		got, err := selectionRun(t, root, forge, "implement", "next")
		if err != nil || got.Status != "work_available" || got.Item == nil || got.Item.Number != 22 {
			t.Fatalf("selection = %#v, %v", got, err)
		}
	})
	t.Run("review", func(t *testing.T) {
		root := selectionRepository(t)
		forge := newCandidateForge()
		forge.addPull(20, "2020-01-01T00:00:00Z", "claimed\n\nCloses #8\n", "slice-eight", strings.Repeat("a", 40), "review", "wip")
		forge.addPull(23, "2021-01-01T00:00:00Z", "later\n\nCloses #9\n", "slice-nine", strings.Repeat("b", 40), "review")
		forge.addPull(22, "2021-01-01T00:00:00Z", "tie\n\nCloses #10\n", "slice-ten", strings.Repeat("c", 40), "review")
		forge.issues[8] = map[string]any{"number": 8, "state": "open", "body": "Branch: `slice-eight`\n"}
		forge.issues[9] = map[string]any{"number": 9, "state": "open", "body": "Branch: `slice-nine`\n"}
		forge.issues[10] = map[string]any{"number": 10, "state": "open", "body": "Branch: `slice-ten`\n"}
		forge.owners[8], forge.owners[9], forge.owners[10] = []int{20}, []int{23}, []int{22}
		forge.reviewPages = [][]int{{20, 23, 22}}
		runGit(t, root, "worktree", "add", root+"/.worktrees/slice-ten", "-b", "slice-ten", "main")

		got, err := selectionRun(t, root, forge, "watchdog", "next")
		if err != nil || got.Status != "work_available" || got.Item == nil || got.Item.Number != 10 {
			t.Fatalf("selection = %#v, %v", got, err)
		}
	})
}

func TestB3NoWorkSkipsUnrelatedDiscovery(t *testing.T) {
	for _, lane := range []string{"implement", "watchdog"} {
		t.Run(lane, func(t *testing.T) {
			root := selectionRepository(t)
			forge := newCandidateForge()
			// Unrelated open records, historical work items, and a discussion
			// endpoint that fails if it is requested.
			for number := 100; number < 130; number++ {
				forge.addIssue(number, "2019-01-01T00:00:00Z", "unrelated\n", "needs-human")
			}
			forge.addPull(200, "2018-01-01T00:00:00Z", "unrelated discussion\n", "unrelated", strings.Repeat("d", 40), "done")
			forge.comments["/issues/200/comments"] = []map[string]any{{"body": strings.Repeat("discussion\n", 500)}}
			forge.fail["GET /issues/200/comments"] = http.StatusInternalServerError
			if lane == "implement" {
				forge.reworkPages = [][]int{{}}
				forge.readyPages = [][]int{{}}
			} else {
				forge.reviewPages = [][]int{{}}
			}

			got, err := selectionRun(t, root, forge, lane, "next")
			if err != nil || got.Status != "no_work" {
				t.Fatalf("no work = %#v, %v", got, err)
			}
			for _, request := range forge.seen() {
				if strings.Contains(request, "/issues/200/comments") || strings.Contains(request, "state=all") {
					t.Fatalf("unrelated discovery: %v", forge.seen())
				}
			}
			if forge.matching(func(request string) bool {
				return strings.Contains(request, "/labels") || strings.HasPrefix(request, "DELETE ") || strings.HasPrefix(request, "PATCH ")
			}) != 0 {
				t.Fatalf("no_work claimed something: %v", forge.seen())
			}
		})
	}
}

func TestB3ClaimedQueuesReturnNoWorkWithoutDiscussion(t *testing.T) {
	root := selectionRepository(t)
	forge := newCandidateForge()
	forge.addPull(30, "2020-01-01T00:00:00Z", "claimed review\n\nCloses #7\n", "slice-seven", strings.Repeat("a", 40), "rework", "wip")
	forge.addIssue(7, "2019-01-01T00:00:00Z", "Branch: `slice-seven`\n", "ready", "wip")
	forge.comments["/issues/7/comments"] = []map[string]any{{"body": "unrelated discussion"}}
	forge.reworkPages = [][]int{{30}}
	forge.readyPages = [][]int{{7}}

	got, err := selectionRun(t, root, forge, "implement", "next")
	if err != nil || got.Status != "no_work" {
		t.Fatalf("claimed queues = %#v, %v", got, err)
	}
	for _, forbidden := range []string{"/issues/7/comments", "/pulls/30/comments", "/pulls/30/reviews", "/issues/7/dependencies/blocked_by"} {
		if forge.matching(func(request string) bool { return strings.Contains(request, forbidden) }) != 0 {
			t.Fatalf("claimed candidate discussions requested: %v", forge.seen())
		}
	}
	if forge.matching(func(request string) bool { return strings.Contains(request, "/labels") }) != 0 {
		t.Fatalf("claimed candidate mutated: %v", forge.seen())
	}
}

func TestB4DependenciesPrecedeReadyContextEnrichment(t *testing.T) {
	root := selectionRepository(t)
	forge := newCandidateForge()
	forge.addIssue(1, "2000-01-01T00:00:00Z", "Branch: `slice-one`\n\nBlocked by: #9\n", "ready")
	forge.addIssue(2, "2001-01-01T00:00:00Z", "Branch: `slice-two`\n", "ready")
	forge.addIssue(3, "2002-01-01T00:00:00Z", "Branch: `slice-three`\n", "ready")
	forge.addIssue(9, "1999-01-01T00:00:00Z", "blocker\n")
	forge.readyPages = [][]int{{1, 2, 3}}
	forge.reworkPages = [][]int{{}}
	forge.owners[1], forge.owners[2], forge.owners[3] = nil, nil, nil

	got, err := selectionRun(t, root, forge, "implement", "next")
	if err != nil || got.Status != "work_available" || got.Item == nil || got.Item.Number != 2 {
		t.Fatalf("selection = %#v, %v", got, err)
	}
	for _, forbidden := range []string{"/issues/1/comments", "/issues/3", "/issues/3/comments"} {
		if forge.matching(func(request string) bool { return strings.Contains(request, forbidden) }) != 0 {
			t.Fatalf("blocked or later candidate enriched: %v", forge.seen())
		}
	}
}

func TestB5BlockerCompletionIsEstablishedOnlyForReferencedDependencies(t *testing.T) {
	cases := []struct {
		name     string
		blocker  map[string]any
		evidence []map[string]any
		want     int
	}{
		{"merged submission", map[string]any{"number": 9, "state": "closed"}, []map[string]any{{"merged": true, "mergedAt": "2026-01-01T00:00:00Z"}}, 1},
		{"done but not merged", map[string]any{"number": 9, "state": "open", "labels": []map[string]string{{"name": "done"}}}, []map[string]any{{"merged": false, "mergedAt": "", "state": "OPEN"}}, 2},
		{"closed without merged submission", map[string]any{"number": 9, "state": "closed"}, nil, 2},
		{"cannot be established as merged", map[string]any{"number": 9, "state": "open"}, nil, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := selectionRepository(t)
			forge := newCandidateForge()
			forge.addIssue(1, "2000-01-01T00:00:00Z", "Branch: `slice-one`\n\nBlocked by: #9\n", "ready")
			forge.addIssue(2, "2001-01-01T00:00:00Z", "Branch: `slice-two`\n", "ready")
			forge.issues[9] = tc.blocker
			forge.evidence[9] = tc.evidence
			forge.addIssue(500, "1990-01-01T00:00:00Z", "historical\n", "needs-human")
			forge.readyPages = [][]int{{1, 2}}
			forge.reworkPages = [][]int{{}}

			got, err := selectionRun(t, root, forge, "implement", "next")
			if err != nil || got.Status != "work_available" || got.Item == nil || got.Item.Number != tc.want {
				t.Fatalf("selection = %#v, %v", got, err)
			}
			for _, forbidden := range []string{"/issues/500", "/issues/3/", "/issues/4/"} {
				if forge.matching(func(request string) bool { return strings.Contains(request, forbidden) }) != 0 {
					t.Fatalf("unrelated terminal item loaded: %v", forge.seen())
				}
			}
		})
	}
}

func TestB6PaginationPreservesCandidateAndDependencyCompleteness(t *testing.T) {
	root := selectionRepository(t)
	forge := newCandidateForge()
	claimed := []int{}
	for number := 1000; number < 1100; number++ {
		forge.addIssue(number, "2020-01-01T00:00:00Z", "claimed\n", "ready", "wip")
		claimed = append(claimed, number)
	}
	forge.addIssue(101, "2021-01-01T00:00:00Z", "Branch: `slice-101`\n", "ready")
	forge.addIssue(200, "1999-01-01T00:00:00Z", "blocker\n")
	forge.addIssue(102, "2022-01-01T00:00:00Z", "Branch: `slice-102`\n", "ready")
	forge.readyPages = [][]int{claimed, {101, 102}}
	forge.reworkPages = [][]int{{}}
	page := []map[string]any{}
	for number := 1; number <= 100; number++ {
		forge.evidence[number] = []map[string]any{{"merged": true, "mergedAt": "2026-01-01T00:00:00Z"}}
		page = append(page, map[string]any{"number": number})
	}
	page = append(page, map[string]any{"number": 200})
	forge.dependencies[101] = page
	forge.evidence[200] = nil

	got, err := selectionRun(t, root, forge, "implement", "next")
	if err != nil || got.Status != "work_available" || got.Item == nil || got.Item.Number != 102 {
		t.Fatalf("selection = %#v, %v requests=%v", got, err, forge.seen())
	}
	if forge.matching(func(request string) bool {
		return strings.Contains(request, "labels=ready&") && strings.Contains(request, "page=2")
	}) == 0 {
		t.Fatalf("candidate continuation missing: %v", forge.seen())
	}
	if forge.matching(func(request string) bool {
		return strings.Contains(request, "/issues/101/dependencies/blocked_by") && strings.Contains(request, "page=2")
	}) == 0 {
		t.Fatalf("dependency continuation missing: %v", forge.seen())
	}
	for _, forbidden := range []string{"/issues/101/comments", "/issues/200/comments"} {
		if forge.matching(func(request string) bool { return strings.Contains(request, forbidden) }) != 0 {
			t.Fatalf("blocked candidate hydrated: %v", forge.seen())
		}
	}
}

func TestB7PublicationAndCommandsUseExplicitOwningLink(t *testing.T) {
	root := selectionRepository(t)
	prepareSlice(t, root, "widget")
	completeAndRetireSlice(t, root, "widget")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	forge := newCandidateForge()
	forge.addIssue(7, "2020-01-01T00:00:00Z", "Branch: `widget`\n", "ready")
	forge.reworkPages = [][]int{{}}
	forge.readyPages = [][]int{{7}}
	forge.heads["widget"] = head

	start, err := selectionRun(t, root, forge, "implement", "next")
	if err != nil || start.Status != "work_available" || start.Item == nil || start.Item.Number != 7 {
		t.Fatalf("start = %#v, %v", start, err)
	}
	body := filepath.Join(start.Packet.Facts.Implementation.ResultDirectory, "submission.md")
	if err := os.WriteFile(body, []byte("candidate"), 0600); err != nil {
		t.Fatal(err)
	}
	submitted, err := selectionRun(t, root, forge, "implement", "submit", "--item", "7", "--body", body)
	if err != nil || submitted.Status != "awaiting_review" {
		t.Fatalf("submit = %#v, %v", submitted, err)
	}
	if len(forge.pulls) != 1 || forge.pulls[11]["body"] != "candidate\n\nCloses #7\n" || !hasLabel(forge.pulls[11], "review") {
		t.Fatalf("publication did not establish the owning association: %#v", forge.pulls)
	}
	// The owning issue is renamed; ownership must survive it.
	forge.issues[7]["title"] = "renamed work item"
	forge.owners[7] = []int{11}

	forge.pulls[11]["labels"] = []map[string]string{{"name": "rework"}, {"name": "wip"}}
	forge.timeline[11] = append(forge.timeline[11], map[string]any{"event": "labeled", "created_at": "2026-01-01T00:00:01Z", "label": map[string]string{"name": "wip"}})
	resumed, err := selectionRun(t, root, forge, "implement", "resume", "--item", "7")
	if err != nil || resumed.Status != "work_available" || resumed.Item == nil || resumed.Item.Number != 7 {
		t.Fatalf("resume = %#v, %v", resumed, err)
	}
	if resumed.Packet.Facts.Implementation.Branch != "widget" {
		t.Fatalf("Git preparation did not use the PR head branch: %#v", resumed.Packet.Facts.Implementation)
	}
	status := selectionStatusRun(t, root, forge)
	if len(status.Items) != 1 || status.Items[0].Submission == nil || status.Items[0].Submission.Number != 11 {
		t.Fatalf("status attachment = %#v", status)
	}
	updated := filepath.Join(resumed.Packet.Facts.Implementation.ResultDirectory, "submission.md")
	if err := os.WriteFile(updated, []byte("updated"), 0600); err != nil {
		t.Fatal(err)
	}
	handoff, err := selectionRun(t, root, forge, "implement", "submit", "--item", "7", "--body", updated)
	if err != nil || handoff.Status != "awaiting_review" || len(forge.pulls) != 1 || forge.pulls[11]["body"] != "updated\n\nCloses #7\n" {
		t.Fatalf("update redirected ownership: %#v, %v pulls=%#v", handoff, err, forge.pulls)
	}
}

func TestB8InvalidOwnershipIsASelectedItemRefusal(t *testing.T) {
	cases := map[string]struct {
		body  string
		extra map[int]map[string]any
	}{
		"no explicit owning issue":    {"unowned body\n", nil},
		"multiple conflicting owners": {"opening\n\nCloses #7\nCloses #8\n", nil},
		"another active Submission":   {"opening\n\nCloses #7\n", map[int]map[string]any{}},
		"outside the repository":      {"opening\n\nCloses acme/other#7\n", nil},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			root := selectionRepository(t)
			forge := newCandidateForge()
			forge.addPull(30, "2020-01-01T00:00:00Z", tc.body, "slice-seven", strings.Repeat("a", 40), "rework")
			forge.addIssue(7, "2019-01-01T00:00:00Z", "Branch: `slice-seven`\n")
			forge.reworkPages = [][]int{{30}}
			switch name {
			case "another active Submission":
				forge.owners[7] = []int{30, 31}
			case "no explicit owning issue":
			default:
				forge.owners[7] = []int{30}
			}
			got, err := selectionRun(t, root, forge, "implement", "next")
			if err != nil || got.Status != "fix_required" || got.Reason == "" || got.Packet != nil {
				t.Fatalf("defect accepted: %#v, %v", got, err)
			}
			if forge.matching(func(request string) bool {
				return strings.Contains(request, "/labels") || strings.Contains(request, "search")
			}) != 0 {
				t.Fatalf("defect caused a claim or a title search: %v", forge.seen())
			}
		})
	}
}

func TestB9ClaimReadbackDoesNotRediscoverTheQueue(t *testing.T) {
	root := selectionRepository(t)
	forge := newCandidateForge()
	forge.addPull(30, "2020-01-01T00:00:00Z", "review\n\nCloses #7\n", "slice-seven", strings.Repeat("a", 40), "rework")
	forge.addIssue(7, "2019-01-01T00:00:00Z", "Branch: `slice-seven`\n")
	forge.reworkPages = [][]int{{30}}
	forge.owners[7] = []int{30}
	forge.comments["/issues/7/comments"] = []map[string]any{{"body": "human direction", "author_association": "OWNER", "user": map[string]string{"login": "maintainer"}}}

	got, err := selectionRun(t, root, forge, "implement", "next")
	if err != nil || got.Status != "work_available" || got.Item == nil || !got.Item.Claimed || got.Item.Number != 7 {
		t.Fatalf("claim = %#v, %v", got, err)
	}
	if queues := forge.matching(func(request string) bool { return request == "graphql:queue" }); queues != 1 {
		t.Fatalf("claim rediscovered the queue: %v", forge.seen())
	}
	if !hasLabel(forge.pulls[30], "wip") || !hasLabel(forge.pulls[30], "rework") {
		t.Fatalf("claim projection missing: %#v", forge.pulls[30]["labels"])
	}
	if len(forge.comments["/issues/7/comments"]) != 1 {
		t.Fatalf("unrelated feedback changed")
	}
}

func TestB10AcquisitionDriftCannotReturnAnInvalidHandoff(t *testing.T) {
	t.Run("candidate becomes claimed", func(t *testing.T) {
		root := selectionRepository(t)
		forge := newCandidateForge()
		forge.addPull(30, "2020-01-01T00:00:00Z", "review\n\nCloses #7\n", "slice-seven", strings.Repeat("a", 40), "rework")
		forge.addIssue(7, "2019-01-01T00:00:00Z", "Branch: `slice-seven`\n")
		forge.reworkPages = [][]int{{30}}
		forge.owners[7] = []int{30}
		forge.onPullRead = func(number int) {
			forge.pulls[number]["labels"] = appendLabels(forge.pulls[number]["labels"], "wip")
		}
		got, err := selectionRun(t, root, forge, "implement", "next")
		if err != nil || got.Status != "fix_required" || got.Packet != nil || !strings.Contains(got.Reason, "claimed before acquisition") {
			t.Fatalf("drift accepted: %#v, %v", got, err)
		}
		if forge.matching(func(request string) bool { return strings.Contains(request, "/labels") }) != 0 {
			t.Fatalf("drift overwrote projections: %v", forge.seen())
		}
	})
	t.Run("uncertain acquisition", func(t *testing.T) {
		root := selectionRepository(t)
		forge := newCandidateForge()
		forge.addPull(30, "2020-01-01T00:00:00Z", "review\n\nCloses #7\n", "slice-seven", strings.Repeat("a", 40), "rework")
		forge.addIssue(7, "2019-01-01T00:00:00Z", "Branch: `slice-seven`\n")
		forge.reworkPages = [][]int{{30}}
		forge.owners[7] = []int{30}
		forge.onLabelWrite = func(int) {
			// Fail both readback paths only after the Claim POST has landed.
			forge.fail["GET /issues/30"] = http.StatusInternalServerError
			forge.fail["GET /pulls/30"] = http.StatusInternalServerError
		}
		forge.errorAfter["POST /issues/30/labels"] = true
		got, err := selectionRun(t, root, forge, "implement", "next")
		recovery := ""
		if err != nil {
			recovery = err.Error()
		} else {
			recovery = got.Reason
		}
		if got.Packet != nil || !strings.Contains(recovery, "inspect") {
			t.Fatalf("uncertain acquisition not reported with recovery: %#v, %v", got, err)
		}
		if queues := forge.matching(func(request string) bool { return request == "graphql:queue" }); queues != 1 {
			t.Fatalf("uncertainty triggered a replacement selection: %v", forge.seen())
		}
		if forge.matching(func(request string) bool { return request == "POST /repos/acme/widgets/issues/30/labels" }) != 1 || !hasLabel(forge.pulls[30], "wip") {
			t.Fatalf("uncertain Claim was never accepted or was released: %v labels=%v", forge.seen(), forge.pulls[30]["labels"])
		}
		if forge.matching(func(request string) bool { return strings.HasPrefix(request, "DELETE ") }) != 0 {
			t.Fatalf("uncertain Claim was released: %v", forge.seen())
		}
	})
}

func TestB11StartupDoesNotRequireProjectObjects(t *testing.T) {
	t.Run("implement", func(t *testing.T) {
		root := selectionRepository(t)
		traceFile := filepath.Join(t.TempDir(), "git-trace.json")
		t.Setenv("GIT_TRACE2_EVENT", traceFile)
		forge := newCandidateForge()
		forge.addIssue(7, "2020-01-01T00:00:00Z", "Branch: `slice-seven`\n", "ready")
		forge.reworkPages = [][]int{{}}
		forge.readyPages = [][]int{{7}}

		got, err := selectionRun(t, root, forge, "implement", "next")
		if err != nil || got.Status != "work_available" || got.Packet == nil {
			t.Fatalf("start = %#v, %v", got, err)
		}
		facts := got.Packet.Facts.Implementation
		if facts.FetchCommand == "" || facts.WorktreeCommand == "" || facts.InspectCommand == "" || facts.ArtifactBaseline != "" || facts.ArtifactCompletion != "" {
			t.Fatalf("preparation facts = %#v", facts)
		}
		if gitRefExists(root, "refs/heads/slice-seven") {
			t.Fatal("startup created or required the local branch")
		}
		if _, err := os.Stat(filepath.Join(root, ".worktrees", "slice-seven")); err == nil {
			t.Fatal("startup created a worktree")
		}
		assertNoProjectObjectReads(t, traceFile)
	})
	t.Run("watchdog", func(t *testing.T) {
		root := selectionRepository(t)
		forge := newCandidateForge()
		forge.addPull(30, "2020-01-01T00:00:00Z", "review\n\nCloses #7\n", "slice-seven", strings.Repeat("a", 40), "review")
		forge.addIssue(7, "2019-01-01T00:00:00Z", "Branch: `slice-seven`\n")
		forge.reviewPages = [][]int{{30}}
		forge.owners[7] = []int{30}

		got, err := selectionRun(t, root, forge, "watchdog", "next")
		if err != nil || got.Status != "work_available" || got.Packet == nil {
			t.Fatalf("start = %#v, %v", got, err)
		}
		facts := got.Packet.Facts.Watchdog
		if facts.ReviewedHead != strings.Repeat("a", 40) || facts.ReviewCount != 0 || facts.ReviewNumber != 1 || facts.PreviousReviewedHead != "" || facts.FetchCommand == "" || facts.WorktreeCommand == "" || facts.InspectCommand == "" {
			t.Fatalf("review facts = %#v", facts)
		}
		if !strings.Contains(got.Packet.Instructions, "Inspect") {
			t.Fatalf("packet lost the inspection instruction: %s", got.Packet.Instructions)
		}
		if gitRefExists(root, "refs/heads/slice-seven") {
			t.Fatal("startup required the local branch")
		}
	})
}

func TestB12ResumeIsExplicitContinuation(t *testing.T) {
	t.Run("explicit item", func(t *testing.T) {
		root := selectionRepository(t)
		prepareSlice(t, root, "slice-seven")
		worktree := filepath.Join(root, ".worktrees", "slice-seven")
		runGit(t, root, "switch", "main")
		runGit(t, root, "worktree", "add", worktree, "slice-seven")
		progress := filepath.Join(worktree, "progress.txt")
		if err := os.WriteFile(progress, []byte("unfinished\n"), 0600); err != nil {
			t.Fatal(err)
		}
		forge := newCandidateForge()
		forge.addPull(30, "2020-01-01T00:00:00Z", "review\n\nCloses #7\n", "slice-seven", strings.TrimSpace(runGitOutput(t, worktree, "rev-parse", "HEAD")), "rework", "wip")
		forge.addIssue(7, "2019-01-01T00:00:00Z", "Branch: `slice-seven`\n")
		forge.timeline[30] = []map[string]any{{"event": "labeled", "created_at": "2026-01-01T00:00:01Z", "label": map[string]string{"name": "wip"}}}
		forge.owners[7] = []int{30}

		got, err := selectionRun(t, root, forge, "implement", "resume", "--item", "7")
		if err != nil || got.Status != "work_available" || got.Item == nil || got.Item.Number != 7 || !got.Item.Claimed {
			t.Fatalf("resume = %#v, %v", got, err)
		}
		if forge.matching(func(request string) bool { return request == "graphql:queue" }) != 0 {
			t.Fatalf("resume selected again: %v", forge.seen())
		}
		if readFile(t, progress) != "unfinished\n" {
			t.Fatal("resume discarded local work")
		}
	})
	t.Run("unavailable claim", func(t *testing.T) {
		root := selectionRepository(t)
		forge := newCandidateForge()
		forge.addPull(30, "2020-01-01T00:00:00Z", "review\n\nCloses #7\n", "slice-seven", strings.Repeat("a", 40), "rework")
		forge.addIssue(7, "2019-01-01T00:00:00Z", "Branch: `slice-seven`\n")
		forge.owners[7] = []int{30}

		got, err := selectionRun(t, root, forge, "implement", "resume", "--item", "7")
		if err != nil || got.Status != "fix_required" || got.Packet != nil {
			t.Fatalf("unavailable claim = %#v, %v", got, err)
		}
		if forge.matching(func(request string) bool { return request == "graphql:queue" }) != 0 {
			t.Fatalf("unavailable claim fell back to selection: %v", forge.seen())
		}
	})
	t.Run("worktree only", func(t *testing.T) {
		root := selectionRepository(t)
		head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "main"))
		runGit(t, root, "worktree", "add", filepath.Join(root, ".worktrees", "slice-seven"), "-b", "slice-seven", "main")
		forge := newCandidateForge()
		forge.addPull(30, "2020-01-01T00:00:00Z", "review\n\nCloses #7\n", "slice-seven", head, "rework", "wip")
		forge.addIssue(7, "2019-01-01T00:00:00Z", "Branch: `slice-seven`\n")
		forge.timeline[30] = []map[string]any{{"event": "labeled", "created_at": "2026-01-01T00:00:01Z", "label": map[string]string{"name": "wip"}}}
		forge.owners[7] = []int{30}

		got, err := selectionRun(t, filepath.Join(root, ".worktrees", "slice-seven"), forge, "implement", "resume")
		if err != nil || got.Status != "work_available" || got.Item == nil || got.Item.Number != 7 {
			t.Fatalf("worktree resume = %#v, %v", got, err)
		}
	})
	t.Run("ambiguous worktree", func(t *testing.T) {
		root := selectionRepository(t)
		runGit(t, root, "worktree", "add", filepath.Join(root, ".worktrees", "orphan"), "-b", "orphan", "main")
		forge := newCandidateForge()
		forge.reworkPages = [][]int{{}}

		got, err := selectionRun(t, root, forge, "implement", "resume")
		if err != nil || got.Status != "fix_required" || !strings.Contains(got.Reason, "--item") {
			t.Fatalf("ambiguous worktree = %#v, %v", got, err)
		}
	})
}

func TestB13OnlySelectedFeedbackIsHydratedComplete(t *testing.T) {
	root := selectionRepository(t)
	forge := newCandidateForge()
	forge.addPull(30, "2020-01-01T00:00:00Z", "review\n\nCloses #7\n", "slice-seven", strings.Repeat("a", 40), "review")
	forge.addIssue(7, "2019-01-01T00:00:00Z", "Branch: `slice-seven`\n")
	forge.reviewPages = [][]int{{30}}
	forge.owners[7] = []int{30}
	source := []map[string]any{}
	for i := 0; i < 100; i++ {
		source = append(source, map[string]any{"body": fmt.Sprintf("source finding %d", i), "author_association": "MEMBER", "created_at": "2026-01-01T00:00:00Z", "user": map[string]string{"login": "source"}})
	}
	source = append(source, map[string]any{"body": "last source finding", "author_association": "OWNER", "created_at": "2026-01-01T00:00:01Z", "user": map[string]string{"login": "owner"}})
	forge.comments["/issues/7/comments"] = source
	forge.comments["/issues/30/comments"] = []map[string]any{{"body": "pr discussion", "author_association": "OWNER", "user": map[string]string{"login": "maintainer"}}}
	forge.comments["/pulls/30/comments"] = []map[string]any{{"body": "inline finding", "path": "main.go", "line": 12, "side": "RIGHT", "commit_id": strings.Repeat("a", 40), "author_association": "OWNER"}}
	forge.comments["/pulls/30/reviews"] = []map[string]any{
		{"body": "review one", "commit_id": strings.Repeat("a", 40), "state": "COMMENTED", "submitted_at": "2026-01-01T00:00:00Z", "author_association": "OWNER", "user": map[string]string{"login": "reviewer"}},
		{"body": "human directive", "commit_id": strings.Repeat("a", 40), "state": "COMMENTED", "submitted_at": "2026-01-01T00:00:01Z", "author_association": "OWNER", "user": map[string]string{"login": "maintainer"}},
	}
	forge.fail["GET /issues/99/comments"] = http.StatusInternalServerError

	got, err := selectionRun(t, root, forge, "watchdog", "next")
	if err != nil || got.Status != "work_available" || got.Packet == nil {
		t.Fatalf("selection = %#v, %v", got, err)
	}
	comments := got.Packet.Facts.Watchdog.Comments
	bodies := map[string]bool{}
	for _, comment := range comments {
		bodies[comment.Body] = true
	}
	if !bodies["source finding 0"] || !bodies["last source finding"] || !bodies["pr discussion"] || !bodies["inline finding"] || !bodies["review one"] || !bodies["human directive"] {
		t.Fatalf("feedback truncated: %d comments", len(comments))
	}
	for _, comment := range comments {
		switch comment.Body {
		case "inline finding":
			if comment.Path != "main.go" || comment.Line != 12 || comment.Side != "RIGHT" || comment.Commit != strings.Repeat("a", 40) {
				t.Fatalf("anchor facts lost: %#v", comment)
			}
		case "last source finding":
			if comment.Author != "owner" || comment.Association != "OWNER" || comment.CreatedAt != "2026-01-01T00:00:01Z" {
				t.Fatalf("source provenance lost: %#v", comment)
			}
		case "pr discussion":
			if comment.Author != "maintainer" || comment.Association != "OWNER" {
				t.Fatalf("discussion provenance lost: %#v", comment)
			}
		case "human directive":
			if comment.Author != "maintainer" || comment.Association != "OWNER" || comment.Commit != strings.Repeat("a", 40) {
				t.Fatalf("directive provenance lost: %#v", comment)
			}
		}
	}
	for _, request := range forge.seen() {
		if strings.Contains(request, "/issues/99") {
			t.Fatalf("unrelated discussion requested: %v", forge.seen())
		}
	}
}

func TestB14RequiredObservationFailureIsNotAnEmptyQueue(t *testing.T) {
	cases := []struct {
		name      string
		configure func(*candidateForge)
	}{
		{"authentication failure", func(f *candidateForge) { f.fail["POST /graphql"] = http.StatusUnauthorized }},
		{"server failure", func(f *candidateForge) { f.fail["POST /graphql"] = http.StatusInternalServerError }},
		{"partial result with an error", func(f *candidateForge) { f.graphqlError = "partial result" }},
		{"missing continuation page", func(f *candidateForge) {
			page := []int{}
			for number := 1000; number < 1100; number++ {
				f.addIssue(number, "2020-01-01T00:00:00Z", "claimed\n", "ready", "wip")
				page = append(page, number)
			}
			f.readyPages = [][]int{page}
			f.fail["GET /issues?state=open&labels=ready&sort=created&direction=asc&filter=all&per_page=100&page=2"] = http.StatusInternalServerError
		}},
		{"inaccessible dependency", func(f *candidateForge) {
			f.addIssue(1, "2020-01-01T00:00:00Z", "Branch: `slice-one`\n\nBlocked by: #9\n", "ready")
			f.readyPages = [][]int{{1}}
			f.fail["GET /issues/1/dependencies/blocked_by"] = http.StatusInternalServerError
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := selectionRepository(t)
			forge := newCandidateForge()
			forge.reworkPages = [][]int{{}}
			tc.configure(forge)
			got, err := selectionRun(t, root, forge, "implement", "next")
			if err == nil && got.Status != "fix_required" {
				t.Fatalf("incomplete observation did not produce an error or refusal: %#v", got)
			}
			if err != nil && got.Status != "" {
				t.Fatalf("error also wrote an outcome: %#v", got)
			}
			if forge.matching(func(request string) bool { return strings.Contains(request, "/labels") }) != 0 {
				t.Fatalf("failure claimed work: %v", forge.seen())
			}
			if tc.name == "missing continuation page" && forge.matching(func(request string) bool {
				return strings.Contains(request, "labels=ready&") && strings.Contains(request, "page=2")
			}) != 1 {
				t.Fatalf("failure never reached the required continuation: %v", forge.seen())
			}
		})
	}
}

func TestB15StartupComposesWithSafeHandoffsAndPrivateCheckpoints(t *testing.T) {
	root := selectionRepository(t)
	forge := newCandidateForge()
	forge.addPull(30, "2020-01-01T00:00:00Z", "review\n\nCloses #7\n", "slice-seven", strings.Repeat("a", 40), "review", "wip")
	forge.addIssue(7, "2019-01-01T00:00:00Z", "Branch: `slice-seven`\n")
	forge.reviewPages = [][]int{{30}}
	forge.owners[7] = []int{30}

	// An in-flight handoff still holding its source Claim is not yet acquirable.
	held, err := selectionRun(t, root, forge, "watchdog", "next")
	if err != nil || held.Status != "no_work" {
		t.Fatalf("claimed destination acquired: %#v, %v", held, err)
	}
	forge.removeLabel(30, "wip")
	got, err := selectionRun(t, root, forge, "watchdog", "next")
	if err != nil || got.Status != "work_available" || got.Packet == nil {
		t.Fatalf("released destination refused: %#v, %v", got, err)
	}
	instructions := got.Packet.Instructions
	for _, forbidden := range []string{".watchdog", "skl.implement/v1", "review_round_head", "target_snapshot"} {
		if strings.Contains(instructions, forbidden) {
			t.Fatalf("packet leaked private or retired state %q: %s", forbidden, instructions)
		}
	}
	for _, required := range []string{got.Packet.Facts.Watchdog.InspectCommand, got.Packet.Facts.Watchdog.SubmitCommand} {
		if !strings.Contains(instructions, required) {
			t.Fatalf("packet lacks concrete command %q", required)
		}
	}
}

func TestW14EachHTTPRequestIsCountedOnce(t *testing.T) {
	forge := newCandidateForge()
	request := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(`{"query":"pullRequests(","variables":{"label":["review"]}}`))
	response := httptest.NewRecorder()
	forge.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !reflect.DeepEqual(forge.seen(), []string{"graphql:queue"}) {
		t.Fatalf("one HTTP request must have exactly one purpose: status=%d requests=%v", response.Code, forge.seen())
	}
}

func TestB16UnrelatedHistoryDoesNotScaleWorkStartCost(t *testing.T) {
	build := func(history int, work bool, lane string) *candidateForge {
		forge := newCandidateForge()
		if work {
			if lane == "implement" {
				forge.addIssue(7, "2020-01-01T00:00:00Z", "Branch: `slice-seven`\n", "ready")
				forge.reworkPages = [][]int{{}}
				forge.readyPages = [][]int{{7}}
			} else {
				forge.addPull(30, "2020-01-01T00:00:00Z", "review\n\nCloses #7\n", "slice-seven", strings.Repeat("a", 40), "review")
				forge.addIssue(7, "2019-01-01T00:00:00Z", "Branch: `slice-seven`\n")
				forge.reviewPages = [][]int{{30}}
				forge.owners[7] = []int{30}
				forge.comments["/issues/30/comments"] = []map[string]any{{"body": "selected feedback", "author_association": "OWNER"}}
			}
		} else if lane == "implement" {
			forge.reworkPages = [][]int{{}}
			forge.readyPages = [][]int{{}}
		} else {
			forge.reviewPages = [][]int{{}}
		}
		for number := 0; number < history; number++ {
			forge.addIssue(10000+number, "2015-01-01T00:00:00Z", "historical work\n", "needs-human")
			forge.addPull(20000+number, "2015-01-01T00:00:00Z", "historical discussion\n", "old", strings.Repeat("e", 40), "done")
			forge.comments[fmt.Sprintf("/issues/%d/comments", 20000+number)] = []map[string]any{{"body": strings.Repeat("discussion\n", 200)}}
		}
		return forge
	}
	for _, lane := range []string{"implement", "watchdog"} {
		for _, work := range []bool{false, true} {
			counts := map[int]int{}
			medians := map[int]time.Duration{}
			tails := map[int]time.Duration{}
			purposes := map[int]map[string]int{}
			for _, history := range []int{2, 200} {
				iterations := 5
				var durations []time.Duration
				purposes[history] = map[string]int{}
				for iteration := 0; iteration < iterations; iteration++ {
					root := selectionRepository(t)
					traceFile := filepath.Join(t.TempDir(), "git-trace.json")
					t.Setenv("GIT_TRACE2_EVENT", traceFile)
					forge := build(history, work, lane)
					start := time.Now()
					got, err := selectionRun(t, root, forge, lane, "next")
					durations = append(durations, time.Since(start))
					if err != nil {
						t.Fatalf("history=%d: %v", history, err)
					}
					if work && got.Status != "work_available" || !work && got.Status != "no_work" {
						t.Fatalf("history=%d selection = %#v", history, got)
					}
					counts[history] += len(forge.seen())
					for _, request := range forge.seen() {
						purposes[history][purposeOf(request)]++
					}
					assertNoProjectObjectReads(t, traceFile)
				}
				slices.Sort(durations)
				medians[history] = durations[len(durations)/2]
				tails[history] = durations[len(durations)-1]
			}
			if counts[2] != counts[200] || !reflect.DeepEqual(purposes[2], purposes[200]) {
				t.Fatalf("%s work=%t request work grew with unrelated history: %d vs %d, %v vs %v", lane, work, counts[2], counts[200], purposes[2], purposes[200])
			}
			perOperation := map[string]int{}
			for purpose, count := range purposes[2] {
				perOperation[purpose] = count / 5
			}
			t.Logf("%s work=%t requests/op=%d by purpose=%v median history=2:%s history=200:%s tail history=2:%s history=200:%s", lane, work, counts[2]/5, perOperation, medians[2], medians[200], tails[2], tails[200])
		}
	}
}

func purposeOf(request string) string {
	switch {
	case strings.HasPrefix(request, "graphql:queue"):
		return "candidate-queue"
	case strings.HasPrefix(request, "graphql:owners"):
		return "owning-link"
	case strings.HasPrefix(request, "graphql:body"):
		return "body-evidence"
	case strings.HasPrefix(request, "graphql:"):
		return "graphql-other"
	case strings.Contains(request, "/labels"):
		return "label-mutation"
	case strings.Contains(request, "labels=ready&"):
		return "ready-queue"
	case strings.Contains(request, "/dependencies/blocked_by"):
		return "dependencies"
	case strings.Contains(request, "/comments") || strings.Contains(request, "/reviews"):
		return "feedback"
	default:
		return "rest-read"
	}
}
