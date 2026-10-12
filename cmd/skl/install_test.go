package main

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	skilldist "github.com/vicrdguez/skills"
)

// observeInput checks the CLI and filesystem at the point confirmation is read.
type observeInput struct {
	io.Reader
	observe func()
}

func (input *observeInput) Read(p []byte) (int, error) {
	if input.observe != nil {
		input.observe()
		input.observe = nil
	}
	return input.Reader.Read(p)
}

func writeInstallFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

type installedFile struct {
	contents string
	mode     fs.FileMode
	modified time.Time
}

func installSnapshot(t *testing.T, home string) map[string]installedFile {
	t.Helper()
	files := map[string]installedFile{}
	if err := filepath.WalkDir(home, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		file := installedFile{mode: info.Mode(), modified: info.ModTime()}
		if !entry.IsDir() {
			contents, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			file.contents = string(contents)
		}
		files[path] = file
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return files
}

// installationChanges leaves one create, one update and legacy removals among
// current files, with a separate user-owned collision.
func installationChanges(t *testing.T, home string) (actions []skilldist.InstallAction, current map[string]string, collision string) {
	t.Helper()
	if _, err := skilldist.Install(home); err != nil {
		t.Fatal(err)
	}
	create := filepath.Join(home, ".codex/skills/audit/SKILL.md")
	update := filepath.Join(home, ".pi/agent/skills/testing/SKILL.md")
	legacy := filepath.Join(home, ".pi/agent/skills/tdd/SKILL.md")
	runner := filepath.Join(home, ".pi/agent/agents/implement-runner.md")
	current = map[string]string{create: readFile(t, create), update: readFile(t, update)}
	if err := os.Remove(create); err != nil {
		t.Fatal(err)
	}
	writeInstallFile(t, update, "<!-- skl-owned: skl.stub/v1 -->\nstale")
	writeInstallFile(t, legacy, "<!-- skl-owned: skl.stub/v1 -->\nlegacy")
	writeInstallFile(t, runner, "skl-owned: skl.pi/v1\nlegacy runner")
	collision = filepath.Join(home, ".codex/skills/domain/SKILL.md")
	writeInstallFile(t, collision, "my domain skill")
	return []skilldist.InstallAction{
		{Path: create, Action: "create"},
		{Path: update, Action: "update"},
		{Path: legacy, Action: "remove"},
		{Path: filepath.Dir(legacy), Action: "remove"},
		{Path: runner, Action: "remove"},
	}, current, collision
}

func TestInstallPreviewsBeforeAcceptanceAndReportsActualCompletion(t *testing.T) {
	home := t.TempDir()
	actions, current, collision := installationChanges(t, home)
	before := installSnapshot(t, home)
	var output bytes.Buffer
	input := &observeInput{Reader: strings.NewReader("YeS\n"), observe: func() {
		if got := installSnapshot(t, home); !reflect.DeepEqual(got, before) {
			t.Fatal("installation mutated the filesystem before confirmation")
		}
		preview, prompt, found := strings.Cut(output.String(), "Install these changes?")
		if !found || !strings.Contains(prompt, "[y/N]") || strings.Contains(preview, "Completed changes:") {
			t.Fatalf("preview/confirmation = %q", output.String())
		}
		for _, action := range actions {
			if !strings.Contains(preview, action.Action+" "+action.Path+"\n") {
				t.Errorf("preview missing %v: %s", action, preview)
			}
		}
		if !strings.Contains(preview, "Preserved user-owned collision: "+collision) {
			t.Errorf("collision not identified separately: %s", preview)
		}
	}}
	app := newAppWithSkillHome(nil, input, &output, &output, home)
	if err := app.Run([]string{"skl", "install"}); err != nil {
		t.Fatal(err)
	}
	if input.observe != nil {
		t.Fatal("confirmation was not read")
	}
	_, completed, found := strings.Cut(output.String(), "Completed changes:")
	if !found || !strings.Contains(completed, "Installation completed successfully.") {
		t.Fatalf("completion not confirmed: %s", output.String())
	}
	for _, action := range actions {
		if !strings.Contains(completed, action.Action+" "+action.Path+"\n") {
			t.Errorf("completed report missing %v: %s", action, completed)
		}
		if action.Action == "remove" {
			if _, err := os.Stat(action.Path); !os.IsNotExist(err) {
				t.Errorf("legacy path remains: %s: %v", action.Path, err)
			}
		} else if got := readFile(t, action.Path); got != current[action.Path] {
			t.Errorf("%s was not installed: %q", action.Path, got)
		}
	}
	if got := readFile(t, collision); got != "my domain skill" {
		t.Errorf("collision changed: %q", got)
	}
	if strings.Contains(completed, "update "+collision) || strings.Contains(completed, "create "+collision) {
		t.Errorf("collision reported as installed: %s", completed)
	}
}

func TestInstallDeclineDefaultAndNoAnswerLeaveFilesUntouched(t *testing.T) {
	for _, answer := range []string{"n\n", "no\n", "\n", "", "not yes\n"} {
		t.Run(answer, func(t *testing.T) {
			home := t.TempDir()
			installationChanges(t, home)
			before := installSnapshot(t, home)
			var output bytes.Buffer
			app := newAppWithSkillHome(nil, strings.NewReader(answer), &output, &output, home)
			if err := app.Run([]string{"skl", "install"}); err != nil {
				t.Fatal(err)
			}
			if got := installSnapshot(t, home); !reflect.DeepEqual(got, before) {
				t.Fatal("cancelled installation changed the filesystem")
			}
			if !strings.Contains(output.String(), "Installation cancelled.") || strings.Contains(output.String(), "Completed changes:") {
				t.Fatalf("cancellation output = %q", output.String())
			}
		})
	}
}

func TestInstallUpToDateSkipsConfirmationAndSeparatesCollisions(t *testing.T) {
	for _, scenario := range []string{"all current", "preserved collision"} {
		t.Run(scenario, func(t *testing.T) {
			home := t.TempDir()
			if _, err := skilldist.Install(home); err != nil {
				t.Fatal(err)
			}
			collision := filepath.Join(home, ".codex/skills/domain/SKILL.md")
			if scenario == "preserved collision" {
				writeInstallFile(t, collision, "my domain skill")
			}
			before := installSnapshot(t, home)
			var output bytes.Buffer
			input := &observeInput{Reader: strings.NewReader(""), observe: func() { t.Fatal("no-op installation requested confirmation") }}
			app := newAppWithSkillHome(nil, input, &output, &output, home)
			if err := app.Run([]string{"skl", "install"}); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output.String(), "Already up to date") || strings.Contains(output.String(), "Install these changes?") {
				t.Fatalf("no-op output = %q", output.String())
			}
			if scenario == "preserved collision" && !strings.Contains(output.String(), "Preserved user-owned collision: "+collision) {
				t.Fatalf("collision not identified separately: %s", output.String())
			}
			if got := installSnapshot(t, home); !reflect.DeepEqual(got, before) {
				t.Fatal("no-op installation changed the filesystem")
			}
		})
	}
}

func TestInstallFailureReportsOnlyCompletedActionsAndDoesNotRollBack(t *testing.T) {
	home := t.TempDir()
	if _, err := skilldist.Install(home); err != nil {
		t.Fatal(err)
	}
	completed := filepath.Join(home, ".pi/agent/skills/audit/SKILL.md")
	failed := filepath.Join(home, ".pi/agent/skills/brainstorm/SKILL.md")
	later := filepath.Join(home, ".pi/agent/skills/decision/SKILL.md")
	wantCompleted := readFile(t, completed)
	for _, path := range []string{completed, failed, later} {
		writeInstallFile(t, path, "<!-- skl-owned: skl.stub/v1 -->\nstale")
	}
	var output bytes.Buffer
	input := &observeInput{Reader: strings.NewReader("y\n"), observe: func() {
		// A dangling symlink with a missing target parent makes WriteFile fail
		// after planning, without depending on permissions or /dev/full.
		if err := os.Remove(failed); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(home, "missing-parent", "stub"), failed); err != nil {
			t.Fatal(err)
		}
	}}
	app := newAppWithSkillHome(nil, input, &output, &output, home)
	if err := app.Run([]string{"skl", "install"}); err == nil {
		t.Fatal("installation succeeded despite filesystem failure")
	}
	_, report, found := strings.Cut(output.String(), "Completed changes:")
	if !found || !strings.Contains(report, "update "+completed+"\n") || !strings.Contains(report, "Failed: create "+failed+"\n") || !strings.Contains(report, "Installation did not finish.") {
		t.Fatalf("partial failure report = %q", output.String())
	}
	if strings.Contains(report, "update "+later) || strings.Contains(report, "update "+failed) || strings.Contains(report, "completed successfully") {
		t.Fatalf("unexecuted actions reported as completed: %s", report)
	}
	if got := readFile(t, completed); got != wantCompleted {
		t.Errorf("completed update was rolled back: %q", got)
	}
	if got := readFile(t, later); got != "<!-- skl-owned: skl.stub/v1 -->\nstale" {
		t.Errorf("later action ran: %q", got)
	}
}

type failedInstallInput struct{}

func (failedInstallInput) Read([]byte) (int, error) { return 0, errors.New("input unavailable") }

func TestInstallInputFailureLeavesFilesUntouched(t *testing.T) {
	home := t.TempDir()
	before := installSnapshot(t, home)
	var output bytes.Buffer
	app := newAppWithSkillHome(nil, failedInstallInput{}, &output, &output, home)
	if err := app.Run([]string{"skl", "install"}); err == nil || !strings.Contains(err.Error(), "input unavailable") {
		t.Fatalf("input failure = %v", err)
	}
	if got := installSnapshot(t, home); !reflect.DeepEqual(got, before) {
		t.Fatal("input failure changed the filesystem")
	}
}
