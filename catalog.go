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

	"github.com/vicrdguez/skills/ledger"
)

const InstructionProtocol = "skl.instructions/v1"

// DeliveryFacts specializes both private-ledger worker phases. Exact evidence
// is data, while the command fields are engine-bound procedures.
type DeliveryFacts struct {
	Phase                 string                    `json:"phase"`
	Procedure             string                    `json:"procedure"`
	Operation             string                    `json:"operation"`
	Repository            string                    `json:"repository"`
	Remote                string                    `json:"remote"`
	Item                  string                    `json:"item"`
	Branch                string                    `json:"branch"`
	Worktree              string                    `json:"worktree"`
	ResultDirectory       string                    `json:"result_directory"`
	Claim                 string                    `json:"claim"`
	PrepareCommand        string                    `json:"prepare_command"`
	InspectCommand        string                    `json:"inspect_command"`
	ResumeCommand         string                    `json:"resume_command"`
	ReleaseCommand        string                    `json:"release_command"`
	SubmitCommand         string                    `json:"submit_command"`
	PauseCommand          string                    `json:"pause_command,omitempty"`
	ResultResourceCommand string                    `json:"result_resource_command"`
	RequiredHead          string                    `json:"required_head,omitempty"`
	RecordedTarget        string                    `json:"recorded_target,omitempty"`
	PreviousReviewed      string                    `json:"previous_reviewed,omitempty"`
	SourceHead            string                    `json:"source_head,omitempty"`
	SourceTarget          string                    `json:"source_target,omitempty"`
	ReviewScope           string                    `json:"review_scope,omitempty"`
	FetchStatus           string                    `json:"fetch_status,omitempty"`
	ReviewCount           uint64                    `json:"review_count"`
	ReviewNumber          uint64                    `json:"review_number"`
	Capability            ExecutionCapability       `json:"capability,omitempty"`
	Documents             []ledger.ContractDocument `json:"documents"`
}

type InvocationFacts struct {
	Delivery *DeliveryFacts `json:"delivery,omitempty"`
	Decision *DecisionFacts `json:"decision,omitempty"`
}

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

// EvidenceSource is a canonical repository-bound identity for one observed or
// required evidence stream.
type EvidenceSource string

// RepositoryEvidenceSource binds an API path to one owner/name repository.
func RepositoryEvidenceSource(repository, apiPath string) EvidenceSource {
	return EvidenceSource("repos/" + strings.Trim(repository, "/") + "/" + strings.TrimPrefix(apiPath, "/"))
}

func PullReviewsEvidenceSource(repository string, number int) EvidenceSource {
	return RepositoryEvidenceSource(repository, fmt.Sprintf("pulls/%d/reviews", number))
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
	"decision":           "skills/dev/decision/SKILL.md",
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
	if facts.Delivery != nil && (facts.Delivery.Operation == "inspect" || facts.Delivery.Operation == "prepare") {
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
	"evidence": evidenceBlock,
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
