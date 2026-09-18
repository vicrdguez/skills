package setup

import (
	"fmt"
	"strings"

	skilldist "github.com/vicrdguez/skills"
)

type watchdogStreamPaths struct {
	source, discussion, summaries, inline string
}

func selectedWatchdogStreams(item, submission int) watchdogStreamPaths {
	return watchdogStreamPaths{
		source: fmt.Sprintf("/issues/%d/comments", item), discussion: fmt.Sprintf("/issues/%d/comments", submission),
		summaries: fmt.Sprintf("/pulls/%d/reviews", submission), inline: fmt.Sprintf("/pulls/%d/comments", submission),
	}
}

func (paths watchdogStreamPaths) all() []string {
	return []string{paths.source, paths.discussion, paths.summaries, paths.inline}
}

func watchdogEvidenceInstructions(f skilldist.WatchdogFacts) string {
	var body strings.Builder
	if f.Repository == "" {
		body.WriteString("Selected repository identity unavailable; resolve it before retrieving missing feedback.\n")
		return body.String()
	}
	states := make(map[string]string, len(f.EvidenceStreams))
	for _, stream := range f.EvidenceStreams {
		states[strings.TrimPrefix(stream.Path, "/")] = stream.State
	}
	for _, stream := range selectedWatchdogStreams(f.WorkItem, f.Submission).all() {
		path := "repos/" + f.Repository + stream
		switch states[path] {
		case "fetched_empty":
			fmt.Fprintf(&body, "- `%s`: fetched completely and empty.\n", path)
		case "fetched":
			fmt.Fprintf(&body, "- `%s`: fetched completely; supplied records appear above.\n", path)
		default:
			fmt.Fprintf(&body, "- `%s`: pending delivery; `gh api --paginate %s`.\n", path, skilldist.ShellQuote(path))
		}
	}
	return body.String()
}
