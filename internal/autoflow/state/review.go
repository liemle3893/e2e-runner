package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ReviewState is the code-review loop state. Unlike LoopState, rounds here
// have a fixed shape (problems, fixes, clean_count, etc.) and the file is
// integrity-protected by a sha256 checksum sidecar.
type ReviewState struct {
	Loop      string        `json:"loop"`
	Ticket    string        `json:"ticket"`
	MaxRounds int           `json:"max_rounds"`
	Rounds    []ReviewRound `json:"rounds"`
}

// ReviewRound is one recorded review round. See review-loop.sh end-round
// for the corresponding jq schema.
type ReviewRound struct {
	Round               int             `json:"round"`
	Timestamp           string          `json:"timestamp"`
	Status              string          `json:"status"`
	CleanCount          int             `json:"clean_count"`
	BugsFound           int             `json:"bugs_found,omitempty"`
	DesignConcernsFound int             `json:"design_concerns_found,omitempty"`
	FeedbackIDs         []string        `json:"feedback_ids,omitempty"`
	Problems            json.RawMessage `json:"problems"`
	Fixes               json.RawMessage `json:"fixes"`
}

// ReviewFeedback is the accumulated feedback ledger shared across rounds.
type ReviewFeedback struct {
	Ticket  string            `json:"ticket"`
	Entries []json.RawMessage `json:"entries"`
}

// ErrReviewStateTampered is returned when the checksum on disk no longer
// matches the state files, signalling an out-of-band mutation.
var ErrReviewStateTampered = errors.New("review state files were modified outside review-loop")

// ReviewPaths groups the three paths that make up a review loop's state.
type ReviewPaths struct {
	StateFile    string // code-review-state.json (or derivative)
	FeedbackFile string // review-feedback.json
	ChecksumFile string // .review-state-checksum
}

// DefaultReviewPaths returns the standard set of review paths under the
// ticket's state directory. stateName defaults to code-review-state.json
// when empty.
func DefaultReviewPaths(root, key, stateName string) ReviewPaths {
	if stateName == "" {
		stateName = "code-review-state.json"
	}
	stateDir := TicketStateDir(root, key)
	return ReviewPaths{
		StateFile:    filepath.Join(stateDir, stateName),
		FeedbackFile: filepath.Join(stateDir, "review-feedback.json"),
		ChecksumFile: filepath.Join(stateDir, ".review-state-checksum"),
	}
}

// LoopName derives the loop label from the state filename (strip
// "-state.json"), matching the bash script's STATE_NAME handling.
func (p ReviewPaths) LoopName() string {
	base := filepath.Base(p.StateFile)
	return strings.TrimSuffix(base, "-state.json")
}

// InitReview initialises both the state and feedback files if they do not
// already exist. Idempotent — existing files are left untouched.
func InitReview(paths ReviewPaths, ticket string, maxRounds int) error {
	if _, err := os.Stat(paths.StateFile); errors.Is(err, os.ErrNotExist) {
		s := ReviewState{
			Loop:      paths.LoopName(),
			Ticket:    ticket,
			MaxRounds: maxRounds,
			Rounds:    []ReviewRound{},
		}
		if err := WriteJSONAtomic(paths.StateFile, s); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	if _, err := os.Stat(paths.FeedbackFile); errors.Is(err, os.ErrNotExist) {
		fb := ReviewFeedback{Ticket: ticket, Entries: []json.RawMessage{}}
		if err := WriteJSONAtomic(paths.FeedbackFile, fb); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	return nil
}

// ValidateReviewIntegrity checks that state + feedback files match the
// stored checksum. Returns nil when no checksum is present yet (first run)
// or when checksums match. Returns ErrReviewStateTampered on mismatch.
func ValidateReviewIntegrity(paths ReviewPaths) error {
	saved, err := os.ReadFile(paths.ChecksumFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read checksum: %w", err)
	}
	parts := strings.SplitN(strings.TrimSpace(string(saved)), ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("malformed checksum file %s", paths.ChecksumFile)
	}
	stateHash, err := hashFile(paths.StateFile)
	if err != nil {
		return err
	}
	feedbackHash, err := hashFile(paths.FeedbackFile)
	if err != nil {
		return err
	}
	if stateHash != parts[0] || feedbackHash != parts[1] {
		return fmt.Errorf("%w (state=%v feedback=%v)", ErrReviewStateTampered,
			stateHash == parts[0], feedbackHash == parts[1])
	}
	return nil
}

// SaveReviewChecksum snapshots the current hashes of state + feedback for
// the next integrity check.
func SaveReviewChecksum(paths ReviewPaths) error {
	stateHash, err := hashFile(paths.StateFile)
	if err != nil {
		return err
	}
	feedbackHash, err := hashFile(paths.FeedbackFile)
	if err != nil {
		return err
	}
	return writeFileAtomic(paths.ChecksumFile, []byte(stateHash+":"+feedbackHash+"\n"), 0o644)
}

// BeginReviewRound returns (nextRound, consecutiveClean). nextRound is one
// past the current length; consecutiveClean is the trailing run of CLEAN
// statuses. Returns an error when the next round would exceed MaxRounds.
func BeginReviewRound(paths ReviewPaths) (nextRound, cleanCount int, err error) {
	if err := ValidateReviewIntegrity(paths); err != nil {
		return 0, 0, err
	}
	var s ReviewState
	if err := readJSON(paths.StateFile, &s); err != nil {
		return 0, 0, err
	}
	nextRound = len(s.Rounds) + 1
	if nextRound > s.MaxRounds {
		return 0, 0, fmt.Errorf("%w: cannot start round %d of %d", ErrMaxRoundsExceeded, nextRound, s.MaxRounds)
	}
	for i := len(s.Rounds) - 1; i >= 0; i-- {
		if s.Rounds[i].Status == "CLEAN" {
			cleanCount++
		} else {
			break
		}
	}
	return nextRound, cleanCount, nil
}

// RoundData is the structured body callers pass to EndReviewRound. Mirrors
// the `--round-data` JSON shape from review-loop.sh end-round.
type RoundData struct {
	BugsFound           int             `json:"bugs_found"`
	DesignConcernsFound int             `json:"design_concerns_found"`
	FeedbackIDs         []string        `json:"feedback_ids"`
	Problems            json.RawMessage `json:"problems"`
	Fixes               json.RawMessage `json:"fixes"`
}

// EndReviewRound appends a round to the state file, optionally appends
// feedback entries, then rewrites the integrity checksum.
func EndReviewRound(paths ReviewPaths, status string, data *RoundData, feedback []json.RawMessage) error {
	if status == "" {
		return fmt.Errorf("status is required (CLEAN or ISSUES_FOUND)")
	}
	if err := ValidateReviewIntegrity(paths); err != nil {
		return err
	}
	var s ReviewState
	if err := readJSON(paths.StateFile, &s); err != nil {
		return err
	}

	cleanCount := 0
	if status == "CLEAN" {
		cleanCount = 1
		for i := len(s.Rounds) - 1; i >= 0; i-- {
			if s.Rounds[i].Status == "CLEAN" {
				cleanCount++
			} else {
				break
			}
		}
	}

	round := ReviewRound{
		Round:      len(s.Rounds) + 1,
		Timestamp:  time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		Status:     status,
		CleanCount: cleanCount,
	}
	if data != nil {
		round.BugsFound = data.BugsFound
		round.DesignConcernsFound = data.DesignConcernsFound
		round.FeedbackIDs = data.FeedbackIDs
		round.Problems = data.Problems
		round.Fixes = data.Fixes
	}
	// Ensure problems/fixes are always present as arrays even when empty,
	// so readers don't need to handle JSON null.
	if round.Problems == nil {
		round.Problems = json.RawMessage(`[]`)
	}
	if round.Fixes == nil {
		round.Fixes = json.RawMessage(`[]`)
	}

	s.Rounds = append(s.Rounds, round)
	if err := WriteJSONAtomic(paths.StateFile, s); err != nil {
		return err
	}

	if len(feedback) > 0 {
		var fb ReviewFeedback
		if err := readJSON(paths.FeedbackFile, &fb); err != nil {
			return err
		}
		fb.Entries = append(fb.Entries, feedback...)
		if err := WriteJSONAtomic(paths.FeedbackFile, fb); err != nil {
			return err
		}
	}

	return SaveReviewChecksum(paths)
}

// ReadFeedback returns the full feedback ledger.
func ReadFeedback(paths ReviewPaths) (*ReviewFeedback, error) {
	var fb ReviewFeedback
	if err := readJSON(paths.FeedbackFile, &fb); err != nil {
		return nil, err
	}
	return &fb, nil
}

func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
