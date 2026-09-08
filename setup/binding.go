package setup

import (
	"errors"
	"fmt"

	"github.com/vicrdguez/skills/github"
)

type RepositoryContext struct {
	Root       string
	Remote     string
	Repository github.RepositoryID
}

func ResolveRepository(location, remote string) (RepositoryContext, error) {
	if location == "" {
		location = "."
	}
	root, err := git(location, "rev-parse", "--show-toplevel")
	if err != nil {
		return RepositoryContext{}, errors.New("not a Git repository")
	}
	remote, err = github.ResolveGitHubRemote(root, remote)
	if err != nil {
		return RepositoryContext{}, err
	}
	remoteURL, err := git(root, "remote", "get-url", remote)
	if err != nil {
		return RepositoryContext{}, fmt.Errorf("resolve GitHub remote %q: %w", remote, err)
	}
	repository, err := github.ParseGitHubRemote(remoteURL)
	if err != nil {
		return RepositoryContext{}, err
	}
	return RepositoryContext{Root: root, Remote: remote, Repository: repository}, nil
}
