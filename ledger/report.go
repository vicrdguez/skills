package ledger

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// The two phase reports persisted in the private Workflow Ledger. Each phase
// has its own accepted outcome values and source-provenance requirements.
const (
	ImplementPhase = "implement"
	WatchdogPhase  = "watchdog"
)

// ReportSchema is the only frontmatter schema this codec reads or writes. A
// report carrying any other value belongs to a format this CLI does not
// understand and is refused rather than reinterpreted.
const ReportSchema = 1

// Accepted semantic outcomes. The phase decides which subset applies; the
// outcome is always worker judgment supplied through the semantic command,
// never something the engine infers from the Markdown body.
const (
	outcomeAwaitingReview = "awaiting_review"
	outcomeNeedsHuman     = "needs_human"
	outcomePass           = "pass"
	outcomeRework         = "rework"
)

// reportDelimiter opens and closes the YAML frontmatter. Only the leading
// frontmatter is parsed; every later delimiter-looking line belongs to the
// opaque Markdown body.
const reportDelimiter = "---"

// commitIdentity matches a full lowercase commit SHA. The codec validates the
// shape only; Git-level existence and ancestry are separate concerns that
// stay outside this format boundary.
var commitIdentity = regexp.MustCompile(`^[0-9a-f]{40}$`)

// Reference identifies one document at one exact ledger revision: the full
// commit at which the path is valid together with the ledger-relative path.
// It need not be the commit that introduced the document; an archived record
// keeps its original path's meaning at the referenced revision.
type Reference struct {
	Commit string `yaml:"commit" json:"commit"`
	Path   string `yaml:"path" json:"path"`
}

// SourceRevisions identifies the source-repository revisions a phase report
// concerns. Head is the reported candidate, Target is the observed Integration
// Target it was integrated against, and Reviewed is the exact revision an
// independent review inspected. They retain source-repository meaning and are
// never ledger revisions.
type SourceRevisions struct {
	Head     string `yaml:"head,omitempty" json:"head,omitempty"`
	Target   string `yaml:"target,omitempty" json:"target,omitempty"`
	Reviewed string `yaml:"reviewed,omitempty" json:"reviewed,omitempty"`
}

// ReportInputs records the exact ledger material a phase consumed. Claim and
// Contract are always required; Implement, Watchdog, and Decision appear when
// that phase consumed the corresponding record.
type ReportInputs struct {
	Claim     Reference   `yaml:"claim" json:"claim"`
	Contract  []Reference `yaml:"contract" json:"contract"`
	Implement *Reference  `yaml:"implement,omitempty" json:"implement,omitempty"`
	Watchdog  *Reference  `yaml:"watchdog,omitempty" json:"watchdog,omitempty"`
	Decision  *Reference  `yaml:"decision,omitempty" json:"decision,omitempty"`
}

// Report is the schema-1 phase-report frontmatter. The engine supplies
// Schema, the exact ledger references, and the completed review Round; a
// worker supplies the semantic Outcome and, through the Markdown body, its
// evidence and judgment.
type Report struct {
	Schema  int             `yaml:"schema" json:"schema"`
	Outcome string          `yaml:"outcome" json:"outcome"`
	Source  SourceRevisions `yaml:"source,omitempty" json:"source"`
	Ledger  ReportInputs    `yaml:"ledger" json:"ledger"`
	Round   uint64          `yaml:"round,omitempty" json:"round,omitempty"`
}

// FormatReport renders one schema-1 phase report: the documented YAML
// frontmatter followed by the Markdown body as exact bytes. It refuses an
// invalid phase or incompatible metadata before any bytes are produced, so a
// refused report is never persisted as an accepted record.
func FormatReport(phase string, report Report, body string) ([]byte, error) {
	if err := validateReport(phase, report); err != nil {
		return nil, err
	}
	var frontmatter bytes.Buffer
	encoder := yaml.NewEncoder(&frontmatter)
	encoder.SetIndent(2)
	if err := encoder.Encode(report); err != nil {
		return nil, fmt.Errorf("encode %s report frontmatter: %w", phase, err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("encode %s report frontmatter: %w", phase, err)
	}
	var document bytes.Buffer
	document.WriteString(reportDelimiter + "\n")
	document.Write(frontmatter.Bytes())
	document.WriteString(reportDelimiter + "\n")
	document.WriteString(body)
	return document.Bytes(), nil
}

// ParseReport reads one schema-1 phase report. Only the leading frontmatter
// between the opening --- line and the first full-line closing --- delimiter
// is interpreted; the body is returned byte for byte as opaque data, so
// delimiter-looking lines or template-like instructions inside it are never
// inspected. Unknown phases, unknown schemas, extra YAML documents, and
// metadata incompatible with the phase are refused explicitly.
func ParseReport(phase string, data []byte) (Report, string, error) {
	if err := validatePhase(phase); err != nil {
		return Report{}, "", err
	}
	frontmatter, body, err := splitReportFrontmatter(data)
	if err != nil {
		return Report{}, "", err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(frontmatter))
	decoder.KnownFields(true)
	var report Report
	if err := decoder.Decode(&report); err != nil {
		if errors.Is(err, io.EOF) {
			return Report{}, "", refuse(
				"report holds no YAML frontmatter metadata",
				"persist the documented schema-1 fields between the --- delimiters",
			)
		}
		return Report{}, "", refuse(
			"report frontmatter is malformed: "+err.Error(),
			"write the documented schema-1 fields with their exact types",
		)
	}
	var extra any
	switch err := decoder.Decode(&extra); {
	case errors.Is(err, io.EOF):
	case err == nil:
		return Report{}, "", refuse(
			"report frontmatter holds more than one YAML document",
			"keep exactly one YAML document between the --- delimiters",
		)
	default:
		return Report{}, "", refuse(
			"report frontmatter is malformed: "+err.Error(),
			"keep exactly one YAML document between the --- delimiters",
		)
	}
	if err := checkReportScalars(frontmatter); err != nil {
		return Report{}, "", err
	}
	if err := validateReport(phase, report); err != nil {
		return Report{}, "", err
	}
	return report, string(body), nil
}

// splitReportFrontmatter separates a phase report's leading YAML frontmatter
// from its opaque body.
func splitReportFrontmatter(data []byte) ([]byte, []byte, error) {
	return splitFrontmatter("report", data)
}

// splitFrontmatter separates a document's leading YAML frontmatter from its
// opaque body. It scans only as far as the first full-line closing delimiter
// and never looks for another one. kind names the document for refusals so
// decision documents and phase reports share one delimiter implementation.
func splitFrontmatter(kind string, data []byte) ([]byte, []byte, error) {
	opening := reportDelimiter + "\n"
	if !bytes.HasPrefix(data, []byte(opening)) {
		return nil, nil, refuse(
			kind+" does not begin with a --- frontmatter delimiter",
			"persist the "+kind+" as YAML between an opening --- line and a closing --- line followed by the Markdown body",
		)
	}
	rest := data[len(opening):]
	offset := 0
	for {
		lineEnd := bytes.IndexByte(rest[offset:], '\n')
		if lineEnd < 0 {
			if string(rest[offset:]) == reportDelimiter {
				return rest[:offset], rest[len(rest):], nil
			}
			return nil, nil, refuse(
				kind+" frontmatter has no closing --- delimiter",
				"close the YAML frontmatter with a --- line before the Markdown body",
			)
		}
		if string(rest[offset:offset+lineEnd]) == reportDelimiter {
			return rest[:offset], rest[offset+lineEnd+1:], nil
		}
		offset += lineEnd + 1
	}
}

// Struct decoding permits scalar coercions (including fractional integers).
// Verify the persisted scalar tags as well; optional nulls retain their normal
// absence meaning and required values are checked by validateReport.
func checkReportScalars(frontmatter []byte) error {
	return checkScalarTags("report", frontmatter, map[string]bool{"schema": true, "round": true})
}

// checkScalarTags verifies documented scalar tags. integers names the fields
// that must carry a YAML integer; every other non-null scalar must be text.
func checkScalarTags(kind string, frontmatter []byte, integers map[string]bool) error {
	var document yaml.Node
	if err := yaml.Unmarshal(frontmatter, &document); err != nil {
		return err
	}
	var visit func(*yaml.Node, string) error
	visit = func(node *yaml.Node, field string) error {
		if node.Kind == yaml.AliasNode {
			return visit(node.Alias, field)
		}
		if node.Kind == yaml.ScalarNode && node.Tag != "!!null" {
			wanted := "!!str"
			if integers[field] {
				wanted = "!!int"
			}
			if node.Tag != wanted {
				if wanted == "!!int" {
					return refuse(kind+" "+field+" is not a YAML integer", "use the documented integer type")
				}
				return refuse(kind+" "+field+" is not a YAML string", "quote scalar text in "+kind+" metadata")
			}
		}
		if node.Kind == yaml.MappingNode {
			for i := 0; i < len(node.Content); i += 2 {
				name := node.Content[i].Value
				if field != "" {
					name = field + "." + name
				}
				if err := visit(node.Content[i+1], name); err != nil {
					return err
				}
			}
		} else {
			for _, child := range node.Content {
				if err := visit(child, field); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return visit(&document, "")
}

// validatePhase refuses any phase without a schema-1 report format.
func validatePhase(phase string) error {
	if phase != ImplementPhase && phase != WatchdogPhase {
		return refuse(
			"phase "+strconv.Quote(phase)+" has no schema-1 report format",
			"use the "+ImplementPhase+" or "+WatchdogPhase+" phase",
		)
	}
	return nil
}

// validateReport checks the common metadata every phase shares and then the
// phase-specific compatibility requirements.
func validateReport(phase string, report Report) error {
	if err := validatePhase(phase); err != nil {
		return err
	}
	if report.Schema != ReportSchema {
		return refuse(
			"report schema "+strconv.Itoa(report.Schema)+" is not the supported schema 1",
			"write schema as the bare integer 1; a different version requires a CLI that understands it",
		)
	}
	if err := validateReference("claim reference", report.Ledger.Claim); err != nil {
		return err
	}
	if len(report.Ledger.Contract) == 0 {
		return refuse(
			"report records no consumed contract references",
			"record at least one exact reference to the frozen contract version read",
		)
	}
	for index, reference := range report.Ledger.Contract {
		if err := validateReference("contract reference "+strconv.Itoa(index+1), reference); err != nil {
			return err
		}
	}
	for _, optional := range []struct {
		label     string
		reference *Reference
	}{
		{"implement reference", report.Ledger.Implement},
		{"watchdog reference", report.Ledger.Watchdog},
		{"decision reference", report.Ledger.Decision},
	} {
		if optional.reference != nil {
			if err := validateReference(optional.label, *optional.reference); err != nil {
				return err
			}
		}
	}
	switch phase {
	case ImplementPhase:
		return validateImplementReport(report)
	case WatchdogPhase:
		return validateWatchdogReport(report)
	}
	return nil
}

// validateImplementReport enforces the implementation phase's outcome values
// and source-provenance requirements.
func validateImplementReport(report Report) error {
	if report.Outcome != outcomeAwaitingReview && report.Outcome != outcomeNeedsHuman {
		return refuse(
			"implement report outcome "+strconv.Quote(report.Outcome)+" is not "+outcomeAwaitingReview+" or "+outcomeNeedsHuman,
			"submit "+outcomeAwaitingReview+" when the work is ready for independent review, or "+outcomeNeedsHuman+" for a permitted human decision",
		)
	}
	if report.Round != 0 {
		return refuse(
			"implement report records review round "+strconv.FormatUint(report.Round, 10),
			"record the completed review round only in a watchdog report",
		)
	}
	if report.Source.Reviewed != "" {
		return refuse(
			"implement report records reviewed revision "+strconv.Quote(report.Source.Reviewed),
			"record the independently reviewed source only in a watchdog report",
		)
	}
	if report.Source.Head != "" {
		if err := validateSourceCommit("source head", report.Source.Head); err != nil {
			return err
		}
	}
	if report.Source.Target != "" {
		if err := validateSourceCommit("source target", report.Source.Target); err != nil {
			return err
		}
	}
	switch report.Outcome {
	case outcomeAwaitingReview:
		if report.Source.Head == "" || report.Source.Target == "" {
			return refuse(
				outcomeAwaitingReview+" implement report requires source head and target revisions",
				"record the implemented source head and the observed Integration Target it was integrated against",
			)
		}
	case outcomeNeedsHuman:
		if (report.Source.Head == "") != (report.Source.Target == "") {
			return refuse(
				outcomeNeedsHuman+" implement report must record source head and target together",
				"record both revisions when a source workspace exists, or neither when it does not exist yet",
			)
		}
	}
	return nil
}

// validateWatchdogReport enforces the watchdog phase's outcome values,
// completed-review round, and required source/ledger provenance.
func validateWatchdogReport(report Report) error {
	switch report.Outcome {
	case outcomePass, outcomeRework, outcomeNeedsHuman:
	default:
		return refuse(
			"watchdog report outcome "+strconv.Quote(report.Outcome)+" is not "+outcomePass+", "+outcomeRework+", or "+outcomeNeedsHuman,
			"submit "+outcomePass+" for an approved review, "+outcomeRework+" for another implementation round, or "+outcomeNeedsHuman+" for a permitted human decision",
		)
	}
	if report.Round < 1 {
		return refuse(
			"watchdog report records completed review round 0",
			"record the round of the completed independent review, starting at 1",
		)
	}
	if report.Ledger.Implement == nil {
		return refuse(
			"watchdog report records no reviewed implementation report",
			"record the exact reference to the implementation report this review consumed",
		)
	}
	if report.Source.Head == "" || report.Source.Target == "" || report.Source.Reviewed == "" {
		return refuse(
			"watchdog report requires source head, target, and reviewed revisions",
			"record the reported head, the observed Integration Target, and the exact independently reviewed revision",
		)
	}
	for _, revision := range []struct{ label, value string }{
		{"source head", report.Source.Head},
		{"source target", report.Source.Target},
		{"source reviewed", report.Source.Reviewed},
	} {
		if err := validateSourceCommit(revision.label, revision.value); err != nil {
			return err
		}
	}
	if report.Outcome != outcomePass && report.Source.Head != report.Source.Reviewed {
		return refuse(
			report.Outcome+" watchdog report records head "+report.Source.Head+" that differs from reviewed "+report.Source.Reviewed,
			"a non-passing review inspects exactly the reported head; only a passing review may record permitted post-marker code",
		)
	}
	return nil
}

// validateReference checks one ledger reference's deterministic shape: a full
// lowercase commit SHA and a safe nonempty relative ledger path. Whether the
// path belongs to the project or the referenced revision is validated by the
// consuming handoff, not by this format codec.
func validateReference(label string, reference Reference) error {
	if !commitIdentity.MatchString(reference.Commit) {
		return refuse(
			label+" commit "+strconv.Quote(reference.Commit)+" is not a full lowercase 40-character commit SHA",
			"supply the exact full commit SHA the ledger recorded",
		)
	}
	if !validLedgerPath(reference.Path) {
		return refuse(
			label+" path "+strconv.Quote(reference.Path)+" is not a safe relative ledger path",
			"supply a nonempty relative path inside the ledger with no . or .. segments, such as projects/<project>/proposals/<proposal>/<slice>/state.json",
		)
	}
	return nil
}

// validateSourceCommit checks one source-repository revision identity.
func validateSourceCommit(label, value string) error {
	if !commitIdentity.MatchString(value) {
		return refuse(
			label+" "+strconv.Quote(value)+" is not a full lowercase 40-character commit SHA",
			"record the exact full source commit SHA the phase concerned",
		)
	}
	return nil
}

// validLedgerPath accepts only a clean relative ledger path with real
// segments, refusing traversal and absolute or unclean forms.
func validLedgerPath(value string) bool {
	if value == "" || strings.HasPrefix(value, "/") || strings.Contains(value, `\`) {
		return false
	}
	if value == "." || value == ".." || strings.HasPrefix(value, "../") {
		return false
	}
	return path.Clean(value) == value
}
