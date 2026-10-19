package ledger_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

// An experiment report resolves only when the Proposal's ledger directory
// records it; acceptance cannot record one yet, so this is checked at the
// declaration-loading seam.
func TestLoadDeclarationResolvesRecordedExperimentReports(t *testing.T) {
	intake := t.TempDir()
	write := func(path, contents string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(intake, "proposal.json"), `{"proposal": "layout", "slices": [{"name": "grid"}]}`)
	write(filepath.Join(intake, "proposal.md"), "# Layout\n")
	write(filepath.Join(intake, "grid", "intent.md"), "# Grid\n\n## Definition of Done\n\n- [ ] The grid holds (B1, A1).\n")
	write(filepath.Join(intake, "grid", "behavior.md"), "# Grid behavior\n\n## B1: The grid holds\n")
	write(filepath.Join(intake, "grid", "plan.md"), "# Grid plan\n\n## A1: Column layout\nSource: experiments/columns.md\n\n## Verification strategy\n\n| Covers | Check | Rank |\n| --- | --- | --- |\n| B1, A1 | Layout test | tests |\n")
	proposals := t.TempDir()
	sources := ledger.ContractSources{Repository: t.TempDir(), Proposals: proposals}

	_, err := ledger.LoadDeclaration(intake, nil, nil, sources)
	if err == nil || !strings.Contains(err.Error(), "experiments/columns.md") || !strings.Contains(err.Error(), "A1") {
		t.Fatalf("an unrecorded experiment report resolved: %v", err)
	}
	write(filepath.Join(proposals, "layout", "experiments", "columns.md"), "# Columns experiment\n")
	if _, err := ledger.LoadDeclaration(intake, nil, nil, sources); err != nil {
		t.Fatalf("a recorded experiment report did not resolve: %v", err)
	}
}
