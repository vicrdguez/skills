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
// selects supported helper recipes for optional implementation/testing
// delegation; a harness name alone establishes nothing, and an unknown
// capability keeps a bounded runtime choice. Audit dispatch never reads it.
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

	// CurrentLine preserves an observed null for outdated inline comments;
	// original anchors never replace it.
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

// proseRoot holds the authored prose, organized by kind: procedures, craft,
// documents, outcomes and adapters. Every procedure, craft, document and
// outcome file is a template named by its path under this root.
const proseRoot = "prose"

// skill is what `skl skill <name>` renders: its definition, and the directory
// whose files are its public resources ("" when it has none).
type skill struct {
	definition string
	resources  string
}

var skills = map[string]skill{
	"audit":              {"procedures/audit.md", "craft/audit"},
	"brainstorm":         {"procedures/brainstorm.md", ""},
	"decision":           {"procedures/decision.md", "procedures/decision"},
	"design":             {"craft/design.md", "craft/design"},
	"domain":             {"craft/domain.md", "craft/domain"},
	"explore":            {"procedures/explore.md", ""},
	"implement":          {"procedures/implement.md", "documents/implement"},
	"propose":            {"procedures/propose.md", "documents/propose"},
	"shape":              {"procedures/shape.md", ""},
	"testing":            {"craft/testing.md", "craft/testing"},
	"watchdog":           {"procedures/watchdog.md", "documents/watchdog"},
	"writing-for-agents": {"craft/writing-for-agents.md", "craft/writing-for-agents"},
}

// composition is the Craft and Procedures a Procedure inlines after its own
// text on every run.
var composition = map[string][]string{
	"explore":   {"domain"},
	"propose":   {"design", "testing"},
	"implement": {"testing", "audit"},
}

func SkillNames() []string {
	names := make([]string, 0, len(skills))
	for name := range skills {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// procedure selects the file an invocation renders for a skill, and the facts
// it renders from. Delivery continuations and the Audit step inside Implement
// are procedures of their own.
func procedure(name string, facts InvocationFacts) (string, any) {
	switch {
	case facts.Delivery != nil && name == "audit":
		return "procedures/audit-step.md", facts.Delivery
	case facts.Delivery != nil && (facts.Delivery.Operation == "prepare" || facts.Delivery.Operation == "inspect"):
		return "procedures/" + name + "-" + facts.Delivery.Operation + ".md", facts.Delivery
	case facts.Delivery != nil:
		return skills[name].definition, facts.Delivery
	case facts.Decision != nil:
		return "procedures/decision-inbox.md", facts.Decision
	}
	return skills[name].definition, nil
}

func BuildPacket(name string, facts InvocationFacts) (Packet, error) {
	if _, ok := skills[name]; !ok {
		return Packet{}, fmt.Errorf("unknown skill %q", name)
	}
	file, data := procedure(name, facts)
	instructions, err := renderDocument(file, data)
	if err != nil {
		return Packet{}, err
	}
	// A read-only inspection returns a narrow continuation, not another copy of
	// every composed skill.
	included := composition[name]
	if facts.Delivery != nil && (facts.Delivery.Operation == "inspect" || facts.Delivery.Operation == "prepare") {
		included = nil
	}
	for _, composed := range included {
		file, data := procedure(composed, facts)
		rendered, err := renderDocument(file, data)
		if err != nil {
			return Packet{}, err
		}
		instructions += "\n\n" + rendered
	}
	resources, err := resourceNames(name)
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

// RenderOutcome renders the Outcome Instruction of one outcome kind,
// prose/outcomes/<kind>.md, from the facts a command established.
func RenderOutcome(kind string, facts any) (string, error) {
	return renderDocument("outcomes/"+kind+".md", facts)
}

// renderDocument executes one authored prose file with ordinary typed data.
// Every procedure, craft, document and outcome file parses into one template
// set, so a file includes another by its path and shared blocks by their
// defined name.
func renderDocument(file string, data any) (string, error) {
	tmpl := template.New("").Option("missingkey=error").Funcs(templateFuncs)
	for _, kind := range []string{"procedures", "craft", "documents", "outcomes"} {
		err := fs.WalkDir(embedded, path.Join(proseRoot, kind), func(source string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			contents, err := fs.ReadFile(embedded, source)
			if err != nil {
				return err
			}
			if _, err := tmpl.New(strings.TrimPrefix(source, proseRoot+"/")).Parse(string(contents)); err != nil {
				return fmt.Errorf("%s: %w", source, err)
			}
			return nil
		})
		if err != nil {
			return "", err
		}
	}
	var rendered bytes.Buffer
	if err := tmpl.ExecuteTemplate(&rendered, file, data); err != nil {
		return "", err
	}
	return rendered.String(), nil
}

// resourceNames lists a skill's public resources by their path under its
// resource directory.
func resourceNames(name string) ([]string, error) {
	directory := skills[name].resources
	if directory == "" {
		return nil, nil
	}
	root := path.Join(proseRoot, directory)
	var names []string
	err := fs.WalkDir(embedded, root, func(file string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		names = append(names, strings.TrimPrefix(file, root+"/"))
		return nil
	})
	sort.Strings(names)
	return names, err
}

func (packet Packet) JSON() ([]byte, error) {
	return json.Marshal(packet)
}
