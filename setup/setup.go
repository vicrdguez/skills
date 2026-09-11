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

Use ` + "`skl`" + ` as the Workflow entrypoint. Do not manually mutate Workflow Projections. Only a human merges.

## Simplicity

Choose the least complexity that satisfies the accepted scope, repository
standards, and required verification. Preserve relevant invariants, error
handling, security, and accessibility. When a simpler approach would change
the agreed behavior, raise that trade-off rather than silently reducing scope.

### Understand and Reuse

Trace the affected behavior and relevant callers before choosing a solution.
Fix causes at the boundary responsible for them, rather than patching symptoms
in individual callers.

Look for adequate existing code, standard-library features, native platform
capabilities, and installed dependencies before adding a mechanism. Check
that a candidate actually satisfies the required semantics and failure modes;
availability alone does not make it suitable.

### Justify Structure

Introduce abstractions, configuration, dependencies, and test infrastructure
for concrete current needs. Prefer the approach that leaves less for callers
and maintainers to understand, rather than the fewest lines or files.

Apply the deletion test while preserving behavior: if removing an abstraction
eliminates complexity, simplify it; if it spreads responsibilities into callers,
keep those responsibilities together. A boundary can earn its keep through
encapsulation or testability without multiple production implementations.

### Keep Tests Direct

Test observable behavior at the agreed seams, using the project's existing
test tools. Keep setup direct and expected results independent of the
implementation.

Ask what meaningful regression would lose protection if a test disappeared.
Keep distinct regression protection; simplify repeated setup and incidental
implementation coupling. Remove redundant or implementation-coupled tests
only when their behavioral and failure-mode protection remains covered at
the appropriate interface.

### Review Concrete Alternatives

Judge simplifications by the burden they remove, not lines saved.

For a proposed simplification, identify the simpler alternative, the burden
it removes, and why required behavior and verification remain intact.
A named code smell is not sufficient justification.

Keep cleanup within the change's scope; raise unrelated opportunities
separately.
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
