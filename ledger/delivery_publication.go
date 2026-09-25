package ledger

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/vicrdguez/skills/github"
)

// PullPresented reports a pull request that presents the selected current
// result. Failed or uncertain presentation reuses IssuePending and
// IssueUnresolved; none of these outcomes is persisted.
const PullPresented = "presented"

// ErrPresentationUncertain marks a forge effect, such as a pull request
// creation whose response was lost, that may or may not have happened. It is
// reported, never retried automatically, and leaves no durable repair record.
var ErrPresentationUncertain = errors.New("pull request presentation outcome is uncertain")

// PullPresentation contains only deliberately public material. Approved is a
// semantic delivery fact; the forge adapter owns draft/readiness mechanics.
// Current, when set, reports whether the selected local result still stands;
// the adapter consults it before each additional forge write.
type PullPresentation struct {
	Number                    int
	Title, Body, Branch, Head string
	Approved                  bool
	Current                   func() error `json:"-"`
}

type DeliveryForge interface {
	PresentPull(context.Context, PullPresentation) (int, error)
}

// CurrentResult is the latest committed phase result of one Work Item, the
// only input a pull request presentation is derived from. It is selected from
// the current lifecycle and the reports' recorded input references, never from
// a publication record or the reports' Markdown prose.
type CurrentResult struct {
	Item       string           `json:"item"`
	Lifecycle  string           `json:"lifecycle"`
	Phase      string           `json:"phase"`
	Outcome    string           `json:"outcome"`
	Round      uint64           `json:"round,omitempty"`
	Report     Reference        `json:"report"`
	Implement  *Reference       `json:"implement,omitempty"`
	Decision   *Reference       `json:"decision,omitempty"`
	Source     SourceRevisions  `json:"source"`
	Branch     string           `json:"branch"`
	Approved   bool             `json:"approved"`
	Submission *ForgeAttachment `json:"submission,omitempty"`
	Claimed    bool             `json:"claimed,omitempty"`

	title, contents string
}

// SelectCurrentResult reads the Work Item's latest committed phase result. A
// review is latest when it consumed the current implementation report;
// otherwise the implementation report is. A lifecycle that no longer follows
// from that result, such as recorded human direction or a terminal state,
// supersedes it and is refused rather than presented over.
func SelectCurrentResult(s *Store, repository github.RepositoryID, item string) (CurrentResult, error) {
	state, directory, head, err := s.deliveryState(repository, item)
	if err != nil {
		return CurrentResult{}, err
	}
	implementPath := directory + "/" + ImplementPhase + "-report.md"
	implementation, err := showPath(s, head, implementPath)
	if err != nil {
		return CurrentResult{}, refuse("Work Item "+item+" has no committed phase result to present", "complete an implementation handoff before presenting a pull request")
	}
	result := CurrentResult{Item: item, Lifecycle: state.State, Phase: ImplementPhase, Report: Reference{Commit: head, Path: implementPath}, Branch: state.Branch, Submission: state.Submission, Claimed: state.Claim != nil, title: state.Title, contents: implementation}
	reviewPath := directory + "/" + WatchdogPhase + "-report.md"
	if review, err := showPath(s, head, reviewPath); err == nil {
		parsed, _, err := ParseReport(WatchdogPhase, []byte(review))
		if err != nil {
			return CurrentResult{}, err
		}
		if consumed := parsed.Ledger.Implement; consumed != nil {
			if reviewed, err := showPath(s, consumed.Commit, consumed.Path); err == nil && reviewed == implementation {
				result.Phase, result.Report.Path, result.contents = WatchdogPhase, reviewPath, review
				result.Implement = consumed
			}
		}
	}
	report, _, err := ParseReport(result.Phase, []byte(result.contents))
	if err != nil {
		return CurrentResult{}, err
	}
	result.Outcome, result.Round, result.Source, result.Decision = report.Outcome, report.Round, report.Source, report.Ledger.Decision
	destination, err := deliveryDestination(result.Phase, report.Outcome, report.Round)
	if err != nil {
		return CurrentResult{}, err
	}
	if destination != state.State {
		return CurrentResult{}, refuse("Work Item "+item+" is "+state.State+", which supersedes its latest "+result.Phase+" result ("+report.Outcome+")", "preserve the later human direction or lifecycle; present a pull request again after the next phase result")
	}
	result.Approved = state.State == ReadyForMerge
	return result, nil
}

// superseded reports whether a later local result replaced this selection.
// Unrelated ledger commits, including a new Claim, leave it current.
func (r CurrentResult) superseded(s *Store, repository github.RepositoryID) error {
	current, err := SelectCurrentResult(s, repository, r.Item)
	if err != nil {
		return fmt.Errorf("the selected %s result is no longer current: %w", r.Phase, err)
	}
	if current.Phase != r.Phase || current.Lifecycle != r.Lifecycle || current.contents != r.contents {
		return refuse("a later local "+current.Phase+" result supersedes the selected "+r.Phase+" result", "present the current view with public prose authored for it")
	}
	return nil
}

// Presentation is the immediate outcome of one explicit current-view
// presentation. Nothing about the attempt is persisted except a newly
// established Submission association.
type Presentation struct {
	Result      CurrentResult    `json:"result"`
	Publication PublicationNote  `json:"publication"`
	Replication *PublicationNote `json:"replication,omitempty"`
}

// PresentCurrent presents a selected current result with freshly authored
// public prose, whatever happened to earlier attempts. It reruns no phase and
// changes no lifecycle, report, Claim, or review count; a selection that a
// later local result superseded is reported rather than presented.
func PresentCurrent(ctx context.Context, s *Store, repository github.RepositoryID, root, remote string, selected CurrentResult, publicBody string, forge DeliveryForge) *Presentation {
	note, attachment, recorded := presentSelected(ctx, s, repository, root, remote, selected, publicBody, forge)
	presentation := &Presentation{Result: selected, Publication: note}
	if attachment != nil {
		presentation.Result.Submission = attachment
	}
	if recorded {
		if head, err := s.head(); err == nil {
			replication, _ := s.push(head)
			presentation.Replication = &replication
		}
	}
	return presentation
}

// PublishDelivery attempts normal presentation of an already committed
// handoff. It never falls back to the private report body, and it presents
// only while the handoff is still the current result.
func PublishDelivery(ctx context.Context, s *Store, repository github.RepositoryID, root, remote string, result *DeliveryResult, publicBody *string, forge DeliveryForge) {
	if publicBody == nil || forge == nil {
		result.Publication = &PublicationNote{Status: IssuePending, Detail: "separately authored public material or forge access is unavailable; the local handoff is complete and its current view can be presented later"}
		return
	}
	selected, err := SelectCurrentResult(s, repository, result.Item)
	if err == nil {
		var handed string
		if handed, err = showPath(s, result.Report.Commit, result.Report.Path); err == nil && (selected.Report.Path != result.Report.Path || selected.contents != handed) {
			err = refuse("a later local result supersedes this handoff's presentation", "present the current view with public prose authored for it")
		}
	}
	if err != nil {
		result.Publication = &PublicationNote{Status: IssuePending, Detail: err.Error()}
		return
	}
	note, attachment, _ := presentSelected(ctx, s, repository, root, remote, selected, *publicBody, forge)
	result.Publication = &note
	if attachment != nil {
		result.State.Submission = attachment
	}
}

// presentSelected publishes the recorded source through an ordinary push and
// presents it with the supplied prose. No lock is held across network work;
// only a newly established association is recorded, in one brief mutation.
func presentSelected(ctx context.Context, s *Store, repository github.RepositoryID, root, remote string, selected CurrentResult, body string, forge DeliveryForge) (PublicationNote, *ForgeAttachment, bool) {
	pending := func(detail string) (PublicationNote, *ForgeAttachment, bool) {
		return PublicationNote{Status: IssuePending, Detail: detail}, nil, false
	}
	if selected.Source.Head == "" {
		return pending("the " + selected.Phase + " result records no source revision to present")
	}
	current := func() error { return selected.superseded(s, repository) }
	if err := current(); err != nil {
		return pending(err.Error())
	}
	if note := synchronizeSource(root, remote, selected.Branch, selected.Source.Head); note != nil {
		return pending(note.Detail + "; no pull request presentation or approval was attempted")
	}
	if err := current(); err != nil {
		return pending(err.Error())
	}
	number := 0
	if selected.Submission != nil {
		number = selected.Submission.Number
	}
	presented, err := forge.PresentPull(ctx, PullPresentation{Number: number, Title: selected.title, Body: body, Branch: selected.Branch, Head: selected.Source.Head, Approved: selected.Approved, Current: current})
	if err != nil {
		if errors.Is(err, ErrPresentationUncertain) {
			return PublicationNote{Status: IssueUnresolved, Detail: err.Error()}, nil, false
		}
		return pending(err.Error())
	}
	if presented <= 0 {
		return pending("forge returned no valid Submission attachment")
	}
	attachment := &ForgeAttachment{Repository: repository.Owner + "/" + repository.Name, Number: presented}
	recorded, err := s.recordSubmission(repository, selected.Item, attachment)
	if err != nil {
		return pending(fmt.Sprintf("pull request #%d presents the %s result, but recording its association is pending: %v", presented, selected.Phase, err))
	}
	readiness := "draft"
	if selected.Approved {
		readiness = "ready for review"
	}
	return PublicationNote{Status: PullPresented, Detail: fmt.Sprintf("pull request #%d presents the %s result at %s as %s", presented, selected.Phase, selected.Source.Head, readiness)}, attachment, recorded
}

// recordSubmission retains a successfully established association and its
// integration destination on the then-current record, preserving every later
// result, Decision, and Claim. A different known association is never
// reassigned.
func (s *Store) recordSubmission(repository github.RepositoryID, item string, attachment *ForgeAttachment) (bool, error) {
	recorded := false
	err := s.withMutation(func() error {
		state, directory, _, err := s.deliveryState(repository, item)
		if err != nil {
			return err
		}
		// PresentPull only creates/validates main in this repository. Bind that
		// destination with the exact owned attachment, including when this is a
		// later presentation of an already attached result.
		target := &IntegrationTarget{Repository: attachment.Repository, Branch: "main"}
		if state.Submission != nil && !sameAttachment(state.Submission, attachment) {
			return refuse(fmt.Sprintf("Work Item %s already records Submission %s#%d", item, state.Submission.Repository, state.Submission.Number), "preserve both attachments and reconcile with human direction")
		}
		if state.Submission != nil && state.Target != nil && *state.Target == *target {
			return nil
		}
		if err := s.requireCleanPaths(directory + "/state.json"); err != nil {
			return err
		}
		state.Submission, state.Target = attachment, target
		if err := writeJSON(filepath.Join(s.Root, directory, "state.json"), state); err != nil {
			return err
		}
		if err := s.commit("record submission "+repository.Name+"/"+item, directory+"/state.json"); err != nil {
			return err
		}
		recorded = true
		return nil
	})
	return recorded, err
}

func synchronizeSource(root, remote, branch, head string) *PublicationNote {
	refspec := head + ":refs/heads/" + branch
	if _, err := git(root, "push", remote, refspec); err != nil {
		return &PublicationNote{Status: PushPending, Detail: "source push unavailable: " + gitError(root, []string{"push", remote, refspec}, err).Error()}
	}
	observed, err := git(root, "ls-remote", "--exit-code", remote, "refs/heads/"+branch)
	fields := strings.Fields(observed)
	if err != nil || len(fields) != 2 || fields[0] != head {
		return &PublicationNote{Status: PushPending, Detail: fmt.Sprintf("remote branch %s could not be verified at the reported source revision %s", branch, head)}
	}
	return nil
}
