package setup

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/vicrdguez/skills/ledger"
)

var _ ledger.CompletionForge = (*GitHubBackend)(nil)

// ObserveSubmission reads one ledger-attached PR. Neither issue history nor
// public content participates in the identity check or terminal verdict.
func (b *GitHubBackend) ObserveSubmission(ctx context.Context, attachment ledger.ForgeAttachment, target ledger.IntegrationTarget, branch string) (ledger.SubmissionObservation, error) {
	if err := b.requireRepository(); err != nil {
		return ledger.SubmissionObservation{}, err
	}
	identity := b.repository.Owner + "/" + b.repository.Name
	if attachment.Number <= 0 || !strings.EqualFold(attachment.Repository, identity) || !strings.EqualFold(target.Repository, identity) || target.Branch == "" || branch == "" {
		return ledger.SubmissionObservation{}, fmt.Errorf("owned Submission and Integration Target attachments are incomplete or inconsistent")
	}
	var pull struct {
		Number      int     `json:"number"`
		State       string  `json:"state"`
		Merged      *bool   `json:"merged"`
		MergedAt    *string `json:"merged_at"`
		MergeCommit string  `json:"merge_commit_sha"`
		Head        struct {
			Ref  string `json:"ref"`
			SHA  string `json:"sha"`
			Repo struct {
				FullName string `json:"full_name"`
			} `json:"repo"`
		} `json:"head"`
		Base struct {
			Ref  string `json:"ref"`
			Repo struct {
				FullName string `json:"full_name"`
			} `json:"repo"`
		} `json:"base"`
	}
	path := b.repositoryPath(b.repository) + fmt.Sprintf("/pulls/%d", attachment.Number)
	if err := b.request(ctx, http.MethodGet, path, nil, &pull); err != nil {
		return ledger.SubmissionObservation{}, fmt.Errorf("owned Submission %s#%d is unavailable: %w", attachment.Repository, attachment.Number, err)
	}
	if pull.Number != attachment.Number || !strings.EqualFold(pull.Head.Repo.FullName, attachment.Repository) || !strings.EqualFold(pull.Base.Repo.FullName, target.Repository) || pull.Base.Ref != target.Branch || pull.Head.Ref != branch {
		return ledger.SubmissionObservation{}, fmt.Errorf("owned Submission %s#%d has mismatched or unknown source/target identity; inspect the exact attachment", attachment.Repository, attachment.Number)
	}
	if pull.Merged == nil || pull.Head.SHA == "" || pull.State != "open" && pull.State != "closed" {
		return ledger.SubmissionObservation{}, fmt.Errorf("owned Submission %s#%d has incomplete lifecycle or source revision evidence", attachment.Repository, attachment.Number)
	}
	if pull.State == "open" {
		if *pull.Merged || pull.MergedAt != nil && *pull.MergedAt != "" {
			return ledger.SubmissionObservation{}, fmt.Errorf("owned Submission %s#%d has contradictory open/merged evidence", attachment.Repository, attachment.Number)
		}
		return ledger.SubmissionObservation{State: "open"}, nil
	}
	if *pull.Merged {
		if pull.MergedAt == nil || *pull.MergedAt == "" {
			return ledger.SubmissionObservation{}, fmt.Errorf("owned Submission %s#%d has incomplete merge confirmation", attachment.Repository, attachment.Number)
		}
		return ledger.SubmissionObservation{State: ledger.Merged, SourceHead: pull.Head.SHA, MergeCommit: pull.MergeCommit}, nil
	}
	if pull.MergedAt != nil && *pull.MergedAt != "" {
		return ledger.SubmissionObservation{}, fmt.Errorf("owned Submission %s#%d has contradictory closure evidence", attachment.Repository, attachment.Number)
	}
	return ledger.SubmissionObservation{State: "closed", SourceHead: pull.Head.SHA}, nil
}
