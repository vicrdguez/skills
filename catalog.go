package skills

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"
	"text/template"
)

const InstructionProtocol = "skl.instructions/v1"

type InvocationFacts struct {
	Watchdog       *WatchdogFacts       `json:"watchdog,omitempty"`
	Implementation *ImplementationFacts `json:"implementation,omitempty"`
}

type ReviewScope string

const (
	FullReview        ReviewScope = "full"
	IncrementalReview ReviewScope = "incremental"
)

type WatchdogFacts struct {
	Repository                 string           `json:"repository,omitempty"`
	EvidenceStreams            []EvidenceStream `json:"evidence_streams,omitempty"`
	EvidenceInstructions       string           `json:"evidence_instructions,omitempty"`
	Remote                     string           `json:"remote"`
	Worktree                   string           `json:"worktree"`
	FetchCommand               string           `json:"fetch_command"`
	WorktreeCommand            string           `json:"worktree_command"`
	InspectCommand             string           `json:"inspect_command"`
	ResultDirectory            string           `json:"result_directory"`
	SubmitCommand              string           `json:"submit_command"`
	ResumeCommand              string           `json:"resume_command"`
	ReviewCount                uint64           `json:"review_count"`
	ReviewNumber               uint64           `json:"review_number"`
	ReviewScope                ReviewScope      `json:"review_scope"`
	PreviousReviewedHead       string           `json:"previous_reviewed_head,omitempty"`
	WorkItem                   int              `json:"work_item"`
	Submission                 int              `json:"submission"`
	Branch                     string           `json:"branch"`
	ReviewedHead               string           `json:"reviewed_head"`
	SubmissionBase             string           `json:"submission_base"`
	SubmissionBodySHA256       string           `json:"submission_body_sha256"`
	ArtifactBaseline           string           `json:"artifact_baseline"`
	ArtifactCompletion         string           `json:"artifact_completion"`
	SuppliedArtifactBaseline   string           `json:"supplied_artifact_baseline,omitempty"`
	SuppliedArtifactCompletion string           `json:"supplied_artifact_completion,omitempty"`
	AuditBody                  string           `json:"audit_body"`
	Comments                   []ReviewComment  `json:"comments,omitempty"`
}

type ImplementationFacts struct {
	WorkItemReference          string              `json:"-"`
	Repository                 string              `json:"repository,omitempty"`
	Remote                     string              `json:"remote"`
	FetchCommand               string              `json:"fetch_command"`
	WorktreeCommand            string              `json:"worktree_command"`
	PushCommand                string              `json:"push_command"`
	InspectCommand             string              `json:"inspect_command"`
	NeedsHumanCommand          string              `json:"needs_human_command"`
	ResultDirectory            string              `json:"result_directory"`
	SubmitCommand              string              `json:"submit_command"`
	Procedure                  ImplementProcedure  `json:"procedure,omitempty"`
	Submission                 int                 `json:"submission,omitempty"`
	SubmissionBody             *SubmissionEvidence `json:"submission_body,omitempty"`
	EvidenceStreams            []EvidenceStream    `json:"evidence_streams,omitempty"`
	Comments                   []ReviewComment     `json:"comments,omitempty"`
	WorkItem                   int                 `json:"work_item"`
	Branch                     string              `json:"branch"`
	Worktree                   string              `json:"worktree"`
	ArtifactBaseline           string              `json:"artifact_baseline,omitempty"`
	ArtifactCompletion         string              `json:"artifact_completion,omitempty"`
	SuppliedArtifactBaseline   string              `json:"supplied_artifact_baseline,omitempty"`
	SuppliedArtifactCompletion string              `json:"supplied_artifact_completion,omitempty"`
	ResumeCommand              string              `json:"resume_command"`
	Inspection                 *InspectionFacts    `json:"inspection,omitempty"`
	Capability                 ExecutionCapability `json:"capability,omitempty"`
}

// InspectionProgress is the observed artifact-ledger progress of one prepared
// worktree. It comes from the read-only inspection, never from a lifecycle
// label, a branch name, or an attached Submission.
type InspectionProgress string

const (
	BaselineOnly      InspectionProgress = "baseline-only"
	ProvisionalLedger InspectionProgress = "provisional"
	CompletionPresent InspectionProgress = "completion-present"
	RetiredLedger     InspectionProgress = "retired"
	LedgerViolations  InspectionProgress = "violations"
)

// ExecutionCapability is the adapter capability an invocation established. It
// selects supported helper recipes for optional implementation/testing delegation
// and mandatory Audit dispatch; a harness name alone establishes nothing, and an
// unknown capability keeps a bounded runtime choice.
type ExecutionCapability string

const (
	ClaudeAgentReview ExecutionCapability = "claude-agents"
	PiSubagentReview  ExecutionCapability = "pi-subagents"
	SequentialReview  ExecutionCapability = "sequential"
	UnknownCapability ExecutionCapability = ""
)

// InspectionFacts narrows one read-only inspection to the applicable
// continuation. It is not a second full Execution Skill.
type InspectionFacts struct {
	Progress   InspectionProgress `json:"progress"`
	Violations []string           `json:"violations,omitempty"`
}

// ImplementProcedure is the engine-established submission procedure for one
// Implement invocation, decided from authoritative Workflow State rather than
// inferred from an attached Submission, branch name, feedback contents, or any
// unobserved artifact phase.
type ImplementProcedure string

const (
	InitialSubmission   ImplementProcedure = "initial"
	ResumedSubmission   ImplementProcedure = "resumed"
	FindingDrivenRework ImplementProcedure = "rework"
)

// EvidenceSource is a canonical repository-bound identity for one observed or
// required evidence stream.
type EvidenceSource string

// RepositoryEvidenceSource binds an API path to one owner/name repository.
func RepositoryEvidenceSource(repository, apiPath string) EvidenceSource {
	return EvidenceSource("repos/" + strings.Trim(repository, "/") + "/" + strings.TrimPrefix(apiPath, "/"))
}

func IssueCommentsEvidenceSource(repository string, number int) EvidenceSource {
	return RepositoryEvidenceSource(repository, fmt.Sprintf("issues/%d/comments", number))
}

func PullEvidenceSource(repository string, number int) EvidenceSource {
	return RepositoryEvidenceSource(repository, fmt.Sprintf("pulls/%d", number))
}

func PullDiscussionEvidenceSource(repository string, number int) EvidenceSource {
	return RepositoryEvidenceSource(repository, fmt.Sprintf("issues/%d/comments", number))
}

func PullReviewsEvidenceSource(repository string, number int) EvidenceSource {
	return RepositoryEvidenceSource(repository, fmt.Sprintf("pulls/%d/reviews", number))
}

func PullCommentsEvidenceSource(repository string, number int) EvidenceSource {
	return RepositoryEvidenceSource(repository, fmt.Sprintf("pulls/%d/comments", number))
}

type ReviewComment struct {
	RawBody         string `json:"-"`
	Line            int    `json:"line,omitempty"`
	Side            string `json:"side,omitempty"`
	Body            string `json:"body"`
	Author          string `json:"author"`
	Association     string `json:"association"`
	Commit          string `json:"commit,omitempty"`
	FinalHead       string `json:"final_head,omitempty"`
	Path            string `json:"path,omitempty"`
	CreatedAt       string `json:"created_at,omitempty"`
	Verdict         string `json:"verdict,omitempty"`
	ReviewNumber    uint64 `json:"review_number,omitempty"`
	ClaimAcquiredAt string `json:"claim_acquired_at,omitempty"`

	// Line is the publication/deduplication anchor. CurrentLine also preserves
	// an observed null for outdated inline comments; original anchors never replace it.
	CurrentLine       *int   `json:"current_line"`
	OriginalLine      int    `json:"original_line,omitempty"`
	StartLine         *int   `json:"start_line"`
	OriginalStartLine int    `json:"original_start_line,omitempty"`
	StartSide         string `json:"start_side,omitempty"`
	OriginalCommit    string `json:"original_commit,omitempty"`

	// EvidenceAuthorized is the backend's assertion that a comment may count as published evidence.
	EvidenceAuthorized bool `json:"evidence_authorized,omitempty"`
	// Source is the observed repository-bound stream this body came from. It is
	// part of the labeled evidence, never a rendering input.
	Source EvidenceSource `json:"source,omitempty"`
}

// EvidenceStream is one required repository-bound evidence source. Bodies is how
// many source bodies the invocation actually observed on it; Command is the
// retrieval command the worker still has to run when it was never observed.
// `fetched empty` (observed, zero bodies), `pending` (Command set), and
// `retrieval failure` (an error rather than a rendered state) are different.
type EvidenceStream struct {
	Path    string         `json:"path,omitempty"`
	State   string         `json:"state,omitempty"`
	Source  EvidenceSource `json:"source,omitempty"`
	Bodies  int            `json:"bodies,omitempty"`
	Command string         `json:"command,omitempty"`
}

// SubmissionEvidence is the attached Submission's own source body, preserved
// whole and labeled as data.
type SubmissionEvidence struct {
	Source      EvidenceSource `json:"source"`
	Author      string         `json:"author,omitempty"`
	Association string         `json:"association,omitempty"`
	CreatedAt   string         `json:"created_at,omitempty"`
	Body        string         `json:"body"`
}

type Packet struct {
	Protocol       string          `json:"protocol"`
	Skill          string          `json:"skill"`
	IncludedSkills []string        `json:"included_skills"`
	Facts          InvocationFacts `json:"facts"`
	Resources      []string        `json:"resources"`
	Instructions   string          `json:"instructions"`
}

var definitionPaths = map[string]string{
	"audit":              "skills/dev/audit/SKILL.md",
	"brainstorm":         "skills/thinking/brainstorm/SKILL.md",
	"design":             "skills/dev/design/SKILL.md",
	"domain":             "skills/dev/domain/SKILL.md",
	"explore":            "skills/dev/explore/SKILL.md",
	"implement":          "skills/dev/implement/SKILL.md",
	"propose":            "skills/dev/propose/SKILL.md",
	"shape":              "skills/thinking/shape/SKILL.md",
	"testing":            "skills/dev/testing/SKILL.md",
	"watchdog":           "skills/dev/watchdog/SKILL.md",
	"writing-for-agents": "skills/misc/writing-for-agents/SKILL.md",
}

var dependencies = map[string][]string{
	"explore":   {"domain"},
	"propose":   {"design", "testing"},
	"implement": {"testing", "audit", "design", "domain"},
}

func SkillNames() []string {
	names := make([]string, 0, len(definitionPaths))
	for name := range definitionPaths {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func BuildPacket(name string, facts InvocationFacts) (Packet, error) {
	definition, ok := definitionPaths[name]
	if !ok {
		return Packet{}, fmt.Errorf("unknown skill %q", name)
	}
	instructions, err := renderDefinition(definition, facts)
	if err != nil {
		return Packet{}, err
	}
	// A read-only inspection returns a narrow continuation, not another copy of
	// every bundled definition.
	included := dependencies[name]
	if facts.Implementation != nil && facts.Implementation.Inspection != nil {
		included = nil
	}
	for _, bundled := range included {
		rendered, err := renderDefinition(definitionPaths[bundled], facts)
		if err != nil {
			return Packet{}, err
		}
		instructions += "\n\n## Included Skill: " + bundled + "\n\n" + rendered
	}
	resources, err := resourceNames(definition)
	if err != nil {
		return Packet{}, err
	}
	return Packet{
		Protocol:       InstructionProtocol,
		Skill:          name,
		IncludedSkills: append([]string(nil), included...),
		Facts:          facts,
		Resources:      resources,
		Instructions:   instructions,
	}, nil
}

// templateFuncs is the deliberately small helper set authored templates share.
var templateFuncs = template.FuncMap{
	"quote":    ShellQuote,
	"fence":    markdownFence,
	"evidence": evidenceBlock,
	"anchor":   anchorValue,
}

func anchorValue(line *int) string {
	if line == nil {
		return "null"
	}
	return strconv.Itoa(*line)
}

// evidenceBlock chooses a fence that cannot be closed by opaque Markdown data.
func evidenceBlock(body string) string {
	fence := markdownFence(body)
	return fence + "\n" + body + "\n" + fence
}

// ShellQuote renders value as one POSIX shell word without changing any of its
// characters.
func ShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

// markdownFence returns a CommonMark fence that source text cannot close.
// Evidence remains verbatim between the delimiters even when it contains
// ordinary Markdown fences.
func markdownFence(value string) string {
	longest, current := 0, 0
	for _, character := range value {
		if character == '`' {
			current++
			longest = max(longest, current)
			continue
		}
		current = 0
	}
	return strings.Repeat("`", max(3, longest+1))
}

func renderDefinition(file string, facts InvocationFacts) (string, error) {
	return renderDocument(path.Dir(file), file, facts)
}

// renderDocument executes one embedded authored document with ordinary typed
// data. Definitions and parameterized resources share it so specialization
// never grows a second rendering mechanism, and the owning skill's private
// modules compose into both.
func renderDocument(skillDirectory, file string, data any) (string, error) {
	tmpl := template.New("modules").Option("missingkey=error").Funcs(templateFuncs)
	modules, err := fs.Glob(embedded, path.Join(skillDirectory, modulesDirectory, "*.md"))
	if err != nil {
		return "", err
	}
	for _, module := range modules {
		source, err := fs.ReadFile(embedded, module)
		if err != nil {
			return "", err
		}
		if _, err := tmpl.Parse(string(source)); err != nil {
			return "", fmt.Errorf("%s: %w", module, err)
		}
	}
	source, err := fs.ReadFile(embedded, file)
	if err != nil {
		return "", err
	}
	document, err := tmpl.New("document").Parse(string(source))
	if err != nil {
		return "", fmt.Errorf("%s: %w", file, err)
	}
	var rendered bytes.Buffer
	if err := document.Execute(&rendered, data); err != nil {
		return "", err
	}
	return rendered.String(), nil
}

// modulesDirectory holds a skill's authored internal modules: embedded and
// composable into that skill's documents, but never public resources.
const modulesDirectory = "modules"

func resourceNames(definition string) ([]string, error) {
	directory := path.Dir(definition)
	private := path.Join(directory, modulesDirectory)
	var names []string
	err := fs.WalkDir(embedded, directory, func(file string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if file == private {
				return fs.SkipDir
			}
			return nil
		}
		if file != definition {
			names = append(names, strings.TrimPrefix(file, directory+"/"))
		}
		return nil
	})
	sort.Strings(names)
	return names, err
}

func (packet Packet) Markdown() string {
	resources := strings.Join(packet.Resources, ", ")
	if resources == "" {
		resources = "none"
	}
	included := strings.Join(packet.IncludedSkills, ", ")
	if included == "" {
		included = "none"
	}
	facts, _ := json.Marshal(packet.Facts)
	return fmt.Sprintf("Protocol: %s\nSkill: %s\nIncluded skills: %s\nFacts: %s\nResources: %s\n\n%s", packet.Protocol, packet.Skill, included, facts, resources, packet.Instructions)
}

func (packet Packet) JSON() ([]byte, error) {
	return json.Marshal(packet)
}
