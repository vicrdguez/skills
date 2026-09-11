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
	Baseline                   string
	Completion                 string
	Deletion                   string
	Phase                      string
	Violations                 []string
	endpointIdentityViolations []string
	acceptedBaselineViolations []string
}

type ArtifactEndpoints struct {
	Baseline   string
	Completion string
}

type LedgerPolicy int

const (
	InspectArtifacts LedgerPolicy = iota
	RequireRetiredArtifacts
	PreserveIncompleteArtifacts
)

// InspectLedger resolves slice markers and validates only accepted endpoint snapshots.
func InspectLedger(root, ref, slug string, explicit ArtifactEndpoints, policy LedgerPolicy) (LedgerHistory, error) {
	result := LedgerHistory{Phase: "absent"}
	if slug == "" || strings.ContainsAny(slug, "/\\") || slug == "." || slug == ".." {
		result.Violations = []string{"invalid ledger slug; use the conventional branch identity"}
		return result, nil
	}
	markers, err := artifactMarkers(root, ref, slug)
	if err != nil {
		return result, err
	}
	result.Baseline = resolveMarker(&result, slug, "baseline", markers["baseline"], explicit.Baseline)
	result.Completion = resolveMarker(&result, slug, "completion", markers["completion"], explicit.Completion)
	for kind, endpoint := range map[string]string{"baseline": explicit.Baseline, "completion": explicit.Completion} {
		if len(markers[kind]) == 0 && endpoint != "" {
			if violation := validateExplicitEndpoint(root, ref, kind, endpoint); violation != "" {
				result.Violations = append(result.Violations, violation)
			}
		}
	}
	if result.Baseline == "" {
		result.Violations = append(result.Violations, "slice "+slug+" is missing [baseline] "+slug+" marker")
	}
	if policy == RequireRetiredArtifacts && result.Completion == "" {
		result.Violations = append(result.Violations, "slice "+slug+" is missing [completion] "+slug+" marker")
	}
	if len(result.Violations) != 0 {
		result.endpointIdentityViolations = slices.Clone(result.Violations)
		slices.Sort(result.Violations)
		return result, nil
	}

	baseline, err := endpointFiles(root, result.Baseline, slug)
	if err != nil {
		return endpointViolation(result, err, true)
	}
	result.acceptedBaselineViolations = append(result.acceptedBaselineViolations, requiredArtifacts(baseline, result.Baseline, slug)...)
	for name, contents := range baseline {
		result.acceptedBaselineViolations = append(result.acceptedBaselineViolations, ledgerBoxes(contents, false, result.Baseline+":"+name)...)
	}
	result.Violations = append(result.Violations, result.acceptedBaselineViolations...)

	if result.Completion != "" {
		if gitOK(root, "merge-base", "--is-ancestor", result.Baseline, result.Completion) != nil {
			violation := "Artifact Baseline " + result.Baseline + " is not an ancestor of Completion " + result.Completion
			result.Violations = append(result.Violations, violation)
			result.endpointIdentityViolations = append(result.endpointIdentityViolations, violation)
		} else {
			completion, err := endpointFiles(root, result.Completion, slug)
			if err != nil {
				return endpointViolation(result, err, false)
			}
			result.Violations = append(result.Violations, compareEndpoints(baseline, completion, result.Baseline, result.Completion, policy != PreserveIncompleteArtifacts)...)
		}
	}

	present := gitOK(root, "cat-file", "-e", ref+":.changes/"+slug) == nil
	if present {
		result.Phase = "present"
		if result.Completion == "" && ref != result.Baseline {
			provisional, err := endpointFiles(root, ref, slug)
			if err != nil {
				return endpointViolation(result, err, false)
			}
			result.Violations = append(result.Violations, compareEndpoints(baseline, provisional, result.Baseline, ref, false)...)
		}
	} else if result.Completion != "" {
		result.Phase = "retired"
	}
	if policy == RequireRetiredArtifacts && present {
		result.Violations = append(result.Violations, "ledger .changes/"+slug+" must be absent at review head "+ref)
	}
	slices.Sort(result.Violations)
	return result, nil
}

func artifactMarkers(root, ref, slug string) (map[string][]string, error) {
	output, err := exec.Command("git", "-C", root, "log", "--format=%H%x00%s%x00", ref).Output()
	if err != nil {
		return nil, err
	}
	markers := map[string][]string{"baseline": {}, "completion": {}}
	fields := strings.Split(strings.TrimSuffix(string(output), "\n"), "\x00")
	for i := 0; i+1 < len(fields); i += 2 {
		sha, subject := strings.TrimSpace(fields[i]), fields[i+1]
		for _, kind := range []string{"baseline", "completion"} {
			prefix := "[" + kind + "] " + slug
			if subject == prefix || strings.HasPrefix(subject, prefix+" ") {
				markers[kind] = append(markers[kind], sha)
			}
		}
	}
	return markers, nil
}

func resolveMarker(result *LedgerHistory, slug, kind string, markers []string, explicit string) string {
	if len(markers) > 1 {
		result.Violations = append(result.Violations, fmt.Sprintf("slice %s has ambiguous [%s] %s markers: %s", slug, kind, slug, strings.Join(markers, ", ")))
		return ""
	}
	if len(markers) == 1 {
		if explicit != "" {
			result.Violations = append(result.Violations, fmt.Sprintf("slice %s has authoritative [%s] marker %s; remove --artifact-%s rather than overriding marked evidence", slug, kind, markers[0], kind))
		}
		return markers[0]
	}
	return explicit
}

func validateExplicitEndpoint(root, ref, kind, endpoint string) string {
	resolved, err := git(root, "rev-parse", "--verify", "--end-of-options", endpoint+"^{commit}")
	if err != nil || resolved != endpoint {
		return fmt.Sprintf("Artifact %s %s must be an available full commit SHA; fetch that exact commit or correct --artifact-%s", strings.Title(kind), endpoint, kind)
	}
	typeName, err := git(root, "cat-file", "-t", endpoint)
	if err != nil || typeName != "commit" {
		return fmt.Sprintf("Artifact %s %s must name a commit object", strings.Title(kind), endpoint)
	}
	if gitOK(root, "merge-base", "--is-ancestor", endpoint, ref) != nil {
		return fmt.Sprintf("Artifact %s %s is not reachable from inspected head %s", strings.Title(kind), endpoint, ref)
	}
	return ""
}

func endpointViolation(result LedgerHistory, err error, acceptedBaseline bool) (LedgerHistory, error) {
	var violation *InvariantError
	if errors.As(err, &violation) {
		result.Violations = append(result.Violations, violation.Reason)
		if acceptedBaseline {
			result.acceptedBaselineViolations = append(result.acceptedBaselineViolations, violation.Reason)
		}
		slices.Sort(result.Violations)
		return result, nil
	}
	return result, err
}

func endpointFiles(root, commit, slug string) (map[string]string, error) {
	path := ".changes/" + slug
	kind, err := git(root, "cat-file", "-t", commit+":"+path)
	if err != nil {
		return nil, Refuse("artifact endpoint " + commit + " is missing ledger directory " + path)
	}
	if kind != "tree" {
		return nil, Refuse("artifact endpoint " + commit + " has invalid ledger shape at " + path + ": expected directory")
	}
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
			return nil, Refuse(fmt.Sprintf("artifact endpoint %s requires mode 100644 blob at %s", commit, name))
		}
		contents, err := exec.Command("git", "-C", root, "cat-file", "blob", fields[2]).Output()
		if err != nil {
			return nil, err
		}
		files[name] = string(contents)
	}
	return files, nil
}

func requiredArtifacts(files map[string]string, endpoint, slug string) []string {
	var violations []string
	for _, name := range []string{"intent.md", "behavior.md"} {
		if _, found := files[".changes/"+slug+"/"+name]; !found {
			violations = append(violations, "ledger misses "+name+" at Artifact Baseline "+endpoint)
		}
	}
	return violations
}

func compareEndpoints(baseline, completion map[string]string, baselineSHA, completionSHA string, complete bool) []string {
	if len(baseline) != len(completion) {
		return []string{"artifact path set differs between Baseline " + baselineSHA + " and Completion " + completionSHA}
	}
	var violations []string
	for name, before := range baseline {
		after, ok := completion[name]
		if !ok {
			return []string{"artifact path set differs between Baseline " + baselineSHA + " and Completion " + completionSHA + ": " + name}
		}
		if !permittedTicks(before, after) {
			violations = append(violations, "artifact content changed outside permitted completion ticks at "+completionSHA+":"+name)
		}
		violations = append(violations, ledgerBoxes(after, complete, completionSHA+":"+name)...)
	}
	return violations
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
