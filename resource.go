package skills

import (
	"flag"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/urfave/cli/v2"
	"github.com/vicrdguez/skills/ledger"
)

const resultDirectoryUsage = "Absolute path of the private Result Document directory this invocation created."

// reviewedHeadPattern matches the full commit SHA the engine records.
var reviewedHeadPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

// checkResultDirectory rejects a private directory that could not name the
// invocation's own result location.
func checkResultDirectory(resource, value string) error {
	if !filepath.IsAbs(value) {
		return fmt.Errorf("invalid input %q for resource %q: want the absolute path of the private Result Document directory", "result_directory", resource)
	}
	return nil
}

// submissionData, decisionData and reviewData are the ordinary typed values a
// converted resource renders from. They are never a CLI context or an
// arbitrary input-name map.
type submissionData struct {
	ResultDirectory string
	Procedure       string
}

type decisionData struct {
	ResultDirectory string
	Preserve        bool
}

type ledgerReviewData struct {
	ResultDirectory string
	Round           uint64
	ReviewedHead    string
}

type issuePublicationData struct {
	Proposal string
	Repo     string
}

type reviewData struct {
	ResultDirectory string
	PR              int
	Round           int
	ReviewedHead    string
}

// resourceInput declares one scalar a named resource accepts, using the CLI
// library's own typed flags so declaration, description and parsing share one
// schema.
type resourceInput struct {
	flag    cli.Flag
	choices []string
}

func (i resourceInput) name() string { return i.flag.Names()[0] }

func (i resourceInput) required() bool {
	required, ok := i.flag.(cli.RequiredFlag)
	return ok && required.IsRequired()
}

func (i resourceInput) usage() string {
	documented, ok := i.flag.(cli.DocGenerationFlag)
	if !ok {
		return ""
	}
	return documented.GetUsage()
}

func (i resourceInput) kind() string {
	switch i.flag.(type) {
	case *cli.BoolFlag:
		return "boolean"
	case *cli.IntFlag, *cli.Uint64Flag:
		return "integer"
	default:
		return "string"
	}
}

func (i resourceInput) wanted() string {
	switch i.kind() {
	case "boolean":
		return "a boolean value (true or false)"
	case "integer":
		return "an integer"
	default:
		return "a string value"
	}
}

// resourceSpec is one call's declaration of a resource's inputs together with
// the typed destination they populate. Every call builds fresh flags, so no
// value survives into the next invocation.
type resourceSpec struct {
	data     any
	inputs   []resourceInput
	validate func(resource string) error
}

func resourceSpecFor(resource string) resourceSpec {
	switch resource {
	case "reference/submission.md", "reference/ledger-submission.md":
		data := &submissionData{}
		return resourceSpec{data: data, inputs: []resourceInput{
			{flag: &cli.StringFlag{Name: "result_directory", Required: true, Usage: resultDirectoryUsage, Destination: &data.ResultDirectory}},
			{flag: &cli.StringFlag{Name: "procedure", Required: true, Usage: "Which submission procedure to render.", Destination: &data.Procedure}, choices: []string{"initial", "resumed", "rework"}},
		}, validate: func(resource string) error {
			return checkResultDirectory(resource, data.ResultDirectory)
		}}
	case "reference/decision.md":
		data := &decisionData{}
		return resourceSpec{data: data, inputs: []resourceInput{
			{flag: &cli.StringFlag{Name: "result_directory", Required: true, Usage: resultDirectoryUsage, Destination: &data.ResultDirectory}},
			{flag: &cli.BoolFlag{Name: "preserve", Required: true, Usage: "Whether implementation work exists that a draft Submission must preserve.", Destination: &data.Preserve}},
		}, validate: func(resource string) error {
			return checkResultDirectory(resource, data.ResultDirectory)
		}}
	case "reference/ledger-review.md":
		data := &ledgerReviewData{}
		return resourceSpec{data: data, inputs: []resourceInput{
			{flag: &cli.StringFlag{Name: "result_directory", Required: true, Usage: resultDirectoryUsage, Destination: &data.ResultDirectory}},
			{flag: &cli.Uint64Flag{Name: "round", Required: true, Usage: "Next completed Work Item review round.", Destination: &data.Round}},
			{flag: &cli.StringFlag{Name: "reviewed_head", Required: true, Usage: "Fixed reviewed source commit.", Destination: &data.ReviewedHead}},
		}, validate: func(resource string) error {
			if data.Round < 1 || !reviewedHeadPattern.MatchString(data.ReviewedHead) {
				return fmt.Errorf("resource %s requires a positive round and full reviewed source SHA (40 lowercase hexadecimal characters for schema 1)", resource)
			}
			return checkResultDirectory(resource, data.ResultDirectory)
		}}
	case "reference/issue-publication.md":
		data := &issuePublicationData{}
		return resourceSpec{data: data, inputs: []resourceInput{
			{flag: &cli.StringFlag{Name: "proposal", Required: true, Usage: "Accepted proposal whose issue presentation is published.", Destination: &data.Proposal}},
			{flag: &cli.StringFlag{Name: "repo", Required: true, Usage: "Absolute path of the source repository root.", Destination: &data.Repo}},
		}, validate: func(resource string) error {
			if !ledger.ValidRecordName(data.Proposal) {
				return fmt.Errorf("invalid input %q for resource %q: want the accepted proposal name", "proposal", resource)
			}
			if !filepath.IsAbs(data.Repo) {
				return fmt.Errorf("invalid input %q for resource %q: want the absolute path of the source repository root", "repo", resource)
			}
			return nil
		}}
	case "reference/review.md":
		data := &reviewData{}
		return resourceSpec{data: data, inputs: []resourceInput{
			{flag: &cli.StringFlag{Name: "result_directory", Required: true, Usage: resultDirectoryUsage, Destination: &data.ResultDirectory}},
			{flag: &cli.IntFlag{Name: "pr", Required: true, Usage: "Selected Submission PR number.", Destination: &data.PR}},
			{flag: &cli.IntFlag{Name: "round", Required: true, Usage: "Review round number for this Submission.", Destination: &data.Round}},
			{flag: &cli.StringFlag{Name: "reviewed_head", Required: true, Usage: "Original full SHA of the reviewed head.", Destination: &data.ReviewedHead}},
		}, validate: func(resource string) error {
			if data.PR < 1 {
				return fmt.Errorf("invalid input %q for resource %q: want a positive Submission PR number", "pr", resource)
			}
			if data.Round < 1 {
				return fmt.Errorf("invalid input %q for resource %q: want a positive review round", "round", resource)
			}
			if err := checkResultDirectory(resource, data.ResultDirectory); err != nil {
				return err
			}
			if !reviewedHeadPattern.MatchString(data.ReviewedHead) {
				return fmt.Errorf("invalid input %q for resource %q: want the full 40-character SHA of the original reviewed head", "reviewed_head", resource)
			}
			return nil
		}}
	}
	return resourceSpec{}
}

func (s resourceSpec) declared(name string) *resourceInput {
	for i := range s.inputs {
		if s.inputs[i].name() == name {
			return &s.inputs[i]
		}
	}
	return nil
}

func discovery(name, resource string) string {
	return fmt.Sprintf("run `skl skill --resource %s --describe-inputs %s` to list accepted inputs", resource, name)
}

// parse validates every raw assignment against the declared contract before
// any procedure is rendered.
func (s resourceSpec) parse(name, resource string, assignments []string) error {
	set := flag.NewFlagSet(resource, flag.ContinueOnError)
	for _, input := range s.inputs {
		if err := input.flag.Apply(set); err != nil {
			return err
		}
	}
	assigned := map[string]bool{}
	for _, assignment := range assignments {
		key, value, ok := strings.Cut(assignment, "=")
		if !ok {
			return fmt.Errorf("invalid --input %q: want name=value", assignment)
		}
		if assigned[key] {
			return fmt.Errorf("duplicate input %q for resource %q", key, resource)
		}
		assigned[key] = true
		declared := s.declared(key)
		if declared == nil {
			return fmt.Errorf("unknown input %q for resource %q; %s", key, resource, discovery(name, resource))
		}
		if err := set.Set(key, value); err != nil {
			return fmt.Errorf("invalid input %q for resource %q: want %s", key, resource, declared.wanted())
		}
	}
	present := map[string]bool{}
	set.Visit(func(flag *flag.Flag) { present[flag.Name] = true })
	for _, input := range s.inputs {
		if input.required() && !present[input.name()] {
			return fmt.Errorf("missing required input %q for resource %q; %s", input.name(), resource, discovery(name, resource))
		}
	}
	for _, input := range s.inputs {
		if len(input.choices) == 0 || !present[input.name()] {
			continue
		}
		if value := set.Lookup(input.name()).Value.String(); !slices.Contains(input.choices, value) {
			return fmt.Errorf("invalid input %q for resource %q: supported choices are %s", input.name(), resource, strings.Join(input.choices, ", "))
		}
	}
	if s.validate != nil {
		return s.validate(resource)
	}
	return nil
}

// RenderResource returns one public named resource rendered from raw --input
// assignments. It never touches the Workflow Backend.
func RenderResource(name, resource string, assignments []string) ([]byte, error) {
	file, err := resourceFile(name, resource)
	if err != nil {
		return nil, err
	}
	spec := resourceSpecFor(resource)
	if err := spec.parse(name, resource, assignments); err != nil {
		return nil, err
	}
	rendered, err := renderDocument(path.Dir(definitionPaths[name]), file, spec.data)
	if err != nil {
		return nil, err
	}
	return []byte(rendered), nil
}

// DescribeResourceInputs reports the accepted inputs of one public named
// resource without disclosing its procedure. It never touches the Workflow
// Backend.
func DescribeResourceInputs(name, resource string) (string, error) {
	if _, err := resourceFile(name, resource); err != nil {
		return "", err
	}
	spec := resourceSpecFor(resource)
	if len(spec.inputs) == 0 {
		return resource + " accepts no inputs.\n", nil
	}
	var description strings.Builder
	description.WriteString(resource + " accepts:\n")
	for _, input := range spec.inputs {
		status := "optional"
		if input.required() {
			status = "required"
		}
		fmt.Fprintf(&description, "  %s (%s, %s", input.name(), input.kind(), status)
		if len(input.choices) > 0 {
			description.WriteString(", one of: " + strings.Join(input.choices, ", "))
		}
		description.WriteString("): " + input.usage() + "\n")
	}
	return description.String(), nil
}

// resourceFile resolves one public resource to its embedded path. Private
// modules and `SKILL.md` are not resources.
func resourceFile(name, resource string) (string, error) {
	definition, ok := definitionPaths[name]
	if !ok {
		return "", fmt.Errorf("unknown skill %q", name)
	}
	resources, err := resourceNames(definition)
	if err != nil {
		return "", err
	}
	for _, available := range resources {
		if resource == available {
			return path.Join(path.Dir(definition), resource), nil
		}
	}
	return "", fmt.Errorf("unknown resource %q for skill %q", resource, name)
}
