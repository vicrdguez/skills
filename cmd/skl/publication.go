package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v2"
	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/ledger"
	"github.com/vicrdguez/skills/setup"
)

// publicationOutput is the shared outcome envelope of the publication
// commands. A successful inspection or recovery carries the owner skill's
// specialized packet together with any independent finding outcomes; a
// refusal carries the actionable reason and repair. Inspection and recovery
// never select or claim a Work Item.
type publicationOutput struct {
	Status   string                      `json:"status,omitempty"`
	Reason   string                      `json:"reason,omitempty"`
	Repair   string                      `json:"repair,omitempty"`
	Packet   *skilldist.Packet           `json:"packet,omitempty"`
	Findings []ledger.FindingPublication `json:"findings,omitempty"`
}

// publicationCommands exposes the explicit presentation recovery surface. A
// parent is selected by any member Work Item; one latest committed view is
// selected by --item and --kind.
func publicationCommands(newBackend backendFactory, stdout io.Writer) *cli.Command {
	return &cli.Command{
		Name: "publication",
		Subcommands: []*cli.Command{
			publicationCommand("inspect", newBackend, stdout),
			publicationCommand("recover", newBackend, stdout),
		},
	}
}

func publicationCommand(operation string, newBackend backendFactory, stdout io.Writer) *cli.Command {
	flags := []cli.Flag{
		&cli.PathFlag{Name: "repo", Value: ".", Usage: "Source repository to resolve the selected Project from"},
		&cli.StringFlag{Name: "remote", Usage: "Remote to resolve the Project from; defaults to the GitHub remote"},
		&cli.StringFlag{Name: "item", Usage: "Selected Work Item as <proposal>/<slice>; any member selects its parent"},
		&cli.StringFlag{Name: "kind", Usage: "Selected presentation: issue, parent, or pull"},
		&cli.PathFlag{Name: "result-directory", Usage: "Existing absolute private directory for newly authored public prose"},
		implementationFormatFlag(),
	}
	if operation == "recover" {
		flags = append(flags,
			&cli.StringFlag{Name: "view", Usage: "Exact current view token inspect returned, required with newly authored prose"},
			&cli.PathFlag{Name: "body", Usage: "Absolute path of the newly authored public body; requires --view"},
			&cli.PathFlag{Name: "findings", Usage: "Absolute path of a JSON array selecting actionable inline findings"},
		)
	}
	return &cli.Command{
		Name:  operation,
		Flags: flags,
		Action: func(command *cli.Context) error {
			return runPublication(command, operation, newBackend, stdout)
		},
	}
}

func runPublication(c *cli.Context, operation string, newBackend backendFactory, stdout io.Writer) error {
	format, err := implementationFormat(c.String("format"))
	if err != nil {
		return err
	}
	emit := func(out publicationOutput) error { return renderPublication(stdout, format, operation, out) }
	refusal := func(err error) error { return emit(publicationRefusal(err)) }
	if c.NArg() != 0 {
		return refusal(errors.New("publication commands take flags, not positional arguments"))
	}
	item, kind := c.String("item"), c.String("kind")
	if item == "" {
		return refusal(errors.New("publication " + operation + " requires --item <proposal>/<slice>"))
	}
	if kind == "" {
		return refusal(errors.New("publication " + operation + " requires --kind issue, parent, or pull"))
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
	if operation == "inspect" {
		// Inspection is read-only: it constructs no forge and performs no
		// source or workflow preparation.
		view, err := ledger.InspectPublication(store, repository.Repository, item, kind)
		if err != nil {
			return refusal(err)
		}
		return publish(emit, repository, operation, item, kind, c.Path("result-directory"), view, nil)
	}

	request := ledger.PublicationRequest{Item: item, Kind: kind, View: c.String("view")}
	if body := c.Path("body"); body != "" {
		absolute, err := filepath.Abs(body)
		if err != nil {
			return refusal(fmt.Errorf("resolve the supplied public body %s: %w", body, err))
		}
		request.BodyPath = absolute
	}
	findings, err := readSelectedFindings(c.Path("findings"))
	if err != nil {
		return refusal(err)
	}
	request.Findings = findings
	backend, err := newBackend(repository.Repository)
	if err != nil {
		return refusal(err)
	}
	forge, ok := backend.(ledger.RecoveryForge)
	if !ok {
		return refusal(errors.New("the configured workflow backend does not support forge publication recovery"))
	}
	result, err := ledger.RecoverPublication(c.Context, store, repository.Repository, repository.Root, repository.Remote, request, forge)
	if err != nil {
		return refusal(err)
	}
	// The recovery result's status is authoritative: several result paths retain
	// the pre-attempt view status. Map the observed outcome into the rendered
	// view so the continuation follows the actual condition.
	view := result.View
	view.Status = result.Status
	view.Detail = result.Detail
	return publish(emit, repository, operation, item, kind, c.Path("result-directory"), view, result.Findings)
}

// publish renders the owner skill's specialized packet for one selected view.
// A view with no reusable temporary body binds a private result directory for
// newly authored prose through the existing deferred authoring resource.
func publish(emit func(publicationOutput) error, repository setup.RepositoryContext, operation, item, kind, requestedDirectory string, view ledger.PublicationView, findings []ledger.FindingPublication) error {
	facts, err := publicationFacts(operation, repository, item, kind, requestedDirectory, view)
	if err != nil {
		return emit(publicationRefusal(err))
	}
	packet, err := skilldist.BuildPacket(publicationOwner(kind, view), skilldist.InvocationFacts{Publication: &facts})
	if err != nil {
		return emit(publicationRefusal(fmt.Errorf("render the `%s` publication packet: %w", publicationOwner(kind, view), err)))
	}
	return emit(publicationOutput{Status: facts.View.Status, Reason: facts.View.Detail, Packet: &packet, Findings: findings})
}

// publicationFacts binds every known field of the selected presentation. The
// recovery command always names the selected view; it adds the private result
// directory and public body only when new prose must be authored, so a
// registered current body is reused without a substitute write path.
func publicationFacts(operation string, repository setup.RepositoryContext, item, kind, requestedDirectory string, view ledger.PublicationView) (skilldist.PublicationFacts, error) {
	condition := publicationCondition(operation, view)
	renderView := view
	if condition == skilldist.PublicationProseNeeded || condition == skilldist.PublicationStale {
		// The ledger exposes the registered path even when those bytes are
		// missing or superseded. The renderer keys its reuse continuation off
		// BodyPath, so a view that must be reauthored presents no reusable body.
		renderView.BodyPath = ""
	}
	facts := skilldist.PublicationFacts{
		Operation: operation, Kind: kind, Condition: condition,
		RepositoryRoot: repository.Root, Remote: repository.Remote, Item: item,
		View: renderView,
	}
	for _, reference := range publicationReferences(repository.Root, view) {
		facts.ReferenceCommands = append(facts.ReferenceCommands, reference)
	}
	base := publicationRecoverCommand(repository, item, kind, view.Token)
	if condition == skilldist.PublicationProseNeeded || condition == skilldist.PublicationStale {
		directory, err := publicationResultDirectory(requestedDirectory)
		if err != nil {
			return facts, err
		}
		facts.ResultDirectory = directory
		facts.ResourceCommand = fmt.Sprintf("skl skill --resource reference/publication.md --input result_directory=%s %s", skilldist.ShellQuote(directory), publicationOwner(kind, view))
		facts.RecoverCommand = base + " --result-directory " + skilldist.ShellQuote(directory) + " --body " + skilldist.ShellQuote(filepath.Join(directory, "public.md"))
		return facts, nil
	}
	facts.RecoverCommand = base
	return facts, nil
}

// publicationCondition maps the ledger's explicit presentation status to the
// renderer's recovery condition. The ledger already distinguishes a current
// pending body from a lost, superseded, or satisfied presentation, so the CLI
// honors the reported status directly rather than re-deriving it from the file
// system.
func publicationCondition(operation string, view ledger.PublicationView) skilldist.PublicationCondition {
	switch view.Status {
	case "published", "already-satisfied":
		return skilldist.PublicationSatisfied
	case "stale":
		return skilldist.PublicationStale
	case "ambiguous":
		return skilldist.PublicationAmbiguous
	case "prose-needed":
		return skilldist.PublicationProseNeeded
	case "pending":
		if view.BodyPath == "" {
			return skilldist.PublicationProseNeeded
		}
		return publicationReuseCondition(operation)
	}
	if view.BodyPath != "" {
		return skilldist.PublicationCurrentBody
	}
	return skilldist.PublicationProseNeeded
}

// publicationReuseCondition selects the reuse continuation: inspection names
// the current registered body, while a recovery attempt reports the effect as
// still pending.
func publicationReuseCondition(operation string) skilldist.PublicationCondition {
	if operation == "inspect" {
		return skilldist.PublicationCurrentBody
	}
	return skilldist.PublicationPending
}

// publicationOwner resolves the existing owner skill whose deferred resource
// authors the selected presentation. Pull presentations belong to the phase
// whose committed report is selected.
func publicationOwner(kind string, view ledger.PublicationView) string {
	if kind != "pull" {
		return "propose"
	}
	if view.Report != nil && strings.HasSuffix(view.Report.Path, "/"+ledger.WatchdogPhase+"-report.md") {
		return "watchdog"
	}
	return "implement"
}

// publicationReferences builds the exact private retrieval commands for the
// selected report and every accepted Contract, including the frozen intent.md
// that carries the human-owned Manual Verification obligations.
func publicationReferences(root string, view ledger.PublicationView) []skilldist.PublicationReference {
	var references []skilldist.PublicationReference
	if view.Report != nil {
		references = append(references, publicationReference(root, *view.Report, "selected "+phaseFromReportPath(view.Report.Path)+" report"))
	}
	for _, contract := range view.Contracts {
		references = append(references, publicationReference(root, contract, "accepted Contract"))
	}
	return references
}

func publicationReference(root string, reference ledger.Reference, purpose string) skilldist.PublicationReference {
	return skilldist.PublicationReference{
		Commit: reference.Commit, Path: reference.Path, Purpose: purpose,
		Command: fmt.Sprintf("skl ledger show --repo %s --commit %s --path %s",
			skilldist.ShellQuote(root), skilldist.ShellQuote(reference.Commit), skilldist.ShellQuote(reference.Path)),
	}
}

func phaseFromReportPath(path string) string {
	switch {
	case strings.HasSuffix(path, "/"+ledger.WatchdogPhase+"-report.md"):
		return ledger.WatchdogPhase
	case strings.HasSuffix(path, "/"+ledger.ImplementPhase+"-report.md"):
		return ledger.ImplementPhase
	}
	return "phase"
}

func publicationRecoverCommand(repository setup.RepositoryContext, item, kind, token string) string {
	return fmt.Sprintf("skl publication recover --repo %s --remote %s --item %s --kind %s --view %s",
		skilldist.ShellQuote(repository.Root), skilldist.ShellQuote(repository.Remote),
		skilldist.ShellQuote(item), skilldist.ShellQuote(kind), skilldist.ShellQuote(token))
}

// publicationResultDirectory binds an existing absolute private directory, or
// generates one through the same private result-location convention the
// delivery resources use when no location was supplied.
func publicationResultDirectory(requested string) (string, error) {
	if requested != "" {
		if !filepath.IsAbs(requested) {
			return "", fmt.Errorf("the bound --result-directory %s is not absolute", requested)
		}
		info, err := os.Stat(requested)
		if err != nil || !info.IsDir() {
			return "", fmt.Errorf("the bound --result-directory %s is not an existing directory", requested)
		}
		return requested, nil
	}
	directory, err := os.MkdirTemp("", "skl-publication-")
	if err != nil {
		return "", fmt.Errorf("create a private result directory for newly authored prose: %w", err)
	}
	if err := os.WriteFile(filepath.Join(directory, ".skl-result"), []byte("skl.publication/v1\n"), 0o600); err != nil {
		os.RemoveAll(directory)
		return "", fmt.Errorf("initialize the private result directory %s: %w", directory, err)
	}
	return directory, nil
}

// readSelectedFindings reads an explicit JSON array of selected inline
// findings. Only human-authored selections shaped exactly as the ledger's
// selection type are accepted; no private report body is ever read or
// exported, and a repeated identity is refused before any effect.
func readSelectedFindings(path string) ([]ledger.SelectedFinding, error) {
	if path == "" {
		return nil, nil
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve the selected findings %s: %w", path, err)
	}
	contents, err := os.ReadFile(absolute)
	if err != nil {
		return nil, fmt.Errorf("read the selected findings %s: %w", absolute, err)
	}
	trimmed := strings.TrimSpace(string(contents))
	if !strings.HasPrefix(trimmed, "[") {
		return nil, fmt.Errorf("the selected findings %s must be a JSON array of finding objects", absolute)
	}
	var findings []ledger.SelectedFinding
	decoder := json.NewDecoder(strings.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&findings); err != nil {
		return nil, fmt.Errorf("the selected findings %s must be a JSON array of finding objects: %w", absolute, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("the selected findings %s must hold exactly one JSON array", absolute)
	}
	seen := make(map[string]bool, len(findings))
	for index, finding := range findings {
		switch {
		case strings.TrimSpace(finding.ID) == "":
			return nil, fmt.Errorf("finding %d has no id; every selected finding requires an id, prose, commit, path, line, and side", index+1)
		case strings.TrimSpace(finding.Body) == "":
			return nil, fmt.Errorf("finding %s has no public prose; only human-authored finding text may be selected", finding.ID)
		case strings.TrimSpace(finding.Commit) == "":
			return nil, fmt.Errorf("finding %s has no commit; name the reviewed source revision", finding.ID)
		case strings.TrimSpace(finding.Path) == "":
			return nil, fmt.Errorf("finding %s has no path; name the reviewed code path", finding.ID)
		case finding.Line <= 0:
			return nil, fmt.Errorf("finding %s has no positive line; name the reviewed code line", finding.ID)
		case strings.TrimSpace(finding.Side) == "":
			return nil, fmt.Errorf("finding %s has no side; name LEFT or RIGHT", finding.ID)
		}
		if seen[finding.ID] {
			return nil, fmt.Errorf("finding %s is selected more than once; each finding must be selected once", finding.ID)
		}
		seen[finding.ID] = true
	}
	return findings, nil
}

// publicationRefusal renders any unusable condition with its concrete repair.
func publicationRefusal(err error) publicationOutput {
	out := publicationOutput{
		Status: "fix_required", Reason: err.Error(),
		Repair: "correct the reported condition, then inspect the selected view before any recovery write",
	}
	var refusal *ledger.Refusal
	if errors.As(err, &refusal) {
		out.Reason = refusal.Invariant
		out.Repair = refusal.Repair
	}
	return out
}

func renderPublication(stdout io.Writer, format implementationFormatKind, operation string, out publicationOutput) error {
	if format == formatJSON {
		return json.NewEncoder(stdout).Encode(out)
	}
	var err error
	if out.Packet != nil {
		_, err = fmt.Fprint(stdout, out.Packet.Markdown())
	} else {
		_, err = fmt.Fprintf(stdout, "Status: %s\n", out.Status)
		if err == nil && out.Reason != "" {
			_, err = fmt.Fprintln(stdout, out.Reason)
		}
		if err == nil && out.Repair != "" {
			_, err = fmt.Fprintln(stdout, "Repair:", out.Repair)
		}
	}
	for _, finding := range out.Findings {
		if err != nil {
			break
		}
		line := "Finding " + finding.ID + ": " + finding.Status
		if finding.Detail != "" {
			line += " — " + finding.Detail
		}
		_, err = fmt.Fprintln(stdout, line)
	}
	if err != nil {
		return fmt.Errorf("publication %s established status %s but output delivery failed; preserve any supplied public body and its result directory, then inspect the selected view rather than reauthoring: %w", operation, out.Status, err)
	}
	return nil
}
