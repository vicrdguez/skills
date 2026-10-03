package ledger

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"path"
	"slices"
	"strings"
)

// ReportVersion is a committed change to report bytes, not an execution round
// or a reference to unchanged bytes at a later ledger revision.
type ReportVersion struct {
	Reference   Reference    `json:"reference"`
	Report      *Report      `json:"report,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

// ReportVersions is one Slice's locally discoverable history for one phase.
// An incomplete result retains every version that could still be identified.
type ReportVersions struct {
	Revision    string          `json:"revision"`
	Project     string          `json:"project"`
	Item        string          `json:"item"`
	Phase       string          `json:"phase"`
	Archived    bool            `json:"archived"`
	Versions    []ReportVersion `json:"versions"`
	Diagnostics []Diagnostic    `json:"diagnostics,omitempty"`
	Incomplete  bool            `json:"incomplete"`
}

// Versions returns the implementation or watchdog report's content changes
// reachable from this snapshot. Active records take precedence when both
// active and archived records use the same name.
func (v *Snapshot) Versions(project, item, phase string) (*ReportVersions, error) {
	return v.reportVersions(project, item, phase, nil)
}

// VersionsAt selects the active or archived record explicitly.
func (v *Snapshot) VersionsAt(project, item, phase string, archived bool) (*ReportVersions, error) {
	return v.reportVersions(project, item, phase, &archived)
}

func (v *Snapshot) reportVersions(projectName, item, phase string, location *bool) (*ReportVersions, error) {
	if err := validatePhase(phase); err != nil {
		return nil, err
	}
	proposalName, sliceName, found := strings.Cut(item, "/")
	if !found || !ValidRecordName(proposalName) || !ValidRecordName(sliceName) {
		return nil, refuse("Slice reference "+item+" is not a proposal/slice identity", "select a Slice listed by its Proposal")
	}
	project, proposal, err := v.proposalLocated(projectName, proposalName, location)
	if err != nil {
		return nil, err
	}
	if proposal.slice(sliceName) == nil {
		return nil, refuse("no Slice "+item+" in project "+project.name+" at ledger revision "+v.Revision, "select a Slice listed by its Proposal")
	}
	result := &ReportVersions{Revision: v.Revision, Project: project.name, Item: item, Phase: phase, Archived: proposal.archived, Versions: []ReportVersion{}}
	subject := project.name + "/" + item
	addProblem := func(problem string) {
		result.Diagnostics = appendDiagnostic(result.Diagnostics, Diagnostic{Scope: ScopeSlice, Subject: subject, Problem: problem})
		result.Incomplete = true
	}
	active := "projects/" + project.name + "/proposals/" + proposalName
	archive := "projects/" + project.name + "/archive/" + proposalName
	name := "/" + sliceName + "/" + phase + "-report.md"
	activeReport, archivedReport := active+name, archive+name
	activeProposal, archivedProposal := active+"/proposal.md", archive+"/proposal.md"

	// Path-limited history avoids treating unrelated commits as versions. Include
	// proposal membership so a report introduced only after archival can still
	// continue to its pre-archive record. First-parent follows the committed
	// ledger view, including a change delivered by a merge commit.
	args := []string{"log", "--first-parent", "--format=%H", v.Revision, "--", activeReport, archivedReport, activeProposal, archivedProposal}
	listing, err := git(v.store.Root, args...)
	if err != nil {
		addProblem("cannot read local report history: " + gitError(v.store.Root, args, err).Error())
	}
	shallow, shallowErr := git(v.store.Root, "rev-parse", "--is-shallow-repository")
	if shallowErr != nil {
		addProblem("cannot determine whether local history is shallow: " + shallowErr.Error())
	} else if shallow == "true" {
		addProblem("local Git history is shallow; older report versions may be unavailable")
	}

	// Each listed commit is compared with its first parent. Git has read the
	// trees of both while listing, so their entries and report blobs are read
	// in one batch each.
	commits := strings.Fields(listing)
	parents, parentErr := v.firstParents(commits)
	revisions := append([]string(nil), commits...)
	for _, commit := range commits {
		revisions = append(revisions, parents[commit])
	}
	trees, err := v.versionTrees(revisions, activeReport, archivedReport, activeProposal, archivedProposal)
	if err != nil {
		addProblem("cannot inspect report history: " + err.Error())
		return result, nil
	}
	reports := v.versionReports(commits, trees, activeReport, archivedReport)

	// Walk backward through one proposal incarnation. On an archive move the
	// selected archived record continues at the old active path; an active
	// replacement of the same name does not inherit archived history.
	archiveSide := proposal.archived
	for _, commit := range commits {
		paths := trees[commit].blobs
		if treeErr := trees[commit].err; treeErr != nil {
			addProblem("cannot inspect report history at " + commit + ": " + treeErr.Error())
			continue
		}
		if parentErr != nil {
			addProblem("cannot read parent of " + commit + ": " + parentErr.Error())
			continue
		}
		older := map[string]string{}
		if parent := parents[commit]; parent != "" {
			older = trees[parent].blobs
			if treeErr := trees[parent].err; treeErr != nil {
				addProblem("cannot inspect parent of report history at " + commit + ": " + treeErr.Error())
				continue
			}
		}
		if archiveSide && paths[archivedProposal] == "" && paths[archivedReport] == "" {
			// We crossed the archive move; continue through its active origin.
			if paths[activeProposal] == "" && paths[activeReport] == "" {
				break
			}
			archiveSide = false
		}
		chosen, before := activeReport, activeReport
		if archiveSide {
			chosen, before = archivedReport, archivedReport
			if older[archivedProposal] == "" && older[archivedReport] == "" && (older[activeProposal] != "" || older[activeReport] != "") {
				before = activeReport
			}
		}
		if blob := paths[chosen]; blob != "" && blob != older[before] {
			ref := Reference{Commit: commit, Path: chosen}
			version := ReportVersion{Reference: ref}
			var document *Document
			var readErr error
			if contents, read := reports[blob]; read {
				document = recordDocument(ref, contents)
			} else {
				document, readErr = v.Document(ref)
			}
			if readErr != nil {
				version.Diagnostics = []Diagnostic{{Scope: ScopeSlice, Subject: subject, Problem: "cannot read report version " + commit + ":" + chosen + ": " + readErr.Error()}}
				result.Diagnostics = appendDiagnostic(result.Diagnostics, version.Diagnostics[0])
				result.Incomplete = true
			} else {
				version.Report, version.Diagnostics = document.Report, document.Diagnostics
				for _, diagnostic := range document.Diagnostics {
					result.Diagnostics = appendDiagnostic(result.Diagnostics, diagnostic)
				}
			}
			result.Versions = append(result.Versions, version)
		}
		if archiveSide && before == activeReport {
			archiveSide = false
		} else if !archiveSide && paths[activeProposal] != "" && older[activeProposal] == "" {
			// The active Proposal was introduced here. Do not inherit an
			// older, archived incarnation with the same name.
			break
		}
	}
	return result, nil
}

// firstParents reads the first parent of each commit with one Git process. A
// commit without a parent, such as a shallow boundary, has none.
func (v *Snapshot) firstParents(commits []string) (map[string]string, error) {
	parents := map[string]string{}
	if len(commits) == 0 {
		return parents, nil
	}
	args := append([]string{"rev-list", "--no-walk=unsorted", "--parents"}, commits...)
	ancestry, err := git(v.store.Root, args...)
	if err != nil {
		return nil, gitError(v.store.Root, args, err)
	}
	for _, line := range strings.Split(ancestry, "\n") {
		if fields := strings.Fields(line); len(fields) > 1 {
			parents[fields[0]] = fields[1]
		}
	}
	return parents, nil
}

// versionTree is the blob identity of each tracked path present at one
// commit, or why that commit's entries cannot be interpreted.
type versionTree struct {
	blobs map[string]string
	err   error
}

// versionTrees reads, with one Git process, the tree entries of the tracked
// paths at every given commit. Entries, not blobs, identify versions: even an
// unavailable report blob keeps its exact version reference and does not
// hide healthy versions.
func (v *Snapshot) versionTrees(commits []string, paths ...string) (map[string]versionTree, error) {
	tracked := map[string]bool{}
	var directories []string
	for _, trackedPath := range paths {
		tracked[trackedPath] = true
		if directory := path.Dir(trackedPath); !slices.Contains(directories, directory) {
			directories = append(directories, directory)
		}
	}
	var references []Reference
	trees := map[string]versionTree{}
	for _, commit := range commits {
		if _, seen := trees[commit]; commit == "" || seen {
			continue
		}
		trees[commit] = versionTree{blobs: map[string]string{}}
		for _, directory := range directories {
			references = append(references, Reference{Commit: commit, Path: directory})
		}
	}
	objects, err := v.objects(references, true)
	if err != nil {
		return nil, err
	}
	// Object identities are hex; a tree holds them raw.
	idLength := len(v.Revision) / 2
	for index, reference := range references {
		if objects[index].kind != "tree" {
			continue
		}
		tree := trees[reference.Commit]
		for entries := objects[index].contents; len(entries) > 0; {
			mode, rest, modeOK := bytes.Cut(entries, []byte(" "))
			name, rest, nameOK := bytes.Cut(rest, []byte{0})
			if !modeOK || !nameOK || len(rest) < idLength {
				tree.err = fmt.Errorf("malformed tree at %s", reference.Commit)
				break
			}
			id, entryPath := hex.EncodeToString(rest[:idLength]), reference.Path+"/"+string(name)
			entries = rest[idLength:]
			switch {
			case !tracked[entryPath] || string(mode) == "40000":
			case string(mode) == "160000":
				tree.err = fmt.Errorf("unexpected tree entry at %s", reference.Commit)
			default:
				tree.blobs[entryPath] = id
			}
		}
		trees[reference.Commit] = tree
	}
	return trees, nil
}

// versionReports reads, with one Git process, the contents of every distinct
// report blob the listed commits hold, keyed by blob identity. A blob it
// cannot read is absent, so its version is read exactly and diagnosed.
func (v *Snapshot) versionReports(commits []string, trees map[string]versionTree, activeReport, archivedReport string) map[string]string {
	var references []Reference
	selected := map[string]bool{}
	for _, commit := range commits {
		for _, report := range []string{activeReport, archivedReport} {
			if blob := trees[commit].blobs[report]; blob != "" && !selected[blob] {
				selected[blob] = true
				references = append(references, Reference{Commit: commit, Path: report})
			}
		}
	}
	contents := map[string]string{}
	objects, err := v.objects(references, true)
	if err != nil {
		return contents
	}
	for _, object := range objects {
		if object.kind == "blob" {
			contents[object.id] = string(object.contents)
		}
	}
	return contents
}
