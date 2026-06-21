package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

// LoopState is the JSON shape of a generic agentic-loop state file
// (coverage-review-state.json, enhance-loop-state.json, e2e-fix-state.json,
// etc.). Round bodies are stored as raw JSON objects so agent-authored
// fields pass through verbatim.
type LoopState struct {
	Loop      string            `json:"loop"`
	Ticket    string            `json:"ticket"`
	MaxRounds int               `json:"max_rounds"`
	Rounds    []json.RawMessage `json:"rounds"`
}

// ErrLoopExists is returned by InitLoop when the state file is already
// present and force is false.
var ErrLoopExists = errors.New("loop state file already exists")

// ErrMaxRoundsExceeded is returned when AppendRound would push the round
// count past max_rounds.
var ErrMaxRoundsExceeded = errors.New("max rounds exceeded")

// ErrRoundMissingStatus is returned when AppendRound receives a body that
// lacks the required `status` field.
var ErrRoundMissingStatus = errors.New("round body missing required field 'status'")

// InitLoop creates an empty loop-state file at path. Refuses to overwrite
// unless force is true. Creates parent directories as needed.
func InitLoop(path, loop, ticket string, maxRounds int, force bool) error {
	if !force {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("%w: %s", ErrLoopExists, path)
		}
	}
	s := LoopState{
		Loop:      loop,
		Ticket:    ticket,
		MaxRounds: maxRounds,
		Rounds:    []json.RawMessage{},
	}
	return WriteJSONAtomic(path, s)
}

// AppendRound appends one round to the state file at path. The provided
// body must be a valid JSON object containing at least a `status` field.
// The round number and timestamp are assigned by this function and merged
// into the body before append (overriding any caller-provided values).
func AppendRound(path string, body json.RawMessage) error {
	s, err := readLoop(path)
	if err != nil {
		return err
	}

	if err := validateRoundBody(body); err != nil {
		return err
	}

	next := len(s.Rounds) + 1
	if next > s.MaxRounds {
		return fmt.Errorf("%w (%d): cannot append round %d", ErrMaxRoundsExceeded, s.MaxRounds, next)
	}

	// Merge {round, timestamp} into the body. Parse as map so we can
	// inject the fields without losing the caller's keys.
	var merged map[string]any
	if err := json.Unmarshal(body, &merged); err != nil {
		return fmt.Errorf("parse round body: %w", err)
	}
	merged["round"] = next
	merged["timestamp"] = time.Now().UTC().Format("2006-01-02T15:04:05Z")

	remerged, err := json.Marshal(merged)
	if err != nil {
		return fmt.Errorf("re-marshal round: %w", err)
	}
	s.Rounds = append(s.Rounds, remerged)

	return WriteJSONAtomic(path, s)
}

// RoundCount returns the number of rounds recorded. A missing file returns
// 0 with no error, matching the bash script's behaviour.
func RoundCount(path string) (int, error) {
	s, err := readLoop(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	return len(s.Rounds), nil
}

// ReadLoop returns the state file contents, or os.ErrNotExist when absent.
func ReadLoop(path string) (*LoopState, error) {
	return readLoop(path)
}

func readLoop(path string) (*LoopState, error) {
	var s LoopState
	if err := readJSON(path, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func validateRoundBody(body json.RawMessage) error {
	var probe struct {
		Status *string `json:"status"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return fmt.Errorf("round body is not valid JSON: %w", err)
	}
	if probe.Status == nil {
		return ErrRoundMissingStatus
	}
	return nil
}
