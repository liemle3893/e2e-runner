package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newReviewPaths(t *testing.T) ReviewPaths {
	t.Helper()
	dir := t.TempDir()
	return ReviewPaths{
		StateFile:    filepath.Join(dir, "code-review-state.json"),
		FeedbackFile: filepath.Join(dir, "review-feedback.json"),
		ChecksumFile: filepath.Join(dir, ".review-state-checksum"),
	}
}

func TestInitReview_Idempotent(t *testing.T) {
	p := newReviewPaths(t)
	if err := InitReview(p, "PROJ-1", 5); err != nil {
		t.Fatal(err)
	}
	// Write a marker to ensure a second init does not clobber.
	_ = os.WriteFile(p.StateFile+".sentinel", []byte("x"), 0o644)
	if err := InitReview(p, "PROJ-1", 5); err != nil {
		t.Fatal(err)
	}
}

func TestBeginReviewRound_AtLimitRejects(t *testing.T) {
	p := newReviewPaths(t)
	_ = InitReview(p, "PROJ-1", 2)
	// Push two rounds through the normal path.
	for i := 0; i < 2; i++ {
		if err := EndReviewRound(p, "ISSUES_FOUND", nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	_, _, err := BeginReviewRound(p)
	if !errors.Is(err, ErrMaxRoundsExceeded) {
		t.Errorf("want ErrMaxRoundsExceeded at %d rounds, got %v", 2, err)
	}
}

func TestEndReviewRound_CleanCountConsecutive(t *testing.T) {
	p := newReviewPaths(t)
	_ = InitReview(p, "PROJ-1", 10)

	if err := EndReviewRound(p, "ISSUES_FOUND", nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := EndReviewRound(p, "CLEAN", nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := EndReviewRound(p, "CLEAN", nil, nil); err != nil {
		t.Fatal(err)
	}

	var s ReviewState
	data, _ := os.ReadFile(p.StateFile)
	_ = json.Unmarshal(data, &s)
	if got := s.Rounds[2].CleanCount; got != 2 {
		t.Errorf("want consecutive clean=2 at round 3, got %d", got)
	}
}

func TestReviewIntegrity_DetectsTampering(t *testing.T) {
	p := newReviewPaths(t)
	_ = InitReview(p, "PROJ-1", 3)
	if err := EndReviewRound(p, "CLEAN", nil, nil); err != nil {
		t.Fatal(err)
	}

	// Manually mutate the state file — simulating an LLM hand-edit.
	data, _ := os.ReadFile(p.StateFile)
	tampered := strings.Replace(string(data), `"status": "CLEAN"`, `"status": "BYPASS"`, 1)
	if err := os.WriteFile(p.StateFile, []byte(tampered), 0o644); err != nil {
		t.Fatal(err)
	}

	err := ValidateReviewIntegrity(p)
	if !errors.Is(err, ErrReviewStateTampered) {
		t.Errorf("want tampering detected, got %v", err)
	}
	// BeginReviewRound also refuses.
	if _, _, err := BeginReviewRound(p); !errors.Is(err, ErrReviewStateTampered) {
		t.Errorf("begin-round should refuse tampered state, got %v", err)
	}
}

func TestEndReviewRound_AppendsFeedback(t *testing.T) {
	p := newReviewPaths(t)
	_ = InitReview(p, "PROJ-1", 3)
	feedback := []json.RawMessage{
		json.RawMessage(`{"id":"CR-01","severity":"critical"}`),
		json.RawMessage(`{"id":"CR-02","severity":"warning"}`),
	}
	if err := EndReviewRound(p, "ISSUES_FOUND", nil, feedback); err != nil {
		t.Fatal(err)
	}
	fb, err := ReadFeedback(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(fb.Entries) != 2 {
		t.Errorf("want 2 feedback entries, got %d", len(fb.Entries))
	}
}

func TestEndReviewRound_RoundDataRoundTrip(t *testing.T) {
	p := newReviewPaths(t)
	_ = InitReview(p, "PROJ-1", 3)
	data := &RoundData{
		BugsFound:           3,
		DesignConcernsFound: 1,
		FeedbackIDs:         []string{"CR-01", "CR-02"},
		Problems:            json.RawMessage(`[{"id":"CR-01"}]`),
		Fixes:               json.RawMessage(`[{"file":"foo.go","action":"renamed"}]`),
	}
	if err := EndReviewRound(p, "ISSUES_FOUND", data, nil); err != nil {
		t.Fatal(err)
	}
	var s ReviewState
	raw, _ := os.ReadFile(p.StateFile)
	_ = json.Unmarshal(raw, &s)
	if s.Rounds[0].BugsFound != 3 {
		t.Errorf("bugs_found lost: got %d", s.Rounds[0].BugsFound)
	}
	if len(s.Rounds[0].FeedbackIDs) != 2 {
		t.Errorf("feedback_ids lost: got %v", s.Rounds[0].FeedbackIDs)
	}
}
