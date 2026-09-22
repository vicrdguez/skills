package ledger

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/vicrdguez/skills/github"
)

// ReadyForImplementation is the initial Workflow State recorded for every
// accepted slice, before any Claim exists.
const ReadyForImplementation = "ready_for_implementation"

// projectsRoot is the ledger directory holding one directory per Project.
const projectsRoot = "projects"

var recordName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ValidRecordName reports whether name is a safe, conventional kebab-case
// record component usable as a ledger directory name.
func ValidRecordName(name string) bool {
	return recordName.MatchString(name)
}

// ProjectIdentity is the source-repository association recorded in
// project.json. One Project belongs to exactly one repository and is named
// after that repository, never after a local checkout directory.
type ProjectIdentity struct {
	Repository string `json:"repository"`
}

// Project is one resolved Project inside the ledger.
type Project struct {
	// Name is the repository name and the ledger directory name.
	Name string
	// Repository is the full owner/name identity of the source repository.
	Repository string
	// Created is true when no project.json existed yet; the acceptance
	// commit then records it for the first time.
	Created bool
}

// ItemPath returns the project-relative record path of one Work Item slice.
func ItemPath(proposal, slice string) string {
	return fmt.Sprintf("proposals/%s/%s", proposal, slice)
}

// resolveProject resolves the Project for one source repository: it reuses
// the existing project directory when its recorded identity matches, and
// reports a new project otherwise. A directory holding a different
// repository identity is a collision and is refused rather than aliased.
// The ledger is not written; the caller commits project.json with the rest
// of an accepted declaration.
func (s *Store) resolveProject(repository github.RepositoryID) (Project, error) {
	name := repository.Name
	identity := repository.Owner + "/" + repository.Name
	directory := filepath.Join(s.Root, projectsRoot, name)
	contents, err := os.ReadFile(filepath.Join(directory, "project.json"))
	switch {
	case err == nil:
		var recorded ProjectIdentity
		if jsonErr := json.Unmarshal(contents, &recorded); jsonErr != nil || recorded.Repository == "" {
			return Project{}, refuse(
				"project "+name+" has an unreadable project.json in the ledger",
				"repair projects/"+name+"/project.json to {\"repository\": \"owner/name\"} or remove the damaged project with human direction, then retry",
			)
		}
		if recorded.Repository != identity {
			return Project{}, refuse(
				"project "+name+" already belongs to "+recorded.Repository+", which is a different repository from "+identity,
				"choose a distinct repository name or move the conflicting project with human direction; skl invents no alias",
			)
		}
		return Project{Name: name, Repository: identity}, nil
	case os.IsNotExist(err):
		if _, statErr := os.Stat(directory); statErr == nil {
			return Project{}, refuse(
				"project directory "+projectsRoot+"/"+name+" exists in the ledger without project.json",
				"restore its project.json or remove the damaged directory with human direction, then retry",
			)
		}
		return Project{Name: name, Repository: identity, Created: true}, nil
	default:
		return Project{}, fmt.Errorf("read %s: %w", filepath.Join(directory, "project.json"), err)
	}
}

// adopted reports whether this source repository already has a Project in
// the ledger. A directory whose recorded identity is a different repository
// does not adopt this one; an unreadable or identity-less directory is
// conservatively adopted rather than granting a forge-authoritative
// fallback through it.
func (s *Store) Adopted(repository github.RepositoryID) (bool, error) {
	directory := filepath.Join(s.Root, projectsRoot, repository.Name)
	contents, err := os.ReadFile(filepath.Join(directory, "project.json"))
	if err != nil {
		if os.IsNotExist(err) {
			if _, statErr := os.Stat(directory); statErr == nil {
				return true, nil
			}
			return false, nil
		}
		return false, err
	}
	var recorded ProjectIdentity
	if json.Unmarshal(contents, &recorded) != nil || recorded.Repository == "" {
		return true, nil
	}
	return recorded.Repository == repository.Owner+"/"+repository.Name, nil
}

// writeProject records one new project identity as part of an acceptance
// commit.
func (s *Store) writeProject(project Project) error {
	directory := filepath.Join(s.Root, projectsRoot, project.Name)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	contents, err := json.MarshalIndent(ProjectIdentity{Repository: project.Repository}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(directory, "project.json"), append(contents, '\n'), 0o644)
}
