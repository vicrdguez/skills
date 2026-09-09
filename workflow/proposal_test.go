package workflow

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

type publicationBackend struct {
	Backend
	items []WorkItem
	ready []WorkItemID
}

func (b *publicationBackend) FindWorkItems(context.Context, []WorkItem, []Dependency) ([]WorkItem, error) {
	return b.items, nil
}

func (b *publicationBackend) CreateWorkItem(_ context.Context, item WorkItem) (WorkItem, error) {
	item.ID = "work:alpha/opaque"
	b.items = append(b.items, item)
	return item, nil
}

func (b *publicationBackend) SetReady(_ context.Context, id WorkItemID) error {
	for i := range b.items {
		if b.items[i].ID == id {
			b.items[i].Ready = true
			b.ready = append(b.ready, id)
			return nil
		}
	}
	return fmt.Errorf("unknown Work Item %q", id)
}

func TestPublishOpaqueIdentityWithExplicitRemote(t *testing.T) {
	root := newGitRepository(t)
	runGit(t, root, "remote", "add", "upstream", t.TempDir())
	runGit(t, root, "update-ref", "refs/remotes/upstream/main", "HEAD")
	runGit(t, root, "switch", "-c", "slice")
	commitLedger(t, root, "slice", true)
	baseline := gitOutput(t, root, "rev-parse", "HEAD")
	runGit(t, root, "update-ref", "refs/remotes/upstream/slice", "HEAD")
	body := filepath.Join(t.TempDir(), "issue.md")
	if err := os.WriteFile(body, []byte("agent-authored body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	request := PublishRequest{Root: root, Target: "main", Slices: []Slice{{Slug: "slice", BodyPath: body}}}
	if outcome, err := Publish(context.Background(), request, nil); err == nil || err.Error() != "publication requires an explicit Git remote" || outcome != (Outcome{}) {
		t.Fatalf("missing remote = %#v, %v", outcome, err)
	}
	request.Remote = "upstream"
	backend := &publicationBackend{}
	for range 2 {
		outcome, err := Publish(context.Background(), request, backend)
		if err != nil || outcome != (Outcome{Status: "completed"}) {
			t.Fatalf("Publish() = %#v, %v", outcome, err)
		}
	}
	if len(backend.items) != 1 {
		t.Fatalf("items = %#v", backend.items)
	}
	item := backend.items[0]
	if item.ID != "work:alpha/opaque" || item.Title != "slice" || item.Branch != "slice" || item.Body != "agent-authored body\n" || item.ArtifactBaseline != baseline || !item.Ready {
		t.Fatalf("item = %#v", item)
	}
	if len(backend.ready) != 1 || backend.ready[0] != "work:alpha/opaque" {
		t.Fatalf("ready identities = %#v", backend.ready)
	}
}

func TestArtifactBaselineHistory(t *testing.T) {
	tests := map[string]struct {
		arrange func(*testing.T, string)
		valid   bool
	}{
		"complete absent-to-present transition": {arrange: func(t *testing.T, root string) {
			commitLedger(t, root, "slice", true)
		}, valid: true},
		"inherited ledger": {arrange: func(t *testing.T, root string) {
			commitLedger(t, root, "slice", true)
			commitFile(t, root, "later", "later\n")
		}},
		"split introduction": {arrange: func(t *testing.T, root string) {
			commitLedger(t, root, "slice", false)
			commitFile(t, root, ".changes/slice/behavior.md", "behavior\n")
		}},
		"recreated ledger": {arrange: func(t *testing.T, root string) {
			commitLedger(t, root, "slice", true)
			if err := os.RemoveAll(filepath.Join(root, ".changes", "slice")); err != nil {
				t.Fatal(err)
			}
			runGit(t, root, "add", "-A")
			runGit(t, root, "commit", "-m", "remove ledger")
			commitLedger(t, root, "slice", true)
		}},
		"merge imports ledger": {arrange: func(t *testing.T, root string) {
			runGit(t, root, "switch", "-c", "side")
			commitLedger(t, root, "slice", true)
			runGit(t, root, "switch", "main")
			runGit(t, root, "merge", "--no-ff", "side", "-m", "merge ledger")
		}},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			root := newGitRepository(t)
			test.arrange(t, root)
			head := gitOutput(t, root, "rev-parse", "HEAD")

			baseline, err := artifactBaseline(root, "slice", head)
			if test.valid && (err != nil || baseline != head) {
				t.Fatalf("artifactBaseline() = %q, %v; want %s", baseline, err, head)
			}
			if !test.valid && err == nil {
				t.Fatalf("artifactBaseline() accepted %s", baseline)
			}
		})
	}
}

func newGitRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runGit(t, root, "init", "-b", "main")
	runGit(t, root, "config", "user.name", "Test")
	runGit(t, root, "config", "user.email", "test@example.com")
	commitFile(t, root, "README.md", "initial\n")
	return root
}

func commitLedger(t *testing.T, root, slug string, complete bool) {
	t.Helper()
	directory := filepath.Join(root, ".changes", slug)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "intent.md"), []byte("intent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if complete {
		if err := os.WriteFile(filepath.Join(directory, "behavior.md"), []byte("behavior\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, root, "add", ".changes/"+slug)
	runGit(t, root, "commit", "-m", "add ledger")
}

func commitFile(t *testing.T, root, name, contents string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", name)
	runGit(t, root, "commit", "-m", "add "+name)
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func gitOutput(t *testing.T, root string, args ...string) string {
	t.Helper()
	output, err := exec.Command("git", append([]string{"-C", root}, args...)...).Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(output[:len(output)-1])
}
