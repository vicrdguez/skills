package release_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// The workflow and tests both execute release.sh. Only external command transport
// is substituted: real Git establishes ancestry, and the artifact test uses Go.
type releaseState struct {
	Exists         bool
	Draft          bool
	Target         string
	GeneratedNotes bool
	Assets         map[string][]byte
	TestsPassed    bool
	Builds         int
}

type releaseFixture struct {
	dir, state, commit string
	env                []string
}

func TestReleaseFailuresAndRecovery(t *testing.T) {
	for _, tc := range []struct {
		name, tag, failure string
		mismatchedTag      bool
		outsideMain        bool
		initial            string
		wantDraft          bool
		wantExists         bool
	}{
		{name: "prerelease", tag: "v0.5.0-rc.1"},
		{name: "other tag", tag: "nightly"},
		{name: "leading zero", tag: "v00.5.0"},
		{name: "outside main", outsideMain: true},
		{name: "tag does not identify checkout", mismatchedTag: true},
		{name: "test failure", failure: "test"},
		{name: "compilation failure", failure: "build"},
		{name: "packaging failure", failure: "tar"},
		{name: "checksum failure", failure: "shasum"},
		{name: "partial upload", failure: "upload", wantDraft: true, wantExists: true},
		{name: "publication failure", failure: "edit", wantDraft: true, wantExists: true},
		{name: "published release", initial: "published", wantExists: true},
		{name: "different draft commit", initial: "different", wantDraft: true, wantExists: true},
		{name: "lookup failure cannot overwrite", initial: "published", failure: "list", wantExists: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.tag == "" {
				tc.tag = "v0.5.0"
			}
			f := newFixture(t, tc.tag, tc.outsideMain)
			if tc.mismatchedTag {
				cmd := exec.Command("git", "tag", "--force", tc.tag, "main")
				cmd.Dir = f.dir
				if output, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("move fixture tag: %v, %s", err, output)
				}
			}
			initial := releaseState{Assets: map[string][]byte{}}
			if tc.initial != "" {
				initial.Exists = true
				initial.Draft = tc.initial == "different"
				initial.Target = "another-commit"
				initial.Assets["untouched"] = []byte("original published asset")
			}
			writeState(t, f.state, initial)
			if output, err := f.run(tc.tag, tc.failure, false); err == nil {
				t.Fatalf("release unexpectedly succeeded: %s", output)
			}
			state := readState(t, f.state)
			if state.Exists != tc.wantExists || state.Draft != tc.wantDraft {
				t.Fatalf("failure left release state %+v", state)
			}
			if tc.initial != "" && !equalAssets(state.Assets, initial.Assets) {
				t.Fatal("refused release changed assets")
			}
			if tc.failure == "upload" {
				if len(state.Assets) != 2 {
					t.Fatalf("fault did not leave a partial upload: %d assets", len(state.Assets))
				}
				if output, err := f.run(tc.tag, "", false); err != nil {
					t.Fatalf("draft recovery: %v\n%s", err, output)
				}
				assertPublished(t, readState(t, f.state), f.commit)
			}
		})
	}
}

func TestReleasePublishesCompleteTaggedArtifacts(t *testing.T) {
	f := newFixture(t, "v0.5.0", false)
	// Build the actual source, with the synthetic eligible tag and main ancestry.
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"cmd", "browse", "github", "ledger", "prose", "setup", "workflow"} {
		if err := os.CopyFS(filepath.Join(f.dir, name), os.DirFS(filepath.Join(root, name))); err != nil {
			t.Fatal(err)
		}
	}
	files, err := filepath.Glob(filepath.Join(root, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	files = append(files, filepath.Join(root, "go.mod"), filepath.Join(root, "go.sum"))
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(f.dir, filepath.Base(file)), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	if output, err := f.run("v0.5.0", "", true); err != nil {
		t.Fatalf("release artifacts: %v\n%s", err, output)
	}
	state := readState(t, f.state)
	assertPublished(t, state, f.commit)
	checksums := string(state.Assets["checksums.txt"])
	for _, target := range []struct{ os, arch string }{
		{"darwin", "amd64"}, {"darwin", "arm64"}, {"linux", "amd64"}, {"linux", "arm64"},
	} {
		name := fmt.Sprintf("skl_v0.5.0_%s_%s.tar.gz", target.os, target.arch)
		data, ok := state.Assets[name]
		if !ok {
			t.Fatalf("missing archive %s", name)
		}
		digest := sha256.Sum256(data)
		if !strings.Contains(checksums, hex.EncodeToString(digest[:])+"  ./"+name+"\n") {
			t.Fatalf("incorrect checksum for %s", name)
		}
		zip, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		archive := tar.NewReader(zip)
		header, err := archive.Next()
		if err != nil || header.Name != "skl" || header.Mode&0111 == 0 {
			t.Fatalf("archive executable: %+v, %v", header, err)
		}
		binary, err := io.ReadAll(archive)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := archive.Next(); err != io.EOF {
			t.Fatalf("unexpected additional archive entry: %v", err)
		}
		zip.Close()
		path := filepath.Join(t.TempDir(), "skl")
		if err := os.WriteFile(path, binary, 0755); err != nil {
			t.Fatal(err)
		}
		info, err := buildinfo.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		settings := map[string]string{}
		for _, setting := range info.Settings {
			settings[setting.Key] = setting.Value
		}
		if settings["GOOS"] != target.os || settings["GOARCH"] != target.arch || settings["CGO_ENABLED"] != "0" || settings["-ldflags"] != "-X main.releaseVersion=v0.5.0" {
			t.Fatalf("wrong build identity/target: %+v", settings)
		}
		if !bytes.Contains(binary, []byte("Tests show that the change keeps its Contract.")) {
			t.Fatal("distributed testing prose missing from binary")
		}
		if target.os == runtime.GOOS && target.arch == runtime.GOARCH {
			if output, err := exec.Command(path, "--version").CombinedOutput(); err != nil || string(output) != "skl version v0.5.0\n" {
				t.Fatalf("native version: %v, %s", err, output)
			}
			cmd := exec.Command(path, "skill", "testing")
			cmd.Dir = t.TempDir()
			if output, err := cmd.CombinedOutput(); err != nil || !bytes.Contains(output, []byte("Tests show that the change keeps its Contract.")) {
				t.Fatalf("embedded prose outside checkout: %v, %s", err, output)
			}
		}
	}
	if len(strings.Split(strings.TrimSpace(checksums), "\n")) != 4 {
		t.Fatal("checksum list must cover exactly four archives")
	}
	if output, err := f.run("v0.5.0", "", false); err == nil {
		t.Fatalf("published rerun succeeded: %s", output)
	}
	if !equalAssets(state.Assets, readState(t, f.state).Assets) {
		t.Fatal("published rerun replaced assets")
	}
}

func TestReleaseWorkflowIsTagOnlyAndSerial(t *testing.T) {
	data, err := os.ReadFile("../.github/workflows/release.yml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		On          map[string]struct{ Tags, Branches []string }
		Concurrency struct {
			Group  string
			Cancel bool `yaml:"cancel-in-progress"`
		}
		Jobs map[string]struct {
			Steps []struct {
				Uses string
				With map[string]any
				Env  map[string]string
				Run  string
			}
		}
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	if len(workflow.On) != 1 || len(workflow.On["push"].Tags) != 1 || workflow.On["push"].Tags[0] != "v*" || len(workflow.On["push"].Branches) != 0 {
		t.Fatalf("release trigger is not exclusively version tags: %+v", workflow.On)
	}
	if workflow.Concurrency.Group != "release-${{ github.ref }}" || workflow.Concurrency.Cancel {
		t.Fatal("release reruns must serialize without cancellation")
	}
	steps := workflow.Jobs["release"].Steps
	if len(steps) != 3 || steps[0].With["fetch-depth"] != 0 || steps[0].With["ref"] != "${{ github.sha }}" || steps[1].With["go-version-file"] != "go.mod" || steps[2].Run != "bash scripts/release.sh" || steps[2].Env["RELEASE_TAG"] != "${{ github.ref_name }}" {
		t.Fatalf("workflow is not wired to verified release entry point: %+v", steps)
	}
}

func newFixture(t *testing.T, tag string, outside bool) releaseFixture {
	t.Helper()
	f := releaseFixture{dir: t.TempDir(), state: filepath.Join(t.TempDir(), "state.json")}
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = f.dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "--initial-branch=main")
	git("config", "user.name", "Release Test")
	git("config", "user.email", "release@example.invalid")
	git("commit", "--allow-empty", "-m", "tagged source")
	f.commit = git("rev-parse", "HEAD")
	git("commit", "--allow-empty", "-m", "later main commit")
	remote := filepath.Join(t.TempDir(), "origin.git")
	git("clone", "--bare", f.dir, remote)
	git("remote", "add", "origin", remote)
	git("checkout", "--detach", f.commit)
	if outside {
		git("commit", "--allow-empty", "-m", "outside main")
		f.commit = git("rev-parse", "HEAD")
	}
	git("tag", "-a", tag, "-m", tag)
	helper, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	realGo, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	for _, name := range []string{"go", "gh"} {
		wrapper := fmt.Sprintf("#!/bin/sh\nexec \"$RELEASE_TEST_BINARY\" -test.run='^TestReleaseCommandProcess$' -- %s \"$@\"\n", name)
		if err := os.WriteFile(filepath.Join(bin, name), []byte(wrapper), 0755); err != nil {
			t.Fatal(err)
		}
	}
	f.env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "RELEASE_TEST_BINARY="+helper, "RELEASE_STATE="+f.state, "RELEASE_REAL_GO="+realGo)
	writeState(t, f.state, releaseState{Assets: map[string][]byte{}})
	return f
}

func (f releaseFixture) run(tag, failure string, realBuild bool) ([]byte, error) {
	script, err := filepath.Abs("release.sh")
	if err != nil {
		return nil, err
	}
	cmd := exec.Command("bash", script)
	cmd.Dir = f.dir
	cmd.Env = append(append([]string{}, f.env...), "RELEASE_TAG="+tag, "RELEASE_FAILURE="+failure, fmt.Sprintf("RELEASE_REAL_BUILD=%t", realBuild))
	if failure == "tar" || failure == "shasum" {
		bin, err := os.MkdirTemp("", "release-fault-")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(bin)
		if err := os.WriteFile(filepath.Join(bin, failure), []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
			return nil, err
		}
		for i, value := range cmd.Env {
			if strings.HasPrefix(value, "PATH=") {
				cmd.Env[i] = "PATH=" + bin + string(os.PathListSeparator) + strings.TrimPrefix(value, "PATH=")
			}
		}
	}
	return cmd.CombinedOutput()
}

func assertPublished(t *testing.T, state releaseState, commit string) {
	t.Helper()
	if !state.Exists || state.Draft || state.Target != commit || !state.GeneratedNotes || !state.TestsPassed || state.Builds < 4 || len(state.Assets) != 5 {
		t.Fatalf("not a complete verified normal release: %+v", state)
	}
}

func equalAssets(a, b map[string][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for name, data := range a {
		if !bytes.Equal(data, b[name]) {
			return false
		}
	}
	return true
}

func readState(t *testing.T, path string) releaseState {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var state releaseState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	return state
}

func writeState(t *testing.T, path string, state releaseState) {
	t.Helper()
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

// This process stands in for GitHub's release transport and Go failure injection.
// It never contacts GitHub. State represents observable releases and their assets.
func TestReleaseCommandProcess(t *testing.T) {
	path := os.Getenv("RELEASE_STATE")
	if path == "" {
		return
	}
	state := readState(t, path)
	args := os.Args
	for len(args) > 0 && args[0] != "--" {
		args = args[1:]
	}
	args = args[1:]
	failure := os.Getenv("RELEASE_FAILURE")
	finish := func(code int) {
		writeState(t, path, state)
		os.Exit(code)
	}
	if args[0] == "go" {
		if args[1] == failure {
			finish(1)
		}
		switch args[1] {
		case "test":
			if strings.Join(args[2:], " ") != "-timeout 20m ./..." {
				t.Fatal("test gate must run the whole suite")
			}
			state.TestsPassed = true
		case "build":
			state.Builds++
			if os.Getenv("RELEASE_REAL_BUILD") == "true" {
				cmd := exec.Command(os.Getenv("RELEASE_REAL_GO"), args[1:]...)
				cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
				if err := cmd.Run(); err != nil {
					finish(1)
				}
			} else {
				for i, arg := range args {
					if arg == "-o" {
						if err := os.WriteFile(args[i+1], []byte("controlled build"), 0755); err != nil {
							t.Fatal(err)
						}
					}
				}
			}
		default:
			t.Fatalf("unexpected Go operation: %v", args)
		}
		finish(0)
	}
	if args[0] == "gh" && args[1] == "api" {
		if failure == "list" {
			finish(1)
		}
		if state.Exists {
			fmt.Printf("%t\t%s\n", state.Draft, state.Target)
		}
		finish(0)
	}
	if args[0] != "gh" || args[1] != "release" {
		t.Fatalf("unexpected transport: %v", args)
	}
	switch args[2] {
	case "create":
		if state.Exists {
			finish(1)
		}
		state.Exists = true
		for i, arg := range args {
			switch arg {
			case "--draft":
				state.Draft = true
			case "--generate-notes":
				state.GeneratedNotes = true
			case "--target":
				state.Target = args[i+1]
			}
		}
	case "upload":
		for _, file := range args[4:] {
			if file == "--clobber" {
				continue
			}
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			state.Assets[filepath.Base(file)] = data
			if failure == "upload" && len(state.Assets) == 2 {
				finish(1)
			}
		}
	case "edit":
		if failure == "edit" {
			finish(1)
		}
		state.Draft = false
	default:
		t.Fatalf("unexpected release operation: %v", args)
	}
	finish(0)
}
