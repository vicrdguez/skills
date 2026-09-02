package skills

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"text/template"
)

const InstructionProtocol = "skl.instructions/v1"

type InvocationFacts struct{}

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
	return fmt.Sprintf("Protocol: %s\nSkill: %s\nIncluded skills: %s\nResources: %s\n\n%s", packet.Protocol, packet.Skill, included, resources, packet.Instructions)
}

func (packet Packet) JSON() ([]byte, error) {
	return json.Marshal(packet)
}
