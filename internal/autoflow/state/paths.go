// Package state implements the durable JSON state files that drive the
// autoflow-deliver workflow. It replaces the shell scripts progress-state.sh,
// loop-state.sh, review-loop.sh, and verify-gates.sh from winx-autoflow.
//
// File layout under a repo root:
//
//	.planning/ticket/<KEY>/
//	    task-brief.md
//	    title.txt
//	    workflow-progress.json
//	    state/
//	        coverage-review-state.json
//	        e2e-fix-state.json
//	        build-gate-state.json
//	        code-review-state.json
//	        review-feedback.json
//	        .review-state-checksum
//	        REVIEW-{code,simplify,rules,FIX}.md
//	        *.marker
package state

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// ticketKeyPattern matches Jira-style keys: one uppercase letter, then
// uppercase letters/digits, then a dash, then digits. Guards against path
// traversal in commands that build paths from a ticket key.
var ticketKeyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9]+-\d+$`)

// ValidateTicketKey returns an error if key is not a syntactically valid
// Jira key (e.g. "PROJ-42"). Used at every boundary that receives a key
// from an external caller.
func ValidateTicketKey(key string) error {
	if !ticketKeyPattern.MatchString(key) {
		return fmt.Errorf("invalid ticket key %q: expected PROJECT-123 format", key)
	}
	return nil
}

// RepoRoot returns the git top-level directory, or falls back to "." when
// the cwd is not inside a git repo. Callers that require a repo should
// check the returned value, but most paths can tolerate "." on fresh
// projects.
func RepoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("resolve repo root: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// TicketDir returns .planning/ticket/<KEY>/ under root. Does not create it.
func TicketDir(root, key string) string {
	return filepath.Join(root, ".planning", "ticket", key)
}

// TicketStateDir returns .planning/ticket/<KEY>/state/ under root. Does not
// create it.
func TicketStateDir(root, key string) string {
	return filepath.Join(TicketDir(root, key), "state")
}

// ProgressFile returns the path to workflow-progress.json for a ticket.
func ProgressFile(root, key string) string {
	return filepath.Join(TicketDir(root, key), "workflow-progress.json")
}
