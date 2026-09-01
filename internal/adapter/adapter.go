package adapter

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/liemle3893/go-tryve/internal/tryve"
)

// unresolvedEnvVarRe matches ${VAR_NAME} placeholders that were not resolved
// during config loading (i.e. the environment variable was not set).
var unresolvedEnvVarRe = regexp.MustCompile(`\$\{(\w+)\}`)

// CheckUnresolvedEnvVars inspects value for leftover ${VAR} placeholders and
// returns a clear ConnectionError naming every missing environment variable.
// Returns nil when no placeholders remain.
func CheckUnresolvedEnvVars(adapterName, fieldName, value string) error {
	matches := unresolvedEnvVarRe.FindAllStringSubmatch(value, -1)
	if len(matches) == 0 {
		return nil
	}
	vars := make([]string, 0, len(matches))
	for _, m := range matches {
		vars = append(vars, m[1])
	}
	return tryve.ConnectionError(
		adapterName,
		fmt.Sprintf(
			"%s contains unresolved environment variable(s): %s — set them in your shell or .env file",
			fieldName, strings.Join(vars, ", "),
		),
		nil,
	)
}

// Adapter is the core interface all protocol adapters must implement.
// Each method must be safe to call concurrently from a single goroutine;
// the Registry serialises Connect/Close calls via its own mutex.
type Adapter interface {
	// Name returns the adapter's registered identifier (e.g. "http", "db").
	Name() string

	// Connect establishes the underlying connection or session.
	// It is called at most once per adapter instance by the Registry.
	Connect(ctx context.Context) error

	// Close tears down the adapter's connection or session.
	// It is called by Registry.CloseAll for every connected adapter.
	Close(ctx context.Context) error

	// Health performs a lightweight connectivity check without executing any
	// test logic. Returns nil when the adapter is reachable.
	Health(ctx context.Context) error

	// Execute runs the named action with the provided parameters and returns
	// a StepResult containing the adapter's output data and timing.
	Execute(ctx context.Context, action string, params map[string]any) (*tryve.StepResult, error)
}

// MeasureDuration executes fn, measures its wall-clock duration, and returns
// that duration alongside any error fn produced.
func MeasureDuration(fn func() error) (time.Duration, error) {
	start := time.Now()
	err := fn()
	return time.Since(start), err
}

// SuccessResult constructs a StepResult for a successful adapter action.
// data holds the primary output, duration records execution time, and
// metadata carries optional diagnostic key-value pairs.
func SuccessResult(data map[string]any, duration time.Duration, metadata map[string]any) *tryve.StepResult {
	return &tryve.StepResult{
		Data:     data,
		Duration: duration,
		Metadata: metadata,
	}
}

// matchCriteria reads the match conditions for a waitFor action.
//
// The documented spelling is "filter"; "match" is the spelling the adapters
// originally implemented. Both are accepted so that neither a test written
// against the documentation nor one written against the old behaviour is
// silently ignored — a waitFor whose criteria were dropped would wait for the
// first message rather than the right one.
func matchCriteria(params map[string]any) map[string]any {
	if m, ok := params["filter"].(map[string]any); ok && len(m) > 0 {
		return m
	}
	if m, ok := params["match"].(map[string]any); ok && len(m) > 0 {
		return m
	}
	return nil
}
