package github

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

type RepositoryID struct {
	Owner string
	Name  string
}

func ParseGitHubRemote(remote string) (RepositoryID, error) {
	remote = strings.TrimSuffix(remote, ".git")
	for _, prefix := range []string{"git@github.com:", "https://github.com/", "ssh://git@github.com/"} {
		if strings.HasPrefix(remote, prefix) {
			parts := strings.Split(strings.TrimPrefix(remote, prefix), "/")
			if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
				return RepositoryID{Owner: parts[0], Name: parts[1]}, nil
			}
		}
	}
	return RepositoryID{}, fmt.Errorf("remote %q is not a GitHub repository", remote)
}

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

func git(directory string, args ...string) (string, error) {
	output, err := exec.Command("git", append([]string{"-C", directory}, args...)...).Output()
	return strings.TrimSpace(string(output)), err
}
