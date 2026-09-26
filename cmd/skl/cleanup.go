package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

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
			rerun := boundCommand(command, "skl propose cleanup")
			if failure != nil {
				failure.rerun = rerun
				return renderLedgerOutcome(stdout, format, *failure)
			}
			if store != nil {
				return renderCleanup(stdout, format, rerun, ledgerCleanup(command, store, repository))
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
				return renderCleanup(stdout, format, "", cleanupOutcome{Status: cleanupStatus(false, len(outcome.Removed) > 0), Source: &outcome})
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

// cleanupFacts are one cleanup outcome and the invocation that reruns it.
type cleanupFacts struct {
	cleanupOutcome
	Rerun string
}

func renderCleanup(stdout io.Writer, format implementationFormatKind, rerun string, outcome cleanupOutcome) error {
	if format == formatJSON {
		return json.NewEncoder(stdout).Encode(outcome)
	}
	return writeOutcome(stdout, "cleanup", cleanupFacts{outcome, rerun})
}
