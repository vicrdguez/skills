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
	"github.com/vicrdguez/skills/ledger"
	"github.com/vicrdguez/skills/setup"
)

// ledgerOutcome is the shared outcome envelope of the ledger commands: the
// established status, an actionable reason for refusals, and the typed
// payload. Markdown and JSON convey the same facts.
type ledgerOutcome struct {
	Status     string                   `json:"status"`
	Reason     string                   `json:"reason,omitempty"`
	Repair     string                   `json:"repair,omitempty"`
	Acceptance *ledger.Acceptance       `json:"acceptance,omitempty"`
	Readback   *ledger.Readback         `json:"readback,omitempty"`
	Document   *ledger.ContractDocument `json:"document,omitempty"`
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
				backend, err := newBackend(repository.Repository)
				if err != nil {
					return renderLedgerRefusal(stdout, format, err)
				}
				forge, _ := backend.(ledger.Forge)
				acceptance, err := ledger.Accept(command.Context, store, repository.Repository, declaration, forge, time.Now)
				if err != nil {
					return renderLedgerRefusal(stdout, format, err)
				}
				return renderLedgerOutcome(stdout, format, ledgerOutcome{Status: acceptance.Status, Acceptance: acceptance})
			},
		}, {
			Name:  "show",
			Usage: "Read back accepted Contracts from the local Workflow Ledger",
			Flags: []cli.Flag{
				&cli.PathFlag{Name: "repo", Value: "."},
				&cli.StringFlag{Name: "remote"},
				&cli.StringFlag{Name: "item"},
				&cli.StringFlag{Name: "commit"},
				&cli.StringFlag{Name: "path"},
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
				item, commit, path := command.String("item"), command.String("commit"), command.String("path")
				explicit := commit != "" || path != ""
				if item != "" && explicit {
					return renderLedgerRefusal(stdout, format, errors.New("ledger show takes either --item or an exact --commit with --path, not both; choose one identity source"))
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
					readback, err := ledger.ShowItem(store, repository.Repository, item)
					if err != nil {
						return renderLedgerRefusal(stdout, format, err)
					}
					return renderLedgerOutcome(stdout, format, ledgerOutcome{Status: "shown", Readback: readback})
				}
				document, err := ledger.ShowReference(store, commit, path)
				if err != nil {
					return renderLedgerRefusal(stdout, format, err)
				}
				return renderLedgerOutcome(stdout, format, ledgerOutcome{Status: "shown", Document: &document})
			},
		}},
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
		Usage: "Repeatable `slice=path` path to the temporary descriptive issue body for one slice",
	}}
}

func (f *issueBodyFlag) Apply(set *flag.FlagSet) error {
	f.Value = &rawInputs{}
	return f.GenericFlag.Apply(set)
}

// issueBodyInputs reads the temporary issue bodies and parent body. They
// are transport inputs, never persisted ledger content.
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
		line("Project: %s (%s)", acceptance.Project, acceptance.Repository)
		line("Proposal: %s", acceptance.Proposal)
		line("Ledger commit: %s (%s)", acceptance.Commit, acceptance.HeadRef)
		line("Ledger push: %s%s", pushWord(acceptance), pushDetail(acceptance))
		if acceptance.ParentTitle != "" {
			line("Parent issue: %s", attachmentWord(acceptance.ParentIssue, acceptance.ParentNote))
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
			line("  Readback: skl ledger show --item %s/%s", acceptance.Proposal, slice.Name)
		}
		line("The local acceptance is authoritative; pending publication effects stay readable and can be retried by repeating this acceptance.")
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
			if pending.Issue != nil {
				line("Pending issue: %s: %s", pending.Issue.Status, pending.Issue.Detail)
			}
		}
		for _, document := range readback.Documents {
			line("")
			line("## %s at %s", document.Path, document.Commit)
			line("")
			line("%s", strings.TrimRight(safeFence(document.Contents)+"\n"+document.Contents+"\n"+safeFence(document.Contents), "\n"))
		}
	}
	if document := outcome.Document; document != nil {
		line("Document: %s at %s", document.Path, document.Commit)
		line("")
		line("%s", strings.TrimRight(document.Contents, "\n"))
	}
	return report.String()
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
		return note.Status + ": " + note.Detail
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
// packet, or Git preparation can occur. It returns the rendered refusal, or
// ok=false when the legacy flow remains the supported path.
func gateUnsupportedDelivery(stdout io.Writer, format implementationFormatKind, repositoryRoot string, remote string, operation string) (bool, error) {
	path, exists, err := ledger.SettingsLocation(os.Getenv)
	if err != nil || !exists {
		// An unconfigured machine keeps the legacy flow; configuration is
		// per machine and adoption is per project.
		return false, err
	}
	config, err := ledger.LoadConfig(path)
	if err != nil {
		return true, renderLedgerOutcome(stdout, format, ledgerOutcome{
			Status: "fix_required", Reason: err.Error(),
			Repair: "repair the ledger configuration before selecting work; skl uses no forge fallback while it is unreadable",
		})
	}
	store, err := ledger.Open(config.Ledger)
	if err != nil {
		return true, renderLedgerOutcome(stdout, format, ledgerOutcome{
			Status: "fix_required", Reason: err.Error(),
			Repair: "repair the configured ledger clone before selecting work; skl uses no forge fallback while it is unusable",
		})
	}
	repository, err := setup.ResolveRepository(repositoryRoot, remote)
	if err != nil {
		return false, nil
	}
	adopted, err := store.Adopted(repository.Repository)
	if err != nil {
		return true, renderLedgerOutcome(stdout, format, ledgerOutcome{
			Status: "fix_required", Reason: "the ledger records of project " + repository.Repository.Name + " are unreadable: " + err.Error(),
			Repair: "repair the ledger records before selecting work; skl uses no forge fallback through unreadable records",
		})
	}
	if !adopted {
		return false, nil
	}
	return true, renderLedgerOutcome(stdout, format, ledgerOutcome{
		Status: "unsupported",
		Reason: operation + " does not deliver ledger-accepted work yet: ledger delivery awaits run-ledger-delivery, and this refusal grants no Claim, no execution packet, and no source branch or worktree",
		Repair: "read the accepted Contract with `skl ledger show --item <proposal>/<slice>` and repeat it once ledger delivery ships; adopting in-flight work requires a human-directed administrative cutover with normal workers stopped",
	})
}
