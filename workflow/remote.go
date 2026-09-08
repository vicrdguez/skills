package workflow

import (
	"errors"
	"fmt"
	"strings"
)

func ResolveGitHubRemote(root, explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if origin, err := git(root, "remote", "get-url", "origin"); err == nil {
		if _, err := ParseGitHubRemote(origin); err == nil {
			return "origin", nil
		}
	}
	names, err := git(root, "remote")
	if err != nil {
		return "", err
	}
	var githubRemotes []string
	for _, name := range strings.Fields(names) {
		remoteURL, err := git(root, "remote", "get-url", name)
		if err == nil {
			if _, err := ParseGitHubRemote(remoteURL); err == nil {
				githubRemotes = append(githubRemotes, name)
			}
		}
	}
	switch len(githubRemotes) {
	case 1:
		return githubRemotes[0], nil
	case 0:
		return "", errors.New("no GitHub remote found")
	default:
		return "", fmt.Errorf("multiple GitHub remotes (%s); choose one with --remote", strings.Join(githubRemotes, ", "))
	}
}
