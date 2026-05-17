package state

import (
	"encoding/json"
	"fmt"
	"os"
)

// VerifyIssue is one finding from state-file validation. Severity is
// either "FAIL" (structural violation) or "WARN" (suspicious but not
// fatal, e.g. a FAILED round with no recorded problems).
type VerifyIssue struct {
	Severity string
	Message  string
}

// VerifyLoopStateFile checks that path has the structure a loop-state
// consumer expects: required top-level fields, required per-round fields,
// and warns when a non-terminal failed round has no problems recorded.
// Returns (rounds, issues). An empty issues slice means pass.
func VerifyLoopStateFile(path string) (int, []VerifyIssue, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, nil, fmt.Errorf("read %s: %w", path, err)
	}

	// Use map[string]any so we can detect presence vs absence of fields
	// uniformly, matching the bash `jq -e` behaviour.
	var top map[string]any
	if err := json.Unmarshal(data, &top); err != nil {
		return 0, []VerifyIssue{{Severity: "FAIL", Message: fmt.Sprintf("not valid JSON: %v", err)}}, nil
	}

	issues := []VerifyIssue{}
	for _, field := range []string{"loop", "ticket", "max_rounds", "rounds"} {
		if _, ok := top[field]; !ok {
			issues = append(issues, VerifyIssue{
				Severity: "FAIL",
				Message:  fmt.Sprintf("missing required field %q", field),
			})
		}
	}

	rounds, _ := top["rounds"].([]any)
	roundCount := len(rounds)

	for i, r := range rounds {
		round, ok := r.(map[string]any)
		if !ok {
			issues = append(issues, VerifyIssue{
				Severity: "FAIL",
				Message:  fmt.Sprintf("round %d is not an object", i+1),
			})
			continue
		}
		for _, field := range []string{"round", "timestamp", "status"} {
			if _, ok := round[field]; !ok {
				issues = append(issues, VerifyIssue{
					Severity: "FAIL",
					Message:  fmt.Sprintf("round %d missing field %q", i+1, field),
				})
			}
		}

		// Warn when a non-terminal failed round has no problems recorded.
		if i < roundCount-1 {
			status, _ := round["status"].(string)
			if status == "FAILED" || status == "GAPS_FOUND" || status == "ISSUES_FOUND" {
				probs, _ := round["problems"].([]any)
				if len(probs) == 0 {
					issues = append(issues, VerifyIssue{
						Severity: "WARN",
						Message: fmt.Sprintf("round %d has status %q but no problems recorded",
							i+1, status),
					})
				}
			}
		}
	}

	return roundCount, issues, nil
}
