package workflow

// RequireBranchAncestor fetches the remote's current head of target and
// refuses unless it is an ancestor of head. The work branch is fetched only
// when head is not already available locally. It returns the observed target
// head, so a refusal can name it.
func RequireBranchAncestor(root, remote, target, branch, head string) (string, error) {
	observed, err := deliveryFetch(root, remote, target)
	if err != nil {
		return "", Refuse("Proposal Branch " + target + " could not be fetched: " + err.Error())
	}
	if deliveryResolveCommit(root, head) != head {
		if _, err := deliveryFetch(root, remote, branch); err != nil || deliveryResolveCommit(root, head) != head {
			return observed, Refuse("final head " + head + " is unavailable in the source repository")
		}
	}
	if gitOK(root, "merge-base", "--is-ancestor", observed, head) != nil {
		return observed, Refuse("Proposal Branch " + target + " is at " + observed + ", which is not an ancestor of final head " + head)
	}
	return observed, nil
}
