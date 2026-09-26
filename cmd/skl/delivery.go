package main

import (
	"encoding/json"
	"errors"
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
			implementationFormatFlag(),
		}, Action: func(c *cli.Context) error { return runDelivery(c, phase, name, newBackend, stdout) }}
		if name == "next" {
			command.Flags = append(command.Flags, waitFlags()...)
			command.Flags = append(command.Flags, dispatchFlags()...)
			command.Aliases = []string{"start"}
		}
		if name == "resume" {
			command.Flags = append(command.Flags, &cli.BoolFlag{Name: "dispatched", Usage: "render the Execution Skill a Dispatch of this Claim stands for"})
		}
		if phase == ledger.ImplementPhase && (name == "next" || name == "resume") {
			command.Flags = append(command.Flags, implementFlags()...)
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
	// Previous is how a continued Dispatch's Claim ended; Dispatch is the
	// Claim this Dispatch made.
	Previous *ledger.ClaimEnding `json:"previous,omitempty"`
	Dispatch *dispatched         `json:"dispatch,omitempty"`
	// Present continues an unpresented handoff with the then-current result.
	Present string `json:"present,omitempty"`
	// kind and facts select and supply the Markdown Outcome Instruction.
	kind  string
	facts any
}

func runDelivery(c *cli.Context, phase, operation string, newBackend backendFactory, stdout io.Writer) error {
	format, err := implementationFormat(c.String("format"))
	if err != nil {
		return err
	}
	emit := func(out deliveryOutput) error { return renderDelivery(stdout, format, phase, operation, out) }
	item, claim := c.String("item"), c.String("claim")
	// Every refusal before acquisition leaves the worker without a Claim. A
	// named Claim is reported kept only once the ledger confirms it current;
	// until then the refusal only leaves it unchanged.
	claimState := claimUnchanged
	if operation == "next" {
		claimState = claimNone
	}
	verified := func() { claimState = claimKept }
	statusCmd := ""
	// A Dispatch ends its Supervisor's lane on every refusal: a rerun could
	// replay a continuation or hide an interrupted worker.
	dispatch := operation == "next" && c.Bool("dispatch")
	var continued *ledger.ClaimEnding
	refusal := func(err error) error {
		reason, repair := refusalParts(err)
		facts := refusalFacts{Status: "fix_required", Reason: reason, Repair: repair, ClaimState: claimState, Claim: claim, StatusCommand: statusCmd, Rerun: boundCommand(c, "skl "+phase+" "+operation), Stop: dispatch, Previous: continued}
		return emit(deliveryOutput{Status: "fix_required", Reason: err.Error(), Repair: deliveryJSONRepair, Previous: continued, kind: "refused", facts: facts})
	}
	if c.NArg() != 0 {
		return refusal(fmt.Errorf("delivery commands take flags, not positional arguments"))
	}
	if operation == "next" {
		if item != "" || claim != "" {
			return refusal(fmt.Errorf("next selects its own project-scoped Work Item; resume an existing reservation explicitly"))
		}
		if !dispatch && (c.IsSet("after") || c.IsSet("worker-model") || c.IsSet("worker-thinking")) {
			return refusal(fmt.Errorf("--after, --worker-model and --worker-thinking apply only to --dispatch"))
		}
	} else if item == "" || claim == "" {
		return refusal(fmt.Errorf("%s requires --item <proposal>/<slice> and the exact --claim <acquisition-commit>", operation))
	}
	choices, err := implementChoices(c)
	if err != nil {
		return refusal(err)
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
		return submitDelivery(c, phase, operation, repository, store, newBackend, emit, refusal, verified)
	}
	var execution *ledger.Execution
	if operation == "next" {
		// A continuation passes only after its dispatched Claim's handoff,
		// established before anything is selected, claimed or awaited.
		if dispatch && c.IsSet("after") {
			ending, err := ledger.ClaimEndingOf(store, repository.Repository, phase, c.String("after"))
			if err != nil {
				return refusal(err)
			}
			if !ending.Handoff() {
				return emit(stoppedOutput(phase, repository, ending))
			}
			continued = &ending
		}
		selection, err := nextWork(c.Context, c.Duration("wait"), c.Duration("poll"), func() (ledger.Selection, error) {
			execution, err := ledger.StartDeliveryContext(c.Context, store, repository.Repository, phase)
			if execution == nil {
				return ledger.Selection{Status: ledger.NoWork}, err
			}
			return ledger.Selection{Status: ledger.WorkAvailable, Execution: execution}, err
		})
		execution = selection.Execution
		if err != nil {
			if c.Context.Err() != nil {
				if format == formatMarkdown {
					if err := writeOutcome(stdout, "interrupted", phaseFacts{Phase: phase, StatusCommand: statusInvocation(repository), Previous: continued}); err != nil {
						return err
					}
				}
				return err
			}
			var refused *ledger.Refusal
			if !errors.As(err, &refused) {
				// Selection can fail after its acquisition commit.
				claimState, statusCmd = claimUncertain, statusInvocation(repository)
			}
			return refusal(err)
		}
		if execution == nil {
			return emit(deliveryOutput{Status: selection.Status, Reason: selectionReasons[selection.Status], Previous: continued, kind: selectionKinds[selection.Status], facts: phaseFacts{Status: selection.Status, Phase: phase, Previous: continued}})
		}
		if dispatch {
			return emit(dispatchOutput(c, phase, repository, execution, continued))
		}
	} else if operation == "release" {
		if err := ledger.ReleaseDelivery(store, repository.Repository, item, phase, claim); err != nil {
			return refusal(err)
		}
		return emit(deliveryOutput{Status: "released", Reason: "the exact reservation was released; source progress and lifecycle eligibility were preserved", kind: "released", facts: releasedFacts{Status: "released", Phase: phase, Item: item, Claim: claim, Next: nextInvocation(phase, repository)}})
	} else {
		execution, err = ledger.ResumeDelivery(store, repository.Repository, item, phase, claim)
		if err != nil {
			return refusal(err)
		}
		verified()
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
	// A dispatched worker receives what next would have rendered for its Claim.
	rendered := operation
	if operation == "resume" && c.Bool("dispatched") {
		rendered = "next"
	}
	packet, err := setup.PresentDelivery(execution, repository, phase, rendered, source, c.Path("result-directory"), choices)
	if err != nil {
		return emit(renderingFailedOutput(phase, repository, execution.Item, execution.Claim.Commit, err))
	}
	return emit(deliveryOutput{Status: map[string]string{"next": ledger.WorkAvailable, "resume": ledger.WorkAvailable, "prepare": "prepared", "inspect": "inspected"}[operation], Execution: execution, Source: source, Packet: &packet})
}

// implementFlags choose Implement's mode and the opaque subagent values its
// Execution Skill renders.
func implementFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "mode", Usage: "Implement mode: standard (default) or team"},
		&cli.StringFlag{Name: "helper-model", Usage: "opaque model for team-mode implementer subagents"},
		&cli.StringFlag{Name: "helper-thinking", Usage: "opaque thinking level for team-mode implementer subagents"},
		&cli.StringFlag{Name: "reviewer-model", Usage: "opaque model for both Audit reviewers"},
		&cli.StringFlag{Name: "reviewer-thinking", Usage: "opaque thinking level for both Audit reviewers"},
	}
}

// implementChoices validates the Implement mode before anything is claimed.
// An empty value counts as omitted, so an adapter can pass an unfilled slot.
func implementChoices(c *cli.Context) (setup.ImplementChoices, error) {
	choices := setup.ImplementChoices{
		Mode:     c.String("mode"),
		Helper:   skilldist.SubagentChoice{Model: c.String("helper-model"), Thinking: c.String("helper-thinking")},
		Reviewer: skilldist.SubagentChoice{Model: c.String("reviewer-model"), Thinking: c.String("reviewer-thinking")},
	}
	switch choices.Mode {
	case "", skilldist.StandardMode:
		if choices.Helper != (skilldist.SubagentChoice{}) {
			return choices, fmt.Errorf("--helper-model and --helper-thinking apply only to --mode team")
		}
	case skilldist.TeamMode:
	default:
		return choices, fmt.Errorf("unknown Implement mode %q; use standard or team", choices.Mode)
	}
	return choices, nil
}

// deliveryJSONRepair is the repair the JSON transport has always carried for
// a delivery refusal. Markdown shows each refusal's own repair instead.
const deliveryJSONRepair = "preserve source progress and Result Documents; use the exact existing Claim reference when retrying or resuming"

// selectionKinds maps an empty selection's status to its outcome kind.
var selectionKinds = map[string]string{ledger.NoWork: "no-work", ledger.IdleTimeout: "idle-timeout"}

// selectionReasons are the JSON reasons of empty selections.
var selectionReasons = map[string]string{ledger.IdleTimeout: "no claimable work in this queue during the idle window; not global completion"}

// renderingFailedOutput reports an acquired Claim whose Execution Skill failed
// to render.
func renderingFailedOutput(phase string, repository setup.RepositoryContext, item, claim string, err error) deliveryOutput {
	return deliveryOutput{
		Status: "fix_required", Reason: fmt.Sprintf("Claim %s remains acquired for %s, but execution rendering failed: %v", claim, item, err),
		Repair: deliveryJSONRepair,
		kind:   "rendering-failed",
		facts: renderingFailedFacts{Status: "fix_required", Phase: phase, Item: item, Claim: claim, Reason: err.Error(),
			Resume: claimCommand(phase, "resume", repository, item, claim), Release: claimCommand(phase, "release", repository, item, claim)},
	}
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

func submitDelivery(c *cli.Context, phase, operation string, repository setup.RepositoryContext, store *ledger.Store, newBackend backendFactory, emit func(deliveryOutput) error, refusal func(error) error, verified func()) error {
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
		verified()
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
		if phase == ledger.ImplementPhase && outcome == ledger.NeedsHuman {
			if source.Head == "" && source.Target == "" {
				if err := workflow.ValidateUnpreparedPause(repository.Root, e.State.Branch); err != nil {
					return refusal(err)
				}
			} else if err := workflow.ValidatePausedDeliverySource(repository.Root, e.State.Branch, source.Head, source.Target); err != nil {
				return refusal(err)
			}
		} else if err := workflow.ValidateDeliverySource(repository.Root, e.State.Branch, source.Head, source.Target, source.Reviewed, outcome == "pass"); err != nil {
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
	return emit(handoffOutput(phase, repository, result))
}

// handoffOutput is the outcome of one committed handoff: submitted, or paused
// for a human decision.
func handoffOutput(phase string, repository setup.RepositoryContext, result *ledger.DeliveryResult) deliveryOutput {
	out := deliveryOutput{Status: result.Status, Result: result, kind: "submitted"}
	if result.Status == ledger.NeedsHuman {
		out.kind = "paused"
	}
	if result.Publication == nil || result.Publication.Status != ledger.PullPresented {
		out.Present = presentInvocation(repository, result.Item)
	}
	out.facts = handoffFacts{Status: result.Status, Phase: phase, Item: result.Item, Report: result.Report, AlreadyCompleted: result.AlreadyCompleted,
		Notes: notesOf(result.Replication, result.Publication), Present: out.Present}
	return out
}

func renderDelivery(stdout io.Writer, format implementationFormatKind, phase, operation string, out deliveryOutput) error {
	var err error
	if format == formatJSON {
		err = json.NewEncoder(stdout).Encode(out)
	} else if out.Packet != nil {
		_, err = fmt.Fprint(stdout, out.Packet.Instructions)
	} else {
		err = writeOutcome(stdout, out.kind, out.facts)
	}
	if err != nil {
		return fmt.Errorf("%s %s established status %s, but writing its output failed: %w", phase, operation, out.Status, err)
	}
	return nil
}
