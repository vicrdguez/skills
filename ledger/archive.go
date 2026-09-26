package ledger

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vicrdguez/skills/github"
)

// archiveRoot is the project directory holding retired whole Proposals.
const archiveRoot = "archive"

// ArchivedProposal is one whole Proposal committed at its archive location.
// FullyDelivered is true only when every slice is Merged; mixed or wholly
// Superseded membership is retirement without full delivery.
type ArchivedProposal struct {
	Proposal       string `json:"proposal"`
	FullyDelivered bool   `json:"fully_delivered"`
	Commit         string `json:"ledger_commit"`
	// Resumed reports that an interrupted earlier move was identified and
	// finished rather than moved again.
	Resumed bool `json:"resumed_interrupted_move,omitempty"`
}

// ProposalOutcome explains why one Proposal stays in the active proposals
// directory: it is not archivable yet (Kept) or moving it needs repair.
type ProposalOutcome struct {
	Proposal string `json:"proposal"`
	Reason   string `json:"reason"`
	Repair   string `json:"repair,omitempty"`
}

// ArchiveResult separates committed archival from kept Proposals, repairs,
// and the attempted replication of the local archive commits.
type ArchiveResult struct {
	Archived    []ArchivedProposal `json:"archived,omitempty"`
	Kept        []ProposalOutcome  `json:"kept,omitempty"`
	Repairs     []ProposalOutcome  `json:"repairs,omitempty"`
	Replication *PublicationNote   `json:"replication,omitempty"`
}

// ArchiveTerminalProposals moves every whole Proposal of one Project whose
// slices are all Merged or Superseded and unclaimed from proposals/ to
// archive/. Eligibility is read inside the serialized mutation, so a Claim or
// other change to the selected Proposal cannot be overtaken by a stale read.
// Lifecycle, reports, and embedded references are never rewritten; a path
// move is ledger organization only.
func ArchiveTerminalProposals(ctx context.Context, s *Store, repository github.RepositoryID) (*ArchiveResult, error) {
	s.observeDeliveryUpstream(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := &ArchiveResult{}
	err := s.withMutation(func() error {
		if _, err := s.projectIdentity(repository); err != nil {
			return err
		}
		if err := s.requireReconciled(); err != nil {
			return err
		}
		head, err := s.head()
		if err != nil {
			return err
		}
		proposals, err := s.proposalNames(head, repository.Name)
		if err != nil {
			return err
		}
		for _, proposal := range proposals {
			full, reason, err := s.archivable(head, repository.Name, proposal)
			if err != nil {
				return err
			}
			if reason != "" {
				result.Kept = append(result.Kept, ProposalOutcome{Proposal: proposal, Reason: reason})
				continue
			}
			archived, repair := s.moveProposal(head, repository.Name, proposal, full)
			if repair != nil {
				result.Repairs = append(result.Repairs, *repair)
				continue
			}
			result.Archived = append(result.Archived, *archived)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(result.Archived) > 0 {
		note, _ := s.push(result.Archived[len(result.Archived)-1].Commit)
		result.Replication = &note
	}
	return result, nil
}

// proposalNames lists the committed active Proposals of one Project.
func (s *Store) proposalNames(head, project string) ([]string, error) {
	directory := filepath.ToSlash(filepath.Join(projectsRoot, project, "proposals"))
	if !gitOK(s.Root, "cat-file", "-e", head+":"+directory) {
		return nil, nil
	}
	output, err := git(s.Root, "ls-tree", "-d", "--name-only", head+":"+directory)
	if err != nil {
		return nil, gitError(s.Root, []string{"ls-tree", "-d", "--name-only", head + ":" + directory}, err)
	}
	names := strings.Fields(output)
	sort.Strings(names)
	return names, nil
}

// archivable decides whole-Proposal eligibility from committed records. A
// nonempty reason keeps the Proposal active; unreadable or unknown records are
// never terminal evidence.
func (s *Store) archivable(head, project, proposal string) (bool, string, error) {
	directory := filepath.ToSlash(filepath.Join(projectsRoot, project, "proposals", proposal))
	var meta ProposalMeta
	if err := readJSONAt(s, head, directory+"/proposal.json", &meta); err != nil {
		return false, "proposal.json is unreadable, so its membership is unknown", nil
	}
	slices, err := git(s.Root, "ls-tree", "-d", "--name-only", head+":"+directory)
	if err != nil {
		return false, "", gitError(s.Root, []string{"ls-tree", "-d", "--name-only", head + ":" + directory}, err)
	}
	if len(strings.Fields(slices)) == 0 {
		return false, "records no slices", nil
	}
	full := true
	var active, claimed, unknown []string
	for _, slice := range strings.Fields(slices) {
		var state SliceState
		if err := readJSONAt(s, head, directory+"/"+slice+"/state.json", &state); err != nil {
			unknown = append(unknown, slice)
			continue
		}
		switch state.State {
		case Merged:
		case Superseded:
			full = false
		case ReadyForImplementation, AwaitingReview, Rework, NeedsHuman, ReadyForMerge:
			active = append(active, slice+" ("+state.State+")")
		default:
			unknown = append(unknown, slice)
		}
		if state.Claim != nil {
			claimed = append(claimed, slice)
		}
	}
	var reasons []string
	if len(active) > 0 {
		reasons = append(reasons, "active slices: "+strings.Join(active, ", "))
	}
	if len(claimed) > 0 {
		reasons = append(reasons, "claimed slices: "+strings.Join(claimed, ", "))
	}
	if len(unknown) > 0 {
		reasons = append(reasons, "slices with unknown state: "+strings.Join(unknown, ", "))
	}
	return full, strings.Join(reasons, "; "), nil
}

// moveProposal moves one eligible Proposal as a whole directory and commits
// the rename. A committed or uncommitted distinct destination is never
// overwritten, and an unexplained partial tree is never swept into success.
// A failed commit restores the original location so the Proposal stays whole.
func (s *Store) moveProposal(head, project, proposal string, full bool) (*ArchivedProposal, *ProposalOutcome) {
	source := filepath.ToSlash(filepath.Join(projectsRoot, project, "proposals", proposal))
	destination := filepath.ToSlash(filepath.Join(projectsRoot, project, archiveRoot, proposal))
	keep := func(reason, repair string) (*ArchivedProposal, *ProposalOutcome) {
		return nil, &ProposalOutcome{Proposal: proposal, Reason: reason, Repair: repair}
	}
	if gitOK(s.Root, "cat-file", "-e", head+":"+destination) {
		return keep("archive destination "+destination+" already holds a committed record",
			"compare both records and resolve the collision with human direction; skl overwrites neither")
	}
	sourcePresent, destinationPresent := exists(filepath.Join(s.Root, source)), exists(filepath.Join(s.Root, destination))
	// Staging and rollback rewrite the index of both paths, so they run only
	// from an index matching the committed record: unchanged, or holding
	// exactly the identified rename. Any other staged content is preserved.
	staged, err := s.classifyStagedArchive(head, source, destination)
	if err != nil {
		return keep("the ledger index for "+source+" and "+destination+" is unobservable: "+err.Error(), "inspect the ledger index before retrying")
	}
	if staged == stagedOther {
		return keep("the ledger index holds staged changes under "+source+" or "+destination+" that are not exactly the committed record renamed to the archive",
			"commit, move aside, or unstage them; skl overwrites no staged content")
	}
	resumed := false
	switch {
	case !sourcePresent && destinationPresent:
		same, err := s.matchesCommittedTree(head, source, destination)
		if err != nil || !same {
			return keep("an uncommitted partial archive of "+proposal+" at "+destination+" does not match its committed record",
				"inspect "+source+" and "+destination+" and restore one complete copy; skl discards no partial tree")
		}
		if staged == stagedCommittedRename {
			// The staged rename holds only committed bytes, so unstaging it
			// loses nothing and lets staging and rollback start from head.
			arguments := []string{"reset", "-q", "--", source, destination}
			if _, err := git(s.Root, arguments...); err != nil {
				return keep("cannot unstage the matching staged rename: "+gitError(s.Root, arguments, err).Error(), "inspect the ledger index before retrying")
			}
		}
		resumed = true
	case destinationPresent:
		return keep("archive destination "+destination+" holds uncommitted content",
			"inspect and move the uncommitted content aside; skl overwrites no distinct archive")
	case !sourcePresent:
		return keep("committed proposal "+source+" is missing from the ledger working tree",
			"restore it with git checkout before retrying; skl invents no archive from history")
	default:
		if err := s.requireCleanPaths(source); err != nil {
			return keep("proposal "+source+" has uncommitted changes",
				"commit or restore them before archiving; skl sweeps no uncommitted edits into the archive")
		}
		if err := os.MkdirAll(filepath.Dir(filepath.Join(s.Root, destination)), 0o755); err != nil {
			return keep("cannot prepare the archive directory: "+err.Error(), "repair the ledger working tree and retry")
		}
		if err := os.Rename(filepath.Join(s.Root, source), filepath.Join(s.Root, destination)); err != nil {
			return keep("cannot move "+source+": "+err.Error(), "repair the ledger working tree and retry")
		}
	}
	if err := s.commit("archive proposal "+project+"/"+proposal, source, destination); err != nil {
		reason := "archive commit failed: " + err.Error()
		_, _ = git(s.Root, "reset", "-q", "--", source, destination)
		if restoreErr := os.Rename(filepath.Join(s.Root, destination), filepath.Join(s.Root, source)); restoreErr != nil {
			return keep(reason+"; the uncommitted move remains at "+destination,
				"repair the commit failure and retry; cleanup finishes the identified move when it still matches the committed record")
		}
		return keep(reason+"; the proposal remains at "+source, "repair the commit failure and retry")
	}
	committed, err := s.head()
	if err != nil {
		return keep("archive commit is unobservable: "+err.Error(), "inspect the ledger head before retrying")
	}
	return &ArchivedProposal{Proposal: proposal, FullyDelivered: full, Commit: committed, Resumed: resumed}, nil
}

// stagedArchive classifies the index under a Proposal's source and archive
// destination against the committed head.
type stagedArchive int

const (
	stagedNothing         stagedArchive = iota // the index matches head
	stagedCommittedRename                      // exactly the committed source files at destination
	stagedOther                                // anything else, which cleanup must preserve
)

// classifyStagedArchive reports what the index holds under source and
// destination relative to head.
func (s *Store) classifyStagedArchive(head, source, destination string) (stagedArchive, error) {
	arguments := []string{"diff", "--cached", "--name-only", head, "--", source, destination}
	changed, err := git(s.Root, arguments...)
	if err != nil {
		return stagedOther, gitError(s.Root, arguments, err)
	}
	if changed == "" {
		return stagedNothing, nil
	}
	committed, err := s.committedEntries(head, source)
	if err != nil {
		return stagedOther, err
	}
	arguments = []string{"ls-files", "--stage", "--", source, destination}
	listing, err := git(s.Root, arguments...)
	if err != nil {
		return stagedOther, gitError(s.Root, arguments, err)
	}
	indexed := 0
	for _, line := range strings.Split(listing, "\n") {
		meta, path, found := strings.Cut(line, "\t")
		fields := strings.Fields(meta)
		relative, inDestination := strings.CutPrefix(path, destination+"/")
		if !found || len(fields) != 3 || fields[2] != "0" || !inDestination || committed[relative] != fields[0]+" "+fields[1] {
			return stagedOther, nil
		}
		indexed++
	}
	if indexed != len(committed) {
		return stagedOther, nil
	}
	return stagedCommittedRename, nil
}

// committedEntries maps each file committed under directory at head, by
// relative path, to its "mode blob".
func (s *Store) committedEntries(head, directory string) (map[string]string, error) {
	listing, err := git(s.Root, "ls-tree", "-r", head+":"+directory)
	if err != nil {
		return nil, err
	}
	committed := make(map[string]string)
	for _, line := range strings.Split(listing, "\n") {
		meta, path, found := strings.Cut(line, "\t")
		fields := strings.Fields(meta)
		if !found || len(fields) != 3 {
			return nil, errors.New("unexpected ls-tree output")
		}
		committed[path] = fields[0] + " " + fields[2]
	}
	return committed, nil
}

// matchesCommittedTree reports whether the uncommitted directory at
// destination holds exactly the files and bytes committed at source.
func (s *Store) matchesCommittedTree(head, source, destination string) (bool, error) {
	committed, err := s.committedEntries(head, source)
	if err != nil {
		return false, err
	}
	root := filepath.Join(s.Root, destination)
	seen := 0
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		// Only regular files can match; a symlink or mode change is a different record.
		if !entry.Type().IsRegular() {
			return errors.New("differs")
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		mode := "100644"
		if info.Mode()&0o111 != 0 {
			mode = "100755"
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		blob, err := git(s.Root, "hash-object", "--", path)
		if err != nil || committed[filepath.ToSlash(relative)] != mode+" "+blob {
			return errors.New("differs")
		}
		seen++
		return nil
	})
	if err != nil {
		return false, nil
	}
	return seen == len(committed), nil
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// proposalDirectoryAt resolves a known Proposal to its active or archive
// location at one committed revision. The identity is looked up directly; no
// history or unrelated inventory is scanned.
func (s *Store) proposalDirectoryAt(commit, project, proposal string) string {
	active := filepath.ToSlash(filepath.Join(projectsRoot, project, "proposals", proposal))
	if gitOK(s.Root, "cat-file", "-e", commit+":"+active) {
		return active
	}
	archived := filepath.ToSlash(filepath.Join(projectsRoot, project, archiveRoot, proposal))
	if gitOK(s.Root, "cat-file", "-e", commit+":"+archived) {
		return archived
	}
	return active
}

// SourceWork is the recorded source-deletion authority of one terminal slice.
// AcceptedHead is set only for confirmed Merged, unclaimed work whose exact
// Submission ownership is consistent; Hold explains why deletion is withheld
// regardless of source Git state.
type SourceWork struct {
	Item         string `json:"item"`
	Branch       string `json:"branch"`
	State        string `json:"state"`
	AcceptedHead string `json:"accepted_head,omitempty"`
	Hold         string `json:"hold,omitempty"`
}

// TerminalSourceWork reads every Merged or Superseded slice of one Project,
// active or archived, from committed records. It grants no deletion authority
// to nonterminal work and never consults forge history or public bodies.
func TerminalSourceWork(s *Store, repository github.RepositoryID) ([]SourceWork, error) {
	project, err := s.projectIdentity(repository)
	if err != nil {
		return nil, err
	}
	head, err := s.head()
	if err != nil {
		return nil, err
	}
	var records []SourceWork
	owners := make(map[string]int)
	var unreadable []string
	for _, location := range []string{"proposals", archiveRoot} {
		prefix := filepath.ToSlash(filepath.Join(projectsRoot, repository.Name, location))
		paths, err := git(s.Root, "ls-tree", "-r", "--name-only", head, "--", prefix)
		if err != nil {
			return nil, err
		}
		// Every slice directory is a member whose state must be read, so a
		// missing state.json is as unknown as a malformed one.
		var items []string
		seen := make(map[string]bool)
		for _, path := range strings.Split(paths, "\n") {
			parts := strings.SplitN(strings.TrimPrefix(path, prefix+"/"), "/", 3)
			if len(parts) != 3 {
				continue
			}
			item := parts[0] + "/" + parts[1]
			if !seen[item] {
				seen[item] = true
				items = append(items, item)
			}
		}
		for _, item := range items {
			var state SliceState
			if err := readJSONAt(s, head, prefix+"/"+item+"/state.json", &state); err != nil {
				// An unreadable record authorizes nothing and may own any branch.
				unreadable = append(unreadable, item)
				continue
			}
			owners[state.Branch]++
			if state.State != Merged && state.State != Superseded {
				continue
			}
			record := SourceWork{Item: item, Branch: state.Branch, State: state.State}
			switch {
			case state.State == Superseded:
				record.Hold = "Superseded work was never merged; its source work is preserved"
			case state.Claim != nil:
				record.Hold = "the slice still holds a Claim"
			case validBranch(state.Branch) != nil:
				record.Hold = "no valid owned source branch is recorded"
			case state.Submission == nil || state.Completion == nil || state.Target == nil ||
				state.Completion.Submission != *state.Submission || state.Completion.Target != *state.Target ||
				!strings.EqualFold(state.Submission.Repository, project.Repository):
				record.Hold = "the owned Submission attachment is absent or contradicts the confirmed merge"
			default:
				record.AcceptedHead = state.Completion.SourceHead
			}
			records = append(records, record)
		}
	}
	for index := range records {
		if records[index].Hold != "" {
			continue
		}
		switch {
		case len(unreadable) > 0:
			records[index].Hold = "missing or unreadable Work Item state (" + strings.Join(unreadable, ", ") + ") could own branch " + records[index].Branch
		case owners[records[index].Branch] > 1:
			records[index].Hold = "more than one Work Item records branch " + records[index].Branch
		default:
			continue
		}
		records[index].AcceptedHead = ""
	}
	return records, nil
}
