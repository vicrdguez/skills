package ledger_test

import (
	"bytes"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

const (
	commitHead      = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	commitTarget    = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	commitReviewed  = "cccccccccccccccccccccccccccccccccccccccc"
	commitClaim     = "dddddddddddddddddddddddddddddddddddddddd"
	commitContract  = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	commitImplement = "a111111111111111111111111111111111111111"
	commitWatchdog  = "a222222222222222222222222222222222222222"
	commitDecision  = "a333333333333333333333333333333333333333"
)

const (
	claimPath     = "projects/widget/proposals/add-order/widget/state.json"
	contractPath  = "projects/widget/proposals/add-order/widget/behavior.md"
	implementPath = "projects/widget/proposals/add-order/widget/implement-report.md"
	watchdogPath  = "projects/widget/proposals/add-order/widget/watchdog-report.md"
	decisionPath  = "projects/widget/proposals/add-order/widget/decision.md"
)

func claimReference() ledger.Reference {
	return ledger.Reference{Commit: commitClaim, Path: claimPath}
}

func contractReferences() []ledger.Reference {
	return []ledger.Reference{{Commit: commitContract, Path: contractPath}}
}

func validImplementReport() ledger.Report {
	return ledger.Report{
		Schema:  ledger.ReportSchema,
		Outcome: "awaiting_review",
		Source:  ledger.SourceRevisions{Head: commitHead, Target: commitTarget},
		Ledger:  ledger.ReportInputs{Claim: claimReference(), Contract: contractReferences()},
	}
}

func validWatchdogReport() ledger.Report {
	return ledger.Report{
		Schema:  ledger.ReportSchema,
		Outcome: "pass",
		Source:  ledger.SourceRevisions{Head: commitHead, Target: commitTarget, Reviewed: commitReviewed},
		Ledger: ledger.ReportInputs{
			Claim:     claimReference(),
			Contract:  contractReferences(),
			Implement: &ledger.Reference{Commit: commitImplement, Path: implementPath},
		},
		Round: 2,
	}
}

func TestReportFormatRoundTripsImplementReports(t *testing.T) {
	decision := ledger.Reference{Commit: commitDecision, Path: decisionPath}
	for _, testCase := range []struct {
		name   string
		report ledger.Report
		body   string
	}{
		{
			name: "awaiting review with head target and decision",
			report: ledger.Report{
				Schema:  ledger.ReportSchema,
				Outcome: "awaiting_review",
				Source:  ledger.SourceRevisions{Head: commitHead, Target: commitTarget},
				Ledger:  ledger.ReportInputs{Claim: claimReference(), Contract: contractReferences(), Decision: &decision},
			},
			body: "# Implementation\n\nEvidence.\n",
		},
		{
			name: "needs human before a workspace exists",
			report: ledger.Report{
				Schema:  ledger.ReportSchema,
				Outcome: "needs_human",
				Ledger:  ledger.ReportInputs{Claim: claimReference(), Contract: contractReferences()},
			},
			body: "Blocked before branch creation.",
		},
		{
			name: "needs human with an existing workspace",
			report: ledger.Report{
				Schema:  ledger.ReportSchema,
				Outcome: "needs_human",
				Source:  ledger.SourceRevisions{Head: commitHead, Target: commitTarget},
				Ledger:  ledger.ReportInputs{Claim: claimReference(), Contract: contractReferences()},
			},
			body: "Blocked on a decision.\n",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			data, err := ledger.FormatReport(ledger.ImplementPhase, testCase.report, testCase.body)
			if err != nil {
				t.Fatalf("FormatReport: %v", err)
			}
			if !strings.HasPrefix(string(data), "---\n") {
				t.Fatalf("persisted report does not open with frontmatter: %q", data)
			}
			for _, marker := range []string{"schema: 1\n", "outcome: " + testCase.report.Outcome + "\n", "ledger:\n"} {
				if !strings.Contains(string(data), marker) {
					t.Errorf("persisted frontmatter lacks %q: %q", marker, data)
				}
			}
			if testCase.report.Source.Head != "" && !strings.Contains(string(data), "source:\n") {
				t.Errorf("persisted frontmatter lacks source revisions: %q", data)
			}
			parsed, body, err := ledger.ParseReport(ledger.ImplementPhase, data)
			if err != nil {
				t.Fatalf("ParseReport: %v", err)
			}
			if !reflect.DeepEqual(parsed, testCase.report) {
				t.Errorf("roundtrip report = %#v, want %#v", parsed, testCase.report)
			}
			if body != testCase.body {
				t.Errorf("roundtrip body = %q, want %q", body, testCase.body)
			}
		})
	}
}

func TestReportFormatRoundTripsWatchdogReports(t *testing.T) {
	implement := &ledger.Reference{Commit: commitImplement, Path: implementPath}
	decision := &ledger.Reference{Commit: commitDecision, Path: decisionPath}
	for _, testCase := range []struct {
		name   string
		report ledger.Report
		body   string
	}{
		{
			name: "pass with a reviewed revision and post-marker head",
			report: ledger.Report{
				Schema:  ledger.ReportSchema,
				Outcome: "pass",
				Source:  ledger.SourceRevisions{Head: commitHead, Target: commitTarget, Reviewed: commitReviewed},
				Ledger:  ledger.ReportInputs{Claim: claimReference(), Contract: contractReferences(), Implement: implement},
				Round:   3,
			},
			body: "Watchdog findings: none.\n",
		},
		{
			name: "rework with a prior review reference and decision",
			report: ledger.Report{
				Schema:  ledger.ReportSchema,
				Outcome: "rework",
				Source:  ledger.SourceRevisions{Head: commitReviewed, Target: commitTarget, Reviewed: commitReviewed},
				Ledger: ledger.ReportInputs{
					Claim:     claimReference(),
					Contract:  contractReferences(),
					Implement: implement,
					Watchdog:  &ledger.Reference{Commit: commitWatchdog, Path: watchdogPath},
					Decision:  decision,
				},
				Round: 1,
			},
			body: "W1 open.\n",
		},
		{
			name: "needs human at the completed round",
			report: ledger.Report{
				Schema:  ledger.ReportSchema,
				Outcome: "needs_human",
				Source:  ledger.SourceRevisions{Head: commitReviewed, Target: commitTarget, Reviewed: commitReviewed},
				Ledger:  ledger.ReportInputs{Claim: claimReference(), Contract: contractReferences(), Implement: implement},
				Round:   2,
			},
			body: "Human direction required.\n",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			data, err := ledger.FormatReport(ledger.WatchdogPhase, testCase.report, testCase.body)
			if err != nil {
				t.Fatalf("FormatReport: %v", err)
			}
			parsed, body, err := ledger.ParseReport(ledger.WatchdogPhase, data)
			if err != nil {
				t.Fatalf("ParseReport: %v", err)
			}
			if !reflect.DeepEqual(parsed, testCase.report) {
				t.Errorf("roundtrip report = %#v, want %#v", parsed, testCase.report)
			}
			if body != testCase.body {
				t.Errorf("roundtrip body = %q, want %q", body, testCase.body)
			}
		})
	}
}

// TestReportParsesIndependentSchemaOneDocuments grounds the persisted format
// in hand-authored documents rather than in FormatReport's own output.
func TestReportParsesIndependentSchemaOneDocuments(t *testing.T) {
	implementDocument := "---\n" +
		"schema: 1\n" +
		"outcome: awaiting_review\n" +
		"source:\n" +
		"  head: " + commitHead + "\n" +
		"  target: " + commitTarget + "\n" +
		"ledger:\n" +
		"  claim:\n" +
		"    commit: " + commitClaim + "\n" +
		"    path: " + claimPath + "\n" +
		"  contract:\n" +
		"    - commit: " + commitContract + "\n" +
		"      path: " + contractPath + "\n" +
		"  decision:\n" +
		"    commit: " + commitDecision + "\n" +
		"    path: " + decisionPath + "\n" +
		"---\n" +
		"# Implementation\n\nIndependent evidence.\n"

	wantImplement := ledger.Report{
		Schema:  ledger.ReportSchema,
		Outcome: "awaiting_review",
		Source:  ledger.SourceRevisions{Head: commitHead, Target: commitTarget},
		Ledger: ledger.ReportInputs{
			Claim:    claimReference(),
			Contract: contractReferences(),
			Decision: &ledger.Reference{Commit: commitDecision, Path: decisionPath},
		},
	}
	parsed, body, err := ledger.ParseReport(ledger.ImplementPhase, []byte(implementDocument))
	if err != nil {
		t.Fatalf("ParseReport implement document: %v", err)
	}
	if !reflect.DeepEqual(parsed, wantImplement) {
		t.Errorf("implement document = %#v, want %#v", parsed, wantImplement)
	}
	if body != "# Implementation\n\nIndependent evidence.\n" {
		t.Errorf("implement body = %q", body)
	}

	watchdogDocument := "---\n" +
		"schema: 1\n" +
		"outcome: rework\n" +
		"round: 1\n" +
		"source:\n" +
		"  head: " + commitReviewed + "\n" +
		"  target: " + commitTarget + "\n" +
		"  reviewed: " + commitReviewed + "\n" +
		"ledger:\n" +
		"  claim:\n" +
		"    commit: " + commitClaim + "\n" +
		"    path: " + claimPath + "\n" +
		"  contract:\n" +
		"    - commit: " + commitContract + "\n" +
		"      path: " + contractPath + "\n" +
		"  implement:\n" +
		"    commit: " + commitImplement + "\n" +
		"    path: " + implementPath + "\n" +
		"---\n" +
		"W1 unresolved.\n"

	wantWatchdog := ledger.Report{
		Schema:  ledger.ReportSchema,
		Outcome: "rework",
		Source:  ledger.SourceRevisions{Head: commitReviewed, Target: commitTarget, Reviewed: commitReviewed},
		Ledger: ledger.ReportInputs{
			Claim:     claimReference(),
			Contract:  contractReferences(),
			Implement: &ledger.Reference{Commit: commitImplement, Path: implementPath},
		},
		Round: 1,
	}
	parsed, body, err = ledger.ParseReport(ledger.WatchdogPhase, []byte(watchdogDocument))
	if err != nil {
		t.Fatalf("ParseReport watchdog document: %v", err)
	}
	if !reflect.DeepEqual(parsed, wantWatchdog) {
		t.Errorf("watchdog document = %#v, want %#v", parsed, wantWatchdog)
	}
	if body != "W1 unresolved.\n" {
		t.Errorf("watchdog body = %q", body)
	}
}

func TestReportPreservesOpaqueBodyByteForByte(t *testing.T) {
	report := validImplementReport()
	for _, testCase := range []struct {
		name string
		body string
	}{
		{name: "delimiter and metadata looking lines", body: "Leading\n---\nschema: 99\noutcome: rework\nround: 7\n---\nTrailing"},
		{name: "template looking instructions", body: "{{if .Implementation}}\nDo not render this body.\n{{end}}"},
		{name: "no final newline", body: "body without trailing newline"},
		{name: "empty body", body: ""},
		{name: "leading delimiter", body: "---\nround: 7\n"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			data, err := ledger.FormatReport(ledger.ImplementPhase, report, testCase.body)
			if err != nil {
				t.Fatalf("FormatReport: %v", err)
			}
			if !bytes.HasSuffix(data, []byte(testCase.body)) {
				t.Fatalf("persisted report changed the body suffix: %q", data)
			}
			parsed, body, err := ledger.ParseReport(ledger.ImplementPhase, data)
			if err != nil {
				t.Fatalf("ParseReport: %v", err)
			}
			if body != testCase.body {
				t.Errorf("body = %q, want %q", body, testCase.body)
			}
			if !reflect.DeepEqual(parsed, report) {
				t.Errorf("report = %#v, want %#v", parsed, report)
			}
		})
	}
}

func TestReportRefusesIncompatibleMetadata(t *testing.T) {
	implementFrontmatter := func() string {
		return "schema: 1\n" +
			"outcome: awaiting_review\n" +
			"source:\n" +
			"  head: " + commitHead + "\n" +
			"  target: " + commitTarget + "\n" +
			"ledger:\n" +
			"  claim:\n" +
			"    commit: " + commitClaim + "\n" +
			"    path: " + claimPath + "\n" +
			"  contract:\n" +
			"    - commit: " + commitContract + "\n" +
			"      path: " + contractPath + "\n"
	}
	watchdogFrontmatter := func() string {
		return "schema: 1\n" +
			"outcome: pass\n" +
			"source:\n" +
			"  head: " + commitHead + "\n" +
			"  target: " + commitTarget + "\n" +
			"  reviewed: " + commitReviewed + "\n" +
			"ledger:\n" +
			"  claim:\n" +
			"    commit: " + commitClaim + "\n" +
			"    path: " + claimPath + "\n" +
			"  contract:\n" +
			"    - commit: " + commitContract + "\n" +
			"      path: " + contractPath + "\n" +
			"  implement:\n" +
			"    commit: " + commitImplement + "\n" +
			"    path: " + implementPath + "\n" +
			"round: 2\n"
	}
	document := func(frontmatter string) []byte {
		return []byte("---\n" + frontmatter + "---\nbody\n")
	}
	implement := ledger.ImplementPhase
	watchdog := ledger.WatchdogPhase

	for _, testCase := range []struct {
		name  string
		phase string
		data  []byte
		want  string
	}{
		{
			name:  "unknown phase",
			phase: "review",
			data:  document(implementFrontmatter()),
			want:  "no schema-1 report format",
		},
		{
			name:  "missing opening delimiter",
			phase: implement,
			data:  []byte("schema: 1\n---\nbody\n"),
			want:  "does not begin with",
		},
		{
			name:  "missing closing delimiter",
			phase: implement,
			data:  []byte("---\nschema: 1\n"),
			want:  "no closing --- delimiter",
		},
		{
			name:  "empty frontmatter",
			phase: implement,
			data:  []byte("---\n---\nbody\n"),
			want:  "no YAML frontmatter metadata",
		},
		{
			name:  "unknown schema",
			phase: implement,
			data:  document(strings.Replace(implementFrontmatter(), "schema: 1", "schema: 2", 1)),
			want:  "not the supported schema 1",
		},
		{
			name:  "absent schema",
			phase: implement,
			data:  document(strings.Replace(implementFrontmatter(), "schema: 1\n", "", 1)),
			want:  "not the supported schema 1",
		},
		{
			name:  "fractional schema",
			phase: implement,
			data:  document(strings.Replace(implementFrontmatter(), "schema: 1", "schema: 1.5", 1)),
			want:  "schema is not a YAML integer",
		},
		{
			name:  "quoted schema",
			phase: implement,
			data:  document(strings.Replace(implementFrontmatter(), "schema: 1", `schema: "1"`, 1)),
			want:  "malformed",
		},
		{
			name:  "extra yaml document",
			phase: implement,
			// A separator with trailing whitespace is a YAML document start,
			// not the exact closing delimiter, so the second document stays in
			// the frontmatter and must be refused.
			data: document(implementFrontmatter() + "--- \nschema: 2\n"),
			want: "more than one YAML document",
		},
		{
			name:  "numeric path is not text",
			phase: implement,
			data:  document(strings.Replace(implementFrontmatter(), claimPath, "123", 1)),
			want:  "not a YAML string",
		},
		{
			name:  "unknown field",
			phase: implement,
			data:  document(implementFrontmatter() + "tick: done\n"),
			want:  "malformed",
		},
		{
			name:  "source as sequence",
			phase: implement,
			data: document(strings.Replace(
				implementFrontmatter(),
				"source:\n  head: "+commitHead+"\n  target: "+commitTarget+"\n",
				"source: [head, target]\n", 1)),
			want: "malformed",
		},
		{
			name:  "absent claim",
			phase: implement,
			data: document(strings.Replace(
				implementFrontmatter(),
				"  claim:\n    commit: "+commitClaim+"\n    path: "+claimPath+"\n", "", 1)),
			want: "claim reference commit",
		},
		{
			name:  "uppercase claim commit",
			phase: implement,
			data:  document(strings.Replace(implementFrontmatter(), commitClaim, strings.ToUpper(commitClaim), 1)),
			want:  "claim reference commit",
		},
		{
			name:  "short claim commit",
			phase: implement,
			data:  document(strings.Replace(implementFrontmatter(), commitClaim, commitClaim[:12], 1)),
			want:  "claim reference commit",
		},
		{
			name:  "empty contract",
			phase: implement,
			data: document(strings.Replace(
				implementFrontmatter(),
				"  contract:\n    - commit: "+commitContract+"\n      path: "+contractPath+"\n",
				"  contract: []\n", 1)),
			want: "no consumed contract references",
		},
		{
			name:  "traversal contract path",
			phase: implement,
			data:  document(strings.Replace(implementFrontmatter(), contractPath, "projects/../../etc/passwd", 1)),
			want:  "not a safe relative ledger path",
		},
		{
			name:  "absolute contract path",
			phase: implement,
			data:  document(strings.Replace(implementFrontmatter(), contractPath, "/etc/passwd", 1)),
			want:  "not a safe relative ledger path",
		},
		{
			name:  "invalid implement outcome",
			phase: implement,
			data:  document(strings.Replace(implementFrontmatter(), "outcome: awaiting_review", "outcome: pass", 1)),
			want:  "is not awaiting_review or needs_human",
		},
		{
			name:  "implement with a review round",
			phase: implement,
			data:  document(implementFrontmatter() + "round: 1\n"),
			want:  "records review round 1",
		},
		{
			name:  "implement with a reviewed revision",
			phase: implement,
			data: document(strings.Replace(
				implementFrontmatter(),
				"  target: "+commitTarget+"\n",
				"  target: "+commitTarget+"\n  reviewed: "+commitReviewed+"\n", 1)),
			want: "records reviewed revision",
		},
		{
			name:  "awaiting review without target",
			phase: implement,
			data:  document(strings.Replace(implementFrontmatter(), "  target: "+commitTarget+"\n", "", 1)),
			want:  "requires source head and target",
		},
		{
			name:  "needs human head without target",
			phase: implement,
			data: document(strings.Replace(
				strings.Replace(implementFrontmatter(), "outcome: awaiting_review", "outcome: needs_human", 1),
				"  target: "+commitTarget+"\n", "", 1)),
			want: "must record source head and target together",
		},
		{
			name:  "invalid watchdog outcome",
			phase: watchdog,
			data:  document(strings.Replace(watchdogFrontmatter(), "outcome: pass", "outcome: approved", 1)),
			want:  "is not pass, rework, or needs_human",
		},
		{
			name:  "watchdog round zero",
			phase: watchdog,
			data:  document(strings.Replace(watchdogFrontmatter(), "round: 2\n", "", 1)),
			want:  "completed review round 0",
		},
		{
			name:  "quoted watchdog round",
			phase: watchdog,
			data:  document(strings.Replace(watchdogFrontmatter(), "round: 2", `round: "2"`, 1)),
			want:  "malformed",
		},
		{
			name:  "fractional watchdog round",
			phase: watchdog,
			data:  document(strings.Replace(watchdogFrontmatter(), "round: 2", "round: 2.5", 1)),
			want:  "round is not a YAML integer",
		},
		{
			name:  "watchdog without implement reference",
			phase: watchdog,
			data: document(strings.Replace(
				watchdogFrontmatter(),
				"  implement:\n    commit: "+commitImplement+"\n    path: "+implementPath+"\n", "", 1)),
			want: "no reviewed implementation report",
		},
		{
			name:  "watchdog without reviewed revision",
			phase: watchdog,
			data:  document(strings.Replace(watchdogFrontmatter(), "  reviewed: "+commitReviewed+"\n", "", 1)),
			want:  "requires source head, target, and reviewed",
		},
		{
			name:  "rework head differs from reviewed",
			phase: watchdog,
			data:  document(strings.Replace(watchdogFrontmatter(), "outcome: pass", "outcome: rework", 1)),
			want:  "differs from reviewed",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, _, err := ledger.ParseReport(testCase.phase, testCase.data)
			if err == nil {
				t.Fatalf("ParseReport accepted incompatible metadata")
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("refusal %q does not mention %q", err.Error(), testCase.want)
			}
			var refusal *ledger.Refusal
			if !errors.As(err, &refusal) {
				t.Errorf("refusal %v is not a ledger.Refusal", err)
			}
		})
	}
}

func TestReportRoundTripsLargestCompletedCount(t *testing.T) {
	report := validWatchdogReport()
	report.Round = math.MaxUint64
	encoded, err := ledger.FormatReport(ledger.WatchdogPhase, report, "evidence")
	if err != nil {
		t.Fatal(err)
	}
	decoded, _, err := ledger.ParseReport(ledger.WatchdogPhase, encoded)
	if err != nil || decoded.Round != math.MaxUint64 {
		t.Fatalf("round lost: %#v, %v", decoded, err)
	}
}

func TestReportFormatRefusesIncompatibleMetadata(t *testing.T) {
	absoluteClaim := claimReference()
	absoluteClaim.Path = "/etc/passwd"
	for _, testCase := range []struct {
		name   string
		phase  string
		report ledger.Report
		want   string
	}{
		{
			name:   "unknown phase",
			phase:  "review",
			report: validImplementReport(),
			want:   "no schema-1 report format",
		},
		{
			name:  "unset schema",
			phase: ledger.ImplementPhase,
			report: func() ledger.Report {
				report := validImplementReport()
				report.Schema = 0
				return report
			}(),
			want: "not the supported schema 1",
		},
		{
			name:  "future schema",
			phase: ledger.ImplementPhase,
			report: func() ledger.Report {
				report := validImplementReport()
				report.Schema = 2
				return report
			}(),
			want: "not the supported schema 1",
		},
		{
			name:  "invalid implement outcome",
			phase: ledger.ImplementPhase,
			report: func() ledger.Report {
				report := validImplementReport()
				report.Outcome = "approved"
				return report
			}(),
			want: "is not awaiting_review or needs_human",
		},
		{
			name:  "implement with a review round",
			phase: ledger.ImplementPhase,
			report: func() ledger.Report {
				report := validImplementReport()
				report.Round = 1
				return report
			}(),
			want: "records review round 1",
		},
		{
			name:  "watchdog round zero",
			phase: ledger.WatchdogPhase,
			report: func() ledger.Report {
				report := validWatchdogReport()
				report.Round = 0
				return report
			}(),
			want: "completed review round 0",
		},
		{
			name:  "watchdog without implement reference",
			phase: ledger.WatchdogPhase,
			report: func() ledger.Report {
				report := validWatchdogReport()
				report.Ledger.Implement = nil
				return report
			}(),
			want: "no reviewed implementation report",
		},
		{
			name:  "absolute claim path",
			phase: ledger.ImplementPhase,
			report: func() ledger.Report {
				report := validImplementReport()
				report.Ledger.Claim = absoluteClaim
				return report
			}(),
			want: "not a safe relative ledger path",
		},
		{
			name:  "empty contract",
			phase: ledger.ImplementPhase,
			report: func() ledger.Report {
				report := validImplementReport()
				report.Ledger.Contract = nil
				return report
			}(),
			want: "no consumed contract references",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			data, err := ledger.FormatReport(testCase.phase, testCase.report, "body\n")
			if err == nil {
				t.Fatalf("FormatReport produced %q for incompatible metadata", data)
			}
			if data != nil {
				t.Errorf("FormatReport returned %q with its refusal", data)
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("refusal %q does not mention %q", err.Error(), testCase.want)
			}
		})
	}
}
