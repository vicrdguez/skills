package setup

import (
	"encoding/json"
	"strings"
)

const reviewSummaryPrefix = "<!-- skl.watchdog.review/v1\n"

type reviewSummaryMetadata struct {
	ReviewNumber uint64 `json:"review_number"`
	Verdict      string `json:"verdict"`
	FinalHead    string `json:"final_head,omitempty"`
}

func parseReviewSummary(body string) (reviewSummaryMetadata, string, bool) {
	body, ok := strings.CutPrefix(body, reviewSummaryPrefix)
	if !ok {
		return reviewSummaryMetadata{}, "", false
	}
	metadata, body, ok := strings.Cut(body, "\n-->\n")
	if !ok {
		return reviewSummaryMetadata{}, "", false
	}
	var parsed reviewSummaryMetadata
	if json.Unmarshal([]byte(metadata), &parsed) != nil || parsed.ReviewNumber == 0 || parsed.Verdict != "rework" && parsed.Verdict != "pass" && parsed.Verdict != "needs-human" {
		return reviewSummaryMetadata{}, "", false
	}
	return parsed, body, true
}
