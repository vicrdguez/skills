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
			if command.NArg() != 0 {
				return renderLedgerRefusal(stdout, format, fmt.Errorf("ledger present takes flags, not arguments"))
			}
			repository, err := setup.ResolveRepository(command.Path("repo"), command.String("remote"))
			if err != nil {
				return renderLedgerRefusal(stdout, format, err)
			}
			store, err := openConfiguredLedger()
			if err != nil {
				return renderLedgerRefusal(stdout, format, err)
			}
			if err := store.RefuseSourceOverlap(repository.Root); err != nil {
				return renderLedgerRefusal(stdout, format, err)
			}
			selected, err := ledger.SelectCurrentResult(store, repository.Repository, command.String("item"))
			if err != nil {
				return renderLedgerRefusal(stdout, format, err)
			}
			if command.Path("public-body") == "" {
				return renderLedgerOutcome(stdout, format, ledgerOutcome{Status: "prose_required", Reason: "no public prose was supplied; author it for the current result from the private evidence below", Guidance: presentGuidance(repository, selected)})
			}
			body, err := os.ReadFile(command.Path("public-body"))
			if err != nil {
				return renderLedgerRefusal(stdout, format, fmt.Errorf("read the freshly authored public prose: %w", err))
			}
			backend, err := newBackend(repository.Repository)
			if err != nil {
				note := ledger.PublicationNote{Status: ledger.IssuePending, Detail: "forge construction unavailable: " + err.Error()}
				return renderLedgerOutcome(stdout, format, ledgerOutcome{Status: note.Status, Presentation: &ledger.Presentation{Result: selected, Publication: note}, Guidance: presentGuidance(repository, selected)})
			}
			forge, ok := backend.(ledger.DeliveryForge)
			if !ok {
				return fmt.Errorf("workflow backend does not support pull request presentation")
			}
			presentation := ledger.PresentCurrent(command.Context, store, repository.Repository, repository.Root, repository.Remote, selected, string(body), forge)
			outcome := ledgerOutcome{Status: presentation.Publication.Status, Presentation: presentation}
			if presentation.Publication.Status != ledger.PullPresented {
				outcome.Guidance = presentGuidance(repository, presentation.Result)
			}
			return renderLedgerOutcome(stdout, format, outcome)
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
		Authoring: "skl skill --resource reference/pull-presentation.md " + result.Phase,
		Continue:  fmt.Sprintf("skl ledger present --repo %s --remote %s --item %s --public-body <fresh-public-prose.md>", q(repository.Root), q(repository.Remote), q(result.Item)),
	}
}

// presentationMarkdown renders the presentation facts the JSON transport
// carries.
func presentationMarkdown(line func(string, ...any), presentation *ledger.Presentation, guidance *presentationGuidance) {
	var result *ledger.CurrentResult
	if presentation != nil {
		result = &presentation.Result
	} else if guidance != nil {
		result = &guidance.Result
	}
	if result != nil {
		line("Work Item: %s (%s)", result.Item, result.Lifecycle)
		round := ""
		if result.Round > 0 {
			round = fmt.Sprintf(", review round %d", result.Round)
		}
		line("Current result: %s %s%s at %s", result.Phase, result.Outcome, round, result.Report.Path)
		line("Ledger commit: %s", result.Report.Commit)
		var source []string
		for _, revision := range []struct{ label, sha string }{{"head", result.Source.Head}, {"target", result.Source.Target}, {"reviewed", result.Source.Reviewed}} {
			if revision.sha != "" {
				source = append(source, revision.label+" "+revision.sha)
			}
		}
		if len(source) > 0 {
			line("Source: %s", strings.Join(source, ", "))
		}
		line("Branch: %s", result.Branch)
		if result.Submission != nil {
			line("Submission: %s#%d", result.Submission.Repository, result.Submission.Number)
		}
		if result.Claimed {
			line("A later Claim is active; presentation leaves it unchanged.")
		}
	}
	if presentation != nil && presentation.Publication.Status != "" {
		line("Public presentation: %s — %s", presentation.Publication.Status, presentation.Publication.Detail)
	}
	if presentation != nil && presentation.Replication != nil {
		line("Ledger replication: %s — %s", presentation.Replication.Status, presentation.Replication.Detail)
	}
	if guidance != nil {
		for _, command := range guidance.Evidence {
			line("Private evidence: `%s`", command)
		}
		line("Authoring guidance: `%s`", guidance.Authoring)
		line("Present with fresh prose: `%s`", guidance.Continue)
	}
}
