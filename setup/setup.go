package setup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const AgentsBlock = `<!-- dev-pipeline:start -->
## Workflow

Use ` + "`skl`" + ` for Workflow operations. Only a human merges.

## Simplicity

Choose the least complexity that satisfies the accepted scope, repository standards and required verification. Preserve invariants, error handling, security and accessibility. When a simpler approach would change the agreed behavior, raise the trade-off instead of reducing scope.

- **Understand and reuse.** Trace the affected behavior and its callers first, and fix causes at the boundary responsible for them. Before adding a mechanism, look for existing code, standard-library and platform features, and installed dependencies, and check that a candidate meets the required semantics and failure modes.
- **Justify structure.** Add abstractions, configuration, dependencies and test infrastructure for concrete current needs. Prefer the approach that leaves less for callers and maintainers to understand over the one with the fewest lines. Apply the deletion test: if removing an abstraction removes complexity, simplify it; if it spreads responsibilities into callers, keep it. A boundary can earn its keep through encapsulation or testability, even with one production implementation.
- **Keep tests direct.** Test observable behavior at the agreed seams with the project's existing test tools, direct setup and expected results independent of the implementation. Simplify repeated setup and incidental coupling to the implementation. Remove a test only when its behavioral and failure-mode protection stays covered at the appropriate interface.
- **Review concrete alternatives.** Judge a simplification by the burden it removes: name the simpler alternative, the burden it removes, and why behavior and verification stay intact. A code-smell name alone does not justify it. Keep cleanup within the change's scope, and raise unrelated opportunities separately.
<!-- dev-pipeline:end -->
`

type Backend interface {
	Validate(context.Context) (targetBranch string, err error)
	Prepare(context.Context) error
}

type Request struct {
	Location string
	Confirm  func(string) (bool, error)
}

type Outcome struct {
	Root         string
	TargetBranch string
}

func Run(ctx context.Context, request Request, backend Backend) (Outcome, error) {
	root, err := git(request.Location, "rev-parse", "--show-toplevel")
	if err != nil {
		return Outcome{}, errors.New("not a Git repository")
	}
	agents, err := planAgents(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		return Outcome{}, err
	}
	claudePath := filepath.Join(root, "CLAUDE.md")
	offerClaude, err := shouldOfferClaudeLink(claudePath)
	if err != nil {
		return Outcome{}, err
	}
	gitignore, err := planGitignore(filepath.Join(root, ".gitignore"))
	if err != nil {
		return Outcome{}, err
	}
	legacyPath := filepath.Join(root, "docs", "github.md")
	legacyExists, err := removableFileExists(legacyPath)
	if err != nil {
		return Outcome{}, err
	}
	targetBranch, err := backend.Validate(ctx)
	if err != nil {
		return Outcome{}, fmt.Errorf("validate repository: %w", err)
	}
	linkClaude := false
	if offerClaude && request.Confirm != nil {
		linkClaude, err = request.Confirm("Link CLAUDE.md to AGENTS.md? [y/N] ")
		if err != nil {
			return Outcome{}, err
		}
	}

	if err := backend.Prepare(ctx); err != nil {
		return Outcome{}, fmt.Errorf("prepare workflow backend: %w", err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), agents, 0o644); err != nil {
		return Outcome{}, err
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), gitignore, 0o644); err != nil {
		return Outcome{}, err
	}
	if legacyExists {
		if err := os.Remove(legacyPath); err != nil {
			return Outcome{}, err
		}
	}
	if linkClaude {
		if err := os.Remove(claudePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return Outcome{}, err
		}
		if err := os.Symlink("AGENTS.md", claudePath); err != nil {
			return Outcome{}, err
		}
	}
	return Outcome{Root: root, TargetBranch: targetBranch}, nil
}

func planGitignore(path string) ([]byte, error) {
	contents, err := readOwnedFile(path)
	if err != nil {
		return nil, err
	}
	var kept strings.Builder
	for _, line := range strings.SplitAfter(string(contents), "\n") {
		if strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r") != ".worktrees/" {
			kept.WriteString(line)
		}
	}
	if kept.Len() > 0 && !strings.HasSuffix(kept.String(), "\n") {
		kept.WriteByte('\n')
	}
	kept.WriteString(".worktrees/\n")
	return []byte(kept.String()), nil
}

func removableFileExists(path string) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.IsDir() {
		return false, fmt.Errorf("legacy setup path %s is a directory", path)
	}
	return true, nil
}

func shouldOfferClaudeLink(path string) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		return target != "AGENTS.md", err
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	return string(contents) == "@AGENTS.md" || string(contents) == "@AGENTS.md\n", nil
}

func planAgents(path string) ([]byte, error) {
	contents, err := readOwnedFile(path)
	if err != nil {
		return nil, err
	}
	if contents == nil {
		return []byte(AgentsBlock), nil
	}
	const start = "<!-- dev-pipeline:start -->"
	const end = "<!-- dev-pipeline:end -->"
	text := string(contents)
	starts, ends := strings.Count(text, start), strings.Count(text, end)
	if starts != ends || starts > 1 {
		return nil, errors.New("AGENTS.md has malformed workflow markers")
	}
	startAt := strings.Index(text, start)
	endAt := strings.Index(text, end)
	if starts == 1 && startAt > endAt {
		return nil, errors.New("AGENTS.md has malformed workflow markers")
	}
	if startAt >= 0 && endAt >= 0 {
		endAt += len(end)
		return []byte(string(contents[:startAt]) + strings.TrimSuffix(AgentsBlock, "\n") + string(contents[endAt:])), nil
	}
	prefix := string(contents)
	if prefix != "" && !strings.HasSuffix(prefix, "\n") {
		prefix += "\n"
	}
	return []byte(prefix + AgentsBlock), nil
}

func readOwnedFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("owned file %s must not be a symlink", path)
	}
	return os.ReadFile(path)
}

func git(directory string, args ...string) (string, error) {
	command := exec.Command("git", append([]string{"-C", directory}, args...)...)
	output, err := command.Output()
	return strings.TrimSpace(string(output)), err
}
