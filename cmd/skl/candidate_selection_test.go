package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

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
