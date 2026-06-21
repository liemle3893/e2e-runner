package state

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"time"
)

// MaxStep is the highest workflow step number. `complete` clamps
// current_step to this; `start` rejects larger values.
const MaxStep = 13

// allowedSetFields lists the fields mutable via (*Progress).Set. Matches
// ALLOWED_FIELDS in progress-state.sh.
var allowedSetFields = []string{
	"pr_url", "gsd_quick_id", "impl_plan_dir", "worktree", "branch", "title",
}

// Progress is the durable shape of workflow-progress.json. Field order
// matches the original jq output so the on-disk layout is unchanged across
// the bash→Go migration.
type Progress struct {
	Ticket      string  `json:"ticket"`
	StartedAt   string  `json:"started_at"`
	Worktree    string  `json:"worktree"`
	Branch      string  `json:"branch"`
	CurrentStep int     `json:"current_step"`
	Completed   []int   `json:"completed"`
	PRURL       *string `json:"pr_url"`
	GSDQuickID  *string `json:"gsd_quick_id"`
	ImplPlanDir *string `json:"impl_plan_dir"`
	Title       *string `json:"title,omitempty"`
}

// ErrProgressExists is returned by InitProgress when the state file is
// already present and force is false.
var ErrProgressExists = errors.New("workflow-progress.json already exists")

// ErrProgressMissing is returned when an operation requires an existing
// progress file but none is present.
var ErrProgressMissing = errors.New("workflow-progress.json not found — run init first")

// ErrUnknownField is returned by Set when the field name is not in the
// whitelist. Mirrors the bash validate_field rejection.
var ErrUnknownField = errors.New("field is not in the allowed set")

// ErrInvalidStep is returned when a step number is outside [1, MaxStep].
var ErrInvalidStep = errors.New("step must be between 1 and MaxStep")

// InitProgress writes a fresh workflow-progress.json for the ticket.
// Refuses to overwrite unless force is true.
func InitProgress(root, key, worktree, branch string, force bool) (*Progress, error) {
	if err := ValidateTicketKey(key); err != nil {
		return nil, err
	}
	path := ProgressFile(root, key)
	if !force {
		if _, err := os.Stat(path); err == nil {
			return nil, fmt.Errorf("%w: %s", ErrProgressExists, path)
		}
	}

	p := &Progress{
		Ticket:      key,
		StartedAt:   time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		Worktree:    worktree,
		Branch:      branch,
		CurrentStep: 1,
		Completed:   []int{},
		// PRURL / GSDQuickID / ImplPlanDir left as nil → serialised as null.
	}
	if err := WriteJSONAtomic(path, p); err != nil {
		return nil, err
	}
	return p, nil
}

// ReadProgress returns the current state for a ticket, or nil (with no
// error) when no progress file exists. A parse error is returned when the
// file exists but is malformed.
func ReadProgress(root, key string) (*Progress, error) {
	if err := ValidateTicketKey(key); err != nil {
		return nil, err
	}
	path := ProgressFile(root, key)
	var p Progress
	if err := readJSON(path, &p); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

// StartStep sets current_step to step. Idempotent — writing the same step
// repeatedly is fine. step must be in [1, MaxStep].
func StartStep(root, key string, step int) error {
	if err := ValidateTicketKey(key); err != nil {
		return err
	}
	if step < 1 || step > MaxStep {
		return fmt.Errorf("%w: got %d", ErrInvalidStep, step)
	}
	p, err := mustReadProgress(root, key)
	if err != nil {
		return err
	}
	p.CurrentStep = step
	return WriteJSONAtomic(ProgressFile(root, key), p)
}

// CompleteStep marks step as done and advances current_step past every
// consecutively-completed step, clamped at MaxStep. Set-semantic: completing
// an already-completed step is a no-op aside from the advance.
func CompleteStep(root, key string, step int) error {
	if err := ValidateTicketKey(key); err != nil {
		return err
	}
	if step < 1 || step > MaxStep {
		return fmt.Errorf("%w: got %d", ErrInvalidStep, step)
	}
	p, err := mustReadProgress(root, key)
	if err != nil {
		return err
	}

	// Append (set-semantic) and sort.
	if !slices.Contains(p.Completed, step) {
		p.Completed = append(p.Completed, step)
	}
	slices.Sort(p.Completed)

	// Advance past consecutive completed steps starting at step+1.
	next := step + 1
	for next <= MaxStep && slices.Contains(p.Completed, next) {
		next++
	}
	if next > MaxStep {
		next = MaxStep
	}
	p.CurrentStep = next

	return WriteJSONAtomic(ProgressFile(root, key), p)
}

// SetField updates one whitelisted field. Returns ErrUnknownField for
// names outside the allowed set.
func SetField(root, key, field, value string) error {
	if err := ValidateTicketKey(key); err != nil {
		return err
	}
	if !slices.Contains(allowedSetFields, field) {
		return fmt.Errorf("%w: %q (allowed: %v)", ErrUnknownField, field, allowedSetFields)
	}
	p, err := mustReadProgress(root, key)
	if err != nil {
		return err
	}

	// Assign through the struct so JSON ordering matches bash output.
	v := value
	switch field {
	case "pr_url":
		p.PRURL = &v
	case "gsd_quick_id":
		p.GSDQuickID = &v
	case "impl_plan_dir":
		p.ImplPlanDir = &v
	case "worktree":
		p.Worktree = v
	case "branch":
		p.Branch = v
	case "title":
		p.Title = &v
	}
	return WriteJSONAtomic(ProgressFile(root, key), p)
}

// GetField returns the current value of one field as its natural string
// form (empty string for null pointer fields). Unknown fields return
// ErrUnknownField.
func GetField(root, key, field string) (string, error) {
	if err := ValidateTicketKey(key); err != nil {
		return "", err
	}
	p, err := mustReadProgress(root, key)
	if err != nil {
		return "", err
	}
	switch field {
	case "ticket":
		return p.Ticket, nil
	case "started_at":
		return p.StartedAt, nil
	case "worktree":
		return p.Worktree, nil
	case "branch":
		return p.Branch, nil
	case "current_step":
		return fmt.Sprintf("%d", p.CurrentStep), nil
	case "pr_url":
		return derefOr(p.PRURL, ""), nil
	case "gsd_quick_id":
		return derefOr(p.GSDQuickID, ""), nil
	case "impl_plan_dir":
		return derefOr(p.ImplPlanDir, ""), nil
	case "title":
		return derefOr(p.Title, ""), nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnknownField, field)
	}
}

func mustReadProgress(root, key string) (*Progress, error) {
	p, err := ReadProgress(root, key)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, fmt.Errorf("%w (ticket=%s)", ErrProgressMissing, key)
	}
	return p, nil
}

func derefOr(p *string, fallback string) string {
	if p == nil {
		return fallback
	}
	return *p
}
