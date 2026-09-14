package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestCommandSignals(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		wait bool
	}{
		{"setup prompt", []string{"setup"}, false},
		{"implement immediate", []string{"implement", "next"}, false},
		{"watchdog immediate", []string{"watchdog", "next"}, false},
		{"implement waiting", []string{"implement", "next", "--wait"}, true},
		{"watchdog waiting", []string{"watchdog", "next", "--wait"}, true},
	} {
		for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
			t.Run(tc.name+"/"+sig.String(), func(t *testing.T) {
				root := proposalRepository(t)
				args := append([]string{"-test.run=^TestCommandSignalProcess$", "--"}, tc.args...)
				args = append(args, "--repo", root)
				cmd := exec.Command(os.Args[0], args...)
				cmd.Env = append(os.Environ(), "SKL_SIGNAL_PROCESS=1", "GH_TOKEN=fixture")
				stdin, err := cmd.StdinPipe()
				if err != nil {
					t.Fatal(err)
				}
				defer stdin.Close() // Keep setup blocked waiting for input until signalled.
				stdout, err := cmd.StdoutPipe()
				if err != nil {
					t.Fatal(err)
				}
				var stderr bytes.Buffer
				cmd.Stderr = &stderr
				if err := cmd.Start(); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
				marker := "observation\n"
				if tc.args[0] == "setup" {
					marker = "Link CLAUDE.md to AGENTS.md? [y/N] "
				}
				ready := make(chan error, 1)
				go func() {
					got := make([]byte, len(marker))
					_, err := io.ReadFull(stdout, got)
					if err == nil && string(got) != marker {
						err = fmt.Errorf("output = %q, want %q", got, marker)
					}
					ready <- err
				}()
				select {
				case err := <-ready:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(10 * time.Second):
					t.Fatal("command never reached signal test point")
				}
				if err := cmd.Process.Signal(sig); err != nil {
					t.Fatal(err)
				}
				done := make(chan error, 1)
				go func() { done <- cmd.Wait() }()
				select {
				case err := <-done:
					var exit *exec.ExitError
					if !errors.As(err, &exit) {
						t.Fatalf("signal did not fail command: %v", err)
					}
					if tc.wait {
						if exit.ExitCode() != 1 || !strings.Contains(stderr.String(), "queue waiting interrupted") || !strings.Contains(stderr.String(), "explicitly resume") {
							t.Fatalf("waiting lost cancellation diagnostics: %v: %s", err, &stderr)
						}
					} else if status := exit.Sys().(syscall.WaitStatus); !status.Signaled() || status.Signal() != sig {
						t.Fatalf("default signal behavior changed: %v: %s", err, &stderr)
					}
				case <-time.After(3 * time.Second):
					_ = cmd.Process.Kill()
					<-done
					t.Fatalf("command ignored %s", sig)
				}
			})
		}
	}
}

func TestCommandSignalProcess(t *testing.T) {
	if os.Getenv("SKL_SIGNAL_PROCESS") != "1" {
		return
	}
	// Exercise the real entrypoint, but never allow a request to reach a forge.
	http.DefaultClient.Transport = httpRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodGet && r.URL.Path == "/repos/acme/widgets" {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"default_branch":"main"}`)), Header: make(http.Header)}, nil
		}
		if r.Method == http.MethodGet && r.URL.Path == "/repos/acme/widgets/issues" {
			fmt.Fprintln(os.Stdout, "observation")
			<-r.Context().Done()
			return nil, r.Context().Err()
		}
		return nil, fmt.Errorf("unexpected fixture request: %s %s", r.Method, r.URL)
	})
	os.Args = append([]string{"skl"}, os.Args[3:]...)
	main()
}
