package main

import (
	"context"
	"fmt"
	"strconv"

	"github.com/vicrdguez/skills/workflow"
)

type implementationMemory struct {
	memoryBackend
	coordination           []workflow.CoordinationItem
	work                   []workflow.ImplementationItem
	beforeReviewSubmission func(int)
	reviewSubmissionCalls  int
	reviewClock            int
}

func (b *implementationMemory) reviewTime() string {
	b.reviewClock++
	return fmt.Sprintf("2026-01-01T00:00:%02dZ", b.reviewClock)
}

// Expand concise initial fixtures into separate records. Once a mutation is made,
// the stored observations, not the derived Work Item State, remain authoritative.
func implementationFixture(item workflow.ImplementationItem) workflow.ImplementationItem {
	if item.Source != nil {
		return item
	}
	item.Source = &workflow.LifecycleObservation{Open: true, Claimed: item.Claimed}
	if item.State != "" {
		item.Source.States = []workflow.State{item.State}
	}
	if item.Submission != nil {
		submission := *item.Submission
		if submission.Lifecycle == nil {
			state := submission.State
			claimed := submission.Claimed
			if item.State != workflow.Ready {
				item.Source.States, item.Source.Claimed = nil, false
				if state == "" {
					state = item.State
				}
				claimed = claimed || item.Claimed
			}
			submission.Lifecycle = &workflow.LifecycleObservation{Open: true, Claimed: claimed}
			if state != "" {
				submission.Lifecycle.States = []workflow.State{state}
			}
			submission.State, submission.Claimed = state, claimed
		}
		item.Submission = &submission
	}
	return item
}

func (b *implementationMemory) ImplementationItems(context.Context) ([]workflow.ImplementationItem, error) {
	items := append([]workflow.ImplementationItem(nil), b.work...)
	for i := range items {
		items[i] = workflow.ReconcileImplementation(implementationFixture(items[i]))
		if number, err := strconv.Atoi(string(items[i].ID)); err == nil {
			if items[i].Order == 0 {
				items[i].Order = number
			}
		}
	}
	return items, nil
}
