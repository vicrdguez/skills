package skills_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	skills "github.com/vicrdguez/skills"
)

func TestInstallationPlansAndResultsRetainOwnershipAndLegacyCleanup(t *testing.T) {
	home := t.TempDir()
	if _, err := skills.Install(home); err != nil {
		t.Fatal(err)
	}
	create := filepath.Join(home, ".codex/skills/audit/SKILL.md")
	update := filepath.Join(home, ".claude/skills/testing/SKILL.md")
	legacy := filepath.Join(home, ".config/opencode/skills/tdd/SKILL.md")
	collision := filepath.Join(home, ".pi/agent/skills/modules/SKILL.md")
	retiredCollision := filepath.Join(home, ".codex/skills/tdd/SKILL.md")
	if err := os.Remove(create); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		update:           "<!-- skl-owned: skl.stub/v1 -->\nstale",
		legacy:           "<!-- skl-owned: skl.stub/v1 -->\nlegacy",
		collision:        "user modules",
		retiredCollision: "user tdd",
	}
	for path, contents := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	wantActions := map[string]string{create: "create", update: "update", legacy: "remove", filepath.Dir(legacy): "remove"}
	plan, err := skills.PlanInstall(home)
	if err != nil {
		t.Fatal(err)
	}
	assertActions(t, plan.Actions, wantActions)
	wantPreserved := []string{collision, retiredCollision}
	if !reflect.DeepEqual(plan.Preserved, wantPreserved) {
		t.Fatalf("planned collisions = %v, want %v", plan.Preserved, wantPreserved)
	}
	for path, contents := range files {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != contents {
			t.Fatalf("preview changed %s: %q: %v", path, got, err)
		}
	}
	if _, err := os.Stat(create); !os.IsNotExist(err) {
		t.Fatalf("preview created file: %v", err)
	}
	outcome, err := skills.Install(home)
	if err != nil {
		t.Fatal(err)
	}
	assertActions(t, outcome.Completed, wantActions)
	if outcome.Failed != nil || outcome.Changed != 3 || !reflect.DeepEqual(outcome.Preserved, wantPreserved) {
		t.Fatalf("installation outcome = %+v", outcome)
	}
	for _, path := range wantPreserved {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != files[path] {
			t.Fatalf("preserved collision changed: %s: %q: %v", path, got, err)
		}
	}
	if _, err := os.Stat(filepath.Dir(legacy)); !os.IsNotExist(err) {
		t.Fatalf("empty legacy directory remains: %v", err)
	}
	plan, err = skills.PlanInstall(home)
	if err != nil || len(plan.Actions) != 0 || !reflect.DeepEqual(plan.Preserved, wantPreserved) {
		t.Fatalf("repeat plan = %+v: %v", plan, err)
	}
}

func assertActions(t *testing.T, actions []skills.InstallAction, want map[string]string) {
	t.Helper()
	got := map[string]string{}
	for _, action := range actions {
		if _, duplicate := got[action.Path]; duplicate {
			t.Errorf("duplicate action for %s", action.Path)
		}
		got[action.Path] = action.Action
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("actions = %v, want %v", got, want)
	}
}

// Every harness gets the verify stubs beside the existing ones; the create
// and maintain stubs are invoked by the user only.
func TestInstallPlacesTheVerifyStubsOnEveryHarness(t *testing.T) {
	home := t.TempDir()
	if _, err := skills.Install(home); err != nil {
		t.Fatal(err)
	}
	for _, directory := range []string{".pi/agent/skills", ".codex/skills", ".claude/skills", ".config/opencode/skills"} {
		for name, userInvoked := range map[string]bool{"verify": false, "verify-create": true, "verify-maintain": true, "testing": false} {
			stub, err := os.ReadFile(filepath.Join(home, directory, name, "SKILL.md"))
			if err != nil {
				t.Fatalf("%s holds no %s stub: %v", directory, name, err)
			}
			text := string(stub)
			if !strings.Contains(text, "Run `skl skill "+name+"`.") || strings.Contains(text, "disable-model-invocation: true") != userInvoked {
				t.Fatalf("%s/%s stub:\n%s", directory, name, text)
			}
		}
	}
}

// The deep-module principle is installed as modules; an owned design stub
// from an earlier installation is retired with its directory, and a user's
// own design skill is kept.
func TestInstallRetiresTheDesignStub(t *testing.T) {
	home := t.TempDir()
	owned := filepath.Join(home, ".claude/skills/design/SKILL.md")
	user := filepath.Join(home, ".codex/skills/design/SKILL.md")
	for path, contents := range map[string]string{owned: "<!-- skl-owned: skl.stub/v1 -->\nRun `skl skill design`.", user: "user design"} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := skills.Install(home); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(owned)); !os.IsNotExist(err) {
		t.Fatalf("owned design stub directory remains: %v", err)
	}
	if got, err := os.ReadFile(user); err != nil || string(got) != "user design" {
		t.Fatalf("user design skill changed: %q: %v", got, err)
	}
	for _, directory := range []string{".pi/agent/skills", ".codex/skills", ".claude/skills", ".config/opencode/skills"} {
		stub, err := os.ReadFile(filepath.Join(home, directory, "modules", "SKILL.md"))
		if err != nil || !strings.Contains(string(stub), "Run `skl skill modules`.") {
			t.Fatalf("%s modules stub: %q: %v", directory, stub, err)
		}
	}
}
