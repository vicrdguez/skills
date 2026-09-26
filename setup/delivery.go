package setup

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/ledger"
	"github.com/vicrdguez/skills/workflow"
)

// ImplementChoices are an Implement invocation's mode and its opaque subagent
// values. An empty value is omitted.
type ImplementChoices struct {
	Mode     string
	Helper   skilldist.SubagentChoice
	Reviewer skilldist.SubagentChoice
}

// flags repeats the supplied choices for the commands that render or print a
// later Implement continuation.
func (choices ImplementChoices) flags() string {
	var words []string
	add := func(flag, value string) {
		if value != "" {
			words = append(words, " --"+flag+" "+skilldist.ShellQuote(value))
		}
	}
	if choices.Mode == skilldist.TeamMode {
		add("mode", choices.Mode)
	}
	add("helper-model", choices.Helper.Model)
	add("helper-thinking", choices.Helper.Thinking)
	add("reviewer-model", choices.Reviewer.Model)
	add("reviewer-thinking", choices.Reviewer.Thinking)
	return strings.Join(words, "")
}

// supplied is the choice to render, or nil when it carries no value.
func supplied(choice skilldist.SubagentChoice) *skilldist.SubagentChoice {
	if choice == (skilldist.SubagentChoice{}) {
		return nil
	}
	return &choice
}

// PresentDelivery binds known commands and evidence without exposing private
// storage navigation or asking workers to choose an engine-resolvable procedure.
// Implement renders and resumes with choices; Watchdog takes none.
func PresentDelivery(e *ledger.Execution, repository RepositoryContext, phase, operation string, source *workflow.DeliverySource, directory string, choices ImplementChoices) (skilldist.Packet, error) {
	worktree, err := workflow.DeliveryWorktree(repository.Root, e.State.Branch)
	if err != nil {
		return skilldist.Packet{}, err
	}
	if directory == "" {
		directory, err = os.MkdirTemp("", "skl-"+phase+"-")
		if err != nil {
			return skilldist.Packet{}, err
		}
	} else if !filepath.IsAbs(directory) {
		return skilldist.Packet{}, fmt.Errorf("result-directory must be absolute")
	}
	f := &skilldist.DeliveryFacts{Phase: phase, Operation: operation, Repository: e.Repository, Remote: repository.Remote, Item: e.Item, Branch: e.State.Branch, Worktree: worktree, ResultDirectory: directory, Claim: e.Claim.Commit, Documents: e.Documents, Procedure: "initial"}
	if operation == "resume" {
		f.Procedure = "resumed"
	}
	if e.State.State == ledger.Rework {
		f.Procedure = "rework"
	}
	if e.Implement != nil {
		f.RequiredHead = e.Implement.Source.Head
		f.RecordedTarget = e.Implement.Source.Target
	}
	if e.Watchdog != nil {
		f.PreviousReviewed = e.Watchdog.Source.Reviewed
		f.ReviewCount = e.Watchdog.Round
	}
	if phase == ledger.WatchdogPhase {
		f.ReviewNumber = f.ReviewCount + 1
	}
	if source != nil {
		f.SourceHead = source.Head
		f.SourceTarget = source.Target
		f.ReviewScope = source.Scope
		f.FetchStatus = source.FetchStatus
	}
	q := skilldist.ShellQuote
	identity := fmt.Sprintf(" --repo %s --remote %s --item %s --claim %s", q(repository.Root), q(repository.Remote), q(e.Item), q(e.Claim.Commit))
	command := func(operation string) string { return "skl " + phase + " " + operation + identity }
	f.PrepareCommand = command("prepare")
	f.ResumeCommand = command("resume")
	f.ReleaseCommand = command("release")
	f.InspectCommand = command("inspect")
	f.PrepareCommand += " --result-directory " + q(directory)
	f.ResumeCommand += " --result-directory " + q(directory)
	f.InspectCommand += " --result-directory " + q(directory)
	if phase == ledger.ImplementPhase {
		f.Mode, f.Helper, f.Reviewer = cmp.Or(choices.Mode, skilldist.StandardMode), supplied(choices.Helper), supplied(choices.Reviewer)
		f.PrepareCommand += choices.flags()
		f.InspectCommand += choices.flags()
		f.ResumeCommand += choices.flags()
	}
	target := f.RecordedTarget
	if source != nil {
		target = source.Target
	}
	if phase == ledger.ImplementPhase {
		if target == "" {
			f.InspectCommand += " --target <observed-target-sha>"
		} else {
			f.InspectCommand += " --target " + q(target)
		}
	}
	private := q(filepath.Join(directory, phase+"-report.md"))
	public := q(filepath.Join(directory, "public.md"))
	f.SubmitCommand = command("submit") + " --body " + private + " --public-body " + public
	f.PauseCommand = command("needs-human") + " --body " + private + " --public-body " + public
	if phase == ledger.ImplementPhase {
		f.SubmitCommand += " --head <final-source-sha> --target <observed-target-sha>"
		f.PauseCommand += " --head <branch-head> --target <observed-target-sha>"
		f.ResultResourceCommand = fmt.Sprintf("skl skill --resource ledger-submission.md --input result_directory=%s --input procedure=%s implement", q(directory), f.Procedure)
	} else {
		f.SubmitCommand += " --outcome <pass|rework|needs-human>"
		f.PauseCommand = command("submit") + " --body " + private + " --public-body " + public + " --outcome needs-human"
		f.ResultResourceCommand = fmt.Sprintf("skl skill --resource ledger-review.md --input result_directory=%s --input round=%d --input reviewed_head=%s watchdog", q(directory), f.ReviewNumber, q(f.RequiredHead))
	}
	return skilldist.BuildPacket(phase, skilldist.InvocationFacts{Delivery: f})
}
