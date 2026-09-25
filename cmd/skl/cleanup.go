package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/urfave/cli/v2"
	"github.com/vicrdguez/skills/ledger"
	"github.com/vicrdguez/skills/setup"
	"github.com/vicrdguez/skills/workflow"
)

// cleanupOutcome keeps ledger archival and source cleanup as independent
// results: either may succeed while the other preserves work or needs repair.
type cleanupOutcome struct {
	Status        string                   `json:"status"`
	Archive       *ledger.ArchiveResult    `json:"archive,omitempty"`
	ArchiveRepair *ledgerOutcome           `json:"archive_repair,omitempty"`
	Source        *workflow.CleanupOutcome `json:"source,omitempty"`
	SourceRepair  *ledgerOutcome           `json:"source_repair,omitempty"`
}

func cleanupCommand(newBackend backendFactory, stdout io.Writer) *cli.Command {
	return &cli.Command{
		Name:  "cleanup",
		Usage: "Archive terminal, unclaimed proposals and remove only safe merged local source work",
		Flags: []cli.Flag{
			&cli.PathFlag{Name: "repo"},
			&cli.StringFlag{Name: "remote"},
			implementationFormatFlag(),
		},
		Action: func(command *cli.Context) error {
			format, err := implementationFormat(command.String("format"))
			if err != nil {
				return err
			}
			remote := command.String("remote")
			if remote == "" {
				remote = "origin"
			}
			repository, err := setup.ResolveRepository(command.Path("repo"), remote)
			if err != nil {
				return err
			}
			store, failure, err := adoptedLedger(repository.Repository)
			if err != nil {
				return err
			}
			if failure != nil {
				return renderLedgerOutcome(stdout, format, *failure)
			}
			if store != nil {
				return renderCleanup(stdout, format, ledgerCleanup(command, store, repository))
			}
			backend, err := newBackend(repository.Repository)
			if err != nil {
				return err
			}
			proposalBackend, ok := backend.(workflow.Backend)
			if !ok {
				return fmt.Errorf("workflow backend does not support proposal cleanup")
			}
			outcome, err := workflow.Cleanup(command.Context, repository.Root, proposalBackend)
			if err != nil {
				return err
			}
			if format == formatJSON {
				return renderCleanup(stdout, format, cleanupOutcome{Status: cleanupStatus(false, len(outcome.Removed) > 0), Source: &outcome})
			}
			for _, slug := range outcome.Removed {
				fmt.Fprintln(stdout, "removed", slug)
			}
			for _, preserved := range outcome.Preserved {
				fmt.Fprintln(stdout, "preserved", preserved.Branch)
			}
			return nil
		},
	}
}

// ledgerCleanup archives first and then evaluates source work from committed
// ledger facts. A refused or partial archive grants no deletion authority, and
// a source failure never undoes a committed archive.
func ledgerCleanup(command *cli.Context, store *ledger.Store, repository setup.RepositoryContext) cleanupOutcome {
	var outcome cleanupOutcome
	archive, err := ledger.ArchiveTerminalProposals(command.Context, store, repository.Repository)
	if err != nil {
		outcome.ArchiveRepair = cleanupRepair(err)
	}
	outcome.Archive = archive
	records, err := ledger.TerminalSourceWork(store, repository.Repository)
	if err == nil {
		var candidates []workflow.SourceCandidate
		for _, record := range records {
			candidates = append(candidates, workflow.SourceCandidate{Branch: record.Branch, AcceptedHead: record.AcceptedHead, Hold: record.Hold})
		}
		var source workflow.CleanupOutcome
		source, err = workflow.CleanupSource(repository.Root, candidates)
		if err == nil {
			outcome.Source = &source
		}
	}
	if err != nil {
		outcome.SourceRepair = cleanupRepair(err)
	}
	needsRepair := outcome.ArchiveRepair != nil || outcome.SourceRepair != nil ||
		(archive != nil && len(archive.Repairs) > 0) || (outcome.Source != nil && len(outcome.Source.Failed) > 0)
	archived := archive != nil && len(archive.Archived) > 0
	outcome.Status = cleanupStatus(needsRepair, archived || (outcome.Source != nil && len(outcome.Source.Removed) > 0))
	return outcome
}

// cleanupStatus never lets completed work hide a repair or failed removal.
func cleanupStatus(needsRepair, didWork bool) string {
	switch {
	case needsRepair:
		return "fix_required"
	case didWork:
		return "completed"
	}
	return "no_work"
}

func cleanupRepair(err error) *ledgerOutcome {
	var refusal *ledger.Refusal
	if errors.As(err, &refusal) {
		return &ledgerOutcome{Status: "fix_required", Reason: refusal.Invariant, Repair: refusal.Repair}
	}
	return &ledgerOutcome{Status: "fix_required", Reason: err.Error()}
}

func renderCleanup(stdout io.Writer, format implementationFormatKind, outcome cleanupOutcome) error {
	if format == formatJSON {
		return json.NewEncoder(stdout).Encode(outcome)
	}
	var text strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&text, format+"\n", args...) }
	line("Status: %s", outcome.Status)
	repair := func(label string, repair *ledgerOutcome) {
		if repair != nil {
			line("%s: %s", label, repair.Reason)
			if repair.Repair != "" {
				line("  Repair: %s", repair.Repair)
			}
		}
	}
	repair("Archive refused", outcome.ArchiveRepair)
	if archive := outcome.Archive; archive != nil {
		for _, archived := range archive.Archived {
			delivery := "fully delivered"
			if !archived.FullyDelivered {
				delivery = "retired without full delivery"
			}
			line("Archived proposal: %s (%s) at %s", archived.Proposal, delivery, archived.Commit)
			if archived.Resumed {
				line("  Finished an interrupted archive move")
			}
		}
		for _, kept := range archive.Kept {
			line("Kept active proposal: %s: %s", kept.Proposal, kept.Reason)
		}
		for _, kept := range archive.Repairs {
			line("Archive repair for %s: %s", kept.Proposal, kept.Reason)
			line("  Repair: %s", kept.Repair)
		}
		if note := archive.Replication; note != nil {
			if note.Status == ledger.PushPushed {
				line("Ledger replication: %s", note.Status)
			} else {
				line("Ledger replication: %s: %s", note.Status, note.Detail)
			}
		}
	}
	repair("Source cleanup refused", outcome.SourceRepair)
	if source := outcome.Source; source != nil {
		for _, branch := range source.Removed {
			line("Removed local source work: %s", branch)
		}
		for _, preserved := range source.Preserved {
			line("Preserved local source work: %s: %s", preserved.Branch, preserved.Reason)
		}
		for _, failed := range source.Failed {
			line("Source removal failed: %s: %s", failed.Branch, failed.Reason)
		}
	}
	_, err := io.WriteString(stdout, text.String())
	return err
}
