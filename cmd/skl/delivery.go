package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/urfave/cli/v2"
	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/ledger"
	"github.com/vicrdguez/skills/setup"
	"github.com/vicrdguez/skills/workflow"
)

// Delivery commands have one authority: the configured private ledger. The
// forge factory is invoked only after a locally committed handoff.
func deliveryCommands(phase string, newBackend backendFactory, stdout io.Writer) []*cli.Command {
	names := []string{"next", "resume", "prepare", "inspect", "release", "submit"}
	if phase == ledger.ImplementPhase {
		names = append(names, "needs-human")
	}
	var commands []*cli.Command
	for _, name := range names {
		command := &cli.Command{Name: name, Flags: []cli.Flag{
			&cli.PathFlag{Name: "repo", Value: "."}, &cli.StringFlag{Name: "remote"},
			&cli.StringFlag{Name: "item"}, &cli.StringFlag{Name: "claim"},
			&cli.StringFlag{Name: "head"}, &cli.StringFlag{Name: "target"},
			&cli.StringFlag{Name: "outcome"}, &cli.PathFlag{Name: "body"}, &cli.PathFlag{Name: "public-body"}, &cli.PathFlag{Name: "result-directory"},
			implementationFormatFlag(), implementationCapabilityFlag(),
		}, Action: func(c *cli.Context) error { return runDelivery(c, phase, name, newBackend, stdout) }}
		if name == "next" {
			command.Flags = append(command.Flags, waitFlags()...)
			command.Aliases = []string{"start"}
		}
		commands = append(commands, command)
	}
	return commands
}

type deliveryOutput struct {
	Status    string                   `json:"status"`
	Reason    string                   `json:"reason,omitempty"`
	Repair    string                   `json:"repair,omitempty"`
	Execution *ledger.Execution        `json:"execution,omitempty"`
	Source    *workflow.DeliverySource `json:"source,omitempty"`
	Result    *ledger.DeliveryResult   `json:"result,omitempty"`
	Packet    *skilldist.Packet        `json:"packet,omitempty"`
}

func runDelivery(c *cli.Context, phase, operation string, newBackend backendFactory, stdout io.Writer) error {
	format, err := implementationFormat(c.String("format"))
	if err != nil {
		return err
	}
	capability, err := implementationCapability(c.String("capability"))
	if err != nil {
		return err
	}
	emit := func(out deliveryOutput) error { return renderDelivery(stdout, format, phase, operation, out) }
	refusal := func(err error) error {
		return emit(deliveryOutput{Status: "fix_required", Reason: err.Error(), Repair: "preserve source progress and Result Documents; use the exact existing Claim reference when retrying or resuming"})
	}
	if c.NArg() != 0 {
		return refusal(fmt.Errorf("delivery commands take flags, not positional arguments"))
	}
	item, claim := c.String("item"), c.String("claim")
	if operation == "next" {
		if item != "" || claim != "" {
			return refusal(fmt.Errorf("next selects its own project-scoped Work Item; resume an existing reservation explicitly"))
		}
	} else if item == "" || claim == "" {
		return refusal(fmt.Errorf("%s requires --item <proposal>/<slice> and the exact --claim <acquisition-commit>", operation))
	}
	repository, err := setup.ResolveRepository(c.Path("repo"), c.String("remote"))
	if err != nil {
		return refusal(err)
	}
	store, err := openConfiguredLedger()
	if err != nil {
		return refusal(err)
	}
	if err := store.RefuseSourceOverlap(repository.Root); err != nil {
		return refusal(err)
	}
	if operation == "submit" || operation == "needs-human" {
		return submitDelivery(c, phase, operation, repository, store, newBackend, emit, refusal)
	}
	var execution *ledger.Execution
	if operation == "next" {
		outcome, err := nextWork(c.Context, c.Duration("wait"), c.Duration("poll"), func() (workflow.ImplementationOutcome, error) {
			var err error
			execution, err = ledger.StartDeliveryContext(c.Context, store, repository.Repository, phase)
			status := "no_work"
			if execution != nil {
				status = "work_available"
			}
			return workflow.ImplementationOutcome{Status: status}, err
		})
		if err != nil {
			if c.Context.Err() != nil {
				return err
			}
			return refusal(err)
		}
		if execution == nil {
			return emit(deliveryOutput{Status: outcome.Status, Reason: outcome.Reason})
		}
	} else if operation == "release" {
		if err := ledger.ReleaseDelivery(store, repository.Repository, item, phase, claim); err != nil {
			return refusal(err)
		}
		return emit(deliveryOutput{Status: "released", Reason: "the exact reservation was released; source progress and lifecycle eligibility were preserved"})
	} else {
		execution, err = ledger.ResumeDelivery(store, repository.Repository, item, phase, claim)
		if err != nil {
			return refusal(err)
		}
	}
	var source *workflow.DeliverySource
	required, target, previous := deliverySourceInputs(execution, phase)
	if operation == "prepare" {
		observed, err := workflow.PrepareDeliverySource(repository.Root, repository.Remote, execution.State.Branch, required, target)
		if err != nil {
			return refusal(err)
		}
		source = &observed
	} else if operation == "inspect" {
		if phase == ledger.ImplementPhase && c.String("target") != "" {
			target = c.String("target")
		}
		observed, err := workflow.InspectDeliverySource(repository.Root, execution.State.Branch, requiredForInspection(phase, required), target, previous)
		if err != nil {
			return refusal(err)
		}
		source = &observed
	}
	packet, err := setup.PresentDelivery(execution, repository, phase, operation, capability, source, c.Path("result-directory"))
	if err != nil {
		return refusal(fmt.Errorf("Claim %s remains acquired for %s, but execution rendering failed: %w", execution.Claim.Commit, execution.Item, err))
	}
	return emit(deliveryOutput{Status: map[string]string{"next": "work_available", "resume": "work_available", "prepare": "prepared", "inspect": "inspected"}[operation], Execution: execution, Source: source, Packet: &packet})
}

func requiredForInspection(phase, head string) string {
	if phase == ledger.WatchdogPhase {
		return head
	}
	return ""
}

func deliverySourceInputs(e *ledger.Execution, phase string) (head, target, previous string) {
	if e.Implement != nil {
		head, target = e.Implement.Source.Head, e.Implement.Source.Target
	}
	if e.Watchdog != nil {
		previous = e.Watchdog.Source.Reviewed
	}
	return
}

func submitDelivery(c *cli.Context, phase, operation string, repository setup.RepositoryContext, store *ledger.Store, newBackend backendFactory, emit func(deliveryOutput) error, refusal func(error) error) error {
	body, err := os.ReadFile(c.Path("body"))
	if err != nil {
		return refusal(fmt.Errorf("read private phase Result Document: %w", err))
	}
	var public *string
	if c.Path("public-body") != "" {
		contents, err := os.ReadFile(c.Path("public-body"))
		if err != nil {
			return refusal(fmt.Errorf("read separately authored public Result Document: %w", err))
		}
		text := string(contents)
		public = &text
	}
	outcome := ledger.AwaitingReview
	if operation == "needs-human" {
		outcome = ledger.NeedsHuman
	} else if phase == ledger.WatchdogPhase {
		outcome = strings.ReplaceAll(c.String("outcome"), "-", "_")
		if outcome != "pass" && outcome != ledger.Rework && outcome != ledger.NeedsHuman {
			return refusal(fmt.Errorf("watchdog submit requires --outcome pass, rework, or needs-human"))
		}
	} else if c.IsSet("outcome") {
		return refusal(fmt.Errorf("implementation uses submit or needs-human, not a second outcome flag"))
	}
	e, activeErr := ledger.ResumeDelivery(store, repository.Repository, c.String("item"), phase, c.String("claim"))
	source := ledger.SourceRevisions{Head: c.String("head"), Target: c.String("target")}
	if activeErr == nil {
		if phase == ledger.WatchdogPhase {
			if e.Implement == nil {
				return refusal(fmt.Errorf("review requires its fixed implementation report"))
			}
			source.Reviewed = e.Implement.Source.Head
			source.Target = e.Implement.Source.Target
			if source.Head == "" {
				source.Head = source.Reviewed
			}
			if c.IsSet("target") && c.String("target") != source.Target {
				return refusal(fmt.Errorf("review cannot replace its fixed Integration Target"))
			}
		}
		if source.Head != "" || source.Target != "" || outcome != ledger.NeedsHuman || phase == ledger.WatchdogPhase {
			if err := workflow.ValidateDeliverySource(repository.Root, e.State.Branch, source.Head, source.Target, source.Reviewed, outcome == "pass"); err != nil {
				return refusal(err)
			}
		} else if err := workflow.ValidateUnpreparedPause(repository.Root, e.State.Branch); err != nil {
			return refusal(err)
		}
	} else {
		// No current Claim is not success. The ledger's exact current-report replay
		// check alone can establish a completed effect, without touching later work.
		if phase == ledger.WatchdogPhase {
			report, _, err := ledger.CurrentReport(store, repository.Repository, c.String("item"), phase)
			if err != nil {
				return refusal(activeErr)
			}
			source.Reviewed = report.Source.Reviewed
			if source.Head == "" {
				source.Head = source.Reviewed
			}
			if source.Target == "" {
				source.Target = report.Source.Target
			}
		}
	}
	result, err := ledger.HandoffDelivery(store, repository.Repository, c.String("item"), phase, c.String("claim"), source, outcome, string(body))
	if err != nil {
		return refusal(err)
	}
	var forge ledger.DeliveryForge
	if public != nil {
		backend, err := newBackend(repository.Repository)
		if err != nil {
			result.Publication = &ledger.PublicationNote{Status: ledger.IssuePending, Detail: "forge construction unavailable: " + err.Error()}
		} else {
			forge, _ = backend.(ledger.DeliveryForge)
		}
	}
	ledger.PublishDelivery(c.Context, store, repository.Repository, repository.Root, repository.Remote, result, public, forge)
	ledger.ReplicateDelivery(store, repository.Repository, result)
	return emit(deliveryOutput{Status: result.Status, Result: result})
}

func renderDelivery(stdout io.Writer, format implementationFormatKind, phase, operation string, out deliveryOutput) error {
	var err error
	if format == formatJSON {
		err = json.NewEncoder(stdout).Encode(out)
	} else if out.Packet != nil {
		_, err = fmt.Fprint(stdout, out.Packet.Instructions)
	} else {
		_, err = fmt.Fprintf(stdout, "Status: %s\n", out.Status)
		if err == nil && out.Reason != "" {
			_, err = fmt.Fprintln(stdout, out.Reason)
		}
		if err == nil && out.Repair != "" {
			_, err = fmt.Fprintln(stdout, "Repair:", out.Repair)
		}
		if err == nil && out.Result != nil {
			result := out.Result
			_, err = fmt.Fprintf(stdout, "Work Item: %s\nPhase Report: %s at %s\nClaim: released by the local handoff; human merge remains separate.\n", result.Item, result.Report.Path, result.Report.Commit)
			for label, note := range map[string]*ledger.PublicationNote{"Ledger replication": result.Replication, "Public presentation": result.Publication} {
				if err == nil && note != nil {
					_, err = fmt.Fprintf(stdout, "%s: %s — %s\n", label, note.Status, note.Detail)
				}
			}
		}
	}
	if err != nil {
		return fmt.Errorf("%s %s established status %s but output delivery failed; preserve fixed inputs and explicitly resume or retry the same handoff rather than selecting another Claim: %w", phase, operation, out.Status, err)
	}
	return nil
}
