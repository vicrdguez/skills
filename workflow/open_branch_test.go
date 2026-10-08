package workflow

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareOpensMissingProposalBranchAtFetchedMainAndReusesIt(t *testing.T) {
	root := newGitRepository(t)
	remote := newBareRemote(t, root)
	main := gitOutput(t, root, "rev-parse", "HEAD")
	first, err := PrepareDeliverySource(root, "upstream", "slice-one", "", "", "proposal/feature")
	if err != nil || first.Target != main || first.Head != main {
		t.Fatalf("initial preparation = %+v, %v; want main %s", first, err, main)
	}
	if got := gitOutput(t, remote, "rev-parse", "refs/heads/proposal/feature"); got != main {
		t.Fatalf("created remote branch = %s; want fetched main %s", got, main)
	}
	// An existing Proposal Branch ahead of main is the target, not a ref to reset.
	branch := filepath.Join(t.TempDir(), "branch")
	runGit(t, root, "clone", "-q", remote, branch)
	runGit(t, branch, "config", "user.name", "Worker")
	runGit(t, branch, "config", "user.email", "worker@example.com")
	runGit(t, branch, "switch", "-q", "proposal/feature")
	commitFile(t, branch, "branch.txt", "branch work\n")
	ahead := gitOutput(t, branch, "rev-parse", "HEAD")
	runGit(t, branch, "push", "-q", "origin", "proposal/feature")
	second, err := PrepareDeliverySource(root, "upstream", "slice-two", "", "", "proposal/feature")
	if err != nil || second.Target != ahead || second.Head != ahead {
		t.Fatalf("later preparation = %+v, %v; want branch %s", second, err, ahead)
	}
	if got := gitOutput(t, remote, "rev-parse", "refs/heads/proposal/feature"); got != ahead {
		t.Fatalf("preparation moved existing remote branch to %s; want %s", got, ahead)
	}
}

func TestPrepareRefusesToFabricateProposalBranchWhenMainUnavailable(t *testing.T) {
	root := newGitRepository(t)
	remote := filepath.Join(t.TempDir(), "empty.git")
	runGit(t, root, "init", "--bare", remote)
	runGit(t, root, "remote", "add", "upstream", remote)
	_, err := PrepareDeliverySource(root, "upstream", "slice", "", "", "proposal/feature")
	if err == nil || !strings.Contains(err.Error(), "main could not be fetched") {
		t.Fatalf("preparation with absent main = %v; want refusal", err)
	}
	if _, err := git(remote, "rev-parse", "refs/heads/proposal/feature"); err == nil {
		t.Fatal("an absent main fabricated the Proposal Branch")
	}
}
