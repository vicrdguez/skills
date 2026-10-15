package setup

import (
	"context"
	"fmt"

	"github.com/vicrdguez/skills/ledger"
	"github.com/vicrdguez/skills/workflow"
)

var _ ledger.ReadinessForge = (*GitHubBackend)(nil)

// ReadyPull reconciles native readiness without publishing or copying prose,
// creating an attachment, or correcting concurrent external changes.
func (b *GitHubBackend) ReadyPull(ctx context.Context, readiness ledger.PullReadiness) error {
	if err := b.requireRepository(); err != nil {
		return err
	}
	if readiness.Number <= 0 || readiness.Branch == "" || readiness.Head == "" || readiness.Base == "" || readiness.Base == "main" {
		return workflow.Refuse("readiness recovery requires an attached pull request and exact source into a Proposal Branch")
	}
	expected := ledger.PullPresentation{Number: readiness.Number, Branch: readiness.Branch, Head: readiness.Head, Base: readiness.Base, Current: readiness.Current}
	if err := presentationCurrent(readiness.Current); err != nil {
		return err
	}
	pull, err := b.observePresentedPull(ctx, b.repository, readiness.Number, expected)
	if err != nil {
		return err
	}
	if reason := presentedPullMismatch(*pull, b.repository, expected); reason != "" {
		return workflow.Refuse(reason)
	}
	if err := presentationCurrent(readiness.Current); err != nil {
		return err
	}
	if !pull.Draft {
		return nil
	}
	if pull.NodeID == "" {
		return workflow.Refuse("the attached pull request has no stable node identity for readiness recovery")
	}
	if err := b.setPullPresentation(ctx, readiness.Current, pull.NodeID, false); err != nil {
		return fmt.Errorf("%w: readiness mutation was not confirmed: %v", ledger.ErrPresentationUncertain, err)
	}
	confirmed, err := b.pullForPresentation(ctx, b.repository, readiness.Number)
	if err != nil {
		return fmt.Errorf("%w: readiness readback unavailable: %v", ledger.ErrPresentationUncertain, err)
	}
	if reason := presentedPullMismatch(*confirmed, b.repository, expected); reason != "" {
		return fmt.Errorf("%w: %s", ledger.ErrPresentationUncertain, reason)
	}
	if confirmed.NodeID != pull.NodeID || confirmed.Draft {
		return fmt.Errorf("%w: attached pull request readiness was not confirmed", ledger.ErrPresentationUncertain)
	}
	return presentationCurrent(readiness.Current)
}
