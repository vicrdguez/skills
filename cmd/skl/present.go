package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/urfave/cli/v2"
	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/ledger"
	"github.com/vicrdguez/skills/setup"
)

// presentationGuidance binds what an agent needs to author fresh public prose
// for the current result: exact private evidence, the phase's authoring
// resource, and the continuation command. The CLI authors no public prose.
type presentationGuidance struct {
	Result    ledger.CurrentResult `json:"result"`
	Evidence  []string             `json:"evidence"`
	Authoring string               `json:"authoring"`
	Continue  string               `json:"continue"`
}

// presentCommand presents the current committed result of one Work Item as
// its pull request, independently of any earlier presentation attempt.
func presentCommand(newBackend backendFactory, stdout io.Writer) *cli.Command {
	return &cli.Command{
		Name:  "present",
		Usage: "Present one Work Item's current committed phase result as its pull request",
		Flags: []cli.Flag{
			&cli.PathFlag{Name: "repo", Value: "."},
			&cli.StringFlag{Name: "remote"},
			&cli.StringFlag{Name: "item", Required: true},
			&cli.PathFlag{Name: "public-body", Usage: "Freshly authored public prose for the current result; omit it to receive evidence and authoring guidance"},
			implementationFormatFlag(),
		},
		Action: func(command *cli.Context) error {
			format, err := implementationFormat(command.String("format"))
			if err != nil {
				return err
			}
			rerun := boundCommand(command, "skl ledger present")
			refuse := func(err error) error { return refuseLedger(stdout, format, rerun, err) }
			if command.NArg() != 0 {
				return refuse(fmt.Errorf("ledger present takes flags, not arguments"))
			}
			repository, err := setup.ResolveRepository(command.Path("repo"), command.String("remote"))
			if err != nil {
				return refuse(err)
			}
			store, err := openConfiguredLedger()
			if err != nil {
				return refuse(err)
			}
			if err := store.RefuseSourceOverlap(repository.Root); err != nil {
				return refuse(err)
			}
			selected, err := ledger.SelectCurrentResult(store, repository.Repository, command.String("item"))
			if err != nil {
				return refuse(err)
			}
			if command.Path("public-body") == "" {
				return renderPresentation(stdout, format, ledgerOutcome{Status: "prose_required", Reason: "no public prose was supplied; author it for the current result from the private evidence below", Guidance: presentGuidance(repository, selected)})
			}
			body, err := os.ReadFile(command.Path("public-body"))
			if err != nil {
				return refuse(fmt.Errorf("read the freshly authored public prose: %w", err))
			}
			backend, err := newBackend(repository.Repository)
			if err != nil {
				note := ledger.PublicationNote{Status: ledger.IssuePending, Detail: "forge construction unavailable: " + err.Error()}
				return renderPresentation(stdout, format, ledgerOutcome{Status: note.Status, Presentation: &ledger.Presentation{Result: selected, Publication: note}, Guidance: presentGuidance(repository, selected)})
			}
			forge, ok := backend.(ledger.DeliveryForge)
			if !ok {
				return refuse(fmt.Errorf("workflow backend does not support pull request presentation"))
			}
			presentation := ledger.PresentCurrent(command.Context, store, repository.Repository, repository.Root, repository.Remote, selected, string(body), forge)
			outcome := ledgerOutcome{Status: presentation.Publication.Status, Presentation: presentation}
			if presentation.Publication.Status != ledger.PullPresented {
				outcome.Guidance = presentGuidance(repository, presentation.Result)
			}
			return renderPresentation(stdout, format, outcome)
		},
	}
}

func presentGuidance(repository setup.RepositoryContext, result ledger.CurrentResult) *presentationGuidance {
	q := skilldist.ShellQuote
	show := func(reference ledger.Reference) string {
		return "skl ledger show --commit " + q(reference.Commit) + " --path " + q(reference.Path)
	}
	evidence := []string{show(result.Report)}
	for _, reference := range []*ledger.Reference{result.Implement, result.Decision} {
		if reference != nil {
			evidence = append(evidence, show(*reference))
		}
	}
	return &presentationGuidance{
		Result:    result,
		Evidence:  evidence,
		Authoring: "skl skill --resource pull-presentation.md " + result.Phase,
		Continue:  presentInvocation(repository, result.Item) + " --public-body <fresh-public-prose.md>",
	}
}

// presentInvocation binds the explicit current-view presentation command.
func presentInvocation(repository setup.RepositoryContext, item string) string {
	q := skilldist.ShellQuote
	return fmt.Sprintf("skl ledger present --repo %s --remote %s --item %s", q(repository.Root), q(repository.Remote), q(item))
}

// presentFacts are one presentation outcome: the selected current result,
// its presentation notes in a fixed order, and the authoring guidance.
type presentFacts struct {
	Status   string
	Result   *ledger.CurrentResult
	Source   string
	Notes    []noteFact
	Guidance *presentationGuidance
}

// renderPresentation renders one presentation outcome with its facts.
func renderPresentation(stdout io.Writer, format implementationFormatKind, outcome ledgerOutcome) error {
	facts := presentFacts{Status: outcome.Status, Guidance: outcome.Guidance}
	if presentation := outcome.Presentation; presentation != nil {
		facts.Result = &presentation.Result
		var publication *ledger.PublicationNote
		if presentation.Publication.Status != "" {
			publication = &presentation.Publication
		}
		facts.Notes = notesOf(presentation.Replication, publication)
	} else if outcome.Guidance != nil {
		facts.Result = &outcome.Guidance.Result
	}
	if result := facts.Result; result != nil {
		var source []string
		for _, revision := range []struct{ label, sha string }{{"head", result.Source.Head}, {"target", result.Source.Target}, {"reviewed", result.Source.Reviewed}} {
			if revision.sha != "" {
				source = append(source, revision.label+" "+revision.sha)
			}
		}
		facts.Source = strings.Join(source, ", ")
	}
	outcome.kind, outcome.facts = "ledger-present", facts
	return renderLedgerOutcome(stdout, format, outcome)
}
