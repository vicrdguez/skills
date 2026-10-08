package main

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"testing"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/setup"
)

func TestVersionIdentifiesModuleInstallsAndCheckoutBuilds(t *testing.T) {
	const mainVersion = "v0.0.0-20261008101046-f7ca70b3f55a"
	for _, test := range []struct {
		name string
		info *debug.BuildInfo
		want string
	}{
		{"tagged module", &debug.BuildInfo{Main: debug.Module{Version: "v0.5.0"}}, "v0.5.0"},
		{"main module", &debug.BuildInfo{Main: debug.Module{Version: mainVersion}}, mainVersion},
		{"checkout with VCS version", &debug.BuildInfo{
			Main:     debug.Module{Version: mainVersion},
			Settings: []debug.BuildSetting{{Key: "vcs", Value: "git"}, {Key: "vcs.revision", Value: "f7ca70b3f55a"}},
		}, "development"},
		{"checkout without version", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, "development"},
		{"missing build information", nil, "development"},
	} {
		t.Run(test.name, func(t *testing.T) {
			withBuildInfo(t, test.info)
			var stdout, stderr bytes.Buffer
			app := newApp(func(github.RepositoryID) (setup.Backend, error) {
				t.Fatal("version contacted backend")
				return nil, nil
			}, bytes.NewReader(nil), &stdout, &stderr)
			if err := app.Run([]string{"skl", "--version"}); err != nil {
				t.Fatalf("skl --version: %v; stderr: %s", err, stderr.String())
			}
			if got, want := stdout.String(), "skl version "+test.want+"\n"; got != want {
				t.Fatalf("skl --version = %q, want %q", got, want)
			}
		})
	}
}

func TestReleaseBuildVersionStampOverridesCheckoutMetadata(t *testing.T) {
	if got, want := versionFromBuild(t, "-ldflags=-X main.releaseVersion=v0.5.0"), "skl version v0.5.0\n"; got != want {
		t.Fatalf("release skl --version = %q, want %q", got, want)
	}
}

func TestUnstampedCheckoutBuildIdentifiesDevelopment(t *testing.T) {
	if got, want := versionFromBuild(t, "-buildvcs=false"), "skl version development\n"; got != want {
		t.Fatalf("checkout skl --version = %q, want %q", got, want)
	}
}

func versionFromBuild(t *testing.T, flags ...string) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "skl")
	args := append([]string{"build"}, flags...)
	args = append(args, "-o", binary, ".")
	if output, err := exec.Command("go", args...).CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, output)
	}
	output, err := exec.Command(binary, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("skl --version: %v\n%s", err, output)
	}
	return string(output)
}
