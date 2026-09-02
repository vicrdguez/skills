package skills

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

const StubProtocol = "skl.stub/v1"

var skillNames = []string{
	"audit", "brainstorm", "design", "domain", "explore", "implement",
	"propose", "shape", "tdd", "watchdog", "writing-for-agents",
}

//go:embed skills stubs/common.md
var embedded embed.FS

type stubData struct {
	Name     string
	Protocol string
}

func Install(home string) error {
	tmpl, err := template.ParseFS(embedded, "stubs/common.md")
	if err != nil {
		return err
	}
	for _, harness := range []string{".pi/agent/skills", ".codex/skills", ".claude/skills"} {
		for _, name := range skillNames {
			directory := filepath.Join(home, harness, name)
			if err := os.MkdirAll(directory, 0o755); err != nil {
				return err
			}
			path := filepath.Join(directory, "SKILL.md")
			current, err := os.ReadFile(path)
			if err == nil && !bytes.HasPrefix(current, []byte("<!-- skl-owned:")) {
				continue
			}
			if err != nil && !os.IsNotExist(err) {
				return err
			}
			var contents bytes.Buffer
			err = tmpl.Execute(&contents, stubData{Name: name, Protocol: StubProtocol})
			if err == nil {
				err = os.WriteFile(path, contents.Bytes(), 0o644)
			}
			if err != nil {
				return fmt.Errorf("install %s for %s: %w", name, harness, err)
			}
		}
	}
	return nil
}
