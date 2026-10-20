package main

// This is an experimental overlay, not part of the default test package.
// It deliberately stops short of a runner: the accompanying probe demonstrates
// why direct self-reexec alone is not a safe replacement for native execution.
import (
	"flag"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"
)

func isolationPilot(t *testing.T) bool {
	t.Helper()
	if os.Getenv("SKL_ISOLATION_PILOT_MODE") != "isolated" || os.Getenv("SKL_ISOLATION_PILOT_CHILD") == t.Name() {
		return false
	}
	t.Parallel()
	// The pilot only accepts simple top-level filters, optionally followed by
	// a subtest filter. General Go matcher syntax is not implemented here.
	_, nested, hasNested := strings.Cut(flag.Lookup("test.run").Value.String(), "/")
	selection := "^" + regexp.QuoteMeta(t.Name()) + "$"
	if hasNested {
		selection += "/" + nested
	}
	args := []string{"-test.run=" + selection, "-test.count=1", "-test.skip=" + flag.Lookup("test.skip").Value.String()}
	if testing.Short() {
		args = append(args, "-test.short")
	}
	if deadline, ok := t.Deadline(); ok {
		args = append(args, "-test.timeout="+time.Until(deadline).String())
	}
	// In particular, do not give the child the parent's -test.testlogfile:
	// testing opens it with O_TRUNC, and each process buffers its own log.
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(os.Environ(), "SKL_ISOLATION_PILOT_CHILD="+t.Name())
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("isolated scenario: %v\n%s", err, output)
	}
	return true
}

func TestIsolationPilotExternalFixture(t *testing.T) {
	if isolationPilot(t) {
		return
	}
	contents, err := os.ReadFile(os.Getenv("SKL_ISOLATION_PILOT_FIXTURE"))
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "expected\n" {
		t.Fatalf("fixture mismatch: got %q, want expected", contents)
	}
}
