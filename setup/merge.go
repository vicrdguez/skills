package setup

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/vicrdguez/skills/ledger"
	"github.com/vicrdguez/skills/workflow"
)

var _ ledger.MergeForge = (*GitHubBackend)(nil)

// MergePull squash-merges the owned pull request into its Proposal Branch,
// pinned to the expected head so GitHub refuses a head that moved. A pull
// request already merged at that head returns its merge commit. GitHub's own
// refusals, such as branch protection, come back as the error.
func (b *GitHubBackend) MergePull(ctx context.Context, merge ledger.PullMerge) (string, error) {
	if err := b.requireRepository(); err != nil {
		return "", err
	}
	if merge.Number <= 0 || merge.Branch == "" || merge.Head == "" || merge.Base == "" || merge.Base == "main" {
		return "", workflow.Refuse("a Proposal Branch merge requires the owned pull request, its source branch and head, and a Proposal Branch base")
	}
	pull, err := b.pullForPresentation(ctx, b.repository, merge.Number)
	if err != nil {
		return "", err
	}
	identity := b.repository.Owner + "/" + b.repository.Name
	switch {
	case pull.Head.Ref != merge.Branch || !strings.EqualFold(pull.Head.Repo.FullName, identity) || pull.Base.Ref != merge.Base:
		return "", workflow.Refuse(fmt.Sprintf("pull request #%d does not track %s into %s", merge.Number, merge.Branch, merge.Base))
	case pull.Head.SHA != merge.Head:
		return "", workflow.Refuse(fmt.Sprintf("pull request #%d shows head %s, not the final head %s", merge.Number, pull.Head.SHA, merge.Head))
	case pull.Merged:
		return pull.MergeSHA, nil
	case pull.State != "open":
		return "", workflow.Refuse(fmt.Sprintf("pull request #%d is %s", merge.Number, pull.State))
	}
	var merged struct {
		SHA    string `json:"sha"`
		Merged bool   `json:"merged"`
	}
	path := b.repositoryPath(b.repository) + fmt.Sprintf("/pulls/%d/merge", merge.Number)
	if err := b.request(ctx, http.MethodPut, path, map[string]string{"merge_method": "squash", "sha": merge.Head}, &merged); err != nil {
		return "", err
	}
	if !merged.Merged || merged.SHA == "" {
		return "", fmt.Errorf("GitHub did not confirm the merge of pull request #%d", merge.Number)
	}
	return merged.SHA, nil
}
