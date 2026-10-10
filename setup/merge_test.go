package setup

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

// mergeForge serves one pull request of widget into proposal/feature and
// records the merge requests it receives.
type mergeForge struct {
	head, mergeSHA string
	refusal        string
	merges         []map[string]string
}

func (f *mergeForge) serve(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/widgets/pulls/7":
		state := "open"
		if f.mergeSHA != "" {
			state = "closed"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"number": 7, "state": state, "merged": f.mergeSHA != "", "merge_commit_sha": f.mergeSHA,
			"head": map[string]any{"ref": "widget", "sha": f.head, "repo": map[string]string{"full_name": "acme/widgets"}},
			"base": map[string]any{"ref": "proposal/feature"},
		})
	case r.Method == http.MethodPut && r.URL.Path == "/repos/acme/widgets/pulls/7/merge":
		var payload map[string]string
		_ = json.NewDecoder(r.Body).Decode(&payload)
		f.merges = append(f.merges, payload)
		if f.refusal != "" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			_, _ = fmt.Fprintf(w, `{"message":%q}`, f.refusal)
			return
		}
		f.mergeSHA = "squash"
		_, _ = fmt.Fprint(w, `{"sha":"squash","merged":true}`)
	default:
		http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusNotFound)
	}
}

func TestMergePullSquashesAtTheExpectedHead(t *testing.T) {
	merge := ledger.PullMerge{Number: 7, Branch: "widget", Head: "final", Base: "proposal/feature"}
	for _, tc := range []struct {
		name           string
		forge          mergeForge
		commit, reason string
		requests       int
	}{
		{name: "merges", forge: mergeForge{head: "final"}, commit: "squash", requests: 1},
		{name: "already merged", forge: mergeForge{head: "final", mergeSHA: "earlier"}, commit: "earlier"},
		{name: "head moved", forge: mergeForge{head: "newer"}, reason: "not the final head final"},
		{name: "forge refuses", forge: mergeForge{head: "final", refusal: "Required status check is expected"}, reason: "Required status check is expected", requests: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			forge := tc.forge
			server := httptest.NewServer(http.HandlerFunc(forge.serve))
			defer server.Close()
			commit, err := deliveryBackend(server).MergePull(context.Background(), merge)
			if commit != tc.commit || (tc.reason == "") != (err == nil) || err != nil && !strings.Contains(err.Error(), tc.reason) {
				t.Fatalf("MergePull = %q, %v; want %q, %q", commit, err, tc.commit, tc.reason)
			}
			if len(forge.merges) != tc.requests {
				t.Fatalf("merge requests = %+v, want %d", forge.merges, tc.requests)
			}
			for _, request := range forge.merges {
				if request["merge_method"] != "squash" || request["sha"] != "final" {
					t.Fatalf("merge request = %+v, want a squash pinned to the final head", request)
				}
			}
		})
	}
	unused := httptest.NewServer(http.NotFoundHandler())
	defer unused.Close()
	if _, err := deliveryBackend(unused).MergePull(context.Background(), ledger.PullMerge{Number: 7, Branch: "widget", Head: "final", Base: "main"}); err == nil {
		t.Fatal("MergePull accepted main as its base")
	}
}
