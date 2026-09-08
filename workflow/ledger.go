package workflow

import (
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"slices"
	"strings"
)

type LedgerHistory struct {
	Baseline   string
	Completion string
	Deletion   string
	Phase      string
	Violations []string
}

// InspectLedger resolves immutable snapshots from first-parent trees, not prose.
func InspectLedger(root, ref, slug string) (LedgerHistory, error) {
	result := LedgerHistory{Phase: "absent"}
	if slug == "" || strings.ContainsAny(slug, "/\\") || slug == "." || slug == ".." {
		result.Violations = []string{"invalid ledger slug; use the conventional branch identity"}
		return result, nil
	}
	commits, err := git(root, "rev-list", "--first-parent", "--reverse", ref)
	if err != nil {
		return result, err
	}
	path := ".changes/" + slug
	previous := ""
	present := false
	var previousFiles map[string]string
	for _, commit := range strings.Fields(commits) {
		parents, err := git(root, "rev-list", "--parents", "-n", "1", commit)
		if err != nil {
			return result, err
		}
		if len(strings.Fields(parents)) > 2 {
			currentTree, _ := git(root, "rev-parse", "--verify", commit+":"+path)
			parentTree, _ := git(root, "rev-parse", "--verify", previous+":"+path)
			if currentTree != parentTree {
				result.Violations = append(result.Violations, "merge changes ledger relative to first parent at "+commit)
			}
		}
		now := gitOK(root, "cat-file", "-e", commit+":"+path) == nil
		var files map[string]string
		if now {
			files, err = ledgerFiles(root, commit, path)
			if err != nil {
				var violation *InvariantError
				if errors.As(err, &violation) {
					result.Violations = append(result.Violations, violation.Reason)
					slices.Sort(result.Violations)
					return result, nil
				}
				return result, err
			}
			for name, contents := range files {
				if present {
					old, exists := previousFiles[name]
					if !exists || !permittedTicks(old, contents) {
						result.Violations = append(result.Violations, "frozen ledger changed at "+commit+":"+name)
					}
				}
				result.Violations = append(result.Violations, ledgerBoxes(contents, false, commit+":"+name)...)
			}
			if present && len(files) != len(previousFiles) {
				result.Violations = append(result.Violations, "frozen ledger paths changed at "+commit)
			}
		}
		if now && !present {
			if result.Baseline != "" {
				result.Violations = append(result.Violations, "ledger is introduced more than once at "+commit)
			} else {
				result.Baseline = commit
			}
			result.Phase = "present"
			for _, required := range []string{"intent.md", "behavior.md"} {
				if _, ok := files[path+"/"+required]; !ok {
					result.Violations = append(result.Violations, "ledger misses "+required+" at Artifact Baseline")
				}
			}
		}
		if !now && present {
			result.Completion, result.Deletion, result.Phase = previous, commit, "retired"
			for name, contents := range previousFiles {
				result.Violations = append(result.Violations, ledgerBoxes(contents, true, previous+":"+name)...)
			}
		}
		previous, present = commit, now
		previousFiles = files
	}
	if result.Baseline == "" {
		result.Violations = append(result.Violations, fmt.Sprintf("ledger is missing at %s", path))
	}
	slices.Sort(result.Violations)
	return result, nil
}

func ledgerFiles(root, commit, path string) (map[string]string, error) {
	entries, err := exec.Command("git", "-C", root, "ls-tree", "-rz", commit, "--", path).Output()
	if err != nil {
		return nil, err
	}
	files := make(map[string]string)
	for _, entry := range strings.Split(string(entries), "\x00") {
		if entry == "" {
			continue
		}
		metadata, name, ok := strings.Cut(entry, "\t")
		fields := strings.Fields(metadata)
		if !ok || len(fields) != 3 || fields[0] != "100644" || fields[1] != "blob" {
			return nil, Refuse(fmt.Sprintf("ledger must contain regular non-executable files at %s: %s; restore the frozen file mode", commit, entry))
		}
		contents, err := exec.Command("git", "-C", root, "cat-file", "blob", fields[2]).Output()
		if err != nil {
			return nil, err
		}
		files[name] = string(contents)
	}
	return files, nil
}

var ledgerCheckbox = regexp.MustCompile(`^(\s*(?:[-*+]|[0-9]+[.)])\s+\[)([ xX])(\].*)$`)

func permittedTicks(old, current string) bool {
	before, after := strings.Split(old, "\n"), strings.Split(current, "\n")
	if len(before) != len(after) {
		return false
	}
	for i, line := range before {
		if line == after[i] {
			continue
		}
		box := ledgerCheckbox.FindStringSubmatch(line)
		if box == nil || box[2] != " " || after[i] != box[1]+"x"+box[3] {
			return false
		}
	}
	return true
}

func ledgerBoxes(contents string, complete bool, location string) []string {
	var violations []string
	manualLevel := 0
	for i, line := range strings.Split(contents, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			heading := strings.TrimLeft(trimmed, "#")
			level := len(trimmed) - len(heading)
			if level <= manualLevel {
				manualLevel = 0
			}
			if strings.EqualFold(strings.TrimSpace(strings.TrimRight(heading, "#")), "Manual verification") {
				manualLevel = level
			}
		}
		box := ledgerCheckbox.FindStringSubmatch(line)
		if box == nil {
			continue
		}
		if manualLevel != 0 && box[2] != " " {
			violations = append(violations, fmt.Sprintf("Manual Verification must remain unchecked at %s:%d", location, i+1))
		}
		if complete && manualLevel == 0 && box[2] == " " {
			violations = append(violations, fmt.Sprintf("agent-verifiable checkbox unchecked at %s:%d", location, i+1))
		}
	}
	return violations
}
