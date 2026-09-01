package migrate_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/liemle3893/go-tryve/internal/migrate"
	"github.com/liemle3893/go-tryve/internal/tryve"
)

// writeTest writes a test file and returns its path.
func writeTest(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// analyze runs a v1→v2 analysis over one file.
func analyze(t *testing.T, path string) *migrate.Report {
	t.Helper()
	report, err := migrate.Analyze([]string{path}, tryve.LegacyCompat(), tryve.ModernCompat())
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	return report
}

// rules returns the set of rules the report contains.
func rules(r *migrate.Report) map[string]int {
	out := map[string]int{}
	for _, f := range r.Findings {
		out[f.Rule]++
	}
	return out
}

// TestDetectsDroppedAssertions covers the forms that were silently discarded.
func TestDetectsDroppedAssertions(t *testing.T) {
	path := writeTest(t, "a.test.yaml", `
name: A
execute:
  - adapter: shell
    action: exec
    command: "true"
    assert:
      exitCode: 0
      stdout:
        contains: "x"
  - adapter: postgresql
    action: query
    sql: "SELECT 1"
    assert:
      - row: 0
        column: a
        equals: 1
  - adapter: http
    action: request
    url: "/x"
    assert:
      json:
        - path: "$.n"
          gte: 1
`)

	got := rules(analyze(t, path))
	for _, want := range []string{
		migrate.RuleExitCodeSuppress,
		migrate.RuleDroppedField,
		migrate.RuleDroppedRowColumn,
		migrate.RuleDroppedOperator,
	} {
		if got[want] == 0 {
			t.Errorf("expected a finding for %q; got %v", want, got)
		}
	}
}

// TestDoesNotFlagAlwaysWorkingForms checks the analysis is not noise: an
// assertion that was always evaluated is not reported.
func TestDoesNotFlagAlwaysWorkingForms(t *testing.T) {
	path := writeTest(t, "b.test.yaml", `
name: B
execute:
  - adapter: http
    action: request
    url: "/x"
    assert:
      status: 200
      headers:
        Content-Type: "application/json"
      json:
        - path: "$.data.id"
          equals: "abc"
        - path: "$.items"
          length: 2
`)

	report := analyze(t, path)
	for _, f := range report.Findings {
		if f.Area == tryve.CompatAssertions {
			t.Errorf("unexpected assertions finding: %s — %s", f.Rule, f.Detail)
		}
	}
}

// TestDetectsExecutionAndAdapterChanges covers the non-assertion areas.
func TestDetectsExecutionAndAdapterChanges(t *testing.T) {
	path := writeTest(t, "c.test.yaml", `
name: C
execute:
  - adapter: shell
    action: exec
    timeout: 500
    command: "true"
  - adapter: shell
    action: exec
    skip: true
    skipReason: "x"
    command: "true"
  - adapter: postgresql
    action: count
    sql: "SELECT COUNT(*) FROM users"
`)

	got := rules(analyze(t, path))
	for _, want := range []string{
		migrate.RuleStepTimeout,
		migrate.RuleStepSkip,
		migrate.RuleCountAggregate,
		migrate.RuleShellUnbounded,
	} {
		if got[want] == 0 {
			t.Errorf("expected a finding for %q; got %v", want, got)
		}
	}
}

// TestCountWithoutAggregateIsNotFlagged checks the count rule is precise: only
// an aggregating query changes meaning.
func TestCountWithoutAggregateIsNotFlagged(t *testing.T) {
	path := writeTest(t, "d.test.yaml", `
name: D
execute:
  - adapter: postgresql
    action: count
    sql: "SELECT * FROM users WHERE active = true"
`)

	if n := rules(analyze(t, path))[migrate.RuleCountAggregate]; n != 0 {
		t.Errorf("a plain SELECT counts rows in both levels; got %d finding(s)", n)
	}
}

// TestTypedValueSitesAreUncertain checks that interpolation findings are
// reported as uncertain, since they depend on the resolved type.
func TestTypedValueSitesAreUncertain(t *testing.T) {
	path := writeTest(t, "e.test.yaml", `
name: E
execute:
  - adapter: http
    action: request
    method: POST
    url: "/x"
    body:
      id: "{{captured.user_id}}"
      label: "user-{{captured.user_id}}"
`)

	report := analyze(t, path)
	var typed []migrate.Finding
	for _, f := range report.Findings {
		if f.Rule == migrate.RuleTypedValue {
			typed = append(typed, f)
		}
	}
	if len(typed) != 1 {
		t.Fatalf("expected exactly the lone expression to be flagged, got %d: %+v", len(typed), typed)
	}
	if typed[0].Certainty != migrate.MayChange {
		t.Errorf("a typed substitution depends on the resolved value; expected %q, got %q",
			migrate.MayChange, typed[0].Certainty)
	}
}

// TestAlreadyPinnedFileIsNotReported checks that a file at its own level is not
// re-reported, so `--status` and repeated runs converge.
func TestAlreadyPinnedFileIsNotReported(t *testing.T) {
	body := `
apiVersion: tryve/v2
name: F
execute:
  - adapter: shell
    action: exec
    command: "true"
    assert:
      exitCode: 0
`
	path := writeTest(t, "f.test.yaml", body)
	if n := len(analyze(t, path).Findings); n != 0 {
		t.Errorf("a file already at v2 has nothing to migrate; got %d finding(s)", n)
	}
}

// TestPinIsPurelyAdditive checks that pinning changes nothing but the added key:
// comments, ordering, and block scalars must survive, or the diff is unreviewable.
func TestPinIsPurelyAdditive(t *testing.T) {
	body := `# yaml-language-server: $schema=../schemas/e2e-test.schema.json
# A header comment.

name: G
description: |
  A block scalar
  across lines.
execute:
  - adapter: shell
    action: exec
    command: >-
      echo one &&
      echo two
`
	path := writeTest(t, "g.test.yaml", body)

	changed, err := migrate.Pin(path, tryve.APIVersionV1)
	if err != nil || !changed {
		t.Fatalf("Pin: changed=%v err=%v", changed, err)
	}

	got, _ := os.ReadFile(path)
	text := string(got)

	// The schema directive must still be the first line.
	if !strings.HasPrefix(text, "# yaml-language-server:") {
		t.Errorf("the schema directive must stay first, got:\n%s", text[:80])
	}
	if !strings.Contains(text, "apiVersion: tryve/v1") {
		t.Errorf("expected the pin to be written")
	}
	// Every original line survives.
	for _, line := range strings.Split(strings.TrimSpace(body), "\n") {
		if !strings.Contains(text, line) {
			t.Errorf("pinning dropped the line %q", line)
		}
	}

	// Pinning twice is a no-op.
	changed, err = migrate.Pin(path, tryve.APIVersionV1)
	if err != nil || changed {
		t.Errorf("pinning an already-pinned file should do nothing; changed=%v err=%v", changed, err)
	}
}

// TestUnpinRestoresTheOriginal checks the pin round-trips.
func TestUnpinRestoresTheOriginal(t *testing.T) {
	body := `# header

name: H
execute:
  - adapter: shell
    action: exec
    command: "true"
`
	path := writeTest(t, "h.test.yaml", body)

	if _, err := migrate.Pin(path, tryve.APIVersionV1); err != nil {
		t.Fatal(err)
	}
	if !migrate.IsPinned(path) {
		t.Fatal("expected the file to report as pinned")
	}
	if _, err := migrate.Unpin(path); err != nil {
		t.Fatal(err)
	}
	if migrate.IsPinned(path) {
		t.Error("expected the pin to be gone")
	}

	got, _ := os.ReadFile(path)
	if strings.TrimSpace(string(got)) != strings.TrimSpace(body) {
		t.Errorf("unpin should restore the original file.\n--- got ---\n%s\n--- want ---\n%s", got, body)
	}
}

// TestAreaScopedAnalysisIgnoresOtherAreas checks that migrating one area does
// not report differences belonging to another.
func TestAreaScopedAnalysisIgnoresOtherAreas(t *testing.T) {
	path := writeTest(t, "i.test.yaml", `
name: I
execute:
  - adapter: shell
    action: exec
    timeout: 500
    command: "true"
    assert:
      exitCode: 0
`)

	onlyAssertions := tryve.LegacyCompat().With(tryve.CompatAssertions, true)
	report, err := migrate.Analyze([]string{path}, tryve.LegacyCompat(), onlyAssertions)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range report.Findings {
		if f.Area != tryve.CompatAssertions {
			t.Errorf("migrating assertions should not report %s findings: %s", f.Area, f.Detail)
		}
	}
	if len(report.Findings) == 0 {
		t.Error("expected the exitCode assertion to be reported")
	}
}
