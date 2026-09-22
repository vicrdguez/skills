package setup

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/ledger"
)

// hunkHeader matches one unified-diff hunk header, for example
// `@@ -12,7 +12,9 @@`. The line counts are optional in the format.
var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

// pullFile is one file of a pull request diff as the files endpoint reports
// it. An empty patch means the anchor lines cannot be established.
type pullFile struct {
	Filename string `json:"filename"`
	Status   string `json:"status"`
	Patch    string `json:"patch"`
}

// publishSelectedFindings reconciles only the explicitly selected
// human-authored findings. Each finding is validated against the actual
// reviewed diff at the exact reviewed revision, deduplicated by its exact
// authorized prose, commit, path, line, and side, and reported separately
// from body publication. Private report text is never exported.
func (b *GitHubBackend) publishSelectedFindings(ctx context.Context, repository github.RepositoryID, pull githubPull, presentation ledger.RecoveryPresentation, guard func() error) ([]ledger.FindingPublication, error) {
	var receipts []ledger.FindingPublication
	reviewed := presentation.Reviewed
	if reviewed == "" {
		reviewed = pull.Head.SHA
	}
	for _, finding := range presentation.Findings {
		publication := ledger.FindingPublication{ID: finding.ID}
		if err := guard(); err != nil {
			return receipts, err
		}
		if finding.ID == "" || finding.Body == "" || finding.Path == "" || finding.Line <= 0 || !b.AnchorSide(finding.Side) {
			publication.Status = recoveryFindingUnresolved
			publication.Detail = "the selected finding lacks the authorized prose or reviewed path, line, and side required for inline publication"
			receipts = append(receipts, publication)
			continue
		}
		if finding.Commit == "" || finding.Commit != reviewed || pull.Head.SHA != reviewed {
			publication.Status = recoveryFindingUnresolved
			publication.Detail = fmt.Sprintf("the selected finding must anchor to the exact reviewed revision %s observable on pull request #%d; no replacement location was guessed", reviewed, pull.Number)
			receipts = append(receipts, publication)
			continue
		}
		if err := b.validateSelectedFinding(ctx, repository, pull.Number, finding); err != nil {
			publication.Status = recoveryFindingUnresolved
			publication.Detail = "the selected anchor is not part of the reviewed diff: " + err.Error() + "; no replacement location was guessed"
			receipts = append(receipts, publication)
			continue
		}
		observed, err := b.selectedFindingObserved(ctx, repository, pull.Number, finding)
		if err != nil {
			publication.Status = recoveryPending
			publication.Detail = "the existing inline discussion could not be read completely: " + err.Error()
			receipts = append(receipts, publication)
			continue
		}
		if observed {
			publication.Status = recoveryFindingSatisfied
			receipts = append(receipts, publication)
			continue
		}
		if presentation.ObserveOnly {
			publication.Status = recoveryPending
			publication.Detail = "the selected finding is not yet published and observation only makes no write"
			receipts = append(receipts, publication)
			continue
		}
		if err := guard(); err != nil {
			return receipts, err
		}
		writeErr := b.request(ctx, http.MethodPost, b.repositoryPath(repository)+fmt.Sprintf("/pulls/%d/comments", pull.Number), map[string]any{"body": finding.Body, "commit_id": finding.Commit, "path": finding.Path, "line": finding.Line, "side": finding.Side}, nil)
		observed, err = b.selectedFindingObserved(ctx, repository, pull.Number, finding)
		if err != nil {
			publication.Status = recoveryPending
			publication.Detail = "inline publication could not be confirmed: " + err.Error()
			receipts = append(receipts, publication)
			continue
		}
		if observed {
			publication.Status = recoveryFindingSatisfied
			receipts = append(receipts, publication)
			continue
		}
		publication.Status = recoveryPending
		if writeErr != nil {
			publication.Detail = writeErr.Error()
		} else {
			publication.Detail = "inline publication was not observed; retry the same selected finding"
		}
		receipts = append(receipts, publication)
	}
	return receipts, nil
}

// selectedFindingObserved reports whether the exact authorized inline effect
// already exists. Deduplication uses the finding's exact prose and anchor,
// not a similar body or a moved location.
func (b *GitHubBackend) selectedFindingObserved(ctx context.Context, repository github.RepositoryID, number int, finding ledger.SelectedFinding) (bool, error) {
	comments, err := b.implementationComments(ctx, repository, fmt.Sprintf("/pulls/%d/comments", number))
	if err != nil {
		return false, err
	}
	for _, comment := range comments {
		if comment.Body == finding.Body && comment.Commit == finding.Commit && comment.Path == finding.Path && comment.Line == finding.Line && comment.Side == finding.Side {
			return true, nil
		}
	}
	return false, nil
}

// validateSelectedFinding checks the supplied path, line, and side against
// the actual reviewed diff of the pull request at its reviewed head. A path
// or line that is not anchorable there is unresolved rather than relocated.
func (b *GitHubBackend) validateSelectedFinding(ctx context.Context, repository github.RepositoryID, number int, finding ledger.SelectedFinding) error {
	if finding.Path == "" || path.IsAbs(finding.Path) || path.Clean(finding.Path) != finding.Path || strings.HasPrefix(finding.Path, "../") {
		return fmt.Errorf("path %q is not a reviewed repository path", finding.Path)
	}
	if !b.AnchorSide(finding.Side) {
		return fmt.Errorf("side %q is not a native review side", finding.Side)
	}
	anchors, err := b.reviewedAnchorLines(ctx, repository, number)
	if err != nil {
		return err
	}
	lines, found := anchors[finding.Path]
	if !found {
		return fmt.Errorf("path %q is not part of the reviewed diff", finding.Path)
	}
	if !lines[finding.Side][finding.Line] {
		return fmt.Errorf("line %d is not an anchorable %s line of the reviewed diff", finding.Line, finding.Side)
	}
	return nil
}

// reviewedAnchorLines reads every page of the reviewed pull request diff and
// reports the file lines that can carry an inline comment on each side.
func (b *GitHubBackend) reviewedAnchorLines(ctx context.Context, repository github.RepositoryID, number int) (map[string]map[string]map[int]bool, error) {
	anchors := map[string]map[string]map[int]bool{}
	for page := 1; ; page++ {
		var files []pullFile
		request := b.repositoryPath(repository) + fmt.Sprintf("/pulls/%d/files?per_page=100&page=%d", number, page)
		if err := b.request(ctx, http.MethodGet, request, nil, &files); err != nil {
			return nil, fmt.Errorf("the reviewed diff could not be read completely: %v", err)
		}
		for _, file := range files {
			left, right := patchAnchorLines(file.Patch)
			set := anchors[file.Filename]
			if set == nil {
				set = map[string]map[int]bool{}
				anchors[file.Filename] = set
			}
			set["LEFT"] = left
			set["RIGHT"] = right
		}
		if len(files) < 100 {
			return anchors, nil
		}
	}
}

// patchAnchorLines reports the old-file numbers anchorable on the LEFT side
// and the new-file numbers anchorable on the RIGHT side of one unified diff
// patch. Context lines are anchorable on both sides; removed lines only on
// the left and added lines only on the right.
func patchAnchorLines(patch string) (map[int]bool, map[int]bool) {
	left := map[int]bool{}
	right := map[int]bool{}
	oldLine, newLine := 0, 0
	inHunk := false
	for _, line := range strings.Split(patch, "\n") {
		if strings.HasPrefix(line, "@@") {
			oldStart, newStart, ok := parseHunkHeader(line)
			if !ok {
				inHunk = false
				continue
			}
			oldLine, newLine = oldStart, newStart
			inHunk = true
			continue
		}
		if !inHunk || line == "" {
			continue
		}
		switch line[0] {
		case ' ':
			left[oldLine] = true
			right[newLine] = true
			oldLine++
			newLine++
		case '-':
			left[oldLine] = true
			oldLine++
		case '+':
			right[newLine] = true
			newLine++
		}
	}
	return left, right
}

func parseHunkHeader(line string) (int, int, bool) {
	matches := hunkHeader.FindStringSubmatch(line)
	if matches == nil {
		return 0, 0, false
	}
	oldStart, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, 0, false
	}
	newStart, err := strconv.Atoi(matches[2])
	if err != nil {
		return 0, 0, false
	}
	return oldStart, newStart, true
}
