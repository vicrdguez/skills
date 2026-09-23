package ledger

// Publication recovery. The ledger owns deterministic selection of the latest
// committed human-facing view, provisional local reservations held only while
// network work is in flight, and the durable receipts that let a repeated
// recovery recognize an already satisfied effect. It never stores public prose
// and never infers Workflow State from Markdown or Git history.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/vicrdguez/skills/github"
)

// Publication kinds and their local outcome statuses.
const (
	publicationKindIssue  = "issue"
	publicationKindParent = "parent"
	publicationKindPull   = "pull"

	publicationPublished        = PublicationPublished
	publicationAlreadySatisfied = PublicationAlreadySatisfied
	publicationPending          = PublicationPending
	publicationProseNeeded      = PublicationProseNeeded
	publicationStale            = PublicationStale
	publicationAmbiguous        = PublicationAmbiguous

	findingSatisfied  = FindingSatisfied
	findingInvalid    = FindingInvalid
	findingUnresolved = FindingUnresolved
)

// findingIdentity accepts the Work-Item-local finding labels Watchdog reports
// carry. The ledger validates identity and anchors without reading report prose.
var findingIdentity = regexp.MustCompile(`^W[0-9]+$`)

// publicationInputs is the canonical selected-view input set. Only
// content-derived facts participate, so an unrelated ledger commit does not
// change the token even though the resolved references name the current head.
type publicationInputs struct {
	Repository string           `json:"repository"`
	Kind       string           `json:"kind"`
	Item       string           `json:"item"`
	Title      string           `json:"title"`
	Branch     string           `json:"branch,omitempty"`
	Lifecycle  string           `json:"lifecycle,omitempty"`
	Phase      string           `json:"phase,omitempty"`
	Source     SourceRevisions  `json:"source,omitempty"`
	Report     string           `json:"report,omitempty"`
	Contracts  []string         `json:"contracts,omitempty"`
	Attachment *ForgeAttachment `json:"attachment,omitempty"`
}

func (in publicationInputs) token() string {
	encoded, err := json.Marshal(in)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func digestOf(contents []byte) string {
	sum := sha256.Sum256(contents)
	return hex.EncodeToString(sum[:])
}

// publicationSelection is one resolved committed publication view together
// with the unexported facts recovery and settlement need.
type publicationSelection struct {
	store             *Store
	head              string
	project           string
	repository        github.RepositoryID
	identity          string
	proposal          string
	slice             string
	item              string
	kind              string
	state             SliceState
	meta              ProposalMeta
	directory         string
	proposalDirectory string
	title             string
	branch            string
	phase             string
	report            *Reference
	reportDigest      string
	source            SourceRevisions
	approved          bool
	contractRefs      []Reference
	contractDigests   []string
	token             string
	attachment        *ForgeAttachment
	parent            *ForgeAttachment
	children          []ForgeAttachment
	body              *PublicationBody
	pending           *PublicationNote
	publishedSource   string
}

// InspectPublication resolves the latest committed issue, parent, or pull
// publication view of one Work Item without mutating the ledger. The returned
// view carries the current exact references and a token that only changes when
// its selected input bytes change.
func InspectPublication(s *Store, repository github.RepositoryID, item, kind string) (PublicationView, error) {
	selection, err := selectPublication(s, repository, item, kind)
	if err != nil {
		return PublicationView{}, err
	}
	status, detail := selection.localStatus()
	return selection.view(status, detail), nil
}

// RecoverPublication attempts publication of the selected latest committed
// view. It reserves briefly, performs the forge call outside the ledger
// mutation lock, revalidates the view through the supplied Guard, and records
// only observable receipts without clearing newer pending work.
func RecoverPublication(ctx context.Context, s *Store, repository github.RepositoryID, root, remote string, request PublicationRequest, forge RecoveryForge) (PublicationResult, error) {
	if forge == nil {
		return PublicationResult{}, refuse(
			"publication recovery requires an available forge attachment surface",
			"construct the configured forge adapter and retry",
		)
	}
	selection, err := selectPublication(s, repository, request.Item, request.Kind)
	if err != nil {
		return PublicationResult{}, err
	}
	result := PublicationResult{View: selection.view(publicationPending, "")}
	if request.View != "" && request.View != selection.token {
		result.Status = publicationStale
		result.Detail = "the supplied view token does not identify the selected committed view; inspect the current view and resubmit"
		return result, nil
	}
	if request.ReconcileReservation && request.View == "" {
		return PublicationResult{}, refuse("reservation reconciliation requires the inspected --view token", "inspect the current publication and retry with --view and --reconcile-reservation after the former publisher exits")
	}
	// The process-scoped lease excludes live normal publishers and recoveries
	// without holding the brief ledger mutation lock over network I/O.
	leaseSlice := selection.slice
	if selection.kind == publicationKindParent {
		leaseSlice = ""
	}
	leases, err := s.publicationLeases(publicationLeaseName(selection.identity, selection.proposal, leaseSlice, selection.kind))
	if err != nil {
		result.Status, result.Detail = publicationPending, err.Error()+"; wait for the active publisher to finish and inspect again"
		return result, nil
	}
	defer releasePublicationLeases(leases)
	if request.ReconcileReservation && !selection.hasReservation() {
		return PublicationResult{}, refuse("the selected publication has no interrupted reservation", "inspect the current view and use ordinary recovery")
	}
	if len(request.Findings) > 0 && request.Kind != publicationKindPull {
		return PublicationResult{}, refuse(
			"inline findings apply only to pull publication",
			"select kind pull or omit the findings",
		)
	}
	if selection.kind == publicationKindPull && selection.phase == "" {
		result.Status = publicationAmbiguous
		result.Detail = "no explicit latest phase is recorded for this Work Item; recovery cannot establish the presented result without guessing"
		return result, nil
	}
	if selection.pending != nil && selection.body == nil {
		switch selection.pending.Status {
		case IssueUnresolved, issueReserved:
			result.Status = publicationAmbiguous
			result.Detail = "an earlier attempt's outcome is unconfirmed and no original body identity is available: " + selection.pending.Detail
			return result, nil
		}
	}
	bodySatisfied := selection.attachment != nil && selection.pending == nil && !selection.hasReservation()
	if bodySatisfied && len(request.Findings) == 0 && request.BodyPath == "" {
		result.Status = publicationAlreadySatisfied
		result.Detail = "the recorded forge attachment already satisfies this publication"
		return result, nil
	}
	var resolved resolvedBody
	groupingOnly := selection.kind == publicationKindIssue && selection.attachment != nil && selection.body == nil && selection.state.Publication != nil && selection.state.Publication.Grouping != nil
	if (!bodySatisfied && !groupingOnly) || request.BodyPath != "" {
		var failure *publicationFailure
		resolved, failure = resolveRecoveryBody(selection, request)
		if failure != nil {
			result.Status = failure.Status
			result.Detail = failure.Detail
			return result, nil
		}
	}
	valid, findingResults, err := selection.validateFindings(request.Findings)
	if err != nil {
		return PublicationResult{}, err
	}
	result.Findings = findingResults
	if bodySatisfied && request.BodyPath == "" && len(valid) == 0 {
		result.Status = publicationAlreadySatisfied
		return result, nil
	}

	var record *PublicationBody
	if resolved.path != "" || resolved.digest != "" {
		record = &PublicationBody{Path: resolved.path, SHA256: resolved.digest, View: selection.token}
	}
	originalDigest := resolved.digest
	if selection.mayHaveCreated() && selection.body != nil {
		originalDigest = selection.body.SHA256
		if selection.body.OriginalSHA256 != "" {
			originalDigest = selection.body.OriginalSHA256
		}
		if record != nil {
			record.OriginalSHA256 = originalDigest
		}
	}
	if err := s.reserveRecovery(selection, record, request.ReconcileReservation); err != nil {
		result.Status = publicationPending
		result.Detail = err.Error()
		return result, nil
	}
	presentationBody := &resolved.contents
	if resolved.identityOnly || record == nil {
		presentationBody = nil
	}
	presentation := RecoveryPresentation{
		Kind:               request.Kind,
		Number:             selection.knownNumber(),
		Title:              selection.title,
		Body:               presentationBody,
		Branch:             selection.branch,
		Head:               selection.source.Head,
		Reviewed:           selection.source.Reviewed,
		SourceRoot:         root,
		Target:             selection.source.Target,
		Approved:           selection.approved,
		OriginalBodySHA256: originalDigest,
		MayHaveCreated:     selection.mayHaveCreated() || resolved.identityOnly,
		ObserveOnly:        resolved.identityOnly || request.ReconcileReservation,
		Parent:             selection.parent,
		Children:           selection.children,
		Findings:           valid,
		Guard:              func() error { return s.recoveryGuard(selection) },
		PrepareSource:      func(observedHead string) error { return s.prepareRecoverySource(selection, root, remote, observedHead) },
	}
	receipt, err := forge.RecoverPresentation(ctx, presentation)
	if err != nil {
		uncertain := receipt.Status == publicationAmbiguous || selection.mayHaveCreated() || isUnknownOutcome(err)
		receipt.Status, receipt.Detail = publicationPending, err.Error()
		if uncertain && receipt.Number == 0 {
			receipt.Status = publicationAmbiguous
		}
	}
	settled, settleErr := s.settleRecovery(selection, record, receipt, valid, findingResults)
	if settleErr != nil {
		return PublicationResult{}, settleErr
	}
	return settled, nil
}

// RememberDeliveryBody registers the temporary public body of one delivery
// result so a pending presentation can reuse it without reauthoring. It is a
// no-op when the supplied result is not the current committed view; the normal
// publication path then refuses the stale presentation on its own.
func RememberDeliveryBody(s *Store, repository github.RepositoryID, result *DeliveryResult, absolutePath string) error {
	if result == nil {
		return refuse("no delivery result was supplied", "submit a committed phase result before remembering its public body")
	}
	if strings.TrimSpace(absolutePath) == "" {
		return nil
	}
	contents, err := os.ReadFile(absolutePath)
	if err != nil {
		return refuse(
			"the temporary public body "+absolutePath+" is unavailable",
			"correct the path or author the public body again before submitting",
		)
	}
	selection, err := selectPublication(s, repository, result.Item, publicationKindPull)
	if err != nil {
		return err
	}
	if !sameReportContent(s, selection, &result.Report) {
		return nil
	}
	record := &PublicationBody{Path: absolutePath, SHA256: digestOf(contents), View: selection.token}
	return s.withMutation(func() error {
		current, err := selectPublication(s, repository, result.Item, publicationKindPull)
		if err != nil {
			return err
		}
		if !sameReportContent(s, current, &result.Report) {
			return nil
		}
		publication := publicationStateOf(current.state)
		publication.PullBody = record
		current.state.Publication = publication
		return s.commitRecoveryState(current, "remember public body "+repository.Name+"/"+result.Item)
	})
}

// selectPublication resolves one committed view. Every read is at the current
// committed head; identity and content facts come from the selected records
// alone, so unrelated commits never stale the selection.
func selectPublication(s *Store, repository github.RepositoryID, item, kind string) (*publicationSelection, error) {
	switch kind {
	case publicationKindIssue, publicationKindParent, publicationKindPull:
	default:
		return nil, refuse(
			"unknown publication kind "+strconv.Quote(kind),
			"use issue, parent, or pull",
		)
	}
	proposal, slice, found := strings.Cut(item, "/")
	if !found || !ValidRecordName(proposal) || !ValidRecordName(slice) {
		return nil, refuse(
			"Work Item reference "+item+" is not a proposal/slice identity",
			"use the accepted proposal/slice identity the acceptance reported",
		)
	}
	head, err := s.head()
	if err != nil {
		return nil, err
	}
	var project ProjectIdentity
	if err := readJSONAt(s, head, filepath.ToSlash(filepath.Join(projectsRoot, repository.Name, "project.json")), &project); err != nil || project.Repository != repository.Owner+"/"+repository.Name {
		return nil, refuse(
			"no matching committed Project for "+repository.Owner+"/"+repository.Name,
			"configure the correct private ledger and accept the Project before publication recovery",
		)
	}
	directory := filepath.ToSlash(filepath.Join(projectsRoot, repository.Name, "proposals", proposal, slice))
	proposalDirectory := filepath.ToSlash(filepath.Join(projectsRoot, repository.Name, "proposals", proposal))
	var state SliceState
	if err := readJSONAt(s, head, directory+"/state.json", &state); err != nil {
		return nil, refuse(
			"selected Work Item "+item+" has no readable committed state",
			"inspect the selected record with human direction; a forge object is not a substitute",
		)
	}
	var meta ProposalMeta
	if err := readJSONAt(s, head, proposalDirectory+"/proposal.json", &meta); err != nil {
		return nil, refuse(
			"proposal record "+proposal+" has no readable proposal.json",
			"repair or restore the damaged record with human direction",
		)
	}
	selection := &publicationSelection{
		store: s, head: head, project: repository.Name, repository: repository,
		identity: repository.Owner + "/" + repository.Name,
		proposal: proposal, slice: slice, item: item, kind: kind,
		state: state, meta: meta, directory: directory, proposalDirectory: proposalDirectory,
	}
	switch kind {
	case publicationKindIssue:
		selection.title = state.Title
		selection.attachment = state.Issue
		selection.parent = meta.ParentIssue
		if state.Publication != nil {
			selection.body = state.Publication.IssueBody
			selection.pending = state.Publication.Issue
			if selection.pending == nil {
				selection.pending = state.Publication.Grouping
			}
		}
		refs, digests, err := s.contractDigestsAt(head, directory)
		if err != nil {
			return nil, err
		}
		selection.contractRefs, selection.contractDigests = refs, digests
	case publicationKindParent:
		if strings.TrimSpace(meta.ParentTitle) == "" {
			return nil, refuse(
				"proposal "+proposal+" has no coordination parent issue",
				"select an issue or pull publication for this single-slice Work Item",
			)
		}
		selection.title = strings.TrimSpace(meta.ParentTitle)
		selection.attachment = meta.ParentIssue
		selection.pending = meta.ParentPublication
		selection.body = meta.ParentBody
		children, err := s.childAttachmentsAt(head, repository.Name, proposal)
		if err != nil {
			return nil, err
		}
		selection.children = children
		refs, digests, err := s.proposalContractRefsAt(head, repository.Name, proposal)
		if err != nil {
			return nil, err
		}
		selection.contractRefs, selection.contractDigests = refs, digests
	case publicationKindPull:
		selection.title = state.Title
		selection.branch = state.Branch
		selection.attachment = state.Submission
		selection.parent = meta.ParentIssue
		if state.Publication != nil {
			selection.phase = state.Publication.Phase
			selection.body = state.Publication.PullBody
			selection.pending = state.Publication.Pull
			selection.publishedSource = state.Publication.PublishedSource
		}
		refs, digests, err := s.contractDigestsAt(head, directory)
		if err != nil {
			return nil, err
		}
		selection.contractRefs, selection.contractDigests = refs, digests
		if selection.phase != "" {
			if selection.phase != ImplementPhase && selection.phase != WatchdogPhase {
				return nil, refuse(
					"recorded latest phase "+strconv.Quote(selection.phase)+" is not an accepted delivery phase",
					"repair the selected state record with human direction",
				)
			}
			reference := Reference{Commit: head, Path: directory + "/" + selection.phase + "-report.md"}
			raw, err := showPath(s, head, reference.Path)
			if err != nil {
				return nil, refuse(
					"recorded latest "+selection.phase+" report is unavailable: "+err.Error(),
					"restore the committed report or repair the state record with human direction",
				)
			}
			report, _, err := ParseReport(selection.phase, []byte(raw))
			if err != nil {
				return nil, refuse(
					"recorded latest "+selection.phase+" report is incompatible: "+err.Error(),
					"use a compatible CLI or repair the selected report with human direction",
				)
			}
			selection.report = &reference
			selection.reportDigest = digestOf([]byte(raw))
			selection.source = report.Source
			selection.approved = report.Outcome == outcomePass
		}
	}
	attachments := slices.Clone(selection.children)
	for _, attachment := range []*ForgeAttachment{selection.attachment, selection.parent} {
		if attachment != nil {
			attachments = append(attachments, *attachment)
		}
	}
	for _, attachment := range attachments {
		if attachment.Repository != selection.identity || attachment.Number <= 0 {
			return nil, refuse("a recorded forge attachment does not belong to the selected repository", "repair the attachment rather than reassigning an object")
		}
	}
	selection.token = selection.computeToken()
	return selection, nil
}

func (sel *publicationSelection) computeToken() string {
	inputs := publicationInputs{
		Repository: sel.identity,
		Kind:       sel.kind,
		Title:      sel.title,
		Contracts:  append([]string(nil), sel.contractDigests...),
		Attachment: sel.attachment,
	}
	switch sel.kind {
	case publicationKindParent:
		inputs.Item = sel.proposal
	case publicationKindPull:
		inputs.Item = sel.item
		inputs.Branch = sel.branch
		inputs.Lifecycle = sel.state.State
		inputs.Phase = sel.phase
		inputs.Source = sel.source
		inputs.Report = sel.reportDigest
	default:
		inputs.Item = sel.item
	}
	return inputs.token()
}

func (sel *publicationSelection) view(status, detail string) PublicationView {
	view := PublicationView{
		Project: sel.project, Repository: sel.identity, Proposal: sel.proposal, Item: sel.item,
		Kind: sel.kind, Token: sel.token, Title: sel.title, Branch: sel.branch, State: sel.state.State,
		Source: sel.source, Phase: sel.phase, Report: sel.report, Contracts: sel.contractRefs,
		Attachment: sel.attachment, Parent: sel.parent, Children: sel.children,
		Status: status, Detail: detail,
	}
	if sel.body != nil {
		view.BodyPath = sel.body.Path
	}
	return view
}

// localStatus classifies the selected view from committed metadata and the
// registered temporary body alone; it performs no network observation. A
// recorded attachment satisfies only the view it was recorded for: a newer
// pending presentation with a registered, lost, or superseded body takes
// precedence over that older attachment.
func (sel *publicationSelection) localStatus() (string, string) {
	if sel.kind == publicationKindPull && sel.hasReservation() {
		return publicationAmbiguous, "an earlier pull publication reserved this presentation without recording its outcome; after confirming the publisher exited, use recover with --reconcile-reservation and this --view token to observe without forge writes"
	}
	if sel.pending != nil {
		switch sel.pending.Status {
		case IssueUnresolved:
			return publicationAmbiguous, "an earlier attempt's outcome is unconfirmed: " + sel.pending.Detail
		case issueReserved:
			return publicationAmbiguous, "an earlier attempt reserved this publication without recording its outcome: " + sel.pending.Detail + "; after confirming the publisher exited, use recover with --reconcile-reservation and this --view token to observe without forge writes"
		}
	}
	if sel.kind == publicationKindPull && sel.phase == "" {
		return publicationProseNeeded, "no committed delivery result with an explicit recorded phase is available; complete delivery before publishing"
	}
	if sel.body != nil {
		if sel.body.View != sel.token {
			return publicationStale, "the registered public body was authored for a different selected view; author a fresh body for the current view"
		}
		if _, err := sel.bodyContents(); err != nil {
			return publicationProseNeeded, err.Error()
		}
		return publicationPending, "the publication is pending and its registered temporary body is current"
	}
	if sel.attachment != nil && sel.pending == nil {
		return publicationPublished, "the recorded forge attachment identifies this publication"
	}
	return publicationProseNeeded, "no temporary public body is registered for this view; author one and resubmit with --body and the current --view token"
}

func (sel *publicationSelection) bodyContents() (string, error) {
	if sel.body == nil {
		return "", refuse(
			"no temporary public body is registered for the selected view",
			"author a descriptive public body and resubmit with --body and the current --view token",
		)
	}
	contents, err := os.ReadFile(sel.body.Path)
	if err != nil {
		return "", refuse(
			"the registered temporary public body "+sel.body.Path+" is unavailable",
			"author a fresh public body and resubmit with --body and the current --view token",
		)
	}
	if digestOf(contents) != sel.body.SHA256 {
		return "", refuse(
			"the registered temporary public body "+sel.body.Path+" changed since it was registered",
			"author a fresh public body and resubmit with --body and the current --view token",
		)
	}
	return string(contents), nil
}

func (sel *publicationSelection) knownNumber() int {
	if sel.attachment != nil {
		return sel.attachment.Number
	}
	return 0
}

// mayHaveCreated reports whether an unconfirmed create may already exist. Only
// an unresolved attempt or a held reservation names a write that was actually
// dispatched; a generic pending presentation (an unavailable backend, a pending
// source sync, or a definite refusal before any write) is observable absence
// and permits an initial create.
func (sel *publicationSelection) mayHaveCreated() bool {
	if sel.attachment != nil {
		return false
	}
	if sel.kind == publicationKindPull && sel.hasReservation() {
		return true
	}
	if sel.pending == nil {
		return false
	}
	switch sel.pending.Status {
	case IssueUnresolved, issueReserved:
		return true
	}
	return false
}

// resolvedBody is one temporary public body that passed reuse validation.
// identityOnly marks a registered body whose original bytes are gone while its
// exact title-and-digest identity remains: the adapter may observe and adopt an
// existing object, but must not create one.
type resolvedBody struct {
	path         string
	digest       string
	contents     string
	identityOnly bool
}

// publicationFailure is a local classification that is reported as a recovery
// result rather than as a refusal: the operator can act on it without retrying
// an unchanged request.
type publicationFailure struct {
	Status string
	Detail string
}

func (f *publicationFailure) Error() string { return f.Status + ": " + f.Detail }

func resolveRecoveryBody(sel *publicationSelection, request PublicationRequest) (resolvedBody, *publicationFailure) {
	if strings.TrimSpace(request.BodyPath) != "" {
		if request.View == "" {
			return resolvedBody{}, &publicationFailure{
				Status: publicationStale,
				Detail: "a supplied public body requires the exact current view token inspect returned",
			}
		}
		contents, err := os.ReadFile(request.BodyPath)
		if err != nil {
			return resolvedBody{}, &publicationFailure{
				Status: publicationStale,
				Detail: "the supplied public body " + request.BodyPath + " is unavailable: " + err.Error(),
			}
		}
		return resolvedBody{path: request.BodyPath, digest: digestOf(contents), contents: string(contents)}, nil
	}
	if sel.body == nil {
		return resolvedBody{}, &publicationFailure{
			Status: publicationProseNeeded,
			Detail: "no temporary public body is registered for this view; author one and resubmit with --body and the current --view token",
		}
	}
	if sel.body.View != sel.token {
		return resolvedBody{}, &publicationFailure{
			Status: publicationStale,
			Detail: "the registered public body describes a superseded selected view; author a fresh body and resubmit with the current --view token",
		}
	}
	contents, err := os.ReadFile(sel.body.Path)
	if err != nil {
		if identity := sel.digestOnlyIdentity(); identity {
			return resolvedBody{path: sel.body.Path, digest: sel.body.SHA256, identityOnly: true}, nil
		}
		return resolvedBody{}, &publicationFailure{
			Status: publicationProseNeeded,
			Detail: "the registered public body " + sel.body.Path + " is unavailable; author a fresh body and resubmit",
		}
	}
	if digestOf(contents) != sel.body.SHA256 {
		if identity := sel.digestOnlyIdentity(); identity {
			return resolvedBody{path: sel.body.Path, digest: sel.body.SHA256, identityOnly: true}, nil
		}
		return resolvedBody{}, &publicationFailure{
			Status: publicationStale,
			Detail: "the registered public body changed since it was registered; author a fresh body and resubmit",
		}
	}
	return resolvedBody{path: sel.body.Path, digest: sel.body.SHA256, contents: string(contents)}, nil
}

// digestOnlyIdentity reports whether a registered issue or parent body keeps a
// usable exact create identity even when its temporary bytes are gone. Pull
// presentation still requires current prose, so it never observes without one.
func (sel *publicationSelection) digestOnlyIdentity() bool {
	if sel.kind == publicationKindPull || sel.body == nil || sel.pending == nil {
		return false
	}
	return sel.pending.Status == IssueUnresolved || sel.pending.Status == issueReserved
}

// validateFindings checks explicit inline selections against the current
// schema-1 reviewed input and their supplied anchors. Invalid or already
// satisfied selections are returned separately from body publication and are
// never sent to the adapter.
func (sel *publicationSelection) validateFindings(findings []SelectedFinding) ([]SelectedFinding, []FindingPublication, error) {
	if len(findings) == 0 {
		return nil, nil, nil
	}
	var valid []SelectedFinding
	var published []FindingPublication
	for _, finding := range findings {
		switch {
		case !findingIdentity.MatchString(finding.ID):
			published = append(published, FindingPublication{ID: finding.ID, Status: findingInvalid, Detail: "a finding identity must be a Work-Item-local W<n> identity"})
			continue
		case strings.TrimSpace(finding.Body) == "":
			published = append(published, FindingPublication{ID: finding.ID, Status: findingInvalid, Detail: "an inline finding requires human-facing prose"})
			continue
		case !validLedgerPath(finding.Path):
			published = append(published, FindingPublication{ID: finding.ID, Status: findingInvalid, Detail: "the finding anchor path is not a clean relative reviewed-code path"})
			continue
		case finding.Line <= 0:
			published = append(published, FindingPublication{ID: finding.ID, Status: findingInvalid, Detail: "the finding anchor requires a positive line"})
			continue
		case finding.Side != "LEFT" && finding.Side != "RIGHT":
			published = append(published, FindingPublication{ID: finding.ID, Status: findingInvalid, Detail: "the finding anchor side must be LEFT or RIGHT"})
			continue
		}
		if sel.kind != publicationKindPull || sel.phase != WatchdogPhase || sel.report == nil {
			published = append(published, FindingPublication{ID: finding.ID, Status: findingUnresolved, Detail: "inline findings require the current committed watchdog review"})
			continue
		}
		if sel.source.Reviewed == "" || finding.Commit != sel.source.Reviewed {
			published = append(published, FindingPublication{ID: finding.ID, Status: findingUnresolved, Detail: "the finding anchor must name the current reviewed source revision " + sel.source.Reviewed})
			continue
		}
		if sel.hasFindingReceipt(finding) {
			published = append(published, FindingPublication{ID: finding.ID, Status: findingSatisfied, Detail: "this exact finding was already published for the current reviewed revision"})
			continue
		}
		valid = append(valid, finding)
	}
	return valid, published, nil
}

func (sel *publicationSelection) hasFindingReceipt(finding SelectedFinding) bool {
	if sel.state.Publication == nil {
		return false
	}
	wanted := findingReceiptOf(finding, findingSatisfied, sel.knownNumber())
	for _, receipt := range sel.state.Publication.Findings {
		if receipt == wanted {
			return true
		}
	}
	return false
}

func findingReceiptOf(finding SelectedFinding, status string, submission int) FindingReceipt {
	return FindingReceipt{
		ID: finding.ID, Body: digestOf([]byte(finding.Body)), Commit: finding.Commit, Submission: submission,
		Path: finding.Path, Line: finding.Line, Side: finding.Side, Status: status,
	}
}

// reserveRecovery holds one provisional local reservation across the network
// call. A competing writer observes the reservation and never performs the same
// list-then-create race.
func (sel *publicationSelection) hasReservation() bool {
	if sel.kind == publicationKindPull {
		return sel.state.Publication != nil && sel.state.Publication.Active != nil
	}
	return sel.pending != nil && sel.pending.Status == issueReserved
}

func (s *Store) reserveRecovery(sel *publicationSelection, record *PublicationBody, reconcile bool) error {
	return s.withMutation(func() error {
		if err := s.requireReconciled(); err != nil {
			return err
		}
		current, err := selectPublication(s, sel.repository, sel.item, sel.kind)
		if err != nil {
			return err
		}
		if current.token != sel.token || !samePublicationAssociations(current, sel) {
			return refuse(
				"the selected publication view changed before recovery could reserve it",
				"inspect the current view and resubmit the recovery",
			)
		}
		if reconcile {
			if !current.hasReservation() {
				return refuse("the interrupted reservation changed before observation", "inspect the current view before continuing")
			}
			if sel.kind == publicationKindPull {
				active := current.state.Publication.Active
				if active == nil || active.Path != current.report.Path {
					return refuse("the interrupted presentation references a different report", "inspect the recorded reservation and current view")
				}
				bytes, err := showPath(s, active.Commit, active.Path)
				if err != nil || digestOf([]byte(bytes)) != current.reportDigest {
					return refuse("the interrupted presentation does not match the latest report", "preserve the newer view and inspect the recorded reservation")
				}
			}
			// Keep its original body identity and reservation intact until the
			// read-only forge observation is settled under the lease.
			return nil
		}
		switch sel.kind {
		case publicationKindIssue:
			if current.state.Publication != nil && current.state.Publication.Issue != nil && current.state.Publication.Issue.Status == issueReserved {
				return refuse(
					"another attempt has reserved this issue publication",
					"inspect that attempt before retrying; no duplicate or guessed create is authorized",
				)
			}
			publication := publicationStateOf(current.state)
			publication.Issue = &PublicationNote{Status: issueReserved, Detail: "publication recovery reserved before network work"}
			publication.IssueBody = record
			current.state.Publication = publication
			return s.commitRecoveryState(current, "reserve issue publication recovery "+sel.repository.Name+"/"+sel.item)
		case publicationKindParent:
			if current.meta.ParentPublication != nil && current.meta.ParentPublication.Status == issueReserved {
				return refuse(
					"another attempt has reserved this parent publication",
					"inspect that attempt before retrying; no duplicate or guessed create is authorized",
				)
			}
			current.meta.ParentPublication = &PublicationNote{Status: issueReserved, Detail: "publication recovery reserved before network work"}
			current.meta.ParentBody = record
			return s.commitRecoveryProposal(current)
		default:
			if current.state.Publication != nil && current.state.Publication.Active != nil {
				return refuse(
					"another presentation attempt is reserved",
					"inspect that attempt before retrying",
				)
			}
			if current.report == nil {
				return refuse(
					"the selected view has no committed report to present",
					"inspect and repair the selected record with human direction",
				)
			}
			publication := publicationStateOf(current.state)
			publication.Active = current.report
			publication.PullBody = record
			current.state.Publication = publication
			return s.commitRecoveryState(current, "reserve pull publication recovery "+sel.repository.Name+"/"+sel.item)
		}
	})
}

// settleRecovery clears the reservation and records only observable receipts.
// The current view is compared by content; when a newer view was committed
// while the effect was in flight, an observable attachment is retained but the
// newer pending work is left untouched.
func (s *Store) settleRecovery(sel *publicationSelection, record *PublicationBody, receipt RecoveryReceipt, validFindings []SelectedFinding, findingResults []FindingPublication) (PublicationResult, error) {
	status := normalizeReceiptStatus(receipt.Status)
	var stillCurrent bool
	err := s.withMutation(func() error {
		current, err := selectPublication(s, sel.repository, sel.item, sel.kind)
		if err != nil {
			return err
		}
		stillCurrent = current.token == sel.token && samePublicationAssociations(current, sel)
		number := receipt.Number
		if number == 0 && current.attachment != nil {
			number = current.attachment.Number
		}
		switch sel.kind {
		case publicationKindIssue:
			if current.state.Publication == nil || current.state.Publication.Issue == nil || current.state.Publication.Issue.Status != issueReserved {
				return refuse(
					"the issue recovery reservation changed while the forge effect was in flight",
					"inspect the current publication record and preserve every observable attachment",
				)
			}
			publication := publicationStateOf(current.state)
			if number > 0 {
				attachment := &ForgeAttachment{Repository: current.identity, Number: number}
				if current.state.Issue != nil && !sameAttachment(current.state.Issue, attachment) {
					return refuse(
						"a concurrent publication recorded a different issue",
						"preserve both attachments and reconcile the Work Item record with human direction",
					)
				}
				current.state.Issue = attachment
				current.attachment = attachment
				if stillCurrent && record != nil {
					record.View = current.computeToken()
				}
			}
			if stillCurrent && isSatisfiedStatus(status) && current.state.Issue != nil {
				publication.Issue = nil
				publication.Grouping = nil
				publication.IssueBody = nil
			} else {
				publication.Issue = &PublicationNote{Status: pendingNoteStatus(status), Detail: receipt.Detail}
				publication.IssueBody = record
			}
			current.state.Publication = publication
			return s.commitRecoveryState(current, "record issue publication recovery "+current.repository.Name+"/"+current.item)
		case publicationKindParent:
			if current.meta.ParentPublication == nil || current.meta.ParentPublication.Status != issueReserved {
				return refuse(
					"the parent recovery reservation changed while the forge effect was in flight",
					"inspect the current publication record and preserve every observable attachment",
				)
			}
			if number > 0 {
				attachment := &ForgeAttachment{Repository: current.identity, Number: number}
				if current.meta.ParentIssue != nil && !sameAttachment(current.meta.ParentIssue, attachment) {
					return refuse(
						"a concurrent publication recorded a different parent issue",
						"preserve both attachments and reconcile the proposal record with human direction",
					)
				}
				current.meta.ParentIssue = attachment
				current.attachment = attachment
				if stillCurrent && record != nil {
					record.View = current.computeToken()
				}
			}
			if stillCurrent && isSatisfiedStatus(status) && current.meta.ParentIssue != nil {
				current.meta.ParentPublication = nil
				current.meta.ParentBody = nil
			} else {
				current.meta.ParentPublication = &PublicationNote{Status: pendingNoteStatus(status), Detail: receipt.Detail}
				current.meta.ParentBody = record
			}
			return s.commitRecoveryProposal(current)
		default:
			if current.state.Publication == nil || !sameReference(current.state.Publication.Active, sel.reservationReport()) {
				return refuse(
					"the presentation reservation changed while the forge effect was in flight",
					"inspect the current publication record and preserve every observable attachment",
				)
			}
			publication := publicationStateOf(current.state)
			publication.Active = nil
			if number > 0 {
				attachment := &ForgeAttachment{Repository: current.identity, Number: number}
				if current.state.Submission != nil && !sameAttachment(current.state.Submission, attachment) {
					return refuse(
						"a concurrent delivery recorded a different Submission",
						"preserve both attachments and reconcile the Work Item record with human direction",
					)
				}
				current.state.Submission = attachment
				current.attachment = attachment
				if stillCurrent && record != nil {
					record.View = current.computeToken()
				}
			}
			switch {
			case stillCurrent && isSatisfiedStatus(status) && current.state.Submission != nil:
				publication.Pull = nil
				publication.Source = nil
				publication.PullBody = nil
				if current.source.Head != "" {
					publication.PublishedSource = current.source.Head
				}
			case !stillCurrent:
				if publication.Pull == nil {
					publication.Pull = &PublicationNote{Status: IssuePending, Detail: "a newer selected view still needs presentation"}
				}
				// The later handoff owns its body registration, including absence.
				// Retain the receipt above, not this attempt's superseded prose.
			default:
				publication.Pull = &PublicationNote{Status: pendingNoteStatus(status), Detail: receipt.Detail}
				publication.PullBody = record
			}
			publication.Findings = mergeFindingReceipts(publication.Findings, validFindings, receipt.Findings, number)
			current.state.Publication = publication
			return s.commitRecoveryState(current, "record pull publication recovery "+current.repository.Name+"/"+current.item)
		}
	})
	if err != nil {
		return PublicationResult{}, err
	}
	after, err := selectPublication(s, sel.repository, sel.item, sel.kind)
	if err != nil {
		return PublicationResult{}, err
	}
	detail := receipt.Detail
	resultStatus := status
	if !stillCurrent {
		resultStatus = publicationPending
		detail = "a newer selected view was recorded while the forge effect was in flight; any observable receipt was retained but the newer view still needs recovery"
	} else if !isSatisfiedStatus(status) && detail == "" {
		detail = "the presentation remains pending"
	}
	result := PublicationResult{Status: resultStatus, Detail: detail, View: after.view(resultStatus, detail)}
	result.Findings = append(append([]FindingPublication(nil), findingResults...), receipt.Findings...)
	return result, nil
}

func isSatisfiedStatus(status string) bool {
	return status == publicationPublished || status == publicationAlreadySatisfied
}

func normalizeReceiptStatus(status PublicationStatus) string {
	switch status {
	case publicationPublished, publicationAlreadySatisfied, publicationProseNeeded, publicationStale, publicationAmbiguous, publicationPending:
		return string(status)
	default:
		return publicationPending
	}
}

func pendingNoteStatus(status string) string {
	if status == publicationAmbiguous {
		return IssueUnresolved
	}
	return IssuePending
}

func mergeFindingReceipts(existing []FindingReceipt, findings []SelectedFinding, results []FindingPublication, submission int) []FindingReceipt {
	satisfied := make(map[string]bool, len(results))
	for _, result := range results {
		if result.Status == findingSatisfied || result.Status == publicationAlreadySatisfied {
			satisfied[result.ID] = true
		}
	}
	receipts := append([]FindingReceipt(nil), existing...)
	for _, finding := range findings {
		if !satisfied[finding.ID] {
			continue
		}
		receipt := findingReceiptOf(finding, findingSatisfied, submission)
		duplicate := false
		for _, known := range receipts {
			if known == receipt {
				duplicate = true
				break
			}
		}
		if !duplicate {
			receipts = append(receipts, receipt)
		}
	}
	return receipts
}

// recoveryGuard revalidates the reserved view before an adapter-side effect.
func (sel *publicationSelection) reservationReport() *Reference {
	if sel.kind == publicationKindPull && sel.state.Publication != nil && sel.state.Publication.Active != nil {
		return sel.state.Publication.Active
	}
	return sel.report
}

func (s *Store) recoveryGuard(sel *publicationSelection) error {
	current, err := selectPublication(s, sel.repository, sel.item, sel.kind)
	if err != nil {
		return err
	}
	if current.token != sel.token || !samePublicationAssociations(current, sel) {
		return refuse(
			"the selected publication view changed while recovery was in flight",
			"preserve the newer local view and recover it separately",
		)
	}
	switch sel.kind {
	case publicationKindIssue:
		if current.state.Publication == nil || current.state.Publication.Issue == nil || current.state.Publication.Issue.Status != issueReserved {
			return refuse("the issue recovery reservation is no longer held", "inspect the current publication record")
		}
	case publicationKindParent:
		if current.meta.ParentPublication == nil || current.meta.ParentPublication.Status != issueReserved {
			return refuse("the parent recovery reservation is no longer held", "inspect the current publication record")
		}
	default:
		if current.state.Publication == nil || !sameReference(current.state.Publication.Active, sel.reservationReport()) {
			return refuse("the presentation reservation is no longer held", "inspect the current publication record")
		}
	}
	return nil
}

// prepareRecoverySource establishes an expected source lag from normal recorded
// receipts, then reuses the ordinary non-force source publication path. It is
// invoked only after the adapter has validated the attached ownership; it never
// replaces branch history and refuses an unexpected observed head.
func (s *Store) prepareRecoverySource(sel *publicationSelection, root, remote, observedHead string) error {
	if sel.kind != publicationKindPull {
		return refuse("source catch-up applies only to pull publication", "select kind pull")
	}
	if err := s.recoveryGuard(sel); err != nil {
		return err
	}
	intended := sel.source.Head
	if intended == "" {
		return refuse("the selected view records no source head to publish", "repair the selected result with human direction")
	}
	if observedHead == intended {
		return nil
	}
	if observedHead == "" {
		// The adapter validated that no pull attachment exists yet. Publish the
		// intended source through the ordinary non-force path before the create;
		// a non-fast-forward branch is refused rather than rewritten.
		if note := synchronizeSource(root, remote, sel.state.Branch, intended); note != nil {
			return fmt.Errorf("source publication remains pending: %s", note.Detail)
		}
		return nil
	}
	if sel.publishedSource == "" || observedHead != sel.publishedSource {
		return refuse(
			"the attached source revision "+observedHead+" is not the recorded published source",
			"repair the source attachment with human direction; recovery does not replace unexpected source history",
		)
	}
	if !gitOK(root, "merge-base", "--is-ancestor", observedHead, intended) {
		return refuse(
			"the attached source revision "+observedHead+" is not an ancestor of the intended head "+intended,
			"repair the source history with human direction; recovery never force-pushes",
		)
	}
	if note := synchronizeSource(root, remote, sel.state.Branch, intended); note != nil {
		return fmt.Errorf("source catch-up remains pending: %s", note.Detail)
	}
	return nil
}

// sameReportContent compares one supplied result reference with the current
// selected report by bytes, so an unrelated ledger commit does not defeat it.
func sameReportContent(s *Store, sel *publicationSelection, reference *Reference) bool {
	if reference == nil {
		return false
	}
	if sel.phase == "" {
		return false
	}
	wanted, err := showPath(s, reference.Commit, reference.Path)
	if err != nil {
		return false
	}
	current, err := showPath(s, sel.head, sel.directory+"/"+sel.phase+"-report.md")
	if err != nil {
		return false
	}
	return wanted == current
}

func samePublicationAssociations(a, b *publicationSelection) bool {
	return sameAttachment(a.parent, b.parent) && slices.Equal(a.children, b.children)
}

func publicationStateOf(state SliceState) *PublicationState {
	if state.Publication != nil {
		copy := *state.Publication
		return &copy
	}
	return &PublicationState{}
}

// commitRecoveryState writes one state record and commits it as one brief
// local mutation.
func (s *Store) commitRecoveryState(sel *publicationSelection, message string) error {
	path := sel.directory + "/state.json"
	if err := s.requireCleanPaths(path); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(s.Root, filepath.FromSlash(path)), sel.state); err != nil {
		return err
	}
	return s.commit(message, path)
}

func (s *Store) commitRecoveryProposal(sel *publicationSelection) error {
	path := sel.proposalDirectory + "/proposal.json"
	if err := s.requireCleanPaths(path); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(s.Root, filepath.FromSlash(path)), sel.meta); err != nil {
		return err
	}
	return s.commit("record parent publication recovery "+sel.repository.Name+"/"+sel.proposal, path)
}

// contractDigestsAt returns the exact references and canonical path/digest
// facts of one slice's committed contract files.
func (s *Store) contractDigestsAt(head, directory string) ([]Reference, []string, error) {
	names, err := acceptedFileNamesAt(s, head, directory)
	if err != nil {
		return nil, nil, err
	}
	var refs []Reference
	var digests []string
	for _, name := range names {
		path := directory + "/" + name
		contents, err := showPath(s, head, path)
		if err != nil {
			return nil, nil, err
		}
		refs = append(refs, Reference{Commit: head, Path: path})
		digests = append(digests, path+":"+digestOf([]byte(contents)))
	}
	return refs, digests, nil
}

// proposalContractRefsAt returns the union of every member slice's committed
// contract facts for the coordination parent's view token.
func (s *Store) proposalContractRefsAt(head, project, proposal string) ([]Reference, []string, error) {
	directory := filepath.ToSlash(filepath.Join(projectsRoot, project, "proposals", proposal))
	entries, err := git(s.Root, "ls-tree", "--name-only", head+":"+directory)
	if err != nil {
		return nil, nil, gitError(s.Root, []string{"ls-tree", "--name-only", head + ":" + directory}, err)
	}
	var slices []string
	for _, name := range strings.Fields(entries) {
		if name == "proposal.json" || name == "proposal.md" {
			continue
		}
		slices = append(slices, name)
	}
	sort.Strings(slices)
	var refs []Reference
	var digests []string
	for _, slice := range slices {
		sliceRefs, sliceDigests, err := s.contractDigestsAt(head, directory+"/"+slice)
		if err != nil {
			return nil, nil, err
		}
		refs = append(refs, sliceRefs...)
		digests = append(digests, sliceDigests...)
	}
	return refs, digests, nil
}

// childAttachmentsAt lists the committed child issue attachments of one
// proposal in a stable order.
func (s *Store) childAttachmentsAt(head, project, proposal string) ([]ForgeAttachment, error) {
	directory := filepath.ToSlash(filepath.Join(projectsRoot, project, "proposals", proposal))
	entries, err := git(s.Root, "ls-tree", "--name-only", head+":"+directory)
	if err != nil {
		return nil, gitError(s.Root, []string{"ls-tree", "--name-only", head + ":" + directory}, err)
	}
	var attachments []ForgeAttachment
	for _, name := range strings.Fields(entries) {
		if name == "proposal.json" || name == "proposal.md" {
			continue
		}
		var state SliceState
		if err := readJSONAt(s, head, directory+"/"+name+"/state.json", &state); err != nil {
			continue
		}
		if state.Issue != nil {
			attachments = append(attachments, *state.Issue)
		}
	}
	sort.Slice(attachments, func(i, j int) bool { return attachments[i].Number < attachments[j].Number })
	return attachments, nil
}

// issueBodyRecordAt builds the registration metadata of one slice's temporary
// descriptive issue body. It returns nil when no body was supplied.
func (s *Store) issueBodyRecordAt(head, project, identity, proposal, slice string, declaration *ProposalDeclaration, state SliceState) (*PublicationBody, error) {
	if declaration == nil {
		return nil, nil
	}
	contents, supplied := declaration.IssueBodies[slice]
	if !supplied {
		return nil, nil
	}
	path := ""
	if declaration.IssueBodyPaths != nil {
		path = declaration.IssueBodyPaths[slice]
	}
	directory := filepath.ToSlash(filepath.Join(projectsRoot, project, "proposals", proposal, slice))
	_, digests, err := s.contractDigestsAt(head, directory)
	if err != nil {
		return nil, err
	}
	token := publicationInputs{
		Repository: identity, Kind: publicationKindIssue, Item: proposal + "/" + slice,
		Title: state.Title, Contracts: digests,
	}.token()
	return &PublicationBody{Path: path, SHA256: digestOf(contents), View: token}, nil
}

// parentBodyRecordAt builds the registration metadata of one proposal's
// temporary parent issue body. It returns nil when no body was supplied.
func (s *Store) parentBodyRecordAt(head, project, identity, proposal string, declaration *ProposalDeclaration) (*PublicationBody, error) {
	if declaration == nil || declaration.ParentBody == nil {
		return nil, nil
	}
	_, digests, err := s.proposalContractRefsAt(head, project, proposal)
	if err != nil {
		return nil, err
	}
	token := publicationInputs{
		Repository: identity, Kind: publicationKindParent, Item: proposal,
		Title: strings.TrimSpace(declaration.ParentTitle), Contracts: digests,
	}.token()
	return &PublicationBody{Path: declaration.ParentBodyPath, SHA256: digestOf(declaration.ParentBody), View: token}, nil
}
