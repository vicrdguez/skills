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
	Status           string                   `json:"status"`
	Reason           string                   `json:"reason,omitempty"`
	Repair           string                   `json:"repair,omitempty"`
	Overview         *ledger.Overview         `json:"overview,omitempty"`
	Inventory        *ledger.ProjectInventory `json:"inventory,omitempty"`
	Proposal         *ledger.ProposalDetail   `json:"proposal,omitempty"`
	Slice            *ledger.SliceDetail      `json:"slice,omitempty"`
	Slices           *ledger.SliceSearch      `json:"slices,omitempty"`
	Documents        *ledger.DocumentSet      `json:"documents,omitempty"`
	Document         *ledger.Document         `json:"document,omitempty"`
	SnapshotRevision string                   `json:"snapshot_revision,omitempty"`
}

func browseCommand(stdin io.Reader, stdout io.Writer) *cli.Command {
	projectFlag := func(usage string) cli.Flag { return &cli.StringFlag{Name: "project", Usage: usage} }
	archivedFlag := &cli.BoolFlag{Name: "include-archived", Usage: "Include archived Proposals"}
	locationFlag := func() cli.Flag {
		return &cli.BoolFlag{Name: "archived", Usage: "Select the archived Proposal rather than preferring the active one"}
	}
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
			Flags: []cli.Flag{projectFlag("Project of the Proposal"), &cli.StringFlag{Name: "proposal"}, locationFlag(), implementationFormatFlag()},
			Action: func(command *cli.Context) error {
				return runBrowseQuery(command, stdout, func(snapshot *ledger.Snapshot) (browseOutcome, error) {
					var proposal *ledger.ProposalDetail
					var err error
					if command.Bool("archived") {
						proposal, err = snapshot.ProposalAt(command.String("project"), command.String("proposal"), true)
					} else {
						proposal, err = snapshot.Proposal(command.String("project"), command.String("proposal"))
					}
					return browseOutcome{Proposal: proposal}, err
				})
			},
		}, {
			Name:  "slice",
			Usage: "Show every recorded fact of one Slice",
			Flags: []cli.Flag{projectFlag("Project of the Slice"), &cli.StringFlag{Name: "item", Usage: "Slice identity (<proposal>/<slice>)"}, locationFlag(), implementationFormatFlag()},
			Action: func(command *cli.Context) error {
				return runBrowseQuery(command, stdout, func(snapshot *ledger.Snapshot) (browseOutcome, error) {
					var slice *ledger.SliceDetail
					var err error
					if command.Bool("archived") {
						slice, err = snapshot.SliceAt(command.String("project"), command.String("item"), true)
					} else {
						slice, err = snapshot.Slice(command.String("project"), command.String("item"))
					}
					return browseOutcome{Slice: slice}, err
				})
			},
		}, {
			Name:  "slices",
			Usage: "Find Slices by lifecycle, Claim, and name in one Project or every Project",
			Flags: []cli.Flag{
				projectFlag("Project to search; every Project when omitted"), archivedFlag,
				&cli.StringSliceFlag{Name: "lifecycle", Usage: "Select any of these recorded lifecycles (" + strings.Join(ledger.Lifecycles, ", ") + ")"},
				&cli.StringSliceFlag{Name: "claim", Usage: "Select any of these Claims (" + strings.Join(ledger.Claims, ", ") + ")"},
				&cli.StringFlag{Name: "search", Usage: "Select Slices whose project/proposal/slice identity or title contains this text"},
				&cli.StringFlag{Name: "group", Value: ledger.GroupByProposal, Usage: "Group each Project's Slices by " + ledger.GroupByProposal + " or " + ledger.GroupByLifecycle},
				implementationFormatFlag(),
			},
			Action: func(command *cli.Context) error {
				return runBrowseQuery(command, stdout, func(snapshot *ledger.Snapshot) (browseOutcome, error) {
					search, err := snapshot.FindSlices(ledger.SliceQuery{
						Project: command.String("project"), IncludeArchived: command.Bool("include-archived"),
						Lifecycles: command.StringSlice("lifecycle"), Claims: command.StringSlice("claim"),
						Text: command.String("search"), GroupBy: command.String("group"),
					})
					return browseOutcome{Slices: search}, err
				})
			},
		}, {
			Name:  "documents",
			Usage: "Read current committed documents for one Proposal or Slice",
			Flags: []cli.Flag{
				projectFlag("Project of the Proposal or Slice"), locationFlag(),
				&cli.StringFlag{Name: "proposal", Usage: "Proposal whose description and Slice documents to read"},
				&cli.StringFlag{Name: "item", Usage: "Slice identity (<proposal>/<slice>) whose current documents to read"},
				implementationFormatFlag(),
			},
			Action: func(command *cli.Context) error {
				return runBrowseQuery(command, stdout, func(snapshot *ledger.Snapshot) (browseOutcome, error) {
					proposalSet, itemSet := command.IsSet("proposal"), command.IsSet("item")
					if proposalSet == itemSet {
						return browseOutcome{}, errors.New("browse documents requires exactly one of --proposal or --item")
					}
					var documents *ledger.DocumentSet
					var err error
					if proposalSet {
						if command.Bool("archived") {
							documents, err = snapshot.ProposalDocumentsAt(command.String("project"), command.String("proposal"), true)
						} else {
							documents, err = snapshot.ProposalDocuments(command.String("project"), command.String("proposal"))
						}
					} else if command.Bool("archived") {
						documents, err = snapshot.SliceDocumentsAt(command.String("project"), command.String("item"), true)
					} else {
						documents, err = snapshot.SliceDocuments(command.String("project"), command.String("item"))
					}
					return browseOutcome{Documents: documents}, err
				})
			},
		}, {
			Name:  "document",
			Usage: "Read one exact current or historical ledger document reference",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "commit", Usage: "Full ledger commit containing the exact document"},
				&cli.StringFlag{Name: "path", Usage: "Ledger-relative document path at --commit"},
				implementationFormatFlag(),
			},
			Action: func(command *cli.Context) error {
				return runBrowseQuery(command, stdout, func(snapshot *ledger.Snapshot) (browseOutcome, error) {
					commit, path := command.String("commit"), command.String("path")
					if commit == "" || path == "" {
						return browseOutcome{}, errors.New("browse document requires both --commit (full ledger SHA) and --path for an exact document reference")
					}
					document, err := snapshot.Document(ledger.Reference{Commit: commit, Path: path})
					return browseOutcome{Document: document, SnapshotRevision: snapshot.Revision}, err
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
		list(diagnosticTexts(overview.Diagnostics))
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
	if documents := outcome.Documents; documents != nil {
		renderBrowseDocuments(&report, line, documents)
	}
	if document := outcome.Document; document != nil {
		line("Ledger snapshot revision: " + outcome.SnapshotRevision)
		identity := "current snapshot"
		if document.Reference.Commit != outcome.SnapshotRevision {
			identity = "historical document; current Slice facts remain at the snapshot revision"
		}
		line("Document ledger revision: " + document.Reference.Commit + " (" + identity + ")")
		renderBrowseDocument(&report, line, document)
	}
	if search := outcome.Slices; search != nil {
		line("Ledger revision: " + search.Revision)
		line("Selection: " + browse.SelectionText(search.Query))
		line("Result: " + browse.ResultText(search))
		list(diagnosticTexts(search.Diagnostics))
		for _, project := range search.Projects {
			line("\n## Project " + project.Name + " (" + orUnknown(project.Repository) + ")")
			if len(project.Diagnostics) > 0 {
				line("")
				list(diagnosticTexts(project.Diagnostics))
			}
			for _, group := range project.Groups {
				line("\n### " + browse.GroupTitle(group) + "\n")
				for _, match := range group.Slices {
					line("- " + browse.MatchRow(match))
				}
			}
			if len(project.Undecided) > 0 {
				line("\n### " + browse.UndecidedTitle + "\n")
				for _, match := range project.Undecided {
					line("- " + browse.MatchRow(match))
					list(diagnosticTexts(match.Diagnostics))
				}
			}
		}
	}
	_, err := fmt.Fprint(stdout, report.String())
	return err
}

func renderBrowseDocuments(report *strings.Builder, line func(string), set *ledger.DocumentSet) {
	line("Ledger revision: " + set.Revision)
	context := "Proposal " + set.Project + "/" + set.Proposal
	if set.Slice != "" {
		context += " / Slice " + set.Slice
	}
	if set.Archived {
		context += " (archived)"
	}
	line("Context: " + context)
	if set.Incomplete {
		line("Record completeness: incomplete")
	} else {
		line("Record completeness: complete")
	}
	documentDiagnostics := make(map[ledger.Diagnostic]bool)
	for _, document := range set.Documents {
		for _, diagnostic := range document.Diagnostics {
			documentDiagnostics[diagnostic] = true
		}
	}
	for _, diagnostic := range set.Diagnostics {
		if !documentDiagnostics[diagnostic] {
			line("Diagnostic: " + browse.DiagnosticText(diagnostic))
		}
	}
	renderReportAvailability(line, set)
	if len(set.Documents) == 0 {
		line("No documents are readable for this selection.")
	}
	for index := range set.Documents {
		renderBrowseDocument(report, line, &set.Documents[index])
	}
}

func renderReportAvailability(line func(string), set *ledger.DocumentSet) {
	for _, availability := range set.Availability {
		if availability.Kind == ledger.DecisionDocumentKind {
			status := "unknown; see diagnostic"
			switch availability.Status {
			case ledger.DocumentAvailable:
				status = "active"
				if availability.Reference != nil {
					status += " at " + availability.Reference.Commit + ":" + availability.Reference.Path
				}
			case ledger.DocumentAbsent:
				status = "not yet available"
			case ledger.DocumentUnavailable:
				status = "unavailable; see diagnostic"
			}
			line("Human Decision availability for " + availability.Slice + " — " + status)
			continue
		}

		phase := ""
		switch availability.Kind {
		case ledger.ImplementReportDocumentKind:
			phase = ledger.ImplementPhase
		case ledger.WatchdogReportDocumentKind:
			phase = ledger.WatchdogPhase
		default:
			continue
		}
		status := "unknown; see diagnostic"
		switch availability.Status {
		case ledger.DocumentAvailable:
			status = "available"
			if availability.Reference != nil {
				status += " at " + availability.Reference.Commit + ":" + availability.Reference.Path
			}
		case ledger.DocumentAbsent:
			status = "not yet available"
		case ledger.DocumentUnavailable:
			status = "unavailable; see diagnostic"
		}
		line("Report availability: " + phase + " for " + availability.Slice + " — " + status)
	}
}

func renderBrowseDocument(report *strings.Builder, line func(string), document *ledger.Document) {
	line("\n## " + document.Reference.Path + "\n")
	line("Document kind: " + string(document.Kind))
	line("Ledger document reference: " + document.Reference.Commit + ":" + document.Reference.Path)
	if metadata := document.Report; metadata != nil {
		line("Report metadata: schema " + strconv.Itoa(metadata.Schema) + ", outcome " + metadata.Outcome)
		if metadata.Round != 0 {
			line("Watchdog round: " + strconv.FormatUint(metadata.Round, 10))
		}
		if metadata.Source.Head != "" || metadata.Source.Target != "" || metadata.Source.Reviewed != "" {
			line("Source repository revisions (not ledger revisions):")
			if metadata.Source.Head != "" {
				line("- Source head: " + metadata.Source.Head)
			}
			if metadata.Source.Target != "" {
				line("- Source target: " + metadata.Source.Target)
			}
			if metadata.Source.Reviewed != "" {
				line("- Source reviewed: " + metadata.Source.Reviewed)
			}
		}
		if len(document.References) > 0 {
			line("Consumed ledger references:")
			for _, reference := range document.References {
				line("- " + reference.Label + ": " + reference.Reference.Commit + ":" + reference.Reference.Path)
			}
		}
	}
	if claim := document.Claim; claim != nil {
		line("Claim metadata: phase " + claim.Phase + ", basis (ledger revision) " + claim.Basis)
		if len(document.References) > 0 {
			line("Claim input ledger references:")
			for _, reference := range document.References {
				line("- " + reference.Label + ": " + reference.Reference.Commit + ":" + reference.Reference.Path)
			}
		}
	}
	if metadata := document.Decision; metadata != nil {
		line("Human Decision metadata: schema " + strconv.Itoa(metadata.Schema))
		line("- Project: " + metadata.Project)
		line("- Slice: " + metadata.Item)
		line("- Route: " + metadata.Route)
		line("- Answered request (ledger reference): " + metadata.AnsweredRequest.Commit + ":" + metadata.AnsweredRequest.Path)
	}
	for _, diagnostic := range document.Diagnostics {
		line("Diagnostic: " + browse.DiagnosticText(diagnostic))
	}
	line("\nContent:\n")
	report.WriteString(document.Contents)
	if !strings.HasSuffix(document.Contents, "\n") {
		report.WriteByte('\n')
	}
	line("")
}

func diagnosticTexts(diagnostics []ledger.Diagnostic) []string {
	var texts []string
	for _, diagnostic := range diagnostics {
		texts = append(texts, browse.DiagnosticText(diagnostic))
	}
	return texts
}

func orUnknown(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}
