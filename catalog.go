package skills

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"sort"
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
	WorkItemReference          string          `json:"-"`
	SubmissionReference        string          `json:"-"`
	Remote                     string          `json:"remote"`
	Worktree                   string          `json:"worktree"`
	FetchCommand               string          `json:"fetch_command"`
	WorktreeCommand            string          `json:"worktree_command"`
	InspectCommand             string          `json:"inspect_command"`
	ResultDirectory            string          `json:"result_directory"`
	SubmitCommand              string          `json:"submit_command"`
	ResumeCommand              string          `json:"resume_command"`
	ReviewCount                uint64          `json:"review_count"`
	ReviewNumber               uint64          `json:"review_number"`
	ReviewScope                ReviewScope     `json:"review_scope"`
	PreviousReviewedHead       string          `json:"previous_reviewed_head,omitempty"`
	WorkItem                   int             `json:"work_item"`
	Submission                 int             `json:"submission"`
	Branch                     string          `json:"branch"`
	ReviewedHead               string          `json:"reviewed_head"`
	ArtifactBaseline           string          `json:"artifact_baseline"`
	ArtifactCompletion         string          `json:"artifact_completion"`
	SuppliedArtifactBaseline   string          `json:"supplied_artifact_baseline,omitempty"`
	SuppliedArtifactCompletion string          `json:"supplied_artifact_completion,omitempty"`
	AuditBody                  string          `json:"audit_body"`
	Comments                   []ReviewComment `json:"comments,omitempty"`
}

type ImplementationFacts struct {
	WorkItemReference          string             `json:"-"`
	Remote                     string             `json:"remote"`
	FetchCommand               string             `json:"fetch_command"`
	WorktreeCommand            string             `json:"worktree_command"`
	InspectCommand             string             `json:"inspect_command"`
	NeedsHumanCommand          string             `json:"needs_human_command"`
	ResultDirectory            string             `json:"result_directory"`
	SubmitCommand              string             `json:"submit_command"`
	Procedure                  ImplementProcedure `json:"procedure,omitempty"`
	Submission                 int                `json:"submission,omitempty"`
	Comments                   []ReviewComment    `json:"comments,omitempty"`
	WorkItem                   int                `json:"work_item"`
	Branch                     string             `json:"branch"`
	Worktree                   string             `json:"worktree"`
	ArtifactBaseline           string             `json:"artifact_baseline,omitempty"`
	ArtifactCompletion         string             `json:"artifact_completion,omitempty"`
	SuppliedArtifactBaseline   string             `json:"supplied_artifact_baseline,omitempty"`
	SuppliedArtifactCompletion string             `json:"supplied_artifact_completion,omitempty"`
	ResumeCommand              string             `json:"resume_command"`
}

// ImplementProcedure is the engine-established submission procedure for one
// Implement invocation, decided from authoritative Workflow State rather than
// inferred from an attached Submission.
type ImplementProcedure string

const (
	InitialSubmission   ImplementProcedure = "initial"
	FindingDrivenRework ImplementProcedure = "rework"
)

type ReviewComment struct {
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
	"tdd":                "skills/dev/tdd/SKILL.md",
	"watchdog":           "skills/dev/watchdog/SKILL.md",
	"writing-for-agents": "skills/misc/writing-for-agents/SKILL.md",
}

var dependencies = map[string][]string{
	"explore":   {"domain"},
	"propose":   {"design", "tdd"},
	"implement": {"tdd", "audit", "design", "domain"},
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
	for _, included := range dependencies[name] {
		rendered, err := renderDefinition(definitionPaths[included], facts)
		if err != nil {
			return Packet{}, err
		}
		instructions += "\n\n## Included Skill: " + included + "\n\n" + rendered
	}
	if facts.Implementation != nil {
		f := facts.Implementation
		instructions += fmt.Sprintf("\n\n## Work Start\n\nWork Item: %s\nBranch: %s\nWorktree: %s\nPrepare: `%s` then `%s`; safely reuse a clean existing worktree instead of recreating it\nInspect: `%s` resolves the Artifact Baseline and Completion from the fetched history\nResume: `%s`\n", f.WorkItemReference, f.Branch, f.Worktree, f.FetchCommand, f.WorktreeCommand, f.InspectCommand, f.ResumeCommand)
		if f.Procedure == FindingDrivenRework {
			instructions += "\nFinding-driven Rework: sync nothing; inspect the current PR comparison, supplied summary, inline evidence, and human comments. Keep the ledger retired.\n"
		}
		instructions += "\nWrite the opaque Result Document using the named template, then run `" + f.SubmitCommand + "`. If pausing, run `" + f.NeedsHumanCommand + "` and add `--body <result>/submission.md` when preserving implementation changes.\n"
	}
	if f := facts.Watchdog; f != nil {
		instructions += fmt.Sprintf("\n\n## Review Start\n\nWork Item: %s\nSubmission: %s\nWorktree: %s\nReviewed head: %s\nCompleted reviews: %d\nReview number: %d\nScope: %s; the comparison rule below applies after Git preparation\nPrepare: `%s` then `%s`; safely reuse a clean existing worktree instead of recreating it\nInspect: `%s` resolves the Artifact Baseline and Completion from the fetched history\nResume: `%s`\n\nRun the Inspect command after preparing the worktree, read the endpoint files from Git at the resolved Baseline and Completion, then use the opaque Submission body, prior findings, and human comments. Review the invocation's current head; rerun the Full Gate, active-finding verification, artifact checks, and whole-change critical-class scan. The engine has not run Audit or project checks.\n\nWrite `summary.md`, optional anchored findings, and on pass `submission.md` in %s. Run `%s --verdict <pass|rework|needs-human>`. Pass also requires `--body <result>/submission.md`; optional inline inputs use `--findings <result>/findings.json`. After permitted Debt Marker comments, commit and push, run the Post-Marker Check, and supply `--head <final-sha>` while retaining the original `--reviewed-head`.\n", f.WorkItemReference, f.SubmissionReference, f.Worktree, f.ReviewedHead, f.ReviewCount, f.ReviewNumber, f.ReviewScope, f.FetchCommand, f.WorktreeCommand, f.InspectCommand, f.ResumeCommand, f.ResultDirectory, f.SubmitCommand)
		if f.PreviousReviewedHead != "" {
			instructions += "\nAfter preparation, compare `" + f.PreviousReviewedHead + "..." + f.ReviewedHead + "` when that prior revision is available and an ancestor of the reviewed head; otherwise review the full PR comparison.\n"
		} else {
			instructions += "\nReview the full PR comparison; no usable retained reviewed revision is required or fetched.\n"
		}
	}
	resources, err := resourceNames(definition)
	if err != nil {
		return Packet{}, err
	}
	return Packet{
		Protocol:       InstructionProtocol,
		Skill:          name,
		IncludedSkills: append([]string(nil), dependencies[name]...),
		Facts:          facts,
		Resources:      resources,
		Instructions:   instructions,
	}, nil
}

// templateFuncs is the deliberately small helper set authored templates share.
var templateFuncs = template.FuncMap{"quote": ShellQuote}

// ShellQuote renders value as one POSIX shell word without changing any of its
// characters.
func ShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
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
