package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/urfave/cli/v2"
	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/ledger"
	"github.com/vicrdguez/skills/setup"
)

// ledgerOutcome is the shared outcome envelope of the ledger commands: the
// established status, an actionable reason for refusals, and the typed
// payload. Markdown and JSON convey the same facts.
type ledgerOutcome struct {
	Status          string                   `json:"status"`
	Reason          string                   `json:"reason,omitempty"`
	Repair          string                   `json:"repair,omitempty"`
	Acceptance      *ledger.Acceptance       `json:"acceptance,omitempty"`
	Publication     *ledger.Acceptance       `json:"publication,omitempty"`
	Authoring       *proseAuthoring          `json:"authoring,omitempty"`
	Readback        *ledger.Readback         `json:"readback,omitempty"`
	Document        *ledger.ContractDocument `json:"document,omitempty"`
	ReadbackCommand string                   `json:"readback_command,omitempty"`
	// Presentation and Guidance carry one current-view pull request attempt.
	Presentation *ledger.Presentation  `json:"presentation,omitempty"`
	Guidance     *presentationGuidance `json:"guidance,omitempty"`
}

// proseAuthoring tells the caller how to author fresh public prose when a
// publication surface lacked it: the private readback commands, the
// specialized authoring guidance, and the continuation command, each with
// its known arguments bound. skl authors no prose itself.
type proseAuthoring struct {
	Readback     []string `json:"readback"`
	Guidance     string   `json:"guidance"`
	Continuation string   `json:"continuation"`
}

// authoringFor returns the prose authoring pointers when any surface of the
// outcome reported missing prose, and nil otherwise. Every pointer binds the
// selected repository root and remote, so it reads and publishes the same
// Project as the operation that produced it.
func authoringFor(repository setup.RepositoryContext, outcome *ledger.Acceptance) *proseAuthoring {
	missing := outcome.ParentNote != nil && outcome.ParentNote.Status == ledger.IssueMissingInput
	for _, slice := range outcome.Slices {
		missing = missing || slice.IssueStatus != nil && slice.IssueStatus.Status == ledger.IssueMissingInput
	}
	if !missing {
		return nil
	}
	selection := "--repo " + skilldist.ShellQuote(repository.Root) + " --remote " + skilldist.ShellQuote(repository.Remote)
	authoring := &proseAuthoring{
		Guidance:     "skl skill --resource issue-publication.md --input " + skilldist.ShellQuote("proposal="+outcome.Proposal) + " --input " + skilldist.ShellQuote("repo="+repository.Root) + " --input " + skilldist.ShellQuote("remote="+repository.Remote) + " propose",
		Continuation: "skl ledger publish " + selection + " --proposal " + skilldist.ShellQuote(outcome.Proposal),
	}
	for _, slice := range outcome.Slices {
		authoring.Readback = append(authoring.Readback, "skl ledger show "+selection+" --item "+skilldist.ShellQuote(outcome.Proposal+"/"+slice.Name))
		authoring.Continuation += " --issue " + skilldist.ShellQuote(slice.Name+"=") + "<body-file>"
	}
	if len(outcome.Slices) > 1 {
		authoring.Continuation += " --parent-body <parent-body-file>"
	}
	return authoring
}

func ledgerCommands(newBackend backendFactory, stdout io.Writer) *cli.Command {
	return &cli.Command{
		Name: "ledger",
		Subcommands: []*cli.Command{{
			Name:  "accept",
			Usage: "Freeze one prepared proposal into the local Workflow Ledger and attempt initial publication",
			Flags: []cli.Flag{
				&cli.PathFlag{Name: "repo", Value: "."},
				&cli.StringFlag{Name: "remote"},
				&cli.PathFlag{Name: "proposal-dir", Required: true},
				newIssueBodyFlag(),
				&cli.PathFlag{Name: "parent-body"},
				implementationFormatFlag(),
			},
			Action: func(command *cli.Context) error {
				format, err := implementationFormat(command.String("format"))
				if err != nil {
					return err
				}
				bodies, parentBody, err := issueBodyInputs(command)
				if err != nil {
					return err
				}
				repository, err := setup.ResolveRepository(command.Path("repo"), command.String("remote"))
				if err != nil {
					return renderLedgerOutcome(stdout, format, ledgerOutcome{Status: "fix_required", Reason: "repository or remote resolution failed: " + err.Error(), Repair: "run acceptance inside the source repository, or pass --repo and --remote explicitly"})
				}
				declaration, err := ledger.LoadDeclaration(command.Path("proposal-dir"), bodies, parentBody)
				if err != nil {
					return renderLedgerRefusal(stdout, format, err)
				}
				store, failure := openConfiguredLedger()
				if failure != nil {
					return renderLedgerRefusal(stdout, format, failure)
				}
				if err := store.RefuseSourceOverlap(repository.Root); err != nil {
					return renderLedgerRefusal(stdout, format, err)
				}
				backend, err := newBackend(repository.Repository)
				if err != nil {
					return renderLedgerRefusal(stdout, format, err)
				}
				forge, ok := backend.(ledger.Forge)
				if !ok {
					return fmt.Errorf("workflow backend does not support ledger issue publication")
				}
				acceptance, err := ledger.Accept(command.Context, store, repository.Repository, declaration, forge, time.Now)
				if err != nil {
					return renderLedgerRefusal(stdout, format, err)
				}
				return renderLedgerOutcome(stdout, format, ledgerOutcome{Status: acceptance.Status, Acceptance: acceptance, Authoring: authoringFor(repository, acceptance)})
			},
		}, {
			Name:  "publish",
			Usage: "Publish the current descriptive issue and parent presentation of one accepted proposal",
			Flags: []cli.Flag{
				&cli.PathFlag{Name: "repo", Value: "."},
				&cli.StringFlag{Name: "remote"},
				&cli.StringFlag{Name: "proposal", Required: true},
				newIssueBodyFlag(),
				&cli.PathFlag{Name: "parent-body"},
				implementationFormatFlag(),
			},
			Action: func(command *cli.Context) error {
				format, err := implementationFormat(command.String("format"))
				if err != nil {
					return err
				}
				bodies, parentBody, err := issueBodyInputs(command)
				if err != nil {
					return err
				}
				repository, err := setup.ResolveRepository(command.Path("repo"), command.String("remote"))
				if err != nil {
					return renderLedgerOutcome(stdout, format, ledgerOutcome{Status: "fix_required", Reason: "repository or remote resolution failed: " + err.Error(), Repair: "run publication inside the source repository, or pass --repo and --remote explicitly"})
				}
				store, failure := openConfiguredLedger()
				if failure != nil {
					return renderLedgerRefusal(stdout, format, failure)
				}
				if err := store.RefuseSourceOverlap(repository.Root); err != nil {
					return renderLedgerRefusal(stdout, format, err)
				}
				backend, err := newBackend(repository.Repository)
				if err != nil {
					return renderLedgerRefusal(stdout, format, err)
				}
				forge, ok := backend.(ledger.Forge)
				if !ok {
					return fmt.Errorf("workflow backend does not support ledger issue publication")
				}
				publication, err := ledger.PublishCurrent(command.Context, store, repository.Repository, command.String("proposal"), ledger.IssueProse{Bodies: bodies, Parent: parentBody}, forge)
				if err != nil {
					return renderLedgerRefusal(stdout, format, err)
				}
				return renderLedgerOutcome(stdout, format, ledgerOutcome{Status: publication.Status, Publication: publication, Authoring: authoringFor(repository, publication)})
			},
		}, {
			Name:  "show",
			Usage: "Read accepted Contracts or inspect current and exact historical phase reports",
			Flags: []cli.Flag{
				&cli.PathFlag{Name: "repo", Value: ".", Usage: "Source repository whose Work Item to read"},
				&cli.StringFlag{Name: "remote"},
				&cli.StringFlag{Name: "item", Usage: "Accepted Work Item identity (<proposal>/<slice>)"},
				&cli.StringFlag{Name: "phase", Usage: "Current report phase (implement or watchdog); requires --item"},
				&cli.StringFlag{Name: "commit", Usage: "Full ledger commit for exact document retrieval"},
				&cli.StringFlag{Name: "path", Usage: "Ledger-relative document path at --commit"},
				implementationFormatFlag(),
			},
			Action: func(command *cli.Context) error {
				format, err := implementationFormat(command.String("format"))
				if err != nil {
					return err
				}
				if command.NArg() != 0 {
					return fmt.Errorf("ledger show takes flags, not arguments")
				}
				item, phase, commit, path := command.String("item"), command.String("phase"), command.String("commit"), command.String("path")
				explicit := commit != "" || path != ""
				if item != "" && explicit {
					return renderLedgerRefusal(stdout, format, errors.New("ledger show takes either --item or an exact --commit with --path, not both; current --phase selection is available only with --item"))
				}
				if phase != "" && (item == "" || explicit) {
					return renderLedgerRefusal(stdout, format, errors.New("ledger show --phase is available only alongside --item; use --commit with --path to retrieve an exact historical document"))
				}
				if phase != "" && phase != ledger.ImplementPhase && phase != ledger.WatchdogPhase {
					return renderLedgerRefusal(stdout, format, errors.New("ledger show --phase must be implement or watchdog; omit --phase to inspect availability, or use --commit with --path for an exact historical document"))
				}
				if item == "" && (!explicit || commit == "" || path == "") {
					return renderLedgerRefusal(stdout, format, errors.New("ledger show requires --item <proposal>/<slice> or an exact --commit with --path; use the identity and readback command the acceptance reported"))
				}
				store, failure := openConfiguredLedger()
				if failure != nil {
					return renderLedgerRefusal(stdout, format, failure)
				}
				if item != "" {
					repository, err := setup.ResolveRepository(command.Path("repo"), command.String("remote"))
					if err != nil {
						return renderLedgerRefusal(stdout, format, err)
					}
					if phase != "" {
						document, err := ledger.ShowReport(store, repository.Repository, item, phase)
						if err != nil {
							return renderLedgerRefusal(stdout, format, err)
						}
						return renderLedgerOutcome(stdout, format, ledgerOutcome{Status: "shown", Document: &document})
					}
					readback, err := ledger.ShowItem(store, repository.Repository, item)
					if err != nil {
						return renderLedgerRefusal(stdout, format, err)
					}
					return renderLedgerOutcome(stdout, format, ledgerOutcome{
						Status: "shown", Readback: readback,
						ReadbackCommand: fmt.Sprintf("skl ledger show --repo %s --remote %s --item %s",
							skilldist.ShellQuote(repository.Root), skilldist.ShellQuote(repository.Remote), skilldist.ShellQuote(item)),
					})
				}
				document, err := ledger.ShowReference(store, commit, path)
				if err != nil {
					return renderLedgerRefusal(stdout, format, err)
				}
				return renderLedgerOutcome(stdout, format, ledgerOutcome{Status: "shown", Document: &document})
			},
		}, presentCommand(newBackend, stdout)},
	}
}

// openConfiguredLedger resolves the machine configuration and opens the
// configured ledger clone, translating every unusable shape into an
// actionable refusal.
func openConfiguredLedger() (*ledger.Store, error) {
	store, path, exists, err := ledger.LoadSettings(os.Getenv)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, &ledger.Refusal{
			Invariant: "no skl configuration exists at " + path,
			Repair:    "create " + path + " as {\"ledger\": \"/absolute/path/to/ledger-clone\"} pointing at an existing local Git clone of the ledger",
		}
	}
	return store, nil
}

// issueBodyFlag collects repeated --issue <slice>=<body-file> occurrences.
type issueBodyFlag struct {
	cli.GenericFlag
}

func newIssueBodyFlag() *issueBodyFlag {
	return &issueBodyFlag{GenericFlag: cli.GenericFlag{
		Name:  "issue",
		Usage: "Repeatable `slice=path` path to current agent-authored descriptive issue prose for one slice",
	}}
}

func (f *issueBodyFlag) Apply(set *flag.FlagSet) error {
	f.Value = &rawInputs{}
	return f.GenericFlag.Apply(set)
}

// issueBodyInputs reads the current issue prose and parent body. They are
// transport inputs, never persisted or registered ledger content.
func issueBodyInputs(command *cli.Context) (map[string][]byte, []byte, error) {
	values, _ := command.Generic("issue").(*rawInputs)
	bodies := make(map[string][]byte)
	for _, value := range *values {
		slice, path, ok := strings.Cut(value, "=")
		if !ok {
			return nil, nil, fmt.Errorf("invalid --issue %q; want slice=body-file", value)
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, fmt.Errorf("read the temporary issue body for %s: %w", slice, err)
		}
		bodies[slice] = contents
	}
	var parentBody []byte
	if command.Path("parent-body") != "" {
		contents, err := os.ReadFile(command.Path("parent-body"))
		if err != nil {
			return nil, nil, fmt.Errorf("read the temporary parent issue body: %w", err)
		}
		parentBody = contents
	}
	return bodies, parentBody, nil
}

// renderLedgerRefusal renders any refusal with its concrete repair.
func renderLedgerRefusal(stdout io.Writer, format implementationFormatKind, err error) error {
	var refusal *ledger.Refusal
	if errors.As(err, &refusal) {
		return renderLedgerOutcome(stdout, format, ledgerOutcome{Status: "fix_required", Reason: refusal.Invariant, Repair: refusal.Repair})
	}
	return renderLedgerOutcome(stdout, format, ledgerOutcome{Status: "fix_required", Reason: err.Error()})
}

// renderLedgerOutcome writes one outcome in the requested transport. Both
// transports carry the same facts.
func renderLedgerOutcome(stdout io.Writer, format implementationFormatKind, outcome ledgerOutcome) error {
	if format == formatJSON {
		return json.NewEncoder(stdout).Encode(outcome)
	}
	_, err := fmt.Fprint(stdout, ledgerMarkdown(outcome))
	return err
}

// ledgerMarkdown renders one ledger outcome as the default Markdown
// transport, including the exact accepted document contents with their full
// commit and path references.
func ledgerMarkdown(outcome ledgerOutcome) string {
	var report strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&report, format+"\n", args...) }
	line("Status: %s", outcome.Status)
	if outcome.Reason != "" {
		line("%s", outcome.Reason)
	}
	if outcome.Repair != "" {
		line("Repair: %s", outcome.Repair)
	}
	if acceptance := outcome.Acceptance; acceptance != nil {
		presentationMarkdown(line, acceptance)
		line("The local acceptance is authoritative; issue publication is best-effort and never gates local work. Publish the current issue view later with `skl ledger publish`, not by repeating acceptance.")
	}
	if publication := outcome.Publication; publication != nil {
		presentationMarkdown(line, publication)
		line("The local records are authoritative: this attempt recorded only established attachments and ledger replication facts, and a later `skl ledger publish` presents the then-current view.")
	}
	if authoring := outcome.Authoring; authoring != nil {
		line("Author fresh public prose from current private evidence:")
		for _, command := range authoring.Readback {
			line("  Read: %s", command)
		}
		line("  Guidance: %s", authoring.Guidance)
		line("  Then publish: %s", authoring.Continuation)
	}
	if readback := outcome.Readback; readback != nil {
		line("Project: %s (%s)", readback.Project, readback.Repository)
		line("Work Item: %s", readback.Item)
		line("State: %s", readback.State)
		line("Title: %s", readback.Title)
		line("Planned branch: %s", readback.Branch)
		if len(readback.Dependencies) == 0 {
			line("Dependencies: none")
		}
		for _, dependency := range readback.Dependencies {
			state := dependency.State
			if state == "" {
				state = "unrecorded"
			}
			line("Depends on: %s (%s)", dependency.Item, state)
		}
		if readback.Issue != nil {
			line("Issue: %s#%d", readback.Issue.Repository, readback.Issue.Number)
		}
		if readback.ParentIssue != nil {
			line("Parent issue: %s#%d", readback.ParentIssue.Repository, readback.ParentIssue.Number)
		}
		if pending := readback.Pending; pending != nil {
			if pending.Push != nil {
				line("Pending push: %s: %s", pending.Push.Status, pending.Push.Detail)
			}
		}
		if len(readback.Reports) != 0 {
			line("Reports:")
			for _, availability := range readback.Reports {
				if availability.Reference == nil {
					line("- %s: absent", availability.Phase)
					continue
				}
				reference := availability.Reference
				line("- %s: available at %s:%s", availability.Phase, reference.Commit, reference.Path)
				line("  Current retrieval: %s --phase %s", outcome.ReadbackCommand, availability.Phase)
				line("  Exact retrieval: skl ledger show --commit %s --path %s", reference.Commit, reference.Path)
			}
			line("To inspect earlier rounds, follow ledger input references in the report's original frontmatter and retrieve each exact document with skl ledger show --commit <ledger-commit> --path <ledger-path>. Source references identify source revisions, not ledger documents.")
		}
		for _, document := range readback.Documents {
			line("")
			line("## %s at %s", document.Path, document.Commit)
			line("")
			line("%s", strings.TrimRight(safeFence(document.Contents)+"\n"+document.Contents+"\n"+safeFence(document.Contents), "\n"))
		}
	}
	if outcome.Presentation != nil || outcome.Guidance != nil {
		pullPresentationMarkdown(line, outcome.Presentation, outcome.Guidance)
	}
	if document := outcome.Document; document != nil {
		fmt.Fprintf(&report, "Document: %s at %s\n\n", document.Path, document.Commit)
		report.WriteString(document.Contents)
	}
	return report.String()
}

// presentationMarkdown renders the facts of one acceptance or publication
// outcome: ledger replication and each surface's immediate issue outcome.
func presentationMarkdown(line func(string, ...any), acceptance *ledger.Acceptance) {
	line("Project: %s (%s)", acceptance.Project, acceptance.Repository)
	line("Proposal: %s", acceptance.Proposal)
	line("Ledger commit: %s (%s)", acceptance.Commit, acceptance.HeadRef)
	line("Ledger push: %s%s", pushWord(acceptance), pushDetail(acceptance))
	if acceptance.ParentTitle != "" {
		line("Parent issue: %s", attachmentWord(acceptance.ParentIssue, acceptance.ParentNote))
		if acceptance.ParentIssue != nil && acceptance.ParentNote != nil {
			line("Parent publication: %s", noteWord(acceptance.ParentNote))
		}
	}
	if acceptance.BookkeepingStatus != nil {
		line("Publication bookkeeping: %s", noteWord(acceptance.BookkeepingStatus))
	}
	for _, slice := range acceptance.Slices {
		line("Slice %s: %s (branch %s)", slice.Name, slice.Title, slice.Branch)
		if len(slice.Dependencies) == 0 {
			line("  Dependencies: none")
		}
		for _, dependency := range slice.Dependencies {
			line("  Depends on: %s", dependency)
		}
		line("  Issue: %s", attachmentWord(slice.Issue, slice.IssueStatus))
		if slice.Issue != nil && slice.IssueStatus != nil {
			line("  Issue publication: %s", noteWord(slice.IssueStatus))
		}
		if slice.GroupingStatus != nil {
			line("  Parent grouping: %s", noteWord(slice.GroupingStatus))
		}
		line("  Readback: skl ledger show --item %s/%s", acceptance.Proposal, slice.Name)
	}
}

func noteWord(note *ledger.PublicationNote) string {
	if note.Detail == "" {
		return note.Status
	}
	return note.Status + ": " + note.Detail
}

func pushWord(acceptance *ledger.Acceptance) string {
	if len(acceptance.Slices) == 0 {
		return ledger.PushPending
	}
	return acceptance.Slices[0].PushStatus.Status
}

func pushDetail(acceptance *ledger.Acceptance) string {
	if len(acceptance.Slices) == 0 {
		return ""
	}
	if detail := acceptance.Slices[0].PushStatus.Detail; detail != "" {
		return ": " + detail
	}
	return ""
}

func attachmentWord(attachment *ledger.ForgeAttachment, note *ledger.PublicationNote) string {
	if attachment != nil {
		return "attached " + attachment.Repository + "#" + fmt.Sprint(attachment.Number)
	}
	if note != nil && note.Status != "" {
		return noteWord(note)
	}
	return "none"
}

// safeFence picks a fence the content cannot close.
func safeFence(content string) string {
	longest, current := 0, 0
	for _, character := range content {
		if character == '`' {
			current++
			longest = max(longest, current)
			continue
		}
		current = 0
	}
	return strings.Repeat("`", max(3, longest+1))
}

// gateUnsupportedDelivery refuses the legacy worker entrypoints for a
// Project that has adopted the ledger, before any legacy selection, Claim,
// packet, or Git preparation can occur. It receives the already-resolved
// source repository, so callers resolve it exactly once. It returns the
// rendered refusal, or gated=false when the legacy flow remains the
// supported path.
func gateUnsupportedDelivery(stdout io.Writer, format implementationFormatKind, repository github.RepositoryID, operation string) (bool, error) {
	store, failure, err := adoptedLedger(repository)
	if err != nil || (store == nil && failure == nil) {
		return false, err
	}
	if failure != nil {
		return true, renderLedgerOutcome(stdout, format, *failure)
	}
	return true, renderLedgerOutcome(stdout, format, ledgerOutcome{
		Status: "unsupported",
		Reason: operation + " is a legacy source-artifact operation and is unavailable for ledger-accepted work; this refusal grants no Claim and changes no source work",
		Repair: "read the accepted Contract with `skl ledger show --item <proposal>/<slice>` and use the ledger-backed implement/watchdog commands for delivery; administrative cutover or cleanup requires human direction with normal workers stopped",
	})
}

// adoptedLedger returns the configured ledger when this source repository has
// adopted it. A nil store and failure mean the legacy flow remains supported;
// an unusable configuration or record is a failure, never a forge fallback.
func adoptedLedger(repository github.RepositoryID) (*ledger.Store, *ledgerOutcome, error) {
	path, exists, err := ledger.SettingsLocation(os.Getenv)
	if err != nil || !exists {
		// An unconfigured machine keeps the legacy flow; configuration is
		// per machine and adoption is per project.
		return nil, nil, err
	}
	config, err := ledger.LoadConfig(path)
	if err != nil {
		return nil, &ledgerOutcome{
			Status: "fix_required", Reason: err.Error(),
			Repair: "repair the ledger configuration before selecting work; skl uses no forge fallback while it is unreadable",
		}, nil
	}
	store, err := ledger.Open(config.Ledger)
	if err != nil {
		return nil, &ledgerOutcome{
			Status: "fix_required", Reason: err.Error(),
			Repair: "repair the configured ledger clone before selecting work; skl uses no forge fallback while it is unusable",
		}, nil
	}
	adopted, err := store.Adopted(repository)
	if err != nil {
		return nil, &ledgerOutcome{
			Status: "fix_required", Reason: "the ledger records of project " + repository.Name + " are unreadable: " + err.Error(),
			Repair: "repair the ledger records before selecting work; skl uses no forge fallback through unreadable records",
		}, nil
	}
	if !adopted {
		return nil, nil, nil
	}
	return store, nil, nil
}
