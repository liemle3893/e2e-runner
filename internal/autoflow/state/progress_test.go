package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateTicketKey(t *testing.T) {
	cases := []struct {
		key   string
		valid bool
	}{
		{"PROJ-1", true},
		{"PROJ-123", true},
		{"WINX-42", true},
		{"A1B-1", true},
		// Bash regex is [A-Z][A-Z0-9]+, which requires >=2 prefix chars.
		{"A-1", false},
		{"", false},
		{"proj-1", false},
		{"PROJ-", false},
		{"-1", false},
		{"PROJ-abc", false},
		{"PROJ/42", false},
		{"../PROJ-1", false},
	}
	for _, c := range cases {
		err := ValidateTicketKey(c.key)
		if c.valid && err != nil {
			t.Errorf("ValidateTicketKey(%q) unexpected error: %v", c.key, err)
		}
		if !c.valid && err == nil {
			t.Errorf("ValidateTicketKey(%q) accepted invalid key", c.key)
		}
	}
}

func TestInitProgress_FreshAndForce(t *testing.T) {
	root := t.TempDir()
	p, err := InitProgress(root, "PROJ-1", "/w", "jira-iss/proj-1", false)
	if err != nil {
		t.Fatalf("first init: %v", err)
	}
	if p.CurrentStep != 1 {
		t.Errorf("want CurrentStep=1, got %d", p.CurrentStep)
	}
	if p.Ticket != "PROJ-1" {
		t.Errorf("want ticket=PROJ-1, got %q", p.Ticket)
	}
	if p.PRURL != nil {
		t.Errorf("PRURL should be nil at init, got %v", *p.PRURL)
	}

	// Second init without force should fail.
	if _, err := InitProgress(root, "PROJ-1", "/w", "b", false); !errors.Is(err, ErrProgressExists) {
		t.Errorf("want ErrProgressExists, got %v", err)
	}
	// With force, overwrites.
	if _, err := InitProgress(root, "PROJ-1", "/w2", "b2", true); err != nil {
		t.Errorf("force init: %v", err)
	}
	p2, _ := ReadProgress(root, "PROJ-1")
	if p2.Worktree != "/w2" {
		t.Errorf("force init did not overwrite worktree, got %q", p2.Worktree)
	}
}

func TestInitProgress_InvalidKey(t *testing.T) {
	root := t.TempDir()
	if _, err := InitProgress(root, "../bad", "/w", "b", false); err == nil {
		t.Errorf("expected error for invalid ticket key")
	}
}

func TestReadProgress_Missing(t *testing.T) {
	p, err := ReadProgress(t.TempDir(), "PROJ-1")
	if err != nil {
		t.Fatalf("missing file should return nil,nil, got err=%v", err)
	}
	if p != nil {
		t.Errorf("missing file should return nil progress, got %+v", p)
	}
}

func TestCompleteStep_AdvancesPastConsecutive(t *testing.T) {
	root := t.TempDir()
	if _, err := InitProgress(root, "PROJ-1", "/w", "b", false); err != nil {
		t.Fatal(err)
	}
	// Pre-seed completed=[2,3,4] manually to test the "advance past
	// consecutive" behaviour.
	path := ProgressFile(root, "PROJ-1")
	var raw map[string]any
	data, _ := os.ReadFile(path)
	_ = json.Unmarshal(data, &raw)
	raw["completed"] = []int{2, 3, 4}
	out, _ := json.MarshalIndent(raw, "", "  ")
	_ = os.WriteFile(path, out, 0o644)

	if err := CompleteStep(root, "PROJ-1", 1); err != nil {
		t.Fatalf("complete: %v", err)
	}
	got, _ := ReadProgress(root, "PROJ-1")
	if got.CurrentStep != 5 {
		t.Errorf("want CurrentStep=5 (skip 2,3,4), got %d", got.CurrentStep)
	}
	if len(got.Completed) != 4 {
		t.Errorf("want completed=[1,2,3,4], got %v", got.Completed)
	}
}

func TestCompleteStep_ClampsToMaxStep(t *testing.T) {
	root := t.TempDir()
	_, _ = InitProgress(root, "PROJ-1", "/w", "b", false)
	// Mark every step as completed then complete MaxStep.
	for i := 1; i <= MaxStep; i++ {
		if err := CompleteStep(root, "PROJ-1", i); err != nil {
			t.Fatalf("complete %d: %v", i, err)
		}
	}
	got, _ := ReadProgress(root, "PROJ-1")
	if got.CurrentStep != MaxStep {
		t.Errorf("want current_step=%d at terminal, got %d", MaxStep, got.CurrentStep)
	}
	if len(got.Completed) != MaxStep {
		t.Errorf("want %d completed, got %d", MaxStep, len(got.Completed))
	}
}

func TestCompleteStep_Idempotent(t *testing.T) {
	root := t.TempDir()
	_, _ = InitProgress(root, "PROJ-1", "/w", "b", false)
	if err := CompleteStep(root, "PROJ-1", 1); err != nil {
		t.Fatal(err)
	}
	if err := CompleteStep(root, "PROJ-1", 1); err != nil {
		t.Fatal(err)
	}
	got, _ := ReadProgress(root, "PROJ-1")
	if len(got.Completed) != 1 {
		t.Errorf("want completed=[1] (dedup), got %v", got.Completed)
	}
}

func TestSetField_Whitelist(t *testing.T) {
	root := t.TempDir()
	_, _ = InitProgress(root, "PROJ-1", "/w", "b", false)

	// Allowed: pr_url.
	if err := SetField(root, "PROJ-1", "pr_url", "https://github.com/org/repo/pull/1"); err != nil {
		t.Fatalf("set pr_url: %v", err)
	}
	got, _ := GetField(root, "PROJ-1", "pr_url")
	if got != "https://github.com/org/repo/pull/1" {
		t.Errorf("pr_url round-trip failed, got %q", got)
	}

	// Rejected: ticket (immutable).
	if err := SetField(root, "PROJ-1", "ticket", "hacked"); !errors.Is(err, ErrUnknownField) {
		t.Errorf("want ErrUnknownField for ticket, got %v", err)
	}
	// Rejected: bogus field.
	if err := SetField(root, "PROJ-1", "__proto__", "oops"); !errors.Is(err, ErrUnknownField) {
		t.Errorf("want ErrUnknownField for __proto__, got %v", err)
	}
}

func TestSetField_TitleAppendedAfterInit(t *testing.T) {
	root := t.TempDir()
	_, _ = InitProgress(root, "PROJ-1", "/w", "b", false)
	if err := SetField(root, "PROJ-1", "title", "Hello world"); err != nil {
		t.Fatal(err)
	}
	got, _ := GetField(root, "PROJ-1", "title")
	if got != "Hello world" {
		t.Errorf("title round-trip: got %q", got)
	}
}

func TestProgress_JSONFieldOrder(t *testing.T) {
	root := t.TempDir()
	_, _ = InitProgress(root, "PROJ-1", "/w", "b", false)
	// Read the raw bytes and assert the jq-style key order used by
	// progress-state.sh init. This protects the bash→Go interop claim
	// in DESIGN §4 ("byte-compatible schema").
	data, err := os.ReadFile(ProgressFile(root, "PROJ-1"))
	if err != nil {
		t.Fatal(err)
	}
	wantOrder := []string{
		"ticket", "started_at", "worktree", "branch", "current_step",
		"completed", "pr_url", "gsd_quick_id", "impl_plan_dir",
	}
	assertJSONKeyOrder(t, data, wantOrder)
}

func TestTicketPaths(t *testing.T) {
	root := "/tmp/root"
	got := TicketDir(root, "PROJ-42")
	want := filepath.Join(root, ".planning", "ticket", "PROJ-42")
	if got != want {
		t.Errorf("TicketDir: got %q, want %q", got, want)
	}
	got = ProgressFile(root, "PROJ-42")
	want = filepath.Join(root, ".planning", "ticket", "PROJ-42", "workflow-progress.json")
	if got != want {
		t.Errorf("ProgressFile: got %q, want %q", got, want)
	}
}

// assertJSONKeyOrder walks the raw JSON bytes and checks the first
// occurrence of each wanted key appears in the given order.
func assertJSONKeyOrder(t *testing.T, data []byte, want []string) {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(data))
	if _, err := dec.Token(); err != nil {
		t.Fatalf("decode opening: %v", err)
	}
	var got []string
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			t.Fatalf("token: %v", err)
		}
		key, ok := tok.(string)
		if !ok {
			t.Fatalf("expected key, got %v", tok)
		}
		got = append(got, key)
		var v any
		if err := dec.Decode(&v); err != nil {
			t.Fatalf("value for %q: %v", key, err)
		}
	}
	for i, k := range want {
		if i >= len(got) || got[i] != k {
			t.Errorf("key order: want[%d]=%q got=%v", i, k, got)
			return
		}
	}
}
