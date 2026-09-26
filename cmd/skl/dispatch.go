package main

import (
	"fmt"
	"strings"

	"github.com/urfave/cli/v2"
	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/ledger"
	"github.com/vicrdguez/skills/setup"
)

// dispatchFlags turn next into a Dispatch: the Claim goes to a fresh Worker
// Session, and the caller receives commands instead of the Execution Skill.
func dispatchFlags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{Name: "dispatch", Usage: "claim work for a fresh Worker Session and answer with its worker and continue commands"},
		&cli.StringFlag{Name: "after", Usage: "continue a dispatch after its worker returns: the exact Claim it dispatched"},
		&cli.StringFlag{Name: "worker-model", Usage: "opaque model for the dispatched Worker Session"},
		&cli.StringFlag{Name: "worker-thinking", Usage: "opaque thinking level for the dispatched Worker Session"},
	}
}

// supervisorFlags configure the Dispatch itself, so the worker command omits
// them. repo and remote are bound to their resolved values instead.
var supervisorFlags = map[string]bool{"repo": true, "remote": true, "format": true, "wait": true, "poll": true, "dispatch": true, "after": true, "worker-model": true, "worker-thinking": true}

// dispatched is one dispatched Claim: how to start its Worker Session, and
// how to continue once that session returns.
type dispatched struct {
	Item           string `json:"item"`
	Claim          string `json:"claim"`
	Worker         string `json:"worker"`
	Continue       string `json:"continue"`
	WorkerModel    string `json:"worker_model,omitempty"`
	WorkerThinking string `json:"worker_thinking,omitempty"`
}

type dispatchedFacts struct {
	Status   string
	Phase    string
	Previous *ledger.ClaimEnding
	dispatched
}

// stoppedFacts report a dispatched Claim that ended without a phase handoff.
// Resume and Release are bound only while the Claim is still held.
type stoppedFacts struct {
	Status  string
	Phase   string
	Ending  ledger.ClaimEnding
	Resume  string
	Release string
}

// dispatchOutput answers a Dispatch that claimed execution's Work Item. The
// worker command resumes that exact Claim as the Execution Skill next would
// have returned; the continue command repeats this Dispatch after it.
func dispatchOutput(c *cli.Context, phase string, repository setup.RepositoryContext, execution *ledger.Execution, previous *ledger.ClaimEnding) deliveryOutput {
	item, claim := execution.Item, execution.Claim.Commit
	worker := []string{claimCommand(phase, "resume", repository, item, claim), "--dispatched"}
	continuation := []string{nextInvocation(phase, repository)}
	for _, flag := range c.Command.Flags {
		name := flag.Names()[0]
		switch {
		case name == "after":
			continuation = append(continuation, "--after", skilldist.ShellQuote(claim))
		case name != "repo" && name != "remote":
			continuation = append(continuation, flagWords(c, name)...)
		}
		if !supervisorFlags[name] {
			worker = append(worker, flagWords(c, name)...)
		}
	}
	d := dispatched{Item: item, Claim: claim, Worker: strings.Join(worker, " "), Continue: strings.Join(continuation, " "),
		WorkerModel: c.String("worker-model"), WorkerThinking: c.String("worker-thinking")}
	return deliveryOutput{Status: "dispatched", Previous: previous, Dispatch: &d, kind: "dispatched",
		facts: dispatchedFacts{Status: "dispatched", Phase: phase, Previous: previous, dispatched: d}}
}

// stoppedOutput answers a continuation whose Claim ended without a handoff.
// It claims nothing: the Claim stays exactly as it is for a human to decide.
func stoppedOutput(phase string, repository setup.RepositoryContext, ending ledger.ClaimEnding) deliveryOutput {
	facts := stoppedFacts{Status: "stopped", Phase: phase, Ending: ending}
	reason := fmt.Sprintf("Claim %s on %s was released without a phase handoff", ending.Claim, ending.Item)
	if ending.Ending == ledger.ClaimHeld {
		reason = fmt.Sprintf("Claim %s on %s is still held", ending.Claim, ending.Item)
		facts.Resume = claimCommand(phase, "resume", repository, ending.Item, ending.Claim)
		facts.Release = claimCommand(phase, "release", repository, ending.Item, ending.Claim)
	}
	return deliveryOutput{Status: "stopped", Reason: reason, Previous: &ending, kind: "stopped", facts: facts}
}
