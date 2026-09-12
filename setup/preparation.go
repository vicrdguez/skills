package setup

import "context"

type Label struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

var WorkflowLabels = []Label{
	{Name: "sync", Color: "fbca04", Description: "Synchronization Rework; does not consume the finding bounce"},
	{Name: "ready", Color: "0e8a16", Description: "proposed change awaiting an implementor"},
	{Name: "wip", Color: "fbca04", Description: "additive Worker Claim. An agent is working on it"},
	{Name: "review", Color: "1d76db", Description: "built change awaiting a reviewer"},
	{Name: "rework", Color: "d93f0b", Description: "reviewer bounced it back to the implementor after review"},
	{Name: "needs-human", Color: "b60205", Description: "automation paused for a narrow human decision"},
	{Name: "done", Color: "5319e7", Description: "passed review, awaiting the human's approval to merge"},
}

func (b *GitHubBackend) Validate(ctx context.Context) (string, error) {
	if err := b.requireRepository(); err != nil {
		return "", err
	}
	return b.validate(ctx, b.repository)
}

func (b *GitHubBackend) Prepare(ctx context.Context) error {
	if err := b.requireRepository(); err != nil {
		return err
	}
	return b.EnsureLabels(ctx, b.repository, WorkflowLabels)
}
