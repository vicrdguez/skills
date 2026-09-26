package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"
	"github.com/urfave/cli/v2"
	"github.com/vicrdguez/skills/browse"
	"github.com/vicrdguez/skills/ledger"
	"github.com/vicrdguez/skills/setup"
)

// The browse commands read the machine-configured ledger at one committed
// revision. Like the decision inbox they construct no forge, and unlike
// status they never reconcile completion or write Workflow State; the source
// checkout only hints the initial Project.

// browseOutcome is the query transport: the established status, an
// actionable reason for refusals, and one typed query result.
type browseOutcome struct {
	Status    string                   `json:"status"`
	Reason    string                   `json:"reason,omitempty"`
	Repair    string                   `json:"repair,omitempty"`
	Overview  *ledger.Overview         `json:"overview,omitempty"`
	Inventory *ledger.ProjectInventory `json:"inventory,omitempty"`
	Proposal  *ledger.ProposalDetail   `json:"proposal,omitempty"`
	Slice     *ledger.SliceDetail      `json:"slice,omitempty"`
}

func browseCommand(stdin io.Reader, stdout io.Writer) *cli.Command {
	projectFlag := func(usage string) cli.Flag { return &cli.StringFlag{Name: "project", Usage: usage} }
	archivedFlag := &cli.BoolFlag{Name: "include-archived", Usage: "Include archived Proposals"}
	return &cli.Command{
		Name:  "browse",
		Usage: "Browse ledger Projects, Proposals, and Slices from committed records",
		Flags: []cli.Flag{projectFlag("Initial Project; overrides the current checkout")},
		Action: func(command *cli.Context) error {
			if command.NArg() != 0 {
				return fmt.Errorf("unknown browse query %q; run `skl browse --help` for the queries", command.Args().First())
			}
			model, err := startBrowser(command.String("project"), command.IsSet("project"), ".")
			if err != nil {
				return err
			}
			output, ok := stdout.(*os.File)
			if !ok || !term.IsTerminal(output.Fd()) {
				return errors.New("skl browse needs an interactive terminal; use `skl browse projects` and the other browse queries for non-interactive output")
			}
			_, err = tea.NewProgram(model, tea.WithInput(stdin), tea.WithOutput(output), tea.WithAltScreen()).Run()
			return err
		},
		Subcommands: []*cli.Command{{
			Name:  "projects",
			Usage: "Summarize every Project in the configured ledger",
			Flags: []cli.Flag{archivedFlag, implementationFormatFlag()},
			Action: func(command *cli.Context) error {
				return runBrowseQuery(command, stdout, func(snapshot *ledger.Snapshot) (browseOutcome, error) {
					overview, err := snapshot.Overview(command.Bool("include-archived"))
					return browseOutcome{Overview: overview}, err
				})
			},
		}, {
			Name:  "project",
			Usage: "List one Project's Proposals",
			Flags: []cli.Flag{projectFlag("Project to list"), archivedFlag, implementationFormatFlag()},
			Action: func(command *cli.Context) error {
				return runBrowseQuery(command, stdout, func(snapshot *ledger.Snapshot) (browseOutcome, error) {
					inventory, err := snapshot.Project(command.String("project"), command.Bool("include-archived"))
					return browseOutcome{Inventory: inventory}, err
				})
			},
		}, {
			Name:  "proposal",
			Usage: "Show one Proposal, active or archived, with its Slices",
			Flags: []cli.Flag{projectFlag("Project of the Proposal"), &cli.StringFlag{Name: "proposal"}, implementationFormatFlag()},
			Action: func(command *cli.Context) error {
				return runBrowseQuery(command, stdout, func(snapshot *ledger.Snapshot) (browseOutcome, error) {
					proposal, err := snapshot.Proposal(command.String("project"), command.String("proposal"))
					return browseOutcome{Proposal: proposal}, err
				})
			},
		}, {
			Name:  "slice",
			Usage: "Show every recorded fact of one Slice",
			Flags: []cli.Flag{projectFlag("Project of the Slice"), &cli.StringFlag{Name: "item", Usage: "Slice identity (<proposal>/<slice>)"}, implementationFormatFlag()},
			Action: func(command *cli.Context) error {
				return runBrowseQuery(command, stdout, func(snapshot *ledger.Snapshot) (browseOutcome, error) {
					slice, err := snapshot.Slice(command.String("project"), command.String("item"))
					return browseOutcome{Slice: slice}, err
				})
			},
		}},
	}
}

// startBrowser resolves the initial Project and returns the session model.
// An explicit selection wins and must name a recorded Project. Otherwise the
// checkout at location selects the Project recording its repository when
// exactly one does; every other case starts at the overview and says why.
func startBrowser(explicit string, explicitSet bool, location string) (browse.Model, error) {
	store, err := openConfiguredLedger()
	if err != nil {
		return browse.Model{}, err
	}
	snapshot, err := store.Snapshot()
	if err != nil {
		return browse.Model{}, err
	}
	if explicitSet {
		if !slices.Contains(snapshot.ProjectNames(), explicit) {
			return browse.Model{}, &ledger.Refusal{
				Invariant: "unknown Project " + strconv.Quote(explicit) + " in the configured ledger at revision " + snapshot.Revision,
				Repair:    "select a Project listed by `skl browse projects`",
			}
		}
		return browse.New(snapshot, browse.Options{Project: explicit}), nil
	}
	repository, err := setup.ResolveRepository(location, "")
	if err != nil {
		return browse.New(snapshot, browse.Options{Notice: "Showing every Project: the current directory identifies no single GitHub repository"}), nil
	}
	overview, err := snapshot.Overview(false)
	if err != nil {
		return browse.Model{}, err
	}
	identity := repository.Repository.Owner + "/" + repository.Repository.Name
	var matches, unreadable []string
	for _, project := range overview.Projects {
		switch project.Repository {
		case identity:
			matches = append(matches, project.Name)
		case "":
			unreadable = append(unreadable, project.Name)
		}
	}
	switch {
	case len(matches) == 1:
		return browse.New(snapshot, browse.Options{Project: matches[0]}), nil
	case len(matches) == 0 && len(unreadable) > 0:
		return browse.New(snapshot, browse.Options{Notice: "Showing every Project: no readable Project records " + identity + "; the repository of " + strings.Join(unreadable, ", ") + " is unreadable"}), nil
	case len(matches) == 0:
		return browse.New(snapshot, browse.Options{Notice: "Showing every Project: no Project records " + identity}), nil
	default:
		return browse.New(snapshot, browse.Options{Notice: "Showing every Project: several Projects record " + identity + " (" + strings.Join(matches, ", ") + ")"}), nil
	}
}

// runBrowseQuery answers one read-only query at the current committed ledger
// revision.
func runBrowseQuery(command *cli.Context, stdout io.Writer, query func(*ledger.Snapshot) (browseOutcome, error)) error {
	format, err := implementationFormat(command.String("format"))
	if err != nil {
		return err
	}
	if command.NArg() != 0 {
		return fmt.Errorf("browse %s takes flags, not positional arguments", command.Command.Name)
	}
	store, err := openConfiguredLedger()
	if err != nil {
		return renderBrowse(stdout, format, refusedBrowse(err))
	}
	snapshot, err := store.Snapshot()
	if err != nil {
		return renderBrowse(stdout, format, refusedBrowse(err))
	}
	outcome, err := query(snapshot)
	if err != nil {
		return renderBrowse(stdout, format, refusedBrowse(err))
	}
	outcome.Status = "shown"
	return renderBrowse(stdout, format, outcome)
}

func refusedBrowse(err error) browseOutcome {
	var refusal *ledger.Refusal
	if errors.As(err, &refusal) {
		return browseOutcome{Status: "fix_required", Reason: refusal.Invariant, Repair: refusal.Repair}
	}
	return browseOutcome{Status: "fix_required", Reason: err.Error()}
}

// renderBrowse writes one query outcome. Markdown and JSON carry the same
// facts.
func renderBrowse(stdout io.Writer, format implementationFormatKind, outcome browseOutcome) error {
	if format == formatJSON {
		return json.NewEncoder(stdout).Encode(outcome)
	}
	var report strings.Builder
	line := func(text string) { report.WriteString(text + "\n") }
	list := func(lines []string) {
		for _, text := range lines {
			line("- " + text)
		}
	}
	line("Status: " + outcome.Status)
	if outcome.Reason != "" {
		line("Reason: " + outcome.Reason)
	}
	if outcome.Repair != "" {
		line("Repair: " + outcome.Repair)
	}
	if overview := outcome.Overview; overview != nil {
		line("Ledger revision: " + overview.Revision)
		for _, diagnostic := range overview.Diagnostics {
			line("- " + browse.DiagnosticText(diagnostic))
		}
		if len(overview.Projects) == 0 {
			line("No Projects are recorded at this revision.")
		}
		for _, project := range overview.Projects {
			line("\n## " + project.Name + "\n")
			list(browse.ProjectLines(project))
		}
	}
	if inventory := outcome.Inventory; inventory != nil {
		line("Ledger revision: " + inventory.Revision)
		line("\n## Project " + inventory.Project.Name + "\n")
		list(browse.ProjectLines(inventory.Project))
		if len(inventory.Proposals) == 0 {
			line("\nNo Proposals are listed; archived Proposals appear with --include-archived.")
		}
		for _, proposal := range inventory.Proposals {
			line("\n### " + proposal.Name + "\n")
			list(browse.ProposalLines(proposal))
		}
	}
	if detail := outcome.Proposal; detail != nil {
		line("Ledger revision: " + detail.Revision)
		line("\n## Proposal " + detail.Project + "/" + detail.Proposal.Name + "\n")
		list(browse.ProposalLines(detail.Proposal))
		for _, slice := range detail.Slices {
			line("\n### " + slice.Slice + "\n")
			list(browse.SliceSummaryLines(slice))
		}
	}
	if slice := outcome.Slice; slice != nil {
		line("Ledger revision: " + slice.Revision)
		line("\n## Slice " + slice.Project + "/" + slice.Item + "\n")
		list(browse.SliceLines(slice))
	}
	_, err := fmt.Fprint(stdout, report.String())
	return err
}
