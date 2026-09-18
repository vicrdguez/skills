package setup

import (
	"fmt"
	"strings"

	skilldist "github.com/vicrdguez/skills"
)

func watchdogEvidenceInstructions(f skilldist.WatchdogFacts) string {
	var body strings.Builder
	fmt.Fprintf(&body, "The selected PR body and Audit ledger were supplied above, including when the body is empty. For feedback, only a successful complete read establishes an empty stream. A failed or truncated read must be repaired and retried before judgment; stop if it remains incomplete.\n\n")
	if f.Repository == "" {
		body.WriteString("Selected repository identity is unavailable in this transport; resolve it before retrieving missing feedback.\n")
		return body.String()
	}
	paths := []string{
		fmt.Sprintf("repos/%s/issues/%d/comments", f.Repository, f.WorkItem),
		fmt.Sprintf("repos/%s/issues/%d/comments", f.Repository, f.Submission),
		fmt.Sprintf("repos/%s/pulls/%d/reviews", f.Repository, f.Submission),
		fmt.Sprintf("repos/%s/pulls/%d/comments", f.Repository, f.Submission),
	}
	states := make(map[string]string, len(f.EvidenceStreams))
	for _, stream := range f.EvidenceStreams {
		states[strings.TrimPrefix(stream.Path, "/")] = stream.State
	}
	for _, path := range paths {
		switch states[path] {
		case "fetched_empty":
			fmt.Fprintf(&body, "- `%s`: fetched completely and empty.\n", path)
		case "fetched":
			fmt.Fprintf(&body, "- `%s`: fetched completely; supplied records appear above.\n", path)
		default:
			fmt.Fprintf(&body, "- `%s`: pending delivery. Retrieve every page with `gh api --paginate %s` before judgment. Preserve the raw author, association, time, commit, and inline anchors.\n", path, skilldist.ShellQuote(path))
		}
	}
	return body.String()
}
