package ledger

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/vicrdguez/skills/github"
)

// PullPresentation contains only deliberately public material. Approved is a
// semantic delivery fact; the forge adapter owns draft/readiness mechanics.
type PullPresentation struct {
	Number                    int
	Title, Body, Branch, Head string
	Approved                  bool
}

type DeliveryForge interface {
	PresentPull(context.Context, PullPresentation) (int, error)
}

// PublishDelivery attempts normal publication of an already committed result.
// It never falls back to the private report body. An in-flight presentation is
// serialized by a brief durable reservation, not by holding a lock over I/O.
func PublishDelivery(ctx context.Context, s *Store, repository github.RepositoryID, root, remote string, result *DeliveryResult, publicBody *string, forge DeliveryForge) {
	if publicBody == nil || forge == nil {
		result.Publication = &PublicationNote{Status: IssuePending, Detail: "separately authored public material or forge access is unavailable; the local handoff is complete"}
		return
	}
	var source SourceRevisions
	reserved := false
	err := s.withMutation(func() error {
		state, directory, head, err := s.deliveryState(repository, result.Item)
		if err != nil {
			return err
		}
		if state.State != result.Status {
			return refuse("a later lifecycle supersedes this public presentation", "publish the latest relevant view with fresh public material")
		}
		if err := s.requireCleanPaths(directory + "/state.json"); err != nil {
			return err
		}
		wanted, err := showPath(s, result.Report.Commit, result.Report.Path)
		if err != nil {
			return err
		}
		current, err := showPath(s, head, result.Report.Path)
		if err != nil {
			return err
		}
		if wanted != current {
			return refuse("a later report supersedes this public presentation", "preserve the newer result")
		}
		phase := ImplementPhase
		if strings.HasSuffix(result.Report.Path, "/watchdog-report.md") {
			phase = WatchdogPhase
		}
		report, _, err := ParseReport(phase, []byte(current))
		if err != nil {
			return err
		}
		source = report.Source
		if state.Publication == nil {
			state.Publication = &PublicationState{}
		}
		if state.Publication.Active != nil {
			return refuse("another delivery presentation is still reserved", "inspect that attempt before retrying publication; no duplicate or guessed public approval is authorized")
		}
		state.Publication.Active = &result.Report
		if err := writeJSON(filepath.Join(s.Root, directory, "state.json"), state); err != nil {
			return err
		}
		if err := s.commit("reserve delivery presentation "+repository.Name+"/"+result.Item, directory+"/state.json"); err != nil {
			return err
		}
		result.State = state
		reserved = true
		return nil
	})
	if err != nil {
		result.Publication = &PublicationNote{Status: IssuePending, Detail: err.Error()}
		return
	}
	if !reserved {
		return
	}
	sourceNote := synchronizeSource(root, remote, result.State.Branch, source.Head)
	pullNote := &PublicationNote{Status: IssuePending, Detail: "source synchronization remains pending; no code approval was published"}
	var attachment *ForgeAttachment
	if sourceNote == nil {
		number := 0
		if result.State.Submission != nil {
			number = result.State.Submission.Number
		}
		presented, err := forge.PresentPull(ctx, PullPresentation{Number: number, Title: result.State.Title, Body: *publicBody, Branch: result.State.Branch, Head: source.Head, Approved: result.Status == ReadyForMerge})
		if err != nil {
			pullNote = &PublicationNote{Status: IssuePending, Detail: err.Error()}
		} else if presented <= 0 {
			pullNote = &PublicationNote{Status: IssuePending, Detail: "forge returned no valid Submission attachment"}
		} else {
			attachment = &ForgeAttachment{Repository: repository.Owner + "/" + repository.Name, Number: presented}
			pullNote = nil
		}
	}
	err = s.withMutation(func() error {
		state, directory, head, err := s.deliveryState(repository, result.Item)
		if err != nil {
			return err
		}
		if err := s.requireCleanPaths(directory + "/state.json"); err != nil {
			return err
		}
		if state.Publication == nil || !sameReference(state.Publication.Active, &result.Report) {
			return refuse("delivery publication reservation changed", "preserve current records and inspect the uncertain presentation")
		}
		state.Publication.Active = nil
		wanted, err := showPath(s, result.Report.Commit, result.Report.Path)
		if err != nil {
			return err
		}
		current, err := showPath(s, head, result.Report.Path)
		if err != nil {
			return err
		}
		// Preserve later Claims, reports, and their pending presentation. A known
		// attachment can still be retained without asserting approval of later code.
		if attachment != nil {
			if state.Submission != nil && !sameAttachment(state.Submission, attachment) {
				return refuse("concurrent delivery recorded a different Submission", "preserve both attachments and reconcile with human direction")
			}
			state.Submission = attachment
		}
		if state.State == result.Status && wanted == current {
			state.Publication.Source = sourceNote
			state.Publication.Pull = pullNote
		}
		if err := writeJSON(filepath.Join(s.Root, directory, "state.json"), state); err != nil {
			return err
		}
		if err := s.commit("record delivery presentation "+repository.Name+"/"+result.Item, directory+"/state.json"); err != nil {
			return err
		}
		result.State = state
		return nil
	})
	result.Publication = pullNote
	if err != nil {
		result.Publication = &PublicationNote{Status: IssuePending, Detail: "publication bookkeeping is pending: " + err.Error()}
	}
}

func synchronizeSource(root, remote, branch, head string) *PublicationNote {
	if head == "" {
		return &PublicationNote{Status: PushPending, Detail: "this paused result has no source revision to publish"}
	}
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

func sameReference(a, b *Reference) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
