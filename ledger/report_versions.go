package ledger

import (
	"fmt"
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

	// Walk backward through one proposal incarnation. On an archive move the
	// selected archived record continues at the old active path; an active
	// replacement of the same name does not inherit archived history.
	archiveSide := proposal.archived
	for _, commit := range strings.Fields(listing) {
		paths, treeErr := v.versionPaths(commit, activeReport, archivedReport, activeProposal, archivedProposal)
		if treeErr != nil {
			addProblem("cannot inspect report history at " + commit + ": " + treeErr.Error())
			continue
		}
		ancestry, parentErr := git(v.store.Root, "rev-list", "--parents", "-n", "1", commit)
		if parentErr != nil {
			addProblem("cannot read parent of " + commit + ": " + parentErr.Error())
			continue
		}
		parent := strings.Fields(ancestry)
		older := map[string]string{}
		if len(parent) > 1 {
			older, treeErr = v.versionPaths(parent[1], activeReport, archivedReport, activeProposal, archivedProposal)
			if treeErr != nil {
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
			document, readErr := v.Document(ref)
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

// versionPaths reads tree entries, not blobs: even an unavailable report blob
// keeps its exact version reference and does not hide healthy versions.
func (v *Snapshot) versionPaths(commit string, paths ...string) (map[string]string, error) {
	args := append([]string{"ls-tree", "-r", "-z", commit, "--"}, paths...)
	output, err := git(v.store.Root, args...)
	if err != nil {
		return nil, gitError(v.store.Root, args, err)
	}
	entries := map[string]string{}
	for _, entry := range strings.Split(output, "\x00") {
		if entry == "" {
			continue
		}
		info, name, ok := strings.Cut(entry, "\t")
		fields := strings.Fields(info)
		if !ok || len(fields) != 3 || fields[1] != "blob" {
			return nil, fmt.Errorf("unexpected tree entry at %s", commit)
		}
		entries[name] = fields[2]
	}
	return entries, nil
}
