package workflow

import "context"

func StartWatchdog(ctx context.Context, root, remote string, backend ImplementationBackend) (ImplementationOutcome, error) {
	remote, err := ResolveGitHubRemote(root, remote)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	_, _, err = loadImplementation(ctx, root, remote, backend)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	return ImplementationOutcome{Status: "no_work"}, nil
}
