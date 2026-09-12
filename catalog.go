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

type WatchdogFacts struct {
	WorkItemReference          string            `json:"-"`
	SubmissionReference        string            `json:"-"`
	Remote                     string            `json:"remote"`
	Worktree                   string            `json:"worktree"`
	ResultDirectory            string            `json:"result_directory"`
	SubmitCommand              string            `json:"submit_command"`
	ResumeCommand              string            `json:"resume_command"`
	BaselineFiles              map[string]string `json:"baseline_files"`
	CompletionFiles            map[string]string `json:"completion_files"`
	Bounces                    int               `json:"completed_bounces"`
	WorkItem                   int               `json:"work_item"`
	Submission                 int               `json:"submission"`
	Branch                     string            `json:"branch"`
	ReviewedHead               string            `json:"reviewed_head"`
	ArtifactBaseline           string            `json:"artifact_baseline"`
	ArtifactCompletion         string            `json:"artifact_completion"`
	SuppliedArtifactBaseline   string            `json:"supplied_artifact_baseline,omitempty"`
	SuppliedArtifactCompletion string            `json:"supplied_artifact_completion,omitempty"`
	AuditBody                  string            `json:"audit_body"`
	Comments                   []ReviewComment   `json:"comments,omitempty"`
}

type ImplementationFacts struct {
	WorkItemReference          string          `json:"-"`
	Remote                     string          `json:"remote"`
	InspectCommand             string          `json:"inspect_command"`
	NeedsHumanCommand          string          `json:"needs_human_command"`
	ResultDirectory            string          `json:"result_directory"`
	SubmitCommand              string          `json:"submit_command"`
	Submission                 int             `json:"submission,omitempty"`
	PreviousReviewedHead       string          `json:"previous_reviewed_head,omitempty"`
	Comments                   []ReviewComment `json:"comments,omitempty"`
	WorkItem                   int             `json:"work_item"`
	Branch                     string          `json:"branch"`
	Worktree                   string          `json:"worktree"`
	TargetSnapshot             string          `json:"target_snapshot,omitempty"`
	ArtifactBaseline           string          `json:"artifact_baseline,omitempty"`
	ArtifactCompletion         string          `json:"artifact_completion,omitempty"`
	SuppliedArtifactBaseline   string          `json:"supplied_artifact_baseline,omitempty"`
	SuppliedArtifactCompletion string          `json:"supplied_artifact_completion,omitempty"`
	ResumeCommand              string          `json:"resume_command"`
}

type ReviewComment struct {
	Line        int    `json:"line,omitempty"`
	Side        string `json:"side,omitempty"`
	Body        string `json:"body"`
	Author      string `json:"author"`
	Association string `json:"association"`
	Commit      string `json:"commit,omitempty"`
	Path        string `json:"path,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
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
		instructions += fmt.Sprintf("\n\n## Work Start\n\nWork Item: %s\nBranch: %s\nWorktree: %s\nArtifact Baseline: %s\nResume: `%s`\n", f.WorkItemReference, f.Branch, f.Worktree, f.ArtifactBaseline, f.ResumeCommand)
		if f.TargetSnapshot != "" {
			instructions += "\nBefore coding, use ordinary Git in the worktree: `git merge " + f.TargetSnapshot + "`. The engine has not merged or run project checks.\n"
		}
		if f.PreviousReviewedHead != "" {
			instructions += "\nFinding-driven Rework: sync nothing; review only `" + f.PreviousReviewedHead + "...HEAD`. Read the supplied summary, inline evidence, and human comments. Keep the ledger retired.\n"
		}
		instructions += "\nWrite the opaque Result Document using the named template, then run `" + f.SubmitCommand + "`. Refresh ledger integrity for Audit with `" + f.InspectCommand + "`. If pausing, run `" + f.NeedsHumanCommand + "` and add `--body <result>/submission.md` when preserving implementation changes.\n"
	}
	if f := facts.Watchdog; f != nil {
		instructions += fmt.Sprintf("\n\n## Review Start\n\nWork Item: %s\nSubmission: %s\nWorktree: %s\nReviewed head: %s\nArtifact Baseline: %s\nArtifact Completion: %s\nCompleted finding bounces: %d\nResume: `%s`\n\nUse the supplied historical files, opaque PR body, prior findings, and human comments. Work in this fresh Worker Session at the fixed reviewed head. The engine has not run Audit or project checks.\n\nWrite `summary.md`, optional anchored findings, and on pass `submission.md` in %s. Run `%s --verdict <pass|rework|needs-human>`. Pass also requires `--body <result>/submission.md`; optional inline inputs use `--findings <result>/findings.json`. After permitted Debt Marker comments, commit and push, run the Post-Marker Check, and supply `--head <final-sha>` while retaining the original `--reviewed-head`.\n", f.WorkItemReference, f.SubmissionReference, f.Worktree, f.ReviewedHead, f.ArtifactBaseline, f.ArtifactCompletion, f.Bounces, f.ResumeCommand, f.ResultDirectory, f.SubmitCommand)
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

func renderDefinition(file string, facts InvocationFacts) (string, error) {
	source, err := fs.ReadFile(embedded, file)
	if err != nil {
		return "", err
	}
	tmpl, err := template.New(file).Option("missingkey=error").Parse(string(source))
	if err != nil {
		return "", err
	}
	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, facts); err != nil {
		return "", err
	}
	return rendered.String(), nil
}

func resourceNames(definition string) ([]string, error) {
	directory := path.Dir(definition)
	var names []string
	err := fs.WalkDir(embedded, directory, func(file string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && file != definition {
			names = append(names, strings.TrimPrefix(file, directory+"/"))
		}
		return err
	})
	sort.Strings(names)
	return names, err
}

func Resource(name, resource string) ([]byte, error) {
	definition, ok := definitionPaths[name]
	if !ok {
		return nil, fmt.Errorf("unknown skill %q", name)
	}
	resources, err := resourceNames(definition)
	if err != nil {
		return nil, err
	}
	for _, available := range resources {
		if resource == available {
			return fs.ReadFile(embedded, path.Join(path.Dir(definition), resource))
		}
	}
	return nil, fmt.Errorf("unknown resource %q for skill %q", resource, name)
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
