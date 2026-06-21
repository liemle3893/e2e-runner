package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInitLoop_FreshAndForce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "coverage-review-state.json")
	if err := InitLoop(path, "coverage-review", "PROJ-1", 3, false); err != nil {
		t.Fatalf("init: %v", err)
	}
	// Refuses to overwrite.
	if err := InitLoop(path, "coverage-review", "PROJ-1", 3, false); !errors.Is(err, ErrLoopExists) {
		t.Errorf("want ErrLoopExists, got %v", err)
	}
	// Force overwrites.
	if err := InitLoop(path, "coverage-review", "PROJ-1", 5, true); err != nil {
		t.Fatalf("force: %v", err)
	}
	s, _ := ReadLoop(path)
	if s.MaxRounds != 5 {
		t.Errorf("force should overwrite max_rounds, got %d", s.MaxRounds)
	}
	if len(s.Rounds) != 0 {
		t.Errorf("new state should have empty rounds, got %d", len(s.Rounds))
	}
}

func TestAppendRound_HappyPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := InitLoop(path, "loop", "PROJ-1", 3, false); err != nil {
		t.Fatal(err)
	}
	if err := AppendRound(path, json.RawMessage(`{"status":"PASS","note":"first"}`)); err != nil {
		t.Fatalf("append: %v", err)
	}
	s, _ := ReadLoop(path)
	if len(s.Rounds) != 1 {
		t.Fatalf("want 1 round, got %d", len(s.Rounds))
	}

	// Round body should contain status, note, round, timestamp.
	var round map[string]any
	_ = json.Unmarshal(s.Rounds[0], &round)
	if round["status"] != "PASS" {
		t.Errorf("want status=PASS, got %v", round["status"])
	}
	if round["note"] != "first" {
		t.Errorf("caller field lost, got %v", round["note"])
	}
	if round["round"] == nil {
		t.Errorf("round number not injected")
	}
	if round["timestamp"] == nil {
		t.Errorf("timestamp not injected")
	}
}

func TestAppendRound_MissingStatus(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	_ = InitLoop(path, "loop", "PROJ-1", 3, false)
	err := AppendRound(path, json.RawMessage(`{"note":"no status"}`))
	if !errors.Is(err, ErrRoundMissingStatus) {
		t.Errorf("want ErrRoundMissingStatus, got %v", err)
	}
}

func TestAppendRound_MaxRoundsEnforced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	_ = InitLoop(path, "loop", "PROJ-1", 2, false)
	if err := AppendRound(path, json.RawMessage(`{"status":"X"}`)); err != nil {
		t.Fatal(err)
	}
	if err := AppendRound(path, json.RawMessage(`{"status":"X"}`)); err != nil {
		t.Fatal(err)
	}
	err := AppendRound(path, json.RawMessage(`{"status":"X"}`))
	if !errors.Is(err, ErrMaxRoundsExceeded) {
		t.Errorf("want ErrMaxRoundsExceeded, got %v", err)
	}
}

func TestRoundCount_MissingIsZero(t *testing.T) {
	n, err := RoundCount(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Errorf("missing file should not error, got %v", err)
	}
	if n != 0 {
		t.Errorf("want 0, got %d", n)
	}
}

func TestAppendRound_BadJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	_ = InitLoop(path, "loop", "PROJ-1", 3, false)
	if err := AppendRound(path, json.RawMessage(`{ not valid`)); err == nil {
		t.Errorf("expected error on invalid JSON")
	}
}

func TestInitLoop_CreatesParentDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "deeper", "state.json")
	if err := InitLoop(path, "l", "K-1", 1, false); err != nil {
		t.Fatalf("init with new parent: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file not created: %v", err)
	}
}
