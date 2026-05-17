package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyLoopStateFile_OK(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	_ = os.WriteFile(path, []byte(`{
  "loop": "l",
  "ticket": "PROJ-1",
  "max_rounds": 3,
  "rounds": [
    {"round":1, "timestamp":"2026-01-01T00:00:00Z", "status":"PASS"}
  ]
}`), 0o644)

	n, issues, err := VerifyLoopStateFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("want 1 round, got %d", n)
	}
	if len(issues) != 0 {
		t.Errorf("want no issues, got %+v", issues)
	}
}

func TestVerifyLoopStateFile_MissingFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	_ = os.WriteFile(path, []byte(`{"loop":"l"}`), 0o644)

	_, issues, _ := VerifyLoopStateFile(path)
	want := map[string]bool{
		`missing required field "ticket"`:     false,
		`missing required field "max_rounds"`: false,
		`missing required field "rounds"`:     false,
	}
	for _, iss := range issues {
		if iss.Severity != "FAIL" {
			t.Errorf("top-level missing fields should be FAIL, got %q", iss.Severity)
		}
		if _, ok := want[iss.Message]; ok {
			want[iss.Message] = true
		}
	}
	for msg, seen := range want {
		if !seen {
			t.Errorf("expected issue %q, not reported", msg)
		}
	}
}

func TestVerifyLoopStateFile_WarnsOnFailedNoProblems(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	_ = os.WriteFile(path, []byte(`{
  "loop": "l",
  "ticket": "PROJ-1",
  "max_rounds": 3,
  "rounds": [
    {"round":1, "timestamp":"2026-01-01T00:00:00Z", "status":"FAILED", "problems":[]},
    {"round":2, "timestamp":"2026-01-01T00:00:00Z", "status":"PASSED"}
  ]
}`), 0o644)

	_, issues, _ := VerifyLoopStateFile(path)
	foundWarn := false
	for _, iss := range issues {
		if iss.Severity == "WARN" {
			foundWarn = true
		}
	}
	if !foundWarn {
		t.Errorf("expected WARN for FAILED round with 0 problems")
	}
}

func TestVerifyLoopStateFile_InvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	_ = os.WriteFile(path, []byte(`not json`), 0o644)

	_, issues, err := VerifyLoopStateFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) == 0 || issues[0].Severity != "FAIL" {
		t.Errorf("expected FAIL on invalid JSON, got %+v", issues)
	}
}
